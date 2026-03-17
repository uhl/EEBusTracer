package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
)

func (s *Server) handleListTraces(w http.ResponseWriter, r *http.Request) {
	traces, err := s.traceRepo.ListTraces()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if traces == nil {
		traces = []*model.Trace{}
	}
	writeJSON(w, http.StatusOK, traces)
}

func (s *Server) handleCreateTrace(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Name == "" {
		req.Name = "Trace " + time.Now().Format("2006-01-02 15:04:05")
	}

	trace := &model.Trace{
		Name:        req.Name,
		Description: req.Description,
		StartedAt:   time.Now(),
		CreatedAt:   time.Now(),
	}
	if err := s.traceRepo.CreateTrace(trace); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, trace)
}

func (s *Server) handleGetTrace(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid trace ID")
		return
	}

	trace, err := s.traceRepo.GetTrace(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if trace == nil {
		writeError(w, http.StatusNotFound, "trace not found")
		return
	}
	writeJSON(w, http.StatusOK, trace)
}

func (s *Server) handleRenameTrace(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid trace ID")
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	trace, err := s.traceRepo.GetTrace(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if trace == nil {
		writeError(w, http.StatusNotFound, "trace not found")
		return
	}

	trace.Name = req.Name
	if err := s.traceRepo.UpdateTrace(trace); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, trace)
}

func (s *Server) handleDeleteTrace(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid trace ID")
		return
	}

	if err := s.traceRepo.DeleteTrace(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
