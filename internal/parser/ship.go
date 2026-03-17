package parser

import (
	"encoding/json"
	"fmt"

	shipmodel "github.com/enbility/ship-go/model"

	"github.com/eebustracer/eebustracer/internal/model"
)

// shipClassification holds the result of classifying a SHIP message.
type shipClassification struct {
	MsgType     model.ShipMsgType
	ShipPayload json.RawMessage // The full JSON of the matched SHIP message
	DataPayload json.RawMessage // For ShipData, the SPINE payload
}

// classifyShipMessage examines the top-level JSON key to determine the SHIP message type.
func classifyShipMessage(data []byte) (*shipClassification, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal SHIP message: %w", err)
	}

	// Check each known top-level key
	if _, ok := raw["connectionHello"]; ok {
		var msg shipmodel.ConnectionHello
		normalized := fixupSliceFields(data, msg)
		if err := json.Unmarshal(normalized, &msg); err != nil {
			return nil, fmt.Errorf("unmarshal connectionHello: %w", err)
		}
		return &shipClassification{
			MsgType:     model.ShipMsgTypeConnectionHello,
			ShipPayload: data,
		}, nil
	}

	if _, ok := raw["messageProtocolHandshake"]; ok {
		var msg shipmodel.MessageProtocolHandshake
		normalized := fixupSliceFields(data, msg)
		if err := json.Unmarshal(normalized, &msg); err != nil {
			return nil, fmt.Errorf("unmarshal messageProtocolHandshake: %w", err)
		}
		return &shipClassification{
			MsgType:     model.ShipMsgTypeProtocolHandshake,
			ShipPayload: data,
		}, nil
	}

	if _, ok := raw["connectionPinState"]; ok {
		var msg shipmodel.ConnectionPinState
		normalized := fixupSliceFields(data, msg)
		if err := json.Unmarshal(normalized, &msg); err != nil {
			return nil, fmt.Errorf("unmarshal connectionPinState: %w", err)
		}
		return &shipClassification{
			MsgType:     model.ShipMsgTypeConnectionPinState,
			ShipPayload: data,
		}, nil
	}

	if _, ok := raw["accessMethods"]; ok {
		return &shipClassification{
			MsgType:     model.ShipMsgTypeAccessMethods,
			ShipPayload: data,
		}, nil
	}

	if _, ok := raw["accessMethodsRequest"]; ok {
		return &shipClassification{
			MsgType:     model.ShipMsgTypeAccessMethods,
			ShipPayload: data,
		}, nil
	}

	if _, ok := raw["connectionClose"]; ok {
		var msg shipmodel.ConnectionClose
		normalized := fixupSliceFields(data, msg)
		if err := json.Unmarshal(normalized, &msg); err != nil {
			return nil, fmt.Errorf("unmarshal connectionClose: %w", err)
		}
		return &shipClassification{
			MsgType:     model.ShipMsgTypeConnectionClose,
			ShipPayload: data,
		}, nil
	}

	if dataRaw, ok := raw["data"]; ok {
		var shipData shipmodel.ShipData
		normalized := fixupSliceFields(data, shipData)
		if err := json.Unmarshal(normalized, &shipData); err != nil {
			return nil, fmt.Errorf("unmarshal ship data: %w", err)
		}
		return &shipClassification{
			MsgType:     model.ShipMsgTypeData,
			ShipPayload: data,
			DataPayload: extractPayload(dataRaw),
		}, nil
	}

	return &shipClassification{
		MsgType:     model.ShipMsgTypeUnknown,
		ShipPayload: data,
	}, nil
}

// extractPayload pulls the payload field from a data JSON object.
func extractPayload(dataRaw json.RawMessage) json.RawMessage {
	var dataObj struct {
		Header  json.RawMessage `json:"header"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(dataRaw, &dataObj); err != nil {
		return nil
	}
	return dataObj.Payload
}
