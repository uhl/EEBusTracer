package api

import (
	"net/http"

	"github.com/eebustracer/eebustracer/internal/analysis"
	"github.com/eebustracer/eebustracer/internal/store"
)

func (s *Server) handleListUseCases(w http.ResponseWriter, r *http.Request) {
	traceID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid trace ID")
		return
	}

	filter := store.MessageFilter{
		FunctionSet: "NodeManagementUseCaseData",
		ShipMsgType: "data",
		Limit:       10000,
	}
	if device := r.URL.Query().Get("device"); device != "" {
		filter.Device = device
	}

	msgs, err := s.msgRepo.ListMessages(traceID, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := analysis.DetectUseCases(msgs)
	writeJSON(w, http.StatusOK, result)
}

// UseCaseContext describes a detected use case with its devices and function sets.
type UseCaseContext struct {
	Abbreviation string   `json:"abbreviation"`
	Name         string   `json:"name"`
	Devices      []string `json:"devices"`
	FunctionSets []string `json:"functionSets"`
}

func (s *Server) handleUseCaseContext(w http.ResponseWriter, r *http.Request) {
	traceID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid trace ID")
		return
	}

	filter := store.MessageFilter{
		FunctionSet: "NodeManagementUseCaseData",
		ShipMsgType: "data",
		Limit:       10000,
	}
	msgs, err := s.msgRepo.ListMessages(traceID, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	detected := analysis.DetectUseCases(msgs)

	// Build a map of abbreviation → {devices, name}
	type ucEntry struct {
		name    string
		devices map[string]bool
	}
	ucMap := map[string]*ucEntry{}
	for _, duc := range detected {
		for _, uc := range duc.UseCases {
			if !uc.Available {
				continue
			}
			abbr := uc.Abbreviation
			entry, ok := ucMap[abbr]
			if !ok {
				entry = &ucEntry{name: uc.UseCaseName, devices: map[string]bool{}}
				ucMap[abbr] = entry
			}
			entry.devices[duc.DeviceAddr] = true
		}
	}

	var result []UseCaseContext
	for abbr, entry := range ucMap {
		spec, ok := analysis.UseCaseFunctionSets[abbr]
		if !ok || len(spec.Functions) == 0 {
			continue
		}
		var devices []string
		for d := range entry.devices {
			devices = append(devices, d)
		}
		result = append(result, UseCaseContext{
			Abbreviation: abbr,
			Name:         entry.name,
			Devices:      devices,
			FunctionSets: spec.Functions,
		})
	}

	if result == nil {
		result = []UseCaseContext{}
	}
	writeJSON(w, http.StatusOK, result)
}
