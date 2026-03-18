package capture

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
	"github.com/eebustracer/eebustracer/internal/parser"
)

const logTailPollInterval = 100 * time.Millisecond

// LogTailSource watches an eebus-go log file and emits new messages as lines
// are appended. It seeks to end-of-file on start and polls for new data.
type LogTailSource struct {
	path   string
	parser *parser.Parser
	logger *slog.Logger
}

// NewLogTailSource creates a new log tail source.
func NewLogTailSource(path string, p *parser.Parser, logger *slog.Logger) *LogTailSource {
	return &LogTailSource{
		path:   path,
		parser: p,
		logger: logger,
	}
}

// Name returns "logtail".
func (s *LogTailSource) Name() string { return "logtail" }

// Run opens the log file, seeks to the end, and polls for new lines.
func (s *LogTailSource) Run(ctx context.Context, emit func(*model.Message)) error {
	f, err := os.Open(s.path)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer f.Close()

	// Seek to end so we only process new lines.
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return fmt.Errorf("seek to end: %w", err)
	}

	baseDate := time.Now().Truncate(24 * time.Hour)
	reader := bufio.NewReader(f)
	var partial string

	ticker := time.NewTicker(logTailPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			s.readNewLines(reader, &partial, baseDate, emit)
		}
	}
}

// readNewLines reads all available data and processes complete lines.
func (s *LogTailSource) readNewLines(reader *bufio.Reader, partial *string, baseDate time.Time, emit func(*model.Message)) {
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			*partial += line
			// If we got a full line (ends with \n), process it.
			if line[len(line)-1] == '\n' {
				s.processLine(*partial, baseDate, emit)
				*partial = ""
			}
		}
		if err != nil {
			// EOF or other error — stop reading for this tick.
			return
		}
	}
}

// processLine parses a single log line and emits a message if valid.
func (s *LogTailSource) processLine(line string, baseDate time.Time, emit func(*model.Message)) {
	// Trim trailing newline/whitespace
	line = trimRight(line)
	if line == "" {
		return
	}

	matches := parser.LogLineRegex.FindStringSubmatch(line)
	if matches == nil {
		preview := line
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		s.logger.Debug("logtail: line did not match log format, skipping",
			"line", preview,
			"len", len(line),
		)
		return
	}

	var seqNum int
	if matches[1] != "" {
		seqNum, _ = strconv.Atoi(matches[1])
	}
	timeStr := matches[2]
	direction := matches[3]
	peer := matches[4]
	jsonPayload := matches[5]

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
		SequenceNum:    seqNum,
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

// trimRight removes trailing whitespace from a string.
func trimRight(s string) string {
	for s != "" {
		c := s[len(s)-1]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			s = s[:len(s)-1]
		} else {
			break
		}
	}
	return s
}
