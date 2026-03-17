package parser

import (
	"testing"
	"time"
)

func TestLogLineRegex(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantNil bool
		wantSeq string
		wantDir string
	}{
		{
			name:    "SEND line",
			input:   `15 [11:38:26.008] SEND to ship_Volvo-CEM-400000270_0xaff223b8 MSG: {"datagram":[]}`,
			wantSeq: "15",
			wantDir: "SEND",
		},
		{
			name:    "RECV line",
			input:   `16 [11:38:26.016] RECV from ship_Volvo-CEM-400000270_0xaff223b8 MSG: {"datagram":[]}`,
			wantSeq: "16",
			wantDir: "RECV",
		},
		{
			name:    "malformed line",
			input:   "28 [11:38:26.280] SEN This is a malformed line",
			wantNil: true,
		},
		{
			name:    "empty line",
			input:   "",
			wantNil: true,
		},
		{
			name:    "CEasierLogger SEND without sequence number",
			input:   `[11:38:26.008] SEND to ship_Volvo-CEM-400000270_0xaff223b8 MSG: {"datagram":[]}`,
			wantSeq: "",
			wantDir: "SEND",
		},
		{
			name:    "CEasierLogger RECV without sequence number",
			input:   `[11:38:26.016] RECV from ship_Volvo-CEM-400000270_0xaff223b8 MSG: {"datagram":[]}`,
			wantSeq: "",
			wantDir: "RECV",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := LogLineRegex.FindStringSubmatch(tt.input)
			if tt.wantNil {
				if matches != nil {
					t.Error("expected nil match")
				}
				return
			}
			if matches == nil {
				t.Fatal("expected match, got nil")
			}
			if matches[1] != tt.wantSeq {
				t.Errorf("seq = %q, want %q", matches[1], tt.wantSeq)
			}
			if matches[3] != tt.wantDir {
				t.Errorf("dir = %q, want %q", matches[3], tt.wantDir)
			}
		})
	}
}

func TestParseLogTimestamp(t *testing.T) {
	baseDate := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		timeStr string
		wantH   int
		wantM   int
		wantS   int
		wantMs  int
		wantErr bool
	}{
		{
			name: "valid time", timeStr: "11:38:26.008",
			wantH: 11, wantM: 38, wantS: 26, wantMs: 8,
		},
		{
			name: "midnight", timeStr: "00:00:00.000",
			wantH: 0, wantM: 0, wantS: 0, wantMs: 0,
		},
		{
			name: "max time", timeStr: "23:59:59.999",
			wantH: 23, wantM: 59, wantS: 59, wantMs: 999,
		},
		{name: "no dot", timeStr: "11:38:26", wantErr: true},
		{name: "bad hms", timeStr: "11:38.000", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, err := ParseLogTimestamp(baseDate, tt.timeStr)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ts.Hour() != tt.wantH || ts.Minute() != tt.wantM || ts.Second() != tt.wantS {
				t.Errorf("time = %v, want %02d:%02d:%02d", ts, tt.wantH, tt.wantM, tt.wantS)
			}
			gotMs := ts.Nanosecond() / int(time.Millisecond)
			if gotMs != tt.wantMs {
				t.Errorf("ms = %d, want %d", gotMs, tt.wantMs)
			}
		})
	}
}

func TestExtractPeerDevice(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"ship_Volvo-CEM-400000270_0xaff223b8", "Volvo-CEM-400000270"},
		{"ship_MyDevice_0x12345678", "MyDevice"},
		{"plain-peer", "plain-peer"},
		{"ship_Device-With-Multiple_Parts_0xabcd", "Device-With-Multiple_Parts"},
	}
	for _, tt := range tests {
		got := ExtractPeerDevice(tt.input)
		if got != tt.want {
			t.Errorf("ExtractPeerDevice(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestEEBusTesterLogRegex(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantNil   bool
		wantDate  string
		wantTime  string
		wantDir   string
		wantPeer  string
		wantJSON  string
	}{
		{
			name:     "Send line",
			input:    `[20260206 11:54:15.338] - INFO - DATAGRAM - Tester_EG - Send message to 'ship_ghostONE-Wallbox-00001616_0x76ce84000df0': {"datagram":[]}`,
			wantDate: "20260206",
			wantTime: "11:54:15.338",
			wantDir:  "Send",
			wantPeer: "ship_ghostONE-Wallbox-00001616_0x76ce84000df0",
			wantJSON: `{"datagram":[]}`,
		},
		{
			name:     "Received line",
			input:    `[20260206 11:54:15.450] - INFO - DATAGRAM - Tester_EG - Received message from 'ship_ghostONE-Wallbox-00001616_0x76ce84000df0': {"datagram":[]}`,
			wantDate: "20260206",
			wantTime: "11:54:15.450",
			wantDir:  "Received",
			wantPeer: "ship_ghostONE-Wallbox-00001616_0x76ce84000df0",
			wantJSON: `{"datagram":[]}`,
		},
		{
			name:    "DEBUG line (not DATAGRAM)",
			input:   `[20260206 11:54:07.811] - DEBUG - USECASE - Tester_EG - Successfully added Feature /0/0`,
			wantNil: true,
		},
		{
			name:    "INFO non-DATAGRAM line",
			input:   `[20260206 11:54:06.816] - INFO - TESTER - + END KW: Init Loggers (37 ms)`,
			wantNil: true,
		},
		{
			name:    "empty line",
			input:   "",
			wantNil: true,
		},
		{
			name:     "different instance name",
			input:    `[20260302 09:00:15.100] - INFO - DATAGRAM - MyInstance_42 - Send message to 'ship_Device-ABC_0xdead': {"datagram":[{"test":1}]}`,
			wantDate: "20260302",
			wantTime: "09:00:15.100",
			wantDir:  "Send",
			wantPeer: "ship_Device-ABC_0xdead",
			wantJSON: `{"datagram":[{"test":1}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := EEBusTesterLogRegex.FindStringSubmatch(tt.input)
			if tt.wantNil {
				if matches != nil {
					t.Error("expected nil match")
				}
				return
			}
			if matches == nil {
				t.Fatal("expected match, got nil")
			}
			if matches[1] != tt.wantDate {
				t.Errorf("date = %q, want %q", matches[1], tt.wantDate)
			}
			if matches[2] != tt.wantTime {
				t.Errorf("time = %q, want %q", matches[2], tt.wantTime)
			}
			if matches[3] != tt.wantDir {
				t.Errorf("dir = %q, want %q", matches[3], tt.wantDir)
			}
			if matches[4] != tt.wantPeer {
				t.Errorf("peer = %q, want %q", matches[4], tt.wantPeer)
			}
			if matches[5] != tt.wantJSON {
				t.Errorf("json = %q, want %q", matches[5], tt.wantJSON)
			}
		})
	}
}

func TestParseEEBusTesterTimestamp(t *testing.T) {
	tests := []struct {
		name    string
		dateStr string
		timeStr string
		wantY   int
		wantMo  time.Month
		wantD   int
		wantH   int
		wantM   int
		wantS   int
		wantMs  int
		wantErr bool
	}{
		{
			name: "valid timestamp", dateStr: "20260206", timeStr: "11:54:15.338",
			wantY: 2026, wantMo: time.February, wantD: 6,
			wantH: 11, wantM: 54, wantS: 15, wantMs: 338,
		},
		{
			name: "midnight", dateStr: "20260101", timeStr: "00:00:00.000",
			wantY: 2026, wantMo: time.January, wantD: 1,
			wantH: 0, wantM: 0, wantS: 0, wantMs: 0,
		},
		{
			name: "end of day", dateStr: "20251231", timeStr: "23:59:59.999",
			wantY: 2025, wantMo: time.December, wantD: 31,
			wantH: 23, wantM: 59, wantS: 59, wantMs: 999,
		},
		{name: "bad date length", dateStr: "2026020", timeStr: "11:54:15.338", wantErr: true},
		{name: "bad time no dot", dateStr: "20260206", timeStr: "11:54:15", wantErr: true},
		{name: "bad time no colon", dateStr: "20260206", timeStr: "1154:15.338", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, err := ParseEEBusTesterTimestamp(tt.dateStr, tt.timeStr)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ts.Year() != tt.wantY || ts.Month() != tt.wantMo || ts.Day() != tt.wantD {
				t.Errorf("date = %v, want %d-%02d-%02d", ts.Format("2006-01-02"), tt.wantY, tt.wantMo, tt.wantD)
			}
			if ts.Hour() != tt.wantH || ts.Minute() != tt.wantM || ts.Second() != tt.wantS {
				t.Errorf("time = %v, want %02d:%02d:%02d", ts.Format("15:04:05"), tt.wantH, tt.wantM, tt.wantS)
			}
			gotMs := ts.Nanosecond() / int(time.Millisecond)
			if gotMs != tt.wantMs {
				t.Errorf("ms = %d, want %d", gotMs, tt.wantMs)
			}
		})
	}
}

func TestDetectLogFormat(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    LogFormat
	}{
		{
			name:    "eebus-go format",
			content: "15 [11:38:26.008] SEND to ship_Device_0xabc MSG: {}\n16 [11:38:26.016] RECV from ship_Device_0xabc MSG: {}\n",
			want:    LogFormatEEBusGo,
		},
		{
			name:    "eebustester format",
			content: "[20260206 11:54:06.816] - INFO - TESTER - some line\n[20260206 11:54:15.338] - INFO - DATAGRAM - Tester_EG - Send message to 'peer': {}\n",
			want:    LogFormatEEBusTester,
		},
		{
			name:    "empty content",
			content: "",
			want:    LogFormatUnknown,
		},
		{
			name:    "unrecognized format",
			content: "some random text\nno log format here\n",
			want:    LogFormatUnknown,
		},
		{
			name:    "eebustester with many non-datagram lines",
			content: "[20260206 11:54:06.816] - DEBUG - USECASE - line1\n[20260206 11:54:06.817] - INFO - TESTER - line2\n",
			want:    LogFormatEEBusTester,
		},
		{
			name:    "CEasierLogger format (no sequence numbers)",
			content: "[11:38:26.008] SEND to ship_Device_0xabc MSG: {}\n[11:38:26.016] RECV from ship_Device_0xabc MSG: {}\n",
			want:    LogFormatEEBusGo,
		},
		{
			name:    "EEBus Hub format",
			content: "2026-03-16 05:19:57    [Send] 1adbb6152b3902b028b2f4c1b3855777f19fb4f7{\"data\":{}}\n",
			want:    LogFormatEEBusHub,
		},
		{
			name:    "EEBus Hub does not false-positive on eebus-go",
			content: "15 [11:38:26.008] SEND to ship_Device_0xabc MSG: {}\n",
			want:    LogFormatEEBusGo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectLogFormat(tt.content)
			if got != tt.want {
				t.Errorf("DetectLogFormat() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestEEBusHubLogRegex(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantNil  bool
		wantDate string
		wantTime string
		wantDir  string
		wantSKI  string
		wantJSON string
	}{
		{
			name:     "Send line",
			input:    `2026-03-16 05:19:57    [Send] 1adbb6152b3902b028b2f4c1b3855777f19fb4f7{"data":[]}`,
			wantDate: "2026-03-16",
			wantTime: "05:19:57",
			wantDir:  "Send",
			wantSKI:  "1adbb6152b3902b028b2f4c1b3855777f19fb4f7",
			wantJSON: `{"data":[]}`,
		},
		{
			name:     "Recv line",
			input:    `2026-03-16 05:19:58    [Recv] AABBCCDDEE0011223344AABBCCDDEE0011223344{"data":{"header":{}}}`,
			wantDate: "2026-03-16",
			wantTime: "05:19:58",
			wantDir:  "Recv",
			wantSKI:  "AABBCCDDEE0011223344AABBCCDDEE0011223344",
			wantJSON: `{"data":{"header":{}}}`,
		},
		{
			name:    "wrong direction keyword",
			input:   `2026-03-16 05:19:57    [SEND] 1adbb6152b3902b028b2f4c1b3855777f19fb4f7{"data":[]}`,
			wantNil: true,
		},
		{
			name:    "SKI too short",
			input:   `2026-03-16 05:19:57    [Send] 1adbb6152b39{"data":[]}`,
			wantNil: true,
		},
		{
			name:    "empty line",
			input:   "",
			wantNil: true,
		},
		{
			name:    "eebus-go format should not match",
			input:   `15 [11:38:26.008] SEND to ship_Device_0xabc MSG: {"datagram":[]}`,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := EEBusHubLogRegex.FindStringSubmatch(tt.input)
			if tt.wantNil {
				if matches != nil {
					t.Error("expected nil match")
				}
				return
			}
			if matches == nil {
				t.Fatal("expected match, got nil")
			}
			if matches[1] != tt.wantDate {
				t.Errorf("date = %q, want %q", matches[1], tt.wantDate)
			}
			if matches[2] != tt.wantTime {
				t.Errorf("time = %q, want %q", matches[2], tt.wantTime)
			}
			if matches[3] != tt.wantDir {
				t.Errorf("dir = %q, want %q", matches[3], tt.wantDir)
			}
			if matches[4] != tt.wantSKI {
				t.Errorf("ski = %q, want %q", matches[4], tt.wantSKI)
			}
			if matches[5] != tt.wantJSON {
				t.Errorf("json = %q, want %q", matches[5], tt.wantJSON)
			}
		})
	}
}

func TestParseEEBusHubTimestamp(t *testing.T) {
	tests := []struct {
		name    string
		dateStr string
		timeStr string
		wantY   int
		wantMo  time.Month
		wantD   int
		wantH   int
		wantM   int
		wantS   int
		wantErr bool
	}{
		{
			name: "valid timestamp", dateStr: "2026-03-16", timeStr: "05:19:57",
			wantY: 2026, wantMo: time.March, wantD: 16,
			wantH: 5, wantM: 19, wantS: 57,
		},
		{
			name: "midnight", dateStr: "2026-01-01", timeStr: "00:00:00",
			wantY: 2026, wantMo: time.January, wantD: 1,
			wantH: 0, wantM: 0, wantS: 0,
		},
		{
			name: "end of day", dateStr: "2025-12-31", timeStr: "23:59:59",
			wantY: 2025, wantMo: time.December, wantD: 31,
			wantH: 23, wantM: 59, wantS: 59,
		},
		{name: "bad date format", dateStr: "20260316", timeStr: "05:19:57", wantErr: true},
		{name: "bad time format", dateStr: "2026-03-16", timeStr: "05:19", wantErr: true},
		{name: "empty strings", dateStr: "", timeStr: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, err := ParseEEBusHubTimestamp(tt.dateStr, tt.timeStr)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ts.Year() != tt.wantY || ts.Month() != tt.wantMo || ts.Day() != tt.wantD {
				t.Errorf("date = %v, want %d-%02d-%02d", ts.Format("2006-01-02"), tt.wantY, tt.wantMo, tt.wantD)
			}
			if ts.Hour() != tt.wantH || ts.Minute() != tt.wantM || ts.Second() != tt.wantS {
				t.Errorf("time = %v, want %02d:%02d:%02d", ts.Format("15:04:05"), tt.wantH, tt.wantM, tt.wantS)
			}
		})
	}
}
