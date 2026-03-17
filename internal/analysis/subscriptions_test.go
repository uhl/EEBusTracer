package analysis

import (
	"testing"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
)

func makeSubscriptionPayload(clientDevice, clientFeature, serverDevice, serverFeature string) []byte {
	return makeSpinePayload("nodeManagementSubscriptionData", map[string]interface{}{
		"subscriptionEntry": []interface{}{
			map[string]interface{}{
				"subscriptionId": "1",
				"clientAddress": map[string]interface{}{
					"device":  clientDevice,
					"entity":  []int{1},
					"feature": 1,
				},
				"serverAddress": map[string]interface{}{
					"device":  serverDevice,
					"entity":  []int{1},
					"feature": 2,
				},
			},
		},
	})
}

func TestTrackSubscriptions_Empty(t *testing.T) {
	result := TrackSubscriptionsAndBindings(nil, 5*time.Minute)
	if len(result.Subscriptions) != 0 {
		t.Errorf("expected 0 subscriptions, got %d", len(result.Subscriptions))
	}
	if len(result.Bindings) != 0 {
		t.Errorf("expected 0 bindings, got %d", len(result.Bindings))
	}
}

func TestTrackSubscriptions_SingleSubscription(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	payload := makeSubscriptionPayload("devA", "1.1", "devB", "1.2")

	msgs := []*model.Message{
		{
			ID:            1,
			Timestamp:     now,
			FunctionSet:   "NodeManagementSubscriptionData",
			CmdClassifier: "reply",
			SpinePayload:  payload,
		},
	}

	result := TrackSubscriptionsAndBindings(msgs, 5*time.Minute)
	if len(result.Subscriptions) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(result.Subscriptions))
	}

	sub := result.Subscriptions[0]
	if sub.ClientDevice != "devA" {
		t.Errorf("clientDevice = %q, want %q", sub.ClientDevice, "devA")
	}
	if sub.ServerDevice != "devB" {
		t.Errorf("serverDevice = %q, want %q", sub.ServerDevice, "devB")
	}
	if !sub.Active {
		t.Error("expected subscription to be active")
	}
	if sub.SubscriptionID != "1" {
		t.Errorf("subscriptionId = %q, want %q", sub.SubscriptionID, "1")
	}
}

func TestTrackSubscriptions_AddAndRemove(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	addPayload := makeSpinePayload("nodeManagementSubscriptionRequestCall", map[string]interface{}{
		"subscriptionEntry": []interface{}{
			map[string]interface{}{
				"clientAddress": map[string]interface{}{
					"device":  "devA",
					"entity":  []int{1},
					"feature": 1,
				},
				"serverAddress": map[string]interface{}{
					"device":  "devB",
					"entity":  []int{1},
					"feature": 2,
				},
			},
		},
	})

	deletePayload := makeSpinePayload("nodeManagementSubscriptionDeleteCall", map[string]interface{}{
		"subscriptionEntry": []interface{}{
			map[string]interface{}{
				"clientAddress": map[string]interface{}{
					"device":  "devA",
					"entity":  []int{1},
					"feature": 1,
				},
				"serverAddress": map[string]interface{}{
					"device":  "devB",
					"entity":  []int{1},
					"feature": 2,
				},
			},
		},
	})

	msgs := []*model.Message{
		{ID: 1, Timestamp: now, FunctionSet: "NodeManagementSubscriptionRequestCall", CmdClassifier: "call", SpinePayload: addPayload},
		{ID: 2, Timestamp: now.Add(time.Minute), FunctionSet: "NodeManagementSubscriptionDeleteCall", CmdClassifier: "call", SpinePayload: deletePayload},
	}

	result := TrackSubscriptionsAndBindings(msgs, 5*time.Minute)
	if len(result.Subscriptions) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(result.Subscriptions))
	}
	sub := result.Subscriptions[0]
	if sub.Active {
		t.Error("expected subscription to be inactive after delete")
	}
	if sub.RemovedAt == nil {
		t.Error("expected RemovedAt to be set")
	}
}

func TestTrackSubscriptions_StalenessDetection(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	payload := makeSubscriptionPayload("devA", "1.1", "devB", "1.2")

	msgs := []*model.Message{
		{ID: 1, Timestamp: now, FunctionSet: "NodeManagementSubscriptionData", CmdClassifier: "reply", SpinePayload: payload},
		// No notify messages within threshold
		{ID: 2, Timestamp: now.Add(10 * time.Minute), FunctionSet: "MeasurementListData", CmdClassifier: "read", DeviceSource: "devA"},
	}

	result := TrackSubscriptionsAndBindings(msgs, 5*time.Minute)
	if len(result.Subscriptions) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(result.Subscriptions))
	}
	if !result.Subscriptions[0].Stale {
		t.Error("expected subscription to be stale")
	}
}

func TestTrackSubscriptions_NotStaleWithNotify(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	// Subscription: clientFeature="1.1" (entity=[1], feature=1), serverFeature="1.2" (entity=[1], feature=2)
	payload := makeSubscriptionPayload("devA", "1.1", "devB", "1.2")

	msgs := []*model.Message{
		{ID: 1, Timestamp: now, FunctionSet: "NodeManagementSubscriptionData", CmdClassifier: "reply", SpinePayload: payload},
		// Notify from server to client — EntitySource="1", FeatureSource="2" → full addr "1.2"
		{ID: 2, Timestamp: now.Add(2 * time.Minute), FunctionSet: "MeasurementListData", CmdClassifier: "notify", DeviceSource: "devB", DeviceDest: "devA", EntitySource: "1", FeatureSource: "2"},
		{ID: 3, Timestamp: now.Add(4 * time.Minute), FunctionSet: "MeasurementListData", CmdClassifier: "read"},
	}

	result := TrackSubscriptionsAndBindings(msgs, 5*time.Minute)
	if len(result.Subscriptions) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(result.Subscriptions))
	}
	if result.Subscriptions[0].Stale {
		t.Error("expected subscription to NOT be stale with recent notify")
	}
	if result.Subscriptions[0].NotifyCount != 1 {
		t.Errorf("notifyCount = %d, want 1", result.Subscriptions[0].NotifyCount)
	}
}

func TestTrackSubscriptions_NotifyCountsMultiple(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	payload := makeSubscriptionPayload("devA", "1.1", "devB", "1.2")

	msgs := []*model.Message{
		{ID: 1, Timestamp: now, FunctionSet: "NodeManagementSubscriptionData", CmdClassifier: "reply", SpinePayload: payload},
		// Three notify messages from server feature 1.2
		{ID: 2, Timestamp: now.Add(1 * time.Minute), FunctionSet: "MeasurementListData", CmdClassifier: "notify", DeviceSource: "devB", DeviceDest: "devA", EntitySource: "1", FeatureSource: "2"},
		{ID: 3, Timestamp: now.Add(2 * time.Minute), FunctionSet: "MeasurementListData", CmdClassifier: "notify", DeviceSource: "devB", DeviceDest: "devA", EntitySource: "1", FeatureSource: "2"},
		{ID: 4, Timestamp: now.Add(3 * time.Minute), FunctionSet: "MeasurementListData", CmdClassifier: "notify", DeviceSource: "devB", DeviceDest: "devA", EntitySource: "1", FeatureSource: "2"},
		// Notify from DIFFERENT feature on same server — should NOT count
		{ID: 5, Timestamp: now.Add(4 * time.Minute), FunctionSet: "LoadControlLimitListData", CmdClassifier: "notify", DeviceSource: "devB", DeviceDest: "devA", EntitySource: "1", FeatureSource: "5"},
	}

	result := TrackSubscriptionsAndBindings(msgs, 5*time.Minute)
	if len(result.Subscriptions) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(result.Subscriptions))
	}
	sub := result.Subscriptions[0]
	if sub.NotifyCount != 3 {
		t.Errorf("notifyCount = %d, want 3 (should not count notify from different feature)", sub.NotifyCount)
	}
	if sub.Stale {
		t.Error("expected subscription to NOT be stale")
	}
}

func TestTrackSubscriptions_NotifyMatchesDeepEntity(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	// Subscription with deep entity path: entity=[1,1], feature=1 → serverFeature="1.1.1"
	payload := makeSpinePayload("nodeManagementSubscriptionRequestCall", map[string]interface{}{
		"subscriptionRequest": map[string]interface{}{
			"clientAddress": map[string]interface{}{
				"device":  "d:_i:CEM",
				"entity":  []int{2},
				"feature": 7,
			},
			"serverAddress": map[string]interface{}{
				"device":  "d:_i:EVSE",
				"entity":  []int{1, 1},
				"feature": 1,
			},
			"serverFeatureType": "LoadControl",
		},
	})

	msgs := []*model.Message{
		{ID: 1, Timestamp: now, FunctionSet: "NodeManagementSubscriptionRequestCall", CmdClassifier: "call", SpinePayload: payload},
		// Notify from entity 1.1, feature 1
		{ID: 2, Timestamp: now.Add(1 * time.Minute), FunctionSet: "LoadControlLimitListData", CmdClassifier: "notify", DeviceSource: "d:_i:EVSE", DeviceDest: "d:_i:CEM", EntitySource: "1.1", FeatureSource: "1"},
		{ID: 3, Timestamp: now.Add(3 * time.Minute), FunctionSet: "SomethingElse", CmdClassifier: "read"},
	}

	result := TrackSubscriptionsAndBindings(msgs, 5*time.Minute)
	if len(result.Subscriptions) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(result.Subscriptions))
	}
	sub := result.Subscriptions[0]
	if sub.NotifyCount != 1 {
		t.Errorf("notifyCount = %d, want 1", sub.NotifyCount)
	}
	if sub.Stale {
		t.Error("expected subscription to NOT be stale with matching deep entity notify")
	}
}

func TestTrackBindings_SingleBinding(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	payload := makeSpinePayload("nodeManagementBindingData", map[string]interface{}{
		"bindingEntry": []interface{}{
			map[string]interface{}{
				"bindingId": "b1",
				"clientAddress": map[string]interface{}{
					"device":  "devA",
					"entity":  []int{1},
					"feature": 3,
				},
				"serverAddress": map[string]interface{}{
					"device":  "devB",
					"entity":  []int{1},
					"feature": 4,
				},
			},
		},
	})

	msgs := []*model.Message{
		{ID: 1, Timestamp: now, FunctionSet: "NodeManagementBindingData", CmdClassifier: "reply", SpinePayload: payload},
	}

	result := TrackSubscriptionsAndBindings(msgs, 5*time.Minute)
	if len(result.Bindings) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(result.Bindings))
	}
	b := result.Bindings[0]
	if b.BindingID != "b1" {
		t.Errorf("bindingId = %q, want %q", b.BindingID, "b1")
	}
	if b.ClientDevice != "devA" {
		t.Errorf("clientDevice = %q, want %q", b.ClientDevice, "devA")
	}
	if !b.Active {
		t.Error("expected binding to be active")
	}
}

// TestTrackSubscriptions_RequestCallRealFormat tests with the real SPINE payload
// structure where nodeManagementSubscriptionRequestCall uses "subscriptionRequest"
// (single object) rather than "subscriptionEntry" (array).
func TestTrackSubscriptions_RequestCallRealFormat(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// This matches the real SPINE format: subscriptionRequest is a single object
	payload := makeSpinePayload("nodeManagementSubscriptionRequestCall", map[string]interface{}{
		"subscriptionRequest": map[string]interface{}{
			"clientAddress": map[string]interface{}{
				"device":  "d:_i:37916_Volvo-00000122",
				"entity":  []int{0},
				"feature": 0,
			},
			"serverAddress": map[string]interface{}{
				"device":  "d:_i:37916_CEM-400000270",
				"entity":  []int{0},
				"feature": 0,
			},
			"serverFeatureType": "NodeManagement",
		},
	})

	msgs := []*model.Message{
		{ID: 1, Timestamp: now, FunctionSet: "NodeManagementSubscriptionRequestCall", CmdClassifier: "call", SpinePayload: payload},
	}

	result := TrackSubscriptionsAndBindings(msgs, 5*time.Minute)
	if len(result.Subscriptions) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(result.Subscriptions))
	}
	sub := result.Subscriptions[0]
	if sub.ClientDevice != "d:_i:37916_Volvo-00000122" {
		t.Errorf("clientDevice = %q, want %q", sub.ClientDevice, "d:_i:37916_Volvo-00000122")
	}
	if sub.ServerDevice != "d:_i:37916_CEM-400000270" {
		t.Errorf("serverDevice = %q, want %q", sub.ServerDevice, "d:_i:37916_CEM-400000270")
	}
	if sub.ServerFeatureType != "NodeManagement" {
		t.Errorf("serverFeatureType = %q, want %q", sub.ServerFeatureType, "NodeManagement")
	}
	if !sub.Active {
		t.Error("expected subscription to be active")
	}
}

// TestTrackBindings_RequestCallRealFormat tests with the real SPINE payload
// structure where nodeManagementBindingRequestCall uses "bindingRequest"
// (single object).
func TestTrackBindings_RequestCallRealFormat(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	payload := makeSpinePayload("nodeManagementBindingRequestCall", map[string]interface{}{
		"bindingRequest": map[string]interface{}{
			"clientAddress": map[string]interface{}{
				"device":  "d:_i:37916_CEM-400000270",
				"entity":  []int{2},
				"feature": 7,
			},
			"serverAddress": map[string]interface{}{
				"device":  "d:_i:37916_Volvo-00000122",
				"entity":  []int{1, 1},
				"feature": 1,
			},
			"serverFeatureType": "LoadControl",
		},
	})

	msgs := []*model.Message{
		{ID: 1, Timestamp: now, FunctionSet: "NodeManagementBindingRequestCall", CmdClassifier: "call", SpinePayload: payload},
	}

	result := TrackSubscriptionsAndBindings(msgs, 0)
	if len(result.Bindings) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(result.Bindings))
	}
	b := result.Bindings[0]
	if b.ClientDevice != "d:_i:37916_CEM-400000270" {
		t.Errorf("clientDevice = %q, want %q", b.ClientDevice, "d:_i:37916_CEM-400000270")
	}
	if b.ServerDevice != "d:_i:37916_Volvo-00000122" {
		t.Errorf("serverDevice = %q, want %q", b.ServerDevice, "d:_i:37916_Volvo-00000122")
	}
	if b.ClientFeature != "2.7" {
		t.Errorf("clientFeature = %q, want %q", b.ClientFeature, "2.7")
	}
	if b.ServerFeature != "1.1.1" {
		t.Errorf("serverFeature = %q, want %q", b.ServerFeature, "1.1.1")
	}
	if b.ServerFeatureType != "LoadControl" {
		t.Errorf("serverFeatureType = %q, want %q", b.ServerFeatureType, "LoadControl")
	}
	if !b.Active {
		t.Error("expected binding to be active")
	}
}

func TestTrackBindings_AddAndRemove(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	addPayload := makeSpinePayload("nodeManagementBindingRequestCall", map[string]interface{}{
		"bindingEntry": []interface{}{
			map[string]interface{}{
				"clientAddress": map[string]interface{}{
					"device":  "devA",
					"entity":  []int{1},
					"feature": 3,
				},
				"serverAddress": map[string]interface{}{
					"device":  "devB",
					"entity":  []int{1},
					"feature": 4,
				},
			},
		},
	})

	deletePayload := makeSpinePayload("nodeManagementBindingDeleteCall", map[string]interface{}{
		"bindingEntry": []interface{}{
			map[string]interface{}{
				"clientAddress": map[string]interface{}{
					"device":  "devA",
					"entity":  []int{1},
					"feature": 3,
				},
				"serverAddress": map[string]interface{}{
					"device":  "devB",
					"entity":  []int{1},
					"feature": 4,
				},
			},
		},
	})

	msgs := []*model.Message{
		{ID: 1, Timestamp: now, FunctionSet: "NodeManagementBindingRequestCall", CmdClassifier: "call", SpinePayload: addPayload},
		{ID: 2, Timestamp: now.Add(time.Minute), FunctionSet: "NodeManagementBindingDeleteCall", CmdClassifier: "call", SpinePayload: deletePayload},
	}

	result := TrackSubscriptionsAndBindings(msgs, 0)
	if len(result.Bindings) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(result.Bindings))
	}
	if result.Bindings[0].Active {
		t.Error("expected binding to be inactive after delete")
	}
	if result.Bindings[0].RemovedAt == nil {
		t.Error("expected RemovedAt to be set")
	}
}
