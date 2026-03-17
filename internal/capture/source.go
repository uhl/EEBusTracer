package capture

import (
	"context"

	"github.com/eebustracer/eebustracer/internal/model"
)

// Source is a pluggable message input for the capture engine.
// Implementations read messages from a specific transport (UDP, log file, etc.)
// and emit them via the provided callback.
type Source interface {
	// Name returns a short identifier for this source type (e.g. "udp", "logtail").
	Name() string

	// Run reads messages and calls emit for each one. It blocks until the
	// context is cancelled or an unrecoverable error occurs.
	Run(ctx context.Context, emit func(*model.Message)) error
}
