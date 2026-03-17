package api

import (
	"context"
	"net/http"

	"github.com/eebustracer/eebustracer/internal/mdns"
)

func (s *Server) handleMDNSDevices(w http.ResponseWriter, r *http.Request) {
	if s.mdnsMonitor == nil {
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}
	devices := s.mdnsMonitor.Devices()
	if len(devices) == 0 {
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}
	writeJSON(w, http.StatusOK, devices)
}

func (s *Server) handleMDNSStatus(w http.ResponseWriter, r *http.Request) {
	running := false
	deviceCount := 0
	if s.mdnsMonitor != nil {
		running = s.mdnsMonitor.IsRunning()
		deviceCount = len(s.mdnsMonitor.Devices())
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"running":     running,
		"deviceCount": deviceCount,
	})
}

func (s *Server) handleMDNSStart(w http.ResponseWriter, r *http.Request) {
	if s.mdnsMonitor == nil {
		writeError(w, http.StatusInternalServerError, "mDNS monitor not available")
		return
	}
	if s.mdnsMonitor.IsRunning() {
		writeError(w, http.StatusConflict, "mDNS monitor already running")
		return
	}

	// Register WS broadcast for device events.
	s.mdnsMonitor.OnDevice(func(d *mdns.DiscoveredDevice) {
		s.hub.BroadcastEvent("mdns_device", d)
	})

	if err := s.mdnsMonitor.Start(context.Background()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"running": true,
	})
}

func (s *Server) handleMDNSStop(w http.ResponseWriter, r *http.Request) {
	if s.mdnsMonitor == nil {
		writeError(w, http.StatusInternalServerError, "mDNS monitor not available")
		return
	}
	if !s.mdnsMonitor.IsRunning() {
		writeError(w, http.StatusConflict, "mDNS monitor not running")
		return
	}

	s.mdnsMonitor.Stop()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"running": false,
	})
}
