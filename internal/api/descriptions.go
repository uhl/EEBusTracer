package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/eebustracer/eebustracer/internal/store"
)

// MeasurementDesc describes a measurement with enriched phase/scope context.
type MeasurementDesc struct {
	MeasurementID   string `json:"measurementId"`
	MeasurementType string `json:"measurementType"`
	Unit            string `json:"unit"`
	ScopeType       string `json:"scopeType"`
	Phase           string `json:"phase"`
	Label           string `json:"label"`
}

// LimitDesc describes a load control limit with enriched context.
type LimitDesc struct {
	LimitID         string `json:"limitId"`
	MeasurementID   string `json:"measurementId"`
	ScopeType       string `json:"scopeType"`
	LimitCategory   string `json:"limitCategory"`
	Unit            string `json:"unit"`
	Phase           string `json:"phase"`
	MeasurementType string `json:"measurementType"`
	Label           string `json:"label"`
}

// KeyValueDesc describes a device configuration key with its value type.
type KeyValueDesc struct {
	KeyID     string `json:"keyId"`
	KeyName   string `json:"keyName"`
	ValueType string `json:"valueType"`
	Unit      string `json:"unit,omitempty"`
}

// DescriptionContext holds enriched descriptions for measurements, limits,
// and device configuration keys.
type DescriptionContext struct {
	Measurements map[string]MeasurementDesc `json:"measurements"`
	Limits       map[string]LimitDesc       `json:"limits"`
	KeyValues    map[string]KeyValueDesc    `json:"keyValues"`
}

func phaseLabel(phase string) string {
	switch phase {
	case "a":
		return "Phase A"
	case "b":
		return "Phase B"
	case "c":
		return "Phase C"
	case "abc":
		return "Total"
	default:
		return phase
	}
}

func scopeTypeLabel(scope string) string {
	switch scope {
	case "overloadProtection":
		return "Overload Protection"
	case "selfConsumption":
		return "Self Consumption"
	case "discharge":
		return "Discharge"
	default:
		return scope
	}
}

func measurementTypeLabel(mtype string) string {
	switch mtype {
	case "current":
		return "Current"
	case "power":
		return "Power"
	case "energy":
		return "Energy"
	case "voltage":
		return "Voltage"
	default:
		return mtype
	}
}

func buildMeasurementLabel(desc MeasurementDesc) string {
	parts := []string{}
	if desc.MeasurementType != "" {
		parts = append(parts, measurementTypeLabel(desc.MeasurementType))
	}
	if desc.Phase != "" {
		parts = append(parts, desc.Phase)
	}
	label := strings.Join(parts, " ")
	if desc.Unit != "" {
		label += " [" + desc.Unit + "]"
	}
	return label
}

func buildLimitLabel(desc LimitDesc) string {
	parts := []string{}
	if desc.ScopeType != "" {
		parts = append(parts, scopeTypeLabel(desc.ScopeType))
	}
	if desc.Phase != "" {
		parts = append(parts, desc.Phase)
	}
	label := strings.Join(parts, " ")
	if label == "" {
		return "Limit " + desc.LimitID
	}
	if desc.Unit != "" {
		label += " [" + desc.Unit + "]"
	}
	return label
}

func loadDescriptionContext(s *Server, traceID int64) *DescriptionContext {
	ctx := &DescriptionContext{
		Measurements: make(map[string]MeasurementDesc),
		Limits:       make(map[string]LimitDesc),
		KeyValues:    make(map[string]KeyValueDesc),
	}

	// Step 1: Load electrical connection parameter descriptions → measurementId → phase
	phaseMap := loadPhaseMap(s, traceID)

	// Step 2: Load measurement descriptions
	loadMeasurementDescs(s, traceID, phaseMap, ctx)

	// Step 3: Load load control limit descriptions
	loadLimitDescs(s, traceID, phaseMap, ctx)

	// Step 4: Load device configuration key/value descriptions
	loadDeviceConfigDescs(s, traceID, ctx)

	return ctx
}

func loadPhaseMap(s *Server, traceID int64) map[string]string {
	phases := make(map[string]string)

	msgs, err := s.msgRepo.ListMessages(traceID, store.MessageFilter{
		FunctionSet:   "ElectricalConnectionParameterDescriptionListData",
		CmdClassifier: "reply",
		ShipMsgType:   "data",
		Limit:         10,
	})
	if err != nil || len(msgs) == 0 {
		return phases
	}

	for _, msg := range msgs {
		if len(msg.SpinePayload) == 0 {
			continue
		}

		var dg struct {
			Datagram struct {
				Payload struct {
					Cmd []json.RawMessage `json:"cmd"`
				} `json:"payload"`
			} `json:"datagram"`
		}
		if err := json.Unmarshal(msg.SpinePayload, &dg); err != nil {
			continue
		}

		for _, cmd := range dg.Datagram.Payload.Cmd {
			var cmdMap map[string]json.RawMessage
			if err := json.Unmarshal(cmd, &cmdMap); err != nil {
				continue
			}

			raw, ok := cmdMap["electricalConnectionParameterDescriptionListData"]
			if !ok {
				continue
			}

			var ecpdl struct {
				Data []struct {
					MeasurementId   *json.Number `json:"measurementId"`
					AcMeasuredPhases *string      `json:"acMeasuredPhases"`
				} `json:"electricalConnectionParameterDescriptionData"`
			}
			if err := json.Unmarshal(raw, &ecpdl); err != nil {
				continue
			}

			for _, d := range ecpdl.Data {
				if d.MeasurementId != nil && d.AcMeasuredPhases != nil {
					phases[d.MeasurementId.String()] = *d.AcMeasuredPhases
				}
			}
		}
	}

	return phases
}

func loadMeasurementDescs(s *Server, traceID int64, phaseMap map[string]string, ctx *DescriptionContext) {
	msgs, err := s.msgRepo.ListMessages(traceID, store.MessageFilter{
		FunctionSet:   "MeasurementDescriptionListData",
		CmdClassifier: "reply",
		ShipMsgType:   "data",
		Limit:         10,
	})
	if err != nil || len(msgs) == 0 {
		return
	}

	for _, msg := range msgs {
		if len(msg.SpinePayload) == 0 {
			continue
		}

		var dg struct {
			Datagram struct {
				Payload struct {
					Cmd []json.RawMessage `json:"cmd"`
				} `json:"payload"`
			} `json:"datagram"`
		}
		if err := json.Unmarshal(msg.SpinePayload, &dg); err != nil {
			continue
		}

		for _, cmd := range dg.Datagram.Payload.Cmd {
			var cmdMap map[string]json.RawMessage
			if err := json.Unmarshal(cmd, &cmdMap); err != nil {
				continue
			}

			raw, ok := cmdMap["measurementDescriptionListData"]
			if !ok {
				continue
			}

			var mdld struct {
				Data []struct {
					MeasurementId   *json.Number `json:"measurementId"`
					MeasurementType *string      `json:"measurementType"`
					Unit            *string      `json:"unit"`
					ScopeType       *string      `json:"scopeType"`
				} `json:"measurementDescriptionData"`
			}
			if err := json.Unmarshal(raw, &mdld); err != nil {
				continue
			}

			for _, d := range mdld.Data {
				if d.MeasurementId == nil {
					continue
				}
				id := d.MeasurementId.String()
				desc := MeasurementDesc{
					MeasurementID: id,
				}
				if d.MeasurementType != nil {
					desc.MeasurementType = *d.MeasurementType
				}
				if d.Unit != nil {
					desc.Unit = *d.Unit
				}
				if d.ScopeType != nil {
					desc.ScopeType = *d.ScopeType
				}
				if rawPhase, ok := phaseMap[id]; ok {
					desc.Phase = phaseLabel(rawPhase)
				}
				desc.Label = buildMeasurementLabel(desc)
				ctx.Measurements[id] = desc
			}
		}
	}
}

func loadLimitDescs(s *Server, traceID int64, phaseMap map[string]string, ctx *DescriptionContext) {
	msgs, err := s.msgRepo.ListMessages(traceID, store.MessageFilter{
		FunctionSet:   "LoadControlLimitDescriptionListData",
		CmdClassifier: "reply",
		ShipMsgType:   "data",
		Limit:         10,
	})
	if err != nil || len(msgs) == 0 {
		return
	}

	for _, msg := range msgs {
		if len(msg.SpinePayload) == 0 {
			continue
		}

		var dg struct {
			Datagram struct {
				Payload struct {
					Cmd []json.RawMessage `json:"cmd"`
				} `json:"payload"`
			} `json:"datagram"`
		}
		if err := json.Unmarshal(msg.SpinePayload, &dg); err != nil {
			continue
		}

		for _, cmd := range dg.Datagram.Payload.Cmd {
			var cmdMap map[string]json.RawMessage
			if err := json.Unmarshal(cmd, &cmdMap); err != nil {
				continue
			}

			raw, ok := cmdMap["loadControlLimitDescriptionListData"]
			if !ok {
				continue
			}

			var lcdld struct {
				Data []struct {
					LimitId       *json.Number `json:"limitId"`
					MeasurementId *json.Number `json:"measurementId"`
					LimitCategory *string      `json:"limitCategory"`
					ScopeType     *string      `json:"scopeType"`
					Unit          *string      `json:"unit"`
				} `json:"loadControlLimitDescriptionData"`
			}
			if err := json.Unmarshal(raw, &lcdld); err != nil {
				continue
			}

			for _, d := range lcdld.Data {
				if d.LimitId == nil {
					continue
				}
				limitID := d.LimitId.String()
				desc := LimitDesc{
					LimitID: limitID,
				}
				if d.MeasurementId != nil {
					desc.MeasurementID = d.MeasurementId.String()
				}
				if d.ScopeType != nil {
					desc.ScopeType = *d.ScopeType
				}
				if d.LimitCategory != nil {
					desc.LimitCategory = *d.LimitCategory
				}
				if d.Unit != nil {
					desc.Unit = *d.Unit
				}

				// Enrich from measurement's phase via measurementId
				if desc.MeasurementID != "" {
					if rawPhase, ok := phaseMap[desc.MeasurementID]; ok {
						desc.Phase = phaseLabel(rawPhase)
					}
					// Also grab measurement type from context
					if m, ok := ctx.Measurements[desc.MeasurementID]; ok {
						desc.MeasurementType = m.MeasurementType
					}
				}

				desc.Label = buildLimitLabel(desc)
				ctx.Limits[limitID] = desc
			}
		}
	}
}

func loadDeviceConfigDescs(s *Server, traceID int64, ctx *DescriptionContext) {
	msgs, err := s.msgRepo.ListMessages(traceID, store.MessageFilter{
		FunctionSet:   "DeviceConfigurationKeyValueDescriptionListData",
		CmdClassifier: "reply",
		ShipMsgType:   "data",
		Limit:         10,
	})
	if err != nil || len(msgs) == 0 {
		return
	}

	for _, msg := range msgs {
		if len(msg.SpinePayload) == 0 {
			continue
		}

		var dg struct {
			Datagram struct {
				Payload struct {
					Cmd []json.RawMessage `json:"cmd"`
				} `json:"payload"`
			} `json:"datagram"`
		}
		if err := json.Unmarshal(msg.SpinePayload, &dg); err != nil {
			continue
		}

		for _, cmd := range dg.Datagram.Payload.Cmd {
			var cmdMap map[string]json.RawMessage
			if err := json.Unmarshal(cmd, &cmdMap); err != nil {
				continue
			}

			raw, ok := cmdMap["deviceConfigurationKeyValueDescriptionListData"]
			if !ok {
				continue
			}

			var dckv struct {
				Data []struct {
					KeyID     *json.Number `json:"keyId"`
					KeyName   *string      `json:"keyName"`
					ValueType *string      `json:"valueType"`
					Unit      *string      `json:"unit"`
				} `json:"deviceConfigurationKeyValueDescriptionData"`
			}
			if err := json.Unmarshal(raw, &dckv); err != nil {
				continue
			}

			for _, d := range dckv.Data {
				if d.KeyID == nil {
					continue
				}
				id := d.KeyID.String()
				desc := KeyValueDesc{KeyID: id}
				if d.KeyName != nil {
					desc.KeyName = *d.KeyName
				}
				if d.ValueType != nil {
					desc.ValueType = *d.ValueType
				}
				if d.Unit != nil {
					desc.Unit = *d.Unit
				}
				ctx.KeyValues[id] = desc
			}
		}
	}
}

func (s *Server) handleDescriptions(w http.ResponseWriter, r *http.Request) {
	traceID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid trace ID")
		return
	}

	ctx := loadDescriptionContext(s, traceID)
	writeJSON(w, http.StatusOK, ctx)
}
