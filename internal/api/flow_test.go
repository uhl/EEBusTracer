package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/eebustracer/eebustracer/internal/analysis"
	"github.com/eebustracer/eebustracer/internal/model"
	"github.com/eebustracer/eebustracer/internal/store"
)

func TestAPI_FlowParticipants(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	now := time.Now()
	msgs := []*model.Message{
		{TraceID: trace.ID, SequenceNum: 1, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "read", DeviceSource: "d:_i:19667_CEM", DeviceDest: "d:_i:12345_EVSE1"},
		{TraceID: trace.ID, SequenceNum: 2, Timestamp: now.Add(time.Second), ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply", DeviceSource: "d:_i:12345_EVSE1", DeviceDest: "d:_i:19667_CEM"},
		{TraceID: trace.ID, SequenceNum: 3, Timestamp: now.Add(2 * time.Second), ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "read", DeviceSource: "d:_i:19667_CEM", DeviceDest: "d:_i:12345_EVSE1"},
	}
	msgRepo.InsertMessages(msgs)

	resp, err := http.Get(ts.URL + "/api/traces/" + strconv.FormatInt(trace.ID, 10) + "/flow/participants")
	if err != nil {
		t.Fatalf("GET flow/participants failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	var participants []analysis.FlowParticipant
	json.NewDecoder(resp.Body).Decode(&participants)
	resp.Body.Close()

	if len(participants) != 2 {
		t.Fatalf("got %d participants, want 2", len(participants))
	}
	if participants[0].ShortName != "CEM" {
		t.Errorf("participants[0].ShortName = %q, want %q", participants[0].ShortName, "CEM")
	}
	if participants[1].ShortName != "EVSE1" {
		t.Errorf("participants[1].ShortName = %q, want %q", participants[1].ShortName, "EVSE1")
	}
}

func TestAPI_FlowParticipants_Empty(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "empty", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	resp, err := http.Get(ts.URL + "/api/traces/" + strconv.FormatInt(trace.ID, 10) + "/flow/participants")
	if err != nil {
		t.Fatalf("GET flow/participants failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	var participants []analysis.FlowParticipant
	json.NewDecoder(resp.Body).Decode(&participants)
	resp.Body.Close()

	if participants == nil || len(participants) != 0 {
		t.Errorf("expected empty array, got %v", participants)
	}
}

func TestAPI_FlowParticipants_InvalidID(t *testing.T) {
	ts, _ := setupTestServer(t)

	resp, err := http.Get(ts.URL + "/api/traces/abc/flow/participants")
	if err != nil {
		t.Fatalf("GET flow/participants failed: %v", err)
	}
	if resp.StatusCode != 400 {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAPI_FlowCorrelations(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	now := time.Now()
	msgs := []*model.Message{
		{TraceID: trace.ID, SequenceNum: 1, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "read", MsgCounter: "10", DeviceSource: "devA", DeviceDest: "devB"},
		{TraceID: trace.ID, SequenceNum: 2, Timestamp: now.Add(50 * time.Millisecond), ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply", MsgCounter: "11", MsgCounterRef: "10", DeviceSource: "devB", DeviceDest: "devA"},
	}
	msgRepo.InsertMessages(msgs)

	resp, err := http.Get(ts.URL + "/api/traces/" + strconv.FormatInt(trace.ID, 10) + "/flow/correlations")
	if err != nil {
		t.Fatalf("GET flow/correlations failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	var pairs []analysis.CorrelationPair
	json.NewDecoder(resp.Body).Decode(&pairs)
	resp.Body.Close()

	if len(pairs) != 1 {
		t.Fatalf("got %d pairs, want 1", len(pairs))
	}
	if pairs[0].Relationship != "read-reply" {
		t.Errorf("Relationship = %q, want %q", pairs[0].Relationship, "read-reply")
	}
	if pairs[0].LatencyMs < 0 {
		t.Errorf("LatencyMs = %f, want >= 0", pairs[0].LatencyMs)
	}
}

func TestAPI_FlowCorrelations_NoPairs(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	now := time.Now()
	msgs := []*model.Message{
		{TraceID: trace.ID, SequenceNum: 1, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "read", MsgCounter: "10", DeviceSource: "devA", DeviceDest: "devB"},
		{TraceID: trace.ID, SequenceNum: 2, Timestamp: now.Add(time.Second), ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "notify", MsgCounter: "11", DeviceSource: "devB", DeviceDest: "devA"},
	}
	msgRepo.InsertMessages(msgs)

	resp, err := http.Get(ts.URL + "/api/traces/" + strconv.FormatInt(trace.ID, 10) + "/flow/correlations")
	if err != nil {
		t.Fatalf("GET flow/correlations failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	var pairs []analysis.CorrelationPair
	json.NewDecoder(resp.Body).Decode(&pairs)
	resp.Body.Close()

	if pairs == nil || len(pairs) != 0 {
		t.Errorf("expected empty array, got %v", pairs)
	}
}
