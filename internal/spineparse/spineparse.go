// Package spineparse provides shared SPINE payload parsing primitives that
// can be reused by both the API layer and the analysis layer without creating
// a package-cycle. Each function operates on a raw SPINE payload and returns
// either decoded primitives or extracted data items.
package spineparse

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"
)

// DiscoveryEntity is a single entity extracted from a
// NodeManagementDetailedDiscoveryData payload.
type DiscoveryEntity struct {
	Address    string             `json:"address"`
	EntityType string             `json:"entityType,omitempty"`
	Features   []DiscoveryFeature `json:"features,omitempty"`
}

// DiscoveryFeature is a single feature attached to a DiscoveryEntity.
type DiscoveryFeature struct {
	Address     string   `json:"address"`
	FeatureType string   `json:"featureType,omitempty"`
	Role        string   `json:"role,omitempty"`
	Functions   []string `json:"functions,omitempty"`
}

// ParseDiscoveryEntities extracts the entity/feature tree from a SPINE
// payload containing a NodeManagementDetailedDiscoveryData reply/notify.
// Returns nil if the payload doesn't contain a parseable discovery command.
func ParseDiscoveryEntities(spinePayload json.RawMessage) []DiscoveryEntity {
	cmds, err := ExtractCmdArray(spinePayload)
	if err != nil {
		return nil
	}

	for _, cmd := range cmds {
		var cmdMap map[string]json.RawMessage
		if err := json.Unmarshal(cmd, &cmdMap); err != nil {
			continue
		}

		raw, ok := cmdMap["nodeManagementDetailedDiscoveryData"]
		if !ok {
			continue
		}

		var discovery struct {
			EntityInformation []struct {
				Description *struct {
					EntityAddress *struct {
						Entity json.RawMessage `json:"entity"`
					} `json:"entityAddress"`
					EntityType *string `json:"entityType"`
				} `json:"description"`
			} `json:"entityInformation"`
			FeatureInformation []struct {
				Description *struct {
					FeatureAddress *struct {
						Entity  json.RawMessage `json:"entity"`
						Feature *int            `json:"feature"`
					} `json:"featureAddress"`
					FeatureType       *string `json:"featureType"`
					Role              *string `json:"role"`
					SupportedFunction []struct {
						Function *string `json:"function"`
					} `json:"supportedFunction"`
				} `json:"description"`
			} `json:"featureInformation"`
		}
		if err := json.Unmarshal(raw, &discovery); err != nil {
			continue
		}

		entityMap := make(map[string]*DiscoveryEntity)
		var entities []DiscoveryEntity

		for _, ei := range discovery.EntityInformation {
			if ei.Description == nil || ei.Description.EntityAddress == nil {
				continue
			}
			addr := string(ei.Description.EntityAddress.Entity)
			entity := DiscoveryEntity{Address: addr}
			if ei.Description.EntityType != nil {
				entity.EntityType = *ei.Description.EntityType
			}
			entities = append(entities, entity)
			entityMap[addr] = &entities[len(entities)-1]
		}

		for _, fi := range discovery.FeatureInformation {
			if fi.Description == nil || fi.Description.FeatureAddress == nil {
				continue
			}
			feature := DiscoveryFeature{}
			if fi.Description.FeatureAddress.Feature != nil {
				feature.Address = strconv.Itoa(*fi.Description.FeatureAddress.Feature)
			}
			if fi.Description.FeatureType != nil {
				feature.FeatureType = *fi.Description.FeatureType
			}
			if fi.Description.Role != nil {
				feature.Role = *fi.Description.Role
			}
			for _, sf := range fi.Description.SupportedFunction {
				if sf.Function != nil {
					feature.Functions = append(feature.Functions, *sf.Function)
				}
			}
			if e, ok := entityMap[string(fi.Description.FeatureAddress.Entity)]; ok {
				e.Features = append(e.Features, feature)
			}
		}

		return entities
	}

	return nil
}

// ExtractionDescriptor parameterizes the generic extraction of data items from
// a SPINE payload. Each descriptor identifies which JSON keys to look for
// within the cmd array and how to find the ID and value fields.
type ExtractionDescriptor struct {
	CmdKey       string   // top-level cmd key, e.g. "measurementListData"
	DataArrayKey string   // nested array key, e.g. "measurementData"
	IDField      string   // ID field name, e.g. "measurementId"
	Classifiers  []string // accepted cmdClassifier values
	ActiveField  string   // optional boolean field, e.g. "isLimitActive"
}

// GenericDataItem is a single extracted data point (ID + value).
type GenericDataItem struct {
	ID       string
	Value    float64
	IsActive *bool
}

// ScaledNumberToFloat converts a SPINE ScaledNumber to a float64.
// ScaledNumber has {"number": N, "scale": S} meaning N * 10^S.
func ScaledNumberToFloat(raw json.RawMessage) (float64, bool) {
	var sn struct {
		Number *float64 `json:"number"`
		Scale  *int     `json:"scale"`
	}
	if err := json.Unmarshal(raw, &sn); err != nil || sn.Number == nil {
		return 0, false
	}
	scale := 0
	if sn.Scale != nil {
		scale = *sn.Scale
	}
	return *sn.Number * math.Pow(10, float64(scale)), true
}

// ExtractCmdArray extracts the cmd array from a SPINE datagram payload.
// It handles both array and single-object forms of the cmd field (EEBUS
// normalization may flatten single-element arrays to plain objects).
func ExtractCmdArray(spinePayload json.RawMessage) ([]json.RawMessage, error) {
	var dg struct {
		Datagram struct {
			Payload struct {
				Cmd []json.RawMessage `json:"cmd"`
			} `json:"payload"`
		} `json:"datagram"`
	}
	if err := json.Unmarshal(spinePayload, &dg); err == nil && len(dg.Datagram.Payload.Cmd) > 0 {
		return dg.Datagram.Payload.Cmd, nil
	}

	var dgSingle struct {
		Datagram struct {
			Payload struct {
				Cmd json.RawMessage `json:"cmd"`
			} `json:"payload"`
		} `json:"datagram"`
	}
	if err := json.Unmarshal(spinePayload, &dgSingle); err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSpace(dgSingle.Datagram.Payload.Cmd)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		return []json.RawMessage{dgSingle.Datagram.Payload.Cmd}, nil
	}

	return nil, nil
}

// ExtractGenericData extracts ID/value pairs from a SPINE payload using the
// given ExtractionDescriptor.
func ExtractGenericData(spinePayload json.RawMessage, desc ExtractionDescriptor) []GenericDataItem {
	cmds, err := ExtractCmdArray(spinePayload)
	if err != nil {
		return nil
	}

	var items []GenericDataItem
	for _, cmd := range cmds {
		var cmdMap map[string]json.RawMessage
		if err := json.Unmarshal(cmd, &cmdMap); err != nil {
			continue
		}

		raw, ok := cmdMap[desc.CmdKey]
		if !ok {
			continue
		}

		var listData map[string]json.RawMessage
		if err := json.Unmarshal(raw, &listData); err != nil {
			continue
		}

		arrayRaw, ok := listData[desc.DataArrayKey]
		if !ok {
			continue
		}

		var dataArray []map[string]json.RawMessage
		if err := json.Unmarshal(arrayRaw, &dataArray); err != nil {
			var single map[string]json.RawMessage
			if err := json.Unmarshal(arrayRaw, &single); err != nil {
				continue
			}
			dataArray = []map[string]json.RawMessage{single}
		}

		for _, entry := range dataArray {
			idRaw, ok := entry[desc.IDField]
			if !ok {
				continue
			}

			var id json.Number
			if err := json.Unmarshal(idRaw, &id); err != nil {
				continue
			}

			valueRaw, ok := entry["value"]
			if !ok {
				continue
			}

			val, ok := ScaledNumberToFloat(valueRaw)
			if !ok {
				continue
			}

			item := GenericDataItem{
				ID:    id.String(),
				Value: val,
			}

			if desc.ActiveField != "" {
				if activeRaw, exists := entry[desc.ActiveField]; exists {
					var active bool
					if err := json.Unmarshal(activeRaw, &active); err == nil {
						item.IsActive = &active
					}
				}
			}

			items = append(items, item)
		}
	}
	return items
}

// ExtractResultStatus parses the resultData.errorNumber from a SPINE payload.
// Returns "accepted" if errorNumber == 0 (or absent), "rejected" if non-zero,
// or "" if no resultData is present.
func ExtractResultStatus(spinePayload json.RawMessage) string {
	if len(spinePayload) == 0 {
		return ""
	}
	cmds, _ := ExtractCmdArray(spinePayload)
	for _, cmd := range cmds {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(cmd, &m); err != nil {
			continue
		}
		rd, ok := m["resultData"]
		if !ok {
			continue
		}
		var result struct {
			ErrorNumber *int `json:"errorNumber"`
		}
		if err := json.Unmarshal(rd, &result); err != nil {
			continue
		}
		if result.ErrorNumber == nil || *result.ErrorNumber == 0 {
			return "accepted"
		}
		return "rejected"
	}
	return ""
}
