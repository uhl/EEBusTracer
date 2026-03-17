package mdns

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
)

const (
	serviceType      = "_ship._tcp"
	serviceDomain    = "local."
	offlineThreshold = 60 * time.Second
	sweepInterval    = 15 * time.Second
)

// DiscoveredDevice represents an EEBus device found via mDNS.
type DiscoveredDevice struct {
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

// DeviceCallback is called when a device is discovered or updated.
type DeviceCallback func(*DiscoveredDevice)

// Monitor browses for EEBus devices via mDNS.
type Monitor struct {
	logger *slog.Logger

	mu        sync.RWMutex
	devices   map[string]*DiscoveredDevice // keyed by instance name
	running   bool
	cancel    context.CancelFunc
	callbacks []DeviceCallback
}

// NewMonitor creates a new mDNS monitor.
func NewMonitor(logger *slog.Logger) *Monitor {
	return &Monitor{
		logger:  logger,
		devices: make(map[string]*DiscoveredDevice),
	}
}

// Start begins browsing for _ship._tcp services.
func (m *Monitor) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return fmt.Errorf("mDNS monitor already running")
	}

	browseCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.running = true
	m.mu.Unlock()

	go m.browse(browseCtx)
	go m.sweepLoop(browseCtx)

	m.logger.Info("mDNS monitor started", "service", serviceType)
	return nil
}

// Stop stops the mDNS monitor.
func (m *Monitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return
	}
	m.cancel()
	m.running = false
	m.logger.Info("mDNS monitor stopped")
}

// IsRunning returns whether the monitor is currently browsing.
func (m *Monitor) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

// Devices returns a snapshot of all discovered devices.
func (m *Monitor) Devices() []*DiscoveredDevice {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*DiscoveredDevice, 0, len(m.devices))
	for _, d := range m.devices {
		cp := *d
		cp.Addresses = make([]string, len(d.Addresses))
		copy(cp.Addresses, d.Addresses)
		result = append(result, &cp)
	}
	return result
}

// OnDevice registers a callback invoked when a device is discovered or updated.
func (m *Monitor) OnDevice(cb DeviceCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callbacks = append(m.callbacks, cb)
}

func (m *Monitor) browse(ctx context.Context) {
	entries := make(chan *zeroconf.ServiceEntry, 32)

	go func() {
		for {
			select {
			case entry, ok := <-entries:
				if !ok {
					return
				}
				m.handleEntry(entry)
			case <-ctx.Done():
				return
			}
		}
	}()

	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		m.logger.Error("create mDNS resolver", "error", err)
		return
	}

	// Browse blocks until context is cancelled.
	if err := resolver.Browse(ctx, serviceType, serviceDomain, entries); err != nil {
		if ctx.Err() == nil {
			m.logger.Error("mDNS browse", "error", err)
		}
	}
}

// HandleEntry processes a zeroconf service entry and updates the device list.
// Exported for testing.
func (m *Monitor) HandleEntry(entry *zeroconf.ServiceEntry) {
	m.handleEntry(entry)
}

func (m *Monitor) handleEntry(entry *zeroconf.ServiceEntry) {
	now := time.Now()

	addrs := make([]string, 0, len(entry.AddrIPv4)+len(entry.AddrIPv6))
	for _, ip := range entry.AddrIPv4 {
		addrs = append(addrs, ip.String())
	}
	for _, ip := range entry.AddrIPv6 {
		addrs = append(addrs, ip.String())
	}

	// Parse TXT records for EEBus metadata.
	txtMap := parseTXTRecords(entry.Text)

	m.mu.Lock()
	existing, found := m.devices[entry.Instance]
	if found {
		existing.HostName = entry.HostName
		existing.Addresses = addrs
		existing.Port = entry.Port
		existing.LastSeenAt = now
		existing.Online = true
		// Update TXT fields if present
		if v, ok := txtMap["ski"]; ok {
			existing.SKI = v
		}
		if v, ok := txtMap["brand"]; ok {
			existing.Brand = v
		}
		if v, ok := txtMap["model"]; ok {
			existing.Model = v
		}
		if v, ok := txtMap["type"]; ok {
			existing.DeviceType = v
		}
		if v, ok := txtMap["id"]; ok {
			existing.Identifier = v
		}
	} else {
		existing = &DiscoveredDevice{
			InstanceName: entry.Instance,
			HostName:     entry.HostName,
			Addresses:    addrs,
			Port:         entry.Port,
			SKI:          txtMap["ski"],
			Brand:        txtMap["brand"],
			Model:        txtMap["model"],
			DeviceType:   txtMap["type"],
			Identifier:   txtMap["id"],
			FirstSeenAt:  now,
			LastSeenAt:   now,
			Online:       true,
		}
		m.devices[entry.Instance] = existing
	}

	// Copy for callbacks.
	cp := *existing
	cp.Addresses = make([]string, len(existing.Addresses))
	copy(cp.Addresses, existing.Addresses)

	cbs := make([]DeviceCallback, len(m.callbacks))
	copy(cbs, m.callbacks)
	m.mu.Unlock()

	m.logger.Debug("mDNS device",
		"instance", entry.Instance,
		"host", entry.HostName,
		"addrs", addrs,
		"port", entry.Port,
	)

	for _, cb := range cbs {
		cb(&cp)
	}
}

func (m *Monitor) sweepLoop(ctx context.Context) {
	ticker := time.NewTicker(sweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.sweep()
		}
	}
}

func (m *Monitor) sweep() {
	threshold := time.Now().Add(-offlineThreshold)

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, d := range m.devices {
		if d.Online && d.LastSeenAt.Before(threshold) {
			d.Online = false
			m.logger.Debug("mDNS device offline", "instance", d.InstanceName)
		}
	}
}

// parseTXTRecords extracts key=value pairs from mDNS TXT records.
func parseTXTRecords(txt []string) map[string]string {
	result := make(map[string]string, len(txt))
	for _, entry := range txt {
		if idx := strings.IndexByte(entry, '='); idx > 0 {
			result[entry[:idx]] = entry[idx+1:]
		}
	}
	return result
}

// ServiceEntryFromValues creates a zeroconf.ServiceEntry from individual values.
// This is a test helper.
func ServiceEntryFromValues(instance, host string, ipv4 []net.IP, port int, txt []string) *zeroconf.ServiceEntry {
	return &zeroconf.ServiceEntry{
		ServiceRecord: zeroconf.ServiceRecord{
			Instance: instance,
			Service:  serviceType,
			Domain:   serviceDomain,
		},
		HostName: host,
		Port:     port,
		AddrIPv4: ipv4,
		Text:     txt,
	}
}
