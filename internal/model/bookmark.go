package model

import "time"

// Bookmark represents a user-created bookmark on a message.
type Bookmark struct {
	ID        int64     `json:"id"`
	MessageID int64     `json:"messageId"`
	TraceID   int64     `json:"traceId"`
	Label     string    `json:"label"`
	Color     string    `json:"color"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"createdAt"`
}
