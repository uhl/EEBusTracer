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
)

func TestUDPSource_Name(t *testing.T) {
	src := NewUDPSource("localhost:4712", parser.New(), slog.Default())
	if src.Name() != "udp" {
		t.Errorf("Name() = %q, want %q", src.Name(), "udp")
	}
}

func TestUDPSource_Run(t *testing.T) {
	// Set up a test UDP server
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer conn.Close()
	addr := conn.LocalAddr().(*net.UDPAddr)

	frame := append([]byte{0x01}, []byte(`{"connectionHello":{"phase":"pending"}}`)...)

	go func() {
		buf := make([]byte, 1024)
		_, clientAddr, err := conn.ReadFrom(buf)
		if err != nil {
			return
		}
		conn.WriteTo(frame, clientAddr)
	}()

	src := NewUDPSource(addr.String(), parser.New(), slog.Default())

	var received []*model.Message
	var mu sync.Mutex

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- src.Run(ctx, func(msg *model.Message) {
			mu.Lock()
			received = append(received, msg)
			mu.Unlock()
		})
	}()

	// Wait for message
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
			cancel()
			t.Fatal("timeout waiting for message")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("expected 1 message, got %d", len(received))
	}
	if received[0].Direction != model.DirectionIncoming {
		t.Errorf("Direction = %q, want %q", received[0].Direction, model.DirectionIncoming)
	}
	if received[0].SourceAddr != addr.String() {
		t.Errorf("SourceAddr = %q, want %q", received[0].SourceAddr, addr.String())
	}
}

func TestUDPSource_InvalidTarget(t *testing.T) {
	src := NewUDPSource("invalid-address-no-port", parser.New(), slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := src.Run(ctx, func(msg *model.Message) {})
	if err == nil {
		t.Error("expected error for invalid target")
	}
}
