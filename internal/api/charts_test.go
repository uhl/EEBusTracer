package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
	"github.com/eebustracer/eebustracer/internal/store"
)

func TestAPI_Charts_ListBuiltIns(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	resp, err := http.Get(ts.URL + "/api/traces/1/charts")
	if err != nil {
		t.Fatalf("GET charts failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET charts status = %d, want 200", resp.StatusCode)
	}

	var charts []model.ChartDefinition
	json.NewDecoder(resp.Body).Decode(&charts)
	resp.Body.Close()

	if len(charts) != 3 {
		t.Fatalf("expected 3 built-in charts, got %d", len(charts))
	}
	// Built-in charts should come first
	for _, c := range charts {
		if !c.IsBuiltIn {
			t.Errorf("expected built-in chart, got %+v", c)
		}
	}
}

func TestAPI_Charts_CRUD(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	// Create
	body := `{"name":"My Chart","chartType":"step","sources":"[{\"functionSet\":\"MeasurementListData\",\"cmdKey\":\"measurementListData\",\"dataArrayKey\":\"measurementData\",\"idField\":\"measurementId\",\"classifiers\":[\"reply\"]}]"}`
	resp, err := http.Post(ts.URL+"/api/traces/1/charts", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST chart failed: %v", err)
	}
	if resp.StatusCode != 201 {
		t.Errorf("POST chart status = %d, want 201", resp.StatusCode)
	}
	var created model.ChartDefinition
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()

	if created.ID == 0 {
		t.Error("expected ID to be set")
	}
	if created.Name != "My Chart" {
		t.Errorf("name = %q, want %q", created.Name, "My Chart")
	}
	if created.ChartType != "step" {
		t.Errorf("chartType = %q, want %q", created.ChartType, "step")
	}

	// Get
	resp, err = http.Get(ts.URL + "/api/charts/" + itoa(created.ID))
	if err != nil {
		t.Fatalf("GET chart failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET chart status = %d, want 200", resp.StatusCode)
	}
	var got model.ChartDefinition
	json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()
	if got.Name != "My Chart" {
		t.Errorf("got name = %q, want %q", got.Name, "My Chart")
	}

	// Update
	updateBody := `{"name":"Renamed"}`
	req, _ := http.NewRequest("PATCH", ts.URL+"/api/charts/"+itoa(created.ID), strings.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH chart failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("PATCH chart status = %d, want 200", resp.StatusCode)
	}
	var updated model.ChartDefinition
	json.NewDecoder(resp.Body).Decode(&updated)
	resp.Body.Close()
	if updated.Name != "Renamed" {
		t.Errorf("updated name = %q, want %q", updated.Name, "Renamed")
	}

	// List should now have 4 (3 built-in + 1 custom)
	resp, _ = http.Get(ts.URL + "/api/traces/1/charts")
	var charts []model.ChartDefinition
	json.NewDecoder(resp.Body).Decode(&charts)
	resp.Body.Close()
	if len(charts) != 4 {
		t.Errorf("expected 4 charts after create, got %d", len(charts))
	}

	// Delete
	req, _ = http.NewRequest("DELETE", ts.URL+"/api/charts/"+itoa(created.ID), http.NoBody)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE chart failed: %v", err)
	}
	if resp.StatusCode != 204 {
		t.Errorf("DELETE chart status = %d, want 204", resp.StatusCode)
	}
	resp.Body.Close()

	// Get after delete → 404
	resp, _ = http.Get(ts.URL + "/api/charts/" + itoa(created.ID))
	if resp.StatusCode != 404 {
		t.Errorf("GET after delete status = %d, want 404", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAPI_Charts_CannotDeleteBuiltIn(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	// Get the first built-in chart ID
	resp, _ := http.Get(ts.URL + "/api/traces/1/charts")
	var charts []model.ChartDefinition
	json.NewDecoder(resp.Body).Decode(&charts)
	resp.Body.Close()

	if len(charts) == 0 {
		t.Fatal("no charts found")
	}

	req, _ := http.NewRequest("DELETE", ts.URL+"/api/charts/"+itoa(charts[0].ID), http.NoBody)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE failed: %v", err)
	}
	if resp.StatusCode != 403 {
		t.Errorf("DELETE built-in status = %d, want 403", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAPI_Charts_DataEndpoint(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	now := time.Now()
	msgs := []*model.Message{
		{
			TraceID: trace.ID, SequenceNum: 1, Timestamp: now,
			ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply",
			FunctionSet: "MeasurementListData",
			SpinePayload: json.RawMessage(`{
				"datagram": {"payload": {"cmd": [{"measurementListData": {
					"measurementData": [
						{"measurementId": 1, "value": {"number": 2300, "scale": 0}}
					]
				}}]}}
			}`),
		},
	}
	msgRepo.InsertMessages(msgs)

	// Get the "Measurements" built-in chart ID
	resp, _ := http.Get(ts.URL + "/api/traces/1/charts")
	var charts []model.ChartDefinition
	json.NewDecoder(resp.Body).Decode(&charts)
	resp.Body.Close()

	var measurementChartID int64
	for _, c := range charts {
		if c.Name == "Measurements" {
			measurementChartID = c.ID
			break
		}
	}
	if measurementChartID == 0 {
		t.Fatal("Measurements chart not found")
	}

	// Fetch data via chart data endpoint
	resp, err := http.Get(ts.URL + "/api/traces/1/charts/" + itoa(measurementChartID) + "/data")
	if err != nil {
		t.Fatalf("GET chart data failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET chart data status = %d, want 200", resp.StatusCode)
	}

	var tsResp TimeseriesResponse
	json.NewDecoder(resp.Body).Decode(&tsResp)
	resp.Body.Close()

	if len(tsResp.Series) != 1 {
		t.Fatalf("expected 1 series, got %d", len(tsResp.Series))
	}
	if tsResp.Series[0].DataPoints[0].Value != 2300.0 {
		t.Errorf("value = %f, want 2300", tsResp.Series[0].DataPoints[0].Value)
	}
}

func TestAPI_Charts_DataEndpoint_EmptyTrace(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "empty", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	// Get built-in chart
	resp, _ := http.Get(ts.URL + "/api/traces/1/charts")
	var charts []model.ChartDefinition
	json.NewDecoder(resp.Body).Decode(&charts)
	resp.Body.Close()

	resp, err := http.Get(ts.URL + "/api/traces/1/charts/" + itoa(charts[0].ID) + "/data")
	if err != nil {
		t.Fatalf("GET chart data failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	var tsResp TimeseriesResponse
	json.NewDecoder(resp.Body).Decode(&tsResp)
	resp.Body.Close()

	if len(tsResp.Series) != 0 {
		t.Errorf("expected 0 series for empty trace, got %d", len(tsResp.Series))
	}
}

func TestAPI_Charts_DataEndpoint_NotFound(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	resp, err := http.Get(ts.URL + "/api/traces/1/charts/999/data")
	if err != nil {
		t.Fatalf("GET chart data failed: %v", err)
	}
	if resp.StatusCode != 404 {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAPI_Charts_DataEndpoint_WithTimeRange(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	t1 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	msgs := []*model.Message{
		{TraceID: trace.ID, SequenceNum: 1, Timestamp: t1, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply", FunctionSet: "MeasurementListData",
			SpinePayload: json.RawMessage(`{"datagram":{"payload":{"cmd":[{"measurementListData":{"measurementData":[{"measurementId":1,"value":{"number":100,"scale":0}}]}}]}}}`)},
		{TraceID: trace.ID, SequenceNum: 2, Timestamp: t2, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply", FunctionSet: "MeasurementListData",
			SpinePayload: json.RawMessage(`{"datagram":{"payload":{"cmd":[{"measurementListData":{"measurementData":[{"measurementId":1,"value":{"number":200,"scale":0}}]}}]}}}`)},
	}
	msgRepo.InsertMessages(msgs)

	// Get measurement chart
	resp, _ := http.Get(ts.URL + "/api/traces/1/charts")
	var charts []model.ChartDefinition
	json.NewDecoder(resp.Body).Decode(&charts)
	resp.Body.Close()

	var chartID int64
	for _, c := range charts {
		if c.Name == "Measurements" {
			chartID = c.ID
			break
		}
	}

	// Fetch with time range filter
	resp, _ = http.Get(ts.URL + "/api/traces/1/charts/" + itoa(chartID) + "/data?timeFrom=" + t1.Format(time.RFC3339) + "&timeTo=" + t1.Format(time.RFC3339))
	var tsResp TimeseriesResponse
	json.NewDecoder(resp.Body).Decode(&tsResp)
	resp.Body.Close()

	if len(tsResp.Series) != 1 {
		t.Fatalf("expected 1 series, got %d", len(tsResp.Series))
	}
	if len(tsResp.Series[0].DataPoints) != 1 {
		t.Errorf("expected 1 data point (time filtered), got %d", len(tsResp.Series[0].DataPoints))
	}
}

func TestAPI_Charts_CreateMissingName(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	body := `{"chartType":"line"}`
	resp, err := http.Post(ts.URL+"/api/traces/1/charts", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST chart failed: %v", err)
	}
	if resp.StatusCode != 400 {
		t.Errorf("POST chart status = %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

// itoa converts int64 to string for URL building.
func itoa(n int64) string {
	return fmt.Sprintf("%d", n)
}
