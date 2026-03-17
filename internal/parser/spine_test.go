package parser

import (
	"encoding/json"
	"testing"
)

func TestParseSpineDatagram(t *testing.T) {
	tests := []struct {
		name          string
		payload       string
		wantClassifier string
		wantFuncSet   string
		wantMsgCtr    string
		wantDevSrc    string
		wantDevDst    string
		wantErr       bool
	}{
		{
			name: "read nodeManagementDetailedDiscoveryData",
			payload: `{"datagram":{"header":{"specificationVersion":"1.3.0","addressSource":{"device":"d:_i:EVSE_HPCHARGER","entity":[0],"feature":0},"addressDestination":{"device":"HEMS","entity":[0],"feature":0},"msgCounter":1,"cmdClassifier":"read"},"payload":{"cmd":[{"nodeManagementDetailedDiscoveryData":{}}]}}}`,
			wantClassifier: "read",
			wantFuncSet:    "NodeManagementDetailedDiscoveryData",
			wantMsgCtr:     "1",
			wantDevSrc:     "d:_i:EVSE_HPCHARGER",
			wantDevDst:     "HEMS",
		},
		{
			name: "reply with measurementListData",
			payload: `{"datagram":{"header":{"specificationVersion":"1.3.0","addressSource":{"device":"HEMS","entity":[1,1],"feature":3},"addressDestination":{"device":"d:_i:EVSE_HPCHARGER","entity":[1],"feature":5},"msgCounter":42,"msgCounterReference":10,"cmdClassifier":"reply"},"payload":{"cmd":[{"measurementListData":{"measurementData":[{"measurementId":1,"value":{"number":230,"scale":0}}]}}]}}}`,
			wantClassifier: "reply",
			wantFuncSet:    "MeasurementListData",
			wantMsgCtr:     "42",
			wantDevSrc:     "HEMS",
			wantDevDst:     "d:_i:EVSE_HPCHARGER",
		},
		{
			name: "notify with empty cmd array",
			payload: `{"datagram":{"header":{"specificationVersion":"1.3.0","addressSource":{"device":"HEMS","entity":[0],"feature":0},"addressDestination":{"device":"EV","entity":[0],"feature":0},"msgCounter":99,"cmdClassifier":"notify"},"payload":{"cmd":[]}}}`,
			wantClassifier: "notify",
			wantFuncSet:    "unknown",
			wantMsgCtr:     "99",
			wantDevSrc:     "HEMS",
			wantDevDst:     "EV",
		},
		{
			name:    "invalid JSON",
			payload: `{not valid json`,
			wantErr: true,
		},
		{
			name:    "empty payload",
			payload: ``,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseSpineDatagram(json.RawMessage(tt.payload))
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.CmdClassifier != tt.wantClassifier {
				t.Errorf("CmdClassifier = %q, want %q", result.CmdClassifier, tt.wantClassifier)
			}
			if result.FunctionSet != tt.wantFuncSet {
				t.Errorf("FunctionSet = %q, want %q", result.FunctionSet, tt.wantFuncSet)
			}
			if result.MsgCounter != tt.wantMsgCtr {
				t.Errorf("MsgCounter = %q, want %q", result.MsgCounter, tt.wantMsgCtr)
			}
			if result.DeviceSource != tt.wantDevSrc {
				t.Errorf("DeviceSource = %q, want %q", result.DeviceSource, tt.wantDevSrc)
			}
			if result.DeviceDest != tt.wantDevDst {
				t.Errorf("DeviceDest = %q, want %q", result.DeviceDest, tt.wantDevDst)
			}
		})
	}
}
