package model

import "testing"

func TestShipMsgTypeFromHeaderByte(t *testing.T) {
	tests := []struct {
		name string
		b    byte
		want ShipMsgType
	}{
		{"init byte 0x00", 0x00, ShipMsgTypeInit},
		{"CMI byte 0x01", 0x01, ShipMsgTypeUnknown},
		{"control byte 0x02", 0x02, ShipMsgTypeUnknown},
		{"unknown byte 0xFF", 0xFF, ShipMsgTypeUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShipMsgTypeFromHeaderByte(tt.b)
			if got != tt.want {
				t.Errorf("ShipMsgTypeFromHeaderByte(0x%02x) = %q, want %q", tt.b, got, tt.want)
			}
		})
	}
}
