package model

import "time"

// Device represents an EEBus device seen in a trace.
type Device struct {
	ID          int64     `json:"id"`
	TraceID     int64     `json:"traceId"`
	DeviceAddr  string    `json:"deviceAddr"`
	SKI         string    `json:"ski,omitempty"`
	Brand       string    `json:"brand,omitempty"`
	Model       string    `json:"model,omitempty"`
	DeviceType  string    `json:"deviceType,omitempty"`
	FirstSeenAt time.Time `json:"firstSeenAt"`
	LastSeenAt  time.Time `json:"lastSeenAt"`
}
