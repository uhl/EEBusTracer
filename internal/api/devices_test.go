package api

import (
	"encoding/json"
	"testing"
)

func TestParseDiscoveryEntities_SupportedFunctionInsideDescription(t *testing.T) {
	// SPINE spec: supportedFunction is inside description
	// (NetworkManagementFeatureDescriptionDataType), not at the
	// featureInformation level.
	payload := json.RawMessage(`{
		"datagram":{"payload":{"cmd":[{"nodeManagementDetailedDiscoveryData":{
			"entityInformation":[
				{"description":{"entityAddress":{"entity":[1]},"entityType":"EVSE"}}
			],
			"featureInformation":[
				{"description":{
					"featureAddress":{"entity":[1],"feature":1},
					"featureType":"LoadControl",
					"role":"server",
					"supportedFunction":[
						{"function":"loadControlLimitListData"},
						{"function":"loadControlLimitDescriptionListData"}
					]
				}}
			]
		}}]}}
	}`)

	entities := parseDiscoveryEntities(payload)
	if len(entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(entities))
	}
	if len(entities[0].Features) != 1 {
		t.Fatalf("expected 1 feature, got %d", len(entities[0].Features))
	}

	feat := entities[0].Features[0]
	if len(feat.Functions) != 2 {
		t.Fatalf("expected 2 functions, got %d: %v", len(feat.Functions), feat.Functions)
	}
	if feat.Functions[0] != "loadControlLimitListData" {
		t.Errorf("function[0] = %q, want %q", feat.Functions[0], "loadControlLimitListData")
	}
	if feat.Functions[1] != "loadControlLimitDescriptionListData" {
		t.Errorf("function[1] = %q, want %q", feat.Functions[1], "loadControlLimitDescriptionListData")
	}
}

func TestParseDiscoveryEntities_MultipleFeaturesOnEntity(t *testing.T) {
	payload := json.RawMessage(`{
		"datagram":{"payload":{"cmd":[{"nodeManagementDetailedDiscoveryData":{
			"entityInformation":[
				{"description":{"entityAddress":{"entity":[1]},"entityType":"EVSE"}}
			],
			"featureInformation":[
				{"description":{
					"featureAddress":{"entity":[1],"feature":1},
					"featureType":"LoadControl",
					"role":"server",
					"supportedFunction":[{"function":"loadControlLimitListData"}]
				}},
				{"description":{
					"featureAddress":{"entity":[1],"feature":2},
					"featureType":"Measurement",
					"role":"server",
					"supportedFunction":[
						{"function":"measurementListData"},
						{"function":"measurementDescriptionListData"}
					]
				}}
			]
		}}]}}
	}`)

	entities := parseDiscoveryEntities(payload)
	if len(entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(entities))
	}

	evse := entities[0]
	if evse.EntityType != "EVSE" {
		t.Errorf("entity type = %q, want EVSE", evse.EntityType)
	}
	if len(evse.Features) != 2 {
		t.Fatalf("expected 2 features, got %d", len(evse.Features))
	}

	// First feature: LoadControl with 1 function
	if evse.Features[0].FeatureType != "LoadControl" {
		t.Errorf("feature[0] type = %q, want LoadControl", evse.Features[0].FeatureType)
	}
	if len(evse.Features[0].Functions) != 1 {
		t.Fatalf("feature[0]: expected 1 function, got %d", len(evse.Features[0].Functions))
	}
	if evse.Features[0].Functions[0] != "loadControlLimitListData" {
		t.Errorf("feature[0] function = %q, want loadControlLimitListData", evse.Features[0].Functions[0])
	}

	// Second feature: Measurement with 2 functions
	if evse.Features[1].FeatureType != "Measurement" {
		t.Errorf("feature[1] type = %q, want Measurement", evse.Features[1].FeatureType)
	}
	if len(evse.Features[1].Functions) != 2 {
		t.Fatalf("feature[1]: expected 2 functions, got %d", len(evse.Features[1].Functions))
	}
}

func TestParseDiscoveryEntities_NoSupportedFunction(t *testing.T) {
	// Features without supportedFunction should still be parsed (Functions stays nil)
	payload := json.RawMessage(`{
		"datagram":{"payload":{"cmd":[{"nodeManagementDetailedDiscoveryData":{
			"entityInformation":[
				{"description":{"entityAddress":{"entity":[0]},"entityType":"DeviceInformation"}}
			],
			"featureInformation":[
				{"description":{
					"featureAddress":{"entity":[0],"feature":0},
					"featureType":"NodeManagement",
					"role":"special"
				}}
			]
		}}]}}
	}`)

	entities := parseDiscoveryEntities(payload)
	if len(entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(entities))
	}
	if len(entities[0].Features) != 1 {
		t.Fatalf("expected 1 feature, got %d", len(entities[0].Features))
	}
	feat := entities[0].Features[0]
	if feat.FeatureType != "NodeManagement" {
		t.Errorf("featureType = %q, want NodeManagement", feat.FeatureType)
	}
	if len(feat.Functions) != 0 {
		t.Errorf("expected 0 functions, got %d", len(feat.Functions))
	}
}

func TestParseDiscoveryEntities_EmptyPayload(t *testing.T) {
	entities := parseDiscoveryEntities(json.RawMessage(`{}`))
	if entities != nil {
		t.Errorf("expected nil for empty payload, got %v", entities)
	}
}

func TestParseDiscoveryEntities_PreservesFeatureMetadata(t *testing.T) {
	payload := json.RawMessage(`{
		"datagram":{"payload":{"cmd":[{"nodeManagementDetailedDiscoveryData":{
			"entityInformation":[
				{"description":{"entityAddress":{"entity":[1]},"entityType":"CEM"}}
			],
			"featureInformation":[
				{"description":{
					"featureAddress":{"entity":[1],"feature":3},
					"featureType":"DeviceConfiguration",
					"role":"client",
					"supportedFunction":[
						{"function":"deviceConfigurationKeyValueListData"}
					]
				}}
			]
		}}]}}
	}`)

	entities := parseDiscoveryEntities(payload)
	if len(entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(entities))
	}

	feat := entities[0].Features[0]
	if feat.Address != "3" {
		t.Errorf("feature address = %q, want %q", feat.Address, "3")
	}
	if feat.FeatureType != "DeviceConfiguration" {
		t.Errorf("featureType = %q, want DeviceConfiguration", feat.FeatureType)
	}
	if feat.Role != "client" {
		t.Errorf("role = %q, want client", feat.Role)
	}
	if len(feat.Functions) != 1 || feat.Functions[0] != "deviceConfigurationKeyValueListData" {
		t.Errorf("functions = %v, want [deviceConfigurationKeyValueListData]", feat.Functions)
	}
}
