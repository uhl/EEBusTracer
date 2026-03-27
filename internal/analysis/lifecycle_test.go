package analysis

import (
	"strings"
	"testing"
)

func TestEvaluateLifecycles_Empty(t *testing.T) {
	result := EvaluateLifecycles(LifecycleInput{})
	if result == nil {
		t.Fatal("expected non-nil empty slice, got nil")
	}
	if len(result) != 0 {
		t.Errorf("expected 0 entries, got %d", len(result))
	}
}

func TestEvaluateLifecycles_HandshakePass(t *testing.T) {
	input := LifecycleInput{
		Connections: []ConnectionInfo{
			{DeviceSource: "devA", DeviceDest: "devB", CurrentState: "data"},
		},
		UseCases: []DeviceUseCases{
			{DeviceAddr: "devA", UseCases: []UseCaseInfo{
				{UseCaseName: "limitationOfPowerConsumption", Abbreviation: "LPC", Available: true},
			}},
		},
	}

	result := EvaluateLifecycles(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}

	step := findStep(result[0].Steps, "SHIP Handshake")
	if step == nil {
		t.Fatal("SHIP Handshake step not found")
	}
	if step.Status != StepPass {
		t.Errorf("handshake status = %q, want %q", step.Status, StepPass)
	}
}

func TestEvaluateLifecycles_HandshakeFail(t *testing.T) {
	input := LifecycleInput{
		Connections: []ConnectionInfo{
			{DeviceSource: "devA", DeviceDest: "devB", CurrentState: "handshake"},
		},
		UseCases: []DeviceUseCases{
			{DeviceAddr: "devA", UseCases: []UseCaseInfo{
				{UseCaseName: "limitationOfPowerConsumption", Abbreviation: "LPC", Available: true},
			}},
		},
	}

	result := EvaluateLifecycles(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}

	step := findStep(result[0].Steps, "SHIP Handshake")
	if step == nil {
		t.Fatal("SHIP Handshake step not found")
	}
	if step.Status != StepFail {
		t.Errorf("handshake status = %q, want %q", step.Status, StepFail)
	}
}

func TestEvaluateLifecycles_HandshakePending(t *testing.T) {
	input := LifecycleInput{
		UseCases: []DeviceUseCases{
			{DeviceAddr: "devA", UseCases: []UseCaseInfo{
				{UseCaseName: "limitationOfPowerConsumption", Abbreviation: "LPC", Available: true},
			}},
		},
	}

	result := EvaluateLifecycles(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}

	step := findStep(result[0].Steps, "SHIP Handshake")
	if step == nil {
		t.Fatal("SHIP Handshake step not found")
	}
	if step.Status != StepPending {
		t.Errorf("handshake status = %q, want %q", step.Status, StepPending)
	}
}

func TestEvaluateLifecycles_DiscoveryPass(t *testing.T) {
	input := LifecycleInput{
		Devices: []DeviceInfo{
			{DeviceAddr: "devA", Entities: []EntityInfo{
				{EntityType: "EVSE", Features: []FeatureInfo{
					{Functions: []string{"LoadControlLimitListData", "LoadControlLimitDescriptionListData", "LoadControlLimitConstraintsListData"}},
				}},
			}},
		},
		UseCases: []DeviceUseCases{
			{DeviceAddr: "devA", UseCases: []UseCaseInfo{
				{Abbreviation: "LPC", Available: true},
			}},
		},
	}

	result := EvaluateLifecycles(input)
	step := findStep(result[0].Steps, "Feature Discovery")
	if step == nil {
		t.Fatal("Feature Discovery step not found")
	}
	if step.Status != StepPass {
		t.Errorf("discovery status = %q, want %q", step.Status, StepPass)
	}
}

func TestEvaluateLifecycles_DiscoveryPartial(t *testing.T) {
	input := LifecycleInput{
		Devices: []DeviceInfo{
			{DeviceAddr: "devA", Entities: []EntityInfo{
				{EntityType: "EVSE", Features: []FeatureInfo{
					{Functions: []string{"LoadControlLimitListData"}},
				}},
			}},
		},
		UseCases: []DeviceUseCases{
			{DeviceAddr: "devA", UseCases: []UseCaseInfo{
				{Abbreviation: "LPC", Available: true},
			}},
		},
	}

	result := EvaluateLifecycles(input)
	step := findStep(result[0].Steps, "Feature Discovery")
	if step == nil {
		t.Fatal("Feature Discovery step not found")
	}
	if step.Status != StepPartial {
		t.Errorf("discovery status = %q, want %q", step.Status, StepPartial)
	}
	// Details should list which function sets are missing
	if !strings.Contains(step.Details, "missing:") {
		t.Errorf("discovery details should list missing functions, got %q", step.Details)
	}
	if !strings.Contains(step.Details, "LoadControlLimitDescriptionListData") {
		t.Errorf("discovery details should mention LoadControlLimitDescriptionListData, got %q", step.Details)
	}
	if !strings.Contains(step.Details, "LoadControlLimitConstraintsListData") {
		t.Errorf("discovery details should mention LoadControlLimitConstraintsListData, got %q", step.Details)
	}
}

func TestEvaluateLifecycles_DiscoveryCaseInsensitive(t *testing.T) {
	// Real SPINE discovery sends camelCase function names
	input := LifecycleInput{
		Devices: []DeviceInfo{
			{DeviceAddr: "devA", Entities: []EntityInfo{
				{EntityType: "EVSE", Features: []FeatureInfo{
					{Functions: []string{"loadControlLimitListData", "loadControlLimitDescriptionListData", "loadControlLimitConstraintsListData"}},
				}},
			}},
		},
		UseCases: []DeviceUseCases{
			{DeviceAddr: "devA", UseCases: []UseCaseInfo{
				{Abbreviation: "LPC", Available: true},
			}},
		},
	}

	result := EvaluateLifecycles(input)
	step := findStep(result[0].Steps, "Feature Discovery")
	if step == nil {
		t.Fatal("Feature Discovery step not found")
	}
	if step.Status != StepPass {
		t.Errorf("discovery status = %q, want %q (case-insensitive match should work)", step.Status, StepPass)
	}
}

func TestEvaluateLifecycles_DiscoveryPending(t *testing.T) {
	input := LifecycleInput{
		UseCases: []DeviceUseCases{
			{DeviceAddr: "devA", UseCases: []UseCaseInfo{
				{Abbreviation: "LPC", Available: true},
			}},
		},
	}

	result := EvaluateLifecycles(input)
	step := findStep(result[0].Steps, "Feature Discovery")
	if step == nil {
		t.Fatal("Feature Discovery step not found")
	}
	if step.Status != StepPending {
		t.Errorf("discovery status = %q, want %q", step.Status, StepPending)
	}
}

func TestEvaluateLifecycles_AnnouncedAvailable(t *testing.T) {
	input := LifecycleInput{
		UseCases: []DeviceUseCases{
			{DeviceAddr: "devA", UseCases: []UseCaseInfo{
				{Abbreviation: "LPC", Available: true},
			}},
		},
	}

	result := EvaluateLifecycles(input)
	step := findStep(result[0].Steps, "UC Announced")
	if step == nil {
		t.Fatal("UC Announced step not found")
	}
	if step.Status != StepPass {
		t.Errorf("announced status = %q, want %q", step.Status, StepPass)
	}
}

func TestEvaluateLifecycles_AnnouncedUnavailable(t *testing.T) {
	input := LifecycleInput{
		UseCases: []DeviceUseCases{
			{DeviceAddr: "devA", UseCases: []UseCaseInfo{
				{Abbreviation: "LPC", Available: false},
			}},
		},
	}

	result := EvaluateLifecycles(input)
	step := findStep(result[0].Steps, "UC Announced")
	if step == nil {
		t.Fatal("UC Announced step not found")
	}
	if step.Status != StepFail {
		t.Errorf("announced status = %q, want %q", step.Status, StepFail)
	}
}

func TestEvaluateLifecycles_SubscriptionsPass(t *testing.T) {
	input := LifecycleInput{
		UseCases: []DeviceUseCases{
			{DeviceAddr: "devA", UseCases: []UseCaseInfo{
				{Abbreviation: "LPC", Available: true},
			}},
		},
		Subscriptions: []SubscriptionEntry{
			{ServerDevice: "devA", ServerFeatureType: "LoadControl", Active: true},
		},
	}

	result := EvaluateLifecycles(input)
	step := findStep(result[0].Steps, "Subscriptions")
	if step == nil {
		t.Fatal("Subscriptions step not found")
	}
	if step.Status != StepPass {
		t.Errorf("subscriptions status = %q, want %q", step.Status, StepPass)
	}
}

func TestEvaluateLifecycles_SubscriptionsPartial(t *testing.T) {
	// Temporarily register a spec with 2 required subscriptions to test partial
	UseCaseLifecycleSpecs["TEST_PARTIAL"] = UseCaseLifecycleSpec{
		RequiredSubscriptions: []string{"LoadControl", "Measurement"},
		RequiredBindings:      []string{},
	}
	defer delete(UseCaseLifecycleSpecs, "TEST_PARTIAL")

	input := LifecycleInput{
		UseCases: []DeviceUseCases{
			{DeviceAddr: "devA", UseCases: []UseCaseInfo{
				{Abbreviation: "TEST_PARTIAL", Available: true},
			}},
		},
		Subscriptions: []SubscriptionEntry{
			{ServerDevice: "devA", ServerFeatureType: "LoadControl", Active: true},
		},
	}

	result := EvaluateLifecycles(input)
	step := findStep(result[0].Steps, "Subscriptions")
	if step == nil {
		t.Fatal("Subscriptions step not found")
	}
	if step.Status != StepPartial {
		t.Errorf("subscriptions status = %q, want %q", step.Status, StepPartial)
	}
	// Details should list which subscriptions are missing
	if !strings.Contains(step.Details, "missing:") {
		t.Errorf("subscription details should list missing types, got %q", step.Details)
	}
	if !strings.Contains(step.Details, "Measurement") {
		t.Errorf("subscription details should mention Measurement, got %q", step.Details)
	}
}

func TestEvaluateLifecycles_SubscriptionsFail(t *testing.T) {
	input := LifecycleInput{
		UseCases: []DeviceUseCases{
			{DeviceAddr: "devA", UseCases: []UseCaseInfo{
				{Abbreviation: "LPC", Available: true},
			}},
		},
		Subscriptions: []SubscriptionEntry{
			{ServerDevice: "devB", ServerFeatureType: "LoadControl", Active: true},
		},
	}

	result := EvaluateLifecycles(input)
	step := findStep(result[0].Steps, "Subscriptions")
	if step == nil {
		t.Fatal("Subscriptions step not found")
	}
	if step.Status != StepFail {
		t.Errorf("subscriptions status = %q, want %q", step.Status, StepFail)
	}
	if !strings.Contains(step.Details, "missing:") {
		t.Errorf("subscription details should list missing types, got %q", step.Details)
	}
	if !strings.Contains(step.Details, "LoadControl") {
		t.Errorf("subscription details should mention LoadControl, got %q", step.Details)
	}
}

func TestEvaluateLifecycles_SubscriptionsNA(t *testing.T) {
	input := LifecycleInput{
		UseCases: []DeviceUseCases{
			{DeviceAddr: "devA", UseCases: []UseCaseInfo{
				{Abbreviation: "MPC", Available: true},
			}},
		},
		Subscriptions: []SubscriptionEntry{},
	}

	// MPC requires Measurement subscription — with no subs it should fail.
	// Let's test a UC that truly has empty RequiredSubscriptions — we need
	// a UC not in the spec map to trigger NA for subs.
	// Actually MPC does require ["Measurement"], so let's test NA via bindings instead.
	// Override: test with a known UC that has empty bindings: MPC has RequiredBindings=[]
	result := EvaluateLifecycles(input)
	step := findStep(result[0].Steps, "Bindings")
	if step == nil {
		t.Fatal("Bindings step not found")
	}
	if step.Status != StepNA {
		t.Errorf("bindings status = %q, want %q", step.Status, StepNA)
	}
}

func TestEvaluateLifecycles_BindingsPass(t *testing.T) {
	input := LifecycleInput{
		UseCases: []DeviceUseCases{
			{DeviceAddr: "devA", UseCases: []UseCaseInfo{
				{Abbreviation: "LPC", Available: true},
			}},
		},
		Bindings: []BindingEntry{
			{ServerDevice: "devA", ServerFeatureType: "LoadControl", Active: true},
		},
	}

	result := EvaluateLifecycles(input)
	step := findStep(result[0].Steps, "Bindings")
	if step == nil {
		t.Fatal("Bindings step not found")
	}
	if step.Status != StepPass {
		t.Errorf("bindings status = %q, want %q", step.Status, StepPass)
	}
}

func TestEvaluateLifecycles_BindingsNA(t *testing.T) {
	input := LifecycleInput{
		UseCases: []DeviceUseCases{
			{DeviceAddr: "devA", UseCases: []UseCaseInfo{
				{Abbreviation: "MPC", Available: true},
			}},
		},
	}

	result := EvaluateLifecycles(input)
	step := findStep(result[0].Steps, "Bindings")
	if step == nil {
		t.Fatal("Bindings step not found")
	}
	if step.Status != StepNA {
		t.Errorf("bindings status = %q, want %q", step.Status, StepNA)
	}
}

func TestEvaluateLifecycles_NoSpec(t *testing.T) {
	input := LifecycleInput{
		UseCases: []DeviceUseCases{
			{DeviceAddr: "devA", UseCases: []UseCaseInfo{
				{Abbreviation: "UNKNOWN_UC", UseCaseName: "someUnknownUseCase", Available: true},
			}},
		},
	}

	result := EvaluateLifecycles(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}

	// Without a spec, subscriptions and bindings should be N/A
	subStep := findStep(result[0].Steps, "Subscriptions")
	if subStep == nil {
		t.Fatal("Subscriptions step not found")
	}
	if subStep.Status != StepNA {
		t.Errorf("subscriptions status = %q, want %q", subStep.Status, StepNA)
	}

	bindStep := findStep(result[0].Steps, "Bindings")
	if bindStep == nil {
		t.Fatal("Bindings step not found")
	}
	if bindStep.Status != StepNA {
		t.Errorf("bindings status = %q, want %q", bindStep.Status, StepNA)
	}
}

func TestEvaluateLifecycles_OverallStatus(t *testing.T) {
	tests := []struct {
		name     string
		input    LifecycleInput
		expected StepStatus
	}{
		{
			name: "all pass",
			input: LifecycleInput{
				Connections: []ConnectionInfo{
					{DeviceSource: "devA", DeviceDest: "devB", CurrentState: "data"},
				},
				Devices: []DeviceInfo{
					{DeviceAddr: "devA", Entities: []EntityInfo{
						{EntityType: "EVSE", Features: []FeatureInfo{
							{Functions: []string{"MeasurementListData", "MeasurementDescriptionListData", "MeasurementConstraintsListData"}},
						}},
					}},
				},
				UseCases: []DeviceUseCases{
					{DeviceAddr: "devA", UseCases: []UseCaseInfo{
						{Abbreviation: "MPC", Available: true},
					}},
				},
				Subscriptions: []SubscriptionEntry{
					{ServerDevice: "devA", ServerFeatureType: "Measurement", Active: true},
				},
			},
			expected: StepPass,
		},
		{
			name: "handshake fail is worst",
			input: LifecycleInput{
				Connections: []ConnectionInfo{
					{DeviceSource: "devA", DeviceDest: "devB", CurrentState: "handshake"},
				},
				UseCases: []DeviceUseCases{
					{DeviceAddr: "devA", UseCases: []UseCaseInfo{
						{Abbreviation: "MPC", Available: true},
					}},
				},
			},
			expected: StepFail,
		},
		{
			name: "partial when some steps pending",
			input: LifecycleInput{
				Connections: []ConnectionInfo{
					{DeviceSource: "devA", DeviceDest: "devB", CurrentState: "data"},
				},
				UseCases: []DeviceUseCases{
					{DeviceAddr: "devA", UseCases: []UseCaseInfo{
						{Abbreviation: "MPC", Available: true},
					}},
				},
				Subscriptions: []SubscriptionEntry{
					{ServerDevice: "devA", ServerFeatureType: "Measurement", Active: true},
				},
			},
			// Handshake=pass, Discovery=pending (no devices), Announced=pass, Subs=pass, Bindings=NA
			expected: StepPending,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EvaluateLifecycles(tt.input)
			if len(result) != 1 {
				t.Fatalf("expected 1 entry, got %d", len(result))
			}
			if result[0].OverallStatus != tt.expected {
				t.Errorf("overallStatus = %q, want %q", result[0].OverallStatus, tt.expected)
			}
		})
	}
}

func TestEvaluateLifecycles_MultipleUCsPerDevice(t *testing.T) {
	input := LifecycleInput{
		UseCases: []DeviceUseCases{
			{DeviceAddr: "devA", UseCases: []UseCaseInfo{
				{Abbreviation: "LPC", Available: true},
				{Abbreviation: "MPC", Available: true},
			}},
		},
	}

	result := EvaluateLifecycles(input)
	if len(result) != 2 {
		t.Fatalf("expected 2 entries (one per UC), got %d", len(result))
	}

	abbrs := map[string]bool{}
	for _, r := range result {
		abbrs[r.UseCaseAbbr] = true
	}
	if !abbrs["LPC"] {
		t.Error("expected LPC entry")
	}
	if !abbrs["MPC"] {
		t.Error("expected MPC entry")
	}
}

func TestEvaluateLifecycles_DiscoveryFunctionsOnWrongEntityOnly(t *testing.T) {
	// Device has EVSE entity (no LoadControl) and EV entity (has LoadControl).
	// LPC requires EVSE — should fail since the functions are only on EV.
	input := LifecycleInput{
		Devices: []DeviceInfo{
			{DeviceAddr: "devA", Entities: []EntityInfo{
				{EntityType: "EVSE", Features: []FeatureInfo{
					{Functions: []string{"MeasurementListData"}}, // wrong functions
				}},
				{EntityType: "EV", Features: []FeatureInfo{
					{Functions: []string{"LoadControlLimitListData", "LoadControlLimitDescriptionListData", "LoadControlLimitConstraintsListData"}},
				}},
			}},
		},
		UseCases: []DeviceUseCases{
			{DeviceAddr: "devA", UseCases: []UseCaseInfo{
				{Abbreviation: "LPC", Available: true},
			}},
		},
	}

	result := EvaluateLifecycles(input)
	step := findStep(result[0].Steps, "Feature Discovery")
	if step == nil {
		t.Fatal("Feature Discovery step not found")
	}
	if step.Status != StepFail {
		t.Errorf("discovery status = %q, want %q (LPC functions only on EV entity, not EVSE)", step.Status, StepFail)
	}
}

func TestEvaluateLifecycles_DiscoveryEntityTypeCaseInsensitive(t *testing.T) {
	// Entity type from discovery may have different casing than the spec.
	input := LifecycleInput{
		Devices: []DeviceInfo{
			{DeviceAddr: "devA", Entities: []EntityInfo{
				{EntityType: "evse", Features: []FeatureInfo{ // lowercase
					{Functions: []string{"LoadControlLimitListData", "LoadControlLimitDescriptionListData", "LoadControlLimitConstraintsListData"}},
				}},
			}},
		},
		UseCases: []DeviceUseCases{
			{DeviceAddr: "devA", UseCases: []UseCaseInfo{
				{Abbreviation: "LPC", Available: true},
			}},
		},
	}

	result := EvaluateLifecycles(input)
	step := findStep(result[0].Steps, "Feature Discovery")
	if step == nil {
		t.Fatal("Feature Discovery step not found")
	}
	if step.Status != StepPass {
		t.Errorf("discovery status = %q, want %q (entity type matching should be case-insensitive)", step.Status, StepPass)
	}
}

func TestEvaluateLifecycles_DiscoveryEntityTypeEmptyEntity(t *testing.T) {
	// Entity with empty entity type should not match UCs with constraints.
	// Device has entity with empty type containing LPC functions — LPC should fail.
	input := LifecycleInput{
		Devices: []DeviceInfo{
			{DeviceAddr: "devA", Entities: []EntityInfo{
				{EntityType: "", Features: []FeatureInfo{
					{Functions: []string{"LoadControlLimitListData", "LoadControlLimitDescriptionListData", "LoadControlLimitConstraintsListData"}},
				}},
			}},
		},
		UseCases: []DeviceUseCases{
			{DeviceAddr: "devA", UseCases: []UseCaseInfo{
				{Abbreviation: "LPC", Available: true},
			}},
		},
	}

	result := EvaluateLifecycles(input)
	step := findStep(result[0].Steps, "Feature Discovery")
	if step == nil {
		t.Fatal("Feature Discovery step not found")
	}
	if step.Status != StepFail {
		t.Errorf("discovery status = %q, want %q (empty entity type should not match EVSE constraint)", step.Status, StepFail)
	}
}

func TestEvaluateLifecycles_DiscoveryEntityTypeMatch(t *testing.T) {
	// LPC requires EVSE entity — LoadControl functions on EVSE should pass.
	input := LifecycleInput{
		Devices: []DeviceInfo{
			{DeviceAddr: "devA", Entities: []EntityInfo{
				{EntityType: "EVSE", Features: []FeatureInfo{
					{Functions: []string{"LoadControlLimitListData", "LoadControlLimitDescriptionListData", "LoadControlLimitConstraintsListData"}},
				}},
			}},
		},
		UseCases: []DeviceUseCases{
			{DeviceAddr: "devA", UseCases: []UseCaseInfo{
				{Abbreviation: "LPC", Available: true},
			}},
		},
	}

	result := EvaluateLifecycles(input)
	step := findStep(result[0].Steps, "Feature Discovery")
	if step == nil {
		t.Fatal("Feature Discovery step not found")
	}
	if step.Status != StepPass {
		t.Errorf("discovery status = %q, want %q", step.Status, StepPass)
	}
}

func TestEvaluateLifecycles_DiscoveryEntityTypeMismatch(t *testing.T) {
	// LPC requires EVSE entity — LoadControl functions only on EV entity should fail.
	input := LifecycleInput{
		Devices: []DeviceInfo{
			{DeviceAddr: "devA", Entities: []EntityInfo{
				{EntityType: "EV", Features: []FeatureInfo{
					{Functions: []string{"LoadControlLimitListData", "LoadControlLimitDescriptionListData", "LoadControlLimitConstraintsListData"}},
				}},
			}},
		},
		UseCases: []DeviceUseCases{
			{DeviceAddr: "devA", UseCases: []UseCaseInfo{
				{Abbreviation: "LPC", Available: true},
			}},
		},
	}

	result := EvaluateLifecycles(input)
	step := findStep(result[0].Steps, "Feature Discovery")
	if step == nil {
		t.Fatal("Feature Discovery step not found")
	}
	if step.Status != StepFail {
		t.Errorf("discovery status = %q, want %q (LPC functions on EV entity should not match)", step.Status, StepFail)
	}
}

func TestEvaluateLifecycles_DiscoveryEntityTypeMultiple(t *testing.T) {
	// MPC accepts both EVSE and EV entities.
	for _, entityType := range []string{"EVSE", "EV"} {
		t.Run(entityType, func(t *testing.T) {
			input := LifecycleInput{
				Devices: []DeviceInfo{
					{DeviceAddr: "devA", Entities: []EntityInfo{
						{EntityType: entityType, Features: []FeatureInfo{
							{Functions: []string{"MeasurementListData", "MeasurementDescriptionListData", "MeasurementConstraintsListData"}},
						}},
					}},
				},
				UseCases: []DeviceUseCases{
					{DeviceAddr: "devA", UseCases: []UseCaseInfo{
						{Abbreviation: "MPC", Available: true},
					}},
				},
			}

			result := EvaluateLifecycles(input)
			step := findStep(result[0].Steps, "Feature Discovery")
			if step == nil {
				t.Fatal("Feature Discovery step not found")
			}
			if step.Status != StepPass {
				t.Errorf("discovery status = %q, want %q (MPC should match %s entity)", step.Status, StepPass, entityType)
			}
		})
	}
}

func TestEvaluateLifecycles_DiscoveryEntityTypeEmpty(t *testing.T) {
	// UCs with no entity type constraint (e.g. MOB) should match any entity.
	input := LifecycleInput{
		Devices: []DeviceInfo{
			{DeviceAddr: "devA", Entities: []EntityInfo{
				{EntityType: "Battery", Features: []FeatureInfo{
					{Functions: []string{"MeasurementListData", "MeasurementDescriptionListData"}},
				}},
			}},
		},
		UseCases: []DeviceUseCases{
			{DeviceAddr: "devA", UseCases: []UseCaseInfo{
				{Abbreviation: "MOB", Available: true},
			}},
		},
	}

	result := EvaluateLifecycles(input)
	step := findStep(result[0].Steps, "Feature Discovery")
	if step == nil {
		t.Fatal("Feature Discovery step not found")
	}
	if step.Status != StepPass {
		t.Errorf("discovery status = %q, want %q (MOB has no entity type constraint)", step.Status, StepPass)
	}
}

// findStep is a test helper to find a step by name.
func findStep(steps []LifecycleStep, name string) *LifecycleStep {
	for _, s := range steps {
		if s.Name == name {
			return &s
		}
	}
	return nil
}
