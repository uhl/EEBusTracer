package model

import (
	"encoding/json"
	"time"
)

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
