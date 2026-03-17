package store

import (
	"strings"
	"time"
)

// MDNSDevice represents a persisted mDNS-discovered device.
type MDNSDevice struct {
	ID           int64     `json:"id"`
	InstanceName string    `json:"instanceName"`
	HostName     string    `json:"hostName"`
	Addresses    []string  `json:"addresses"`
	Port         int       `json:"port"`
	SKI          string    `json:"ski,omitempty"`
	Brand        string    `json:"brand,omitempty"`
	Model        string    `json:"model,omitempty"`
	DeviceType   string    `json:"deviceType,omitempty"`
	Identifier   string    `json:"identifier,omitempty"`
	FirstSeenAt  time.Time `json:"firstSeenAt"`
	LastSeenAt   time.Time `json:"lastSeenAt"`
	Online       bool      `json:"online"`
}

// MDNSDeviceRepo manages mDNS device persistence.
type MDNSDeviceRepo struct {
	db *DB
}

// NewMDNSDeviceRepo creates a new MDNSDeviceRepo.
func NewMDNSDeviceRepo(db *DB) *MDNSDeviceRepo {
	return &MDNSDeviceRepo{db: db}
}

// Upsert inserts or updates an mDNS device.
func (r *MDNSDeviceRepo) Upsert(d *MDNSDevice) error {
	addrs := strings.Join(d.Addresses, ",")
	_, err := r.db.db.Exec(`
		INSERT INTO mdns_devices (instance_name, host_name, addresses, port, ski, brand, model, device_type, identifier, first_seen_at, last_seen_at, online)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(instance_name) DO UPDATE SET
			host_name = excluded.host_name,
			addresses = excluded.addresses,
			port = excluded.port,
			ski = excluded.ski,
			brand = excluded.brand,
			model = excluded.model,
			device_type = excluded.device_type,
			identifier = excluded.identifier,
			last_seen_at = excluded.last_seen_at,
			online = excluded.online`,
		d.InstanceName, d.HostName, addrs, d.Port,
		d.SKI, d.Brand, d.Model, d.DeviceType, d.Identifier,
		d.FirstSeenAt, d.LastSeenAt, d.Online,
	)
	return err
}

// List returns all stored mDNS devices.
func (r *MDNSDeviceRepo) List() ([]*MDNSDevice, error) {
	rows, err := r.db.db.Query(`
		SELECT id, instance_name, host_name, addresses, port, ski, brand, model, device_type, identifier, first_seen_at, last_seen_at, online
		FROM mdns_devices ORDER BY instance_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []*MDNSDevice
	for rows.Next() {
		d := &MDNSDevice{}
		var addrs string
		if err := rows.Scan(&d.ID, &d.InstanceName, &d.HostName, &addrs, &d.Port,
			&d.SKI, &d.Brand, &d.Model, &d.DeviceType, &d.Identifier,
			&d.FirstSeenAt, &d.LastSeenAt, &d.Online); err != nil {
			return nil, err
		}
		if addrs != "" {
			d.Addresses = strings.Split(addrs, ",")
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

// MarkOffline sets all devices to offline.
func (r *MDNSDeviceRepo) MarkOffline() error {
	_, err := r.db.db.Exec(`UPDATE mdns_devices SET online = 0`)
	return err
}
