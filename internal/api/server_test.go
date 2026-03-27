package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eebustracer/eebustracer/internal/capture"
	"github.com/eebustracer/eebustracer/internal/model"
	"github.com/eebustracer/eebustracer/internal/parser"
	"github.com/eebustracer/eebustracer/internal/store"
)

func setupTestServer(t *testing.T) (*httptest.Server, *store.DB) {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	traceRepo := store.NewTraceRepo(db)
	msgRepo := store.NewMessageRepo(db)
	deviceRepo := store.NewDeviceRepo(db)
	presetRepo := store.NewPresetRepo(db)
	bookmarkRepo := store.NewBookmarkRepo(db)
	chartRepo := store.NewChartRepo(db)
	p := parser.New()
	engine := capture.NewEngine(p, msgRepo, logger)
	hub := NewHub(logger)

	srv := NewServer(traceRepo, msgRepo, deviceRepo, presetRepo, bookmarkRepo, chartRepo, engine, hub, nil, "test", nil, logger)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, db
}

func TestAPI_TracesCRUD(t *testing.T) {
	ts, _ := setupTestServer(t)

	// List empty
	resp, err := http.Get(ts.URL + "/api/traces")
	if err != nil {
		t.Fatalf("GET /api/traces failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET /api/traces status = %d, want 200", resp.StatusCode)
	}
	var traces []model.Trace
	json.NewDecoder(resp.Body).Decode(&traces)
	resp.Body.Close()
	if len(traces) != 0 {
		t.Errorf("expected empty traces, got %d", len(traces))
	}

	// Create
	body := `{"name":"Test Trace"}`
	resp, err = http.Post(ts.URL+"/api/traces", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/traces failed: %v", err)
	}
	if resp.StatusCode != 201 {
		t.Errorf("POST /api/traces status = %d, want 201", resp.StatusCode)
	}
	var created model.Trace
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	if created.ID == 0 {
		t.Error("expected ID to be set")
	}

	// Get by ID
	resp, err = http.Get(ts.URL + "/api/traces/1")
	if err != nil {
		t.Fatalf("GET /api/traces/1 failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET /api/traces/1 status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// Delete
	req, _ := http.NewRequest("DELETE", ts.URL+"/api/traces/1", http.NoBody)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /api/traces/1 failed: %v", err)
	}
	if resp.StatusCode != 204 {
		t.Errorf("DELETE status = %d, want 204", resp.StatusCode)
	}
	resp.Body.Close()

	// Get after delete → 404
	resp, err = http.Get(ts.URL + "/api/traces/1")
	if err != nil {
		t.Fatalf("GET after delete failed: %v", err)
	}
	if resp.StatusCode != 404 {
		t.Errorf("GET after delete status = %d, want 404", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAPI_RenameTrace(t *testing.T) {
	ts, db := setupTestServer(t)

	// Create a trace to rename
	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "Original Name", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	tests := []struct {
		name       string
		url        string
		body       string
		wantStatus int
		wantName   string
	}{
		{
			name:       "rename success",
			url:        ts.URL + "/api/traces/1",
			body:       `{"name":"Renamed Trace"}`,
			wantStatus: 200,
			wantName:   "Renamed Trace",
		},
		{
			name:       "rename not found",
			url:        ts.URL + "/api/traces/999",
			body:       `{"name":"Nope"}`,
			wantStatus: 404,
		},
		{
			name:       "rename empty name",
			url:        ts.URL + "/api/traces/1",
			body:       `{"name":""}`,
			wantStatus: 400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("PATCH", tt.url, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("PATCH failed: %v", err)
			}
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
			if tt.wantName != "" {
				var updated model.Trace
				json.NewDecoder(resp.Body).Decode(&updated)
				if updated.Name != tt.wantName {
					t.Errorf("name = %q, want %q", updated.Name, tt.wantName)
				}
			}
			resp.Body.Close()
		})
	}
}

func TestAPI_Messages(t *testing.T) {
	ts, db := setupTestServer(t)

	// Create trace and messages
	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	for i := 0; i < 15; i++ {
		msg := &model.Message{
			TraceID:       trace.ID,
			SequenceNum:   i + 1,
			Timestamp:     time.Now(),
			ShipMsgType:   model.ShipMsgTypeData,
			CmdClassifier: "read",
		}
		msgRepo.InsertMessage(msg)
	}

	// List with pagination
	resp, err := http.Get(ts.URL + "/api/traces/1/messages?limit=10&offset=0")
	if err != nil {
		t.Fatalf("GET messages failed: %v", err)
	}
	var msgs []model.Message
	json.NewDecoder(resp.Body).Decode(&msgs)
	resp.Body.Close()
	if len(msgs) != 10 {
		t.Errorf("expected 10 messages, got %d", len(msgs))
	}

	// Filter by cmdClassifier
	resp, err = http.Get(ts.URL + "/api/traces/1/messages?cmdClassifier=read")
	if err != nil {
		t.Fatalf("GET filtered messages failed: %v", err)
	}
	json.NewDecoder(resp.Body).Decode(&msgs)
	resp.Body.Close()
	if len(msgs) != 15 {
		t.Errorf("expected 15 read messages, got %d", len(msgs))
	}
}

func TestAPI_ImportExportRoundTrip(t *testing.T) {
	ts, _ := setupTestServer(t)

	// Create initial data via import
	eebtData := `{"version":"1.0","trace":{"name":"imported","startedAt":"2024-01-01T12:00:00Z"},"messages":[{"sequenceNum":1,"timestamp":"2024-01-01T12:00:01Z","direction":"incoming","rawHex":"00","shipMsgType":"init"}]}`

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("file", "test.eet")
	fw.Write([]byte(eebtData))
	w.Close()

	resp, err := http.Post(ts.URL+"/api/traces/import", w.FormDataContentType(), &buf)
	if err != nil {
		t.Fatalf("POST import failed: %v", err)
	}
	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST import status = %d, body = %s", resp.StatusCode, body)
	}
	var imported model.Trace
	json.NewDecoder(resp.Body).Decode(&imported)
	resp.Body.Close()

	if imported.Name != "imported" {
		t.Errorf("Name = %q, want %q", imported.Name, "imported")
	}

	// Export
	resp, err = http.Get(ts.URL + "/api/traces/1/export")
	if err != nil {
		t.Fatalf("GET export failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET export status = %d, want 200", resp.StatusCode)
	}
	exportData, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if !json.Valid(exportData) {
		t.Error("exported data is not valid JSON")
	}
}

func TestAPI_IndexPage(t *testing.T) {
	ts, _ := setupTestServer(t)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET / failed: %v", err)
	}
	// Without templates, falls back to JSON
	if resp.StatusCode != 200 {
		t.Errorf("GET / status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAPI_Presets(t *testing.T) {
	ts, _ := setupTestServer(t)

	// List empty
	resp, err := http.Get(ts.URL + "/api/presets")
	if err != nil {
		t.Fatalf("GET /api/presets failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET /api/presets status = %d, want 200", resp.StatusCode)
	}
	var presets []model.FilterPreset
	json.NewDecoder(resp.Body).Decode(&presets)
	resp.Body.Close()
	if len(presets) != 0 {
		t.Errorf("expected empty presets, got %d", len(presets))
	}

	// Create
	body := `{"name":"My Filter","filter":"{\"cmdClassifier\":\"read\"}"}`
	resp, err = http.Post(ts.URL+"/api/presets", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/presets failed: %v", err)
	}
	if resp.StatusCode != 201 {
		t.Errorf("POST /api/presets status = %d, want 201", resp.StatusCode)
	}
	var created model.FilterPreset
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	if created.ID == 0 {
		t.Error("expected preset ID to be set")
	}

	// List again
	resp, _ = http.Get(ts.URL + "/api/presets")
	json.NewDecoder(resp.Body).Decode(&presets)
	resp.Body.Close()
	if len(presets) != 1 {
		t.Errorf("expected 1 preset, got %d", len(presets))
	}

	// Delete
	req, _ := http.NewRequest("DELETE", ts.URL+"/api/presets/1", http.NoBody)
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != 204 {
		t.Errorf("DELETE /api/presets/1 status = %d, want 204", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAPI_Bookmarks(t *testing.T) {
	ts, db := setupTestServer(t)

	// Create trace and message
	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	msg := &model.Message{
		TraceID:     trace.ID,
		SequenceNum: 1,
		Timestamp:   time.Now(),
		ShipMsgType: model.ShipMsgTypeData,
	}
	msgRepo.InsertMessage(msg)

	// List empty
	resp, err := http.Get(ts.URL + "/api/traces/1/bookmarks")
	if err != nil {
		t.Fatalf("GET bookmarks failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET bookmarks status = %d, want 200", resp.StatusCode)
	}
	var bookmarks []model.Bookmark
	json.NewDecoder(resp.Body).Decode(&bookmarks)
	resp.Body.Close()
	if len(bookmarks) != 0 {
		t.Errorf("expected empty bookmarks, got %d", len(bookmarks))
	}

	// Create
	body := `{"messageId":1,"label":"Important","color":"#ff0000","note":"test note"}`
	resp, err = http.Post(ts.URL+"/api/traces/1/bookmarks", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST bookmarks failed: %v", err)
	}
	if resp.StatusCode != 201 {
		t.Errorf("POST bookmarks status = %d, want 201", resp.StatusCode)
	}
	resp.Body.Close()

	// List again
	resp, _ = http.Get(ts.URL + "/api/traces/1/bookmarks")
	json.NewDecoder(resp.Body).Decode(&bookmarks)
	resp.Body.Close()
	if len(bookmarks) != 1 {
		t.Errorf("expected 1 bookmark, got %d", len(bookmarks))
	}

	// Delete
	req, _ := http.NewRequest("DELETE", ts.URL+"/api/bookmarks/1", http.NoBody)
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != 204 {
		t.Errorf("DELETE bookmark status = %d, want 204", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAPI_Devices(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	deviceRepo := store.NewDeviceRepo(db)
	d := &model.Device{
		TraceID:    trace.ID,
		DeviceAddr: "d:_i:19667_HEMS",
		FirstSeenAt: time.Now(),
		LastSeenAt:  time.Now(),
	}
	deviceRepo.UpsertDevice(d)

	resp, err := http.Get(ts.URL + "/api/traces/1/devices")
	if err != nil {
		t.Fatalf("GET devices failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET devices status = %d, want 200", resp.StatusCode)
	}
	var devices []DeviceWithDiscovery
	json.NewDecoder(resp.Body).Decode(&devices)
	resp.Body.Close()
	if len(devices) != 1 {
		t.Errorf("expected 1 device, got %d", len(devices))
	}
}

func TestAPI_Connections(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	now := time.Now()
	msgs := []*model.Message{
		{TraceID: trace.ID, SequenceNum: 1, Timestamp: now, ShipMsgType: "init", SourceAddr: "192.168.1.1", DestAddr: "192.168.1.2"},
		{TraceID: trace.ID, SequenceNum: 2, Timestamp: now.Add(time.Second), ShipMsgType: "connectionHello", SourceAddr: "192.168.1.1", DestAddr: "192.168.1.2"},
		{TraceID: trace.ID, SequenceNum: 3, Timestamp: now.Add(2 * time.Second), ShipMsgType: "data", SourceAddr: "192.168.1.1", DestAddr: "192.168.1.2"},
	}
	msgRepo.InsertMessages(msgs)

	resp, err := http.Get(ts.URL + "/api/traces/1/connections")
	if err != nil {
		t.Fatalf("GET connections failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET connections status = %d, want 200", resp.StatusCode)
	}
	var connections []ConnectionState
	json.NewDecoder(resp.Body).Decode(&connections)
	resp.Body.Close()
	if len(connections) != 1 {
		t.Errorf("expected 1 connection, got %d", len(connections))
	}
	if len(connections) > 0 && connections[0].CurrentState != "data" {
		t.Errorf("current state = %q, want %q", connections[0].CurrentState, "data")
	}
}

func TestAPI_Connections_Anomalies(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	now := time.Now()
	// Init then close without hello or data = anomalies
	msgs := []*model.Message{
		{TraceID: trace.ID, SequenceNum: 1, Timestamp: now, ShipMsgType: "init", SourceAddr: "10.0.0.1", DestAddr: "10.0.0.2"},
		{TraceID: trace.ID, SequenceNum: 2, Timestamp: now.Add(time.Second), ShipMsgType: "connectionClose", SourceAddr: "10.0.0.1", DestAddr: "10.0.0.2"},
	}
	msgRepo.InsertMessages(msgs)

	resp, _ := http.Get(ts.URL + "/api/traces/1/connections")
	var connections []ConnectionState
	json.NewDecoder(resp.Body).Decode(&connections)
	resp.Body.Close()

	if len(connections) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(connections))
	}
	if len(connections[0].Anomalies) == 0 {
		t.Error("expected anomalies to be detected")
	}
}

func TestAPI_Correlation(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	now := time.Now()
	msgs := []*model.Message{
		{TraceID: trace.ID, SequenceNum: 1, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "read", MsgCounter: "42", DeviceSource: "devA"},
		{TraceID: trace.ID, SequenceNum: 2, Timestamp: now.Add(time.Second), ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply", MsgCounter: "43", MsgCounterRef: "42", DeviceSource: "devB"},
	}
	msgRepo.InsertMessages(msgs)

	// Get related for the request (msgCounter=42)
	resp, err := http.Get(ts.URL + "/api/traces/1/messages/1/related")
	if err != nil {
		t.Fatalf("GET related failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET related status = %d, want 200", resp.StatusCode)
	}
	var related []RelatedMessage
	json.NewDecoder(resp.Body).Decode(&related)
	resp.Body.Close()
	if len(related) != 1 {
		t.Errorf("expected 1 related message, got %d", len(related))
	}
	if len(related) > 0 && related[0].Relationship != "read-reply" {
		t.Errorf("relationship = %q, want %q", related[0].Relationship, "read-reply")
	}
	if len(related) > 0 && related[0].LatencyMs == nil {
		t.Error("expected latencyMs to be set")
	}
	if len(related) > 0 && related[0].LatencyMs != nil && *related[0].LatencyMs < 0 {
		t.Errorf("latencyMs = %f, want >= 0", *related[0].LatencyMs)
	}

	// Get related for the response (msgCounterRef=42)
	resp, _ = http.Get(ts.URL + "/api/traces/1/messages/2/related")
	json.NewDecoder(resp.Body).Decode(&related)
	resp.Body.Close()
	if len(related) != 1 {
		t.Errorf("expected 1 related (reverse), got %d", len(related))
	}
}

func TestAPI_VizPages_NotFound(t *testing.T) {
	ts, _ := setupTestServer(t)

	pages := []string{
		"/traces/999/charts",
		"/traces/999/intelligence",
	}

	for _, page := range pages {
		resp, err := http.Get(ts.URL + page)
		if err != nil {
			t.Fatalf("GET %s failed: %v", page, err)
		}
		if resp.StatusCode != 404 {
			t.Errorf("GET %s status = %d, want 404", page, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

func TestAPI_VizPages_OK(t *testing.T) {
	ts, db := setupTestServer(t)

	// Without templates, the handlers fall back to JSON
	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	pages := []string{
		"/traces/1/charts",
		"/traces/1/intelligence",
	}

	for _, page := range pages {
		resp, err := http.Get(ts.URL + page)
		if err != nil {
			t.Fatalf("GET %s failed: %v", page, err)
		}
		if resp.StatusCode != 200 {
			t.Errorf("GET %s status = %d, want 200", page, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

func TestAPI_VizPages_InvalidID(t *testing.T) {
	ts, _ := setupTestServer(t)

	pages := []string{
		"/traces/abc/charts",
		"/traces/abc/intelligence",
	}

	for _, page := range pages {
		resp, err := http.Get(ts.URL + page)
		if err != nil {
			t.Fatalf("GET %s failed: %v", page, err)
		}
		if resp.StatusCode != 400 {
			t.Errorf("GET %s status = %d, want 400", page, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

func TestAPI_UseCases(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	// Use case message with SPINE payload
	payload := `{"datagram":{"payload":{"cmd":[{"nodeManagementUseCaseData":{"useCaseInformation":[{"actor":"CEM","useCaseSupport":[{"useCaseName":"limitationOfPowerConsumption","useCaseAvailable":true}]}]}}]}}}`
	msg := &model.Message{
		TraceID:       trace.ID,
		SequenceNum:   1,
		Timestamp:     time.Now(),
		ShipMsgType:   model.ShipMsgTypeData,
		FunctionSet:   "NodeManagementUseCaseData",
		CmdClassifier: "reply",
		DeviceSource:  "d:_i:HEMS",
		SpinePayload:  []byte(payload),
	}
	msgRepo.InsertMessage(msg)

	resp, err := http.Get(ts.URL + "/api/traces/1/usecases")
	if err != nil {
		t.Fatalf("GET usecases failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET usecases status = %d, want 200", resp.StatusCode)
	}
	var result []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()
	if len(result) != 1 {
		t.Errorf("expected 1 device use case, got %d", len(result))
	}
}

func TestAPI_Subscriptions(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	payload := `{"datagram":{"payload":{"cmd":[{"nodeManagementSubscriptionData":{"subscriptionEntry":[{"subscriptionId":"1","clientAddress":{"device":"devA","entity":[1],"feature":1},"serverAddress":{"device":"devB","entity":[1],"feature":2}}]}}]}}}`
	msg := &model.Message{
		TraceID:       trace.ID,
		SequenceNum:   1,
		Timestamp:     time.Now(),
		ShipMsgType:   model.ShipMsgTypeData,
		FunctionSet:   "NodeManagementSubscriptionData",
		CmdClassifier: "reply",
		SpinePayload:  []byte(payload),
	}
	msgRepo.InsertMessage(msg)

	resp, err := http.Get(ts.URL + "/api/traces/1/subscriptions")
	if err != nil {
		t.Fatalf("GET subscriptions failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET subscriptions status = %d, want 200", resp.StatusCode)
	}
	var result []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()
	if len(result) != 1 {
		t.Errorf("expected 1 subscription, got %d", len(result))
	}
}

func TestAPI_Bindings(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	// Empty trace should return empty array, not null
	resp, err := http.Get(ts.URL + "/api/traces/1/bindings")
	if err != nil {
		t.Fatalf("GET bindings failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET bindings status = %d, want 200", resp.StatusCode)
	}
	var result []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestAPI_Metrics(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	now := time.Now()
	msgs := []*model.Message{
		{TraceID: trace.ID, SequenceNum: 1, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, FunctionSet: "DeviceDiagnosisHeartbeatData", DeviceSource: "A", DeviceDest: "B"},
		{TraceID: trace.ID, SequenceNum: 2, Timestamp: now.Add(10 * time.Second), ShipMsgType: model.ShipMsgTypeData, FunctionSet: "DeviceDiagnosisHeartbeatData", DeviceSource: "A", DeviceDest: "B"},
	}
	msgRepo.InsertMessages(msgs)

	resp, err := http.Get(ts.URL + "/api/traces/1/metrics")
	if err != nil {
		t.Fatalf("GET metrics failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET metrics status = %d, want 200", resp.StatusCode)
	}
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()
	if _, ok := result["heartbeatJitter"]; !ok {
		t.Error("expected heartbeatJitter in response")
	}
}

func TestAPI_IntelligencePage_NotFound(t *testing.T) {
	ts, _ := setupTestServer(t)
	resp, err := http.Get(ts.URL + "/traces/999/intelligence")
	if err != nil {
		t.Fatalf("GET intelligence failed: %v", err)
	}
	if resp.StatusCode != 404 {
		t.Errorf("GET intelligence status = %d, want 404", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAPI_IntelligencePage_OK(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	resp, err := http.Get(ts.URL + "/traces/1/intelligence")
	if err != nil {
		t.Fatalf("GET intelligence failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET intelligence status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAPI_AboutPage(t *testing.T) {
	ts, _ := setupTestServer(t)

	resp, err := http.Get(ts.URL + "/about")
	if err != nil {
		t.Fatalf("GET /about failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET /about status = %d, want 200", resp.StatusCode)
	}

	// Without templates, falls back to JSON — verify key fields
	var data map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&data)
	resp.Body.Close()

	// Project section
	project, ok := data["Project"].(map[string]interface{})
	if !ok {
		t.Fatal("expected Project in response")
	}
	if project["Name"] != "EEBus Tracer" {
		t.Errorf("Project.Name = %q, want %q", project["Name"], "EEBus Tracer")
	}
	if project["Version"] != "test" {
		t.Errorf("Project.Version = %q, want %q", project["Version"], "test")
	}

	// System section
	system, ok := data["System"].(map[string]interface{})
	if !ok {
		t.Fatal("expected System in response")
	}
	if system["GoVersion"] == nil || system["GoVersion"] == "" {
		t.Error("expected System.GoVersion to be set")
	}
	if system["OS"] == nil || system["OS"] == "" {
		t.Error("expected System.OS to be set")
	}
	if system["Arch"] == nil || system["Arch"] == "" {
		t.Error("expected System.Arch to be set")
	}

	// Dependencies section
	deps, ok := data["Dependencies"]
	if !ok {
		t.Fatal("expected Dependencies in response")
	}
	// Should be a non-nil slice (may be empty in test env)
	if deps == nil {
		t.Error("expected Dependencies to be non-nil")
	}
}

func TestAPI_MessageSummaries(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	now := time.Now()
	msgs := []*model.Message{
		{TraceID: trace.ID, SequenceNum: 1, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "read", FunctionSet: "MeasurementListData", DeviceSource: "devA", DeviceDest: "devB"},
		{TraceID: trace.ID, SequenceNum: 2, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply", FunctionSet: "MeasurementListData", DeviceSource: "devB", DeviceDest: "devA"},
		{TraceID: trace.ID, SequenceNum: 3, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "read", FunctionSet: "LoadControlLimitListData", DeviceSource: "devA", DeviceDest: "devB"},
	}
	msgRepo.InsertMessages(msgs)

	t.Run("returns all summaries", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/api/traces/" + strconv.FormatInt(trace.ID, 10) + "/messages/summaries")
		if err != nil {
			t.Fatalf("GET summaries failed: %v", err)
		}
		if resp.StatusCode != 200 {
			t.Errorf("status = %d, want 200", resp.StatusCode)
		}
		var summaries []model.MessageSummary
		json.NewDecoder(resp.Body).Decode(&summaries)
		resp.Body.Close()
		if len(summaries) != 3 {
			t.Errorf("got %d summaries, want 3", len(summaries))
		}
		// Verify summary fields
		if len(summaries) > 0 {
			s := summaries[0]
			if s.SequenceNum != 1 {
				t.Errorf("SequenceNum = %d, want 1", s.SequenceNum)
			}
			if s.DeviceSource != "devA" {
				t.Errorf("DeviceSource = %q, want %q", s.DeviceSource, "devA")
			}
		}
	})

	t.Run("filter by cmdClassifier", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/api/traces/" + strconv.FormatInt(trace.ID, 10) + "/messages/summaries?cmdClassifier=read")
		if err != nil {
			t.Fatalf("GET summaries failed: %v", err)
		}
		var summaries []model.MessageSummary
		json.NewDecoder(resp.Body).Decode(&summaries)
		resp.Body.Close()
		if len(summaries) != 2 {
			t.Errorf("got %d summaries, want 2", len(summaries))
		}
	})

	t.Run("empty trace returns empty array", func(t *testing.T) {
		trace2 := &model.Trace{Name: "empty", StartedAt: now, CreatedAt: now}
		traceRepo.CreateTrace(trace2)
		resp, err := http.Get(ts.URL + "/api/traces/" + strconv.FormatInt(trace2.ID, 10) + "/messages/summaries")
		if err != nil {
			t.Fatalf("GET summaries failed: %v", err)
		}
		var summaries []model.MessageSummary
		json.NewDecoder(resp.Body).Decode(&summaries)
		resp.Body.Close()
		if summaries == nil || len(summaries) != 0 {
			t.Errorf("expected empty array, got %v", summaries)
		}
	})
}

func TestAPI_MessagesSearch(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	now := time.Now()
	msgs := []*model.Message{
		{TraceID: trace.ID, SequenceNum: 1, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, SpinePayload: []byte(`{"MeasurementListData":"values"}`)},
		{TraceID: trace.ID, SequenceNum: 2, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, SpinePayload: []byte(`{"DeviceClassification":"info"}`)},
	}
	msgRepo.InsertMessages(msgs)

	resp, err := http.Get(ts.URL + "/api/traces/1/messages?search=MeasurementListData")
	if err != nil {
		t.Fatalf("GET search failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("search status = %d, want 200", resp.StatusCode)
	}
	var results []model.Message
	json.NewDecoder(resp.Body).Decode(&results)
	resp.Body.Close()
	if len(results) != 1 {
		t.Errorf("search results = %d, want 1", len(results))
	}
}

func TestAPI_Conversation(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	now := time.Now()
	msgs := []*model.Message{
		{TraceID: trace.ID, SequenceNum: 1, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "read", FunctionSet: "MeasurementListData", DeviceSource: "devA", DeviceDest: "devB", MsgCounter: "1"},
		{TraceID: trace.ID, SequenceNum: 2, Timestamp: now.Add(time.Second), ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply", FunctionSet: "MeasurementListData", DeviceSource: "devB", DeviceDest: "devA", MsgCounter: "2", MsgCounterRef: "1"},
		{TraceID: trace.ID, SequenceNum: 3, Timestamp: now.Add(2 * time.Second), ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "notify", FunctionSet: "MeasurementListData", DeviceSource: "devB", DeviceDest: "devA", MsgCounter: "3"},
		// Different function set
		{TraceID: trace.ID, SequenceNum: 4, Timestamp: now.Add(3 * time.Second), ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "read", FunctionSet: "LoadControlLimitListData", DeviceSource: "devA", DeviceDest: "devB", MsgCounter: "4"},
	}
	msgRepo.InsertMessages(msgs)

	t.Run("conversation with 3 messages", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/api/traces/1/messages/1/conversation")
		if err != nil {
			t.Fatalf("GET conversation failed: %v", err)
		}
		if resp.StatusCode != 200 {
			t.Errorf("status = %d, want 200", resp.StatusCode)
		}
		var conv ConversationResponse
		json.NewDecoder(resp.Body).Decode(&conv)
		resp.Body.Close()
		if conv.Total != 3 {
			t.Errorf("total = %d, want 3", conv.Total)
		}
		if len(conv.Messages) != 3 {
			t.Errorf("messages len = %d, want 3", len(conv.Messages))
		}
	})

	t.Run("different function set excluded", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/api/traces/1/messages/4/conversation")
		if err != nil {
			t.Fatalf("GET conversation failed: %v", err)
		}
		var conv ConversationResponse
		json.NewDecoder(resp.Body).Decode(&conv)
		resp.Body.Close()
		if conv.Total != 1 {
			t.Errorf("total = %d, want 1", conv.Total)
		}
	})

	t.Run("non-SPINE message returns empty", func(t *testing.T) {
		// Insert a non-SPINE message
		shipMsg := &model.Message{
			TraceID: trace.ID, SequenceNum: 10, Timestamp: now,
			ShipMsgType: "init", DeviceSource: "devA", DeviceDest: "devB",
		}
		msgRepo.InsertMessage(shipMsg)

		resp, err := http.Get(ts.URL + "/api/traces/1/messages/" + strconv.FormatInt(shipMsg.ID, 10) + "/conversation")
		if err != nil {
			t.Fatalf("GET conversation failed: %v", err)
		}
		var conv ConversationResponse
		json.NewDecoder(resp.Body).Decode(&conv)
		resp.Body.Close()
		if conv.Total != 0 {
			t.Errorf("total = %d, want 0", conv.Total)
		}
	})
}

func TestAPI_CorrelationResultStatus(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	now := time.Now()

	// Request with accepted result
	acceptedPayload := `{"datagram":{"payload":{"cmd":[{"resultData":{"errorNumber":0}}]}}}`
	msgs := []*model.Message{
		{TraceID: trace.ID, SequenceNum: 1, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "read", MsgCounter: "10"},
		{TraceID: trace.ID, SequenceNum: 2, Timestamp: now.Add(50 * time.Millisecond), ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply", MsgCounter: "11", MsgCounterRef: "10", SpinePayload: []byte(acceptedPayload)},
	}
	msgRepo.InsertMessages(msgs)

	resp, err := http.Get(ts.URL + "/api/traces/1/messages/1/related")
	if err != nil {
		t.Fatalf("GET related failed: %v", err)
	}
	var related []RelatedMessage
	json.NewDecoder(resp.Body).Decode(&related)
	resp.Body.Close()

	if len(related) != 1 {
		t.Fatalf("expected 1 related, got %d", len(related))
	}
	if related[0].ResultStatus != "accepted" {
		t.Errorf("resultStatus = %q, want %q", related[0].ResultStatus, "accepted")
	}

	// Request with rejected result
	rejectedPayload := `{"datagram":{"payload":{"cmd":[{"resultData":{"errorNumber":1}}]}}}`
	msgs2 := []*model.Message{
		{TraceID: trace.ID, SequenceNum: 3, Timestamp: now.Add(time.Second), ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "write", MsgCounter: "20"},
		{TraceID: trace.ID, SequenceNum: 4, Timestamp: now.Add(2 * time.Second), ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply", MsgCounter: "21", MsgCounterRef: "20", SpinePayload: []byte(rejectedPayload)},
	}
	msgRepo.InsertMessages(msgs2)

	resp, err = http.Get(ts.URL + "/api/traces/1/messages/3/related")
	if err != nil {
		t.Fatalf("GET related failed: %v", err)
	}
	json.NewDecoder(resp.Body).Decode(&related)
	resp.Body.Close()

	if len(related) != 1 {
		t.Fatalf("expected 1 related, got %d", len(related))
	}
	if related[0].ResultStatus != "rejected" {
		t.Errorf("resultStatus = %q, want %q", related[0].ResultStatus, "rejected")
	}
	if related[0].Relationship != "write-result" {
		t.Errorf("relationship = %q, want %q", related[0].Relationship, "write-result")
	}
}

func TestAPI_OrphanedRequests(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	now := time.Now()
	msgs := []*model.Message{
		{TraceID: trace.ID, SequenceNum: 1, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "read", MsgCounter: "10"},
		{TraceID: trace.ID, SequenceNum: 2, Timestamp: now.Add(time.Second), ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply", MsgCounter: "11", MsgCounterRef: "10"},
		// msg3: write with no response → orphaned
		{TraceID: trace.ID, SequenceNum: 3, Timestamp: now.Add(2 * time.Second), ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "write", MsgCounter: "20"},
		// msg4: notify with no response → NOT orphaned (notify never expects one)
		{TraceID: trace.ID, SequenceNum: 4, Timestamp: now.Add(3 * time.Second), ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "notify", MsgCounter: "30"},
	}
	msgRepo.InsertMessages(msgs)

	resp, err := http.Get(ts.URL + "/api/traces/" + strconv.FormatInt(trace.ID, 10) + "/orphaned-requests")
	if err != nil {
		t.Fatalf("GET orphaned-requests failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET orphaned-requests status = %d, want 200", resp.StatusCode)
	}
	var result OrphanedRequestsResponse
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	// Only msg3 (write, counter=20) is orphaned; reply and notify are excluded
	if len(result.IDs) != 1 {
		t.Errorf("expected 1 orphaned ID, got %d: %v", len(result.IDs), result.IDs)
	}
}

func TestAPI_OrphanedRequests_Empty(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	now := time.Now()
	msgs := []*model.Message{
		{TraceID: trace.ID, SequenceNum: 1, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "read", MsgCounter: "10"},
		{TraceID: trace.ID, SequenceNum: 2, Timestamp: now.Add(time.Second), ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply", MsgCounter: "11", MsgCounterRef: "10"},
	}
	msgRepo.InsertMessages(msgs)

	resp, err := http.Get(ts.URL + "/api/traces/" + strconv.FormatInt(trace.ID, 10) + "/orphaned-requests")
	if err != nil {
		t.Fatalf("GET orphaned-requests failed: %v", err)
	}
	var result OrphanedRequestsResponse
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	// read(counter=10) is answered by reply(ref=10); reply is excluded → 0 orphans
	if len(result.IDs) != 0 {
		t.Errorf("expected 0 orphaned IDs, got %d: %v", len(result.IDs), result.IDs)
	}
}

func TestAPI_MessagesTimeRangeFilter(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	base := time.Date(2026, 3, 20, 10, 0, 0, 0, time.UTC)
	msgs := []*model.Message{
		{TraceID: trace.ID, SequenceNum: 1, Timestamp: base, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "read"},
		{TraceID: trace.ID, SequenceNum: 2, Timestamp: base.Add(2 * time.Hour), ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "read"},
		{TraceID: trace.ID, SequenceNum: 3, Timestamp: base.Add(4 * time.Hour), ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "read"},
		{TraceID: trace.ID, SequenceNum: 4, Timestamp: base.Add(6 * time.Hour), ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "read"},
	}
	msgRepo.InsertMessages(msgs)

	tests := []struct {
		name      string
		query     string
		wantCount int
	}{
		{
			name:      "both timeFrom and timeTo",
			query:     "timeFrom=" + base.Add(1*time.Hour).Format(time.RFC3339) + "&timeTo=" + base.Add(5*time.Hour).Format(time.RFC3339),
			wantCount: 2, // seq 2 (12:00) and seq 3 (14:00)
		},
		{
			name:      "only timeFrom",
			query:     "timeFrom=" + base.Add(3*time.Hour).Format(time.RFC3339),
			wantCount: 2, // seq 3 (14:00) and seq 4 (16:00)
		},
		{
			name:      "only timeTo",
			query:     "timeTo=" + base.Add(3*time.Hour).Format(time.RFC3339),
			wantCount: 2, // seq 1 (10:00) and seq 2 (12:00)
		},
		{
			name:      "no time filter returns all",
			query:     "",
			wantCount: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := ts.URL + "/api/traces/" + strconv.FormatInt(trace.ID, 10) + "/messages"
			if tt.query != "" {
				url += "?" + tt.query
			}
			resp, err := http.Get(url)
			if err != nil {
				t.Fatalf("GET messages failed: %v", err)
			}
			if resp.StatusCode != 200 {
				t.Errorf("status = %d, want 200", resp.StatusCode)
			}
			var results []model.Message
			json.NewDecoder(resp.Body).Decode(&results)
			resp.Body.Close()
			if len(results) != tt.wantCount {
				t.Errorf("got %d messages, want %d", len(results), tt.wantCount)
			}
		})
	}
}

func TestAPI_UseCaseContext(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	now := time.Now()

	useCasePayload := `{"datagram":{"payload":{"cmd":[{"nodeManagementUseCaseData":{"useCaseInformation":[{"actor":"CEM","useCaseSupport":[{"useCaseName":"limitationOfPowerConsumption","useCaseAvailable":true}]}]}}]}}}`
	msgs := []*model.Message{
		{TraceID: trace.ID, SequenceNum: 1, Timestamp: now, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply", FunctionSet: "NodeManagementUseCaseData", DeviceSource: "deviceA", SpinePayload: []byte(useCasePayload)},
	}
	msgRepo.InsertMessages(msgs)

	resp, err := http.Get(ts.URL + "/api/traces/" + strconv.FormatInt(trace.ID, 10) + "/usecase-context")
	if err != nil {
		t.Fatalf("GET usecase-context failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET usecase-context status = %d, want 200", resp.StatusCode)
	}
	var result []UseCaseContext
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	if len(result) != 1 {
		t.Fatalf("expected 1 use case context, got %d", len(result))
	}
	if result[0].Abbreviation != "LPC" {
		t.Errorf("abbreviation = %q, want %q", result[0].Abbreviation, "LPC")
	}
	if len(result[0].FunctionSets) == 0 {
		t.Error("expected non-empty function sets")
	}
	if len(result[0].Devices) != 1 || result[0].Devices[0] != "deviceA" {
		t.Errorf("devices = %v, want [deviceA]", result[0].Devices)
	}
}

func TestAPI_UseCaseContext_Empty(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	resp, err := http.Get(ts.URL + "/api/traces/" + strconv.FormatInt(trace.ID, 10) + "/usecase-context")
	if err != nil {
		t.Fatalf("GET usecase-context failed: %v", err)
	}
	var result []UseCaseContext
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	if len(result) != 0 {
		t.Errorf("expected 0 use case contexts for empty trace, got %d", len(result))
	}
}

func TestAPI_DependencyGraph_Empty(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	resp, err := http.Get(ts.URL + "/api/traces/1/depgraph")
	if err != nil {
		t.Fatalf("GET depgraph failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET depgraph status = %d, want 200", resp.StatusCode)
	}

	var tree struct {
		Devices []map[string]interface{} `json:"devices"`
		Edges   []map[string]interface{} `json:"edges"`
	}
	json.NewDecoder(resp.Body).Decode(&tree)
	resp.Body.Close()

	if tree.Devices == nil {
		t.Error("expected non-nil devices")
	}
	if tree.Edges == nil {
		t.Error("expected non-nil edges")
	}
	if len(tree.Devices) != 0 {
		t.Errorf("expected 0 devices, got %d", len(tree.Devices))
	}
	if len(tree.Edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(tree.Edges))
	}
}

func TestAPI_DependencyGraph_DiscoveryNotifyUpdatesTree(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	now := time.Now()

	// Initial discovery reply: EVSE has only one entity [1] with one feature
	initialDiscovery := `{"datagram":{"payload":{"cmd":[{"nodeManagementDetailedDiscoveryData":{` +
		`"entityInformation":[{"description":{"entityAddress":{"entity":[1]},"entityType":"EVSE"}}],` +
		`"featureInformation":[{"description":{"featureAddress":{"entity":[1],"feature":1},"featureType":"LoadControlLimit","role":"server","supportedFunction":[{"function":"LoadControlLimitListData"}]}}]` +
		`}}]}}}`
	msg1 := &model.Message{
		TraceID:       trace.ID,
		SequenceNum:   1,
		Timestamp:     now,
		ShipMsgType:   model.ShipMsgTypeData,
		FunctionSet:   "NodeManagementDetailedDiscoveryData",
		CmdClassifier: "reply",
		DeviceSource:  "d:_i:EVSE",
		SpinePayload:  []byte(initialDiscovery),
	}

	// Later discovery notify: EV connected — partial update with ONLY the new EV entity
	updatedDiscovery := `{"datagram":{"payload":{"cmd":[{"nodeManagementDetailedDiscoveryData":{` +
		`"entityInformation":[` +
		`{"description":{"entityAddress":{"entity":[1,1]},"entityType":"EV"}}` +
		`],` +
		`"featureInformation":[` +
		`{"description":{"featureAddress":{"entity":[1,1],"feature":1},"featureType":"Measurement","role":"server","supportedFunction":[{"function":"MeasurementListData"}]}}` +
		`]` +
		`}}]}}}`
	msg2 := &model.Message{
		TraceID:       trace.ID,
		SequenceNum:   2,
		Timestamp:     now.Add(time.Second),
		ShipMsgType:   model.ShipMsgTypeData,
		FunctionSet:   "NodeManagementDetailedDiscoveryData",
		CmdClassifier: "notify",
		DeviceSource:  "d:_i:EVSE",
		SpinePayload:  []byte(updatedDiscovery),
	}
	msgRepo.InsertMessages([]*model.Message{msg1, msg2})

	resp, err := http.Get(ts.URL + "/api/traces/" + strconv.FormatInt(trace.ID, 10) + "/depgraph")
	if err != nil {
		t.Fatalf("GET depgraph failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("GET depgraph status = %d, want 200", resp.StatusCode)
	}

	var tree struct {
		Devices []struct {
			DeviceAddr string `json:"deviceAddr"`
			ShortName  string `json:"shortName"`
			Entities   []struct {
				Address    string `json:"address"`
				EntityType string `json:"entityType"`
			} `json:"entities"`
		} `json:"devices"`
	}
	json.NewDecoder(resp.Body).Decode(&tree)
	resp.Body.Close()

	if len(tree.Devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(tree.Devices))
	}

	// The partial notify should have been merged with the initial reply — we should see 2 entities (EVSE + EV)
	dev := tree.Devices[0]
	if len(dev.Entities) != 2 {
		t.Errorf("expected 2 entities (EVSE + EV after notify), got %d", len(dev.Entities))
	}

	entityTypes := map[string]bool{}
	for _, e := range dev.Entities {
		entityTypes[e.EntityType] = true
	}
	if !entityTypes["EVSE"] {
		t.Error("expected EVSE entity")
	}
	if !entityTypes["EV"] {
		t.Error("expected EV entity from discovery notify update")
	}
}

func TestAPI_DependencyGraph_WithData(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	// Create a device
	deviceRepo := store.NewDeviceRepo(db)
	d := &model.Device{
		TraceID:     trace.ID,
		DeviceAddr:  "d:_i:19667_HEMS",
		FirstSeenAt: time.Now(),
		LastSeenAt:  time.Now(),
	}
	deviceRepo.UpsertDevice(d)

	// Insert use case message
	msgRepo := store.NewMessageRepo(db)
	ucPayload := `{"datagram":{"payload":{"cmd":[{"nodeManagementUseCaseData":{"useCaseInformation":[{"actor":"CEM","useCaseSupport":[{"useCaseName":"limitationOfPowerConsumption","useCaseAvailable":true}]}]}}]}}}`
	ucMsg := &model.Message{
		TraceID:       trace.ID,
		SequenceNum:   1,
		Timestamp:     time.Now(),
		ShipMsgType:   model.ShipMsgTypeData,
		FunctionSet:   "NodeManagementUseCaseData",
		CmdClassifier: "reply",
		DeviceSource:  "d:_i:19667_HEMS",
		SpinePayload:  []byte(ucPayload),
	}
	msgRepo.InsertMessage(ucMsg)

	resp, err := http.Get(ts.URL + "/api/traces/1/depgraph")
	if err != nil {
		t.Fatalf("GET depgraph failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET depgraph status = %d, want 200", resp.StatusCode)
	}

	var tree struct {
		Devices []map[string]interface{} `json:"devices"`
		Edges   []map[string]interface{} `json:"edges"`
	}
	json.NewDecoder(resp.Body).Decode(&tree)
	resp.Body.Close()

	// Should have at least the HEMS device
	if len(tree.Devices) < 1 {
		t.Errorf("expected at least 1 device, got %d", len(tree.Devices))
	}

	// Verify we have the HEMS device with the correct short name
	hasHEMS := false
	for _, d := range tree.Devices {
		if d["shortName"] == "HEMS" {
			hasHEMS = true
			break
		}
	}
	if !hasHEMS {
		t.Error("expected device with shortName HEMS")
	}
}
