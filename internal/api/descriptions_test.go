package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
	"github.com/eebustracer/eebustracer/internal/store"
)

func TestPhaseLabel(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"a", "Phase A"},
		{"b", "Phase B"},
		{"c", "Phase C"},
		{"abc", "Total"},
		{"unknown", "unknown"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := phaseLabel(tt.input)
			if got != tt.want {
				t.Errorf("phaseLabel(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestScopeTypeLabel(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"overloadProtection", "Overload Protection"},
		{"selfConsumption", "Self Consumption"},
		{"discharge", "Discharge"},
		{"other", "other"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := scopeTypeLabel(tt.input)
			if got != tt.want {
				t.Errorf("scopeTypeLabel(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMeasurementTypeLabel(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"current", "Current"},
		{"power", "Power"},
		{"energy", "Energy"},
		{"voltage", "Voltage"},
		{"percentagePeak", "percentagePeak"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := measurementTypeLabel(tt.input)
			if got != tt.want {
				t.Errorf("measurementTypeLabel(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildMeasurementLabel(t *testing.T) {
	tests := []struct {
		name  string
		desc  MeasurementDesc
		want  string
	}{
		{
			name: "current phase A",
			desc: MeasurementDesc{MeasurementType: "current", Phase: "Phase A", Unit: "A"},
			want: "Current Phase A [A]",
		},
		{
			name: "power total",
			desc: MeasurementDesc{MeasurementType: "power", Phase: "Total", Unit: "W"},
			want: "Power Total [W]",
		},
		{
			name: "no phase",
			desc: MeasurementDesc{MeasurementType: "energy", Unit: "Wh"},
			want: "Energy [Wh]",
		},
		{
			name: "no unit",
			desc: MeasurementDesc{MeasurementType: "current", Phase: "Phase A"},
			want: "Current Phase A",
		},
		{
			name: "only type",
			desc: MeasurementDesc{MeasurementType: "voltage"},
			want: "Voltage",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildMeasurementLabel(tt.desc)
			if got != tt.want {
				t.Errorf("buildMeasurementLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildLimitLabel(t *testing.T) {
	tests := []struct {
		name string
		desc LimitDesc
		want string
	}{
		{
			name: "overload protection phase A",
			desc: LimitDesc{ScopeType: "overloadProtection", Phase: "Phase A", Unit: "A", LimitCategory: "obligation"},
			want: "Overload Protection Phase A [A]",
		},
		{
			name: "self consumption total",
			desc: LimitDesc{ScopeType: "selfConsumption", Phase: "Total", Unit: "W", LimitCategory: "recommendation"},
			want: "Self Consumption Total [W]",
		},
		{
			name: "no phase",
			desc: LimitDesc{ScopeType: "discharge", Unit: "A"},
			want: "Discharge [A]",
		},
		{
			name: "no unit",
			desc: LimitDesc{ScopeType: "overloadProtection", Phase: "Phase B"},
			want: "Overload Protection Phase B",
		},
		{
			name: "fallback with limit ID",
			desc: LimitDesc{LimitID: "5"},
			want: "Limit 5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildLimitLabel(tt.desc)
			if got != tt.want {
				t.Errorf("buildLimitLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoadDescriptionContext(t *testing.T) {
	_, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	now := time.Now()

	msgs := []*model.Message{
		// ElectricalConnectionParameterDescriptionListData — maps measurementId → phase
		{
			TraceID: trace.ID, SequenceNum: 1, Timestamp: now,
			ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply",
			FunctionSet: "ElectricalConnectionParameterDescriptionListData",
			SpinePayload: json.RawMessage(`{
				"datagram": {"payload": {"cmd": [{"electricalConnectionParameterDescriptionListData": {
					"electricalConnectionParameterDescriptionData": [
						{"measurementId": 1, "acMeasuredPhases": "a"},
						{"measurementId": 2, "acMeasuredPhases": "b"},
						{"measurementId": 3, "acMeasuredPhases": "c"},
						{"measurementId": 4, "acMeasuredPhases": "a"},
						{"measurementId": 5, "acMeasuredPhases": "abc"}
					]
				}}]}}
			}`),
		},
		// MeasurementDescriptionListData
		{
			TraceID: trace.ID, SequenceNum: 2, Timestamp: now.Add(time.Second),
			ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply",
			FunctionSet: "MeasurementDescriptionListData",
			SpinePayload: json.RawMessage(`{
				"datagram": {"payload": {"cmd": [{"measurementDescriptionListData": {
					"measurementDescriptionData": [
						{"measurementId": 1, "measurementType": "current", "unit": "A", "scopeType": "acCurrent"},
						{"measurementId": 2, "measurementType": "current", "unit": "A", "scopeType": "acCurrent"},
						{"measurementId": 3, "measurementType": "current", "unit": "A", "scopeType": "acCurrent"},
						{"measurementId": 4, "measurementType": "power", "unit": "W", "scopeType": "acPower"},
						{"measurementId": 5, "measurementType": "power", "unit": "W", "scopeType": "acPowerTotal"}
					]
				}}]}}
			}`),
		},
		// LoadControlLimitDescriptionListData
		{
			TraceID: trace.ID, SequenceNum: 3, Timestamp: now.Add(2 * time.Second),
			ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply",
			FunctionSet: "LoadControlLimitDescriptionListData",
			SpinePayload: json.RawMessage(`{
				"datagram": {"payload": {"cmd": [{"loadControlLimitDescriptionListData": {
					"loadControlLimitDescriptionData": [
						{"limitId": 1, "measurementId": 1, "limitType": "maxValueLimit", "limitCategory": "obligation", "scopeType": "overloadProtection", "unit": "A"},
						{"limitId": 2, "measurementId": 1, "limitType": "maxValueLimit", "limitCategory": "recommendation", "scopeType": "selfConsumption", "unit": "A"},
						{"limitId": 3, "measurementId": 4, "limitType": "maxValueLimit", "limitCategory": "obligation", "scopeType": "overloadProtection", "unit": "W"}
					]
				}}]}}
			}`),
		},
	}
	msgRepo.InsertMessages(msgs)

	logger := testLogger()
	srv := &Server{msgRepo: msgRepo, logger: logger}
	ctx := loadDescriptionContext(srv, trace.ID)

	if ctx == nil {
		t.Fatal("expected non-nil description context")
	}

	// Check measurements
	if len(ctx.Measurements) != 5 {
		t.Errorf("expected 5 measurements, got %d", len(ctx.Measurements))
	}

	m1, ok := ctx.Measurements["1"]
	if !ok {
		t.Fatal("measurement 1 not found")
	}
	if m1.Phase != "Phase A" {
		t.Errorf("m1 phase = %q, want %q", m1.Phase, "Phase A")
	}
	if m1.Label != "Current Phase A [A]" {
		t.Errorf("m1 label = %q, want %q", m1.Label, "Current Phase A [A]")
	}

	m4, ok := ctx.Measurements["4"]
	if !ok {
		t.Fatal("measurement 4 not found")
	}
	if m4.Label != "Power Phase A [W]" {
		t.Errorf("m4 label = %q, want %q", m4.Label, "Power Phase A [W]")
	}

	m5, ok := ctx.Measurements["5"]
	if !ok {
		t.Fatal("measurement 5 not found")
	}
	if m5.Label != "Power Total [W]" {
		t.Errorf("m5 label = %q, want %q", m5.Label, "Power Total [W]")
	}

	// Check limits
	if len(ctx.Limits) != 3 {
		t.Errorf("expected 3 limits, got %d", len(ctx.Limits))
	}

	l1, ok := ctx.Limits["1"]
	if !ok {
		t.Fatal("limit 1 not found")
	}
	if l1.Phase != "Phase A" {
		t.Errorf("l1 phase = %q, want %q", l1.Phase, "Phase A")
	}
	if l1.Label != "Overload Protection Phase A [A]" {
		t.Errorf("l1 label = %q, want %q", l1.Label, "Overload Protection Phase A [A]")
	}
	if l1.LimitCategory != "obligation" {
		t.Errorf("l1 category = %q, want %q", l1.LimitCategory, "obligation")
	}

	l2, ok := ctx.Limits["2"]
	if !ok {
		t.Fatal("limit 2 not found")
	}
	if l2.Label != "Self Consumption Phase A [A]" {
		t.Errorf("l2 label = %q, want %q", l2.Label, "Self Consumption Phase A [A]")
	}

	l3, ok := ctx.Limits["3"]
	if !ok {
		t.Fatal("limit 3 not found")
	}
	if l3.Label != "Overload Protection Phase A [W]" {
		t.Errorf("l3 label = %q, want %q", l3.Label, "Overload Protection Phase A [W]")
	}
}

func TestLoadDescriptionContext_NoDescriptions(t *testing.T) {
	_, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "empty", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	logger := testLogger()
	srv := &Server{msgRepo: store.NewMessageRepo(db), logger: logger}
	ctx := loadDescriptionContext(srv, trace.ID)

	if ctx == nil {
		t.Fatal("expected non-nil context even with no descriptions")
	}
	if len(ctx.Measurements) != 0 {
		t.Errorf("expected 0 measurements, got %d", len(ctx.Measurements))
	}
	if len(ctx.Limits) != 0 {
		t.Errorf("expected 0 limits, got %d", len(ctx.Limits))
	}
}

func TestAPI_Descriptions(t *testing.T) {
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
			FunctionSet: "ElectricalConnectionParameterDescriptionListData",
			SpinePayload: json.RawMessage(`{
				"datagram": {"payload": {"cmd": [{"electricalConnectionParameterDescriptionListData": {
					"electricalConnectionParameterDescriptionData": [
						{"measurementId": 1, "acMeasuredPhases": "a"}
					]
				}}]}}
			}`),
		},
		{
			TraceID: trace.ID, SequenceNum: 2, Timestamp: now.Add(time.Second),
			ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply",
			FunctionSet: "MeasurementDescriptionListData",
			SpinePayload: json.RawMessage(`{
				"datagram": {"payload": {"cmd": [{"measurementDescriptionListData": {
					"measurementDescriptionData": [
						{"measurementId": 1, "measurementType": "current", "unit": "A", "scopeType": "acCurrent"}
					]
				}}]}}
			}`),
		},
	}
	msgRepo.InsertMessages(msgs)

	resp, err := http.Get(ts.URL + "/api/traces/1/descriptions")
	if err != nil {
		t.Fatalf("GET descriptions failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET descriptions status = %d, want 200", resp.StatusCode)
	}

	var ctx DescriptionContext
	json.NewDecoder(resp.Body).Decode(&ctx)
	resp.Body.Close()

	if len(ctx.Measurements) != 1 {
		t.Errorf("expected 1 measurement, got %d", len(ctx.Measurements))
	}

	m1, ok := ctx.Measurements["1"]
	if !ok {
		t.Fatal("measurement 1 not found in API response")
	}
	if m1.Label != "Current Phase A [A]" {
		t.Errorf("label = %q, want %q", m1.Label, "Current Phase A [A]")
	}
}

func TestAPI_Descriptions_NotFound(t *testing.T) {
	ts, _ := setupTestServer(t)

	resp, err := http.Get(ts.URL + "/api/traces/999/descriptions")
	if err != nil {
		t.Fatalf("GET descriptions failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("GET descriptions status = %d, want 200 (empty context)", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestLoadDeviceConfigDescs(t *testing.T) {
	_, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	now := time.Now()

	msgs := []*model.Message{
		// DeviceConfigurationKeyValueDescriptionListData — maps keyId → keyName/valueType/unit
		{
			TraceID: trace.ID, SequenceNum: 1, Timestamp: now,
			ShipMsgType: model.ShipMsgTypeData, CmdClassifier: "reply",
			FunctionSet: "DeviceConfigurationKeyValueDescriptionListData",
			SpinePayload: json.RawMessage(`{
				"datagram": {"payload": {"cmd": [{"deviceConfigurationKeyValueDescriptionListData": {
					"deviceConfigurationKeyValueDescriptionData": [
						{"keyId": 0, "keyName": "communicationsStandard", "valueType": "string"},
						{"keyId": 1, "keyName": "asymmetricChargingSupported", "valueType": "boolean"},
						{"keyId": 2, "keyName": "failsafeConsumptionActivePowerLimit", "valueType": "scaledNumber", "unit": "W"},
						{"keyId": 3, "keyName": "failsafeDurationMinimum", "valueType": "duration", "unit": "ns"}
					]
				}}]}}
			}`),
		},
	}
	msgRepo.InsertMessages(msgs)

	logger := testLogger()
	srv := &Server{msgRepo: msgRepo, logger: logger}
	ctx := loadDescriptionContext(srv, trace.ID)

	if len(ctx.KeyValues) != 4 {
		t.Fatalf("expected 4 key values, got %d", len(ctx.KeyValues))
	}

	kv0, ok := ctx.KeyValues["0"]
	if !ok {
		t.Fatal("key 0 not found")
	}
	if kv0.KeyName != "communicationsStandard" {
		t.Errorf("kv0 keyName = %q, want %q", kv0.KeyName, "communicationsStandard")
	}
	if kv0.ValueType != "string" {
		t.Errorf("kv0 valueType = %q, want %q", kv0.ValueType, "string")
	}

	kv2, ok := ctx.KeyValues["2"]
	if !ok {
		t.Fatal("key 2 not found")
	}
	if kv2.KeyName != "failsafeConsumptionActivePowerLimit" {
		t.Errorf("kv2 keyName = %q, want %q", kv2.KeyName, "failsafeConsumptionActivePowerLimit")
	}
	if kv2.Unit != "W" {
		t.Errorf("kv2 unit = %q, want %q", kv2.Unit, "W")
	}
	if kv2.ValueType != "scaledNumber" {
		t.Errorf("kv2 valueType = %q, want %q", kv2.ValueType, "scaledNumber")
	}

	kv3, ok := ctx.KeyValues["3"]
	if !ok {
		t.Fatal("key 3 not found")
	}
	if kv3.ValueType != "duration" {
		t.Errorf("kv3 valueType = %q, want %q", kv3.ValueType, "duration")
	}
}

func TestLoadDeviceConfigDescs_EmptyTrace(t *testing.T) {
	_, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "empty", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	logger := testLogger()
	srv := &Server{msgRepo: store.NewMessageRepo(db), logger: logger}
	ctx := loadDescriptionContext(srv, trace.ID)

	if len(ctx.KeyValues) != 0 {
		t.Errorf("expected 0 key values, got %d", len(ctx.KeyValues))
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
