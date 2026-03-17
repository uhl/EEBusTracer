package capture

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
	"github.com/eebustracer/eebustracer/internal/parser"
	"github.com/eebustracer/eebustracer/internal/store"
)

func setupEngine(t *testing.T) (*Engine, *store.DB) {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Create a test trace
	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{
		Name:      "test",
		StartedAt: time.Now(),
		CreatedAt: time.Now(),
	}
	if err := traceRepo.CreateTrace(trace); err != nil {
		t.Fatalf("CreateTrace failed: %v", err)
	}

	msgRepo := store.NewMessageRepo(db)
	p := parser.New()
	logger := slog.Default()

	engine := NewEngine(p, msgRepo, logger)
	return engine, db
}

// startTestEEBusStack simulates an EEBus stack that listens on a UDP port,
// waits for a registration packet, and then sends SHIP frames back.
func startTestEEBusStack(t *testing.T, frames [][]byte) *net.UDPAddr {
	t.Helper()

	// Listen on an ephemeral port
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	addr := conn.LocalAddr().(*net.UDPAddr)

	go func() {
		buf := make([]byte, 1024)
		// Wait for registration packet from the tracer
		_, clientAddr, err := conn.ReadFrom(buf)
		if err != nil {
			return
		}

		// Send SHIP frames back to the tracer
		for _, frame := range frames {
			conn.WriteTo(frame, clientAddr)
			time.Sleep(10 * time.Millisecond) // Small delay between frames
		}
	}()

	return addr
}

func TestEngine_StartStop(t *testing.T) {
	engine, _ := setupEngine(t)

	addr := startTestEEBusStack(t, nil)
	target := addr.String()

	if err := engine.Start(1, target); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if !engine.IsCapturing() {
		t.Error("expected IsCapturing() to be true")
	}

	if engine.TargetAddr() != target {
		t.Errorf("TargetAddr() = %q, want %q", engine.TargetAddr(), target)
	}

	if engine.SourceType() != "udp" {
		t.Errorf("SourceType() = %q, want %q", engine.SourceType(), "udp")
	}

	if err := engine.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if engine.IsCapturing() {
		t.Error("expected IsCapturing() to be false after Stop")
	}
}

func TestEngine_ReceiveAndCallback(t *testing.T) {
	engine, _ := setupEngine(t)

	var received []*model.Message
	var mu sync.Mutex
	engine.OnMessage(func(msg *model.Message) {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
	})

	// SHIP frame: CMI header + connectionHello JSON
	frame := append([]byte{0x01}, []byte(`{"connectionHello":{"phase":"pending"}}`)...)
	addr := startTestEEBusStack(t, [][]byte{frame})

	if err := engine.Start(1, addr.String()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for the message to be processed
	deadline := time.After(2 * time.Second)
	for {
		mu.Lock()
		count := len(received)
		mu.Unlock()
		if count > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timeout waiting for message")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	if err := engine.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("expected 1 message, got %d", len(received))
	}
	if received[0].ShipMsgType != model.ShipMsgTypeConnectionHello {
		t.Errorf("ShipMsgType = %q, want %q", received[0].ShipMsgType, model.ShipMsgTypeConnectionHello)
	}
}

func TestEngine_Stats(t *testing.T) {
	engine, _ := setupEngine(t)

	frame := []byte{0x00} // SHIP init
	addr := startTestEEBusStack(t, [][]byte{frame})

	if err := engine.Start(1, addr.String()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for processing
	deadline := time.After(2 * time.Second)
	for {
		stats := engine.Stats()
		if stats.PacketsReceived > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timeout waiting for stats update")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	if err := engine.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	stats := engine.Stats()
	if stats.PacketsReceived < 1 {
		t.Errorf("PacketsReceived = %d, want >= 1", stats.PacketsReceived)
	}
	if stats.BytesReceived < 1 {
		t.Errorf("BytesReceived = %d, want >= 1", stats.BytesReceived)
	}
}

// mockSource is a test Source that emits predefined messages.
type mockSource struct {
	name     string
	messages []*model.Message
}

func (m *mockSource) Name() string { return m.name }

func (m *mockSource) Run(ctx context.Context, emit func(*model.Message)) error {
	for _, msg := range m.messages {
		select {
		case <-ctx.Done():
			return nil
		default:
			emit(msg)
		}
	}
	// Wait for context cancellation
	<-ctx.Done()
	return nil
}

func TestEngine_StartWithSource(t *testing.T) {
	engine, _ := setupEngine(t)

	var received []*model.Message
	var mu sync.Mutex
	engine.OnMessage(func(msg *model.Message) {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
	})

	src := &mockSource{
		name: "test",
		messages: []*model.Message{
			{
				ShipMsgType:   model.ShipMsgTypeData,
				Direction:     model.DirectionIncoming,
				CmdClassifier: "read",
				FunctionSet:   "TestFunction",
				Timestamp:     time.Now(),
			},
			{
				ShipMsgType:   model.ShipMsgTypeData,
				Direction:     model.DirectionOutgoing,
				CmdClassifier: "reply",
				FunctionSet:   "TestFunction",
				Timestamp:     time.Now(),
			},
		},
	}

	if err := engine.StartWithSource(1, src, "test-target"); err != nil {
		t.Fatalf("StartWithSource failed: %v", err)
	}

	if engine.SourceType() != "test" {
		t.Errorf("SourceType() = %q, want %q", engine.SourceType(), "test")
	}

	// Wait for messages
	deadline := time.After(2 * time.Second)
	for {
		mu.Lock()
		count := len(received)
		mu.Unlock()
		if count >= 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timeout waiting for messages")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	if err := engine.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(received))
	}
	if received[0].SequenceNum != 1 {
		t.Errorf("received[0].SequenceNum = %d, want 1", received[0].SequenceNum)
	}
	if received[1].SequenceNum != 2 {
		t.Errorf("received[1].SequenceNum = %d, want 2", received[1].SequenceNum)
	}
}

func TestEngine_CallbackHasDBID(t *testing.T) {
	engine, _ := setupEngine(t)

	var received []*model.Message
	var mu sync.Mutex
	engine.OnMessage(func(msg *model.Message) {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
	})

	src := &mockSource{
		name: "test",
		messages: []*model.Message{
			{
				ShipMsgType:   model.ShipMsgTypeData,
				Direction:     model.DirectionIncoming,
				CmdClassifier: "read",
				FunctionSet:   "TestFunction",
				Timestamp:     time.Now(),
			},
		},
	}

	if err := engine.StartWithSource(1, src, ""); err != nil {
		t.Fatalf("StartWithSource failed: %v", err)
	}

	deadline := time.After(2 * time.Second)
	for {
		mu.Lock()
		count := len(received)
		mu.Unlock()
		if count >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timeout waiting for message")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	engine.Stop()

	mu.Lock()
	defer mu.Unlock()

	// The callback must receive a message with a valid database ID,
	// so that WebSocket clients can fetch it via the API.
	if received[0].ID == 0 {
		t.Error("expected msg.ID to be set (non-zero) in callback, got 0")
	}
}

func TestEngine_DoubleStart(t *testing.T) {
	engine, _ := setupEngine(t)

	src := &mockSource{name: "test"}
	if err := engine.StartWithSource(1, src, ""); err != nil {
		t.Fatalf("first Start failed: %v", err)
	}

	err := engine.StartWithSource(2, src, "")
	if err == nil {
		t.Error("expected error on double start")
	}

	engine.Stop()
}
