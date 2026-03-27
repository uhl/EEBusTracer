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

func TestAPI_Lifecycle_Empty(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	resp, err := http.Get(ts.URL + "/api/traces/" + strconv.FormatInt(trace.ID, 10) + "/lifecycle")
	if err != nil {
		t.Fatalf("GET lifecycle failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var result []analysis.DeviceUseCaseLifecycle
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	if len(result) != 0 {
		t.Errorf("expected 0 entries, got %d", len(result))
	}
}

func TestAPI_Lifecycle_FullSetup(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	now := time.Now()

	// SHIP data message to establish connection at "data" state
	// Use case announcement (LPC available)
	ucPayload := `{"datagram":{"payload":{"cmd":[{"nodeManagementUseCaseData":{"useCaseInformation":[{"actor":"CEM","useCaseSupport":[{"useCaseName":"limitationOfPowerConsumption","useCaseVersion":"2.0.0","useCaseAvailable":true,"scenarioSupport":[1,2,3,4]}]}]}}]}}}`

	// Discovery data with LoadControl features (camelCase function names as sent by real devices)
	discoveryPayload := `{"datagram":{"payload":{"cmd":[{"nodeManagementDetailedDiscoveryData":{"specificationVersionList":{"specificationVersion":["1.3.0"]},"deviceInformation":{"description":{"deviceAddress":{"device":"d:_i:123_EVSE"}}},"entityInformation":[{"description":{"entityAddress":{"entity":[1,1]},"entityType":"EVSE"}}],"featureInformation":[{"description":{"featureAddress":{"entity":[1,1],"feature":1},"featureType":"LoadControl","role":"server","supportedFunction":[{"function":"loadControlLimitListData"},{"function":"loadControlLimitDescriptionListData"},{"function":"loadControlLimitConstraintsListData"}]}}]}}]}}}`

	// Subscription data
	subPayload := `{"datagram":{"payload":{"cmd":[{"nodeManagementSubscriptionData":{"subscriptionEntry":[{"subscriptionId":"1","serverFeatureType":"LoadControl","clientAddress":{"device":"d:_i:456_CEM","entity":[1],"feature":1},"serverAddress":{"device":"d:_i:123_EVSE","entity":[1,1],"feature":1}}]}}]}}}`

	// Binding data
	bindPayload := `{"datagram":{"payload":{"cmd":[{"nodeManagementBindingData":{"bindingEntry":[{"bindingId":"1","serverFeatureType":"LoadControl","clientAddress":{"device":"d:_i:456_CEM","entity":[1],"feature":1},"serverAddress":{"device":"d:_i:123_EVSE","entity":[1,1],"feature":1}}]}}]}}}`

	msgs := []*model.Message{
		{
			TraceID: trace.ID, SequenceNum: 1, Timestamp: now,
			ShipMsgType: "data", CmdClassifier: "reply",
			FunctionSet: "NodeManagementUseCaseData",
			DeviceSource: "d:_i:123_EVSE", DeviceDest: "d:_i:456_CEM",
			SpinePayload: json.RawMessage(ucPayload),
		},
		{
			TraceID: trace.ID, SequenceNum: 2, Timestamp: now.Add(time.Second),
			ShipMsgType: "data", CmdClassifier: "reply",
			FunctionSet: "NodeManagementDetailedDiscoveryData",
			DeviceSource: "d:_i:123_EVSE", DeviceDest: "d:_i:456_CEM",
			SpinePayload: json.RawMessage(discoveryPayload),
		},
		{
			TraceID: trace.ID, SequenceNum: 3, Timestamp: now.Add(2 * time.Second),
			ShipMsgType: "data", CmdClassifier: "reply",
			FunctionSet: "NodeManagementSubscriptionData",
			DeviceSource: "d:_i:123_EVSE", DeviceDest: "d:_i:456_CEM",
			SpinePayload: json.RawMessage(subPayload),
		},
		{
			TraceID: trace.ID, SequenceNum: 4, Timestamp: now.Add(3 * time.Second),
			ShipMsgType: "data", CmdClassifier: "reply",
			FunctionSet: "NodeManagementBindingData",
			DeviceSource: "d:_i:123_EVSE", DeviceDest: "d:_i:456_CEM",
			SpinePayload: json.RawMessage(bindPayload),
		},
	}
	msgRepo.InsertMessages(msgs)

	resp, err := http.Get(ts.URL + "/api/traces/" + strconv.FormatInt(trace.ID, 10) + "/lifecycle")
	if err != nil {
		t.Fatalf("GET lifecycle failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var result []analysis.DeviceUseCaseLifecycle
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	if len(result) != 1 {
		t.Fatalf("expected 1 lifecycle entry, got %d", len(result))
	}

	lc := result[0]
	if lc.UseCaseAbbr != "LPC" {
		t.Errorf("useCaseAbbr = %q, want LPC", lc.UseCaseAbbr)
	}
	if !lc.Available {
		t.Error("expected available = true")
	}

	// Verify we have 5 steps
	if len(lc.Steps) != 5 {
		t.Fatalf("expected 5 steps, got %d", len(lc.Steps))
	}

	// UC Announced should pass
	for _, step := range lc.Steps {
		if step.Name == "UC Announced" && step.Status != "pass" {
			t.Errorf("UC Announced status = %q, want pass", step.Status)
		}
	}
}

func TestAPI_Lifecycle_PartialSetup(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	now := time.Now()

	// Use case announced but no subscriptions or bindings
	ucPayload := `{"datagram":{"payload":{"cmd":[{"nodeManagementUseCaseData":{"useCaseInformation":[{"actor":"CEM","useCaseSupport":[{"useCaseName":"limitationOfPowerConsumption","useCaseVersion":"2.0.0","useCaseAvailable":true,"scenarioSupport":[1,2,3,4]}]}]}}]}}}`

	msgs := []*model.Message{
		{
			TraceID: trace.ID, SequenceNum: 1, Timestamp: now,
			ShipMsgType: "data", CmdClassifier: "reply",
			FunctionSet: "NodeManagementUseCaseData",
			DeviceSource: "d:_i:123_EVSE", DeviceDest: "d:_i:456_CEM",
			SpinePayload: json.RawMessage(ucPayload),
		},
	}
	msgRepo.InsertMessages(msgs)

	resp, err := http.Get(ts.URL + "/api/traces/" + strconv.FormatInt(trace.ID, 10) + "/lifecycle")
	if err != nil {
		t.Fatalf("GET lifecycle failed: %v", err)
	}

	var result []analysis.DeviceUseCaseLifecycle
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	if len(result) != 1 {
		t.Fatalf("expected 1 lifecycle entry, got %d", len(result))
	}

	lc := result[0]
	if lc.OverallStatus != "fail" {
		t.Errorf("overallStatus = %q, want fail", lc.OverallStatus)
	}
}

func TestAPI_Lifecycle_NoSpec(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	now := time.Now()

	// Use case with unknown abbreviation
	ucPayload := `{"datagram":{"payload":{"cmd":[{"nodeManagementUseCaseData":{"useCaseInformation":[{"actor":"CEM","useCaseSupport":[{"useCaseName":"someExoticUseCase","useCaseAvailable":true}]}]}}]}}}`

	msgs := []*model.Message{
		{
			TraceID: trace.ID, SequenceNum: 1, Timestamp: now,
			ShipMsgType: "data", CmdClassifier: "reply",
			FunctionSet: "NodeManagementUseCaseData",
			DeviceSource: "d:_i:123_EVSE",
			SpinePayload: json.RawMessage(ucPayload),
		},
	}
	msgRepo.InsertMessages(msgs)

	resp, err := http.Get(ts.URL + "/api/traces/" + strconv.FormatInt(trace.ID, 10) + "/lifecycle")
	if err != nil {
		t.Fatalf("GET lifecycle failed: %v", err)
	}

	var result []analysis.DeviceUseCaseLifecycle
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	if len(result) != 1 {
		t.Fatalf("expected 1 lifecycle entry, got %d", len(result))
	}

	// Subscriptions and bindings should be N/A for unknown UC
	for _, step := range result[0].Steps {
		if step.Name == "Subscriptions" && step.Status != "na" {
			t.Errorf("Subscriptions status = %q, want na", step.Status)
		}
		if step.Name == "Bindings" && step.Status != "na" {
			t.Errorf("Bindings status = %q, want na", step.Status)
		}
	}
}
