package analysis

import "strings"

// DeviceTree represents a single device with its entity/feature hierarchy.
type DeviceTree struct {
	DeviceAddr string       `json:"deviceAddr"`
	ShortName  string       `json:"shortName"`
	Entities   []EntityTree `json:"entities"`
}

// EntityTree represents an entity within a device.
type EntityTree struct {
	Address    string        `json:"address"`
	EntityType string        `json:"entityType"`
	Features   []FeatureTree `json:"features"`
}

// FeatureTree represents a feature within an entity.
type FeatureTree struct {
	Address     string   `json:"address"`
	FeatureType string   `json:"featureType"`
	Role        string   `json:"role"`
	UseCases    []string `json:"useCases,omitempty"`
	Functions   []string `json:"functions,omitempty"`
}

// DepEdge represents a subscription or binding between features on different devices.
type DepEdge struct {
	ClientDevice  string `json:"clientDevice"`
	ClientFeature string `json:"clientFeature"`
	ServerDevice  string `json:"serverDevice"`
	ServerFeature string `json:"serverFeature"`
	EdgeType      string `json:"type"`
	Active        bool   `json:"active"`
}

// DependencyTree is the tree-structured response returned by the API.
type DependencyTree struct {
	Devices []DeviceTree `json:"devices"`
	Edges   []DepEdge    `json:"edges"`
}

// DeviceInfo describes a device with its entity/feature tree for graph building.
type DeviceInfo struct {
	DeviceAddr string
	Entities   []EntityInfo
}

// EntityInfo describes an entity within a device.
type EntityInfo struct {
	Address    string
	EntityType string
	Features   []FeatureInfo
}

// FeatureInfo describes a feature within an entity.
type FeatureInfo struct {
	Address     string
	FeatureType string
	Role        string
	Functions   []string
}

// BuildDependencyTree constructs a dependency tree from use cases, devices,
// subscriptions, and bindings.
func BuildDependencyTree(
	useCases []DeviceUseCases,
	devices []DeviceInfo,
	subscriptions []SubscriptionEntry,
	bindings []BindingEntry,
) DependencyTree {
	tree := DependencyTree{
		Devices: []DeviceTree{},
		Edges:   []DepEdge{},
	}

	// Collect all known UC abbreviations
	ucSet := map[string]bool{}
	for _, duc := range useCases {
		for _, uc := range duc.UseCases {
			ucSet[uc.Abbreviation] = true
		}
	}

	// Build device trees
	for _, dev := range devices {
		dt := DeviceTree{
			DeviceAddr: dev.DeviceAddr,
			ShortName:  shortDeviceAddr(dev.DeviceAddr),
			Entities:   []EntityTree{},
		}

		for _, ent := range dev.Entities {
			et := EntityTree{
				Address:    ent.Address,
				EntityType: ent.EntityType,
				Features:   []FeatureTree{},
			}

			for _, feat := range ent.Features {
				ft := FeatureTree{
					Address:     feat.Address,
					FeatureType: feat.FeatureType,
					Role:        feat.Role,
					Functions:   feat.Functions,
				}

				// Match feature functions against UC function sets
				for abbr := range ucSet {
					spec, ok := UseCaseFunctionSets[abbr]
					if !ok {
						continue
					}
					if !matchesEntityType(ent.EntityType, spec.EntityTypes) {
						continue
					}
					if hasIntersection(feat.Functions, spec.Functions) {
						ft.UseCases = append(ft.UseCases, abbr)
					}
				}

				et.Features = append(et.Features, ft)
			}

			dt.Entities = append(dt.Entities, et)
		}

		tree.Devices = append(tree.Devices, dt)
	}

	// Subscription edges
	for _, sub := range subscriptions {
		tree.Edges = append(tree.Edges, DepEdge{
			ClientDevice:  sub.ClientDevice,
			ClientFeature: sub.ClientFeature,
			ServerDevice:  sub.ServerDevice,
			ServerFeature: sub.ServerFeature,
			EdgeType:      "subscription",
			Active:        sub.Active,
		})
	}

	// Binding edges
	for _, b := range bindings {
		tree.Edges = append(tree.Edges, DepEdge{
			ClientDevice:  b.ClientDevice,
			ClientFeature: b.ClientFeature,
			ServerDevice:  b.ServerDevice,
			ServerFeature: b.ServerFeature,
			EdgeType:      "binding",
			Active:        b.Active,
		})
	}

	return tree
}

// shortDeviceAddr extracts a short name from a device address.
// e.g., "d:_i:37916_CEM-400000270" → "CEM-400000270"
func shortDeviceAddr(addr string) string {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == '_' {
			return addr[i+1:]
		}
	}
	return addr
}

// matchesEntityType returns true if the entity type matches one of the allowed
// types. An empty allowed list matches any entity type.
func matchesEntityType(entityType string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, a := range allowed {
		if strings.EqualFold(a, entityType) {
			return true
		}
	}
	return false
}

// hasIntersection returns true if any element in a appears in b (case-insensitive).
func hasIntersection(a, b []string) bool {
	set := make(map[string]bool, len(b))
	for _, s := range b {
		set[strings.ToLower(s)] = true
	}
	for _, s := range a {
		if set[strings.ToLower(s)] {
			return true
		}
	}
	return false
}
