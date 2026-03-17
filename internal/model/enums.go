package model

// Direction indicates whether a message was sent or received.
type Direction string

const (
	DirectionIncoming Direction = "incoming"
	DirectionOutgoing Direction = "outgoing"
	DirectionUnknown  Direction = "unknown"
)

// ShipMsgType represents the type of SHIP protocol message.
type ShipMsgType string

const (
	ShipMsgTypeInit               ShipMsgType = "init"
	ShipMsgTypeConnectionHello    ShipMsgType = "connectionHello"
	ShipMsgTypeProtocolHandshake  ShipMsgType = "messageProtocolHandshake"
	ShipMsgTypeConnectionPinState ShipMsgType = "connectionPinState"
	ShipMsgTypeAccessMethods      ShipMsgType = "accessMethods"
	ShipMsgTypeData               ShipMsgType = "data"
	ShipMsgTypeConnectionClose    ShipMsgType = "connectionClose"
	ShipMsgTypeUnknown            ShipMsgType = "unknown"
)

// ShipHeaderByte is the first byte of a SHIP message frame.
type ShipHeaderByte byte

const (
	ShipHeaderInit    ShipHeaderByte = 0x00
	ShipHeaderCMI     ShipHeaderByte = 0x01
	ShipHeaderControl ShipHeaderByte = 0x02 // Not used by EEBus currently
)

// ShipMsgTypeFromHeaderByte classifies the SHIP header byte.
func ShipMsgTypeFromHeaderByte(b byte) ShipMsgType {
	switch ShipHeaderByte(b) {
	case ShipHeaderInit:
		return ShipMsgTypeInit
	case ShipHeaderCMI:
		// CMI messages contain JSON; the actual type is determined by parsing
		return ShipMsgTypeUnknown
	default:
		return ShipMsgTypeUnknown
	}
}
