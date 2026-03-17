package store

import (
	"testing"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
)

func TestDeviceRepo_UpsertNew(t *testing.T) {
	db := newTestDB(t)
	trace := createTestTrace(t, db)
	repo := NewDeviceRepo(db)

	dev := &model.Device{
		TraceID:     trace.ID,
		DeviceAddr:  "d:_i:EVSE_001",
		SKI:         "abc123",
		FirstSeenAt: time.Now().Truncate(time.Second),
		LastSeenAt:  time.Now().Truncate(time.Second),
	}
	if err := repo.UpsertDevice(dev); err != nil {
		t.Fatalf("UpsertDevice failed: %v", err)
	}

	devices, err := repo.ListDevices(trace.ID)
	if err != nil {
		t.Fatalf("ListDevices failed: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("len(devices) = %d, want 1", len(devices))
	}
	if devices[0].DeviceAddr != "d:_i:EVSE_001" {
		t.Errorf("DeviceAddr = %q, want %q", devices[0].DeviceAddr, "d:_i:EVSE_001")
	}
}

func TestDeviceRepo_UpsertUpdatesLastSeen(t *testing.T) {
	db := newTestDB(t)
	trace := createTestTrace(t, db)
	repo := NewDeviceRepo(db)

	t1 := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 1, 13, 0, 0, 0, time.UTC)

	dev := &model.Device{
		TraceID:     trace.ID,
		DeviceAddr:  "d:_i:EVSE_001",
		FirstSeenAt: t1,
		LastSeenAt:  t1,
	}
	if err := repo.UpsertDevice(dev); err != nil {
		t.Fatalf("first UpsertDevice failed: %v", err)
	}

	dev2 := &model.Device{
		TraceID:     trace.ID,
		DeviceAddr:  "d:_i:EVSE_001",
		FirstSeenAt: t2,
		LastSeenAt:  t2,
	}
	if err := repo.UpsertDevice(dev2); err != nil {
		t.Fatalf("second UpsertDevice failed: %v", err)
	}

	devices, err := repo.ListDevices(trace.ID)
	if err != nil {
		t.Fatalf("ListDevices failed: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device after upsert, got %d", len(devices))
	}
	if !devices[0].LastSeenAt.Equal(t2) {
		t.Errorf("LastSeenAt = %v, want %v", devices[0].LastSeenAt, t2)
	}
}
