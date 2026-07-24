package api

import (
	"encoding/json"
	"net/http"

	"github.com/eebustracer/eebustracer/internal/spineparse"
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

// parseDiscoveryEntities extracts entity/feature tree from a SPINE payload
// (NodeManagementDetailedDiscoveryData). Delegates to spineparse so the same
// parser is reused by the CLI and analysis code.
func parseDiscoveryEntities(spinePayload json.RawMessage) []EntityInfoResult {
	src := spineparse.ParseDiscoveryEntities(spinePayload)
	if len(src) == 0 {
		return nil
	}
	out := make([]EntityInfoResult, len(src))
	for i, e := range src {
		out[i] = EntityInfoResult{Address: e.Address, EntityType: e.EntityType}
		for _, f := range e.Features {
			out[i].Features = append(out[i].Features, FeatureInfoResult{
				Address:     f.Address,
				FeatureType: f.FeatureType,
				Role:        f.Role,
				Functions:   f.Functions,
			})
		}
	}
	return out
}
