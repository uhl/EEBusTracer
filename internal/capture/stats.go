package capture

import "sync/atomic"

// CaptureStats holds capture statistics.
type CaptureStats struct {
	PacketsReceived atomic.Int64
	PacketsParsed   atomic.Int64
	ParseErrors     atomic.Int64
	BytesReceived   atomic.Int64
}

// Snapshot returns a copy of the current stats.
func (s *CaptureStats) Snapshot() StatsSnapshot {
	return StatsSnapshot{
		PacketsReceived: s.PacketsReceived.Load(),
		PacketsParsed:   s.PacketsParsed.Load(),
		ParseErrors:     s.ParseErrors.Load(),
		BytesReceived:   s.BytesReceived.Load(),
	}
}

// StatsSnapshot is a point-in-time copy of capture statistics.
type StatsSnapshot struct {
	PacketsReceived int64  `json:"packetsReceived"`
	PacketsParsed   int64  `json:"packetsParsed"`
	ParseErrors     int64  `json:"parseErrors"`
	BytesReceived   int64  `json:"bytesReceived"`
	SourceType      string `json:"sourceType,omitempty"`
}
