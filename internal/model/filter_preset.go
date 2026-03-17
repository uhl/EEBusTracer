package model

import "time"

// FilterPreset represents a saved filter configuration.
type FilterPreset struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Filter    string    `json:"filter"`    // JSON-serialized MessageFilter
	CreatedAt time.Time `json:"createdAt"`
}
