package api

import (
	"net/http"

	"github.com/eebustracer/eebustracer/internal/analysis"
	"github.com/eebustracer/eebustracer/internal/store"
)

func (s *Server) handleFlowParticipants(w http.ResponseWriter, r *http.Request) {
	traceID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid trace ID")
		return
	}

	summaries, err := s.msgRepo.ListMessageSummaries(traceID, store.MessageFilter{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	participants := analysis.ExtractFlowParticipants(summaries)
	writeJSON(w, http.StatusOK, participants)
}

func (s *Server) handleFlowCorrelations(w http.ResponseWriter, r *http.Request) {
	traceID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid trace ID")
		return
	}

	summaries, err := s.msgRepo.ListMessageSummaries(traceID, store.MessageFilter{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	pairs := analysis.BuildCorrelationPairs(summaries)
	writeJSON(w, http.StatusOK, pairs)
}
