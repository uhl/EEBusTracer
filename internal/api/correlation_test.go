package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
	"github.com/eebustracer/eebustracer/internal/store"
)

func TestClassifyRelationship(t *testing.T) {
	tests := []struct {
		name     string
		reqCmd   string
		respCmd  string
		want     string
	}{
		{"read request", "read", "reply", "read-reply"},
		{"write request", "write", "result", "write-result"},
		{"call request", "call", "result", "call-result"},
		{"call with notify response", "call", "notify", "subscription-notify"},
		{"unknown classifier", "unknown", "reply", "request-response"},
		{"empty classifier", "", "reply", "request-response"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &model.Message{CmdClassifier: tt.reqCmd}
			resp := &model.Message{CmdClassifier: tt.respCmd}
			got := classifyRelationship(req, resp)
			if got != tt.want {
				t.Errorf("classifyRelationship() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestComputeLatencyMs(t *testing.T) {
	t1 := time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(150 * time.Millisecond)

	req := &model.Message{Timestamp: t1}
	resp := &model.Message{Timestamp: t2}

	got := computeLatencyMs(req, resp)
	if got == nil {
		t.Fatal("computeLatencyMs() returned nil")
	}
	if *got < 149.9 || *got > 150.1 {
		t.Errorf("latency = %f, want ~150.0", *got)
	}
}

func TestComputeLatencyMs_SameTimestamp(t *testing.T) {
	ts := time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC)
	req := &model.Message{Timestamp: ts}
	resp := &model.Message{Timestamp: ts}

	got := computeLatencyMs(req, resp)
	if got == nil {
		t.Fatal("computeLatencyMs() returned nil")
	}
	if *got != 0 {
		t.Errorf("latency = %f, want 0", *got)
	}
}

func TestExtractResultStatus(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    string
	}{
		{
			name:    "empty payload",
			payload: "",
			want:    "",
		},
		{
			name:    "no resultData",
			payload: `{"datagram":{"payload":{"cmd":[{"measurementListData":{}}]}}}`,
			want:    "",
		},
		{
			name:    "accepted (errorNumber 0)",
			payload: `{"datagram":{"payload":{"cmd":[{"resultData":{"errorNumber":0}}]}}}`,
			want:    "accepted",
		},
		{
			name:    "accepted (errorNumber absent)",
			payload: `{"datagram":{"payload":{"cmd":[{"resultData":{}}]}}}`,
			want:    "accepted",
		},
		{
			name:    "rejected (errorNumber non-zero)",
			payload: `{"datagram":{"payload":{"cmd":[{"resultData":{"errorNumber":7}}]}}}`,
			want:    "rejected",
		},
		{
			name:    "malformed JSON",
			payload: `{not valid json}`,
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var payload json.RawMessage
			if tt.payload != "" {
				payload = json.RawMessage(tt.payload)
			}
			got := extractResultStatus(payload)
			if got != tt.want {
				t.Errorf("extractResultStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHandleRelatedMessages_InvalidTraceID(t *testing.T) {
	ts, _ := setupTestServer(t)

	resp, err := http.Get(ts.URL + "/api/traces/abc/messages/1/related")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestHandleRelatedMessages_InvalidMessageID(t *testing.T) {
	ts, _ := setupTestServer(t)

	resp, err := http.Get(ts.URL + "/api/traces/1/messages/xyz/related")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestHandleRelatedMessages_MessageNotFound(t *testing.T) {
	ts, _ := setupTestServer(t)

	resp, err := http.Get(ts.URL + "/api/traces/999/messages/999/related")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestHandleRelatedMessages_EmptyResult(t *testing.T) {
	ts, db := setupTestServer(t)

	// Create trace and message via repos (respects FTS triggers)
	traceRepo := store.NewTraceRepo(db)
	msgRepo := store.NewMessageRepo(db)

	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	if err := traceRepo.CreateTrace(trace); err != nil {
		t.Fatalf("create trace: %v", err)
	}

	msg := &model.Message{
		TraceID:     trace.ID,
		SequenceNum: 1,
		Timestamp:   time.Now(),
		Direction:   model.DirectionIncoming,
		ShipMsgType: model.ShipMsgTypeData,
	}
	if err := msgRepo.InsertMessages([]*model.Message{msg}); err != nil {
		t.Fatalf("insert message: %v", err)
	}

	resp, err := http.Get(ts.URL + fmt.Sprintf("/api/traces/%d/messages/%d/related", trace.ID, msg.ID))
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var related []RelatedMessage
	if err := json.NewDecoder(resp.Body).Decode(&related); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(related) != 0 {
		t.Errorf("len(related) = %d, want 0", len(related))
	}
}

func TestHandleConversation_InvalidTraceID(t *testing.T) {
	ts, _ := setupTestServer(t)

	resp, err := http.Get(ts.URL + "/api/traces/abc/messages/1/conversation")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestHandleConversation_MessageNotFound(t *testing.T) {
	ts, _ := setupTestServer(t)

	resp, err := http.Get(ts.URL + "/api/traces/999/messages/999/conversation")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestHandleOrphanedRequests_InvalidTraceID(t *testing.T) {
	ts, _ := setupTestServer(t)

	resp, err := http.Get(ts.URL + "/api/traces/abc/orphaned-requests")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestHandleOrphanedRequests_EmptyResult(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	if err := traceRepo.CreateTrace(trace); err != nil {
		t.Fatalf("create trace: %v", err)
	}

	resp, err := http.Get(ts.URL + fmt.Sprintf("/api/traces/%d/orphaned-requests", trace.ID))
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var result OrphanedRequestsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result.IDs) != 0 {
		t.Errorf("len(IDs) = %d, want 0", len(result.IDs))
	}
}
