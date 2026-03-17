package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
)

// MessageRepo provides CRUD operations for messages.
type MessageRepo struct {
	db *sql.DB
}

// NewMessageRepo creates a new MessageRepo.
func NewMessageRepo(db *DB) *MessageRepo {
	return &MessageRepo{db: db.SqlDB()}
}

// InsertMessage inserts a single message.
func (r *MessageRepo) InsertMessage(m *model.Message) error {
	result, err := r.db.Exec(
		`INSERT INTO messages (trace_id, sequence_num, timestamp, direction, source_addr, dest_addr,
		 raw_hex, normalized_json, ship_msg_type, ship_payload, spine_payload,
		 cmd_classifier, function_set, msg_counter, msg_counter_ref,
		 device_source, device_dest, entity_source, entity_dest,
		 feature_source, feature_dest, parse_error)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.TraceID, m.SequenceNum, m.Timestamp, m.Direction, m.SourceAddr, m.DestAddr,
		m.RawHex, jsonStr(m.NormalizedJSON), m.ShipMsgType, jsonStr(m.ShipPayload), jsonStr(m.SpinePayload),
		m.CmdClassifier, m.FunctionSet, m.MsgCounter, m.MsgCounterRef,
		m.DeviceSource, m.DeviceDest, m.EntitySource, m.EntityDest,
		m.FeatureSource, m.FeatureDest, m.ParseError,
	)
	if err != nil {
		return fmt.Errorf("insert message: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	m.ID = id
	return nil
}

// InsertMessages inserts multiple messages in a single transaction.
func (r *MessageRepo) InsertMessages(msgs []*model.Message) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(
		`INSERT INTO messages (trace_id, sequence_num, timestamp, direction, source_addr, dest_addr,
		 raw_hex, normalized_json, ship_msg_type, ship_payload, spine_payload,
		 cmd_classifier, function_set, msg_counter, msg_counter_ref,
		 device_source, device_dest, entity_source, entity_dest,
		 feature_source, feature_dest, parse_error)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return fmt.Errorf("prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, m := range msgs {
		result, err := stmt.Exec(
			m.TraceID, m.SequenceNum, m.Timestamp, m.Direction, m.SourceAddr, m.DestAddr,
			m.RawHex, jsonStr(m.NormalizedJSON), m.ShipMsgType, jsonStr(m.ShipPayload), jsonStr(m.SpinePayload),
			m.CmdClassifier, m.FunctionSet, m.MsgCounter, m.MsgCounterRef,
			m.DeviceSource, m.DeviceDest, m.EntitySource, m.EntityDest,
			m.FeatureSource, m.FeatureDest, m.ParseError,
		)
		if err != nil {
			return fmt.Errorf("insert message %d: %w", m.SequenceNum, err)
		}
		id, _ := result.LastInsertId()
		m.ID = id
	}

	return tx.Commit()
}

// MessageFilter defines criteria for listing messages.
type MessageFilter struct {
	CmdClassifier string
	FunctionSet   string
	Direction     string
	ShipMsgType   string
	Limit         int
	Offset        int
	Search        string     // FTS5 full-text query
	TimeFrom      *time.Time // filter messages >= this timestamp
	TimeTo        *time.Time // filter messages <= this timestamp
	DeviceSource  string
	DeviceDest    string
	Device        string // matches source OR dest
	EntitySource  string
	EntityDest    string
	FeatureSource string
	FeatureDest   string
}

// GetMessage retrieves a single message by trace ID and message ID.
func (r *MessageRepo) GetMessage(traceID, msgID int64) (*model.Message, error) {
	m := &model.Message{}
	var normalizedJSON, shipPayload, spinePayload sql.NullString
	err := r.db.QueryRow(
		`SELECT id, trace_id, sequence_num, timestamp, direction, source_addr, dest_addr,
		 raw_hex, normalized_json, ship_msg_type, ship_payload, spine_payload,
		 cmd_classifier, function_set, msg_counter, msg_counter_ref,
		 device_source, device_dest, entity_source, entity_dest,
		 feature_source, feature_dest, parse_error
		 FROM messages WHERE trace_id = ? AND id = ?`, traceID, msgID,
	).Scan(&m.ID, &m.TraceID, &m.SequenceNum, &m.Timestamp, &m.Direction, &m.SourceAddr, &m.DestAddr,
		&m.RawHex, &normalizedJSON, &m.ShipMsgType, &shipPayload, &spinePayload,
		&m.CmdClassifier, &m.FunctionSet, &m.MsgCounter, &m.MsgCounterRef,
		&m.DeviceSource, &m.DeviceDest, &m.EntitySource, &m.EntityDest,
		&m.FeatureSource, &m.FeatureDest, &m.ParseError)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get message: %w", err)
	}
	m.NormalizedJSON = nullToRawMessage(normalizedJSON)
	m.ShipPayload = nullToRawMessage(shipPayload)
	m.SpinePayload = nullToRawMessage(spinePayload)
	return m, nil
}

// ftsPrefix converts a plain search string into an FTS5 prefix query so that
// partial terms match. Each whitespace-separated token gets a trailing "*"
// appended (e.g. "LoadContro" becomes "LoadContro*"), which makes FTS5 match
// any token that starts with the given prefix.
func ftsPrefix(search string) string {
	words := strings.Fields(search)
	for i, w := range words {
		if !strings.HasSuffix(w, "*") {
			words[i] = w + "*"
		}
	}
	return strings.Join(words, " ")
}

// ListMessages returns messages for a trace, with optional filtering and pagination.
func (r *MessageRepo) ListMessages(traceID int64, filter MessageFilter) ([]*model.Message, error) {
	var conditions []string
	var args []interface{}
	useFTS := filter.Search != ""

	conditions = append(conditions, "m.trace_id = ?")
	args = append(args, traceID)

	if filter.CmdClassifier != "" {
		conditions = append(conditions, "m.cmd_classifier = ?")
		args = append(args, filter.CmdClassifier)
	}
	if filter.FunctionSet != "" {
		conditions = append(conditions, "m.function_set = ?")
		args = append(args, filter.FunctionSet)
	}
	if filter.Direction != "" {
		conditions = append(conditions, "m.direction = ?")
		args = append(args, filter.Direction)
	}
	if filter.ShipMsgType != "" {
		conditions = append(conditions, "m.ship_msg_type = ?")
		args = append(args, filter.ShipMsgType)
	}
	if filter.TimeFrom != nil {
		conditions = append(conditions, "m.timestamp >= ?")
		args = append(args, *filter.TimeFrom)
	}
	if filter.TimeTo != nil {
		conditions = append(conditions, "m.timestamp <= ?")
		args = append(args, *filter.TimeTo)
	}
	if filter.DeviceSource != "" {
		conditions = append(conditions, "m.device_source = ?")
		args = append(args, filter.DeviceSource)
	}
	if filter.DeviceDest != "" {
		conditions = append(conditions, "m.device_dest = ?")
		args = append(args, filter.DeviceDest)
	}
	if filter.Device != "" {
		conditions = append(conditions, "(m.device_source = ? OR m.device_dest = ?)")
		args = append(args, filter.Device, filter.Device)
	}
	if filter.EntitySource != "" {
		conditions = append(conditions, "m.entity_source = ?")
		args = append(args, filter.EntitySource)
	}
	if filter.EntityDest != "" {
		conditions = append(conditions, "m.entity_dest = ?")
		args = append(args, filter.EntityDest)
	}
	if filter.FeatureSource != "" {
		conditions = append(conditions, "m.feature_source = ?")
		args = append(args, filter.FeatureSource)
	}
	if filter.FeatureDest != "" {
		conditions = append(conditions, "m.feature_dest = ?")
		args = append(args, filter.FeatureDest)
	}

	selectCols := "m.id, m.trace_id, m.sequence_num, m.timestamp, m.direction, m.source_addr, m.dest_addr, " +
		"m.raw_hex, m.normalized_json, m.ship_msg_type, m.ship_payload, m.spine_payload, " +
		"m.cmd_classifier, m.function_set, m.msg_counter, m.msg_counter_ref, " +
		"m.device_source, m.device_dest, m.entity_source, m.entity_dest, " +
		"m.feature_source, m.feature_dest, m.parse_error"

	var query string
	if useFTS {
		conditions = append(conditions, "messages_fts MATCH ?")
		args = append(args, ftsPrefix(filter.Search))
		query = "SELECT " + selectCols + " FROM messages m " +
			"JOIN messages_fts ON messages_fts.rowid = m.id " +
			"WHERE " + strings.Join(conditions, " AND ") +
			" ORDER BY m.sequence_num ASC"
	} else {
		query = "SELECT " + selectCols + " FROM messages m " +
			"WHERE " + strings.Join(conditions, " AND ") +
			" ORDER BY m.sequence_num ASC"
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	query += fmt.Sprintf(" LIMIT %d", limit)
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()

	return scanMessages(rows)
}

// FindByMsgCounter returns messages where msg_counter matches the given value.
func (r *MessageRepo) FindByMsgCounter(traceID int64, msgCounter string) ([]*model.Message, error) {
	rows, err := r.db.Query(
		"SELECT "+messageCols("m")+" FROM messages m WHERE m.trace_id = ? AND m.msg_counter = ? ORDER BY m.sequence_num ASC",
		traceID, msgCounter,
	)
	if err != nil {
		return nil, fmt.Errorf("find by msg counter: %w", err)
	}
	defer rows.Close()
	return scanMessages(rows)
}

// FindByMsgCounterRef returns messages where msg_counter_ref matches the given value.
func (r *MessageRepo) FindByMsgCounterRef(traceID int64, msgCounterRef string) ([]*model.Message, error) {
	rows, err := r.db.Query(
		"SELECT "+messageCols("m")+" FROM messages m WHERE m.trace_id = ? AND m.msg_counter_ref = ? ORDER BY m.sequence_num ASC",
		traceID, msgCounterRef,
	)
	if err != nil {
		return nil, fmt.Errorf("find by msg counter ref: %w", err)
	}
	defer rows.Close()
	return scanMessages(rows)
}

func messageCols(alias string) string {
	return alias + ".id, " + alias + ".trace_id, " + alias + ".sequence_num, " + alias + ".timestamp, " +
		alias + ".direction, " + alias + ".source_addr, " + alias + ".dest_addr, " +
		alias + ".raw_hex, " + alias + ".normalized_json, " + alias + ".ship_msg_type, " +
		alias + ".ship_payload, " + alias + ".spine_payload, " +
		alias + ".cmd_classifier, " + alias + ".function_set, " + alias + ".msg_counter, " + alias + ".msg_counter_ref, " +
		alias + ".device_source, " + alias + ".device_dest, " + alias + ".entity_source, " + alias + ".entity_dest, " +
		alias + ".feature_source, " + alias + ".feature_dest, " + alias + ".parse_error"
}

func scanMessages(rows *sql.Rows) ([]*model.Message, error) {
	var messages []*model.Message
	for rows.Next() {
		m := &model.Message{}
		var normalizedJSON, shipPayload, spinePayload sql.NullString
		if err := rows.Scan(&m.ID, &m.TraceID, &m.SequenceNum, &m.Timestamp, &m.Direction, &m.SourceAddr, &m.DestAddr,
			&m.RawHex, &normalizedJSON, &m.ShipMsgType, &shipPayload, &spinePayload,
			&m.CmdClassifier, &m.FunctionSet, &m.MsgCounter, &m.MsgCounterRef,
			&m.DeviceSource, &m.DeviceDest, &m.EntitySource, &m.EntityDest,
			&m.FeatureSource, &m.FeatureDest, &m.ParseError); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		m.NormalizedJSON = nullToRawMessage(normalizedJSON)
		m.ShipPayload = nullToRawMessage(shipPayload)
		m.SpinePayload = nullToRawMessage(spinePayload)
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

// ListDistinctFunctionSets returns unique non-empty function set values for data messages in a trace.
func (r *MessageRepo) ListDistinctFunctionSets(traceID int64) ([]string, error) {
	rows, err := r.db.Query(
		`SELECT DISTINCT function_set FROM messages WHERE trace_id = ? AND ship_msg_type = 'data' AND function_set != '' ORDER BY function_set`,
		traceID,
	)
	if err != nil {
		return nil, fmt.Errorf("list distinct function sets: %w", err)
	}
	defer rows.Close()

	var result []string
	for rows.Next() {
		var fs string
		if err := rows.Scan(&fs); err != nil {
			return nil, fmt.Errorf("scan function set: %w", err)
		}
		result = append(result, fs)
	}
	return result, rows.Err()
}

// CountMessages returns the total number of messages for a trace.
func (r *MessageRepo) CountMessages(traceID int64) (int, error) {
	var count int
	err := r.db.QueryRow("SELECT COUNT(*) FROM messages WHERE trace_id = ?", traceID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count messages: %w", err)
	}
	return count, nil
}

func jsonStr(data json.RawMessage) *string {
	if len(data) == 0 {
		return nil
	}
	s := string(data)
	return &s
}

func nullToRawMessage(ns sql.NullString) json.RawMessage {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	return json.RawMessage(ns.String)
}
