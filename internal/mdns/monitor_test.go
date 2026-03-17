package mdns

import (
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"
)

func TestMonitor_HandleEntry(t *testing.T) {
	m := NewMonitor(slog.Default())

	entry := ServiceEntryFromValues(
		"evcc-home", "evcc.local.",
		[]net.IP{net.ParseIP("192.168.1.42")},
		4712,
		[]string{"ski=abc123", "brand=SMA", "model=HomeBoy", "type=EnergyManagementSystem", "id=evcc-001"},
	)

	m.HandleEntry(entry)

	devices := m.Devices()
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}

	d := devices[0]
	if d.InstanceName != "evcc-home" {
		t.Errorf("InstanceName = %q, want %q", d.InstanceName, "evcc-home")
	}
	if d.HostName != "evcc.local." {
		t.Errorf("HostName = %q, want %q", d.HostName, "evcc.local.")
	}
	if len(d.Addresses) != 1 || d.Addresses[0] != "192.168.1.42" {
		t.Errorf("Addresses = %v, want [192.168.1.42]", d.Addresses)
	}
	if d.Port != 4712 {
		t.Errorf("Port = %d, want 4712", d.Port)
	}
	if d.SKI != "abc123" {
		t.Errorf("SKI = %q, want %q", d.SKI, "abc123")
	}
	if d.Brand != "SMA" {
		t.Errorf("Brand = %q, want %q", d.Brand, "SMA")
	}
	if d.Model != "HomeBoy" {
		t.Errorf("Model = %q, want %q", d.Model, "HomeBoy")
	}
	if d.DeviceType != "EnergyManagementSystem" {
		t.Errorf("DeviceType = %q, want %q", d.DeviceType, "EnergyManagementSystem")
	}
	if d.Identifier != "evcc-001" {
		t.Errorf("Identifier = %q, want %q", d.Identifier, "evcc-001")
	}
	if !d.Online {
		t.Error("expected Online = true")
	}
}

func TestMonitor_HandleEntry_Update(t *testing.T) {
	m := NewMonitor(slog.Default())

	entry1 := ServiceEntryFromValues(
		"device1", "dev.local.",
		[]net.IP{net.ParseIP("192.168.1.10")},
		4712,
		[]string{"ski=old"},
	)
	m.HandleEntry(entry1)

	// Update with new IP and SKI.
	entry2 := ServiceEntryFromValues(
		"device1", "dev.local.",
		[]net.IP{net.ParseIP("192.168.1.20")},
		4712,
		[]string{"ski=new"},
	)
	m.HandleEntry(entry2)

	devices := m.Devices()
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}
	if devices[0].SKI != "new" {
		t.Errorf("SKI after update = %q, want %q", devices[0].SKI, "new")
	}
	if len(devices[0].Addresses) != 1 || devices[0].Addresses[0] != "192.168.1.20" {
		t.Errorf("Addresses after update = %v, want [192.168.1.20]", devices[0].Addresses)
	}
}

func TestMonitor_Callbacks(t *testing.T) {
	m := NewMonitor(slog.Default())

	var received []*DiscoveredDevice
	var mu sync.Mutex

	m.OnDevice(func(d *DiscoveredDevice) {
		mu.Lock()
		received = append(received, d)
		mu.Unlock()
	})

	entry := ServiceEntryFromValues(
		"cb-test", "host.local.",
		[]net.IP{net.ParseIP("10.0.0.1")},
		4712,
		nil,
	)
	m.HandleEntry(entry)

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("expected 1 callback, got %d", len(received))
	}
	if received[0].InstanceName != "cb-test" {
		t.Errorf("callback device = %q, want %q", received[0].InstanceName, "cb-test")
	}
}

func TestMonitor_DeviceList(t *testing.T) {
	m := NewMonitor(slog.Default())

	for i, name := range []string{"dev-a", "dev-b", "dev-c"} {
		entry := ServiceEntryFromValues(
			name, name+".local.",
			[]net.IP{net.ParseIP("192.168.1." + string(rune('1'+i)))},
			4712,
			nil,
		)
		m.HandleEntry(entry)
	}

	devices := m.Devices()
	if len(devices) != 3 {
		t.Fatalf("expected 3 devices, got %d", len(devices))
	}

	// Verify devices are independent copies.
	devices[0].InstanceName = "modified"
	original := m.Devices()
	for _, d := range original {
		if d.InstanceName == "modified" {
			t.Error("Devices() returned mutable reference")
		}
	}
}

func TestMonitor_OnlineOffline(t *testing.T) {
	m := NewMonitor(slog.Default())

	entry := ServiceEntryFromValues(
		"stale-dev", "stale.local.",
		[]net.IP{net.ParseIP("10.0.0.1")},
		4712,
		nil,
	)
	m.HandleEntry(entry)

	// Manually set LastSeenAt to the past to simulate a stale device.
	m.mu.Lock()
	m.devices["stale-dev"].LastSeenAt = time.Now().Add(-2 * offlineThreshold)
	m.mu.Unlock()

	m.sweep()

	devices := m.Devices()
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}
	if devices[0].Online {
		t.Error("expected device to be offline after sweep")
	}

	// Re-announce brings device back online.
	m.HandleEntry(entry)
	devices = m.Devices()
	if !devices[0].Online {
		t.Error("expected device to be back online after re-announce")
	}
}

func TestParseTXTRecords(t *testing.T) {
	tests := []struct {
		input []string
		want  map[string]string
	}{
		{
			input: []string{"ski=abc", "brand=SMA", "model=Home"},
			want:  map[string]string{"ski": "abc", "brand": "SMA", "model": "Home"},
		},
		{
			input: []string{"key=value=with=equals"},
			want:  map[string]string{"key": "value=with=equals"},
		},
		{
			input: []string{"no-value-entry"},
			want:  map[string]string{},
		},
		{
			input: nil,
			want:  map[string]string{},
		},
	}

	for _, tt := range tests {
		got := parseTXTRecords(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("parseTXTRecords(%v) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for k, v := range tt.want {
			if got[k] != v {
				t.Errorf("parseTXTRecords(%v)[%q] = %q, want %q", tt.input, k, got[k], v)
			}
		}
	}
}
