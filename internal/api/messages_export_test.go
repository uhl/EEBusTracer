package api

import (
	"encoding/csv"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
	"github.com/eebustracer/eebustracer/internal/store"
)

func TestAPI_ExportMessages_CSV(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "export", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	for i := 0; i < 3; i++ {
		msg := &model.Message{
			TraceID:       trace.ID,
			SequenceNum:   i + 1,
			Timestamp:     time.Unix(1700000000, 0).UTC(),
			ShipMsgType:   model.ShipMsgTypeData,
			CmdClassifier: "read",
			DeviceSource:  "devA",
			DeviceDest:    "devB",
		}
		msgRepo.InsertMessage(msg)
	}

	resp, err := http.Get(ts.URL + "/api/traces/1/messages/export")
	if err != nil {
		t.Fatalf("GET export failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("Content-Type = %q, want text/csv...", ct)
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "attachment") || !strings.Contains(cd, ".csv") {
		t.Errorf("Content-Disposition = %q, want attachment with .csv", cd)
	}

	rows, err := csv.NewReader(resp.Body).ReadAll()
	if err != nil {
		t.Fatalf("csv parse: %v", err)
	}
	if len(rows) != 4 { // header + 3 data rows
		t.Fatalf("row count = %d, want 4", len(rows))
	}
	if rows[0][0] != "id" || rows[0][3] != "direction" || rows[0][4] != "shipMsgType" {
		t.Errorf("unexpected header row: %v", rows[0])
	}
	if rows[1][5] != "read" || rows[1][9] != "devA" {
		t.Errorf("unexpected data row 1: %v", rows[1])
	}
}

func TestAPI_ExportMessages_JSON_WithFilter(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "export", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	classifiers := []string{"read", "reply", "read", "notify"}
	for i, cls := range classifiers {
		msg := &model.Message{
			TraceID:       trace.ID,
			SequenceNum:   i + 1,
			Timestamp:     time.Now(),
			ShipMsgType:   model.ShipMsgTypeData,
			CmdClassifier: cls,
		}
		msgRepo.InsertMessage(msg)
	}

	resp, err := http.Get(ts.URL + "/api/traces/1/messages/export?format=json&cmdClassifier=read")
	if err != nil {
		t.Fatalf("GET export failed: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var out []model.Message
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 read messages, got %d", len(out))
	}
	for _, m := range out {
		if m.CmdClassifier != "read" {
			t.Errorf("classifier = %q, want read", m.CmdClassifier)
		}
	}
}

func TestAPI_ExportMessages_BadFormat(t *testing.T) {
	ts, db := setupTestServer(t)
	traceRepo := store.NewTraceRepo(db)
	traceRepo.CreateTrace(&model.Trace{Name: "x", StartedAt: time.Now(), CreatedAt: time.Now()})

	resp, err := http.Get(ts.URL + "/api/traces/1/messages/export?format=xml")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}
