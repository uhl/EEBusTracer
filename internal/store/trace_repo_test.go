package store

import (
	"testing"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
)

func TestTraceRepo_CreateAndGet(t *testing.T) {
	db := newTestDB(t)
	repo := NewTraceRepo(db)

	trace := &model.Trace{
		Name:      "Test Trace",
		StartedAt: time.Now().Truncate(time.Second),
		CreatedAt: time.Now().Truncate(time.Second),
	}

	if err := repo.CreateTrace(trace); err != nil {
		t.Fatalf("CreateTrace failed: %v", err)
	}
	if trace.ID == 0 {
		t.Error("expected ID to be set")
	}

	got, err := repo.GetTrace(trace.ID)
	if err != nil {
		t.Fatalf("GetTrace failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetTrace returned nil")
	}
	if got.Name != "Test Trace" {
		t.Errorf("Name = %q, want %q", got.Name, "Test Trace")
	}
}

func TestTraceRepo_ListTraces(t *testing.T) {
	db := newTestDB(t)
	repo := NewTraceRepo(db)

	for i := 0; i < 3; i++ {
		trace := &model.Trace{
			Name:      "Trace",
			StartedAt: time.Now().Truncate(time.Second),
			CreatedAt: time.Now().Truncate(time.Second),
		}
		if err := repo.CreateTrace(trace); err != nil {
			t.Fatalf("CreateTrace failed: %v", err)
		}
	}

	traces, err := repo.ListTraces()
	if err != nil {
		t.Fatalf("ListTraces failed: %v", err)
	}
	if len(traces) != 3 {
		t.Errorf("len(traces) = %d, want 3", len(traces))
	}
}

func TestTraceRepo_DeleteTrace(t *testing.T) {
	db := newTestDB(t)
	repo := NewTraceRepo(db)

	trace := &model.Trace{
		Name:      "To Delete",
		StartedAt: time.Now().Truncate(time.Second),
		CreatedAt: time.Now().Truncate(time.Second),
	}
	if err := repo.CreateTrace(trace); err != nil {
		t.Fatalf("CreateTrace failed: %v", err)
	}

	if err := repo.DeleteTrace(trace.ID); err != nil {
		t.Fatalf("DeleteTrace failed: %v", err)
	}

	got, err := repo.GetTrace(trace.ID)
	if err != nil {
		t.Fatalf("GetTrace failed: %v", err)
	}
	if got != nil {
		t.Error("expected nil after delete")
	}
}

func TestTraceRepo_StopTrace(t *testing.T) {
	db := newTestDB(t)
	repo := NewTraceRepo(db)

	trace := &model.Trace{
		Name:      "To Stop",
		StartedAt: time.Now().Truncate(time.Second),
		CreatedAt: time.Now().Truncate(time.Second),
	}
	if err := repo.CreateTrace(trace); err != nil {
		t.Fatalf("CreateTrace failed: %v", err)
	}

	stopTime := time.Now().Truncate(time.Second)
	if err := repo.StopTrace(trace.ID, stopTime, 42); err != nil {
		t.Fatalf("StopTrace failed: %v", err)
	}

	got, err := repo.GetTrace(trace.ID)
	if err != nil {
		t.Fatalf("GetTrace failed: %v", err)
	}
	if got.StoppedAt == nil {
		t.Error("expected StoppedAt to be set")
	}
	if got.MessageCount != 42 {
		t.Errorf("MessageCount = %d, want 42", got.MessageCount)
	}
}

func TestTraceRepo_CascadeDelete(t *testing.T) {
	db := newTestDB(t)
	traceRepo := NewTraceRepo(db)
	msgRepo := NewMessageRepo(db)

	trace := &model.Trace{
		Name:      "With Messages",
		StartedAt: time.Now().Truncate(time.Second),
		CreatedAt: time.Now().Truncate(time.Second),
	}
	if err := traceRepo.CreateTrace(trace); err != nil {
		t.Fatalf("CreateTrace failed: %v", err)
	}

	msg := &model.Message{
		TraceID:     trace.ID,
		SequenceNum: 1,
		Timestamp:   time.Now().Truncate(time.Second),
		ShipMsgType: model.ShipMsgTypeInit,
	}
	if err := msgRepo.InsertMessage(msg); err != nil {
		t.Fatalf("InsertMessage failed: %v", err)
	}

	if err := traceRepo.DeleteTrace(trace.ID); err != nil {
		t.Fatalf("DeleteTrace failed: %v", err)
	}

	count, err := msgRepo.CountMessages(trace.ID)
	if err != nil {
		t.Fatalf("CountMessages failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 messages after cascade delete, got %d", count)
	}
}
