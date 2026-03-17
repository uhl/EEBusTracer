package api

import (
	"encoding/json"
	"net/http"

	"github.com/eebustracer/eebustracer/internal/model"
)

func (s *Server) handleListPresets(w http.ResponseWriter, r *http.Request) {
	presets, err := s.presetRepo.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if presets == nil {
		presets = []*model.FilterPreset{}
	}
	writeJSON(w, http.StatusOK, presets)
}

func (s *Server) handleCreatePreset(w http.ResponseWriter, r *http.Request) {
	var p model.FilterPreset
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if p.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := s.presetRepo.Create(&p); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handleDeletePreset(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid preset ID")
		return
	}
	if err := s.presetRepo.Delete(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
