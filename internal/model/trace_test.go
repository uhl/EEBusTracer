package model

import (
	"testing"
	"time"
)

func TestTrace_IsRecording(t *testing.T) {
	tests := []struct {
		name      string
		stoppedAt *time.Time
		want      bool
	}{
		{"nil StoppedAt means recording", nil, true},
		{"non-nil StoppedAt means stopped", timePtr(time.Now()), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := &Trace{StoppedAt: tt.stoppedAt}
			if got := tr.IsRecording(); got != tt.want {
				t.Errorf("IsRecording() = %v, want %v", got, tt.want)
			}
		})
	}
}

func timePtr(t time.Time) *time.Time { return &t }
