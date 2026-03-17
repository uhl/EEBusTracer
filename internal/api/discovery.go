package api

import (
	"encoding/json"
	"net/http"

	"github.com/eebustracer/eebustracer/internal/store"
)

// DiscoveredSource describes a chartable data source found in a trace.
type DiscoveredSource struct {
	FunctionSet  string   `json:"functionSet"`
	CmdKey       string   `json:"cmdKey"`
	DataArrayKey string   `json:"dataArrayKey"`
	IDField      string   `json:"idField"`
	SampleIDs    []string `json:"sampleIds"`
	MessageCount int      `json:"messageCount"`
}

// DiscoveryResponse is the JSON response for the discover endpoint.
type DiscoveryResponse struct {
	Sources []DiscoveredSource `json:"sources"`
}

// discoverChartableData scans a trace to find function sets containing ScaledNumber values.
func discoverChartableData(s *Server, traceID int64) (*DiscoveryResponse, error) {
	resp := &DiscoveryResponse{
		Sources: []DiscoveredSource{},
	}

	fsets, err := s.msgRepo.ListDistinctFunctionSets(traceID)
	if err != nil {
		return nil, err
	}

	for _, fs := range fsets {
		// Sample up to 5 messages from this function set
		msgs, err := s.msgRepo.ListMessages(traceID, store.MessageFilter{
			FunctionSet: fs,
			ShipMsgType: "data",
			Limit:       5,
		})
		if err != nil {
			continue
		}

		// Count total messages for this function set
		allMsgs, err := s.msgRepo.ListMessages(traceID, store.MessageFilter{
			FunctionSet: fs,
			ShipMsgType: "data",
			Limit:       100000,
		})
		if err != nil {
			continue
		}
		msgCount := len(allMsgs)

		// Try to introspect each message's cmd array
		for _, msg := range msgs {
			if len(msg.SpinePayload) == 0 {
				continue
			}

			source := introspectPayload(msg.SpinePayload, fs, msgCount)
			if source != nil {
				resp.Sources = append(resp.Sources, *source)
				break // one match per function set is enough
			}
		}
	}

	return resp, nil
}

// introspectPayload examines a SPINE payload to detect chartable data (ScaledNumber values).
func introspectPayload(spinePayload json.RawMessage, functionSet string, msgCount int) *DiscoveredSource {
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

		for cmdKey, cmdValue := range cmdMap {
			// Skip cmdClassifier and other non-data keys
			if cmdKey == "cmdClassifier" || cmdKey == "function" {
				continue
			}

			// Try to parse as a map with array fields
			var listData map[string]json.RawMessage
			if err := json.Unmarshal(cmdValue, &listData); err != nil {
				continue
			}

			for arrayKey, arrayValue := range listData {
				// Try to parse as array of objects
				var dataArray []map[string]json.RawMessage
				if err := json.Unmarshal(arrayValue, &dataArray); err != nil {
					continue
				}

				if len(dataArray) == 0 {
					continue
				}

				// Check if items have an ID-like field and a value field with ScaledNumber
				idField, sampleIDs := findIDFieldAndSamples(dataArray)
				if idField == "" {
					continue
				}

				if !hasScaledNumberValues(dataArray) {
					continue
				}

				return &DiscoveredSource{
					FunctionSet:  functionSet,
					CmdKey:       cmdKey,
					DataArrayKey: arrayKey,
					IDField:      idField,
					SampleIDs:    sampleIDs,
					MessageCount: msgCount,
				}
			}
		}
	}

	return nil
}

// findIDFieldAndSamples looks for a field ending in "Id" or "id" in the data array items.
func findIDFieldAndSamples(dataArray []map[string]json.RawMessage) (string, []string) {
	// Check each field name for an ID-like pattern
	for key := range dataArray[0] {
		if !isIDField(key) {
			continue
		}

		// Collect sample IDs
		seen := make(map[string]bool)
		var sampleIDs []string
		for _, entry := range dataArray {
			raw, ok := entry[key]
			if !ok {
				continue
			}
			var id json.Number
			if err := json.Unmarshal(raw, &id); err != nil {
				continue
			}
			idStr := id.String()
			if !seen[idStr] {
				seen[idStr] = true
				sampleIDs = append(sampleIDs, idStr)
			}
		}

		if len(sampleIDs) > 0 {
			return key, sampleIDs
		}
	}
	return "", nil
}

// isIDField returns true if the field name looks like an ID field (ends with "Id" or "id").
func isIDField(name string) bool {
	if len(name) < 2 {
		return false
	}
	suffix := name[len(name)-2:]
	return suffix == "Id" || suffix == "id"
}

// hasScaledNumberValues checks if any item in the array has a "value" field containing
// a ScaledNumber ({"number": ..., "scale": ...}).
func hasScaledNumberValues(dataArray []map[string]json.RawMessage) bool {
	for _, entry := range dataArray {
		raw, ok := entry["value"]
		if !ok {
			continue
		}
		if _, ok := scaledNumberToFloat(raw); ok {
			return true
		}
	}
	return false
}

func (s *Server) handleDiscoverTimeseries(w http.ResponseWriter, r *http.Request) {
	traceID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid trace ID")
		return
	}

	resp, err := discoverChartableData(s, traceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}
