package api

import (
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
	"github.com/eebustracer/eebustracer/internal/store"
)

// WriteEntry represents a single write operation extracted from the trace.
type WriteEntry struct {
	Timestamp  time.Time `json:"timestamp"`
	MessageID  int64     `json:"messageId"`
	Source     string    `json:"source"`
	Dest       string    `json:"dest"`
	DataType   string    `json:"dataType"`
	ItemID     string    `json:"itemId"`
	Label      string    `json:"label"`
	Unit       string    `json:"unit,omitempty"`
	ScopeType  string    `json:"scopeType,omitempty"`
	Value      float64   `json:"value"`
	IsActive   *bool     `json:"isActive,omitempty"`
	Result     string    `json:"result"`
	LatencyMs  *float64  `json:"latencyMs,omitempty"`
	DurationMs *float64  `json:"durationMs,omitempty"`
}

// EffectiveStateEntry represents the current effective value for a limit or setpoint.
type EffectiveStateEntry struct {
	ItemID    string    `json:"itemId"`
	Label     string    `json:"label"`
	Unit      string    `json:"unit,omitempty"`
	ScopeType string    `json:"scopeType,omitempty"`
	DataType  string    `json:"dataType"`
	Value     float64   `json:"value"`
	IsActive  *bool     `json:"isActive,omitempty"`
	Result    string    `json:"result"`
	Since     time.Time `json:"since"`
	MessageID int64     `json:"messageId"`
}

// WriteTrackingResponse is the JSON response for the write tracking API.
type WriteTrackingResponse struct {
	Writes         []WriteEntry         `json:"writes"`
	EffectiveState []EffectiveStateEntry `json:"effectiveState"`
}

func (s *Server) handleWriteTracking(w http.ResponseWriter, r *http.Request) {
	traceID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid trace ID")
		return
	}

	// Collect write-capable descriptors from builtInDescriptors
	type writeDesc struct {
		name string
		desc ExtractionDescriptor
		fs   string
	}
	var writeDescs []writeDesc
	for name, desc := range builtInDescriptors {
		if slices.Contains(desc.Classifiers, "write") {
			if fs, ok := builtInFunctionSets[name]; ok {
				writeDescs = append(writeDescs, writeDesc{name: name, desc: desc, fs: fs})
			}
		}
	}

	if len(writeDescs) == 0 {
		writeJSON(w, http.StatusOK, WriteTrackingResponse{
			Writes:         []WriteEntry{},
			EffectiveState: []EffectiveStateEntry{},
		})
		return
	}

	// Build comma-separated FunctionSet filter
	fsets := make([]string, len(writeDescs))
	for i, wd := range writeDescs {
		fsets[i] = wd.fs
	}

	msgs, err := s.msgRepo.ListMessages(traceID, store.MessageFilter{
		CmdClassifier: "write",
		FunctionSet:   strings.Join(fsets, ","),
		ShipMsgType:   "data",
		Limit:         100000,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Build labels from description context
	descCtx := loadDescriptionContext(s, traceID)
	spLabels := loadSetpointDescriptions(s, traceID)

	// Extract write entries from each message
	var entries []WriteEntry
	for _, msg := range msgs {
		if len(msg.SpinePayload) == 0 {
			continue
		}

		for _, wd := range writeDescs {
			if msg.FunctionSet != wd.fs {
				continue
			}

			items := extractGenericData(msg.SpinePayload, wd.desc)
			for _, item := range items {
				entry := WriteEntry{
					Timestamp: msg.Timestamp,
					MessageID: msg.ID,
					Source:    msg.DeviceSource,
					Dest:     msg.DeviceDest,
					DataType:  wd.name,
					ItemID:    item.ID,
					Value:     item.Value,
					IsActive:  item.IsActive,
				}

				// Enrich label from descriptions
				entry.Label, entry.Unit, entry.ScopeType = enrichWriteLabel(wd.name, item.ID, descCtx, spLabels)

				// Correlate result
				entry.Result, entry.LatencyMs = correlateWriteResult(s, traceID, msg)

				entries = append(entries, entry)
			}
		}
	}

	if entries == nil {
		entries = []WriteEntry{}
	}

	// Sort entries by timestamp
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.Before(entries[j].Timestamp)
	})

	// Compute durations between successive writes to the same item
	computeWriteDurations(entries)

	// Build effective state: last write per itemID
	effectiveState := buildEffectiveState(entries)

	writeJSON(w, http.StatusOK, WriteTrackingResponse{
		Writes:         entries,
		EffectiveState: effectiveState,
	})
}

// enrichWriteLabel returns label, unit, and scopeType for a write entry based on descriptions.
func enrichWriteLabel(dataType, itemID string, descCtx *DescriptionContext, spLabels map[string]string) (label, unit, scopeType string) {
	switch dataType {
	case "loadcontrol":
		if l, ok := descCtx.Limits[itemID]; ok {
			return l.Label, l.Unit, l.ScopeType
		}
		return "Limit " + itemID, "", ""
	case "setpoint":
		if sl, ok := spLabels[itemID]; ok {
			return sl, "", ""
		}
		return "Setpoint " + itemID, "", ""
	default:
		return itemID, "", ""
	}
}

// correlateWriteResult finds the result message for a write by looking up the msgCounterRef.
func correlateWriteResult(s *Server, traceID int64, msg *model.Message) (result string, latencyMs *float64) {
	if msg.MsgCounter == "" {
		return "pending", nil
	}

	refs, err := s.msgRepo.FindByMsgCounterRef(traceID, msg.MsgCounter)
	if err != nil || len(refs) == 0 {
		return "pending", nil
	}

	for _, ref := range refs {
		status := extractResultStatus(ref.SpinePayload)
		if status != "" {
			latency := computeLatencyMs(msg, ref)
			return status, latency
		}
	}

	return "pending", nil
}

// computeWriteDurations computes the duration each write was effective (until the
// next write to the same itemID). Entries must be sorted by timestamp.
func computeWriteDurations(entries []WriteEntry) {
	// Group indices by itemID
	byID := make(map[string][]int)
	for i := range entries {
		byID[entries[i].ItemID] = append(byID[entries[i].ItemID], i)
	}

	for _, indices := range byID {
		for i := 0; i < len(indices)-1; i++ {
			curr := indices[i]
			next := indices[i+1]
			dur := float64(entries[next].Timestamp.Sub(entries[curr].Timestamp).Microseconds()) / 1000.0
			entries[curr].DurationMs = &dur
		}
	}
}

// buildEffectiveState returns the last write per itemID as effective state.
func buildEffectiveState(entries []WriteEntry) []EffectiveStateEntry {
	// Last write wins (entries are sorted by timestamp)
	lastByID := make(map[string]WriteEntry)
	idOrder := []string{}
	for _, e := range entries {
		if _, seen := lastByID[e.ItemID]; !seen {
			idOrder = append(idOrder, e.ItemID)
		}
		lastByID[e.ItemID] = e
	}

	state := make([]EffectiveStateEntry, 0, len(lastByID))
	for _, id := range idOrder {
		e := lastByID[id]
		state = append(state, EffectiveStateEntry{
			ItemID:    e.ItemID,
			Label:     e.Label,
			Unit:      e.Unit,
			ScopeType: e.ScopeType,
			DataType:  e.DataType,
			Value:     e.Value,
			IsActive:  e.IsActive,
			Result:    e.Result,
			Since:     e.Timestamp,
			MessageID: e.MessageID,
		})
	}
	return state
}
