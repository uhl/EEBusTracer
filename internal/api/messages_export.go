package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
)

// handleExportMessages streams the filtered message set as CSV or JSON.
// Uses the same MessageFilter as /api/traces/{id}/messages so any active
// UI filter can be exported verbatim by appending ?format=csv or
// ?format=json (default csv). Ignores limit/offset — the export always
// covers the full filtered set.
func (s *Server) handleExportMessages(w http.ResponseWriter, r *http.Request) {
	traceID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid trace ID")
		return
	}

	filter := buildMessageFilter(r.URL.Query())
	filter.Limit = 0
	filter.Offset = 0

	messages, err := s.msgRepo.ListMessages(traceID, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "csv"
	}
	stamp := time.Now().UTC().Format("20060102-150405")

	switch format {
	case "json":
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="trace-%d-messages-%s.json"`, traceID, stamp))
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if messages == nil {
			messages = []*model.Message{}
		}
		_ = enc.Encode(messages)
	case "csv":
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="trace-%d-messages-%s.csv"`, traceID, stamp))
		writeMessagesCSV(w, messages)
	default:
		writeError(w, http.StatusBadRequest, "format must be csv or json")
	}
}

func writeMessagesCSV(w http.ResponseWriter, messages []*model.Message) {
	cw := csv.NewWriter(w)
	defer cw.Flush()

	_ = cw.Write([]string{
		"id",
		"sequenceNum",
		"timestamp",
		"direction",
		"shipMsgType",
		"cmdClassifier",
		"functionSet",
		"msgCounter",
		"msgCounterRef",
		"deviceSource",
		"deviceDest",
		"entitySource",
		"entityDest",
		"featureSource",
		"featureDest",
		"sourceAddr",
		"destAddr",
		"shipPayload",
		"spinePayload",
		"parseError",
	})

	for _, m := range messages {
		_ = cw.Write([]string{
			strconv.FormatInt(m.ID, 10),
			strconv.Itoa(m.SequenceNum),
			m.Timestamp.UTC().Format(time.RFC3339Nano),
			string(m.Direction),
			string(m.ShipMsgType),
			m.CmdClassifier,
			m.FunctionSet,
			m.MsgCounter,
			m.MsgCounterRef,
			m.DeviceSource,
			m.DeviceDest,
			m.EntitySource,
			m.EntityDest,
			m.FeatureSource,
			m.FeatureDest,
			m.SourceAddr,
			m.DestAddr,
			string(m.ShipPayload),
			string(m.SpinePayload),
			m.ParseError,
		})
	}
}
