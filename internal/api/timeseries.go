package api

import (
	"encoding/json"
	"math"
	"net/http"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
	"github.com/eebustracer/eebustracer/internal/store"
)

// TimeseriesDataPoint represents a single data point.
type TimeseriesDataPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
	MessageID int64     `json:"messageId"`
	IsActive  *bool     `json:"isActive,omitempty"`
}

// TimeseriesSeries represents a named series of data points.
type TimeseriesSeries struct {
	ID         string                `json:"id"`
	Label      string                `json:"label"`
	Unit       string                `json:"unit,omitempty"`
	DataPoints []TimeseriesDataPoint `json:"dataPoints"`
}

// TimeseriesResponse is the JSON response for the timeseries API.
type TimeseriesResponse struct {
	Type   string             `json:"type"`
	Series []TimeseriesSeries `json:"series"`
}

// ExtractionDescriptor parameterizes the generic extraction of timeseries data
// from SPINE payloads. Each descriptor identifies which JSON keys to look for
// within the cmd array and how to find the ID and value fields.
type ExtractionDescriptor struct {
	CmdKey       string   // top-level cmd key, e.g. "measurementListData"
	DataArrayKey string   // nested array key, e.g. "measurementData"
	IDField      string   // ID field name, e.g. "measurementId"
	Classifiers  []string // accepted cmdClassifier values
	ActiveField  string   // optional boolean field, e.g. "isLimitActive"
}

// SeriesLabel provides label and unit for a timeseries series.
type SeriesLabel struct {
	Label string
	Unit  string
}

// GenericDataItem is a single extracted data point (ID + value).
type GenericDataItem struct {
	ID       string
	Value    float64
	IsActive *bool
}

// builtInDescriptors maps the known timeseries type names to their extraction descriptors.
var builtInDescriptors = map[string]ExtractionDescriptor{
	"measurement": {
		CmdKey:       "measurementListData",
		DataArrayKey: "measurementData",
		IDField:      "measurementId",
		Classifiers:  []string{"reply", "notify"},
	},
	"loadcontrol": {
		CmdKey:       "loadControlLimitListData",
		DataArrayKey: "loadControlLimitData",
		IDField:      "limitId",
		Classifiers:  []string{"reply", "notify", "write"},
		ActiveField:  "isLimitActive",
	},
	"setpoint": {
		CmdKey:       "setpointListData",
		DataArrayKey: "setpointData",
		IDField:      "setpointId",
		Classifiers:  []string{"reply", "notify"},
		ActiveField:  "isSetpointActive",
	},
}

// builtInFunctionSets maps the known type names to the FunctionSet filter value.
var builtInFunctionSets = map[string]string{
	"measurement": "MeasurementListData",
	"loadcontrol": "LoadControlLimitListData",
	"setpoint":    "SetpointListData",
}

func (s *Server) handleTimeseries(w http.ResponseWriter, r *http.Request) {
	traceID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid trace ID")
		return
	}

	q := r.URL.Query()
	dataType := q.Get("type")
	if dataType == "" {
		dataType = "measurement"
	}

	desc, ok := builtInDescriptors[dataType]
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown type: "+dataType)
		return
	}

	filter := store.MessageFilter{
		ShipMsgType: "data",
		Limit:       100000,
	}

	if v := q.Get("timeFrom"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.TimeFrom = &t
		}
	}
	if v := q.Get("timeTo"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.TimeTo = &t
		}
	}

	filter.FunctionSet = builtInFunctionSets[dataType]
	msgs, err := s.msgRepo.ListMessages(traceID, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Build labels from description context
	labels := s.buildLabelsForType(dataType, traceID)

	// Determine the filter ID parameter name based on type
	filterID := ""
	switch dataType {
	case "measurement":
		filterID = q.Get("measurementId")
	case "loadcontrol":
		filterID = q.Get("limitId")
	case "setpoint":
		filterID = q.Get("setpointId")
	}

	resp := TimeseriesResponse{
		Type:   dataType,
		Series: extractGenericSeries(msgs, desc, labels, filterID),
	}

	writeJSON(w, http.StatusOK, resp)
}

// buildLabelsForType loads description context and converts it to a generic label map.
func (s *Server) buildLabelsForType(dataType string, traceID int64) map[string]SeriesLabel {
	labels := make(map[string]SeriesLabel)

	switch dataType {
	case "measurement":
		descCtx := loadDescriptionContext(s, traceID)
		for id, m := range descCtx.Measurements {
			labels[id] = SeriesLabel{Label: m.Label, Unit: m.Unit}
		}
	case "loadcontrol":
		descCtx := loadDescriptionContext(s, traceID)
		for id, l := range descCtx.Limits {
			labels[id] = SeriesLabel{Label: l.Label, Unit: l.Unit}
		}
	case "setpoint":
		spLabels := loadSetpointDescriptions(s, traceID)
		for id, label := range spLabels {
			labels[id] = SeriesLabel{Label: label}
		}
	}

	return labels
}

// scaledNumberToFloat converts a SPINE ScaledNumber to a float64.
// ScaledNumber has {"number": N, "scale": S} meaning N * 10^S.
func scaledNumberToFloat(raw json.RawMessage) (float64, bool) {
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

// extractGenericData extracts ID/value pairs from a SPINE payload using the
// given ExtractionDescriptor. This is the single parameterized replacement for
// extractMeasurementData, extractLoadControlData, and extractSetpointData.
func extractGenericData(spinePayload json.RawMessage, desc ExtractionDescriptor) []GenericDataItem {
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

	var items []GenericDataItem
	for _, cmd := range dg.Datagram.Payload.Cmd {
		var cmdMap map[string]json.RawMessage
		if err := json.Unmarshal(cmd, &cmdMap); err != nil {
			continue
		}

		raw, ok := cmdMap[desc.CmdKey]
		if !ok {
			continue
		}

		// Parse the list data as a map to find the array key dynamically
		var listData map[string]json.RawMessage
		if err := json.Unmarshal(raw, &listData); err != nil {
			continue
		}

		arrayRaw, ok := listData[desc.DataArrayKey]
		if !ok {
			continue
		}

		// Parse the array as a slice of generic objects
		var dataArray []map[string]json.RawMessage
		if err := json.Unmarshal(arrayRaw, &dataArray); err != nil {
			continue
		}

		for _, entry := range dataArray {
			idRaw, ok := entry[desc.IDField]
			if !ok {
				continue
			}

			// Parse ID as json.Number to get a string representation
			var id json.Number
			if err := json.Unmarshal(idRaw, &id); err != nil {
				continue
			}

			valueRaw, ok := entry["value"]
			if !ok {
				continue
			}

			val, ok := scaledNumberToFloat(valueRaw)
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

// extractGenericSeries builds timeseries from messages using a generic extraction descriptor.
// labels provides label/unit enrichment by ID. filterID, if non-empty, limits to a single series.
func extractGenericSeries(msgs []*model.Message, desc ExtractionDescriptor, labels map[string]SeriesLabel, filterID string) []TimeseriesSeries {
	classifierSet := make(map[string]bool, len(desc.Classifiers))
	for _, c := range desc.Classifiers {
		classifierSet[c] = true
	}

	seriesMap := make(map[string]*TimeseriesSeries)
	seriesOrder := []string{}

	for _, msg := range msgs {
		if !classifierSet[msg.CmdClassifier] {
			continue
		}
		if len(msg.SpinePayload) == 0 {
			continue
		}

		dataList := extractGenericData(msg.SpinePayload, desc)
		for _, item := range dataList {
			if filterID != "" && item.ID != filterID {
				continue
			}

			series, ok := seriesMap[item.ID]
			if !ok {
				label := item.ID
				unit := ""
				if labels != nil {
					if sl, exists := labels[item.ID]; exists {
						label = sl.Label
						unit = sl.Unit
					}
				}
				series = &TimeseriesSeries{
					ID:    item.ID,
					Label: label,
					Unit:  unit,
				}
				seriesMap[item.ID] = series
				seriesOrder = append(seriesOrder, item.ID)
			}
			dp := TimeseriesDataPoint{
				Timestamp: msg.Timestamp,
				Value:     item.Value,
				MessageID: msg.ID,
			}
			if item.IsActive != nil {
				active := *item.IsActive
				dp.IsActive = &active
			}
			series.DataPoints = append(series.DataPoints, dp)
		}
	}

	result := make([]TimeseriesSeries, 0, len(seriesOrder))
	for _, id := range seriesOrder {
		result = append(result, *seriesMap[id])
	}
	return result
}

func loadSetpointDescriptions(s *Server, traceID int64) map[string]string {
	labels := make(map[string]string)

	descMsgs, err := s.msgRepo.ListMessages(traceID, store.MessageFilter{
		FunctionSet:   "SetpointDescriptionListData",
		CmdClassifier: "reply",
		ShipMsgType:   "data",
		Limit:         10,
	})
	if err != nil || len(descMsgs) == 0 {
		return labels
	}

	for _, msg := range descMsgs {
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

			raw, ok := cmdMap["setpointDescriptionListData"]
			if !ok {
				continue
			}

			var sdld struct {
				SetpointDescriptionData []struct {
					SetpointId   *json.Number `json:"setpointId"`
					SetpointType *string      `json:"setpointType"`
					Unit         *string      `json:"unit"`
				} `json:"setpointDescriptionData"`
			}
			if err := json.Unmarshal(raw, &sdld); err != nil {
				continue
			}

			for _, desc := range sdld.SetpointDescriptionData {
				if desc.SetpointId == nil {
					continue
				}
				id := desc.SetpointId.String()
				label := "Setpoint " + id
				if desc.SetpointType != nil {
					label = *desc.SetpointType
				}
				if desc.Unit != nil {
					label += " [" + *desc.Unit + "]"
				}
				labels[id] = label
			}
		}
	}

	return labels
}
