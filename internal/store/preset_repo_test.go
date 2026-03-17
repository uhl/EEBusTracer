package store

import (
	"testing"

	"github.com/eebustracer/eebustracer/internal/model"
)

func TestPresetRepo_CRUDRoundTrip(t *testing.T) {
	db := newTestDB(t)
	repo := NewPresetRepo(db)

	// Create
	p := &model.FilterPreset{
		Name:   "My Filter",
		Filter: `{"cmdClassifier":"read","functionSet":"MeasurementListData"}`,
	}
	if err := repo.Create(p); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if p.ID == 0 {
		t.Error("expected ID to be set")
	}

	// Get
	got, err := repo.Get(p.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil")
	}
	if got.Name != "My Filter" {
		t.Errorf("Name = %q, want %q", got.Name, "My Filter")
	}
	if got.Filter != p.Filter {
		t.Errorf("Filter = %q, want %q", got.Filter, p.Filter)
	}

	// List
	presets, err := repo.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(presets) != 1 {
		t.Errorf("List len = %d, want 1", len(presets))
	}

	// Delete
	if err := repo.Delete(p.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	got, err = repo.Get(p.ID)
	if err != nil {
		t.Fatalf("Get after delete failed: %v", err)
	}
	if got != nil {
		t.Error("expected nil after delete")
	}
}
