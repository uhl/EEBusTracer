package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/eebustracer/eebustracer/internal/capture"
	"github.com/eebustracer/eebustracer/internal/model"
)

func (s *Server) handleCaptureStatus(w http.ResponseWriter, r *http.Request) {
	stats := s.engine.Stats()
	stats.SourceType = s.engine.SourceType()

	status := map[string]interface{}{
		"capturing":  s.engine.IsCapturing(),
		"stats":      stats,
		"traceId":    s.engine.TraceID(),
		"target":     s.engine.TargetAddr(),
		"sourceType": s.engine.SourceType(),
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleCaptureStart(w http.ResponseWriter, r *http.Request) {
	if s.engine.IsCapturing() {
		writeError(w, http.StatusConflict, "capture already in progress")
		return
	}

	var req struct {
		Host string `json:"host"`
		Port int    `json:"port"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Host == "" {
		writeError(w, http.StatusBadRequest, "host is required")
		return
	}
	if req.Port <= 0 {
		req.Port = 4712
	}
	if req.Name == "" {
		req.Name = "Capture " + time.Now().Format("2006-01-02 15:04:05")
	}

	target := fmt.Sprintf("%s:%d", req.Host, req.Port)

	trace := &model.Trace{
		Name:      req.Name,
		StartedAt: time.Now(),
		CreatedAt: time.Now(),
	}
	if err := s.traceRepo.CreateTrace(trace); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := s.engine.Start(trace.ID, target); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"traceId": trace.ID,
		"target":  target,
	})
}

func (s *Server) handleCaptureStartLogTail(w http.ResponseWriter, r *http.Request) {
	if s.engine.IsCapturing() {
		writeError(w, http.StatusConflict, "capture already in progress")
		return
	}

	var req struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}

	// Validate file exists and is readable.
	info, err := os.Stat(req.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, "file not accessible: "+err.Error())
		return
	}
	if info.IsDir() {
		writeError(w, http.StatusBadRequest, "path is a directory, not a file")
		return
	}

	if req.Name == "" {
		req.Name = "Log Tail " + time.Now().Format("2006-01-02 15:04:05")
	}

	trace := &model.Trace{
		Name:      req.Name,
		StartedAt: time.Now(),
		CreatedAt: time.Now(),
	}
	if err := s.traceRepo.CreateTrace(trace); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	src := capture.NewLogTailSource(req.Path, s.engine.Parser(), s.logger)
	if err := s.engine.StartWithSource(trace.ID, src, req.Path); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"traceId": trace.ID,
		"path":    req.Path,
	})
}

func (s *Server) handleCaptureStartTCP(w http.ResponseWriter, r *http.Request) {
	if s.engine.IsCapturing() {
		writeError(w, http.StatusConflict, "capture already in progress")
		return
	}

	var req struct {
		Host string `json:"host"`
		Port int    `json:"port"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Host == "" {
		writeError(w, http.StatusBadRequest, "host is required")
		return
	}
	if req.Port <= 0 {
		req.Port = 54546
	}
	if req.Name == "" {
		req.Name = "TCP Capture " + time.Now().Format("2006-01-02 15:04:05")
	}

	target := fmt.Sprintf("%s:%d", req.Host, req.Port)

	trace := &model.Trace{
		Name:      req.Name,
		StartedAt: time.Now(),
		CreatedAt: time.Now(),
	}
	if err := s.traceRepo.CreateTrace(trace); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	src := capture.NewTCPSource(target, s.engine.Parser(), s.logger)
	if err := s.engine.StartWithSource(trace.ID, src, target); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"traceId": trace.ID,
		"target":  target,
	})
}

func (s *Server) handleCaptureStop(w http.ResponseWriter, r *http.Request) {
	if !s.engine.IsCapturing() {
		writeError(w, http.StatusConflict, "no capture in progress")
		return
	}

	traceID := s.engine.TraceID()
	stats := s.engine.Stats()

	if err := s.engine.Stop(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Update trace with stop time and message count
	if err := s.traceRepo.StopTrace(traceID, time.Now(), int(stats.PacketsParsed)); err != nil {
		s.logger.Error("failed to update trace after stop", "error", err)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"traceId": traceID,
		"stats":   stats,
	})
}
