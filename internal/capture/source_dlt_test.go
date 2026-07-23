package capture

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
	"github.com/eebustracer/eebustracer/internal/parser"
)

// buildStreamFrame builds a live DLT wire frame (no storage header) with one
// verbose string arg. Mirrors buildDLTFrame in the parser package.
func buildStreamFrame(t *testing.T, apid, ctid, s string) []byte {
	t.Helper()
	strBytes := append([]byte(s), 0)
	strLen := len(strBytes)
	payload := make([]byte, 0, 4+2+strLen)
	typeInfo := make([]byte, 4)
	binary.LittleEndian.PutUint32(typeInfo, 0x00000200) // TYPE_STRING
	payload = append(payload, typeInfo...)
	lenField := make([]byte, 2)
	binary.LittleEndian.PutUint16(lenField, uint16(strLen))
	payload = append(payload, lenField...)
	payload = append(payload, strBytes...)

	pad4 := func(x string) []byte {
		b := make([]byte, 4)
		copy(b, x)
		return b
	}

	ext := make([]byte, 10)
	ext[0] = 0x01 // MSIN: verbose
	ext[1] = 1
	copy(ext[2:6], pad4(apid))
	copy(ext[6:10], pad4(ctid))

	ecu := pad4("ECU1")
	wtms := []byte{0, 0, 0, 0}

	body := append([]byte{}, ecu...)
	body = append(body, wtms...)
	body = append(body, ext...)
	body = append(body, payload...)

	msgLen := uint16(4 + len(body))
	std := make([]byte, 4)
	std[0] = 0x01 | 0x04 | 0x10 // UEH | WEID | WTMS
	std[1] = 0
	binary.BigEndian.PutUint16(std[2:4], msgLen)

	frame := append([]byte{}, std...)
	frame = append(frame, body...)
	return frame
}

func TestParseDLTFilter(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		accepts [][2]string // pairs that must pass
		rejects [][2]string // pairs that must fail
		wantErr bool
	}{
		{
			name:    "empty accepts all",
			input:   "",
			accepts: [][2]string{{"CEM", "CEM"}, {"HEMS", "SVC"}, {"", ""}},
		},
		{
			name:    "single APID wildcard",
			input:   "CEM",
			accepts: [][2]string{{"CEM", "CEM"}, {"CEM", "SVC"}, {"CEM", ""}},
			rejects: [][2]string{{"HEMS", "HEMS"}, {"", "CEM"}},
		},
		{
			name:    "APID:CTID exact",
			input:   "CEM:CEM",
			accepts: [][2]string{{"CEM", "CEM"}},
			rejects: [][2]string{{"CEM", "SVC"}, {"HEMS", "CEM"}},
		},
		{
			name:    "multi rule union",
			input:   "CEM,HEMS:HEMS",
			accepts: [][2]string{{"CEM", "CEM"}, {"CEM", "SVC"}, {"HEMS", "HEMS"}},
			rejects: [][2]string{{"HEMS", "SVC"}},
		},
		{
			name:    "whitespace tolerated",
			input:   " CEM , HEMS : HEMS ",
			accepts: [][2]string{{"CEM", "SVC"}, {"HEMS", "HEMS"}},
			rejects: [][2]string{{"HEMS", "SVC"}},
		},
		{
			name:    "empty APID rejected",
			input:   ":CTID",
			wantErr: true,
		},
		{
			name:    "APID too long rejected",
			input:   "TOOLONG",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := ParseDLTFilter(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for _, p := range tt.accepts {
				if !f.Accept(p[0], p[1]) {
					t.Errorf("Accept(%q,%q) = false, want true", p[0], p[1])
				}
			}
			for _, p := range tt.rejects {
				if f.Accept(p[0], p[1]) {
					t.Errorf("Accept(%q,%q) = true, want false", p[0], p[1])
				}
			}
		})
	}
}

func TestDLTFilter_String(t *testing.T) {
	f, _ := ParseDLTFilter("CEM,HEMS:HEMS")
	if got := f.String(); got != "CEM,HEMS:HEMS" {
		t.Errorf("String = %q, want CEM,HEMS:HEMS", got)
	}
	empty, _ := ParseDLTFilter("")
	if got := empty.String(); got != "" {
		t.Errorf("empty String = %q, want empty", got)
	}
}

// dltTestServer serves a fixed byte sequence over TCP once, then closes.
// Returns the listener address and a cleanup func.
func dltTestServer(t *testing.T, payload []byte) (addr string, cleanup func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.Write(payload)
		// Hold the conn open until the client closes it (simulates a real
		// dlt-daemon with no more data to send).
		io.Copy(io.Discard, conn)
	}()
	return ln.Addr().String(), func() { ln.Close() }
}

func TestDLTStreamSource_ReadsFrames(t *testing.T) {
	eebusJSON := `{"datagram":[{"header":[{"specificationVersion":"1.3.0"},{"addressSource":[{"device":"d:_i:CEM"},{"entity":[0]},{"feature":0}]},{"addressDestination":[{"device":"d:_i:EV"},{"entity":[0]},{"feature":0}]},{"msgCounter":1},{"cmdClassifier":"read"}]},{"payload":[{"cmd":[[{"nodeManagementDetailedDiscoveryData":[]}]]}]}]}`
	f1 := buildStreamFrame(t, "CEM", "CEM", `[Session 1] Send: `+eebusJSON)
	f2 := buildStreamFrame(t, "CEM", "SVC", `#rtm powTot=9839`) // non-EEBus
	f3 := buildStreamFrame(t, "HEMS", "HEMS", `[ShipTransport] tryReconnect`)

	payload := append([]byte{}, f1...)
	payload = append(payload, f2...)
	payload = append(payload, f3...)

	addr, cleanup := dltTestServer(t, payload)
	defer cleanup()

	src := NewDLTStreamSource(addr, DLTFilter{}, parser.New(), slog.Default())

	var mu sync.Mutex
	var got []*model.Message
	emit := func(m *model.Message) {
		mu.Lock()
		got = append(got, m)
		mu.Unlock()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- src.Run(ctx, emit) }()

	// Wait for the frames to be processed, then cancel to unblock the read.
	time.Sleep(150 * time.Millisecond)
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 {
		t.Fatalf("emitted %d msgs, want 1 (only the EEBus frame)", len(got))
	}
	if got[0].CmdClassifier != "read" {
		t.Errorf("classifier = %q, want read", got[0].CmdClassifier)
	}
	if got[0].Direction != model.DirectionOutgoing {
		t.Errorf("direction = %q, want outgoing", got[0].Direction)
	}
}

func TestDLTStreamSource_FilterDropsNonMatching(t *testing.T) {
	eebusJSON := `{"datagram":[{"header":[{"specificationVersion":"1.3.0"},{"addressSource":[{"device":"d:_i:A"},{"entity":[0]},{"feature":0}]},{"addressDestination":[{"device":"d:_i:B"},{"entity":[0]},{"feature":0}]},{"msgCounter":1},{"cmdClassifier":"read"}]},{"payload":[{"cmd":[[{"nodeManagementDetailedDiscoveryData":[]}]]}]}]}`
	// Both frames carry EEBus content but different APID.
	f1 := buildStreamFrame(t, "CEM", "CEM", `[Session 1] Send: `+eebusJSON)
	f2 := buildStreamFrame(t, "OTHR", "OTHR", `[Session 2] Send: `+eebusJSON)

	addr, cleanup := dltTestServer(t, append(f1, f2...))
	defer cleanup()

	filter, _ := ParseDLTFilter("CEM")
	src := NewDLTStreamSource(addr, filter, parser.New(), slog.Default())

	var mu sync.Mutex
	var count int
	emit := func(m *model.Message) { mu.Lock(); count++; mu.Unlock() }

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- src.Run(ctx, emit) }()
	time.Sleep(150 * time.Millisecond)
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()
	if count != 1 {
		t.Errorf("emitted %d, want 1 (OTHR frame must be filtered out)", count)
	}
}

func TestDLTStreamSource_ReconnectsOnDialFailure(t *testing.T) {
	// Use the test dialer hook: fail twice, then succeed.
	var attempts int
	var mu sync.Mutex

	// Real listener that will serve one EEBus frame when finally reached.
	eebusJSON := `{"datagram":[{"header":[{"specificationVersion":"1.3.0"},{"addressSource":[{"device":"d:_i:X"},{"entity":[0]},{"feature":0}]},{"addressDestination":[{"device":"d:_i:Y"},{"entity":[0]},{"feature":0}]},{"msgCounter":9},{"cmdClassifier":"read"}]},{"payload":[{"cmd":[[{"nodeManagementDetailedDiscoveryData":[]}]]}]}]}`
	frame := buildStreamFrame(t, "CEM", "CEM", `[Session 1] Send: `+eebusJSON)
	addr, cleanup := dltTestServer(t, frame)
	defer cleanup()

	src := NewDLTStreamSource("ignored", DLTFilter{}, parser.New(), slog.Default())
	src.dialFunc = func(ctx context.Context, target string) (net.Conn, error) {
		mu.Lock()
		attempts++
		n := attempts
		mu.Unlock()
		if n < 3 {
			return nil, errors.New("simulated connect failure")
		}
		var d net.Dialer
		return d.DialContext(ctx, "tcp", addr)
	}

	var msgCount int
	emit := func(m *model.Message) { mu.Lock(); msgCount++; mu.Unlock() }

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- src.Run(ctx, emit) }()

	// Poll until we get the message (after 2 backoffs: 1s + 2s = 3s).
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		got := msgCount
		mu.Unlock()
		if got >= 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()
	if attempts < 3 {
		t.Errorf("attempts = %d, want ≥3", attempts)
	}
	if msgCount != 1 {
		t.Errorf("msgCount = %d, want 1", msgCount)
	}
}

// countingReporter records IncTruncated calls for assertion in tests.
type countingReporter struct {
	n int
	sync.Mutex
}

func (c *countingReporter) IncTruncated() {
	c.Lock()
	c.n++
	c.Unlock()
}

func (c *countingReporter) count() int {
	c.Lock()
	defer c.Unlock()
	return c.n
}

func TestDLTStreamSource_ReportsTruncatedFrames(t *testing.T) {
	// Truncated EEBus-shaped payload (Porsche pattern, JSON cut off mid-key).
	truncated := `[Session 1] Send: {"data":[{"header":[{"protocolId":"ee1.0"`
	f1 := buildStreamFrame(t, "CEM", "CEM", truncated)
	// A second truncated frame — reporter should see 2.
	f2 := buildStreamFrame(t, "CEM", "CEM", truncated)
	// A well-formed frame afterwards so we know processing continues.
	eebusJSON := `{"datagram":[{"header":[{"specificationVersion":"1.3.0"},{"addressSource":[{"device":"d:_i:CEM"},{"entity":[0]},{"feature":0}]},{"addressDestination":[{"device":"d:_i:EV"},{"entity":[0]},{"feature":0}]},{"msgCounter":1},{"cmdClassifier":"read"}]},{"payload":[{"cmd":[[{"nodeManagementDetailedDiscoveryData":[]}]]}]}]}`
	f3 := buildStreamFrame(t, "CEM", "CEM", `[Session 2] Send: `+eebusJSON)

	payload := append(append(f1, f2...), f3...)
	addr, cleanup := dltTestServer(t, payload)
	defer cleanup()

	reporter := &countingReporter{}
	src := NewDLTStreamSource(addr, DLTFilter{}, parser.New(), slog.Default())
	src.SetTruncatedReporter(reporter)

	var mu sync.Mutex
	var got []*model.Message
	emit := func(m *model.Message) { mu.Lock(); got = append(got, m); mu.Unlock() }

	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- src.Run(ctx, emit) }()
	time.Sleep(150 * time.Millisecond)
	cancel()
	<-done

	mu.Lock()
	msgCount := len(got)
	mu.Unlock()
	if msgCount != 1 {
		t.Errorf("emitted messages = %d, want 1 (only the well-formed frame)", msgCount)
	}
	if n := reporter.count(); n != 2 {
		t.Errorf("reporter.count = %d, want 2 (two truncated frames)", n)
	}
}

func TestDLTStreamSource_CancelStopsCleanly(t *testing.T) {
	// A listener that accepts and hangs forever with no data.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		conn, _ := ln.Accept()
		if conn != nil {
			io.Copy(io.Discard, conn)
		}
	}()

	src := NewDLTStreamSource(ln.Addr().String(), DLTFilter{}, parser.New(), slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- src.Run(ctx, func(*model.Message) {}) }()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned error on cancel: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Errorf("Run did not return within 2s of cancel")
	}
}
