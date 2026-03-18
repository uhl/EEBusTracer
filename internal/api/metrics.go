package api

import (
	"net/http"

	"github.com/eebustracer/eebustracer/internal/analysis"
	"github.com/eebustracer/eebustracer/internal/store"
)

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	traceID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid trace ID")
		return
	}

	msgs, err := s.msgRepo.ListMessages(traceID, store.MessageFilter{Limit: 100000})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := analysis.ComputeHeartbeatMetrics(msgs)
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleMetricsExport(w http.ResponseWriter, r *http.Request) {
	traceID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid trace ID")
		return
	}

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}

	msgs, err := s.msgRepo.ListMessages(traceID, store.MessageFilter{Limit: 100000})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	metrics := analysis.ComputeHeartbeatMetrics(msgs)

	switch format {
	case "csv":
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=heartbeat-metrics.csv")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(analysis.FormatHeartbeatCSV(metrics.HeartbeatJitter)))
	default:
		writeJSON(w, http.StatusOK, metrics)
	}
}
