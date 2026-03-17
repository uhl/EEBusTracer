package store

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
)

func TestExportImportRoundTrip(t *testing.T) {
	trace := &model.Trace{
		Name:        "Test Export",
		Description: "A test trace",
		StartedAt:   time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
	}

	messages := []*model.Message{
		{
			SequenceNum:   1,
			Timestamp:     time.Date(2024, 1, 1, 12, 0, 1, 0, time.UTC),
			Direction:     model.DirectionIncoming,
			ShipMsgType:   model.ShipMsgTypeConnectionHello,
			CmdClassifier: "",
			FunctionSet:   "",
		},
		{
			SequenceNum:   2,
			Timestamp:     time.Date(2024, 1, 1, 12, 0, 2, 0, time.UTC),
			Direction:     model.DirectionIncoming,
			ShipMsgType:   model.ShipMsgTypeData,
			CmdClassifier: "read",
			FunctionSet:   "MeasurementListData",
		},
	}

	var buf bytes.Buffer
	if err := ExportTrace(&buf, trace, messages); err != nil {
		t.Fatalf("ExportTrace failed: %v", err)
	}

	// Verify it's valid JSON
	if !json.Valid(buf.Bytes()) {
		t.Fatal("exported data is not valid JSON")
	}

	// Import
	imported, importedMsgs, err := ImportTrace(&buf)
	if err != nil {
		t.Fatalf("ImportTrace failed: %v", err)
	}

	if imported.Name != trace.Name {
		t.Errorf("Name = %q, want %q", imported.Name, trace.Name)
	}
	if imported.Description != trace.Description {
		t.Errorf("Description = %q, want %q", imported.Description, trace.Description)
	}
	if len(importedMsgs) != 2 {
		t.Fatalf("len(messages) = %d, want 2", len(importedMsgs))
	}
	if importedMsgs[1].CmdClassifier != "read" {
		t.Errorf("CmdClassifier = %q, want %q", importedMsgs[1].CmdClassifier, "read")
	}
	if importedMsgs[1].FunctionSet != "MeasurementListData" {
		t.Errorf("FunctionSet = %q, want %q", importedMsgs[1].FunctionSet, "MeasurementListData")
	}
}

func TestExportEmptyTrace(t *testing.T) {
	trace := &model.Trace{
		Name:      "Empty",
		StartedAt: time.Now(),
	}

	var buf bytes.Buffer
	if err := ExportTrace(&buf, trace, nil); err != nil {
		t.Fatalf("ExportTrace failed: %v", err)
	}

	imported, msgs, err := ImportTrace(&buf)
	if err != nil {
		t.Fatalf("ImportTrace failed: %v", err)
	}
	if imported.Name != "Empty" {
		t.Errorf("Name = %q, want %q", imported.Name, "Empty")
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages, got %d", len(msgs))
	}
}

func TestImportVersionMismatch(t *testing.T) {
	data := `{"version":"99.0","trace":{"name":"test","startedAt":"2024-01-01T00:00:00Z"},"messages":[]}`
	_, _, err := ImportTrace(strings.NewReader(data))
	if err == nil {
		t.Error("expected error for version mismatch")
	}
}
