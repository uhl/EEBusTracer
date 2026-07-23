package analysis

import (
	"fmt"

	"github.com/eebustracer/eebustracer/internal/model"
)

// FlowParticipant represents a device participating in a message flow.
type FlowParticipant struct {
	DeviceAddr string `json:"deviceAddr"`
	ShortName  string `json:"shortName"`
}

// CorrelationPair links a request message to its response.
type CorrelationPair struct {
	RequestID     int64   `json:"requestId"`
	ResponseID    int64   `json:"responseId"`
	RequestIndex  int     `json:"requestIndex"`
	ResponseIndex int     `json:"responseIndex"`
	LatencyMs     float64 `json:"latencyMs"`
	Relationship  string  `json:"relationship"`
}

// ExtractFlowParticipants returns unique devices ordered by first appearance
// in the summaries list. Empty device addresses are skipped.
func ExtractFlowParticipants(summaries []model.MessageSummary) []FlowParticipant {
	seen := map[string]bool{}
	var participants []FlowParticipant

	addIfNew := func(addr string) {
		if addr == "" || seen[addr] {
			return
		}
		seen[addr] = true
		participants = append(participants, FlowParticipant{
			DeviceAddr: addr,
			ShortName:  ShortDeviceName(addr),
		})
	}

	for _, s := range summaries {
		src, dst := s.DeviceSource, s.DeviceDest
		// Fall back to network addresses for SHIP handshake messages
		// that don't have SPINE device addressing yet.
		if src == "" && dst == "" {
			src, dst = s.SourceAddr, s.DestAddr
		}
		addIfNew(src)
		addIfNew(dst)
	}

	if participants == nil {
		return []FlowParticipant{}
	}
	return participants
}

// BuildCorrelationPairs matches request/response pairs using MsgCounter
// and MsgCounterRef fields. It returns pairs with latency and relationship
// classification.
func BuildCorrelationPairs(summaries []model.MessageSummary) []CorrelationPair {
	// Build index: msgCounter → index in summaries slice
	counterIndex := map[string]int{}
	for i, s := range summaries {
		if s.MsgCounter != "" {
			counterIndex[s.MsgCounter] = i
		}
	}

	var pairs []CorrelationPair
	for i, s := range summaries {
		if s.MsgCounterRef == "" {
			continue
		}
		reqIdx, ok := counterIndex[s.MsgCounterRef]
		if !ok {
			continue
		}
		req := summaries[reqIdx]
		latency := s.Timestamp.Sub(req.Timestamp).Seconds() * 1000

		relationship := classifyRelationship(req.CmdClassifier, s.CmdClassifier)

		pairs = append(pairs, CorrelationPair{
			RequestID:     req.ID,
			ResponseID:    s.ID,
			RequestIndex:  reqIdx,
			ResponseIndex: i,
			LatencyMs:     latency,
			Relationship:  relationship,
		})
	}

	if pairs == nil {
		return []CorrelationPair{}
	}
	return pairs
}

// classifyRelationship returns a label like "read-reply", "write-result".
func classifyRelationship(reqClassifier, respClassifier string) string {
	return fmt.Sprintf("%s-%s", reqClassifier, respClassifier)
}
