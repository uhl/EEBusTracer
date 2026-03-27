package capture

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
	"github.com/eebustracer/eebustracer/internal/parser"
	"github.com/eebustracer/eebustracer/internal/store"
)

// MessageCallback is called for each parsed message during capture.
type MessageCallback func(*model.Message)

// Engine manages packet capture and processing.
type Engine struct {
	parser  *parser.Parser
	msgRepo *store.MessageRepo
	logger  *slog.Logger

	mu         sync.Mutex
	cancel     context.CancelFunc
	targetAddr string
	sourceType string
	capturing  bool
	traceID    int64
	seqNum     int
	stats      CaptureStats
	callbacks  []MessageCallback
}

const maxUDPSize = 65535

// NewEngine creates a new capture engine.
func NewEngine(p *parser.Parser, msgRepo *store.MessageRepo, logger *slog.Logger) *Engine {
	return &Engine{
		parser:  p,
		msgRepo: msgRepo,
		logger:  logger,
	}
}

// OnMessage registers a callback that fires for each parsed message.
func (e *Engine) OnMessage(cb MessageCallback) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.callbacks = append(e.callbacks, cb)
}

// Start connects to the EEBus stack at the given address and begins
// receiving SHIP frames. This is a convenience wrapper around StartWithSource
// that creates a UDPSource.
func (e *Engine) Start(traceID int64, target string) error {
	src := NewUDPSource(target, e.parser, e.logger)
	return e.StartWithSource(traceID, src, target)
}

// StartWithSource begins capturing using the provided Source.
// The targetAddr parameter is stored for status reporting (can be empty for
// sources that don't have a network target).
func (e *Engine) StartWithSource(traceID int64, src Source, targetAddr string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.capturing {
		return fmt.Errorf("capture already in progress")
	}

	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel
	e.targetAddr = targetAddr
	e.sourceType = src.Name()
	e.capturing = true
	e.traceID = traceID
	e.seqNum = 0
	e.stats = CaptureStats{}

	go e.runSource(ctx, src)

	e.logger.Info("capture started",
		"source", src.Name(),
		"target", targetAddr,
		"traceID", traceID,
	)
	return nil
}

// Stop halts the capture.
func (e *Engine) Stop() error {
	e.mu.Lock()
	if !e.capturing {
		e.mu.Unlock()
		return nil
	}
	cancel := e.cancel
	e.capturing = false
	e.mu.Unlock()

	// Cancel the context to signal the source to stop.
	cancel()

	e.logger.Info("capture stopped", "traceID", e.traceID)
	return nil
}

// IsCapturing returns whether the engine is currently capturing.
func (e *Engine) IsCapturing() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.capturing
}

// Stats returns a snapshot of capture statistics.
func (e *Engine) Stats() StatsSnapshot {
	return e.stats.Snapshot()
}

// TraceID returns the current trace ID.
func (e *Engine) TraceID() int64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.traceID
}

// TargetAddr returns the address of the EEBus stack being traced.
func (e *Engine) TargetAddr() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.targetAddr
}

// SourceType returns the type of the active capture source.
func (e *Engine) SourceType() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.sourceType
}

// Parser returns the engine's parser instance.
func (e *Engine) Parser() *parser.Parser {
	return e.parser
}

// runSource runs the given Source and processes emitted messages.
func (e *Engine) runSource(ctx context.Context, src Source) {
	err := src.Run(ctx, func(msg *model.Message) {
		ts := time.Now()

		e.stats.PacketsReceived.Add(1)
		if len(msg.RawBytes) > 0 {
			e.stats.BytesReceived.Add(int64(len(msg.RawBytes)))
		}

		e.mu.Lock()
		e.seqNum++
		seqNum := e.seqNum
		traceID := e.traceID
		e.mu.Unlock()

		// If the source provides raw bytes but no parsed fields, parse them.
		if len(msg.RawBytes) > 0 && msg.ShipMsgType == "" {
			parsed := e.parser.Parse(msg.RawBytes, traceID, seqNum, ts)
			// Copy parsed fields into msg, preserving source-set fields.
			parsed.SourceAddr = msg.SourceAddr
			parsed.Direction = msg.Direction
			msg = parsed
		} else {
			// Source already parsed — just fill in trace/seq fields.
			msg.TraceID = traceID
			msg.SequenceNum = seqNum
			if msg.Timestamp.IsZero() {
				msg.Timestamp = ts
			}
			if msg.RawHex == "" && len(msg.RawBytes) > 0 {
				msg.RawHex = hex.EncodeToString(msg.RawBytes)
			}
		}

		e.stats.PacketsParsed.Add(1)
		if msg.ParseError != "" {
			e.stats.ParseErrors.Add(1)
		}

		// Insert into database immediately so msg.ID is set before
		// callbacks fire (WebSocket clients need the ID to fetch details).
		if err := e.msgRepo.InsertMessage(msg); err != nil {
			e.logger.Error("insert message", "error", err, "seq", msg.SequenceNum)
		}

		// Notify callbacks (msg.ID is now set).
		e.mu.Lock()
		cbs := make([]MessageCallback, len(e.callbacks))
		copy(cbs, e.callbacks)
		e.mu.Unlock()
		for _, cb := range cbs {
			cb(msg)
		}
	})

	// Reset capturing state so a new capture can be started.
	e.mu.Lock()
	e.capturing = false
	e.mu.Unlock()

	if err != nil {
		e.logger.Error("source stopped with error", "source", src.Name(), "error", err)
	} else {
		e.logger.Info("source stopped", "source", src.Name())
	}
}
