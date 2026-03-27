package capture

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
	"github.com/eebustracer/eebustracer/internal/parser"
)

// timestampFinder locates the [HH:MM:SS.mmm] timestamp pattern within a line.
// Used to find the start of the actual log content inside padded C buffers.
var timestampFinder = regexp.MustCompile(`\[\d{2}:\d{2}:\d{2}\.\d{3}\]`)

// msgHeaderRegex matches the fixed-format header that precedes each JSON
// payload: [HH:MM:SS.mmm] SEND|RECV to|from <peer> MSG:
// It is used un-anchored so it can match anywhere within a binary buffer.
var msgHeaderRegex = regexp.MustCompile(
	`\[(\d{2}:\d{2}:\d{2}\.\d{3})\]\s+(SEND|RECV)\s+(?:to|from)\s+(\S+)\s+MSG:\s+`,
)

// extractLeadingJSON decodes the first complete JSON value from s and returns
// it, discarding any trailing bytes. This handles C-based TCP servers that
// send fixed-size buffers with arbitrary binary padding after the JSON payload.
// Returns s unchanged if no valid JSON value can be decoded.
func extractLeadingJSON(s string) string {
	dec := json.NewDecoder(strings.NewReader(s))
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return s
	}
	return string(raw)
}

// TCPSource connects to an EEBus device's TCP log server (e.g. CEasierLogger
// CNetLogServer) and reads newline-delimited trace lines.
type TCPSource struct {
	target string
	parser *parser.Parser
	logger *slog.Logger
}

// NewTCPSource creates a new TCP source that connects to the given target
// address (host:port).
func NewTCPSource(target string, p *parser.Parser, logger *slog.Logger) *TCPSource {
	return &TCPSource{
		target: target,
		parser: p,
		logger: logger,
	}
}

// Name returns "tcp".
func (s *TCPSource) Name() string { return "tcp" }

// dialWithRetry attempts to connect to the target with retries for transient
// network errors (e.g. "no route to host" from stale ARP cache on macOS).
func (s *TCPSource) dialWithRetry(ctx context.Context) (net.Conn, error) {
	const maxRetries = 3
	const dialTimeout = 10 * time.Second
	backoff := time.Second

	var lastErr error
	for attempt := range maxRetries {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		conn, err := net.DialTimeout("tcp", s.target, dialTimeout)
		if err == nil {
			if attempt > 0 {
				s.logger.Info("tcp: connected after retry", "attempt", attempt+1)
			}
			return conn, nil
		}
		lastErr = err
		s.logger.Warn("tcp: dial failed, retrying",
			"attempt", attempt+1,
			"error", err,
			"backoff", backoff,
		)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
	}
	return nil, lastErr
}

// Run connects to the TCP log server and reads data until the context is
// cancelled or the connection is closed. Instead of line-based scanning,
// it reads raw chunks and splits on the [HH:MM:SS.mmm] timestamp pattern
// to handle CNetLogServer's fixed-size binary buffers that may contain
// multiple messages and arbitrary binary padding.
func (s *TCPSource) Run(ctx context.Context, emit func(*model.Message)) error {
	conn, err := s.dialWithRetry(ctx)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", s.target, err)
	}

	// Close connection when context is cancelled to unblock reads.
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	baseDate := time.Now().Truncate(24 * time.Hour)

	// Read raw bytes in chunks. We accumulate data and extract complete
	// messages by finding [HH:MM:SS.mmm] ... MSG: {json} patterns.
	buf := make([]byte, 256*1024) // 256KB read buffer
	var carry string              // leftover data from previous read

	for {
		n, readErr := conn.Read(buf)
		if n > 0 {
			data := carry + string(buf[:n])

			s.logger.Debug("tcp: chunk received",
				"bytes", n,
				"totalBuf", len(data),
			)

			carry = s.extractMessages(data, baseDate, emit)
		}

		if readErr != nil {
			if ctx.Err() != nil {
				return nil // normal shutdown
			}
			if readErr == io.EOF {
				// Process any remaining data.
				if carry != "" {
					s.extractMessages(carry, baseDate, emit)
				}
				return nil
			}
			return fmt.Errorf("tcp read from %s: %w", s.target, readErr)
		}
	}
}

// extractMessages finds all complete messages in data by locating
// [HH:MM:SS.mmm] ... MSG: {json} patterns. Returns any trailing data
// that might be an incomplete message (to be prepended to the next chunk).
func (s *TCPSource) extractMessages(data string, baseDate time.Time, emit func(*model.Message)) string {
	// Find all message header positions.
	allLocs := msgHeaderRegex.FindAllStringSubmatchIndex(data, -1)
	if len(allLocs) == 0 {
		// No headers found. If there's a partial timestamp near the end,
		// keep it as carry for the next chunk.
		if loc := timestampFinder.FindStringIndex(data); loc != nil {
			return data[loc[0]:]
		}
		return ""
	}

	for i, loc := range allLocs {
		// loc indices: [full_start, full_end, time_start, time_end,
		//               dir_start, dir_end, peer_start, peer_end]
		headerEnd := loc[1] // end of "MSG: " — JSON starts here
		timeStr := data[loc[2]:loc[3]]
		direction := data[loc[4]:loc[5]]
		peer := data[loc[6]:loc[7]]

		// Determine where the JSON region ends: either at the next
		// header's start, or at the end of data.
		var jsonRegionEnd int
		if i+1 < len(allLocs) {
			jsonRegionEnd = allLocs[i+1][0]
		} else {
			jsonRegionEnd = len(data)
		}

		jsonRegion := data[headerEnd:jsonRegionEnd]

		// Extract the first valid JSON value from the region,
		// discarding any trailing binary garbage.
		jsonPayload := extractLeadingJSON(jsonRegion)
		if jsonPayload == jsonRegion && !json.Valid([]byte(jsonPayload)) {
			// Could not extract valid JSON — might be an incomplete
			// message at the end of the buffer. If this is the last
			// match, return it as carry.
			if i == len(allLocs)-1 {
				return data[loc[0]:]
			}
			s.logger.Debug("tcp: could not extract JSON from message, skipping",
				"time", timeStr,
				"dir", direction,
				"jsonLen", len(jsonRegion),
			)
			continue
		}

		s.emitMessage(timeStr, direction, peer, jsonPayload, baseDate, emit)
	}

	// If the last message was successfully processed, check if there's
	// a partial timestamp after it that should be carried over.
	lastEnd := allLocs[len(allLocs)-1][1]
	remaining := data[lastEnd:]
	if loc := timestampFinder.FindStringIndex(remaining); loc != nil {
		return remaining[loc[0]:]
	}

	return ""
}

// emitMessage parses fields from a single log entry and emits it.
func (s *TCPSource) emitMessage(timeStr, direction, peer, jsonPayload string, baseDate time.Time, emit func(*model.Message)) {
	ts, err := parser.ParseLogTimestamp(baseDate, timeStr)
	if err != nil {
		return
	}

	var dir model.Direction
	if direction == "SEND" {
		dir = model.DirectionOutgoing
	} else {
		dir = model.DirectionIncoming
	}

	normalized := parser.NormalizeEEBUSJSON([]byte(jsonPayload))

	msg := &model.Message{
		Timestamp:      ts,
		Direction:      dir,
		NormalizedJSON: json.RawMessage(normalized),
		ShipMsgType:    model.ShipMsgTypeData,
	}

	peerDevice := parser.ExtractPeerDevice(peer)

	spineMsg := s.parser.ParseSpineFromJSON(normalized)
	if spineMsg != nil {
		msg.SpinePayload = spineMsg.SpinePayload
		msg.CmdClassifier = spineMsg.CmdClassifier
		msg.FunctionSet = spineMsg.FunctionSet
		msg.MsgCounter = spineMsg.MsgCounter
		msg.MsgCounterRef = spineMsg.MsgCounterRef
		msg.DeviceSource = spineMsg.DeviceSource
		msg.DeviceDest = spineMsg.DeviceDest
		msg.EntitySource = spineMsg.EntitySource
		msg.EntityDest = spineMsg.EntityDest
		msg.FeatureSource = spineMsg.FeatureSource
		msg.FeatureDest = spineMsg.FeatureDest
	} else {
		msg.ParseError = "could not parse SPINE datagram from log line"
		if dir == model.DirectionOutgoing {
			msg.DeviceDest = peerDevice
		} else {
			msg.DeviceSource = peerDevice
		}
	}

	if dir == model.DirectionOutgoing && msg.DeviceDest == "" {
		msg.DeviceDest = peerDevice
	} else if dir == model.DirectionIncoming && msg.DeviceSource == "" {
		msg.DeviceSource = peerDevice
	}

	emit(msg)
}
