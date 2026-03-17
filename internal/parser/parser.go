package parser

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
)

// Parser decodes raw EEBus protocol messages.
type Parser struct{}

// New creates a new Parser.
func New() *Parser {
	return &Parser{}
}

// Parse decodes a raw byte message into a Message.
// Parse errors are stored in the Message's ParseError field rather than
// being returned as errors, so that partially decoded messages are preserved.
func (p *Parser) Parse(raw []byte, traceID int64, seqNum int, ts time.Time) *model.Message {
	msg := &model.Message{
		TraceID:     traceID,
		SequenceNum: seqNum,
		Timestamp:   ts,
		RawBytes:    raw,
		RawHex:      hex.EncodeToString(raw),
	}

	if len(raw) == 0 {
		msg.ParseError = "empty message"
		msg.ShipMsgType = model.ShipMsgTypeUnknown
		return msg
	}

	// First byte is the SHIP header
	headerByte := raw[0]
	headerType := model.ShipMsgTypeFromHeaderByte(headerByte)

	if headerType == model.ShipMsgTypeInit {
		msg.ShipMsgType = model.ShipMsgTypeInit
		return msg
	}

	// Rest is JSON payload
	if len(raw) < 2 {
		msg.ShipMsgType = model.ShipMsgTypeUnknown
		msg.ParseError = "message too short for JSON payload"
		return msg
	}

	jsonData := bytes.TrimRight(raw[1:], "\x00")

	// Determine if this is EEBUS-encoded JSON (starts with "[{")
	// EEBUS format uses arrays of single-key objects instead of standard JSON objects.
	// Only apply NormalizeEEBUSJSON if the data is in EEBUS format.
	isEEBUSFormat := len(jsonData) > 1 && jsonData[0] == '[' && jsonData[1] == '{'
	var normalized []byte
	if isEEBUSFormat {
		normalized = NormalizeEEBUSJSON(jsonData)
	} else {
		normalized = jsonData
	}
	msg.NormalizedJSON = json.RawMessage(normalized)

	// Classify SHIP message
	classification, err := classifyShipMessage(normalized)
	if err != nil {
		msg.ShipMsgType = model.ShipMsgTypeUnknown
		msg.ParseError = "SHIP classification: " + err.Error()
		return msg
	}

	msg.ShipMsgType = classification.MsgType
	msg.ShipPayload = classification.ShipPayload

	// If it's a data message, parse SPINE
	if classification.MsgType == model.ShipMsgTypeData && len(classification.DataPayload) > 0 {
		spineResult, err := parseSpineDatagram(classification.DataPayload)
		if err != nil {
			msg.ParseError = "SPINE parsing: " + err.Error()
			return msg
		}

		msg.SpinePayload = spineResult.SpinePayload
		msg.CmdClassifier = spineResult.CmdClassifier
		msg.FunctionSet = spineResult.FunctionSet
		msg.MsgCounter = spineResult.MsgCounter
		msg.MsgCounterRef = spineResult.MsgCounterRef
		msg.DeviceSource = spineResult.DeviceSource
		msg.DeviceDest = spineResult.DeviceDest
		msg.EntitySource = spineResult.EntitySource
		msg.EntityDest = spineResult.EntityDest
		msg.FeatureSource = spineResult.FeatureSource
		msg.FeatureDest = spineResult.FeatureDest
	}

	return msg
}

// ShipParseResult holds the combined SHIP classification and SPINE parse result.
type ShipParseResult struct {
	ShipMsgType model.ShipMsgType
	ShipPayload json.RawMessage
	Spine       *SpineResult // nil if not a data message or SPINE parse failed
}

// ParseShipFromJSON classifies a normalized SHIP JSON message and, if it is a
// data message, extracts SPINE fields. Used for log formats that include the
// full SHIP framing (e.g. EEBus Hub).
func (p *Parser) ParseShipFromJSON(normalized []byte) (*ShipParseResult, error) {
	classification, err := classifyShipMessage(normalized)
	if err != nil {
		return nil, err
	}

	result := &ShipParseResult{
		ShipMsgType: classification.MsgType,
		ShipPayload: classification.ShipPayload,
	}

	if classification.MsgType == model.ShipMsgTypeData && len(classification.DataPayload) > 0 {
		spine, err := parseSpineDatagram(classification.DataPayload)
		if err == nil {
			result.Spine = spine
		}
	}

	return result, nil
}

// ParseSpineFromJSON parses a normalized JSON blob that contains a SPINE
// datagram at the top level (e.g. {"datagram":{...}}). This is used for
// importing log files where the SHIP framing is already stripped.
// Returns nil if the JSON does not contain a parseable datagram.
func (p *Parser) ParseSpineFromJSON(data []byte) *SpineResult {
	// Check if the top-level key is "datagram"
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	if _, ok := raw["datagram"]; !ok {
		return nil
	}

	result, err := parseSpineDatagram(data)
	if err != nil {
		return nil
	}
	return result
}
