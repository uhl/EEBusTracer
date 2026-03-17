package store

import (
	"database/sql"
	"fmt"

	"github.com/eebustracer/eebustracer/internal/model"
)

// BookmarkRepo provides CRUD operations for bookmarks.
type BookmarkRepo struct {
	db *sql.DB
}

// NewBookmarkRepo creates a new BookmarkRepo.
func NewBookmarkRepo(db *DB) *BookmarkRepo {
	return &BookmarkRepo{db: db.SqlDB()}
}

// Create inserts a new bookmark.
func (r *BookmarkRepo) Create(b *model.Bookmark) error {
	result, err := r.db.Exec(
		`INSERT INTO bookmarks (message_id, trace_id, label, color, note, created_at)
		 VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		b.MessageID, b.TraceID, b.Label, b.Color, b.Note,
	)
	if err != nil {
		return fmt.Errorf("insert bookmark: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	b.ID = id
	return nil
}

// List returns all bookmarks for a trace.
func (r *BookmarkRepo) List(traceID int64) ([]*model.Bookmark, error) {
	rows, err := r.db.Query(
		`SELECT id, message_id, trace_id, label, color, note, created_at
		 FROM bookmarks WHERE trace_id = ? ORDER BY created_at ASC`, traceID,
	)
	if err != nil {
		return nil, fmt.Errorf("list bookmarks: %w", err)
	}
	defer rows.Close()

	var bookmarks []*model.Bookmark
	for rows.Next() {
		b := &model.Bookmark{}
		if err := rows.Scan(&b.ID, &b.MessageID, &b.TraceID, &b.Label, &b.Color, &b.Note, &b.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan bookmark: %w", err)
		}
		bookmarks = append(bookmarks, b)
	}
	return bookmarks, rows.Err()
}

// GetByMessage retrieves a bookmark by message ID.
func (r *BookmarkRepo) GetByMessage(messageID int64) (*model.Bookmark, error) {
	b := &model.Bookmark{}
	err := r.db.QueryRow(
		`SELECT id, message_id, trace_id, label, color, note, created_at
		 FROM bookmarks WHERE message_id = ?`, messageID,
	).Scan(&b.ID, &b.MessageID, &b.TraceID, &b.Label, &b.Color, &b.Note, &b.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get bookmark by message: %w", err)
	}
	return b, nil
}

// Delete removes a bookmark by ID.
func (r *BookmarkRepo) Delete(id int64) error {
	_, err := r.db.Exec(`DELETE FROM bookmarks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete bookmark: %w", err)
	}
	return nil
}
