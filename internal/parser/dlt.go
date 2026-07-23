package parser

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
)

// DLTTextLineRegex matches a single line from a DLT Viewer plain-text export.
// The column layout is:
//
//	<index> <YYYY/MM/DD> <HH:MM:SS.ffffff> <mono> <ctr> <ECU> <APID> <CTID> <sess> <type> <lvl> <mode> <argCount> : <payload>
//
// Example:
//
//	7390 2026/07/22 11:52:30.315139 162336.0780 252 ECU1 CEM CEM 668 log info verbose 1 : [Session 38099] Send: {"data":[...]}
//
// The trailing message is captured verbatim in group 7 (may contain colons, JSON, etc).
var DLTTextLineRegex = regexp.MustCompile(
	`^\s*(\d+)\s+` + // 1: index
		`(\d{4}/\d{2}/\d{2})\s+` + // 2: date
		`(\d{2}:\d{2}:\d{2}\.\d+)\s+` + // 3: time
		`\S+\s+` + // monotonic
		`\S+\s+` + // ctr
		`\S+\s+` + // ECU
		`(\S+)\s+` + // 4: APID
		`(\S+)\s+` + // 5: CTID
		`\S+\s+` + // sess
		`(\S+)\s+` + // 6: type (log/trace/...)
		`\S+\s+\S+\s+\S+\s+` + // level, mode, argCount
		`:\s*(.*)$`, // 7: payload
)

// dltTextPrefixRegex is a lightweight probe used by DetectLogFormat. It only
// checks the opening columns (index + date + time), not the full structure.
var dltTextPrefixRegex = regexp.MustCompile(`^\s*\d+\s+\d{4}/\d{2}/\d{2}\s+\d{2}:\d{2}:\d{2}\.\d+`)

// dltTruncatedMarker appears at the end of a DLT payload string when the
// original message exceeded the DLT column width. We skip such lines because
// the JSON is unrecoverable.
const dltTruncatedMarker = "<<Message truncated, too long>>"

// eebusSHIPFramePrefix identifies the start of a full SHIP-framed EEBus JSON
// (protocolId ee1.0). Used by the generic extractor to locate JSON embedded in
// arbitrary DLT payload text.
var eebusSHIPFramePrefix = []byte(`{"data":[{"header":[{"protocolId":"ee1.0"`)

// eebusDatagramPrefix identifies a bare SPINE datagram (rare in raw DLT).
var eebusDatagramPrefix = []byte(`{"datagram":`)

// porscheCEMSendPattern extracts EEBus JSON from Porsche CEM outbound log lines.
// Example payload: "[Session 38099] Send: {"data":[...]}"
var porscheCEMSendPattern = regexp.MustCompile(`^\[Session\s+\d+\]\s+Send:\s+(.+)$`)

// porscheCEMRecvPattern extracts EEBus JSON from Porsche CEM inbound log lines.
// Example payload: "[ConnectionWorker 38099] Received 386 Data bytes during ConnectionDataExchange: {"data":[...]}"
var porscheCEMRecvPattern = regexp.MustCompile(
	`^\[ConnectionWorker\s+\d+\]\s+Received\s+\d+\s+Data bytes during ConnectionDataExchange:\s+(.+)$`,
)

// DLTExtractResult holds an EEBus payload extracted from a DLT text line.
// When Truncated is true, JSON is empty and Direction is unset — the caller
// should count the drop and continue.
type DLTExtractResult struct {
	JSON      string
	Direction model.Direction
	Truncated bool
}

// ExtractEEBusFromDLTPayload attempts to pull an EEBus JSON blob out of a DLT
// payload string. It tries known ECU-specific patterns first (currently
// Porsche CEM), then falls back to a generic search for SHIP/SPINE JSON
// prefixes anywhere in the payload.
//
// Returns nil if the payload contains no EEBus content at all. If the line is
// marked as truncated by DLT ("<<Message truncated, too long>>") AND appears
// to contain EEBus content, a Truncated=true result is returned so the caller
// can surface "N messages dropped" to the user.
func ExtractEEBusFromDLTPayload(apid, ctid, payload string) *DLTExtractResult {
	if strings.Contains(payload, dltTruncatedMarker) {
		// Only treat as an EEBus truncation if the payload looks EEBus-ish.
		// Otherwise ordinary truncated telemetry would inflate the counter.
		if looksLikeEEBusPayload(apid, ctid, payload) {
			return &DLTExtractResult{Truncated: true}
		}
		return nil
	}

	// Porsche CEM: APID and CTID are both "CEM" for the SHIP layer.
	if apid == "CEM" && ctid == "CEM" {
		if m := porscheCEMSendPattern.FindStringSubmatch(payload); m != nil {
			return &DLTExtractResult{JSON: m[1], Direction: model.DirectionOutgoing}
		}
		if m := porscheCEMRecvPattern.FindStringSubmatch(payload); m != nil {
			return &DLTExtractResult{JSON: m[1], Direction: model.DirectionIncoming}
		}
		// Fall through to generic scan for other CEM sub-formats.
	}

	// Generic fallback: search for an EEBus JSON prefix anywhere in the payload.
	// Direction is unknown in this path — the UI infers it visually from
	// deviceSource / deviceDest.
	if idx := strings.Index(payload, string(eebusSHIPFramePrefix)); idx >= 0 {
		return &DLTExtractResult{JSON: payload[idx:], Direction: model.DirectionUnknown}
	}
	if idx := strings.Index(payload, string(eebusDatagramPrefix)); idx >= 0 {
		return &DLTExtractResult{JSON: payload[idx:], Direction: model.DirectionUnknown}
	}

	return nil
}

// looksLikeEEBusPayload heuristically decides whether a DLT payload string
// was carrying EEBus content before it got truncated. Used to keep the
// "N truncated" counter honest — we don't want unrelated overrun telemetry
// (e.g. long modbus dumps) to be counted as lost EEBus messages.
func looksLikeEEBusPayload(apid, ctid, payload string) bool {
	if apid == "CEM" && ctid == "CEM" &&
		(strings.Contains(payload, "[Session ") || strings.Contains(payload, "[ConnectionWorker ")) {
		return true
	}
	return strings.Contains(payload, `"protocolId":"ee1.0"`) ||
		strings.Contains(payload, `{"datagram":`)
}

// IsCompleteJSON returns true if b appears to be a complete JSON value. It
// only checks brace/bracket balance and string delimiters — it does not
// fully validate — because DLT-truncated payloads always end mid-value with
// unmatched braces or an unterminated string. Skipping such payloads avoids
// storing garbage messages that can't be parsed downstream.
func IsCompleteJSON(b []byte) bool {
	depth := 0
	inString := false
	escape := false
	for _, c := range b {
		if escape {
			escape = false
			continue
		}
		if inString {
			switch c {
			case '\\':
				escape = true
			case '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth < 0 {
				return false
			}
		}
	}
	return depth == 0 && !inString
}

// ParseDLTTextTimestamp parses a DLT export date+time pair into UTC.
// Date is "YYYY/MM/DD", time is "HH:MM:SS.ffffff".
func ParseDLTTextTimestamp(dateStr, timeStr string) (time.Time, error) {
	if len(dateStr) != 10 {
		return time.Time{}, fmt.Errorf("invalid date: %s", dateStr)
	}
	year, e1 := strconv.Atoi(dateStr[0:4])
	month, e2 := strconv.Atoi(dateStr[5:7])
	day, e3 := strconv.Atoi(dateStr[8:10])
	if e1 != nil || e2 != nil || e3 != nil {
		return time.Time{}, fmt.Errorf("invalid date: %s", dateStr)
	}
	parts := strings.SplitN(timeStr, ".", 2)
	if len(parts) != 2 {
		return time.Time{}, fmt.Errorf("invalid time: %s", timeStr)
	}
	hms := strings.Split(parts[0], ":")
	if len(hms) != 3 {
		return time.Time{}, fmt.Errorf("invalid time: %s", timeStr)
	}
	h, _ := strconv.Atoi(hms[0])
	m, _ := strconv.Atoi(hms[1])
	s, _ := strconv.Atoi(hms[2])
	// Fractional part: normalise to nanoseconds regardless of digit count.
	frac := parts[1]
	if len(frac) > 9 {
		frac = frac[:9]
	}
	for len(frac) < 9 {
		frac += "0"
	}
	ns, _ := strconv.Atoi(frac)
	return time.Date(year, time.Month(month), day, h, m, s, ns, time.UTC), nil
}
