package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
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
	req, _ := http.NewRequest("DELETE", ts.URL+"/api/traces/1", nil)
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
	req, _ := http.NewRequest("DELETE", ts.URL+"/api/presets/1", nil)
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
	req, _ := http.NewRequest("DELETE", ts.URL+"/api/bookmarks/1", nil)
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
	if len(related) > 0 && related[0].Relationship != "request-response" {
		t.Errorf("relationship = %q, want %q", related[0].Relationship, "request-response")
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
