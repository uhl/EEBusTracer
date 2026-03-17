package store

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
	"github.com/eebustracer/eebustracer/internal/parser"
)

// ImportLogFile parses an eebus-go or CEasierLogger style .log file into a
// trace and messages. The sequence number prefix is optional — when absent,
// sequence numbers are auto-generated starting from 1.
//
//	<seq> [HH:MM:SS.mmm] SEND|RECV to|from <peer> MSG: <eebus_json>
//	[HH:MM:SS.mmm] SEND|RECV to|from <peer> MSG: <eebus_json>
func ImportLogFile(r io.Reader, name string) (*model.Trace, []*model.Message, error) {
	p := parser.New()
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // up to 10 MB per line

	var messages []*model.Message
	var firstTS, lastTS time.Time
	baseDate := time.Now().Truncate(24 * time.Hour)
	autoSeq := 0

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		matches := parser.LogLineRegex.FindStringSubmatch(line)
		if matches == nil {
			continue // skip malformed lines
		}

		var seqNum int
		if matches[1] != "" {
			seqNum, _ = strconv.Atoi(matches[1])
		} else {
			autoSeq++
			seqNum = autoSeq
		}
		timeStr := matches[2]
		direction := matches[3]
		peer := matches[4]
		jsonPayload := matches[5]

		// Parse time (HH:MM:SS.mmm) relative to base date
		ts, err := parser.ParseLogTimestamp(baseDate, timeStr)
		if err != nil {
			continue
		}

		if firstTS.IsZero() {
			firstTS = ts
		}
		lastTS = ts

		// Map direction
		var dir model.Direction
		if direction == "SEND" {
			dir = model.DirectionOutgoing
		} else {
			dir = model.DirectionIncoming
		}

		// The JSON is in EEBUS format — normalize it
		normalized := parser.NormalizeEEBUSJSON([]byte(jsonPayload))

		// Check if this is a datagram (SPINE message) or a SHIP message
		msg := &model.Message{
			SequenceNum:    seqNum,
			Timestamp:      ts,
			Direction:      dir,
			NormalizedJSON: json.RawMessage(normalized),
			ShipMsgType:    model.ShipMsgTypeData,
		}

		// Extract peer device name from ship_<name>_<hex>
		peerDevice := parser.ExtractPeerDevice(peer)

		// Try to parse as SPINE datagram using the existing parser
		spineMsg := p.ParseSpineFromJSON(normalized)
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
			// Not a datagram — might be a SHIP-level message or unparseable
			msg.ParseError = "could not parse SPINE datagram from log line"
			// Try to identify the peer at least
			if dir == model.DirectionOutgoing {
				msg.DeviceDest = peerDevice
			} else {
				msg.DeviceSource = peerDevice
			}
		}

		// Fill in peer info for the non-addressed side
		if dir == model.DirectionOutgoing && msg.DeviceDest == "" {
			msg.DeviceDest = peerDevice
		} else if dir == model.DirectionIncoming && msg.DeviceSource == "" {
			msg.DeviceSource = peerDevice
		}

		messages = append(messages, msg)
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("scan log file: %w", err)
	}

	if len(messages) == 0 {
		return nil, nil, fmt.Errorf("no valid log lines found")
	}

	trace := &model.Trace{
		Name:         name,
		StartedAt:    firstTS,
		MessageCount: len(messages),
		CreatedAt:    time.Now(),
	}
	if !lastTS.IsZero() {
		trace.StoppedAt = &lastTS
	}

	return trace, messages, nil
}

// ImportEEBusTesterLogFile parses an eebustester style .log file into a trace and messages.
// Only DATAGRAM lines (Send/Received) are extracted; all other lines are skipped.
// Sequence numbers are auto-generated starting from 1.
func ImportEEBusTesterLogFile(r io.Reader, name string) (*model.Trace, []*model.Message, error) {
	p := parser.New()
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	var messages []*model.Message
	var firstTS, lastTS time.Time
	seqNum := 0

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		matches := parser.EEBusTesterLogRegex.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		seqNum++
		dateStr := matches[1]
		timeStr := matches[2]
		direction := matches[3]
		peer := matches[4]
		jsonPayload := matches[5]

		ts, err := parser.ParseEEBusTesterTimestamp(dateStr, timeStr)
		if err != nil {
			continue
		}

		if firstTS.IsZero() {
			firstTS = ts
		}
		lastTS = ts

		var dir model.Direction
		if direction == "Send" {
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

		spineMsg := p.ParseSpineFromJSON(normalized)
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

		messages = append(messages, msg)
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("scan log file: %w", err)
	}

	if len(messages) == 0 {
		return nil, nil, fmt.Errorf("no valid DATAGRAM lines found")
	}

	trace := &model.Trace{
		Name:         name,
		StartedAt:    firstTS,
		MessageCount: len(messages),
		CreatedAt:    time.Now(),
	}
	if !lastTS.IsZero() {
		trace.StoppedAt = &lastTS
	}

	return trace, messages, nil
}

// ImportEEBusHubLogFile parses an EEBus Hub style .log file into a trace and messages.
// The format is: YYYY-MM-DD HH:MM:SS    [Send|Recv] <SKI:40hex><JSON>
// Sequence numbers are auto-generated starting from 1.
func ImportEEBusHubLogFile(r io.Reader, name string) (*model.Trace, []*model.Message, error) {
	p := parser.New()
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	var messages []*model.Message
	var firstTS, lastTS time.Time
	seqNum := 0

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		matches := parser.EEBusHubLogRegex.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		seqNum++
		dateStr := matches[1]
		timeStr := matches[2]
		direction := matches[3]
		ski := matches[4]
		jsonPayload := matches[5]

		ts, err := parser.ParseEEBusHubTimestamp(dateStr, timeStr)
		if err != nil {
			continue
		}

		if firstTS.IsZero() {
			firstTS = ts
		}
		lastTS = ts

		var dir model.Direction
		if direction == "Send" {
			dir = model.DirectionOutgoing
		} else {
			dir = model.DirectionIncoming
		}

		// The JSON is in EEBUS format — normalize it
		normalized := parser.NormalizeEEBUSJSON([]byte(jsonPayload))

		msg := &model.Message{
			SequenceNum:    seqNum,
			Timestamp:      ts,
			Direction:      dir,
			NormalizedJSON: json.RawMessage(normalized),
		}

		// Parse as full SHIP message (includes SHIP framing + SPINE)
		shipResult, err := p.ParseShipFromJSON(normalized)
		if err != nil {
			msg.ShipMsgType = model.ShipMsgTypeUnknown
			msg.ParseError = "SHIP classification: " + err.Error()
		} else {
			msg.ShipMsgType = shipResult.ShipMsgType
			msg.ShipPayload = shipResult.ShipPayload

			if shipResult.Spine != nil {
				msg.SpinePayload = shipResult.Spine.SpinePayload
				msg.CmdClassifier = shipResult.Spine.CmdClassifier
				msg.FunctionSet = shipResult.Spine.FunctionSet
				msg.MsgCounter = shipResult.Spine.MsgCounter
				msg.MsgCounterRef = shipResult.Spine.MsgCounterRef
				msg.DeviceSource = shipResult.Spine.DeviceSource
				msg.DeviceDest = shipResult.Spine.DeviceDest
				msg.EntitySource = shipResult.Spine.EntitySource
				msg.EntityDest = shipResult.Spine.EntityDest
				msg.FeatureSource = shipResult.Spine.FeatureSource
				msg.FeatureDest = shipResult.Spine.FeatureDest
			}
		}

		// Use SKI as fallback peer device when SPINE didn't provide it
		if dir == model.DirectionOutgoing && msg.DeviceDest == "" {
			msg.DeviceDest = ski
		} else if dir == model.DirectionIncoming && msg.DeviceSource == "" {
			msg.DeviceSource = ski
		}

		messages = append(messages, msg)
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("scan log file: %w", err)
	}

	if len(messages) == 0 {
		return nil, nil, fmt.Errorf("no valid EEBus Hub log lines found")
	}

	trace := &model.Trace{
		Name:         name,
		StartedAt:    firstTS,
		MessageCount: len(messages),
		CreatedAt:    time.Now(),
	}
	if !lastTS.IsZero() {
		trace.StoppedAt = &lastTS
	}

	return trace, messages, nil
}

// ImportLogFileAutoDetect reads a .log file, detects its format (eebus-go, eebustester,
// or EEBus Hub), and delegates to the appropriate importer.
func ImportLogFileAutoDetect(r io.Reader, name string) (*model.Trace, []*model.Message, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, fmt.Errorf("read log file: %w", err)
	}

	// Peek at first 4KB for format detection
	peek := string(data)
	if len(peek) > 4096 {
		peek = peek[:4096]
	}

	format := parser.DetectLogFormat(peek)
	reader := strings.NewReader(string(data))

	switch format {
	case parser.LogFormatEEBusGo:
		return ImportLogFile(reader, name)
	case parser.LogFormatEEBusTester:
		return ImportEEBusTesterLogFile(reader, name)
	case parser.LogFormatEEBusHub:
		return ImportEEBusHubLogFile(reader, name)
	default:
		return nil, nil, fmt.Errorf("unrecognized log format")
	}
}

// ImportFileAutoDetect reads a file and imports it based on content detection.
// It first tries to detect a known log format; if none is found, it falls back
// to EET (JSON) import. This allows importing files regardless of extension.
func ImportFileAutoDetect(r io.Reader, name string) (*model.Trace, []*model.Message, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, fmt.Errorf("read file: %w", err)
	}

	// Peek at first 4KB for format detection
	peek := string(data)
	if len(peek) > 4096 {
		peek = peek[:4096]
	}

	format := parser.DetectLogFormat(peek)
	reader := strings.NewReader(string(data))

	switch format {
	case parser.LogFormatEEBusGo:
		return ImportLogFile(reader, name)
	case parser.LogFormatEEBusTester:
		return ImportEEBusTesterLogFile(reader, name)
	case parser.LogFormatEEBusHub:
		return ImportEEBusHubLogFile(reader, name)
	default:
		// Fall back to EET format
		return ImportTrace(reader)
	}
}
