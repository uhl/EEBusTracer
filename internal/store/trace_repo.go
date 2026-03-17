package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
)

// TraceRepo provides CRUD operations for traces.
type TraceRepo struct {
	db *sql.DB
}

// NewTraceRepo creates a new TraceRepo.
func NewTraceRepo(db *DB) *TraceRepo {
	return &TraceRepo{db: db.SqlDB()}
}

// CreateTrace inserts a new trace.
func (r *TraceRepo) CreateTrace(t *model.Trace) error {
	result, err := r.db.Exec(
		`INSERT INTO traces (name, description, started_at, stopped_at, message_count, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		t.Name, t.Description, t.StartedAt, t.StoppedAt, t.MessageCount, t.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert trace: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	t.ID = id
	return nil
}

// GetTrace retrieves a trace by ID.
func (r *TraceRepo) GetTrace(id int64) (*model.Trace, error) {
	t := &model.Trace{}
	err := r.db.QueryRow(
		`SELECT id, name, description, started_at, stopped_at, message_count, created_at
		 FROM traces WHERE id = ?`, id,
	).Scan(&t.ID, &t.Name, &t.Description, &t.StartedAt, &t.StoppedAt, &t.MessageCount, &t.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get trace: %w", err)
	}
	return t, nil
}

// ListTraces returns all traces ordered by creation time (newest first).
func (r *TraceRepo) ListTraces() ([]*model.Trace, error) {
	rows, err := r.db.Query(
		`SELECT id, name, description, started_at, stopped_at, message_count, created_at
		 FROM traces ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list traces: %w", err)
	}
	defer rows.Close()

	var traces []*model.Trace
	for rows.Next() {
		t := &model.Trace{}
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.StartedAt, &t.StoppedAt, &t.MessageCount, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan trace: %w", err)
		}
		traces = append(traces, t)
	}
	return traces, rows.Err()
}

// UpdateTrace updates a trace's name and description.
func (r *TraceRepo) UpdateTrace(t *model.Trace) error {
	_, err := r.db.Exec(
		`UPDATE traces SET name = ?, description = ?, message_count = ? WHERE id = ?`,
		t.Name, t.Description, t.MessageCount, t.ID,
	)
	if err != nil {
		return fmt.Errorf("update trace: %w", err)
	}
	return nil
}

// StopTrace sets the stopped_at timestamp and updates the message count.
func (r *TraceRepo) StopTrace(id int64, stoppedAt time.Time, messageCount int) error {
	_, err := r.db.Exec(
		`UPDATE traces SET stopped_at = ?, message_count = ? WHERE id = ?`,
		stoppedAt, messageCount, id,
	)
	if err != nil {
		return fmt.Errorf("stop trace: %w", err)
	}
	return nil
}

// DeleteTrace removes a trace and its associated messages (via CASCADE).
func (r *TraceRepo) DeleteTrace(id int64) error {
	_, err := r.db.Exec(`DELETE FROM traces WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete trace: %w", err)
	}
	return nil
}
