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
	defer tx.Rollback() //nolint:errcheck // rollback after commit is a no-op

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

// buildFilterQuery builds the WHERE clause, FROM clause, and args for message filtering.
// It returns the join clause (empty or FTS join), conditions, and args.
func buildFilterQuery(traceID int64, filter MessageFilter) (ftsJoin string, conditions []string, args []interface{}) {
	conditions = append(conditions, "m.trace_id = ?")
	args = append(args, traceID)

	if filter.CmdClassifier != "" {
		conditions = append(conditions, "m.cmd_classifier = ?")
		args = append(args, filter.CmdClassifier)
	}
	if filter.FunctionSet != "" {
		fsets := strings.Split(filter.FunctionSet, ",")
		if len(fsets) == 1 {
			conditions = append(conditions, "m.function_set = ?")
			args = append(args, fsets[0])
		} else {
			placeholders := strings.Repeat("?,", len(fsets))
			placeholders = placeholders[:len(placeholders)-1]
			conditions = append(conditions, "m.function_set IN ("+placeholders+")")
			for _, fs := range fsets {
				args = append(args, fs)
			}
		}
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

	if filter.Search != "" {
		conditions = append(conditions, "messages_fts MATCH ?")
		args = append(args, ftsPrefix(filter.Search))
		ftsJoin = "JOIN messages_fts ON messages_fts.rowid = m.id "
	}

	return ftsJoin, conditions, args
}

// ListMessages returns messages for a trace, with optional filtering and pagination.
func (r *MessageRepo) ListMessages(traceID int64, filter MessageFilter) ([]*model.Message, error) {
	ftsJoin, conditions, args := buildFilterQuery(traceID, filter)

	selectCols := "m.id, m.trace_id, m.sequence_num, m.timestamp, m.direction, m.source_addr, m.dest_addr, " +
		"m.raw_hex, m.normalized_json, m.ship_msg_type, m.ship_payload, m.spine_payload, " +
		"m.cmd_classifier, m.function_set, m.msg_counter, m.msg_counter_ref, " +
		"m.device_source, m.device_dest, m.entity_source, m.entity_dest, " +
		"m.feature_source, m.feature_dest, m.parse_error"

	query := "SELECT " + selectCols + " FROM messages m " + ftsJoin +
		"WHERE " + strings.Join(conditions, " AND ") +
		" ORDER BY m.sequence_num ASC"

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

// CountFilteredMessages returns the total count of messages matching the filter,
// ignoring Limit and Offset. This is useful for pagination metadata.
func (r *MessageRepo) CountFilteredMessages(traceID int64, filter MessageFilter) (int, error) {
	ftsJoin, conditions, args := buildFilterQuery(traceID, filter)

	query := "SELECT COUNT(*) FROM messages m " + ftsJoin +
		"WHERE " + strings.Join(conditions, " AND ")

	var count int
	err := r.db.QueryRow(query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count filtered messages: %w", err)
	}
	return count, nil
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

// FindConversationMessages returns all SPINE data messages between a device pair for a given function set.
// The device pair is bidirectional (A→B and B→A are both included).
// Returns (messages, totalCount, error).
func (r *MessageRepo) FindConversationMessages(traceID int64, deviceA, deviceB, functionSet string, limit, offset int) ([]*model.Message, int, error) {
	where := `m.trace_id = ? AND m.ship_msg_type = 'data' AND m.function_set = ?
		AND ((m.device_source = ? AND m.device_dest = ?) OR (m.device_source = ? AND m.device_dest = ?))`
	args := []interface{}{traceID, functionSet, deviceA, deviceB, deviceB, deviceA}

	// Count total
	var total int
	countQuery := "SELECT COUNT(*) FROM messages m WHERE " + where
	if err := r.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count conversation messages: %w", err)
	}

	// Fetch page
	if limit <= 0 {
		limit = 50
	}
	query := "SELECT " + messageCols("m") + " FROM messages m WHERE " + where +
		" ORDER BY m.sequence_num ASC" +
		fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("find conversation messages: %w", err)
	}
	defer rows.Close()

	msgs, err := scanMessages(rows)
	if err != nil {
		return nil, 0, err
	}
	return msgs, total, nil
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

// FindOrphanedRequestIDs returns message IDs for request-type data messages
// (read, write, call) that have a msgCounter but no other message references
// them via msgCounterRef. Notify and reply messages are excluded because they
// are terminal in SPINE and never expect a response.
func (r *MessageRepo) FindOrphanedRequestIDs(traceID int64) ([]int64, error) {
	rows, err := r.db.Query(
		`SELECT m.id FROM messages m
		 WHERE m.trace_id = ? AND m.msg_counter != '' AND m.ship_msg_type = 'data'
		   AND m.cmd_classifier IN ('read', 'write', 'call')
		   AND NOT EXISTS (
		     SELECT 1 FROM messages m2
		     WHERE m2.trace_id = m.trace_id AND m2.msg_counter_ref = m.msg_counter
		   )
		 ORDER BY m.id`,
		traceID,
	)
	if err != nil {
		return nil, fmt.Errorf("find orphaned request IDs: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan orphaned ID: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
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
