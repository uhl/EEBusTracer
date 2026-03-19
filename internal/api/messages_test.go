package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
	"github.com/eebustracer/eebustracer/internal/store"
)

func TestAPI_MessagesXTotalCountHeader(t *testing.T) {
	ts, db := setupTestServer(t)

	traceRepo := store.NewTraceRepo(db)
	trace := &model.Trace{Name: "test", StartedAt: time.Now(), CreatedAt: time.Now()}
	traceRepo.CreateTrace(trace)

	msgRepo := store.NewMessageRepo(db)
	now := time.Now()
	for i := 0; i < 20; i++ {
		classifier := "read"
		if i%2 == 0 {
			classifier = "reply"
		}
		msg := &model.Message{
			TraceID:       trace.ID,
			SequenceNum:   i + 1,
			Timestamp:     now,
			ShipMsgType:   model.ShipMsgTypeData,
			CmdClassifier: classifier,
		}
		msgRepo.InsertMessage(msg)
	}

	tests := []struct {
		name           string
		url            string
		wantTotal      string
		wantUnfiltered string
		wantBodyLen    int
	}{
		{
			name:           "all messages with limit",
			url:            ts.URL + "/api/traces/1/messages?limit=5",
			wantTotal:      "20",
			wantUnfiltered: "20",
			wantBodyLen:    5,
		},
		{
			name:           "filtered messages",
			url:            ts.URL + "/api/traces/1/messages?cmdClassifier=read&limit=100",
			wantTotal:      "10",
			wantUnfiltered: "20",
			wantBodyLen:    10,
		},
		{
			name:           "no limit uses default",
			url:            ts.URL + "/api/traces/1/messages",
			wantTotal:      "20",
			wantUnfiltered: "20",
			wantBodyLen:    20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := http.Get(tt.url)
			if err != nil {
				t.Fatalf("GET failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != 200 {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}

			totalCount := resp.Header.Get("X-Total-Count")
			if totalCount != tt.wantTotal {
				t.Errorf("X-Total-Count = %q, want %q", totalCount, tt.wantTotal)
			}

			unfilteredCount := resp.Header.Get("X-Unfiltered-Count")
			if unfilteredCount != tt.wantUnfiltered {
				t.Errorf("X-Unfiltered-Count = %q, want %q", unfilteredCount, tt.wantUnfiltered)
			}

			var msgs []model.Message
			json.NewDecoder(resp.Body).Decode(&msgs)
			if len(msgs) != tt.wantBodyLen {
				t.Errorf("body len = %d, want %d", len(msgs), tt.wantBodyLen)
			}
		})
	}
}
