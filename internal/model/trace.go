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

	// SkippedTruncated counts EEBus frames that were detected but discarded
	// because the source (e.g. DLT verbose string arg) truncated the payload
	// mid-JSON. Storing them would produce garbage rows; hiding them entirely
	// would mislead users into thinking those slots were quiet. Surfaced in
	// the trace header instead.
	SkippedTruncated int `json:"skippedTruncated,omitempty"`
}

// IsRecording returns true if the trace has not been stopped.
func (t *Trace) IsRecording() bool {
	return t.StoppedAt == nil
}
