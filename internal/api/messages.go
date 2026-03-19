package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/eebustracer/eebustracer/internal/store"
)

func (s *Server) handleListMessages(w http.ResponseWriter, r *http.Request) {
	traceID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid trace ID")
		return
	}

	q := r.URL.Query()
	filter := store.MessageFilter{
		CmdClassifier: q.Get("cmdClassifier"),
		FunctionSet:   q.Get("functionSet"),
		Direction:     q.Get("direction"),
		ShipMsgType:   q.Get("shipMsgType"),
		Search:        q.Get("search"),
		DeviceSource:  q.Get("deviceSource"),
		DeviceDest:    q.Get("deviceDest"),
		Device:        q.Get("device"),
		EntitySource:  q.Get("entitySource"),
		EntityDest:    q.Get("entityDest"),
		FeatureSource: q.Get("featureSource"),
		FeatureDest:   q.Get("featureDest"),
	}
	if v := q.Get("limit"); v != "" {
		filter.Limit, _ = strconv.Atoi(v)
	}
	if v := q.Get("offset"); v != "" {
		filter.Offset, _ = strconv.Atoi(v)
	}
	if v := q.Get("timeFrom"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.TimeFrom = &t
		}
	}
	if v := q.Get("timeTo"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.TimeTo = &t
		}
	}

	messages, err := s.msgRepo.ListMessages(traceID, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	totalCount, err := s.msgRepo.CountFilteredMessages(traceID, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("X-Total-Count", strconv.Itoa(totalCount))

	unfilteredCount, err := s.msgRepo.CountMessages(traceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("X-Unfiltered-Count", strconv.Itoa(unfilteredCount))

	writeJSON(w, http.StatusOK, messages)
}

func (s *Server) handleGetMessage(w http.ResponseWriter, r *http.Request) {
	traceID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid trace ID")
		return
	}
	msgID, err := parseID(r, "mid")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid message ID")
		return
	}

	msg, err := s.msgRepo.GetMessage(traceID, msgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if msg == nil {
		writeError(w, http.StatusNotFound, "message not found")
		return
	}
	writeJSON(w, http.StatusOK, msg)
}
