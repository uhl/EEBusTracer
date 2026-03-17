package api

import (
	"net/http"

	"github.com/eebustracer/eebustracer/internal/model"
)

// RelatedMessage describes a message related to another via correlation.
type RelatedMessage struct {
	Message      *model.Message `json:"message"`
	Relationship string         `json:"relationship"` // "request-response", "call-result", "subscription-notify"
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
			related = append(related, RelatedMessage{Message: ref, Relationship: rel})
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
			related = append(related, RelatedMessage{Message: orig, Relationship: rel})
		}
	}

	if related == nil {
		related = []RelatedMessage{}
	}
	writeJSON(w, http.StatusOK, related)
}

func classifyRelationship(request, response *model.Message) string {
	switch request.CmdClassifier {
	case "call":
		if response.CmdClassifier == "notify" {
			return "subscription-notify"
		}
		return "call-result"
	case "read":
		return "request-response"
	case "write":
		return "request-response"
	default:
		return "request-response"
	}
}
