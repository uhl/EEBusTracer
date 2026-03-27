package api

import (
	"net/http"
	"time"

	"github.com/eebustracer/eebustracer/internal/analysis"
	"github.com/eebustracer/eebustracer/internal/model"
	"github.com/eebustracer/eebustracer/internal/store"
)

func (s *Server) handleDependencyGraph(w http.ResponseWriter, r *http.Request) {
	traceID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid trace ID")
		return
	}

	// 1. Fetch all data messages (used for use cases, subscriptions, bindings)
	allMsgs, err := s.msgRepo.ListMessages(traceID, store.MessageFilter{
		ShipMsgType: "data",
		Limit:       100000,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 2. Detect use cases
	useCases := analysis.DetectUseCases(allMsgs)

	// 3. Extract devices from discovery messages (not from device repo,
	//    which may be empty for imported traces)
	discoveryMsgs, err := s.msgRepo.ListMessages(traceID, store.MessageFilter{
		FunctionSet: "NodeManagementDetailedDiscoveryData",
		Limit:       100000,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	devices := extractDevicesFromDiscovery(discoveryMsgs)

	// If no discovery data, fall back to extracting unique device addresses
	// from all messages so we at least show device nodes
	if len(devices) == 0 {
		devices = extractDevicesFromMessages(allMsgs)
	}

	// 4. Track subscriptions and bindings
	sbResult := analysis.TrackSubscriptionsAndBindings(allMsgs, 5*time.Minute)

	// 5. Build and return tree
	tree := analysis.BuildDependencyTree(useCases, devices, sbResult.Subscriptions, sbResult.Bindings)
	writeJSON(w, http.StatusOK, tree)
}

// deviceEntityAccumulator merges entity/feature data across multiple discovery
// messages for a single device. Discovery notify messages may contain only
// added entities (partial update), so we accumulate all entities and features
// across all messages to build the complete tree.
type deviceEntityAccumulator struct {
	addr         string
	entityOrder  []string                          // preserves insertion order
	entityMap    map[string]*analysis.EntityInfo    // entity address → entity
	featureSeen  map[string]map[string]bool         // entity address → feature address → seen
}

func newDeviceEntityAccumulator(addr string) *deviceEntityAccumulator {
	return &deviceEntityAccumulator{
		addr:        addr,
		entityMap:   make(map[string]*analysis.EntityInfo),
		featureSeen: make(map[string]map[string]bool),
	}
}

func (a *deviceEntityAccumulator) merge(entities []EntityInfoResult) {
	for _, e := range entities {
		existing, ok := a.entityMap[e.Address]
		if !ok {
			ei := analysis.EntityInfo{
				Address:    e.Address,
				EntityType: e.EntityType,
			}
			a.entityMap[e.Address] = &ei
			a.entityOrder = append(a.entityOrder, e.Address)
			a.featureSeen[e.Address] = make(map[string]bool)
			existing = &ei
		}

		for _, f := range e.Features {
			if a.featureSeen[e.Address][f.Address] {
				continue
			}
			a.featureSeen[e.Address][f.Address] = true
			existing.Features = append(existing.Features, analysis.FeatureInfo{
				Address:     f.Address,
				FeatureType: f.FeatureType,
				Role:        f.Role,
				Functions:   f.Functions,
			})
		}
	}
}

func (a *deviceEntityAccumulator) build() analysis.DeviceInfo {
	di := analysis.DeviceInfo{DeviceAddr: a.addr}
	for _, addr := range a.entityOrder {
		di.Entities = append(di.Entities, *a.entityMap[addr])
	}
	return di
}

// extractDevicesFromDiscovery builds the device info list from discovery reply/notify
// messages. Discovery notifies may be partial updates (e.g. only the added EV entity
// when a vehicle connects), so entities are merged across all messages per device
// rather than overwritten.
func extractDevicesFromDiscovery(msgs []*model.Message) []analysis.DeviceInfo {
	accumulators := map[string]*deviceEntityAccumulator{}
	var deviceOrder []string

	for _, msg := range msgs {
		if msg.CmdClassifier != "reply" && msg.CmdClassifier != "notify" {
			continue
		}
		addr := msg.DeviceSource
		if addr == "" {
			continue
		}

		acc, exists := accumulators[addr]
		if !exists {
			acc = newDeviceEntityAccumulator(addr)
			accumulators[addr] = acc
			deviceOrder = append(deviceOrder, addr)
		}

		if len(msg.SpinePayload) > 0 {
			entities := parseDiscoveryEntities(msg.SpinePayload)
			acc.merge(entities)
		}
	}

	devices := make([]analysis.DeviceInfo, 0, len(deviceOrder))
	for _, addr := range deviceOrder {
		devices = append(devices, accumulators[addr].build())
	}
	return devices
}

// extractDevicesFromMessages extracts unique device addresses from messages
// when no discovery data is available (bare device nodes without entities).
func extractDevicesFromMessages(msgs []*model.Message) []analysis.DeviceInfo {
	seen := map[string]bool{}
	var devices []analysis.DeviceInfo

	for _, msg := range msgs {
		for _, addr := range []string{msg.DeviceSource, msg.DeviceDest} {
			if addr == "" || seen[addr] {
				continue
			}
			seen[addr] = true
			devices = append(devices, analysis.DeviceInfo{DeviceAddr: addr})
		}
	}
	return devices
}
