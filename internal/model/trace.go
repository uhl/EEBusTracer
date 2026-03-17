package model

import "time"

// Trace represents a recording session of EEBus messages.
type Trace struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	Description  string     `json:"description,omitempty"`
	StartedAt    time.Time  `json:"startedAt"`
	StoppedAt    *time.Time `json:"stoppedAt,omitempty"`
	MessageCount int        `json:"messageCount"`
	CreatedAt    time.Time  `json:"createdAt"`
}

// IsRecording returns true if the trace has not been stopped.
func (t *Trace) IsRecording() bool {
	return t.StoppedAt == nil
}
