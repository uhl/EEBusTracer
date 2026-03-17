package analysis

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
)

func TestComputeHeartbeatMetrics_Empty(t *testing.T) {
	result := ComputeHeartbeatMetrics(nil)
	if len(result.HeartbeatJitter) != 0 {
		t.Errorf("expected empty jitter, got %d", len(result.HeartbeatJitter))
	}
}

func TestComputeHeartbeatJitter(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	msgs := []*model.Message{
		{ID: 1, Timestamp: now, FunctionSet: "DeviceDiagnosisHeartbeatData", DeviceSource: "A", DeviceDest: "B", ShipMsgType: "data"},
		{ID: 2, Timestamp: now.Add(10 * time.Second), FunctionSet: "DeviceDiagnosisHeartbeatData", DeviceSource: "A", DeviceDest: "B", ShipMsgType: "data"},
		{ID: 3, Timestamp: now.Add(20 * time.Second), FunctionSet: "DeviceDiagnosisHeartbeatData", DeviceSource: "A", DeviceDest: "B", ShipMsgType: "data"},
		{ID: 4, Timestamp: now.Add(35 * time.Second), FunctionSet: "DeviceDiagnosisHeartbeatData", DeviceSource: "A", DeviceDest: "B", ShipMsgType: "data"},
	}

	jitters := computeHeartbeatJitter(msgs)
	if len(jitters) != 1 {
		t.Fatalf("expected 1 jitter entry, got %d", len(jitters))
	}

	j := jitters[0]
	if j.SampleCount != 3 {
		t.Errorf("sampleCount = %d, want 3", j.SampleCount)
	}
	if j.MinMs != 10000 {
		t.Errorf("minMs = %f, want 10000", j.MinMs)
	}
	if j.MaxMs != 15000 {
		t.Errorf("maxMs = %f, want 15000", j.MaxMs)
	}
	// Mean: (10000 + 10000 + 15000) / 3 ≈ 11666.67
	expectedMean := (10000.0 + 10000.0 + 15000.0) / 3.0
	if math.Abs(j.MeanMs-expectedMean) > 1 {
		t.Errorf("meanMs = %f, want %f", j.MeanMs, expectedMean)
	}
	if j.StdDevMs < 1 {
		t.Errorf("expected non-zero stdDev, got %f", j.StdDevMs)
	}
}

func TestComputeHeartbeatJitter_SingleHeartbeat(t *testing.T) {
	now := time.Now()
	msgs := []*model.Message{
		{ID: 1, Timestamp: now, FunctionSet: "DeviceDiagnosisHeartbeatData", DeviceSource: "A", DeviceDest: "B", ShipMsgType: "data"},
	}

	jitters := computeHeartbeatJitter(msgs)
	if len(jitters) != 0 {
		t.Errorf("expected 0 jitter entries for single heartbeat, got %d", len(jitters))
	}
}

func TestFormatHeartbeatCSV(t *testing.T) {
	jitters := []HeartbeatJitter{
		{DevicePair: "A → B", MeanMs: 10000, StdDevMs: 500, MinMs: 9500, MaxMs: 10500, SampleCount: 5},
	}

	csv := FormatHeartbeatCSV(jitters)
	lines := strings.Split(strings.TrimSpace(csv), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (header + data), got %d", len(lines))
	}
	if !strings.HasPrefix(lines[0], "devicePair,") {
		t.Errorf("expected CSV header, got %q", lines[0])
	}
	if !strings.Contains(lines[1], "A → B") {
		t.Errorf("expected device pair in CSV, got %q", lines[1])
	}
	if !strings.Contains(lines[1], "10000.0") {
		t.Errorf("expected mean value in CSV, got %q", lines[1])
	}
}

func TestComputeHeartbeatJitter_BothDirections(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	msgs := []*model.Message{
		// A→B heartbeats every 10s
		{ID: 1, Timestamp: now, FunctionSet: "DeviceDiagnosisHeartbeatData", DeviceSource: "A", DeviceDest: "B"},
		// B→A heartbeats every 5s (different cadence)
		{ID: 2, Timestamp: now.Add(3 * time.Second), FunctionSet: "DeviceDiagnosisHeartbeatData", DeviceSource: "B", DeviceDest: "A"},
		{ID: 3, Timestamp: now.Add(8 * time.Second), FunctionSet: "DeviceDiagnosisHeartbeatData", DeviceSource: "B", DeviceDest: "A"},
		{ID: 4, Timestamp: now.Add(10 * time.Second), FunctionSet: "DeviceDiagnosisHeartbeatData", DeviceSource: "A", DeviceDest: "B"},
		{ID: 5, Timestamp: now.Add(13 * time.Second), FunctionSet: "DeviceDiagnosisHeartbeatData", DeviceSource: "B", DeviceDest: "A"},
		{ID: 6, Timestamp: now.Add(20 * time.Second), FunctionSet: "DeviceDiagnosisHeartbeatData", DeviceSource: "A", DeviceDest: "B"},
	}

	jitters := computeHeartbeatJitter(msgs)
	if len(jitters) != 2 {
		t.Fatalf("expected 2 jitter entries (one per direction), got %d", len(jitters))
	}

	// Find each direction
	var ab, ba *HeartbeatJitter
	for i := range jitters {
		if strings.Contains(jitters[i].DevicePair, "A → B") {
			ab = &jitters[i]
		}
		if strings.Contains(jitters[i].DevicePair, "B → A") {
			ba = &jitters[i]
		}
	}
	if ab == nil {
		t.Fatal("missing A → B jitter entry")
	}
	if ba == nil {
		t.Fatal("missing B → A jitter entry")
	}

	// A→B: intervals 10s, 10s → mean 10000ms
	if ab.SampleCount != 2 {
		t.Errorf("A→B sampleCount = %d, want 2", ab.SampleCount)
	}
	if math.Abs(ab.MeanMs-10000) > 1 {
		t.Errorf("A→B meanMs = %f, want 10000", ab.MeanMs)
	}

	// B→A: intervals 5s, 5s → mean 5000ms
	if ba.SampleCount != 2 {
		t.Errorf("B→A sampleCount = %d, want 2", ba.SampleCount)
	}
	if math.Abs(ba.MeanMs-5000) > 1 {
		t.Errorf("B→A meanMs = %f, want 5000", ba.MeanMs)
	}
}

func TestComputeHeartbeatMetrics_Integration(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	msgs := []*model.Message{
		{ID: 1, Timestamp: now, FunctionSet: "DeviceDiagnosisHeartbeatData", DeviceSource: "A", DeviceDest: "B", ShipMsgType: "data"},
		{ID: 2, Timestamp: now.Add(10 * time.Second), FunctionSet: "DeviceDiagnosisHeartbeatData", DeviceSource: "A", DeviceDest: "B", ShipMsgType: "data"},
		// Non-heartbeat message should be ignored
		{ID: 3, Timestamp: now.Add(15 * time.Second), FunctionSet: "MeasurementListData", DeviceSource: "A", DeviceDest: "B", ShipMsgType: "data"},
		{ID: 4, Timestamp: now.Add(20 * time.Second), FunctionSet: "DeviceDiagnosisHeartbeatData", DeviceSource: "A", DeviceDest: "B", ShipMsgType: "data"},
	}

	result := ComputeHeartbeatMetrics(msgs)
	if len(result.HeartbeatJitter) != 1 {
		t.Fatalf("expected 1 jitter entry, got %d", len(result.HeartbeatJitter))
	}
	if result.HeartbeatJitter[0].SampleCount != 2 {
		t.Errorf("sampleCount = %d, want 2", result.HeartbeatJitter[0].SampleCount)
	}
}
