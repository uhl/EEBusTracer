package analysis

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
)

func makeSpinePayload(cmdKey string, cmdValue interface{}) json.RawMessage {
	cmdBytes, _ := json.Marshal(cmdValue)
	payload := map[string]interface{}{
		"datagram": map[string]interface{}{
			"payload": map[string]interface{}{
				"cmd": []json.RawMessage{
					json.RawMessage(`{"` + cmdKey + `":` + string(cmdBytes) + `}`),
				},
			},
		},
	}
	b, _ := json.Marshal(payload)
	return b
}

func TestDetectUseCases_Empty(t *testing.T) {
	result := DetectUseCases(nil)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}
}

func TestDetectUseCases_NoUseCaseMessages(t *testing.T) {
	msgs := []*model.Message{
		{ID: 1, FunctionSet: "MeasurementListData", CmdClassifier: "reply"},
	}
	result := DetectUseCases(msgs)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}
}

func TestDetectUseCases_SingleDevice(t *testing.T) {
	ucPayload := makeSpinePayload("nodeManagementUseCaseData", map[string]interface{}{
		"useCaseInformation": []interface{}{
			map[string]interface{}{
				"actor": "CEM",
				"useCaseSupport": []interface{}{
					map[string]interface{}{
						"useCaseName":      "limitationOfPowerConsumption",
						"useCaseVersion":   "2.0.0",
						"useCaseAvailable": true,
						"scenarioSupport": []uint{1, 2},
					},
					map[string]interface{}{
						"useCaseName":      "monitoringOfPowerConsumption",
						"useCaseAvailable": true,
					},
				},
			},
		},
	})

	msgs := []*model.Message{
		{
			ID:            1,
			Timestamp:     time.Now(),
			FunctionSet:   "NodeManagementUseCaseData",
			CmdClassifier: "reply",
			ShipMsgType:   "data",
			DeviceSource:  "d:_i:HEMS_001",
			SpinePayload:  ucPayload,
		},
	}

	result := DetectUseCases(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 device use case group, got %d", len(result))
	}

	duc := result[0]
	if duc.DeviceAddr != "d:_i:HEMS_001" {
		t.Errorf("device addr = %q, want %q", duc.DeviceAddr, "d:_i:HEMS_001")
	}
	if duc.Actor != "CEM" {
		t.Errorf("actor = %q, want %q", duc.Actor, "CEM")
	}
	if len(duc.UseCases) != 2 {
		t.Fatalf("expected 2 use cases, got %d", len(duc.UseCases))
	}

	lpc := duc.UseCases[0]
	if lpc.Abbreviation != "LPC" {
		t.Errorf("abbreviation = %q, want %q", lpc.Abbreviation, "LPC")
	}
	if lpc.UseCaseVersion != "2.0.0" {
		t.Errorf("version = %q, want %q", lpc.UseCaseVersion, "2.0.0")
	}
	if !lpc.Available {
		t.Error("expected LPC to be available")
	}
	if len(lpc.Scenarios) != 2 {
		t.Errorf("expected 2 scenarios, got %d", len(lpc.Scenarios))
	}

	mpc := duc.UseCases[1]
	if mpc.Abbreviation != "MPC" {
		t.Errorf("abbreviation = %q, want %q", mpc.Abbreviation, "MPC")
	}
}

func TestDetectUseCases_MultipleDevices(t *testing.T) {
	makePayload := func(actor, useCaseName string) json.RawMessage {
		return makeSpinePayload("nodeManagementUseCaseData", map[string]interface{}{
			"useCaseInformation": []interface{}{
				map[string]interface{}{
					"actor": actor,
					"useCaseSupport": []interface{}{
						map[string]interface{}{
							"useCaseName":      useCaseName,
							"useCaseAvailable": true,
						},
					},
				},
			},
		})
	}

	msgs := []*model.Message{
		{ID: 1, FunctionSet: "NodeManagementUseCaseData", CmdClassifier: "reply", DeviceSource: "devA", SpinePayload: makePayload("CEM", "limitationOfPowerConsumption")},
		{ID: 2, FunctionSet: "NodeManagementUseCaseData", CmdClassifier: "notify", DeviceSource: "devB", SpinePayload: makePayload("ControllableSystem", "monitoringOfPowerConsumption")},
	}

	result := DetectUseCases(msgs)
	if len(result) != 2 {
		t.Fatalf("expected 2 device groups, got %d", len(result))
	}
	if result[0].DeviceAddr != "devA" {
		t.Errorf("first device = %q, want %q", result[0].DeviceAddr, "devA")
	}
	if result[1].DeviceAddr != "devB" {
		t.Errorf("second device = %q, want %q", result[1].DeviceAddr, "devB")
	}
}

func TestDetectUseCases_UnknownUseCaseName(t *testing.T) {
	payload := makeSpinePayload("nodeManagementUseCaseData", map[string]interface{}{
		"useCaseInformation": []interface{}{
			map[string]interface{}{
				"actor": "CEM",
				"useCaseSupport": []interface{}{
					map[string]interface{}{
						"useCaseName":      "someUnknownUseCase",
						"useCaseAvailable": true,
					},
				},
			},
		},
	})

	msgs := []*model.Message{
		{ID: 1, FunctionSet: "NodeManagementUseCaseData", CmdClassifier: "reply", DeviceSource: "devA", SpinePayload: payload},
	}

	result := DetectUseCases(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	// Unknown use case should use the full name as abbreviation
	if result[0].UseCases[0].Abbreviation != "someUnknownUseCase" {
		t.Errorf("abbreviation = %q, want full name as fallback", result[0].UseCases[0].Abbreviation)
	}
}

func TestDetectUseCases_IgnoresCallClassifier(t *testing.T) {
	payload := makeSpinePayload("nodeManagementUseCaseData", map[string]interface{}{
		"useCaseInformation": []interface{}{
			map[string]interface{}{
				"actor": "CEM",
				"useCaseSupport": []interface{}{
					map[string]interface{}{
						"useCaseName":      "limitationOfPowerConsumption",
						"useCaseAvailable": true,
					},
				},
			},
		},
	})

	msgs := []*model.Message{
		{ID: 1, FunctionSet: "NodeManagementUseCaseData", CmdClassifier: "call", DeviceSource: "devA", SpinePayload: payload},
	}

	result := DetectUseCases(msgs)
	if len(result) != 0 {
		t.Errorf("expected 0 results for call classifier, got %d", len(result))
	}
}

func TestDetectUseCases_UpdatesOnLaterMessage(t *testing.T) {
	makePayload := func(available bool) json.RawMessage {
		return makeSpinePayload("nodeManagementUseCaseData", map[string]interface{}{
			"useCaseInformation": []interface{}{
				map[string]interface{}{
					"actor": "CEM",
					"useCaseSupport": []interface{}{
						map[string]interface{}{
							"useCaseName":      "limitationOfPowerConsumption",
							"useCaseAvailable": available,
						},
					},
				},
			},
		})
	}

	msgs := []*model.Message{
		{ID: 1, FunctionSet: "NodeManagementUseCaseData", CmdClassifier: "reply", DeviceSource: "devA", SpinePayload: makePayload(true)},
		{ID: 2, FunctionSet: "NodeManagementUseCaseData", CmdClassifier: "notify", DeviceSource: "devA", SpinePayload: makePayload(false)},
	}

	result := DetectUseCases(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].UseCases[0].Available {
		t.Error("expected use case to be unavailable after update")
	}
	if result[0].MessageID != 2 {
		t.Errorf("messageID = %d, want 2", result[0].MessageID)
	}
}
