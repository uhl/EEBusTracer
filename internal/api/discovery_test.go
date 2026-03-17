package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
	"github.com/eebustracer/eebustracer/internal/store"
)

func TestIntrospectPayload_Measurement(t *testing.T) {
	payload := json.RawMessage(`{
		"datagram": {
			"payload": {
				"cmd": [
					{
						"measurementListData": {
							"measurementData": [
								{"measurementId": 1, "value": {"number": 2300, "scale": 0}},
								{"measurementId": 2, "value": {"number": 1500, "scale": -3}}
							]
						}
					}
				]
			}
		}
	}`)

	source := introspectPayload(payload, "MeasurementListData", 42)
	if source == nil {
		t.Fatal("expected a discovered source, got nil")
	}

	if source.FunctionSet != "MeasurementListData" {
		t.Errorf("functionSet = %q, want MeasurementListData", source.FunctionSet)
	}
	if source.CmdKey != "measurementListData" {
		t.Errorf("cmdKey = %q, want measurementListData", source.CmdKey)
	}
	if source.DataArrayKey != "measurementData" {
		t.Errorf("dataArrayKey = %q, want measurementData", source.DataArrayKey)
	}
	if source.IDField != "measurementId" {
		t.Errorf("idField = %q, want measurementId", source.IDField)
	}
	if len(source.SampleIDs) != 2 {
		t.Errorf("sampleIds = %v, want 2 ids", source.SampleIDs)
	}
	if source.MessageCount != 42 {
		t.Errorf("messageCount = %d, want 42", source.MessageCount)
	}
}

func TestIntrospectPayload_NoScaledNumber(t *testing.T) {
	// NodeManagement data does not have ScaledNumber values
	payload := json.RawMessage(`{
		"datagram": {
			"payload": {
				"cmd": [
					{
						"nodeManagementDetailedDiscoveryData": {
							"specificationVersionList": {
								"specificationVersion": [{"specificationVersion": "1.0.0"}]
							}
						}
					}
				]
			}
		}
	}`)

	source := introspectPayload(payload, "NodeManagementDetailedDiscoveryData", 5)
	if source != nil {
		t.Errorf("expected nil for non-numeric data, got %+v", source)
	}
}

func TestIntrospectPayload_EmptyPayload(t *testing.T) {
	source := introspectPayload(json.RawMessage(`{}`), "Whatever", 0)
	if source != nil {
		t.Errorf("expected nil for empty payload, got %+v", source)
	}
}

func TestIsIDField(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"measurementId", true},
		{"limitId", true},
		{"setpointId", true},
		{"subscriptionId", true},
		{"value", false},
		{"id", true},
		{"Id", true},
		{"x", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := isIDField(tt.name); got != tt.want {
			t.Errorf("isIDField(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestAPI_DiscoverTimeseries(t *testing.T) {
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
		{
			TraceID: trace.ID, SequenceNum: 2, Timestamp: now,
			ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply",
			FunctionSet: "LoadControlLimitListData",
			SpinePayload: json.RawMessage(`{
				"datagram": {"payload": {"cmd": [{"loadControlLimitListData": {
					"loadControlLimitData": [
						{"limitId": 0, "value": {"number": 4600, "scale": 0}}
					]
				}}]}}
			}`),
		},
		// Non-chartable data
		{
			TraceID: trace.ID, SequenceNum: 3, Timestamp: now,
			ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply",
			FunctionSet: "NodeManagementDetailedDiscoveryData",
			SpinePayload: json.RawMessage(`{
				"datagram": {"payload": {"cmd": [{"nodeManagementDetailedDiscoveryData": {
					"specificationVersionList": {"specificationVersion": [{"specificationVersion": "1.0.0"}]}
				}}]}}
			}`),
		},
	}
	msgRepo.InsertMessages(msgs)

	resp, err := http.Get(ts.URL + "/api/traces/1/timeseries/discover")
	if err != nil {
		t.Fatalf("GET discover failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET discover status = %d, want 200", resp.StatusCode)
	}

	var result DiscoveryResponse
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	if len(result.Sources) != 2 {
		t.Fatalf("expected 2 discovered sources, got %d", len(result.Sources))
	}

	// Sources should be sorted alphabetically by function set
	foundMeasurement := false
	foundLoadControl := false
	for _, s := range result.Sources {
		if s.FunctionSet == "MeasurementListData" {
			foundMeasurement = true
			if s.CmdKey != "measurementListData" {
				t.Errorf("measurement cmdKey = %q", s.CmdKey)
			}
		}
		if s.FunctionSet == "LoadControlLimitListData" {
			foundLoadControl = true
			if s.CmdKey != "loadControlLimitListData" {
				t.Errorf("loadcontrol cmdKey = %q", s.CmdKey)
			}
		}
	}
	if !foundMeasurement {
		t.Error("measurement source not discovered")
	}
	if !foundLoadControl {
		t.Error("load control source not discovered")
	}
}

func TestAPI_DiscoverTimeseries_EmptyTrace(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "empty", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	resp, err := http.Get(ts.URL + "/api/traces/1/timeseries/discover")
	if err != nil {
		t.Fatalf("GET discover failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET discover status = %d, want 200", resp.StatusCode)
	}

	var result DiscoveryResponse
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	if len(result.Sources) != 0 {
		t.Errorf("expected 0 sources for empty trace, got %d", len(result.Sources))
	}
}
