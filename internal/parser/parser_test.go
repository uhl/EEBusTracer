package parser

import (
	"testing"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
)

func TestParser_Parse(t *testing.T) {
	p := New()
	ts := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		raw          []byte
		wantShipType model.ShipMsgType
		wantError    bool
	}{
		{
			"init message (single 0x00 byte)",
			[]byte{0x00},
			model.ShipMsgTypeInit,
			false,
		},
		{
			"connectionHello with CMI header",
			append([]byte{0x01}, []byte(`{"connectionHello":{"phase":"pending"}}`)...),
			model.ShipMsgTypeConnectionHello,
			false,
		},
		{
			"data message with SPINE payload",
			append([]byte{0x01}, []byte(`{"data":{"header":{"protocolId":"ee1.0"},"payload":{"datagram":{"header":{"addressSource":{"device":"DEV1","entity":[0],"feature":0},"addressDestination":{"device":"DEV2","entity":[0],"feature":0},"msgCounter":1,"cmdClassifier":"read"},"payload":{"cmd":[{"nodeManagementDetailedDiscoveryData":{}}]}}}}}`)...),
			model.ShipMsgTypeData,
			false,
		},
		{
			"empty message",
			[]byte{},
			model.ShipMsgTypeUnknown,
			true,
		},
		{
			"message too short",
			[]byte{0x01},
			model.ShipMsgTypeUnknown,
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := p.Parse(tt.raw, 1, 1, ts)
			if msg.ShipMsgType != tt.wantShipType {
				t.Errorf("ShipMsgType = %q, want %q", msg.ShipMsgType, tt.wantShipType)
			}
			if tt.wantError && msg.ParseError == "" {
				t.Error("expected ParseError to be set")
			}
			if !tt.wantError && msg.ParseError != "" {
				t.Errorf("unexpected ParseError: %s", msg.ParseError)
			}
		})
	}
}

func TestParser_ParseSpineFromJSON(t *testing.T) {
	p := New()

	tests := []struct {
		name          string
		json          string
		wantNil       bool
		wantClassifier string
		wantFunction  string
		wantMsgCounter string
		wantDevSrc    string
		wantDevDst    string
	}{
		{
			name: "valid EEBUS-normalized datagram",
			json: `{"datagram":{"header":{"addressSource":{"device":"d:_i:_Volvo-00000122","entity":[0],"feature":0},"addressDestination":{"device":"d:_i:37916_CEM-400000270","entity":[0],"feature":0},"msgCounter":21,"cmdClassifier":"read"},"payload":{"cmd":[{"nodeManagementDetailedDiscoveryData":{}}]}}}`,
			wantClassifier: "read",
			wantFunction:   "NodeManagementDetailedDiscoveryData",
			wantMsgCounter: "21",
			wantDevSrc:     "d:_i:_Volvo-00000122",
			wantDevDst:     "d:_i:37916_CEM-400000270",
		},
		{
			name:    "not a datagram (missing key)",
			json:    `{"connectionHello":{"phase":"pending"}}`,
			wantNil: true,
		},
		{
			name:    "invalid JSON",
			json:    `not json`,
			wantNil: true,
		},
		{
			name:    "empty object",
			json:    `{}`,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.ParseSpineFromJSON([]byte(tt.json))
			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil result, got %+v", result)
				}
				return
			}
			if result == nil {
				t.Fatal("expected non-nil result")
			}
			if result.CmdClassifier != tt.wantClassifier {
				t.Errorf("CmdClassifier = %q, want %q", result.CmdClassifier, tt.wantClassifier)
			}
			if result.FunctionSet != tt.wantFunction {
				t.Errorf("FunctionSet = %q, want %q", result.FunctionSet, tt.wantFunction)
			}
			if result.MsgCounter != tt.wantMsgCounter {
				t.Errorf("MsgCounter = %q, want %q", result.MsgCounter, tt.wantMsgCounter)
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

func TestParser_Parse_SpineExtraction(t *testing.T) {
	p := New()
	ts := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	raw := append([]byte{0x01}, []byte(`{"data":{"header":{"protocolId":"ee1.0"},"payload":{"datagram":{"header":{"addressSource":{"device":"DEV1","entity":[1],"feature":2},"addressDestination":{"device":"DEV2","entity":[1],"feature":3},"msgCounter":42,"cmdClassifier":"reply"},"payload":{"cmd":[{"measurementListData":{"measurementData":[{"measurementId":1}]}}]}}}}}`)...)

	msg := p.Parse(raw, 1, 1, ts)

	if msg.CmdClassifier != "reply" {
		t.Errorf("CmdClassifier = %q, want %q", msg.CmdClassifier, "reply")
	}
	if msg.FunctionSet != "MeasurementListData" {
		t.Errorf("FunctionSet = %q, want %q", msg.FunctionSet, "MeasurementListData")
	}
	if msg.MsgCounter != "42" {
		t.Errorf("MsgCounter = %q, want %q", msg.MsgCounter, "42")
	}
	if msg.DeviceSource != "DEV1" {
		t.Errorf("DeviceSource = %q, want %q", msg.DeviceSource, "DEV1")
	}
	if msg.DeviceDest != "DEV2" {
		t.Errorf("DeviceDest = %q, want %q", msg.DeviceDest, "DEV2")
	}
	if msg.ParseError != "" {
		t.Errorf("unexpected ParseError: %s", msg.ParseError)
	}
}
