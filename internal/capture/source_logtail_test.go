package capture

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
	"github.com/eebustracer/eebustracer/internal/parser"
)

func TestLogTailSource_Name(t *testing.T) {
	src := NewLogTailSource("/tmp/test.log", parser.New(), slog.Default())
	if src.Name() != "logtail" {
		t.Errorf("Name() = %q, want %q", src.Name(), "logtail")
	}
}

func TestLogTailSource_NewLines(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	// Create an empty log file.
	f, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("create file: %v", err)
	}

	src := NewLogTailSource(logPath, parser.New(), slog.Default())
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

	// Wait for source to start polling.
	time.Sleep(200 * time.Millisecond)

	// Append log lines.
	logLine1 := `15 [11:38:26.008] SEND to ship_Volvo-CEM-400000270_0xaff223b8 MSG: {"datagram":[{"header":[{"specificationVersion":"1.3.0"},{"addressSource":[{"device":"d:_i:_Volvo-00000122"},{"entity":[0]},{"feature":0}]},{"addressDestination":[{"device":"d:_i:37916_CEM-400000270"},{"entity":[0]},{"feature":0}]},{"msgCounter":21},{"cmdClassifier":"read"},{"ackRequest":true}]},{"payload":[{"cmd":[[{"nodeManagementDetailedDiscoveryData":[]}]]}]}]}` + "\n"
	logLine2 := `16 [11:38:26.016] RECV from ship_Volvo-CEM-400000270_0xaff223b8 MSG: {"datagram":[{"header":[{"specificationVersion":"1.3.0"},{"addressSource":[{"device":"d:_i:37916_CEM-400000270"},{"entity":[2]},{"feature":3}]},{"addressDestination":[{"device":"d:_i:_Volvo-00000122"},{"entity":[1]},{"feature":1}]},{"msgCounter":6},{"cmdClassifier":"read"}]},{"payload":[{"cmd":[[{"deviceClassificationManufacturerData":[]}]]}]}]}` + "\n"

	f.WriteString(logLine1)
	f.WriteString(logLine2)
	f.Sync()

	// Wait for processing.
	deadline := time.After(3 * time.Second)
	for {
		mu.Lock()
		count := len(received)
		mu.Unlock()
		if count >= 2 {
			break
		}
		select {
		case <-deadline:
			cancel()
			<-done
			mu.Lock()
			t.Fatalf("timeout waiting for messages, got %d", len(received))
			mu.Unlock()
			return
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}

	cancel()
	<-done
	f.Close()

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
	if received[0].FunctionSet != "NodeManagementDetailedDiscoveryData" {
		t.Errorf("msg[0].FunctionSet = %q, want %q", received[0].FunctionSet, "NodeManagementDetailedDiscoveryData")
	}
	if received[0].MsgCounter != "21" {
		t.Errorf("msg[0].MsgCounter = %q, want %q", received[0].MsgCounter, "21")
	}

	if received[1].Direction != model.DirectionIncoming {
		t.Errorf("msg[1].Direction = %q, want %q", received[1].Direction, model.DirectionIncoming)
	}
	if received[1].FunctionSet != "DeviceClassificationManufacturerData" {
		t.Errorf("msg[1].FunctionSet = %q, want %q", received[1].FunctionSet, "DeviceClassificationManufacturerData")
	}
}

func TestLogTailSource_MalformedLines(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	f, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("create file: %v", err)
	}

	src := NewLogTailSource(logPath, parser.New(), slog.Default())
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

	time.Sleep(200 * time.Millisecond)

	// Write malformed lines followed by a valid one.
	f.WriteString("this is not a valid line\n")
	f.WriteString("28 [11:38:26.280] SEN This is also malformed\n")
	f.WriteString(`15 [11:38:26.008] SEND to ship_Device_0xaabb MSG: {"datagram":[{"header":[{"specificationVersion":"1.3.0"},{"addressSource":[{"device":"d:_i:_Dev"},{"entity":[0]},{"feature":0}]},{"addressDestination":[{"device":"d:_i:_Other"},{"entity":[0]},{"feature":0}]},{"msgCounter":1},{"cmdClassifier":"read"}]},{"payload":[{"cmd":[[{"nodeManagementDetailedDiscoveryData":[]}]]}]}]}` + "\n")
	f.Sync()

	deadline := time.After(3 * time.Second)
	for {
		mu.Lock()
		count := len(received)
		mu.Unlock()
		if count >= 1 {
			break
		}
		select {
		case <-deadline:
			cancel()
			<-done
			t.Fatal("timeout waiting for valid message")
			return
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}

	cancel()
	<-done
	f.Close()

	mu.Lock()
	defer mu.Unlock()

	// Only the valid line should have been emitted.
	if len(received) != 1 {
		t.Fatalf("expected 1 message (malformed skipped), got %d", len(received))
	}
}

func TestLogTailSource_Shutdown(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	f, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	f.Close()

	src := NewLogTailSource(logPath, parser.New(), slog.Default())
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- src.Run(ctx, func(msg *model.Message) {})
	}()

	// Give it a moment to start
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

func TestLogTailSource_FileNotFound(t *testing.T) {
	src := NewLogTailSource("/nonexistent/path.log", parser.New(), slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := src.Run(ctx, func(msg *model.Message) {})
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}
