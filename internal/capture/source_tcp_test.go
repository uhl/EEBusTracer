package capture

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
	"github.com/eebustracer/eebustracer/internal/parser"
)

// sampleJSON returns a valid EEBUS SPINE JSON payload for testing.
func sampleJSON(funcSet string) string {
	return `{"datagram":[{"header":[{"specificationVersion":"1.3.0"},{"addressSource":[{"device":"d:_i:_Dev"},{"entity":[0]},{"feature":0}]},{"addressDestination":[{"device":"d:_i:_Other"},{"entity":[0]},{"feature":0}]},{"msgCounter":1},{"cmdClassifier":"read"}]},{"payload":[{"cmd":[[{"` + funcSet + `":[]}]]}]}]}`
}

// waitForMessages polls until at least wantCount messages are received or timeout.
func waitForMessages(t *testing.T, mu *sync.Mutex, received *[]*model.Message, wantCount int) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		mu.Lock()
		count := len(*received)
		mu.Unlock()
		if count >= wantCount {
			return
		}
		select {
		case <-deadline:
			mu.Lock()
			t.Fatalf("timeout: wanted %d messages, got %d", wantCount, len(*received))
			mu.Unlock()
			return
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}
}

func TestTCPSource_Name(t *testing.T) {
	src := NewTCPSource("localhost:12345", parser.New(), slog.Default())
	if src.Name() != "tcp" {
		t.Errorf("Name() = %q, want %q", src.Name(), "tcp")
	}
}

func TestTCPSource_Run(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	line1 := `[11:38:26.008] SEND to ship_Volvo-CEM-400000270_0xaff223b8 MSG: ` + sampleJSON("nodeManagementDetailedDiscoveryData") + "\r\n"
	line2 := `[11:38:26.016] RECV from ship_Volvo-CEM-400000270_0xaff223b8 MSG: ` + sampleJSON("deviceClassificationManufacturerData") + "\r\n"

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.Write([]byte(line1))
		conn.Write([]byte(line2))
		buf := make([]byte, 1)
		conn.Read(buf)
	}()

	src := NewTCPSource(ln.Addr().String(), parser.New(), slog.Default())
	ctx, cancel := context.WithCancel(context.Background())

	var received []*model.Message
	var mu sync.Mutex

	done := make(chan error, 1)
	go func() {
		done <- src.Run(ctx, func(msg *model.Message) {
			mu.Lock()
			received = append(received, msg)
			mu.Unlock()
		})
	}()

	waitForMessages(t, &mu, &received, 2)
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()

	if len(received) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(received))
	}
	if received[0].Direction != model.DirectionOutgoing {
		t.Errorf("msg[0].Direction = %q, want %q", received[0].Direction, model.DirectionOutgoing)
	}
	if received[0].CmdClassifier != "read" {
		t.Errorf("msg[0].CmdClassifier = %q, want %q", received[0].CmdClassifier, "read")
	}
	if received[1].Direction != model.DirectionIncoming {
		t.Errorf("msg[1].Direction = %q, want %q", received[1].Direction, model.DirectionIncoming)
	}
}

func TestTCPSource_ConnectionRefused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	src := NewTCPSource(addr, parser.New(), slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = src.Run(ctx, func(msg *model.Message) {})
	if err == nil {
		t.Error("expected error for connection refused")
	}
}

func TestTCPSource_Shutdown(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 1024)
		for {
			_, err := conn.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	src := NewTCPSource(ln.Addr().String(), parser.New(), slog.Default())
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- src.Run(ctx, func(msg *model.Message) {})
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected nil error on shutdown, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for source to stop")
	}
}

func TestTCPSource_BinaryPaddingStripped(t *testing.T) {
	tests := []struct {
		name    string
		padding string
	}{
		{"trailing null bytes", "\x00\x00\x00"},
		{"trailing 0xFF bytes", "\xff\xff\xff"},
		{"mixed binary padding", "\x00\xff\x00\xff"},
		{"valid UTF-8 garbage", "Ý\xc3\x9dgarbage"},
		{"printable ASCII garbage", "XXXX"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatalf("listen: %v", err)
			}
			defer ln.Close()

			go func() {
				conn, err := ln.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				line := `[11:38:26.008] SEND to ship_Device_0xaabb MSG: ` + sampleJSON("nodeManagementDetailedDiscoveryData")
				conn.Write([]byte(line + tt.padding + "\r\n"))
				buf := make([]byte, 1)
				conn.Read(buf)
			}()

			src := NewTCPSource(ln.Addr().String(), parser.New(), slog.Default())
			ctx, cancel := context.WithCancel(context.Background())

			var received []*model.Message
			var mu sync.Mutex

			done := make(chan error, 1)
			go func() {
				done <- src.Run(ctx, func(msg *model.Message) {
					mu.Lock()
					received = append(received, msg)
					mu.Unlock()
				})
			}()

			waitForMessages(t, &mu, &received, 1)
			cancel()
			<-done

			mu.Lock()
			defer mu.Unlock()

			if len(received) != 1 {
				t.Fatalf("expected 1 message, got %d", len(received))
			}
			msg := received[0]
			if msg.NormalizedJSON == nil {
				t.Fatal("NormalizedJSON is nil")
			}
			if !json.Valid(msg.NormalizedJSON) {
				t.Fatalf("NormalizedJSON is not valid JSON: %q", string(msg.NormalizedJSON))
			}
			if msg.Direction != model.DirectionOutgoing {
				t.Errorf("msg.Direction = %q, want %q", msg.Direction, model.DirectionOutgoing)
			}
		})
	}
}

func TestTCPSource_LeadingBinaryGarbage(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	jsonPayload := sampleJSON("nodeManagementDetailedDiscoveryData")
	data := "\x00\xff\xdd\xaa\x00\x00\x01E[13:33:06.915] SEND to ship_Katek-CEM-200000414_0x11c4c18 MSG: " + jsonPayload + "\x00\x00\xff\r\n"

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.Write([]byte(data))
		buf := make([]byte, 1)
		conn.Read(buf)
	}()

	src := NewTCPSource(ln.Addr().String(), parser.New(), slog.Default())
	ctx, cancel := context.WithCancel(context.Background())

	var received []*model.Message
	var mu sync.Mutex

	done := make(chan error, 1)
	go func() {
		done <- src.Run(ctx, func(msg *model.Message) {
			mu.Lock()
			received = append(received, msg)
			mu.Unlock()
		})
	}()

	waitForMessages(t, &mu, &received, 1)
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()

	if len(received) != 1 {
		t.Fatalf("expected 1 message, got %d", len(received))
	}
	if received[0].Direction != model.DirectionOutgoing {
		t.Errorf("Direction = %q, want %q", received[0].Direction, model.DirectionOutgoing)
	}
	if !json.Valid(received[0].NormalizedJSON) {
		t.Errorf("NormalizedJSON is not valid JSON: %q", string(received[0].NormalizedJSON))
	}
}

func TestTCPSource_MultipleMessagesInOneBuffer(t *testing.T) {
	// CNetLogServer dumps its entire recorded buffer as one blob with
	// multiple messages separated by binary garbage (no newlines).
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	json1 := sampleJSON("nodeManagementDetailedDiscoveryData")
	json2 := sampleJSON("deviceClassificationManufacturerData")
	json3 := sampleJSON("measurementListData")

	// Simulate: garbage + msg1 + garbage + msg2 + garbage + msg3 + garbage
	blob := "\x00\xff\xaa" +
		"[13:39:39.929] SEND to ship_Device_0x11c MSG: " + json1 +
		"\x00\x00\xff\xbb\xcc" +
		"[13:39:40.100] RECV from ship_Device_0x11c MSG: " + json2 +
		"\x00\xff" +
		"[13:39:47.943] SEND to ship_Device_0x11c MSG: " + json3 +
		"\x00\x00\x00\r\n"

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// Send entire blob at once — no newlines between messages.
		conn.Write([]byte(blob))
		buf := make([]byte, 1)
		conn.Read(buf)
	}()

	src := NewTCPSource(ln.Addr().String(), parser.New(), slog.Default())
	ctx, cancel := context.WithCancel(context.Background())

	var received []*model.Message
	var mu sync.Mutex

	done := make(chan error, 1)
	go func() {
		done <- src.Run(ctx, func(msg *model.Message) {
			mu.Lock()
			received = append(received, msg)
			mu.Unlock()
		})
	}()

	waitForMessages(t, &mu, &received, 3)
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()

	if len(received) != 3 {
		t.Fatalf("expected 3 messages from single buffer, got %d", len(received))
	}

	// Message 1: SEND discovery
	if received[0].Direction != model.DirectionOutgoing {
		t.Errorf("msg[0].Direction = %q, want outgoing", received[0].Direction)
	}
	// Message 2: RECV manufacturer
	if received[1].Direction != model.DirectionIncoming {
		t.Errorf("msg[1].Direction = %q, want incoming", received[1].Direction)
	}
	// Message 3: SEND measurement
	if received[2].Direction != model.DirectionOutgoing {
		t.Errorf("msg[2].Direction = %q, want outgoing", received[2].Direction)
	}

	// All should have valid JSON
	for i, msg := range received {
		if !json.Valid(msg.NormalizedJSON) {
			t.Errorf("msg[%d].NormalizedJSON is not valid JSON", i)
		}
	}
}

func TestTCPSource_MalformedLines(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// Send malformed data followed by a valid message.
		fmt.Fprintln(conn, "this is not a valid line")
		fmt.Fprintln(conn, "28 [11:38:26.280] SEN This is also malformed")
		fmt.Fprint(conn, `[11:38:26.008] SEND to ship_Device_0xaabb MSG: `+sampleJSON("nodeManagementDetailedDiscoveryData")+"\n")
		buf := make([]byte, 1)
		conn.Read(buf)
	}()

	src := NewTCPSource(ln.Addr().String(), parser.New(), slog.Default())
	ctx, cancel := context.WithCancel(context.Background())

	var received []*model.Message
	var mu sync.Mutex

	done := make(chan error, 1)
	go func() {
		done <- src.Run(ctx, func(msg *model.Message) {
			mu.Lock()
			received = append(received, msg)
			mu.Unlock()
		})
	}()

	waitForMessages(t, &mu, &received, 1)
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()

	if len(received) != 1 {
		t.Fatalf("expected 1 message (malformed skipped), got %d", len(received))
	}
}

func TestExtractMessages(t *testing.T) {
	src := NewTCPSource("localhost:0", parser.New(), slog.Default())
	baseDate := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)

	json1 := sampleJSON("nodeManagementDetailedDiscoveryData")
	json2 := sampleJSON("deviceClassificationManufacturerData")

	tests := []struct {
		name      string
		data      string
		wantCount int
		wantCarry bool
	}{
		{
			name:      "single clean message",
			data:      "[11:00:00.000] SEND to ship_Dev_0xaa MSG: " + json1,
			wantCount: 1,
		},
		{
			name:      "two messages with garbage between",
			data:      "[11:00:00.000] SEND to ship_Dev_0xaa MSG: " + json1 + "\x00\xff[11:00:01.000] RECV from ship_Dev_0xaa MSG: " + json2,
			wantCount: 2,
		},
		{
			name:      "leading garbage then message",
			data:      "\x00\xff\xaa\xbb[11:00:00.000] SEND to ship_Dev_0xaa MSG: " + json1,
			wantCount: 1,
		},
		{
			name:      "no messages at all",
			data:      "totally random garbage data",
			wantCount: 0,
		},
		{
			name:      "empty data",
			data:      "",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var received []*model.Message
			src.extractMessages(tt.data, baseDate, func(msg *model.Message) {
				received = append(received, msg)
			})
			if len(received) != tt.wantCount {
				t.Errorf("extracted %d messages, want %d", len(received), tt.wantCount)
			}
			for i, msg := range received {
				if !json.Valid(msg.NormalizedJSON) {
					t.Errorf("msg[%d].NormalizedJSON is not valid JSON", i)
				}
			}
		})
	}
}
