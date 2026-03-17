package api

import (
	"encoding/json"
	"math"
	"net/http"
	"testing"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
	"github.com/eebustracer/eebustracer/internal/store"
)

func TestScaledNumberToFloat(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   float64
		wantOK bool
	}{
		{
			name:   "simple integer",
			input:  `{"number": 4200, "scale": 0}`,
			want:   4200.0,
			wantOK: true,
		},
		{
			name:   "with positive scale",
			input:  `{"number": 42, "scale": 2}`,
			want:   4200.0,
			wantOK: true,
		},
		{
			name:   "with negative scale (milliwatt to watt)",
			input:  `{"number": 1500, "scale": -3}`,
			want:   1.5,
			wantOK: true,
		},
		{
			name:   "zero value",
			input:  `{"number": 0, "scale": 0}`,
			want:   0.0,
			wantOK: true,
		},
		{
			name:   "no scale field defaults to 0",
			input:  `{"number": 42}`,
			want:   42.0,
			wantOK: true,
		},
		{
			name:   "missing number",
			input:  `{"scale": 2}`,
			want:   0,
			wantOK: false,
		},
		{
			name:   "invalid json",
			input:  `not json`,
			want:   0,
			wantOK: false,
		},
		{
			name:   "empty object",
			input:  `{}`,
			want:   0,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := scaledNumberToFloat(json.RawMessage(tt.input))
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && math.Abs(got-tt.want) > 0.0001 {
				t.Errorf("value = %f, want %f", got, tt.want)
			}
		})
	}
}

// TestExtractGenericData covers all 3 built-in descriptors with table-driven tests.
func TestExtractGenericData(t *testing.T) {
	tests := []struct {
		name     string
		desc     ExtractionDescriptor
		payload  string
		wantLen  int
		wantID   string
		wantVal  float64
		wantID2  string
		wantVal2 float64
	}{
		{
			name: "measurement data",
			desc: builtInDescriptors["measurement"],
			payload: `{
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
			}`,
			wantLen:  2,
			wantID:   "1",
			wantVal:  2300.0,
			wantID2:  "2",
			wantVal2: 1.5,
		},
		{
			name: "load control data",
			desc: builtInDescriptors["loadcontrol"],
			payload: `{
				"datagram": {
					"payload": {
						"cmd": [
							{
								"loadControlLimitListData": {
									"loadControlLimitData": [
										{"limitId": 0, "value": {"number": 4600, "scale": 0}},
										{"limitId": 1, "value": {"number": 6000, "scale": 0}}
									]
								}
							}
						]
					}
				}
			}`,
			wantLen:  2,
			wantID:   "0",
			wantVal:  4600.0,
			wantID2:  "1",
			wantVal2: 6000.0,
		},
		{
			name: "setpoint data",
			desc: builtInDescriptors["setpoint"],
			payload: `{
				"datagram": {
					"payload": {
						"cmd": [
							{
								"setpointListData": {
									"setpointData": [
										{"setpointId": 1, "value": {"number": 220, "scale": 0}},
										{"setpointId": 2, "value": {"number": 500, "scale": -1}}
									]
								}
							}
						]
					}
				}
			}`,
			wantLen:  2,
			wantID:   "1",
			wantVal:  220.0,
			wantID2:  "2",
			wantVal2: 50.0,
		},
		{
			name:    "empty payload",
			desc:    builtInDescriptors["measurement"],
			payload: `{}`,
			wantLen: 0,
		},
		{
			name:    "invalid json",
			desc:    builtInDescriptors["measurement"],
			payload: `not json`,
			wantLen: 0,
		},
		{
			name: "missing value field",
			desc: builtInDescriptors["measurement"],
			payload: `{
				"datagram": {
					"payload": {
						"cmd": [
							{
								"measurementListData": {
									"measurementData": [
										{"measurementId": 1}
									]
								}
							}
						]
					}
				}
			}`,
			wantLen: 0,
		},
		{
			name: "missing id field",
			desc: builtInDescriptors["measurement"],
			payload: `{
				"datagram": {
					"payload": {
						"cmd": [
							{
								"measurementListData": {
									"measurementData": [
										{"value": {"number": 100, "scale": 0}}
									]
								}
							}
						]
					}
				}
			}`,
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := extractGenericData(json.RawMessage(tt.payload), tt.desc)
			if len(items) != tt.wantLen {
				t.Fatalf("expected %d items, got %d", tt.wantLen, len(items))
			}
			if tt.wantLen >= 1 {
				if items[0].ID != tt.wantID || math.Abs(items[0].Value-tt.wantVal) > 0.0001 {
					t.Errorf("item 0: ID=%q Value=%f, want ID=%s Value=%f", items[0].ID, items[0].Value, tt.wantID, tt.wantVal)
				}
			}
			if tt.wantLen >= 2 {
				if items[1].ID != tt.wantID2 || math.Abs(items[1].Value-tt.wantVal2) > 0.0001 {
					t.Errorf("item 1: ID=%q Value=%f, want ID=%s Value=%f", items[1].ID, items[1].Value, tt.wantID2, tt.wantVal2)
				}
			}
		})
	}
}

// TestExtractGenericSeries covers label enrichment, filterID, ordering, and classifier filtering.
func TestExtractGenericSeries(t *testing.T) {
	now := time.Now()

	t.Run("measurement series with labels", func(t *testing.T) {
		msgs := []*model.Message{
			{
				ID: 1, Timestamp: now, CmdClassifier: "reply",
				SpinePayload: json.RawMessage(`{
					"datagram": {"payload": {"cmd": [{"measurementListData": {
						"measurementData": [
							{"measurementId": 1, "value": {"number": 100, "scale": 0}},
							{"measurementId": 2, "value": {"number": 200, "scale": 0}}
						]
					}}]}}
				}`),
			},
			{
				ID: 2, Timestamp: now.Add(time.Second), CmdClassifier: "notify",
				SpinePayload: json.RawMessage(`{
					"datagram": {"payload": {"cmd": [{"measurementListData": {
						"measurementData": [
							{"measurementId": 1, "value": {"number": 150, "scale": 0}}
						]
					}}]}}
				}`),
			},
		}

		labels := map[string]SeriesLabel{
			"1": {Label: "Power [W]", Unit: "W"},
		}
		series := extractGenericSeries(msgs, builtInDescriptors["measurement"], labels, "")

		if len(series) != 2 {
			t.Fatalf("expected 2 series, got %d", len(series))
		}

		if series[0].Label != "Power [W]" {
			t.Errorf("series 0 label = %q, want %q", series[0].Label, "Power [W]")
		}
		if series[0].Unit != "W" {
			t.Errorf("series 0 unit = %q, want %q", series[0].Unit, "W")
		}
		if len(series[0].DataPoints) != 2 {
			t.Errorf("series 0 data points = %d, want 2", len(series[0].DataPoints))
		}
		// Series 2 should use ID as default label
		if series[1].Label != "2" {
			t.Errorf("series 1 label = %q, want %q", series[1].Label, "2")
		}
	})

	t.Run("filter by ID", func(t *testing.T) {
		msgs := []*model.Message{
			{
				ID: 1, Timestamp: now, CmdClassifier: "reply",
				SpinePayload: json.RawMessage(`{
					"datagram": {"payload": {"cmd": [{"measurementListData": {
						"measurementData": [
							{"measurementId": 1, "value": {"number": 100, "scale": 0}},
							{"measurementId": 2, "value": {"number": 200, "scale": 0}}
						]
					}}]}}
				}`),
			},
		}

		series := extractGenericSeries(msgs, builtInDescriptors["measurement"], nil, "1")

		if len(series) != 1 {
			t.Fatalf("expected 1 series (filtered), got %d", len(series))
		}
		if series[0].ID != "1" {
			t.Errorf("series ID = %q, want %q", series[0].ID, "1")
		}
	})

	t.Run("skips non-matching classifiers", func(t *testing.T) {
		msgs := []*model.Message{
			{
				ID: 1, Timestamp: now, CmdClassifier: "read",
				SpinePayload: json.RawMessage(`{
					"datagram": {"payload": {"cmd": [{"measurementListData": {
						"measurementData": [
							{"measurementId": 1, "value": {"number": 100, "scale": 0}}
						]
					}}]}}
				}`),
			},
		}

		series := extractGenericSeries(msgs, builtInDescriptors["measurement"], nil, "")
		if len(series) != 0 {
			t.Errorf("expected 0 series (read skipped), got %d", len(series))
		}
	})

	t.Run("loadcontrol accepts write classifier", func(t *testing.T) {
		msgs := []*model.Message{
			{
				ID: 1, Timestamp: now, CmdClassifier: "write",
				SpinePayload: json.RawMessage(`{
					"datagram": {"payload": {"cmd": [{"loadControlLimitListData": {
						"loadControlLimitData": [
							{"limitId": 0, "value": {"number": 4600, "scale": 0}}
						]
					}}]}}
				}`),
			},
		}

		series := extractGenericSeries(msgs, builtInDescriptors["loadcontrol"], nil, "")
		if len(series) != 1 {
			t.Fatalf("expected 1 series (write accepted for loadcontrol), got %d", len(series))
		}
		if series[0].DataPoints[0].Value != 4600.0 {
			t.Errorf("value = %f, want 4600", series[0].DataPoints[0].Value)
		}
	})

	t.Run("setpoint series with labels", func(t *testing.T) {
		msgs := []*model.Message{
			{
				ID: 1, Timestamp: now, CmdClassifier: "reply",
				SpinePayload: json.RawMessage(`{
					"datagram": {"payload": {"cmd": [{"setpointListData": {
						"setpointData": [
							{"setpointId": 1, "value": {"number": 220, "scale": 0}},
							{"setpointId": 2, "value": {"number": 400, "scale": 0}}
						]
					}}]}}
				}`),
			},
			{
				ID: 2, Timestamp: now.Add(time.Second), CmdClassifier: "notify",
				SpinePayload: json.RawMessage(`{
					"datagram": {"payload": {"cmd": [{"setpointListData": {
						"setpointData": [
							{"setpointId": 1, "value": {"number": 230, "scale": 0}}
						]
					}}]}}
				}`),
			},
		}

		labels := map[string]SeriesLabel{"1": {Label: "valueAbsolute [°C]"}}
		series := extractGenericSeries(msgs, builtInDescriptors["setpoint"], labels, "")

		if len(series) != 2 {
			t.Fatalf("expected 2 series, got %d", len(series))
		}

		if series[0].Label != "valueAbsolute [°C]" {
			t.Errorf("series 0 label = %q, want %q", series[0].Label, "valueAbsolute [°C]")
		}
		if len(series[0].DataPoints) != 2 {
			t.Errorf("series 0 data points = %d, want 2", len(series[0].DataPoints))
		}
		// Series 2 should use ID as default label
		if series[1].Label != "2" {
			t.Errorf("series 1 label = %q, want %q", series[1].Label, "2")
		}
	})

	t.Run("preserves series order", func(t *testing.T) {
		msgs := []*model.Message{
			{
				ID: 1, Timestamp: now, CmdClassifier: "reply",
				SpinePayload: json.RawMessage(`{
					"datagram": {"payload": {"cmd": [{"measurementListData": {
						"measurementData": [
							{"measurementId": 3, "value": {"number": 300, "scale": 0}},
							{"measurementId": 1, "value": {"number": 100, "scale": 0}}
						]
					}}]}}
				}`),
			},
		}

		series := extractGenericSeries(msgs, builtInDescriptors["measurement"], nil, "")
		if len(series) != 2 {
			t.Fatalf("expected 2 series, got %d", len(series))
		}
		if series[0].ID != "3" {
			t.Errorf("first series ID = %q, want %q (order should be preserved)", series[0].ID, "3")
		}
		if series[1].ID != "1" {
			t.Errorf("second series ID = %q, want %q", series[1].ID, "1")
		}
	})

	t.Run("empty messages", func(t *testing.T) {
		series := extractGenericSeries(nil, builtInDescriptors["measurement"], nil, "")
		if len(series) != 0 {
			t.Errorf("expected 0 series for nil msgs, got %d", len(series))
		}
	})
}

// TestBuiltInDescriptors verifies that all expected built-in descriptors exist.
func TestBuiltInDescriptors(t *testing.T) {
	expectedTypes := []string{"measurement", "loadcontrol", "setpoint"}
	for _, dt := range expectedTypes {
		if _, ok := builtInDescriptors[dt]; !ok {
			t.Errorf("missing built-in descriptor for %q", dt)
		}
		if _, ok := builtInFunctionSets[dt]; !ok {
			t.Errorf("missing built-in function set for %q", dt)
		}
	}
}

// Integration tests — these test the full HTTP API path.

func TestAPI_Timeseries_Measurement(t *testing.T) {
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

	resp, err := http.Get(ts.URL + "/api/traces/1/timeseries?type=measurement")
	if err != nil {
		t.Fatalf("GET timeseries failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET timeseries status = %d, want 200", resp.StatusCode)
	}

	var tsResp TimeseriesResponse
	json.NewDecoder(resp.Body).Decode(&tsResp)
	resp.Body.Close()

	if tsResp.Type != "measurement" {
		t.Errorf("type = %q, want %q", tsResp.Type, "measurement")
	}
	if len(tsResp.Series) != 1 {
		t.Fatalf("expected 1 series, got %d", len(tsResp.Series))
	}
	if len(tsResp.Series[0].DataPoints) != 1 {
		t.Errorf("expected 1 data point, got %d", len(tsResp.Series[0].DataPoints))
	}
	if tsResp.Series[0].DataPoints[0].Value != 2300.0 {
		t.Errorf("value = %f, want 2300", tsResp.Series[0].DataPoints[0].Value)
	}
}

func TestAPI_Timeseries_EmptyTrace(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "empty", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	resp, err := http.Get(ts.URL + "/api/traces/1/timeseries")
	if err != nil {
		t.Fatalf("GET timeseries failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET timeseries status = %d, want 200", resp.StatusCode)
	}

	var tsResp TimeseriesResponse
	json.NewDecoder(resp.Body).Decode(&tsResp)
	resp.Body.Close()

	if len(tsResp.Series) != 0 {
		t.Errorf("expected 0 series for empty trace, got %d", len(tsResp.Series))
	}
}

func TestAPI_Timeseries_LoadControl(t *testing.T) {
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
			FunctionSet: "LoadControlLimitListData",
			SpinePayload: json.RawMessage(`{
				"datagram": {"payload": {"cmd": [{"loadControlLimitListData": {
					"loadControlLimitData": [
						{"limitId": 0, "value": {"number": 4600, "scale": 0}}
					]
				}}]}}
			}`),
		},
	}
	msgRepo.InsertMessages(msgs)

	resp, err := http.Get(ts.URL + "/api/traces/1/timeseries?type=loadcontrol")
	if err != nil {
		t.Fatalf("GET loadcontrol failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET loadcontrol status = %d, want 200", resp.StatusCode)
	}

	var tsResp TimeseriesResponse
	json.NewDecoder(resp.Body).Decode(&tsResp)
	resp.Body.Close()

	if tsResp.Type != "loadcontrol" {
		t.Errorf("type = %q, want %q", tsResp.Type, "loadcontrol")
	}
	if len(tsResp.Series) != 1 {
		t.Fatalf("expected 1 series, got %d", len(tsResp.Series))
	}
	if tsResp.Series[0].DataPoints[0].Value != 4600.0 {
		t.Errorf("value = %f, want 4600", tsResp.Series[0].DataPoints[0].Value)
	}
}

func TestAPI_Timeseries_Setpoint(t *testing.T) {
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
			FunctionSet: "SetpointListData",
			SpinePayload: json.RawMessage(`{
				"datagram": {"payload": {"cmd": [{"setpointListData": {
					"setpointData": [
						{"setpointId": 1, "value": {"number": 220, "scale": 0}}
					]
				}}]}}
			}`),
		},
	}
	msgRepo.InsertMessages(msgs)

	resp, err := http.Get(ts.URL + "/api/traces/1/timeseries?type=setpoint")
	if err != nil {
		t.Fatalf("GET setpoint failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET setpoint status = %d, want 200", resp.StatusCode)
	}

	var tsResp TimeseriesResponse
	json.NewDecoder(resp.Body).Decode(&tsResp)
	resp.Body.Close()

	if tsResp.Type != "setpoint" {
		t.Errorf("type = %q, want %q", tsResp.Type, "setpoint")
	}
	if len(tsResp.Series) != 1 {
		t.Fatalf("expected 1 series, got %d", len(tsResp.Series))
	}
	if len(tsResp.Series[0].DataPoints) != 1 {
		t.Errorf("expected 1 data point, got %d", len(tsResp.Series[0].DataPoints))
	}
	if tsResp.Series[0].DataPoints[0].Value != 220.0 {
		t.Errorf("value = %f, want 220", tsResp.Series[0].DataPoints[0].Value)
	}
}

func TestAPI_Timeseries_SetpointDescriptionEnrichment(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	now := time.Now()
	msgs := []*model.Message{
		// Description message
		{
			TraceID: trace.ID, SequenceNum: 1, Timestamp: now,
			ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply",
			FunctionSet: "SetpointDescriptionListData",
			SpinePayload: json.RawMessage(`{
				"datagram": {"payload": {"cmd": [{"setpointDescriptionListData": {
					"setpointDescriptionData": [
						{"setpointId": 1, "setpointType": "valueAbsolute", "unit": "°C"}
					]
				}}]}}
			}`),
		},
		// Setpoint message
		{
			TraceID: trace.ID, SequenceNum: 2, Timestamp: now.Add(time.Second),
			ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply",
			FunctionSet: "SetpointListData",
			SpinePayload: json.RawMessage(`{
				"datagram": {"payload": {"cmd": [{"setpointListData": {
					"setpointData": [
						{"setpointId": 1, "value": {"number": 220, "scale": 0}}
					]
				}}]}}
			}`),
		},
	}
	msgRepo.InsertMessages(msgs)

	resp, err := http.Get(ts.URL + "/api/traces/1/timeseries?type=setpoint")
	if err != nil {
		t.Fatalf("GET timeseries failed: %v", err)
	}

	var tsResp TimeseriesResponse
	json.NewDecoder(resp.Body).Decode(&tsResp)
	resp.Body.Close()

	if len(tsResp.Series) != 1 {
		t.Fatalf("expected 1 series, got %d", len(tsResp.Series))
	}

	expectedLabel := "valueAbsolute [°C]"
	if tsResp.Series[0].Label != expectedLabel {
		t.Errorf("label = %q, want %q", tsResp.Series[0].Label, expectedLabel)
	}
}

func TestAPI_Timeseries_TimeRangeFilter(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	t1 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC)
	t3 := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	msgs := []*model.Message{
		{TraceID: trace.ID, SequenceNum: 1, Timestamp: t1, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply", FunctionSet: "MeasurementListData",
			SpinePayload: json.RawMessage(`{"datagram":{"payload":{"cmd":[{"measurementListData":{"measurementData":[{"measurementId":1,"value":{"number":100,"scale":0}}]}}]}}}`)},
		{TraceID: trace.ID, SequenceNum: 2, Timestamp: t2, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply", FunctionSet: "MeasurementListData",
			SpinePayload: json.RawMessage(`{"datagram":{"payload":{"cmd":[{"measurementListData":{"measurementData":[{"measurementId":1,"value":{"number":200,"scale":0}}]}}]}}}`)},
		{TraceID: trace.ID, SequenceNum: 3, Timestamp: t3, ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply", FunctionSet: "MeasurementListData",
			SpinePayload: json.RawMessage(`{"datagram":{"payload":{"cmd":[{"measurementListData":{"measurementData":[{"measurementId":1,"value":{"number":300,"scale":0}}]}}]}}}`)},
	}
	msgRepo.InsertMessages(msgs)

	// Filter to only middle time range
	resp, _ := http.Get(ts.URL + "/api/traces/1/timeseries?type=measurement&timeFrom=" + t2.Format(time.RFC3339) + "&timeTo=" + t2.Format(time.RFC3339))
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

func TestAPI_Timeseries_DescriptionEnrichment(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	now := time.Now()
	msgs := []*model.Message{
		// Description message
		{
			TraceID: trace.ID, SequenceNum: 1, Timestamp: now,
			ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply",
			FunctionSet: "MeasurementDescriptionListData",
			SpinePayload: json.RawMessage(`{
				"datagram": {"payload": {"cmd": [{"measurementDescriptionListData": {
					"measurementDescriptionData": [
						{"measurementId": 1, "measurementType": "power", "unit": "W", "scopeType": "acPower"}
					]
				}}]}}
			}`),
		},
		// Measurement message
		{
			TraceID: trace.ID, SequenceNum: 2, Timestamp: now.Add(time.Second),
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

	resp, err := http.Get(ts.URL + "/api/traces/1/timeseries?type=measurement")
	if err != nil {
		t.Fatalf("GET timeseries failed: %v", err)
	}

	var tsResp TimeseriesResponse
	json.NewDecoder(resp.Body).Decode(&tsResp)
	resp.Body.Close()

	if len(tsResp.Series) != 1 {
		t.Fatalf("expected 1 series, got %d", len(tsResp.Series))
	}

	// Label should be enriched from description (no phase data, so just type + unit)
	expectedLabel := "Power [W]"
	if tsResp.Series[0].Label != expectedLabel {
		t.Errorf("label = %q, want %q", tsResp.Series[0].Label, expectedLabel)
	}
}
