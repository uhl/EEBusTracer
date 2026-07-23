package parser

import (
	"testing"

	"github.com/eebustracer/eebustracer/internal/model"
)

func TestDLTTextLineRegex_PorscheCEM(t *testing.T) {
	line := `7390 2026/07/22 11:52:30.315139 162336.0780 252 ECU1 CEM CEM 668 log info verbose 1 : [Session 38099] Send: {"data":[{"header":[{"protocolId":"ee1.0"}]}]}`
	m := DLTTextLineRegex.FindStringSubmatch(line)
	if m == nil {
		t.Fatalf("regex did not match line")
	}
	if m[1] != "7390" {
		t.Errorf("index = %q, want 7390", m[1])
	}
	if m[2] != "2026/07/22" {
		t.Errorf("date = %q", m[2])
	}
	if m[3] != "11:52:30.315139" {
		t.Errorf("time = %q", m[3])
	}
	if m[4] != "CEM" {
		t.Errorf("apid = %q", m[4])
	}
	if m[5] != "CEM" {
		t.Errorf("ctid = %q", m[5])
	}
	if got := m[7]; got[:11] != "[Session 38" {
		t.Errorf("payload = %q...", got[:30])
	}
}

func TestExtractEEBusFromDLTPayload_PorscheSend(t *testing.T) {
	payload := `[Session 38099] Send: {"data":[{"header":[{"protocolId":"ee1.0"}]}]}`
	got := ExtractEEBusFromDLTPayload("CEM", "CEM", payload)
	if got == nil {
		t.Fatalf("expected extraction, got nil")
	}
	if got.Direction != model.DirectionOutgoing {
		t.Errorf("Direction = %q, want outgoing", got.Direction)
	}
	if got.JSON[:8] != `{"data":` {
		t.Errorf("JSON = %q", got.JSON[:20])
	}
}

func TestExtractEEBusFromDLTPayload_PorscheRecv(t *testing.T) {
	payload := `[ConnectionWorker 38099] Received 386 Data bytes during ConnectionDataExchange: {"data":[{"header":[{"protocolId":"ee1.0"}]}]}`
	got := ExtractEEBusFromDLTPayload("CEM", "CEM", payload)
	if got == nil {
		t.Fatalf("expected extraction, got nil")
	}
	if got.Direction != model.DirectionIncoming {
		t.Errorf("Direction = %q, want incoming", got.Direction)
	}
	if got.JSON[:8] != `{"data":` {
		t.Errorf("JSON = %q", got.JSON[:20])
	}
}

func TestExtractEEBusFromDLTPayload_Truncated(t *testing.T) {
	// A CEM Send line cut off by DLT — must return a Truncated marker so the
	// importer can surface "N truncated" to the user, not silently vanish.
	payload := `[Session 38099] Send: {"data":[{"header":... <<Message truncated, too long>>`
	got := ExtractEEBusFromDLTPayload("CEM", "CEM", payload)
	if got == nil {
		t.Fatal("expected Truncated marker, got nil")
	}
	if !got.Truncated {
		t.Errorf("Truncated = false, want true")
	}
	if got.JSON != "" {
		t.Errorf("JSON should be empty when truncated, got %q", got.JSON)
	}
}

func TestExtractEEBusFromDLTPayload_TruncatedNonEEBusReturnsNil(t *testing.T) {
	// Truncated telemetry that isn't EEBus-shaped must NOT inflate the counter.
	payload := `#rtm powTot=9839 currL1=14.1 currL2=... <<Message truncated, too long>>`
	got := ExtractEEBusFromDLTPayload("CEM", "SVC", payload)
	if got != nil {
		t.Errorf("truncated non-EEBus payload should return nil, got %+v", got)
	}
}

func TestExtractEEBusFromDLTPayload_NonEEBusSkipped(t *testing.T) {
	payload := `Session 38099 ended`
	if got := ExtractEEBusFromDLTPayload("CEM", "CEM", payload); got != nil {
		t.Errorf("non-EEBus line should return nil, got %+v", got)
	}
	// HEMS ShipTransport lines have no JSON payload and must not extract.
	if got := ExtractEEBusFromDLTPayload("HEMS", "HEMS", `[ShipTransport] tryReconnect`); got != nil {
		t.Errorf("HEMS ShipTransport line should return nil, got %+v", got)
	}
}

func TestExtractEEBusFromDLTPayload_GenericFallback(t *testing.T) {
	// Unknown APID/CTID but payload contains a SHIP-framed EEBus JSON.
	payload := `some prefix text {"data":[{"header":[{"protocolId":"ee1.0"}]}]} trailing`
	got := ExtractEEBusFromDLTPayload("FOO", "BAR", payload)
	if got == nil {
		t.Fatalf("expected generic extraction")
	}
	if got.JSON[:8] != `{"data":` {
		t.Errorf("JSON = %q", got.JSON[:20])
	}
	// Generic fallback can't know direction — see the dedicated test above.
}

func TestParseDLTTextTimestamp(t *testing.T) {
	ts, err := ParseDLTTextTimestamp("2026/07/22", "11:52:30.315139")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if ts.Year() != 2026 || ts.Month() != 7 || ts.Day() != 22 {
		t.Errorf("date = %v", ts)
	}
	if ts.Hour() != 11 || ts.Minute() != 52 || ts.Second() != 30 {
		t.Errorf("time = %v", ts)
	}
	// 315139 microseconds → 315139000 nanoseconds
	if ts.Nanosecond() != 315139000 {
		t.Errorf("nanos = %d, want 315139000", ts.Nanosecond())
	}
}

func TestExtractEEBusFromDLTPayload_GenericFallbackUnknownDirection(t *testing.T) {
	// The generic path can't determine direction; it must return Unknown so
	// the UI can infer it from source/dest device addresses rather than
	// showing everything as incoming.
	payload := `some prefix {"data":[{"header":[{"protocolId":"ee1.0"}]}]}`
	got := ExtractEEBusFromDLTPayload("FOO", "BAR", payload)
	if got == nil {
		t.Fatalf("expected extraction")
	}
	if got.Direction != model.DirectionUnknown {
		t.Errorf("Direction = %q, want unknown", got.Direction)
	}
}

func TestIsCompleteJSON(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"complete object", `{"a":1}`, true},
		{"complete nested", `{"a":{"b":[1,2,3]}}`, true},
		{"truncated object", `{"a":1`, false},
		{"truncated string", `{"a":"unter`, false},
		{"trailing garbage", `{"a":1}}`, false},
		{"escaped quote", `{"a":"b\"c"}`, true},
		{"bracket balance", `[1,2,3]`, true},
		{"unbalanced brackets", `[1,2,3`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsCompleteJSON([]byte(tt.in)); got != tt.want {
				t.Errorf("IsCompleteJSON(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestDetectLogFormat_DLTText(t *testing.T) {
	sample := `7390 2026/07/22 11:52:30.315139 162336.0780 252 ECU1 CEM CEM 668 log info verbose 1 : [Session 38099] Send: {"data":[]}`
	if got := DetectLogFormat(sample); got != LogFormatDLTText {
		t.Errorf("DetectLogFormat = %v, want LogFormatDLTText", got)
	}
}
