package analysis

import (
	"testing"
)

func TestBuildDependencyTree_Empty(t *testing.T) {
	tree := BuildDependencyTree(nil, nil, nil, nil)

	if tree.Devices == nil {
		t.Error("expected non-nil devices slice")
	}
	if tree.Edges == nil {
		t.Error("expected non-nil edges slice")
	}
	if len(tree.Devices) != 0 {
		t.Errorf("expected 0 devices, got %d", len(tree.Devices))
	}
	if len(tree.Edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(tree.Edges))
	}
}

func TestBuildDependencyTree_DeviceTree(t *testing.T) {
	devices := []DeviceInfo{
		{
			DeviceAddr: "d:_i:19667_HEMS",
			Entities: []EntityInfo{
				{
					Address:    "[1]",
					EntityType: "DeviceInformation",
					Features: []FeatureInfo{
						{Address: "1", FeatureType: "LoadControlLimit", Role: "server", Functions: []string{"LoadControlLimitListData"}},
						{Address: "2", FeatureType: "Measurement", Role: "server", Functions: []string{"MeasurementListData"}},
					},
				},
				{
					Address:    "[1,1]",
					EntityType: "EV",
					Features: []FeatureInfo{
						{Address: "3", FeatureType: "DeviceClassification", Role: "client"},
					},
				},
			},
		},
	}

	tree := BuildDependencyTree(nil, devices, nil, nil)

	if len(tree.Devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(tree.Devices))
	}

	dev := tree.Devices[0]
	if dev.DeviceAddr != "d:_i:19667_HEMS" {
		t.Errorf("deviceAddr = %q, want %q", dev.DeviceAddr, "d:_i:19667_HEMS")
	}
	if dev.ShortName != "HEMS" {
		t.Errorf("shortName = %q, want %q", dev.ShortName, "HEMS")
	}
	if len(dev.Entities) != 2 {
		t.Fatalf("expected 2 entities, got %d", len(dev.Entities))
	}

	// First entity
	ent0 := dev.Entities[0]
	if ent0.EntityType != "DeviceInformation" {
		t.Errorf("entity[0].entityType = %q, want %q", ent0.EntityType, "DeviceInformation")
	}
	if len(ent0.Features) != 2 {
		t.Errorf("entity[0] features = %d, want 2", len(ent0.Features))
	}

	// Second entity
	ent1 := dev.Entities[1]
	if ent1.EntityType != "EV" {
		t.Errorf("entity[1].entityType = %q, want %q", ent1.EntityType, "EV")
	}
	if len(ent1.Features) != 1 {
		t.Errorf("entity[1] features = %d, want 1", len(ent1.Features))
	}

	// Feature details
	feat := ent0.Features[0]
	if feat.FeatureType != "LoadControlLimit" {
		t.Errorf("feature.featureType = %q, want %q", feat.FeatureType, "LoadControlLimit")
	}
	if feat.Role != "server" {
		t.Errorf("feature.role = %q, want %q", feat.Role, "server")
	}
}

func TestBuildDependencyTree_FeatureUseCases(t *testing.T) {
	useCases := []DeviceUseCases{
		{
			DeviceAddr: "devA",
			Actor:      "CEM",
			UseCases: []UseCaseInfo{
				{Abbreviation: "LPC", UseCaseName: "limitationOfPowerConsumption", Available: true},
			},
		},
	}

	devices := []DeviceInfo{
		{
			DeviceAddr: "devA",
			Entities: []EntityInfo{
				{
					Address:    "[1]",
					EntityType: "EVSE",
					Features: []FeatureInfo{
						{Address: "1", FeatureType: "LoadControlLimit", Role: "server", Functions: []string{"LoadControlLimitListData", "LoadControlLimitDescriptionListData"}},
					},
				},
			},
		},
	}

	tree := BuildDependencyTree(useCases, devices, nil, nil)

	if len(tree.Devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(tree.Devices))
	}

	feat := tree.Devices[0].Entities[0].Features[0]
	if len(feat.UseCases) == 0 {
		t.Fatal("expected at least one use case on feature")
	}

	found := false
	for _, uc := range feat.UseCases {
		if uc == "LPC" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected LPC in use cases, got %v", feat.UseCases)
	}
}

func TestBuildDependencyTree_NoFalseUseCases(t *testing.T) {
	useCases := []DeviceUseCases{
		{
			DeviceAddr: "devA",
			Actor:      "CEM",
			UseCases: []UseCaseInfo{
				{Abbreviation: "LPC", UseCaseName: "limitationOfPowerConsumption", Available: true},
			},
		},
	}

	devices := []DeviceInfo{
		{
			DeviceAddr: "devA",
			Entities: []EntityInfo{
				{
					Address:    "[1]",
					EntityType: "DeviceInformation",
					Features: []FeatureInfo{
						{Address: "1", FeatureType: "DeviceClassification", Role: "client", Functions: []string{"DeviceClassificationManufacturerData"}},
					},
				},
			},
		},
	}

	tree := BuildDependencyTree(useCases, devices, nil, nil)

	feat := tree.Devices[0].Entities[0].Features[0]
	if len(feat.UseCases) != 0 {
		t.Errorf("expected 0 use cases for non-matching feature, got %v", feat.UseCases)
	}
}

func TestBuildDependencyTree_Edges(t *testing.T) {
	devices := []DeviceInfo{
		{
			DeviceAddr: "devA",
			Entities: []EntityInfo{
				{Address: "[1]", EntityType: "CEM", Features: []FeatureInfo{
					{Address: "1", FeatureType: "NodeManagement", Role: "special"},
				}},
			},
		},
		{
			DeviceAddr: "devB",
			Entities: []EntityInfo{
				{Address: "[1]", EntityType: "EVSE", Features: []FeatureInfo{
					{Address: "1", FeatureType: "LoadControlLimit", Role: "server"},
				}},
			},
		},
	}

	tests := []struct {
		name         string
		subs         []SubscriptionEntry
		bindings     []BindingEntry
		wantSubCount int
		wantSubActive bool
		wantBindCount int
		wantBindActive bool
	}{
		{
			name: "active subscription",
			subs: []SubscriptionEntry{
				{ClientDevice: "devA", ClientFeature: "1.1", ServerDevice: "devB", ServerFeature: "1.1", Active: true},
			},
			wantSubCount: 1, wantSubActive: true,
			wantBindCount: 0,
		},
		{
			name: "active binding",
			bindings: []BindingEntry{
				{ClientDevice: "devA", ClientFeature: "1.1", ServerDevice: "devB", ServerFeature: "1.1", Active: true},
			},
			wantSubCount: 0,
			wantBindCount: 1, wantBindActive: true,
		},
		{
			name: "inactive subscription and binding",
			subs: []SubscriptionEntry{
				{ClientDevice: "devA", ClientFeature: "1.1", ServerDevice: "devB", ServerFeature: "1.1", Active: false},
			},
			bindings: []BindingEntry{
				{ClientDevice: "devA", ClientFeature: "1.1", ServerDevice: "devB", ServerFeature: "1.1", Active: false},
			},
			wantSubCount: 1, wantSubActive: false,
			wantBindCount: 1, wantBindActive: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree := BuildDependencyTree(nil, devices, tt.subs, tt.bindings)

			subEdges := filterDepEdgesByType(tree.Edges, "subscription")
			if len(subEdges) != tt.wantSubCount {
				t.Fatalf("subscription edges = %d, want %d", len(subEdges), tt.wantSubCount)
			}
			if tt.wantSubCount > 0 {
				if subEdges[0].Active != tt.wantSubActive {
					t.Errorf("subscription active = %v, want %v", subEdges[0].Active, tt.wantSubActive)
				}
				if subEdges[0].ClientDevice != "devA" {
					t.Errorf("clientDevice = %q, want %q", subEdges[0].ClientDevice, "devA")
				}
				if subEdges[0].ServerDevice != "devB" {
					t.Errorf("serverDevice = %q, want %q", subEdges[0].ServerDevice, "devB")
				}
			}

			bindEdges := filterDepEdgesByType(tree.Edges, "binding")
			if len(bindEdges) != tt.wantBindCount {
				t.Fatalf("binding edges = %d, want %d", len(bindEdges), tt.wantBindCount)
			}
			if tt.wantBindCount > 0 && bindEdges[0].Active != tt.wantBindActive {
				t.Errorf("binding active = %v, want %v", bindEdges[0].Active, tt.wantBindActive)
			}
		})
	}
}

func TestMatchesEntityType(t *testing.T) {
	tests := []struct {
		name       string
		entityType string
		allowed    []string
		want       bool
	}{
		{name: "empty allowed matches anything", entityType: "EVSE", allowed: nil, want: true},
		{name: "empty allowed matches empty entity", entityType: "", allowed: nil, want: true},
		{name: "exact match", entityType: "EVSE", allowed: []string{"EVSE"}, want: true},
		{name: "case insensitive match", entityType: "evse", allowed: []string{"EVSE"}, want: true},
		{name: "case insensitive match reversed", entityType: "EVSE", allowed: []string{"evse"}, want: true},
		{name: "no match", entityType: "CEM", allowed: []string{"EVSE", "EV"}, want: false},
		{name: "empty entity vs constraint", entityType: "", allowed: []string{"EVSE"}, want: false},
		{name: "one of multiple", entityType: "EV", allowed: []string{"EVSE", "EV"}, want: true},
		{name: "empty allowed slice matches", entityType: "anything", allowed: []string{}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesEntityType(tt.entityType, tt.allowed)
			if got != tt.want {
				t.Errorf("matchesEntityType(%q, %v) = %v, want %v", tt.entityType, tt.allowed, got, tt.want)
			}
		})
	}
}

func TestBuildDependencyTree_EntityTypeEmptyString(t *testing.T) {
	// Entity with empty entity type should NOT match UCs with entity type constraints
	// (e.g. root entity [0] with no entityType set).
	useCases := []DeviceUseCases{
		{
			DeviceAddr: "devA",
			Actor:      "CEM",
			UseCases: []UseCaseInfo{
				{Abbreviation: "LPC", Available: true},
				{Abbreviation: "MOB", Available: true}, // no entity type constraint
			},
		},
	}

	devices := []DeviceInfo{
		{
			DeviceAddr: "devA",
			Entities: []EntityInfo{
				{
					Address:    "[0]",
					EntityType: "", // empty — e.g. root entity without type
					Features: []FeatureInfo{
						{Address: "1", FeatureType: "LoadControl", Role: "server",
							Functions: []string{"LoadControlLimitListData", "LoadControlLimitDescriptionListData"}},
						{Address: "2", FeatureType: "Measurement", Role: "server",
							Functions: []string{"MeasurementListData", "MeasurementDescriptionListData"}},
					},
				},
			},
		},
	}

	tree := BuildDependencyTree(useCases, devices, nil, nil)

	lcFeat := tree.Devices[0].Entities[0].Features[0]
	for _, uc := range lcFeat.UseCases {
		if uc == "LPC" {
			t.Error("empty entity type should not match LPC (requires EVSE)")
		}
	}

	measFeat := tree.Devices[0].Entities[0].Features[1]
	hasMOB := false
	for _, uc := range measFeat.UseCases {
		if uc == "MOB" {
			hasMOB = true
		}
	}
	if !hasMOB {
		t.Errorf("MOB (no entity constraint) should match empty entity type, got %v", measFeat.UseCases)
	}
}

func TestBuildDependencyTree_EntityTypeCaseInsensitive(t *testing.T) {
	// Some implementations may report entity types in different casing.
	useCases := []DeviceUseCases{
		{
			DeviceAddr: "devA",
			Actor:      "CEM",
			UseCases: []UseCaseInfo{
				{Abbreviation: "LPC", Available: true},
			},
		},
	}

	devices := []DeviceInfo{
		{
			DeviceAddr: "devA",
			Entities: []EntityInfo{
				{
					Address:    "[1]",
					EntityType: "evse", // lowercase
					Features: []FeatureInfo{
						{Address: "1", FeatureType: "LoadControl", Role: "server",
							Functions: []string{"LoadControlLimitListData", "LoadControlLimitDescriptionListData"}},
					},
				},
			},
		},
	}

	tree := BuildDependencyTree(useCases, devices, nil, nil)

	feat := tree.Devices[0].Entities[0].Features[0]
	hasLPC := false
	for _, uc := range feat.UseCases {
		if uc == "LPC" {
			hasLPC = true
		}
	}
	if !hasLPC {
		t.Errorf("LPC should match entity type 'evse' (case-insensitive), got %v", feat.UseCases)
	}
}

func TestBuildDependencyTree_EntityTypeFiltering(t *testing.T) {
	// Both LPC and OPEV use LoadControl functions, but LPC lives on EVSE
	// entities and OPEV lives on EV entities.
	useCases := []DeviceUseCases{
		{
			DeviceAddr: "devA",
			Actor:      "CEM",
			UseCases: []UseCaseInfo{
				{Abbreviation: "LPC", Available: true},
				{Abbreviation: "OPEV", Available: true},
			},
		},
	}

	loadControlFuncs := []string{"LoadControlLimitListData", "LoadControlLimitDescriptionListData", "LoadControlLimitConstraintsListData"}

	devices := []DeviceInfo{
		{
			DeviceAddr: "devA",
			Entities: []EntityInfo{
				{
					Address:    "[1]",
					EntityType: "EVSE",
					Features: []FeatureInfo{
						{Address: "1", FeatureType: "LoadControl", Role: "server", Functions: loadControlFuncs},
					},
				},
				{
					Address:    "[1,1]",
					EntityType: "EV",
					Features: []FeatureInfo{
						{Address: "2", FeatureType: "LoadControl", Role: "server", Functions: loadControlFuncs},
					},
				},
			},
		},
	}

	tree := BuildDependencyTree(useCases, devices, nil, nil)

	if len(tree.Devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(tree.Devices))
	}

	// EVSE entity feature should get LPC but not OPEV
	evseFeat := tree.Devices[0].Entities[0].Features[0]
	evseUCs := map[string]bool{}
	for _, uc := range evseFeat.UseCases {
		evseUCs[uc] = true
	}
	if !evseUCs["LPC"] {
		t.Errorf("EVSE feature: expected LPC, got %v", evseFeat.UseCases)
	}
	if evseUCs["OPEV"] {
		t.Errorf("EVSE feature: should not have OPEV, got %v", evseFeat.UseCases)
	}

	// EV entity feature should get OPEV but not LPC
	evFeat := tree.Devices[0].Entities[1].Features[0]
	evUCs := map[string]bool{}
	for _, uc := range evFeat.UseCases {
		evUCs[uc] = true
	}
	if !evUCs["OPEV"] {
		t.Errorf("EV feature: expected OPEV, got %v", evFeat.UseCases)
	}
	if evUCs["LPC"] {
		t.Errorf("EV feature: should not have LPC, got %v", evFeat.UseCases)
	}
}

// --- helpers ---

func filterDepEdgesByType(edges []DepEdge, edgeType string) []DepEdge {
	var result []DepEdge
	for _, e := range edges {
		if e.EdgeType == edgeType {
			result = append(result, e)
		}
	}
	return result
}
