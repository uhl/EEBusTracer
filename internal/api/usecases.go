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
