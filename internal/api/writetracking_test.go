package api

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
	"github.com/eebustracer/eebustracer/internal/store"
)

func TestExtractWriteEntries_LoadControl(t *testing.T) {
	payload := json.RawMessage(`{
		"datagram": {"payload": {"cmd": [{"loadControlLimitListData": {
			"loadControlLimitData": [
				{"limitId": 0, "isLimitActive": true, "value": {"number": 4600, "scale": 0}},
				{"limitId": 1, "isLimitActive": false, "value": {"number": 0, "scale": 0}}
			]
		}}]}}
	}`)

	desc := builtInDescriptors["loadcontrol"]
	items := extractGenericData(payload, desc)

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	if items[0].ID != "0" || items[0].Value != 4600.0 {
		t.Errorf("item 0: ID=%q Value=%f, want ID=0 Value=4600", items[0].ID, items[0].Value)
	}
	if items[0].IsActive == nil || *items[0].IsActive != true {
		t.Errorf("item 0: IsActive = %v, want true", items[0].IsActive)
	}

	if items[1].ID != "1" || items[1].Value != 0.0 {
		t.Errorf("item 1: ID=%q Value=%f, want ID=1 Value=0", items[1].ID, items[1].Value)
	}
	if items[1].IsActive == nil || *items[1].IsActive != false {
		t.Errorf("item 1: IsActive = %v, want false", items[1].IsActive)
	}
}

func TestExtractWriteEntries_Setpoint(t *testing.T) {
	payload := json.RawMessage(`{
		"datagram": {"payload": {"cmd": [{"setpointListData": {
			"setpointData": [
				{"setpointId": 1, "isSetpointActive": true, "value": {"number": -1650, "scale": 0}}
			]
		}}]}}
	}`)

	desc := builtInDescriptors["setpoint"]
	items := extractGenericData(payload, desc)

	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	if items[0].ID != "1" || items[0].Value != -1650.0 {
		t.Errorf("item 0: ID=%q Value=%f, want ID=1 Value=-1650", items[0].ID, items[0].Value)
	}
	if items[0].IsActive == nil || *items[0].IsActive != true {
		t.Errorf("item 0: IsActive = %v, want true", items[0].IsActive)
	}
}

func TestExtractWriteEntries_EmptyPayload(t *testing.T) {
	payload := json.RawMessage(`{}`)

	desc := builtInDescriptors["loadcontrol"]
	items := extractGenericData(payload, desc)

	if len(items) != 0 {
		t.Errorf("expected 0 items for empty payload, got %d", len(items))
	}
}

func TestComputeWriteDurations(t *testing.T) {
	now := time.Now()
	entries := []WriteEntry{
		{ItemID: "0", Timestamp: now},
		{ItemID: "0", Timestamp: now.Add(5 * time.Second)},
		{ItemID: "1", Timestamp: now.Add(1 * time.Second)},
		{ItemID: "0", Timestamp: now.Add(10 * time.Second)},
		{ItemID: "1", Timestamp: now.Add(3 * time.Second)},
	}

	computeWriteDurations(entries)

	// ItemID "0": entries[0] → entries[1] = 5s, entries[1] → entries[3] = 5s, entries[3] = nil
	if entries[0].DurationMs == nil {
		t.Fatal("entries[0].DurationMs should not be nil")
	}
	if math.Abs(*entries[0].DurationMs-5000) > 1 {
		t.Errorf("entries[0].DurationMs = %f, want 5000", *entries[0].DurationMs)
	}

	if entries[1].DurationMs == nil {
		t.Fatal("entries[1].DurationMs should not be nil")
	}
	if math.Abs(*entries[1].DurationMs-5000) > 1 {
		t.Errorf("entries[1].DurationMs = %f, want 5000", *entries[1].DurationMs)
	}

	if entries[3].DurationMs != nil {
		t.Errorf("entries[3].DurationMs = %f, want nil (last write for ID 0)", *entries[3].DurationMs)
	}

	// ItemID "1": entries[2] → entries[4] = 2s, entries[4] = nil
	if entries[2].DurationMs == nil {
		t.Fatal("entries[2].DurationMs should not be nil")
	}
	if math.Abs(*entries[2].DurationMs-2000) > 1 {
		t.Errorf("entries[2].DurationMs = %f, want 2000", *entries[2].DurationMs)
	}

	if entries[4].DurationMs != nil {
		t.Errorf("entries[4].DurationMs = %f, want nil (last write for ID 1)", *entries[4].DurationMs)
	}
}

func TestComputeWriteDurations_SinglePerID(t *testing.T) {
	entries := []WriteEntry{
		{ItemID: "0", Timestamp: time.Now()},
		{ItemID: "1", Timestamp: time.Now()},
	}

	computeWriteDurations(entries)

	if entries[0].DurationMs != nil {
		t.Errorf("entries[0].DurationMs = %f, want nil", *entries[0].DurationMs)
	}
	if entries[1].DurationMs != nil {
		t.Errorf("entries[1].DurationMs = %f, want nil", *entries[1].DurationMs)
	}
}

func TestExtractCmdArray_SingleObject(t *testing.T) {
	payload := json.RawMessage(`{
		"datagram": {
			"payload": {
				"cmd": {"resultData": {"errorNumber": 0}}
			}
		}
	}`)

	cmds, err := extractCmdArray(payload)
	if err != nil {
		t.Fatalf("extractCmdArray failed: %v", err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 cmd, got %d", len(cmds))
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(cmds[0], &m); err != nil {
		t.Fatalf("unmarshal cmd failed: %v", err)
	}
	if _, ok := m["resultData"]; !ok {
		t.Error("expected resultData in cmd")
	}
}

// --- Integration tests ---

func TestAPI_WriteTracking_Basic(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	now := time.Now()

	// Write message with accepted result
	writePayload := `{"datagram":{"payload":{"cmd":[{"loadControlLimitListData":{"loadControlLimitData":[{"limitId":0,"isLimitActive":true,"value":{"number":4600,"scale":0}}]}}]}}}`
	resultPayload := `{"datagram":{"payload":{"cmd":[{"resultData":{"errorNumber":0}}]}}}`

	msgs := []*model.Message{
		{
			TraceID: trace.ID, SequenceNum: 1, Timestamp: now,
			ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "write",
			FunctionSet: "LoadControlLimitListData", MsgCounter: "10",
			DeviceSource: "devA", DeviceDest: "devB",
			SpinePayload: json.RawMessage(writePayload),
		},
		{
			TraceID: trace.ID, SequenceNum: 2, Timestamp: now.Add(50 * time.Millisecond),
			ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply",
			FunctionSet: "LoadControlLimitListData", MsgCounter: "11", MsgCounterRef: "10",
			DeviceSource: "devB", DeviceDest: "devA",
			SpinePayload: json.RawMessage(resultPayload),
		},
	}
	msgRepo.InsertMessages(msgs)

	resp, err := http.Get(ts.URL + "/api/traces/" + strconv.FormatInt(trace.ID, 10) + "/writetracking")
	if err != nil {
		t.Fatalf("GET writetracking failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("GET writetracking status = %d, want 200", resp.StatusCode)
	}

	var result WriteTrackingResponse
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	if len(result.Writes) != 1 {
		t.Fatalf("expected 1 write, got %d", len(result.Writes))
	}

	w := result.Writes[0]
	if w.DataType != "loadcontrol" {
		t.Errorf("dataType = %q, want %q", w.DataType, "loadcontrol")
	}
	if w.ItemID != "0" {
		t.Errorf("itemID = %q, want %q", w.ItemID, "0")
	}
	if w.Value != 4600 {
		t.Errorf("value = %f, want 4600", w.Value)
	}
	if w.IsActive == nil || *w.IsActive != true {
		t.Errorf("isActive = %v, want true", w.IsActive)
	}
	if w.Result != "accepted" {
		t.Errorf("result = %q, want %q", w.Result, "accepted")
	}
	if w.LatencyMs == nil {
		t.Error("expected latencyMs to be set")
	}
	if w.Source != "devA" {
		t.Errorf("source = %q, want %q", w.Source, "devA")
	}

	// Effective state
	if len(result.EffectiveState) != 1 {
		t.Fatalf("expected 1 effective state entry, got %d", len(result.EffectiveState))
	}
	if result.EffectiveState[0].Value != 4600 {
		t.Errorf("effective value = %f, want 4600", result.EffectiveState[0].Value)
	}
}

func TestAPI_WriteTracking_WithSetpoint(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	now := time.Now()

	lcPayload := `{"datagram":{"payload":{"cmd":[{"loadControlLimitListData":{"loadControlLimitData":[{"limitId":0,"isLimitActive":true,"value":{"number":4600,"scale":0}}]}}]}}}`
	spPayload := `{"datagram":{"payload":{"cmd":[{"setpointListData":{"setpointData":[{"setpointId":1,"isSetpointActive":true,"value":{"number":-1650,"scale":0}}]}}]}}}`

	msgs := []*model.Message{
		{
			TraceID: trace.ID, SequenceNum: 1, Timestamp: now,
			ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "write",
			FunctionSet: "LoadControlLimitListData",
			DeviceSource: "devA", DeviceDest: "devB",
			SpinePayload: json.RawMessage(lcPayload),
		},
		{
			TraceID: trace.ID, SequenceNum: 2, Timestamp: now.Add(time.Second),
			ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "write",
			FunctionSet: "SetpointListData",
			DeviceSource: "devA", DeviceDest: "devB",
			SpinePayload: json.RawMessage(spPayload),
		},
	}
	msgRepo.InsertMessages(msgs)

	resp, err := http.Get(ts.URL + "/api/traces/" + strconv.FormatInt(trace.ID, 10) + "/writetracking")
	if err != nil {
		t.Fatalf("GET writetracking failed: %v", err)
	}

	var result WriteTrackingResponse
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	if len(result.Writes) != 2 {
		t.Fatalf("expected 2 writes, got %d", len(result.Writes))
	}

	// Verify we have both types
	types := map[string]bool{}
	for _, w := range result.Writes {
		types[w.DataType] = true
	}
	if !types["loadcontrol"] {
		t.Error("expected loadcontrol write")
	}
	if !types["setpoint"] {
		t.Error("expected setpoint write")
	}
}

func TestAPI_WriteTracking_Empty(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	resp, err := http.Get(ts.URL + "/api/traces/" + strconv.FormatInt(trace.ID, 10) + "/writetracking")
	if err != nil {
		t.Fatalf("GET writetracking failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("GET writetracking status = %d, want 200", resp.StatusCode)
	}

	var result WriteTrackingResponse
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	if len(result.Writes) != 0 {
		t.Errorf("expected 0 writes, got %d", len(result.Writes))
	}
	if len(result.EffectiveState) != 0 {
		t.Errorf("expected 0 effective state, got %d", len(result.EffectiveState))
	}
}

func TestAPI_WriteTracking_RejectedResult(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	now := time.Now()

	writePayload := `{"datagram":{"payload":{"cmd":[{"loadControlLimitListData":{"loadControlLimitData":[{"limitId":0,"isLimitActive":true,"value":{"number":4600,"scale":0}}]}}]}}}`
	rejectedPayload := `{"datagram":{"payload":{"cmd":[{"resultData":{"errorNumber":1}}]}}}`

	msgs := []*model.Message{
		{
			TraceID: trace.ID, SequenceNum: 1, Timestamp: now,
			ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "write",
			FunctionSet: "LoadControlLimitListData", MsgCounter: "20",
			DeviceSource: "devA", DeviceDest: "devB",
			SpinePayload: json.RawMessage(writePayload),
		},
		{
			TraceID: trace.ID, SequenceNum: 2, Timestamp: now.Add(100 * time.Millisecond),
			ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply",
			FunctionSet: "LoadControlLimitListData", MsgCounter: "21", MsgCounterRef: "20",
			DeviceSource: "devB", DeviceDest: "devA",
			SpinePayload: json.RawMessage(rejectedPayload),
		},
	}
	msgRepo.InsertMessages(msgs)

	resp, err := http.Get(ts.URL + "/api/traces/" + strconv.FormatInt(trace.ID, 10) + "/writetracking")
	if err != nil {
		t.Fatalf("GET writetracking failed: %v", err)
	}

	var result WriteTrackingResponse
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	if len(result.Writes) != 1 {
		t.Fatalf("expected 1 write, got %d", len(result.Writes))
	}
	if result.Writes[0].Result != "rejected" {
		t.Errorf("result = %q, want %q", result.Writes[0].Result, "rejected")
	}
}

func TestAPI_WriteTracking_DescriptionEnrichment(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	now := time.Now()

	// Limit description message
	descPayload := `{"datagram":{"payload":{"cmd":[{"loadControlLimitDescriptionListData":{"loadControlLimitDescriptionData":[{"limitId":0,"scopeType":"overloadProtection","unit":"W"}]}}]}}}`

	// Write message
	writePayload := `{"datagram":{"payload":{"cmd":[{"loadControlLimitListData":{"loadControlLimitData":[{"limitId":0,"isLimitActive":true,"value":{"number":4600,"scale":0}}]}}]}}}`

	msgs := []*model.Message{
		{
			TraceID: trace.ID, SequenceNum: 1, Timestamp: now,
			ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply",
			FunctionSet: "LoadControlLimitDescriptionListData",
			SpinePayload: json.RawMessage(descPayload),
		},
		{
			TraceID: trace.ID, SequenceNum: 2, Timestamp: now.Add(time.Second),
			ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "write",
			FunctionSet: "LoadControlLimitListData",
			DeviceSource: "devA", DeviceDest: "devB",
			SpinePayload: json.RawMessage(writePayload),
		},
	}
	msgRepo.InsertMessages(msgs)

	resp, err := http.Get(ts.URL + "/api/traces/" + strconv.FormatInt(trace.ID, 10) + "/writetracking")
	if err != nil {
		t.Fatalf("GET writetracking failed: %v", err)
	}

	var result WriteTrackingResponse
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	if len(result.Writes) != 1 {
		t.Fatalf("expected 1 write, got %d", len(result.Writes))
	}

	w := result.Writes[0]
	if w.Label != "Overload Protection [W]" {
		t.Errorf("label = %q, want %q", w.Label, "Overload Protection [W]")
	}
	if w.Unit != "W" {
		t.Errorf("unit = %q, want %q", w.Unit, "W")
	}
	if w.ScopeType != "overloadProtection" {
		t.Errorf("scopeType = %q, want %q", w.ScopeType, "overloadProtection")
	}
}

func TestAPI_WriteTracking_Duration(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	now := time.Now()

	payload1 := `{"datagram":{"payload":{"cmd":[{"loadControlLimitListData":{"loadControlLimitData":[{"limitId":0,"isLimitActive":true,"value":{"number":4600,"scale":0}}]}}]}}}`
	payload2 := `{"datagram":{"payload":{"cmd":[{"loadControlLimitListData":{"loadControlLimitData":[{"limitId":0,"isLimitActive":true,"value":{"number":6000,"scale":0}}]}}]}}}`

	msgs := []*model.Message{
		{
			TraceID: trace.ID, SequenceNum: 1, Timestamp: now,
			ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "write",
			FunctionSet: "LoadControlLimitListData",
			DeviceSource: "devA", DeviceDest: "devB",
			SpinePayload: json.RawMessage(payload1),
		},
		{
			TraceID: trace.ID, SequenceNum: 2, Timestamp: now.Add(5 * time.Second),
			ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "write",
			FunctionSet: "LoadControlLimitListData",
			DeviceSource: "devA", DeviceDest: "devB",
			SpinePayload: json.RawMessage(payload2),
		},
	}
	msgRepo.InsertMessages(msgs)

	resp, err := http.Get(ts.URL + "/api/traces/" + strconv.FormatInt(trace.ID, 10) + "/writetracking")
	if err != nil {
		t.Fatalf("GET writetracking failed: %v", err)
	}

	var result WriteTrackingResponse
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	if len(result.Writes) != 2 {
		t.Fatalf("expected 2 writes, got %d", len(result.Writes))
	}

	// First write should have duration ~5000ms
	if result.Writes[0].DurationMs == nil {
		t.Fatal("first write durationMs should not be nil")
	}
	if math.Abs(*result.Writes[0].DurationMs-5000) > 10 {
		t.Errorf("durationMs = %f, want ~5000", *result.Writes[0].DurationMs)
	}

	// Second write should have nil duration (last write)
	if result.Writes[1].DurationMs != nil {
		t.Errorf("last write durationMs = %f, want nil", *result.Writes[1].DurationMs)
	}

	// Effective state should show the latest value
	if len(result.EffectiveState) != 1 {
		t.Fatalf("expected 1 effective state, got %d", len(result.EffectiveState))
	}
	if result.EffectiveState[0].Value != 6000 {
		t.Errorf("effective value = %f, want 6000", result.EffectiveState[0].Value)
	}
}
