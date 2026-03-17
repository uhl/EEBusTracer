package parser

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	spinemodel "github.com/enbility/spine-go/model"
)

// SpineResult holds the extracted fields from a SPINE datagram.
type SpineResult struct {
	CmdClassifier string
	FunctionSet   string
	MsgCounter    string
	MsgCounterRef string
	DeviceSource  string
	DeviceDest    string
	EntitySource  string
	EntityDest    string
	FeatureSource string
	FeatureDest   string
	SpinePayload  json.RawMessage
}

// parseSpineDatagram decodes a SPINE datagram from raw JSON.
// After NormalizeEEBUSJSON, array fields may have been flattened to single objects.
// This function fixes known array fields before unmarshaling.
func parseSpineDatagram(payload json.RawMessage) (*SpineResult, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty SPINE payload")
	}

	fixed := fixupSpineDatagram(payload)

	var dg spinemodel.Datagram
	if err := json.Unmarshal(fixed, &dg); err != nil {
		return nil, fmt.Errorf("unmarshal SPINE datagram: %w", err)
	}

	result := &SpineResult{
		SpinePayload: json.RawMessage(fixed),
	}

	header := dg.Datagram.Header

	if header.CmdClassifier != nil {
		result.CmdClassifier = string(*header.CmdClassifier)
	}

	if header.MsgCounter != nil {
		result.MsgCounter = strconv.FormatUint(uint64(*header.MsgCounter), 10)
	}

	if header.MsgCounterReference != nil {
		result.MsgCounterRef = strconv.FormatUint(uint64(*header.MsgCounterReference), 10)
	}

	if header.AddressSource != nil {
		result.DeviceSource = deviceAddrString(header.AddressSource.Device)
		result.EntitySource = entityAddrString(header.AddressSource.Entity)
		result.FeatureSource = featureAddrString(header.AddressSource.Feature)
	}

	if header.AddressDestination != nil {
		result.DeviceDest = deviceAddrString(header.AddressDestination.Device)
		result.EntityDest = entityAddrString(header.AddressDestination.Entity)
		result.FeatureDest = featureAddrString(header.AddressDestination.Feature)
	}

	// Extract function set from first Cmd entry
	result.FunctionSet = "unknown"
	if len(dg.Datagram.Payload.Cmd) > 0 {
		cmd := dg.Datagram.Payload.Cmd[0]
		name := cmd.DataName()
		if name != "" {
			result.FunctionSet = name
		}
	}

	return result, nil
}

// fixupSpineDatagram restores array fields that NormalizeEEBUSJSON may have
// flattened. It walks the known structure: datagram → payload → cmd, and
// datagram → header → addressSource/addressDestination → entity.
func fixupSpineDatagram(data []byte) []byte {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return data
	}

	dgRaw, ok := top["datagram"]
	if !ok {
		return data
	}

	var dg map[string]json.RawMessage
	if err := json.Unmarshal(dgRaw, &dg); err != nil {
		return data
	}

	changed := false

	// Fix payload.cmd array
	if payRaw, ok := dg["payload"]; ok {
		if fixed, didFix := fixJSONArrayField(payRaw, "cmd"); didFix {
			dg["payload"] = fixed
			changed = true
		}
	}

	// Fix header address entity arrays
	if hdrRaw, ok := dg["header"]; ok {
		var hdr map[string]json.RawMessage
		if err := json.Unmarshal(hdrRaw, &hdr); err == nil {
			hdrChanged := false
			for _, key := range []string{"addressSource", "addressDestination", "addressOriginator"} {
				if addrRaw, ok := hdr[key]; ok {
					if fixed, didFix := fixJSONArrayField(addrRaw, "entity"); didFix {
						hdr[key] = fixed
						hdrChanged = true
					}
				}
			}
			if hdrChanged {
				if b, err := json.Marshal(hdr); err == nil {
					dg["header"] = json.RawMessage(b)
					changed = true
				}
			}
		}
	}

	if !changed {
		return data
	}

	if b, err := json.Marshal(dg); err == nil {
		top["datagram"] = json.RawMessage(b)
	}
	result, err := json.Marshal(top)
	if err != nil {
		return data
	}
	return result
}

// fixJSONArrayField wraps a non-array value in an array for a specific field
// inside a JSON object. Returns the modified object and whether a change was made.
func fixJSONArrayField(objData json.RawMessage, fieldName string) (json.RawMessage, bool) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(objData, &obj); err != nil {
		return objData, false
	}
	val, ok := obj[fieldName]
	if !ok || len(val) == 0 {
		return objData, false
	}
	trimmed := bytes.TrimSpace(val)
	if len(trimmed) == 0 || trimmed[0] == '[' {
		return objData, false
	}
	obj[fieldName] = json.RawMessage("[" + string(val) + "]")
	result, err := json.Marshal(obj)
	if err != nil {
		return objData, false
	}
	return json.RawMessage(result), true
}

func deviceAddrString(d *spinemodel.AddressDeviceType) string {
	if d == nil {
		return ""
	}
	return string(*d)
}

func entityAddrString(e []spinemodel.AddressEntityType) string {
	if len(e) == 0 {
		return ""
	}
	parts := make([]string, len(e))
	for i, v := range e {
		parts[i] = strconv.FormatUint(uint64(v), 10)
	}
	return strings.Join(parts, ".")
}

func featureAddrString(f *spinemodel.AddressFeatureType) string {
	if f == nil {
		return ""
	}
	return strconv.FormatUint(uint64(*f), 10)
}
