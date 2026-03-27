package model

import (
	"encoding/json"
	"testing"
	"time"
)

func TestMessage_ToSummary(t *testing.T) {
	ts := time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC)

	msg := &Message{
		ID:             42,
		TraceID:        7,
		SequenceNum:    100,
		Timestamp:      ts,
		Direction:      DirectionIncoming,
		SourceAddr:     "192.168.1.1:4711",
		DestAddr:       "192.168.1.2:4712",
		RawBytes:       []byte{0x01, 0x02},
		RawHex:         "0102",
		NormalizedJSON: json.RawMessage(`{"data":1}`),
		ShipMsgType:    ShipMsgTypeData,
		ShipPayload:    json.RawMessage(`{"ship":1}`),
		SpinePayload:   json.RawMessage(`{"spine":1}`),
		CmdClassifier:  "read",
		FunctionSet:    "MeasurementListData",
		MsgCounter:     "5",
		MsgCounterRef:  "3",
		DeviceSource:   "d:_i:CEM-001",
		DeviceDest:     "d:_i:EVSE-002",
		EntitySource:   "1",
		EntityDest:     "2",
		FeatureSource:  "7",
		FeatureDest:    "3",
		ParseError:     "",
	}

	s := msg.ToSummary()

	// Fields that must be copied
	if s.ID != msg.ID {
		t.Errorf("ID = %d, want %d", s.ID, msg.ID)
	}
	if s.TraceID != msg.TraceID {
		t.Errorf("TraceID = %d, want %d", s.TraceID, msg.TraceID)
	}
	if s.SequenceNum != msg.SequenceNum {
		t.Errorf("SequenceNum = %d, want %d", s.SequenceNum, msg.SequenceNum)
	}
	if !s.Timestamp.Equal(msg.Timestamp) {
		t.Errorf("Timestamp = %v, want %v", s.Timestamp, msg.Timestamp)
	}
	if s.Direction != msg.Direction {
		t.Errorf("Direction = %v, want %v", s.Direction, msg.Direction)
	}
	if s.ShipMsgType != msg.ShipMsgType {
		t.Errorf("ShipMsgType = %v, want %v", s.ShipMsgType, msg.ShipMsgType)
	}
	if s.CmdClassifier != msg.CmdClassifier {
		t.Errorf("CmdClassifier = %q, want %q", s.CmdClassifier, msg.CmdClassifier)
	}
	if s.FunctionSet != msg.FunctionSet {
		t.Errorf("FunctionSet = %q, want %q", s.FunctionSet, msg.FunctionSet)
	}
	if s.MsgCounter != msg.MsgCounter {
		t.Errorf("MsgCounter = %q, want %q", s.MsgCounter, msg.MsgCounter)
	}
	if s.DeviceSource != msg.DeviceSource {
		t.Errorf("DeviceSource = %q, want %q", s.DeviceSource, msg.DeviceSource)
	}
	if s.DeviceDest != msg.DeviceDest {
		t.Errorf("DeviceDest = %q, want %q", s.DeviceDest, msg.DeviceDest)
	}
}

func TestMessage_ToSummary_EmptyMessage(t *testing.T) {
	msg := &Message{}
	s := msg.ToSummary()

	if s.ID != 0 {
		t.Errorf("ID = %d, want 0", s.ID)
	}
	if s.CmdClassifier != "" {
		t.Errorf("CmdClassifier = %q, want empty", s.CmdClassifier)
	}
	if s.FunctionSet != "" {
		t.Errorf("FunctionSet = %q, want empty", s.FunctionSet)
	}
}
