package store

import (
	"database/sql"
	"fmt"

	"github.com/eebustracer/eebustracer/internal/model"
)

// DeviceRepo provides CRUD operations for devices.
type DeviceRepo struct {
	db *sql.DB
}

// NewDeviceRepo creates a new DeviceRepo.
func NewDeviceRepo(db *DB) *DeviceRepo {
	return &DeviceRepo{db: db.SqlDB()}
}

// UpsertDevice inserts or updates a device.
func (r *DeviceRepo) UpsertDevice(d *model.Device) error {
	result, err := r.db.Exec(
		`INSERT INTO devices (trace_id, device_addr, ski, brand, model, device_type, first_seen_at, last_seen_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(trace_id, device_addr) DO UPDATE SET
		   last_seen_at = excluded.last_seen_at,
		   ski = CASE WHEN excluded.ski != '' THEN excluded.ski ELSE devices.ski END,
		   brand = CASE WHEN excluded.brand != '' THEN excluded.brand ELSE devices.brand END,
		   model = CASE WHEN excluded.model != '' THEN excluded.model ELSE devices.model END,
		   device_type = CASE WHEN excluded.device_type != '' THEN excluded.device_type ELSE devices.device_type END`,
		d.TraceID, d.DeviceAddr, d.SKI, d.Brand, d.Model, d.DeviceType, d.FirstSeenAt, d.LastSeenAt,
	)
	if err != nil {
		return fmt.Errorf("upsert device: %w", err)
	}
	id, _ := result.LastInsertId()
	if id > 0 {
		d.ID = id
	}
	return nil
}

// ListDevices returns all devices for a trace.
func (r *DeviceRepo) ListDevices(traceID int64) ([]*model.Device, error) {
	rows, err := r.db.Query(
		`SELECT id, trace_id, device_addr, ski, brand, model, device_type, first_seen_at, last_seen_at
		 FROM devices WHERE trace_id = ? ORDER BY first_seen_at ASC`, traceID,
	)
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	defer rows.Close()

	var devices []*model.Device
	for rows.Next() {
		d := &model.Device{}
		if err := rows.Scan(&d.ID, &d.TraceID, &d.DeviceAddr, &d.SKI, &d.Brand, &d.Model, &d.DeviceType, &d.FirstSeenAt, &d.LastSeenAt); err != nil {
			return nil, fmt.Errorf("scan device: %w", err)
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

// GetDevice retrieves a device by ID.
func (r *DeviceRepo) GetDevice(id int64) (*model.Device, error) {
	d := &model.Device{}
	err := r.db.QueryRow(
		`SELECT id, trace_id, device_addr, ski, brand, model, device_type, first_seen_at, last_seen_at
		 FROM devices WHERE id = ?`, id,
	).Scan(&d.ID, &d.TraceID, &d.DeviceAddr, &d.SKI, &d.Brand, &d.Model, &d.DeviceType, &d.FirstSeenAt, &d.LastSeenAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get device: %w", err)
	}
	return d, nil
}
