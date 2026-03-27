package model

import (
	"encoding/json"
	"time"
)

// MessageSummary is a lightweight projection of Message excluding large payload
// fields. It is used for virtual-scroll listing where all matching messages are
// returned at once but only summary columns are needed.
type MessageSummary struct {
	ID            int64     `json:"id"`
	TraceID       int64     `json:"traceId"`
	SequenceNum   int       `json:"sequenceNum"`
	Timestamp     time.Time `json:"timestamp"`
	Direction     Direction `json:"direction"`
	ShipMsgType   ShipMsgType `json:"shipMsgType"`
	CmdClassifier string    `json:"cmdClassifier,omitempty"`
	FunctionSet   string    `json:"functionSet,omitempty"`
	MsgCounter    string    `json:"msgCounter,omitempty"`
	DeviceSource  string    `json:"deviceSource,omitempty"`
	DeviceDest    string    `json:"deviceDest,omitempty"`
}

// Message represents a decoded EEBus protocol message.
type Message struct {
	ID            int64           `json:"id"`
	TraceID       int64           `json:"traceId"`
	SequenceNum   int             `json:"sequenceNum"`
	Timestamp     time.Time       `json:"timestamp"`
	Direction     Direction       `json:"direction"`
	SourceAddr    string          `json:"sourceAddr,omitempty"`
	DestAddr      string          `json:"destAddr,omitempty"`
	RawBytes      []byte          `json:"-"`
	RawHex        string          `json:"rawHex,omitempty"`
	NormalizedJSON json.RawMessage `json:"normalizedJson,omitempty"`
	ShipMsgType   ShipMsgType     `json:"shipMsgType"`
	ShipPayload   json.RawMessage `json:"shipPayload,omitempty"`
	SpinePayload  json.RawMessage `json:"spinePayload,omitempty"`
	CmdClassifier string          `json:"cmdClassifier,omitempty"`
	FunctionSet   string          `json:"functionSet,omitempty"`
	MsgCounter    string          `json:"msgCounter,omitempty"`
	MsgCounterRef string          `json:"msgCounterRef,omitempty"`
	DeviceSource  string          `json:"deviceSource,omitempty"`
	DeviceDest    string          `json:"deviceDest,omitempty"`
	EntitySource  string          `json:"entitySource,omitempty"`
	EntityDest    string          `json:"entityDest,omitempty"`
	FeatureSource string          `json:"featureSource,omitempty"`
	FeatureDest   string          `json:"featureDest,omitempty"`
	ParseError    string          `json:"parseError,omitempty"`
}

// ToSummary returns a lightweight summary of the message without payload fields.
func (m *Message) ToSummary() MessageSummary {
	return MessageSummary{
		ID:            m.ID,
		TraceID:       m.TraceID,
		SequenceNum:   m.SequenceNum,
		Timestamp:     m.Timestamp,
		Direction:     m.Direction,
		ShipMsgType:   m.ShipMsgType,
		CmdClassifier: m.CmdClassifier,
		FunctionSet:   m.FunctionSet,
		MsgCounter:    m.MsgCounter,
		DeviceSource:  m.DeviceSource,
		DeviceDest:    m.DeviceDest,
	}
}
