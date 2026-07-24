package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
)

func TestDevicesCommand_InvalidFile(t *testing.T) {
	devicesOutput = "text"
	if err := runDevices(nil, []string{"/nonexistent/file.eet"}); err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestBuildDeviceReports(t *testing.T) {
	msgs := []*model.Message{
		{
			SequenceNum:   1,
			Timestamp:     time.Now(),
			ShipMsgType:   model.ShipMsgTypeData,
			CmdClassifier: "read",
			DeviceSource:  "devA",
			DeviceDest:    "devB",
		},
		{
			SequenceNum:   2,
			Timestamp:     time.Now(),
			ShipMsgType:   model.ShipMsgTypeData,
			CmdClassifier: "reply",
			DeviceSource:  "devB",
			DeviceDest:    "devA",
			FunctionSet:   "NodeManagementDetailedDiscoveryData",
			SpinePayload: json.RawMessage(`{
				"datagram": {
					"payload": {
						"cmd": [{
							"nodeManagementDetailedDiscoveryData": {
								"entityInformation": [{
									"description": {
										"entityAddress": {"entity": [0]},
										"entityType": "EVSE"
									}
								}],
								"featureInformation": [{
									"description": {
										"featureAddress": {"entity": [0], "feature": 7},
										"featureType": "LoadControl",
										"role": "server",
										"supportedFunction": [{"function": "loadControlLimitData"}]
									}
								}]
							}
						}]
					}
				}
			}`),
		},
	}

	reports := buildDeviceReports(msgs)
	if len(reports) != 2 {
		t.Fatalf("report count = %d, want 2", len(reports))
	}

	// Find devB (has discovery data)
	var devB *deviceReport
	for i := range reports {
		if reports[i].DeviceAddr == "devB" {
			devB = &reports[i]
		}
	}
	if devB == nil {
		t.Fatal("devB not found in reports")
	}
	if len(devB.Entities) != 1 || devB.Entities[0].EntityType != "EVSE" {
		t.Errorf("devB entities = %+v, want one EVSE entity", devB.Entities)
	}
	if len(devB.Entities[0].Features) != 1 || devB.Entities[0].Features[0].FeatureType != "LoadControl" {
		t.Errorf("devB features = %+v, want one LoadControl", devB.Entities[0].Features)
	}
	if devB.MessageCnt != 1 {
		t.Errorf("devB message count = %d, want 1", devB.MessageCnt)
	}
}

func TestDevicesCommand_JSONOutput(t *testing.T) {
	eebt := map[string]interface{}{
		"version": "1.0",
		"trace":   map[string]interface{}{"name": "t", "startedAt": "2024-01-01T12:00:00Z"},
		"messages": []map[string]interface{}{
			{
				"sequenceNum":  1,
				"timestamp":    "2024-01-01T12:00:01Z",
				"direction":    "incoming",
				"rawHex":       "00",
				"shipMsgType":  "data",
				"sourceAddr":   "A",
				"destAddr":     "B",
				"deviceSource": "devA",
				"deviceDest":   "devB",
			},
		},
	}

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.eet")
	f, _ := os.Create(filePath)
	_ = json.NewEncoder(f).Encode(eebt)
	f.Close()

	devicesOutput = "json"
	if err := runDevices(nil, []string{filePath}); err != nil {
		t.Fatalf("devices failed: %v", err)
	}
	devicesOutput = "text"
}
