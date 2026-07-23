package analysis

import (
	"testing"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
)

func TestExtractFlowParticipants(t *testing.T) {
	tests := []struct {
		name      string
		summaries []model.MessageSummary
		want      []FlowParticipant
	}{
		{
			name:      "empty",
			summaries: nil,
			want:      []FlowParticipant{},
		},
		{
			name: "two devices ordered by first appearance",
			summaries: []model.MessageSummary{
				{DeviceSource: "d:_i:19667_CEM", DeviceDest: "d:_i:12345_EVSE1"},
				{DeviceSource: "d:_i:12345_EVSE1", DeviceDest: "d:_i:19667_CEM"},
			},
			want: []FlowParticipant{
				{DeviceAddr: "d:_i:19667_CEM", ShortName: "CEM"},
				{DeviceAddr: "d:_i:12345_EVSE1", ShortName: "EVSE1"},
			},
		},
		{
			name: "three devices",
			summaries: []model.MessageSummary{
				{DeviceSource: "devA", DeviceDest: "devB"},
				{DeviceSource: "devC", DeviceDest: "devA"},
			},
			want: []FlowParticipant{
				{DeviceAddr: "devA", ShortName: "devA"},
				{DeviceAddr: "devB", ShortName: "devB"},
				{DeviceAddr: "devC", ShortName: "devC"},
			},
		},
		{
			name: "skip empty addresses",
			summaries: []model.MessageSummary{
				{DeviceSource: "devA", DeviceDest: ""},
				{DeviceSource: "", DeviceDest: "devB"},
			},
			want: []FlowParticipant{
				{DeviceAddr: "devA", ShortName: "devA"},
				{DeviceAddr: "devB", ShortName: "devB"},
			},
		},
		{
			name: "duplicate addresses",
			summaries: []model.MessageSummary{
				{DeviceSource: "devA", DeviceDest: "devB"},
				{DeviceSource: "devA", DeviceDest: "devB"},
				{DeviceSource: "devB", DeviceDest: "devA"},
			},
			want: []FlowParticipant{
				{DeviceAddr: "devA", ShortName: "devA"},
				{DeviceAddr: "devB", ShortName: "devB"},
			},
		},
		{
			name: "fallback to sourceAddr/destAddr for SHIP handshake messages",
			summaries: []model.MessageSummary{
				{SourceAddr: "192.168.1.1:4712", DestAddr: "192.168.1.2:4712"},
				{DeviceSource: "d:_i:19667_CEM", DeviceDest: "d:_i:12345_EVSE1"},
			},
			want: []FlowParticipant{
				{DeviceAddr: "192.168.1.1:4712", ShortName: "192.168.1.1"},
				{DeviceAddr: "192.168.1.2:4712", ShortName: "192.168.1.2"},
				{DeviceAddr: "d:_i:19667_CEM", ShortName: "CEM"},
				{DeviceAddr: "d:_i:12345_EVSE1", ShortName: "EVSE1"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractFlowParticipants(tt.summaries)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d", len(got), len(tt.want))
			}
			for i, p := range got {
				if p.DeviceAddr != tt.want[i].DeviceAddr {
					t.Errorf("[%d] DeviceAddr = %q, want %q", i, p.DeviceAddr, tt.want[i].DeviceAddr)
				}
				if p.ShortName != tt.want[i].ShortName {
					t.Errorf("[%d] ShortName = %q, want %q", i, p.ShortName, tt.want[i].ShortName)
				}
			}
		})
	}
}

func TestBuildCorrelationPairs(t *testing.T) {
	now := time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		summaries []model.MessageSummary
		want      []CorrelationPair
	}{
		{
			name:      "empty",
			summaries: nil,
			want:      []CorrelationPair{},
		},
		{
			name: "read-reply pair",
			summaries: []model.MessageSummary{
				{ID: 1, Timestamp: now, CmdClassifier: "read", MsgCounter: "10"},
				{ID: 2, Timestamp: now.Add(50 * time.Millisecond), CmdClassifier: "reply", MsgCounter: "11", MsgCounterRef: "10"},
			},
			want: []CorrelationPair{
				{RequestID: 1, ResponseID: 2, RequestIndex: 0, ResponseIndex: 1, LatencyMs: 50, Relationship: "read-reply"},
			},
		},
		{
			name: "write-result pair",
			summaries: []model.MessageSummary{
				{ID: 10, Timestamp: now, CmdClassifier: "write", MsgCounter: "20"},
				{ID: 11, Timestamp: now.Add(100 * time.Millisecond), CmdClassifier: "result", MsgCounter: "21", MsgCounterRef: "20"},
			},
			want: []CorrelationPair{
				{RequestID: 10, ResponseID: 11, RequestIndex: 0, ResponseIndex: 1, LatencyMs: 100, Relationship: "write-result"},
			},
		},
		{
			name: "call-result pair",
			summaries: []model.MessageSummary{
				{ID: 20, Timestamp: now, CmdClassifier: "call", MsgCounter: "30"},
				{ID: 21, Timestamp: now.Add(200 * time.Millisecond), CmdClassifier: "reply", MsgCounter: "31", MsgCounterRef: "30"},
			},
			want: []CorrelationPair{
				{RequestID: 20, ResponseID: 21, RequestIndex: 0, ResponseIndex: 1, LatencyMs: 200, Relationship: "call-reply"},
			},
		},
		{
			name: "no match when ref not found",
			summaries: []model.MessageSummary{
				{ID: 1, Timestamp: now, CmdClassifier: "read", MsgCounter: "10"},
				{ID: 2, Timestamp: now.Add(50 * time.Millisecond), CmdClassifier: "reply", MsgCounter: "11", MsgCounterRef: "99"},
			},
			want: []CorrelationPair{},
		},
		{
			name: "multiple pairs in one trace",
			summaries: []model.MessageSummary{
				{ID: 1, Timestamp: now, CmdClassifier: "read", MsgCounter: "10"},
				{ID: 2, Timestamp: now.Add(50 * time.Millisecond), CmdClassifier: "reply", MsgCounter: "11", MsgCounterRef: "10"},
				{ID: 3, Timestamp: now.Add(100 * time.Millisecond), CmdClassifier: "write", MsgCounter: "12"},
				{ID: 4, Timestamp: now.Add(150 * time.Millisecond), CmdClassifier: "result", MsgCounter: "13", MsgCounterRef: "12"},
			},
			want: []CorrelationPair{
				{RequestID: 1, ResponseID: 2, RequestIndex: 0, ResponseIndex: 1, LatencyMs: 50, Relationship: "read-reply"},
				{RequestID: 3, ResponseID: 4, RequestIndex: 2, ResponseIndex: 3, LatencyMs: 50, Relationship: "write-result"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildCorrelationPairs(tt.summaries)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d", len(got), len(tt.want))
			}
			for i, p := range got {
				w := tt.want[i]
				if p.RequestID != w.RequestID {
					t.Errorf("[%d] RequestID = %d, want %d", i, p.RequestID, w.RequestID)
				}
				if p.ResponseID != w.ResponseID {
					t.Errorf("[%d] ResponseID = %d, want %d", i, p.ResponseID, w.ResponseID)
				}
				if p.RequestIndex != w.RequestIndex {
					t.Errorf("[%d] RequestIndex = %d, want %d", i, p.RequestIndex, w.RequestIndex)
				}
				if p.ResponseIndex != w.ResponseIndex {
					t.Errorf("[%d] ResponseIndex = %d, want %d", i, p.ResponseIndex, w.ResponseIndex)
				}
				if p.LatencyMs != w.LatencyMs {
					t.Errorf("[%d] LatencyMs = %f, want %f", i, p.LatencyMs, w.LatencyMs)
				}
				if p.Relationship != w.Relationship {
					t.Errorf("[%d] Relationship = %q, want %q", i, p.Relationship, w.Relationship)
				}
			}
		})
	}
}
