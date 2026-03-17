package analysis

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
)

// SubscriptionEntry represents a tracked subscription.
type SubscriptionEntry struct {
	SubscriptionID    string     `json:"subscriptionId,omitempty"`
	ClientDevice      string     `json:"clientDevice"`
	ClientFeature     string     `json:"clientFeature,omitempty"`
	ServerDevice      string     `json:"serverDevice"`
	ServerFeature     string     `json:"serverFeature,omitempty"`
	ServerFeatureType string     `json:"serverFeatureType,omitempty"`
	Active            bool       `json:"active"`
	CreatedAt         time.Time  `json:"createdAt"`
	RemovedAt         *time.Time `json:"removedAt,omitempty"`
	LastNotifyAt      *time.Time `json:"lastNotifyAt,omitempty"`
	NotifyCount       int        `json:"notifyCount"`
	Stale             bool       `json:"stale"`
	MessageID         int64      `json:"messageId"`
}

// BindingEntry represents a tracked binding.
type BindingEntry struct {
	BindingID         string     `json:"bindingId,omitempty"`
	ClientDevice      string     `json:"clientDevice"`
	ClientFeature     string     `json:"clientFeature,omitempty"`
	ServerDevice      string     `json:"serverDevice"`
	ServerFeature     string     `json:"serverFeature,omitempty"`
	ServerFeatureType string     `json:"serverFeatureType,omitempty"`
	Active            bool       `json:"active"`
	CreatedAt         time.Time  `json:"createdAt"`
	RemovedAt         *time.Time `json:"removedAt,omitempty"`
	MessageID         int64      `json:"messageId"`
}

// SubscriptionBindingResult contains tracked subscriptions and bindings.
type SubscriptionBindingResult struct {
	Subscriptions []SubscriptionEntry `json:"subscriptions"`
	Bindings      []BindingEntry      `json:"bindings"`
}

// subscriptionKey uniquely identifies a subscription by client/server features.
type subscriptionKey struct {
	clientDevice, clientFeature, serverDevice, serverFeature string
}

// TrackSubscriptionsAndBindings walks messages in sequence order to track
// subscription and binding lifecycle, detecting stale subscriptions.
func TrackSubscriptionsAndBindings(msgs []*model.Message, stalenessThreshold time.Duration) SubscriptionBindingResult {
	subMap := map[subscriptionKey]*SubscriptionEntry{}
	subOrder := []subscriptionKey{}

	bindMap := map[subscriptionKey]*BindingEntry{}
	bindOrder := []subscriptionKey{}

	for _, msg := range msgs {
		switch msg.FunctionSet {
		case "NodeManagementSubscriptionData":
			if msg.CmdClassifier == "reply" || msg.CmdClassifier == "notify" {
				// Snapshot: replace all subscriptions for this device pair
				subs := extractSubscriptionData(msg.SpinePayload)
				for _, sub := range subs {
					key := subscriptionKey{
						clientDevice:  sub.ClientDevice,
						clientFeature: sub.ClientFeature,
						serverDevice:  sub.ServerDevice,
						serverFeature: sub.ServerFeature,
					}
					if _, ok := subMap[key]; !ok {
						subOrder = append(subOrder, key)
					}
					sub.CreatedAt = msg.Timestamp
					sub.MessageID = msg.ID
					sub.Active = true
					subMap[key] = &sub
				}
			}
		case "NodeManagementSubscriptionRequestCall":
			if msg.CmdClassifier == "call" {
				subs := extractSubscriptionData(msg.SpinePayload)
				for _, sub := range subs {
					key := subscriptionKey{
						clientDevice:  sub.ClientDevice,
						clientFeature: sub.ClientFeature,
						serverDevice:  sub.ServerDevice,
						serverFeature: sub.ServerFeature,
					}
					if _, ok := subMap[key]; !ok {
						subOrder = append(subOrder, key)
					}
					sub.CreatedAt = msg.Timestamp
					sub.MessageID = msg.ID
					sub.Active = true
					subMap[key] = &sub
				}
			}
		case "NodeManagementSubscriptionDeleteCall":
			if msg.CmdClassifier == "call" {
				subs := extractSubscriptionData(msg.SpinePayload)
				for _, sub := range subs {
					key := subscriptionKey{
						clientDevice:  sub.ClientDevice,
						clientFeature: sub.ClientFeature,
						serverDevice:  sub.ServerDevice,
						serverFeature: sub.ServerFeature,
					}
					if existing, ok := subMap[key]; ok {
						existing.Active = false
						ts := msg.Timestamp
						existing.RemovedAt = &ts
					}
				}
			}
		case "NodeManagementBindingData":
			if msg.CmdClassifier == "reply" || msg.CmdClassifier == "notify" {
				bindings := extractBindingData(msg.SpinePayload)
				for _, b := range bindings {
					key := subscriptionKey{
						clientDevice:  b.ClientDevice,
						clientFeature: b.ClientFeature,
						serverDevice:  b.ServerDevice,
						serverFeature: b.ServerFeature,
					}
					if _, ok := bindMap[key]; !ok {
						bindOrder = append(bindOrder, key)
					}
					b.CreatedAt = msg.Timestamp
					b.MessageID = msg.ID
					b.Active = true
					bindMap[key] = &b
				}
			}
		case "NodeManagementBindingRequestCall":
			if msg.CmdClassifier == "call" {
				bindings := extractBindingData(msg.SpinePayload)
				for _, b := range bindings {
					key := subscriptionKey{
						clientDevice:  b.ClientDevice,
						clientFeature: b.ClientFeature,
						serverDevice:  b.ServerDevice,
						serverFeature: b.ServerFeature,
					}
					if _, ok := bindMap[key]; !ok {
						bindOrder = append(bindOrder, key)
					}
					b.CreatedAt = msg.Timestamp
					b.MessageID = msg.ID
					b.Active = true
					bindMap[key] = &b
				}
			}
		case "NodeManagementBindingDeleteCall":
			if msg.CmdClassifier == "call" {
				bindings := extractBindingData(msg.SpinePayload)
				for _, b := range bindings {
					key := subscriptionKey{
						clientDevice:  b.ClientDevice,
						clientFeature: b.ClientFeature,
						serverDevice:  b.ServerDevice,
						serverFeature: b.ServerFeature,
					}
					if existing, ok := bindMap[key]; ok {
						existing.Active = false
						ts := msg.Timestamp
						existing.RemovedAt = &ts
					}
				}
			}
		}

		// Track notify messages for subscription staleness
		if msg.CmdClassifier == "notify" && msg.FunctionSet != "" {
			for key, sub := range subMap {
				if !sub.Active {
					continue
				}
				// Match: notify from server feature to client device
				if matchesSubscriptionNotify(msg, key) {
					ts := msg.Timestamp
					sub.LastNotifyAt = &ts
					sub.NotifyCount++
				}
			}
		}
	}

	// Determine staleness
	var lastTimestamp time.Time
	if len(msgs) > 0 {
		lastTimestamp = msgs[len(msgs)-1].Timestamp
	}

	subscriptions := make([]SubscriptionEntry, 0, len(subOrder))
	for _, key := range subOrder {
		sub := subMap[key]
		if sub.Active && stalenessThreshold > 0 {
			if sub.LastNotifyAt == nil {
				// No notify ever received
				if lastTimestamp.Sub(sub.CreatedAt) > stalenessThreshold {
					sub.Stale = true
				}
			} else if lastTimestamp.Sub(*sub.LastNotifyAt) > stalenessThreshold {
				sub.Stale = true
			}
		}
		subscriptions = append(subscriptions, *sub)
	}

	bindings := make([]BindingEntry, 0, len(bindOrder))
	for _, key := range bindOrder {
		bindings = append(bindings, *bindMap[key])
	}

	return SubscriptionBindingResult{
		Subscriptions: subscriptions,
		Bindings:      bindings,
	}
}

func matchesSubscriptionNotify(msg *model.Message, key subscriptionKey) bool {
	// Notify should come from the server device/feature toward the client
	if msg.DeviceSource != key.serverDevice || msg.DeviceDest != key.clientDevice {
		return false
	}
	if key.serverFeature == "" {
		return true
	}
	// Build full feature address from entity + feature to match the
	// subscription's formatFeatureAddr output (e.g., "1.1.1").
	// The parser stores entity and feature separately on messages:
	//   EntitySource="1.1", FeatureSource="1" → full addr "1.1.1"
	msgFeature := msg.FeatureSource
	if msg.EntitySource != "" && msgFeature != "" {
		msgFeature = msg.EntitySource + "." + msgFeature
	}
	return msgFeature == key.serverFeature
}

// subAddrInfo is the shared address structure used in subscription/binding entries.
type subAddrInfo struct {
	SubscriptionID    *string `json:"subscriptionId"`
	BindingID         *string `json:"bindingId"`
	ServerFeatureType *string `json:"serverFeatureType"`
	ClientAddress     *struct {
		Device  *string `json:"device"`
		Entity  []uint  `json:"entity"`
		Feature *uint   `json:"feature"`
	} `json:"clientAddress"`
	ServerAddress *struct {
		Device  *string `json:"device"`
		Entity  []uint  `json:"entity"`
		Feature *uint   `json:"feature"`
	} `json:"serverAddress"`
}

func extractSubscriptionData(spinePayload json.RawMessage) []SubscriptionEntry {
	cmds := extractCmds(spinePayload)

	var entries []SubscriptionEntry
	for _, cmd := range cmds {
		var cmdMap map[string]json.RawMessage
		if err := json.Unmarshal(cmd, &cmdMap); err != nil {
			continue
		}

		addrs := extractSubAddrs(cmdMap, []string{
			"nodeManagementSubscriptionData",
			"nodeManagementSubscriptionRequestCall",
			"nodeManagementSubscriptionDeleteCall",
		}, []string{
			"subscriptionEntry",
			"subscriptionRequest",
			"subscriptionDelete",
		})

		for _, a := range addrs {
			entry := SubscriptionEntry{}
			if a.SubscriptionID != nil {
				entry.SubscriptionID = *a.SubscriptionID
			}
			if a.ServerFeatureType != nil {
				entry.ServerFeatureType = *a.ServerFeatureType
			}
			if a.ClientAddress != nil {
				if a.ClientAddress.Device != nil {
					entry.ClientDevice = *a.ClientAddress.Device
				}
				if a.ClientAddress.Feature != nil {
					entry.ClientFeature = formatFeatureAddr(a.ClientAddress.Entity, *a.ClientAddress.Feature)
				}
			}
			if a.ServerAddress != nil {
				if a.ServerAddress.Device != nil {
					entry.ServerDevice = *a.ServerAddress.Device
				}
				if a.ServerAddress.Feature != nil {
					entry.ServerFeature = formatFeatureAddr(a.ServerAddress.Entity, *a.ServerAddress.Feature)
				}
			}
			entries = append(entries, entry)
		}
	}
	return entries
}

func extractBindingData(spinePayload json.RawMessage) []BindingEntry {
	cmds := extractCmds(spinePayload)

	var entries []BindingEntry
	for _, cmd := range cmds {
		var cmdMap map[string]json.RawMessage
		if err := json.Unmarshal(cmd, &cmdMap); err != nil {
			continue
		}

		addrs := extractSubAddrs(cmdMap, []string{
			"nodeManagementBindingData",
			"nodeManagementBindingRequestCall",
			"nodeManagementBindingDeleteCall",
		}, []string{
			"bindingEntry",
			"bindingRequest",
			"bindingDelete",
		})

		for _, a := range addrs {
			entry := BindingEntry{}
			if a.BindingID != nil {
				entry.BindingID = *a.BindingID
			}
			if a.ServerFeatureType != nil {
				entry.ServerFeatureType = *a.ServerFeatureType
			}
			if a.ClientAddress != nil {
				if a.ClientAddress.Device != nil {
					entry.ClientDevice = *a.ClientAddress.Device
				}
				if a.ClientAddress.Feature != nil {
					entry.ClientFeature = formatFeatureAddr(a.ClientAddress.Entity, *a.ClientAddress.Feature)
				}
			}
			if a.ServerAddress != nil {
				if a.ServerAddress.Device != nil {
					entry.ServerDevice = *a.ServerAddress.Device
				}
				if a.ServerAddress.Feature != nil {
					entry.ServerFeature = formatFeatureAddr(a.ServerAddress.Entity, *a.ServerAddress.Feature)
				}
			}
			entries = append(entries, entry)
		}
	}
	return entries
}

// extractCmds extracts the cmd entries from a SPINE datagram payload,
// handling the case where cmd may be a single object instead of an array
// (due to EEBUS JSON normalization).
func extractCmds(spinePayload json.RawMessage) []json.RawMessage {
	var dg struct {
		Datagram struct {
			Payload struct {
				Cmd []json.RawMessage `json:"cmd"`
			} `json:"payload"`
		} `json:"datagram"`
	}
	if err := json.Unmarshal(spinePayload, &dg); err == nil && len(dg.Datagram.Payload.Cmd) > 0 {
		return dg.Datagram.Payload.Cmd
	}

	// Try with cmd as a single object (flattened by EEBUS normalization)
	var dgSingle struct {
		Datagram struct {
			Payload struct {
				Cmd json.RawMessage `json:"cmd"`
			} `json:"payload"`
		} `json:"datagram"`
	}
	if err := json.Unmarshal(spinePayload, &dgSingle); err == nil && len(dgSingle.Datagram.Payload.Cmd) > 0 {
		trimmed := bytes.TrimSpace(dgSingle.Datagram.Payload.Cmd)
		if len(trimmed) > 0 && trimmed[0] == '{' {
			return []json.RawMessage{dgSingle.Datagram.Payload.Cmd}
		}
	}

	return nil
}

// extractSubAddrs finds subscription/binding address entries from a cmd map.
// outerKeys are the top-level command keys (e.g., "nodeManagementSubscriptionData").
// innerKeys are the entry field names to look for (e.g., "subscriptionEntry", "subscriptionRequest").
// Handles both array and single-object forms for each inner key.
func extractSubAddrs(cmdMap map[string]json.RawMessage, outerKeys, innerKeys []string) []subAddrInfo {
	var raw json.RawMessage
	for _, key := range outerKeys {
		if r, ok := cmdMap[key]; ok {
			raw = r
			break
		}
	}
	if raw == nil {
		return nil
	}

	// Parse the inner object to find entry fields
	var innerMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &innerMap); err != nil {
		return nil
	}

	for _, innerKey := range innerKeys {
		entryRaw, ok := innerMap[innerKey]
		if !ok {
			continue
		}

		trimmed := bytes.TrimSpace(entryRaw)
		if len(trimmed) == 0 {
			continue
		}

		// Try as array first
		if trimmed[0] == '[' {
			var addrs []subAddrInfo
			if err := json.Unmarshal(entryRaw, &addrs); err == nil && len(addrs) > 0 {
				return addrs
			}
		}

		// Try as single object (EEBUS normalization may have flattened)
		if trimmed[0] == '{' {
			var addr subAddrInfo
			if err := json.Unmarshal(entryRaw, &addr); err == nil {
				return []subAddrInfo{addr}
			}
		}
	}

	return nil
}

func formatFeatureAddr(entity []uint, feature uint) string {
	s := ""
	for i, e := range entity {
		if i > 0 {
			s += "."
		}
		s += uintToStr(e)
	}
	if s != "" {
		s += "."
	}
	s += uintToStr(feature)
	return s
}

func uintToStr(n uint) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}
