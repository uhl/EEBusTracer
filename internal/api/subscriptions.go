package api

import (
	"net/http"
	"time"

	"github.com/eebustracer/eebustracer/internal/analysis"
	"github.com/eebustracer/eebustracer/internal/store"
)

func (s *Server) handleListSubscriptions(w http.ResponseWriter, r *http.Request) {
	traceID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid trace ID")
		return
	}

	staleThreshold := 5 * time.Minute
	if v := r.URL.Query().Get("staleThreshold"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			staleThreshold = d
		}
	}

	msgs, err := s.msgRepo.ListMessages(traceID, store.MessageFilter{
		ShipMsgType: "data",
		Limit:       100000,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := analysis.TrackSubscriptionsAndBindings(msgs, staleThreshold)
	writeJSON(w, http.StatusOK, result.Subscriptions)
}

func (s *Server) handleListBindings(w http.ResponseWriter, r *http.Request) {
	traceID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid trace ID")
		return
	}

	msgs, err := s.msgRepo.ListMessages(traceID, store.MessageFilter{
		ShipMsgType: "data",
		Limit:       100000,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := analysis.TrackSubscriptionsAndBindings(msgs, 0)
	writeJSON(w, http.StatusOK, result.Bindings)
}
