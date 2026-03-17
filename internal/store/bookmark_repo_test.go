package store

import (
	"testing"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
)

func TestBookmarkRepo_CRUDRoundTrip(t *testing.T) {
	db := newTestDB(t)
	trace := createTestTrace(t, db)
	msgRepo := NewMessageRepo(db)

	msg := &model.Message{
		TraceID:     trace.ID,
		SequenceNum: 1,
		Timestamp:   time.Now().Truncate(time.Second),
		ShipMsgType: model.ShipMsgTypeData,
	}
	if err := msgRepo.InsertMessage(msg); err != nil {
		t.Fatalf("InsertMessage failed: %v", err)
	}

	repo := NewBookmarkRepo(db)

	// Create
	b := &model.Bookmark{
		MessageID: msg.ID,
		TraceID:   trace.ID,
		Label:     "Important",
		Color:     "#ff0000",
		Note:      "Check this message",
	}
	if err := repo.Create(b); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if b.ID == 0 {
		t.Error("expected ID to be set")
	}

	// List
	bookmarks, err := repo.List(trace.ID)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(bookmarks) != 1 {
		t.Errorf("List len = %d, want 1", len(bookmarks))
	}

	// GetByMessage
	got, err := repo.GetByMessage(msg.ID)
	if err != nil {
		t.Fatalf("GetByMessage failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetByMessage returned nil")
	}
	if got.Label != "Important" {
		t.Errorf("Label = %q, want %q", got.Label, "Important")
	}

	// Delete
	if err := repo.Delete(b.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	got, err = repo.GetByMessage(msg.ID)
	if err != nil {
		t.Fatalf("GetByMessage after delete failed: %v", err)
	}
	if got != nil {
		t.Error("expected nil after delete")
	}
}

func TestBookmarkRepo_CascadeDelete(t *testing.T) {
	db := newTestDB(t)
	trace := createTestTrace(t, db)
	msgRepo := NewMessageRepo(db)

	msg := &model.Message{
		TraceID:     trace.ID,
		SequenceNum: 1,
		Timestamp:   time.Now().Truncate(time.Second),
		ShipMsgType: model.ShipMsgTypeData,
	}
	if err := msgRepo.InsertMessage(msg); err != nil {
		t.Fatalf("InsertMessage failed: %v", err)
	}

	bookmarkRepo := NewBookmarkRepo(db)
	b := &model.Bookmark{
		MessageID: msg.ID,
		TraceID:   trace.ID,
		Label:     "test",
	}
	if err := bookmarkRepo.Create(b); err != nil {
		t.Fatalf("Create bookmark failed: %v", err)
	}

	// Delete the trace — bookmark should cascade
	traceRepo := NewTraceRepo(db)
	if err := traceRepo.DeleteTrace(trace.ID); err != nil {
		t.Fatalf("DeleteTrace failed: %v", err)
	}

	bookmarks, err := bookmarkRepo.List(trace.ID)
	if err != nil {
		t.Fatalf("List after cascade delete failed: %v", err)
	}
	if len(bookmarks) != 0 {
		t.Errorf("expected 0 bookmarks after cascade delete, got %d", len(bookmarks))
	}
}
