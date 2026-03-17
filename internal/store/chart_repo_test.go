package store

import (
	"testing"

	"github.com/eebustracer/eebustracer/internal/model"
)

func TestChartRepo_MigrationSeedsBuiltIns(t *testing.T) {
	db := newTestDB(t)
	repo := NewChartRepo(db)

	charts, err := repo.List(nil)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(charts) != 3 {
		t.Fatalf("expected 3 built-in charts, got %d", len(charts))
	}

	// Check names and that they are built-in
	names := map[string]bool{}
	for _, c := range charts {
		names[c.Name] = true
		if !c.IsBuiltIn {
			t.Errorf("chart %q should be built-in", c.Name)
		}
	}
	for _, expected := range []string{"Measurements", "Load Control", "Setpoints"} {
		if !names[expected] {
			t.Errorf("missing built-in chart %q", expected)
		}
	}
}

func TestChartRepo_CRUD(t *testing.T) {
	db := newTestDB(t)
	repo := NewChartRepo(db)
	trace := createTestTrace(t, db)

	// Create
	traceID := trace.ID
	cd := &model.ChartDefinition{
		Name:      "My Custom Chart",
		TraceID:   &traceID,
		ChartType: "line",
		Sources:   `[{"functionSet":"MeasurementListData","cmdKey":"measurementListData","dataArrayKey":"measurementData","idField":"measurementId","classifiers":["reply","notify"]}]`,
	}
	if err := repo.Create(cd); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if cd.ID == 0 {
		t.Error("expected ID to be set after create")
	}

	// Get
	got, err := repo.Get(cd.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil")
	}
	if got.Name != "My Custom Chart" {
		t.Errorf("name = %q, want %q", got.Name, "My Custom Chart")
	}
	if got.TraceID == nil || *got.TraceID != traceID {
		t.Errorf("traceID = %v, want %d", got.TraceID, traceID)
	}
	if got.ChartType != "line" {
		t.Errorf("chartType = %q, want %q", got.ChartType, "line")
	}
	if got.IsBuiltIn {
		t.Error("custom chart should not be built-in")
	}

	// Update
	got.Name = "Renamed Chart"
	got.ChartType = "step"
	if err := repo.Update(got); err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	updated, _ := repo.Get(cd.ID)
	if updated.Name != "Renamed Chart" {
		t.Errorf("updated name = %q, want %q", updated.Name, "Renamed Chart")
	}
	if updated.ChartType != "step" {
		t.Errorf("updated chartType = %q, want %q", updated.ChartType, "step")
	}

	// Delete
	if err := repo.Delete(cd.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	deleted, _ := repo.Get(cd.ID)
	if deleted != nil {
		t.Error("expected nil after delete")
	}
}

func TestChartRepo_ListReturnsGlobalAndTraceScoped(t *testing.T) {
	db := newTestDB(t)
	repo := NewChartRepo(db)
	trace := createTestTrace(t, db)

	// Built-in charts are global (3 from migration)
	traceID := trace.ID
	custom := &model.ChartDefinition{
		Name:      "Trace-specific",
		TraceID:   &traceID,
		ChartType: "line",
		Sources:   `[]`,
	}
	if err := repo.Create(custom); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// List for this trace should include global + trace-specific
	charts, err := repo.List(&traceID)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(charts) != 4 { // 3 built-in + 1 custom
		t.Fatalf("expected 4 charts, got %d", len(charts))
	}

	// Create a second trace
	trace2 := createTestTrace(t, db)
	otherTraceID := trace2.ID
	charts2, err := repo.List(&otherTraceID)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(charts2) != 3 { // just built-in
		t.Fatalf("expected 3 charts for other trace, got %d", len(charts2))
	}

	// List with nil traceID should only include global
	chartsNil, err := repo.List(nil)
	if err != nil {
		t.Fatalf("List nil failed: %v", err)
	}
	if len(chartsNil) != 3 {
		t.Fatalf("expected 3 charts for nil trace, got %d", len(chartsNil))
	}
}

func TestChartRepo_CannotDeleteBuiltIn(t *testing.T) {
	db := newTestDB(t)
	repo := NewChartRepo(db)

	// Get first built-in chart
	charts, _ := repo.List(nil)
	if len(charts) == 0 {
		t.Fatal("no built-in charts found")
	}

	err := repo.Delete(charts[0].ID)
	if err == nil {
		t.Error("expected error when deleting built-in chart")
	}
	if err.Error() != "cannot delete built-in chart" {
		t.Errorf("error = %q, want %q", err.Error(), "cannot delete built-in chart")
	}
}

func TestChartRepo_GetNonExistent(t *testing.T) {
	db := newTestDB(t)
	repo := NewChartRepo(db)

	got, err := repo.Get(999)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for non-existent, got %+v", got)
	}
}
