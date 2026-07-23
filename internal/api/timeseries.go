package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
	"github.com/eebustracer/eebustracer/internal/spineparse"
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

// SeriesLabel provides label and unit for a timeseries series.
type SeriesLabel struct {
	Label string
	Unit  string
}

// builtInDescriptors maps the known timeseries type names to their extraction descriptors.
var builtInDescriptors = map[string]spineparse.ExtractionDescriptor{
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
		Classifiers:  []string{"reply", "notify", "write"},
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

// extractGenericSeries builds timeseries from messages using a generic extraction descriptor.
// labels provides label/unit enrichment by ID. filterID, if non-empty, limits to a single series.
func extractGenericSeries(msgs []*model.Message, desc spineparse.ExtractionDescriptor, labels map[string]SeriesLabel, filterID string) []TimeseriesSeries {
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

		dataList := spineparse.ExtractGenericData(msg.SpinePayload, desc)
		for _, item := range dataList {
			if filterID != "" && item.ID != filterID {
				continue
			}

			series, ok := seriesMap[item.ID]
			if !ok {
				label := item.ID
				unit := ""
				if sl, exists := labels[item.ID]; exists {
					label = sl.Label
					unit = sl.Unit
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

		cmds, err := spineparse.ExtractCmdArray(msg.SpinePayload)
		if err != nil {
			continue
		}

		for _, cmd := range cmds {
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
