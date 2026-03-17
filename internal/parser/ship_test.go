package parser

import (
	"testing"

	"github.com/eebustracer/eebustracer/internal/model"
)

func TestClassifyShipMessage(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		want    model.ShipMsgType
		wantErr bool
	}{
		{
			"connectionHello",
			`{"connectionHello":{"phase":"pending"}}`,
			model.ShipMsgTypeConnectionHello,
			false,
		},
		{
			"messageProtocolHandshake",
			`{"messageProtocolHandshake":{"handshakeType":"announceMax","version":{"major":1,"minor":0},"formats":{"format":["JSON-UTF8"]}}}`,
			model.ShipMsgTypeProtocolHandshake,
			false,
		},
		{
			"connectionPinState",
			`{"connectionPinState":{"pinState":"none"}}`,
			model.ShipMsgTypeConnectionPinState,
			false,
		},
		{
			"accessMethods",
			`{"accessMethods":{"id":"test"}}`,
			model.ShipMsgTypeAccessMethods,
			false,
		},
		{
			"accessMethodsRequest",
			`{"accessMethodsRequest":{}}`,
			model.ShipMsgTypeAccessMethods,
			false,
		},
		{
			"connectionClose",
			`{"connectionClose":{"phase":"announce","maxTime":30000}}`,
			model.ShipMsgTypeConnectionClose,
			false,
		},
		{
			"data with SPINE payload",
			`{"data":{"header":{"protocolId":"ee1.0"},"payload":{"datagram":{"header":{},"payload":{"cmd":[]}}}}}`,
			model.ShipMsgTypeData,
			false,
		},
		{
			"unknown key",
			`{"somethingElse":{}}`,
			model.ShipMsgTypeUnknown,
			false,
		},
		{
			"invalid JSON",
			`not json`,
			model.ShipMsgTypeUnknown,
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := classifyShipMessage([]byte(tt.json))
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.MsgType != tt.want {
				t.Errorf("MsgType = %q, want %q", result.MsgType, tt.want)
			}
		})
	}
}

func TestClassifyShipMessage_DataPayload(t *testing.T) {
	json := `{"data":{"header":{"protocolId":"ee1.0"},"payload":{"datagram":{"header":{},"payload":{"cmd":[]}}}}}`
	result, err := classifyShipMessage([]byte(json))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.DataPayload == nil {
		t.Error("expected DataPayload to be non-nil for data message")
	}
}
