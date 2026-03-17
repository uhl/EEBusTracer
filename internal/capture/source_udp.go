package capture

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"github.com/eebustracer/eebustracer/internal/model"
	"github.com/eebustracer/eebustracer/internal/parser"
)

// UDPSource captures SHIP frames from an EEBus stack over UDP.
type UDPSource struct {
	target string
	parser *parser.Parser
	logger *slog.Logger
}

// NewUDPSource creates a new UDP source that connects to the given target
// address (host:port).
func NewUDPSource(target string, p *parser.Parser, logger *slog.Logger) *UDPSource {
	return &UDPSource{
		target: target,
		parser: p,
		logger: logger,
	}
}

// Name returns "udp".
func (s *UDPSource) Name() string { return "udp" }

// Run connects to the EEBus stack, sends a registration byte, and loops
// reading UDP packets until the context is cancelled.
func (s *UDPSource) Run(ctx context.Context, emit func(*model.Message)) error {
	conn, err := net.Dial("udp", s.target)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", s.target, err)
	}

	// Close connection when context is cancelled to unblock Read.
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	// Send a registration packet to let the EEBus stack know we're listening.
	if _, err := conn.Write([]byte{0x00}); err != nil {
		conn.Close()
		return fmt.Errorf("send registration to %s: %w", s.target, err)
	}

	buf := make([]byte, maxUDPSize)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			// Check if context was cancelled (normal shutdown).
			if ctx.Err() != nil {
				return nil
			}
			s.logger.Error("udp read error", "error", err)
			continue
		}

		raw := make([]byte, n)
		copy(raw, buf[:n])

		msg := &model.Message{
			RawBytes:  raw,
			SourceAddr: s.target,
			Direction:  model.DirectionIncoming,
		}

		emit(msg)
	}
}
