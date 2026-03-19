package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
	"github.com/eebustracer/eebustracer/internal/store"
)

func (s *Server) handleListCharts(w http.ResponseWriter, r *http.Request) {
	traceID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid trace ID")
		return
	}

	charts, err := s.chartRepo.List(&traceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if charts == nil {
		charts = []*model.ChartDefinition{}
	}

	writeJSON(w, http.StatusOK, charts)
}

func (s *Server) handleCreateChart(w http.ResponseWriter, r *http.Request) {
	traceID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid trace ID")
		return
	}

	var req struct {
		Name      string `json:"name"`
		ChartType string `json:"chartType"`
		Sources   string `json:"sources"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.ChartType == "" {
		req.ChartType = "line"
	}
	if req.Sources == "" {
		req.Sources = "[]"
	}

	cd := &model.ChartDefinition{
		Name:      req.Name,
		TraceID:   &traceID,
		ChartType: req.ChartType,
		Sources:   req.Sources,
	}
	if err := s.chartRepo.Create(cd); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, cd)
}

func (s *Server) handleGetChart(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid chart ID")
		return
	}

	cd, err := s.chartRepo.Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if cd == nil {
		writeError(w, http.StatusNotFound, "chart not found")
		return
	}

	writeJSON(w, http.StatusOK, cd)
}

func (s *Server) handleUpdateChart(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid chart ID")
		return
	}

	cd, err := s.chartRepo.Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if cd == nil {
		writeError(w, http.StatusNotFound, "chart not found")
		return
	}

	var req struct {
		Name      *string `json:"name"`
		ChartType *string `json:"chartType"`
		Sources   *string `json:"sources"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.Name != nil {
		cd.Name = *req.Name
	}
	if req.ChartType != nil {
		cd.ChartType = *req.ChartType
	}
	if req.Sources != nil {
		cd.Sources = *req.Sources
	}

	if err := s.chartRepo.Update(cd); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, cd)
}

func (s *Server) handleDeleteChart(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid chart ID")
		return
	}

	if err := s.chartRepo.Delete(id); err != nil {
		if err.Error() == "cannot delete built-in chart" {
			writeError(w, http.StatusForbidden, err.Error())
			return
		}
		if err.Error() == "chart definition not found" {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleChartData(w http.ResponseWriter, r *http.Request) {
	traceID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid trace ID")
		return
	}

	chartID, err := parseID(r, "cid")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid chart ID")
		return
	}

	cd, err := s.chartRepo.Get(chartID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if cd == nil {
		writeError(w, http.StatusNotFound, "chart not found")
		return
	}

	var sources []model.ChartSource
	if err := json.Unmarshal([]byte(cd.Sources), &sources); err != nil {
		writeError(w, http.StatusInternalServerError, "invalid sources JSON")
		return
	}

	q := r.URL.Query()
	baseFilter := store.MessageFilter{
		ShipMsgType: "data",
		Limit:       100000,
	}
	if v := q.Get("timeFrom"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			baseFilter.TimeFrom = &t
		}
	}
	if v := q.Get("timeTo"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			baseFilter.TimeTo = &t
		}
	}

	// Build labels for built-in types
	var descCtx *DescriptionContext
	needsDesc := false
	for _, src := range sources {
		if src.FunctionSet == "MeasurementListData" || src.FunctionSet == "LoadControlLimitListData" {
			needsDesc = true
			break
		}
	}
	if needsDesc {
		descCtx = loadDescriptionContext(s, traceID)
	}

	var allSeries []TimeseriesSeries

	for _, src := range sources {
		filter := baseFilter
		filter.FunctionSet = src.FunctionSet

		msgs, err := s.msgRepo.ListMessages(traceID, filter)
		if err != nil {
			continue
		}

		desc := ExtractionDescriptor{
			CmdKey:       src.CmdKey,
			DataArrayKey: src.DataArrayKey,
			IDField:      src.IDField,
			Classifiers:  src.Classifiers,
			ActiveField:  activeFieldForFunctionSet(src.FunctionSet),
		}

		// Build labels
		labels := make(map[string]SeriesLabel)
		if descCtx != nil {
			switch src.FunctionSet {
			case "MeasurementListData":
				for id, m := range descCtx.Measurements {
					labels[id] = SeriesLabel{Label: m.Label, Unit: m.Unit}
				}
			case "LoadControlLimitListData":
				for id, l := range descCtx.Limits {
					labels[id] = SeriesLabel{Label: l.Label, Unit: l.Unit}
				}
			}
		}
		if src.FunctionSet == "SetpointListData" {
			spLabels := loadSetpointDescriptions(s, traceID)
			for id, label := range spLabels {
				labels[id] = SeriesLabel{Label: label}
			}
		}

		// Apply filterIDs if any
		filterID := ""
		if len(src.FilterIDs) == 1 {
			filterID = src.FilterIDs[0]
		}

		series := extractGenericSeries(msgs, desc, labels, filterID)

		// If multiple filterIDs, filter post-extraction
		if len(src.FilterIDs) > 1 {
			filterSet := make(map[string]bool)
			for _, fid := range src.FilterIDs {
				filterSet[fid] = true
			}
			var filtered []TimeseriesSeries
			for _, s := range series {
				if filterSet[s.ID] {
					filtered = append(filtered, s)
				}
			}
			series = filtered
		}

		allSeries = append(allSeries, series...)
	}

	resp := TimeseriesResponse{
		Type:   cd.ChartType,
		Series: allSeries,
	}
	if resp.Series == nil {
		resp.Series = []TimeseriesSeries{}
	}

	writeJSON(w, http.StatusOK, resp)
}

// activeFieldForFunctionSet returns the SPINE boolean field name that indicates
// whether a limit or setpoint is active, or "" if not applicable.
func activeFieldForFunctionSet(functionSet string) string {
	switch functionSet {
	case "LoadControlLimitListData":
		return "isLimitActive"
	case "SetpointListData":
		return "isSetpointActive"
	default:
		return ""
	}
}
