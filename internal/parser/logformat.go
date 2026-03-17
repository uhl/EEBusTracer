package parser

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// LogLineRegex matches eebus-go and CEasierLogger log lines. The sequence
// number prefix is optional — when absent, matches[1] is empty.
//
//	15 [11:38:26.008] SEND to ship_Volvo-CEM-400000270_0xaff223b8 MSG: {"datagram":[...]}
//	[11:38:26.008] SEND to ship_Device_0xabc MSG: {"datagram":[...]}
var LogLineRegex = regexp.MustCompile(
	`^(?:(\d+)\s+)?\[(\d{2}:\d{2}:\d{2}\.\d{3})\]\s+(SEND|RECV)\s+(?:to|from)\s+(\S+)\s+MSG:\s+(.+)$`,
)

// ParseLogTimestamp parses a log time string (HH:MM:SS.mmm) relative to a base date.
func ParseLogTimestamp(baseDate time.Time, timeStr string) (time.Time, error) {
	parts := strings.SplitN(timeStr, ".", 2)
	if len(parts) != 2 {
		return time.Time{}, fmt.Errorf("invalid time format: %s", timeStr)
	}
	hms := strings.Split(parts[0], ":")
	if len(hms) != 3 {
		return time.Time{}, fmt.Errorf("invalid time format: %s", timeStr)
	}
	h, _ := strconv.Atoi(hms[0])
	m, _ := strconv.Atoi(hms[1])
	s, _ := strconv.Atoi(hms[2])
	ms, _ := strconv.Atoi(parts[1])

	return baseDate.Add(
		time.Duration(h)*time.Hour +
			time.Duration(m)*time.Minute +
			time.Duration(s)*time.Second +
			time.Duration(ms)*time.Millisecond,
	), nil
}

// EEBusTesterLogRegex matches eebustester DATAGRAM log lines like:
//
//	[20260206 11:54:15.338] - INFO - DATAGRAM - Tester_EG - Send message to 'ship_ghostONE-Wallbox-00001616_0x76ce84000df0': {"datagram":[...]}
//	[20260206 11:54:15.450] - INFO - DATAGRAM - Tester_EG - Received message from 'ship_ghostONE-Wallbox-00001616_0x76ce84000df0': {"datagram":[...]}
var EEBusTesterLogRegex = regexp.MustCompile(
	`^\[(\d{8})\s+(\d{2}:\d{2}:\d{2}\.\d{3})\]\s+-\s+INFO\s+-\s+DATAGRAM\s+-\s+\S+\s+-\s+(Send|Received)\s+message\s+(?:to|from)\s+'([^']+)':\s+(.+)$`,
)

// eebustesterPrefixRegex detects lines starting with the eebustester timestamp format.
var eebustesterPrefixRegex = regexp.MustCompile(`^\[\d{8}\s+\d{2}:\d{2}:\d{2}\.\d{3}\]`)

// eebusgoPrefixRegex detects lines starting with the eebus-go or CEasierLogger
// format. Matches both "15 [11:38:26..." and "[11:38:26..." but not the
// eebustester format "[20260206 11:54:..." (checked first by detection order).
var eebusgoPrefixRegex = regexp.MustCompile(`^(?:\d+\s+)?\[\d{2}:\d{2}:\d{2}`)

// LogFormat represents the detected log file format.
type LogFormat int

const (
	LogFormatUnknown     LogFormat = iota
	LogFormatEEBusGo               // eebus-go / spine-go log format
	LogFormatEEBusTester            // eebustester log format
	LogFormatEEBusHub              // EEBus Hub log format
)

// EEBusHubLogRegex matches EEBus Hub log lines:
//
//	2026-03-16 05:19:57    [Send] 1adbb6152b3902b028b2f4c1b3855777f19fb4f7{"data":[...]}
var EEBusHubLogRegex = regexp.MustCompile(
	`^(\d{4}-\d{2}-\d{2})\s+(\d{2}:\d{2}:\d{2})\s+\[(Send|Recv)\]\s+([0-9a-fA-F]{40})(.+)$`,
)

// eebushubPrefixRegex detects lines starting with the EEBus Hub timestamp format.
var eebushubPrefixRegex = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}\s+\[(Send|Recv)\]`)

// DetectLogFormat examines the first lines of content to determine the log format.
func DetectLogFormat(content string) LogFormat {
	// Check up to 4KB or 50 lines, whichever comes first
	peek := content
	if len(peek) > 4096 {
		peek = peek[:4096]
	}

	lines := strings.SplitN(peek, "\n", 51)
	if len(lines) > 50 {
		lines = lines[:50]
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if eebushubPrefixRegex.MatchString(line) {
			return LogFormatEEBusHub
		}
		if eebustesterPrefixRegex.MatchString(line) {
			return LogFormatEEBusTester
		}
		if eebusgoPrefixRegex.MatchString(line) {
			return LogFormatEEBusGo
		}
	}
	return LogFormatUnknown
}

// ParseEEBusTesterTimestamp parses an eebustester date+time pair (YYYYMMDD + HH:MM:SS.mmm)
// into a time.Time in UTC.
func ParseEEBusTesterTimestamp(dateStr, timeStr string) (time.Time, error) {
	if len(dateStr) != 8 {
		return time.Time{}, fmt.Errorf("invalid date format: %s", dateStr)
	}

	year, err := strconv.Atoi(dateStr[0:4])
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date format: %s", dateStr)
	}
	month, err := strconv.Atoi(dateStr[4:6])
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date format: %s", dateStr)
	}
	day, err := strconv.Atoi(dateStr[6:8])
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date format: %s", dateStr)
	}

	parts := strings.SplitN(timeStr, ".", 2)
	if len(parts) != 2 {
		return time.Time{}, fmt.Errorf("invalid time format: %s", timeStr)
	}
	hms := strings.Split(parts[0], ":")
	if len(hms) != 3 {
		return time.Time{}, fmt.Errorf("invalid time format: %s", timeStr)
	}
	h, _ := strconv.Atoi(hms[0])
	m, _ := strconv.Atoi(hms[1])
	s, _ := strconv.Atoi(hms[2])
	ms, _ := strconv.Atoi(parts[1])

	return time.Date(year, time.Month(month), day, h, m, s, ms*int(time.Millisecond), time.UTC), nil
}

// ParseEEBusHubTimestamp parses an EEBus Hub date+time pair (YYYY-MM-DD + HH:MM:SS)
// into a time.Time in UTC.
func ParseEEBusHubTimestamp(dateStr, timeStr string) (time.Time, error) {
	return time.Parse("2006-01-02 15:04:05", dateStr+" "+timeStr)
}

// ExtractPeerDevice extracts a device name from a ship peer identifier
// like "ship_Volvo-CEM-400000270_0xaff223b8" -> "Volvo-CEM-400000270".
func ExtractPeerDevice(peer string) string {
	peer = strings.TrimPrefix(peer, "ship_")
	// Remove the trailing _0x... hex suffix
	if idx := strings.LastIndex(peer, "_0x"); idx > 0 {
		return peer[:idx]
	}
	return peer
}
