package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/eebustracer/eebustracer/internal/store"
)

// DeviceWithDiscovery extends a device with its entity/feature tree.
type DeviceWithDiscovery struct {
	ID         int64              `json:"id"`
	DeviceAddr string             `json:"deviceAddr"`
	SKI        string             `json:"ski,omitempty"`
	Brand      string             `json:"brand,omitempty"`
	Model      string             `json:"model,omitempty"`
	DeviceType string             `json:"deviceType,omitempty"`
	Entities   []EntityInfoResult `json:"entities,omitempty"`
}

// EntityInfoResult represents an entity in the device tree.
type EntityInfoResult struct {
	Address    string              `json:"address"`
	EntityType string              `json:"entityType,omitempty"`
	Features   []FeatureInfoResult `json:"features,omitempty"`
}

// FeatureInfoResult represents a feature in the device tree.
type FeatureInfoResult struct {
	Address     string   `json:"address"`
	FeatureType string   `json:"featureType,omitempty"`
	Role        string   `json:"role,omitempty"`
	Functions   []string `json:"functions,omitempty"`
}

func (s *Server) handleListDevices(w http.ResponseWriter, r *http.Request) {
	traceID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid trace ID")
		return
	}

	devices, err := s.deviceRepo.ListDevices(traceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Enrich with discovery data from discovery reply messages
	results := make([]DeviceWithDiscovery, len(devices))
	for i, d := range devices {
		results[i] = DeviceWithDiscovery{
			ID:         d.ID,
			DeviceAddr: d.DeviceAddr,
			SKI:        d.SKI,
			Brand:      d.Brand,
			Model:      d.Model,
			DeviceType: d.DeviceType,
		}

		// Find discovery reply messages for this device
		discoveryMsgs, err := s.msgRepo.ListMessages(traceID, store.MessageFilter{
			FunctionSet:   "NodeManagementDetailedDiscoveryData",
			CmdClassifier: "reply",
			DeviceSource:  d.DeviceAddr,
			Limit:         1,
		})
		if err != nil || len(discoveryMsgs) == 0 {
			continue
		}

		// Parse discovery data from spine payload
		if len(discoveryMsgs[0].SpinePayload) > 0 {
			entities := parseDiscoveryEntities(discoveryMsgs[0].SpinePayload)
			results[i].Entities = entities
		}
	}

	writeJSON(w, http.StatusOK, results)
}

func (s *Server) handleGetDevice(w http.ResponseWriter, r *http.Request) {
	traceID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid trace ID")
		return
	}
	deviceID, err := parseID(r, "did")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid device ID")
		return
	}

	device, err := s.deviceRepo.GetDevice(deviceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if device == nil {
		writeError(w, http.StatusNotFound, "device not found")
		return
	}

	result := DeviceWithDiscovery{
		ID:         device.ID,
		DeviceAddr: device.DeviceAddr,
		SKI:        device.SKI,
		Brand:      device.Brand,
		Model:      device.Model,
		DeviceType: device.DeviceType,
	}

	// Find discovery reply
	discoveryMsgs, err := s.msgRepo.ListMessages(traceID, store.MessageFilter{
		FunctionSet:   "NodeManagementDetailedDiscoveryData",
		CmdClassifier: "reply",
		DeviceSource:  device.DeviceAddr,
		Limit:         1,
	})
	if err == nil && len(discoveryMsgs) > 0 && len(discoveryMsgs[0].SpinePayload) > 0 {
		result.Entities = parseDiscoveryEntities(discoveryMsgs[0].SpinePayload)
	}

	writeJSON(w, http.StatusOK, result)
}

// parseDiscoveryEntities extracts entity/feature tree from a SPINE payload.
// This is a best-effort parser that extracts from the NodeManagementDetailedDiscoveryData structure.
func parseDiscoveryEntities(spinePayload json.RawMessage) []EntityInfoResult {
	// Parse the datagram structure to get to the cmd payload
	var dg struct {
		Datagram struct {
			Payload struct {
				Cmd []json.RawMessage `json:"cmd"`
			} `json:"payload"`
		} `json:"datagram"`
	}
	if err := json.Unmarshal(spinePayload, &dg); err != nil {
		return nil
	}

	for _, cmd := range dg.Datagram.Payload.Cmd {
		var cmdMap map[string]json.RawMessage
		if err := json.Unmarshal(cmd, &cmdMap); err != nil {
			continue
		}

		raw, ok := cmdMap["nodeManagementDetailedDiscoveryData"]
		if !ok {
			continue
		}

		// SPINE spec: supportedFunction is inside description
		// (NetworkManagementFeatureDescriptionDataType), not at the
		// featureInformation level.
		var discovery struct {
			EntityInformation []struct {
				Description *struct {
					EntityAddress *struct {
						Entity json.RawMessage `json:"entity"`
					} `json:"entityAddress"`
					EntityType  *string `json:"entityType"`
					Description *string `json:"description"`
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
					Description       *string `json:"description"`
					SupportedFunction []struct {
						Function           *string     `json:"function"`
						PossibleOperations interface{} `json:"possibleOperations"`
					} `json:"supportedFunction"`
				} `json:"description"`
			} `json:"featureInformation"`
		}
		if err := json.Unmarshal(raw, &discovery); err != nil {
			continue
		}

		// Build entity map
		entityMap := make(map[string]*EntityInfoResult)
		var entities []EntityInfoResult

		for _, ei := range discovery.EntityInformation {
			if ei.Description == nil || ei.Description.EntityAddress == nil {
				continue
			}
			addr := string(ei.Description.EntityAddress.Entity)
			entity := EntityInfoResult{Address: addr}
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
			feature := FeatureInfoResult{}
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
