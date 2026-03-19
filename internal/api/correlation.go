package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/eebustracer/eebustracer/internal/model"
)

// RelatedMessage describes a message related to another via correlation.
type RelatedMessage struct {
	Message      *model.Message `json:"message"`
	Relationship string         `json:"relationship"`
	LatencyMs    *float64       `json:"latencyMs,omitempty"`
	ResultStatus string         `json:"resultStatus,omitempty"`
}

// ConversationResponse is the JSON envelope for the conversation endpoint.
type ConversationResponse struct {
	Messages []*model.Message `json:"messages"`
	Total    int              `json:"total"`
}

func (s *Server) handleRelatedMessages(w http.ResponseWriter, r *http.Request) {
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

	var related []RelatedMessage

	// If this message has a msgCounter, find responses referencing it
	if msg.MsgCounter != "" {
		refs, err := s.msgRepo.FindByMsgCounterRef(traceID, msg.MsgCounter)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		for _, ref := range refs {
			rel := classifyRelationship(msg, ref)
			latency := computeLatencyMs(msg, ref)
			status := extractResultStatus(ref.SpinePayload)
			related = append(related, RelatedMessage{
				Message:      ref,
				Relationship: rel,
				LatencyMs:    latency,
				ResultStatus: status,
			})
		}
	}

	// If this message has a msgCounterRef, find the original request
	if msg.MsgCounterRef != "" {
		originals, err := s.msgRepo.FindByMsgCounter(traceID, msg.MsgCounterRef)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		for _, orig := range originals {
			rel := classifyRelationship(orig, msg)
			latency := computeLatencyMs(orig, msg)
			status := extractResultStatus(msg.SpinePayload)
			related = append(related, RelatedMessage{
				Message:      orig,
				Relationship: rel,
				LatencyMs:    latency,
				ResultStatus: status,
			})
		}
	}

	if related == nil {
		related = []RelatedMessage{}
	}
	writeJSON(w, http.StatusOK, related)
}

func (s *Server) handleConversation(w http.ResponseWriter, r *http.Request) {
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

	// Only SPINE data messages with function set and device addresses qualify
	if msg.ShipMsgType != model.ShipMsgTypeData || msg.FunctionSet == "" || msg.DeviceSource == "" || msg.DeviceDest == "" {
		writeJSON(w, http.StatusOK, ConversationResponse{Messages: []*model.Message{}, Total: 0})
		return
	}

	q := r.URL.Query()
	limit := 50
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	offset := 0
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	msgs, total, err := s.msgRepo.FindConversationMessages(traceID, msg.DeviceSource, msg.DeviceDest, msg.FunctionSet, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if msgs == nil {
		msgs = []*model.Message{}
	}

	writeJSON(w, http.StatusOK, ConversationResponse{Messages: msgs, Total: total})
}

// OrphanedRequestsResponse is the JSON envelope for the orphaned-requests endpoint.
type OrphanedRequestsResponse struct {
	IDs []int64 `json:"ids"`
}

func (s *Server) handleOrphanedRequests(w http.ResponseWriter, r *http.Request) {
	traceID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid trace ID")
		return
	}

	ids, err := s.msgRepo.FindOrphanedRequestIDs(traceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ids == nil {
		ids = []int64{}
	}

	writeJSON(w, http.StatusOK, OrphanedRequestsResponse{IDs: ids})
}

func classifyRelationship(request, response *model.Message) string {
	switch request.CmdClassifier {
	case "read":
		return "read-reply"
	case "write":
		return "write-result"
	case "call":
		if response.CmdClassifier == "notify" {
			return "subscription-notify"
		}
		return "call-result"
	default:
		return "request-response"
	}
}

func computeLatencyMs(request, response *model.Message) *float64 {
	delta := response.Timestamp.Sub(request.Timestamp)
	ms := float64(delta.Microseconds()) / 1000.0
	return &ms
}

// extractResultStatus parses the resultData.errorNumber from a SPINE payload.
// Returns "accepted" if errorNumber == 0 (or absent), "rejected" if non-zero,
// or "" if no resultData is present.
func extractResultStatus(spinePayload json.RawMessage) string {
	if len(spinePayload) == 0 {
		return ""
	}
	cmds := extractCmdsLocal(spinePayload)
	for _, cmd := range cmds {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(cmd, &m); err != nil {
			continue
		}
		rd, ok := m["resultData"]
		if !ok {
			continue
		}
		var result struct {
			ErrorNumber *int `json:"errorNumber"`
		}
		if err := json.Unmarshal(rd, &result); err != nil {
			continue
		}
		if result.ErrorNumber == nil || *result.ErrorNumber == 0 {
			return "accepted"
		}
		return "rejected"
	}
	return ""
}

// extractCmdsLocal extracts the cmd entries from a SPINE datagram payload.
// This is a local copy of the function in internal/analysis to avoid cross-package dependency.
func extractCmdsLocal(spinePayload json.RawMessage) []json.RawMessage {
	var dg struct {
		Datagram struct {
			Payload struct {
				Cmd []json.RawMessage `json:"cmd"`
			} `json:"payload"`
		} `json:"datagram"`
	}
	if err := json.Unmarshal(spinePayload, &dg); err == nil && len(dg.Datagram.Payload.Cmd) > 0 {
		return dg.Datagram.Payload.Cmd
	}

	// Try with cmd as a single object
	var dgSingle struct {
		Datagram struct {
			Payload struct {
				Cmd json.RawMessage `json:"cmd"`
			} `json:"payload"`
		} `json:"datagram"`
	}
	if err := json.Unmarshal(spinePayload, &dgSingle); err == nil && len(dgSingle.Datagram.Payload.Cmd) > 0 {
		trimmed := bytes.TrimSpace(dgSingle.Datagram.Payload.Cmd)
		if len(trimmed) > 0 && trimmed[0] == '{' {
			return []json.RawMessage{dgSingle.Datagram.Payload.Cmd}
		}
	}

	return nil
}
