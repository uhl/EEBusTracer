package api

import (
	"encoding/json"
	"net/http"

	"github.com/eebustracer/eebustracer/internal/model"
)

func (s *Server) handleListBookmarks(w http.ResponseWriter, r *http.Request) {
	traceID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid trace ID")
		return
	}

	bookmarks, err := s.bookmarkRepo.List(traceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if bookmarks == nil {
		bookmarks = []*model.Bookmark{}
	}
	writeJSON(w, http.StatusOK, bookmarks)
}

func (s *Server) handleCreateBookmark(w http.ResponseWriter, r *http.Request) {
	traceID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid trace ID")
		return
	}

	var b model.Bookmark
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	b.TraceID = traceID
	if b.MessageID == 0 {
		writeError(w, http.StatusBadRequest, "messageId is required")
		return
	}

	if err := s.bookmarkRepo.Create(&b); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

func (s *Server) handleDeleteBookmark(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid bookmark ID")
		return
	}
	if err := s.bookmarkRepo.Delete(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
