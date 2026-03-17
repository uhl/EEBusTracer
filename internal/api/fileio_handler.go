package api

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/eebustracer/eebustracer/internal/store"
)

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing file field")
		return
	}
	defer file.Close()

	name := strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename))
	trace, messages, err := store.ImportFileAutoDetect(file, name)

	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.traceRepo.CreateTrace(trace); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	for _, m := range messages {
		m.TraceID = trace.ID
	}
	if len(messages) > 0 {
		if err := s.msgRepo.InsertMessages(messages); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	writeJSON(w, http.StatusCreated, trace)
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	traceID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid trace ID")
		return
	}

	trace, err := s.traceRepo.GetTrace(traceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if trace == nil {
		writeError(w, http.StatusNotFound, "trace not found")
		return
	}

	messages, err := s.msgRepo.ListMessages(traceID, store.MessageFilter{Limit: 100000})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.eet"`, trace.Name))
	if err := store.ExportTrace(w, trace, messages); err != nil {
		s.logger.Error("export failed", "error", err)
	}
}
