package store

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
)

const eetFormatVersion = "1.0"

// EETFile represents the .eet trace file format.
type EETFile struct {
	Version  string            `json:"version"`
	Trace    EETTrace          `json:"trace"`
	Messages []*EETMessage     `json:"messages"`
}

// EETTrace is the trace metadata in an .eet file.
type EETTrace struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	StartedAt   time.Time  `json:"startedAt"`
	StoppedAt   *time.Time `json:"stoppedAt,omitempty"`
}

// EETMessage is a message in an .eet file.
type EETMessage struct {
	SequenceNum   int             `json:"sequenceNum"`
	Timestamp     time.Time       `json:"timestamp"`
	Direction     string          `json:"direction"`
	SourceAddr    string          `json:"sourceAddr,omitempty"`
	DestAddr      string          `json:"destAddr,omitempty"`
	RawHex        string          `json:"rawHex"`
	NormalizedJSON json.RawMessage `json:"normalizedJson,omitempty"`
	ShipMsgType   string          `json:"shipMsgType"`
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

// ExportTrace exports a trace and its messages to a writer in .eet format.
func ExportTrace(w io.Writer, trace *model.Trace, messages []*model.Message) error {
	file := &EETFile{
		Version: eetFormatVersion,
		Trace: EETTrace{
			Name:        trace.Name,
			Description: trace.Description,
			StartedAt:   trace.StartedAt,
			StoppedAt:   trace.StoppedAt,
		},
	}

	file.Messages = make([]*EETMessage, len(messages))
	for i, m := range messages {
		file.Messages[i] = &EETMessage{
			SequenceNum:    m.SequenceNum,
			Timestamp:      m.Timestamp,
			Direction:      string(m.Direction),
			SourceAddr:     m.SourceAddr,
			DestAddr:       m.DestAddr,
			RawHex:         m.RawHex,
			NormalizedJSON: m.NormalizedJSON,
			ShipMsgType:    string(m.ShipMsgType),
			ShipPayload:    m.ShipPayload,
			SpinePayload:   m.SpinePayload,
			CmdClassifier:  m.CmdClassifier,
			FunctionSet:    m.FunctionSet,
			MsgCounter:     m.MsgCounter,
			MsgCounterRef:  m.MsgCounterRef,
			DeviceSource:   m.DeviceSource,
			DeviceDest:     m.DeviceDest,
			EntitySource:   m.EntitySource,
			EntityDest:     m.EntityDest,
			FeatureSource:  m.FeatureSource,
			FeatureDest:    m.FeatureDest,
			ParseError:     m.ParseError,
		}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(file)
}

// ImportTrace reads an .eet file and returns the trace and messages.
func ImportTrace(r io.Reader) (*model.Trace, []*model.Message, error) {
	var file EETFile
	if err := json.NewDecoder(r).Decode(&file); err != nil {
		return nil, nil, fmt.Errorf("decode eet file: %w", err)
	}

	if file.Version != eetFormatVersion {
		return nil, nil, fmt.Errorf("unsupported eet version %q (expected %q)", file.Version, eetFormatVersion)
	}

	trace := &model.Trace{
		Name:        file.Trace.Name,
		Description: file.Trace.Description,
		StartedAt:   file.Trace.StartedAt,
		StoppedAt:   file.Trace.StoppedAt,
		CreatedAt:   time.Now(),
	}

	messages := make([]*model.Message, len(file.Messages))
	for i, m := range file.Messages {
		messages[i] = &model.Message{
			SequenceNum:    m.SequenceNum,
			Timestamp:      m.Timestamp,
			Direction:      model.Direction(m.Direction),
			SourceAddr:     m.SourceAddr,
			DestAddr:       m.DestAddr,
			RawHex:         m.RawHex,
			NormalizedJSON: m.NormalizedJSON,
			ShipMsgType:    model.ShipMsgType(m.ShipMsgType),
			ShipPayload:    m.ShipPayload,
			SpinePayload:   m.SpinePayload,
			CmdClassifier:  m.CmdClassifier,
			FunctionSet:    m.FunctionSet,
			MsgCounter:     m.MsgCounter,
			MsgCounterRef:  m.MsgCounterRef,
			DeviceSource:   m.DeviceSource,
			DeviceDest:     m.DeviceDest,
			EntitySource:   m.EntitySource,
			EntityDest:     m.EntityDest,
			FeatureSource:  m.FeatureSource,
			FeatureDest:    m.FeatureDest,
			ParseError:     m.ParseError,
		}
	}

	trace.MessageCount = len(messages)
	return trace, messages, nil
}
