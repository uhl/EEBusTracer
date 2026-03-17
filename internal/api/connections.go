package api

import (
	"net/http"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
	"github.com/eebustracer/eebustracer/internal/store"
)

// ConnectionState represents the SHIP connection lifecycle for a device pair.
type ConnectionState struct {
	DeviceSource string          `json:"deviceSource"`
	DeviceDest   string          `json:"deviceDest"`
	States       []StateEntry    `json:"states"`
	CurrentState string          `json:"currentState"`
	Anomalies    []string        `json:"anomalies,omitempty"`
}

// StateEntry represents a state transition in the connection lifecycle.
type StateEntry struct {
	State     string    `json:"state"`
	Timestamp time.Time `json:"timestamp"`
	MessageID int64     `json:"messageId"`
}

func (s *Server) handleListConnections(w http.ResponseWriter, r *http.Request) {
	traceID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid trace ID")
		return
	}

	// Fetch all messages ordered by sequence
	msgs, err := s.msgRepo.ListMessages(traceID, store.MessageFilter{Limit: 10000})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	connections := buildConnectionStates(msgs)
	writeJSON(w, http.StatusOK, connections)
}

func buildConnectionStates(msgs []*model.Message) []ConnectionState {
	type pairKey struct {
		src, dst string
	}
	pairOrder := []pairKey{}
	pairMap := map[pairKey]*ConnectionState{}

	for _, msg := range msgs {
		state := shipMsgTypeToState(string(msg.ShipMsgType))
		if state == "" {
			continue
		}

		// Normalize pair key (always use sorted order for bidirectional tracking)
		src, dst := msg.DeviceSource, msg.DeviceDest
		if src == "" && dst == "" {
			// Use network addresses if device addresses aren't available
			src, dst = msg.SourceAddr, msg.DestAddr
		}
		if src == "" || dst == "" {
			continue
		}

		key := pairKey{src, dst}
		reverseKey := pairKey{dst, src}

		var cs *ConnectionState
		if c, ok := pairMap[key]; ok {
			cs = c
		} else if c, ok := pairMap[reverseKey]; ok {
			cs = c
		} else {
			cs = &ConnectionState{
				DeviceSource: src,
				DeviceDest:   dst,
			}
			pairMap[key] = cs
			pairOrder = append(pairOrder, key)
		}

		cs.States = append(cs.States, StateEntry{
			State:     state,
			Timestamp: msg.Timestamp,
			MessageID: msg.ID,
		})
		cs.CurrentState = state
	}

	// Detect anomalies and collect results in order
	results := make([]ConnectionState, 0, len(pairOrder))
	for _, key := range pairOrder {
		cs := pairMap[key]
		cs.Anomalies = detectAnomalies(cs.States)
		results = append(results, *cs)
	}
	return results
}

func shipMsgTypeToState(shipType string) string {
	switch shipType {
	case "init":
		return "init"
	case "connectionHello":
		return "hello"
	case "messageProtocolHandshake":
		return "handshake"
	case "connectionPinState":
		return "pin"
	case "accessMethods":
		return "access"
	case "data":
		return "data"
	case "connectionClose":
		return "closed"
	default:
		return ""
	}
}

func detectAnomalies(states []StateEntry) []string {
	var anomalies []string

	if len(states) == 0 {
		return anomalies
	}

	// Expected order
	expected := []string{"init", "hello", "handshake", "pin", "access", "data"}

	// Check for missing hello after init
	hasInit := false
	hasHello := false
	for _, s := range states {
		if s.State == "init" {
			hasInit = true
		}
		if s.State == "hello" {
			hasHello = true
		}
	}
	if hasInit && !hasHello {
		anomalies = append(anomalies, "missing connectionHello after init")
	}

	// Check for unexpected close (close without reaching data state)
	lastState := states[len(states)-1].State
	if lastState == "closed" {
		reachedData := false
		for _, s := range states {
			if s.State == "data" {
				reachedData = true
				break
			}
		}
		if !reachedData {
			anomalies = append(anomalies, "connection closed before data exchange")
		}
	}

	// Check for out-of-order states
	expectedIdx := 0
	for _, s := range states {
		if s.State == "closed" {
			continue
		}
		for expectedIdx < len(expected) && expected[expectedIdx] != s.State {
			expectedIdx++
		}
		if expectedIdx >= len(expected) {
			anomalies = append(anomalies, "out-of-order state transitions detected")
			break
		}
	}

	return anomalies
}
