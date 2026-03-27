package api

import (
	"net/http"
	"time"

	"github.com/eebustracer/eebustracer/internal/analysis"
	"github.com/eebustracer/eebustracer/internal/store"
)

func (s *Server) handleLifecycle(w http.ResponseWriter, r *http.Request) {
	traceID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid trace ID")
		return
	}

	// 1. Fetch all data messages (for use cases, subscriptions, bindings)
	dataMsgs, err := s.msgRepo.ListMessages(traceID, store.MessageFilter{
		ShipMsgType: "data",
		Limit:       100000,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 2. Fetch all messages (for SHIP connection states)
	allMsgs, err := s.msgRepo.ListMessages(traceID, store.MessageFilter{
		Limit: 100000,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 3. Build connection states
	connStates := buildConnectionStates(allMsgs)
	connections := make([]analysis.ConnectionInfo, 0, len(connStates))
	for _, cs := range connStates {
		connections = append(connections, analysis.ConnectionInfo{
			DeviceSource: cs.DeviceSource,
			DeviceDest:   cs.DeviceDest,
			CurrentState: cs.CurrentState,
		})
	}

	// 4. Detect use cases
	useCases := analysis.DetectUseCases(dataMsgs)

	// 5. Extract devices from discovery
	discoveryMsgs, err := s.msgRepo.ListMessages(traceID, store.MessageFilter{
		FunctionSet: "NodeManagementDetailedDiscoveryData",
		Limit:       100000,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	devices := extractDevicesFromDiscovery(discoveryMsgs)

	// 6. Track subscriptions and bindings
	sbResult := analysis.TrackSubscriptionsAndBindings(dataMsgs, 5*time.Minute)

	// 7. Evaluate lifecycles
	input := analysis.LifecycleInput{
		Connections:   connections,
		Devices:       devices,
		UseCases:      useCases,
		Subscriptions: sbResult.Subscriptions,
		Bindings:      sbResult.Bindings,
	}
	result := analysis.EvaluateLifecycles(input)

	writeJSON(w, http.StatusOK, result)
}
