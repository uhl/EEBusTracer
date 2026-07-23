package store

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/eebustracer/eebustracer/internal/model"
	"github.com/eebustracer/eebustracer/internal/parser"
)

// buildDLTFrame is a local helper duplicating the parser-package test builder,
// used to feed synthetic .dlt bytes into the store-level importer.
func buildDLTFrame(t *testing.T, apid, ctid, s string) []byte {
	t.Helper()

	strBytes := append([]byte(s), 0)
	strLen := len(strBytes)
	payload := make([]byte, 0, 4+2+strLen)
	typeInfo := make([]byte, 4)
	binary.LittleEndian.PutUint32(typeInfo, 0x00000200) // TYPE_STRING
	payload = append(payload, typeInfo...)
	lenField := make([]byte, 2)
	binary.LittleEndian.PutUint16(lenField, uint16(strLen))
	payload = append(payload, lenField...)
	payload = append(payload, strBytes...)

	pad4 := func(x string) []byte {
		b := make([]byte, 4)
		copy(b, x)
		return b
	}

	ext := make([]byte, 10)
	ext[0] = 0x01 // MSIN: verbose bit
	ext[1] = 1    // noar
	copy(ext[2:6], pad4(apid))
	copy(ext[6:10], pad4(ctid))

	ecu := pad4("ECU1")
	wtms := []byte{0, 0, 0, 0}

	stdBody := append([]byte{}, ecu...)
	stdBody = append(stdBody, wtms...)
	stdBody = append(stdBody, ext...)
	stdBody = append(stdBody, payload...)

	msgLen := uint16(4 + len(stdBody))
	std := make([]byte, 4)
	std[0] = 0x01 | 0x04 | 0x10 // UEH | WEID | WTMS
	std[1] = 0
	binary.BigEndian.PutUint16(std[2:4], msgLen)

	storage := make([]byte, 16)
	copy(storage[0:4], []byte("DLT\x01"))
	binary.LittleEndian.PutUint32(storage[4:8], 1700000000)
	binary.LittleEndian.PutUint32(storage[8:12], 123456)
	copy(storage[12:16], pad4("ECU1"))

	frame := append([]byte{}, storage...)
	frame = append(frame, std...)
	frame = append(frame, stdBody...)
	return frame
}

const testLogData = `15 [11:38:26.008] SEND to ship_Volvo-CEM-400000270_0xaff223b8 MSG: {"datagram":[{"header":[{"specificationVersion":"1.3.0"},{"addressSource":[{"device":"d:_i:_Volvo-00000122"},{"entity":[0]},{"feature":0}]},{"addressDestination":[{"device":"d:_i:37916_CEM-400000270"},{"entity":[0]},{"feature":0}]},{"msgCounter":21},{"cmdClassifier":"read"},{"ackRequest":true}]},{"payload":[{"cmd":[[{"nodeManagementDetailedDiscoveryData":[]}]]}]}]}
16 [11:38:26.016] RECV from ship_Volvo-CEM-400000270_0xaff223b8 MSG: {"datagram":[{"header":[{"specificationVersion":"1.3.0"},{"addressSource":[{"device":"d:_i:37916_CEM-400000270"},{"entity":[2]},{"feature":3}]},{"addressDestination":[{"device":"d:_i:_Volvo-00000122"},{"entity":[1]},{"feature":1}]},{"msgCounter":6},{"cmdClassifier":"read"}]},{"payload":[{"cmd":[[{"deviceClassificationManufacturerData":[]}]]}]}]}
17 [11:38:26.030] RECV from ship_Volvo-CEM-400000270_0xaff223b8 MSG: {"datagram":[{"header":[{"specificationVersion":"1.3.0"},{"addressSource":[{"device":"d:_i:37916_CEM-400000270"},{"entity":[2]},{"feature":5}]},{"addressDestination":[{"device":"d:_i:_Volvo-00000122"},{"entity":[1]},{"feature":2}]},{"msgCounter":7},{"cmdClassifier":"read"}]},{"payload":[{"cmd":[[{"deviceDiagnosisStateData":[]}]]}]}]}
28 [11:38:26.280] SEN This is a malformed line that should be skipped`

func TestImportLogFile(t *testing.T) {
	trace, messages, err := ImportLogFile(strings.NewReader(testLogData), "test-trace")
	if err != nil {
		t.Fatalf("ImportLogFile failed: %v", err)
	}

	if trace.Name != "test-trace" {
		t.Errorf("trace name = %q, want %q", trace.Name, "test-trace")
	}
	if trace.MessageCount != 3 {
		t.Errorf("trace.MessageCount = %d, want 3", trace.MessageCount)
	}
	if len(messages) != 3 {
		t.Fatalf("len(messages) = %d, want 3", len(messages))
	}

	// First message: SEND → outgoing
	m0 := messages[0]
	if m0.SequenceNum != 15 {
		t.Errorf("m0.SequenceNum = %d, want 15", m0.SequenceNum)
	}
	if m0.Direction != model.DirectionOutgoing {
		t.Errorf("m0.Direction = %q, want %q", m0.Direction, model.DirectionOutgoing)
	}
	if m0.CmdClassifier != "read" {
		t.Errorf("m0.CmdClassifier = %q, want %q", m0.CmdClassifier, "read")
	}
	if m0.FunctionSet != "NodeManagementDetailedDiscoveryData" {
		t.Errorf("m0.FunctionSet = %q, want %q", m0.FunctionSet, "NodeManagementDetailedDiscoveryData")
	}
	if m0.MsgCounter != "21" {
		t.Errorf("m0.MsgCounter = %q, want %q", m0.MsgCounter, "21")
	}
	if m0.DeviceSource != "d:_i:_Volvo-00000122" {
		t.Errorf("m0.DeviceSource = %q, want %q", m0.DeviceSource, "d:_i:_Volvo-00000122")
	}
	if m0.DeviceDest != "d:_i:37916_CEM-400000270" {
		t.Errorf("m0.DeviceDest = %q, want %q", m0.DeviceDest, "d:_i:37916_CEM-400000270")
	}
	if m0.ShipMsgType != model.ShipMsgTypeData {
		t.Errorf("m0.ShipMsgType = %q, want %q", m0.ShipMsgType, model.ShipMsgTypeData)
	}

	// Second message: RECV → incoming
	m1 := messages[1]
	if m1.Direction != model.DirectionIncoming {
		t.Errorf("m1.Direction = %q, want %q", m1.Direction, model.DirectionIncoming)
	}
	if m1.FunctionSet != "DeviceClassificationManufacturerData" {
		t.Errorf("m1.FunctionSet = %q, want %q", m1.FunctionSet, "DeviceClassificationManufacturerData")
	}
	if m1.MsgCounter != "6" {
		t.Errorf("m1.MsgCounter = %q, want %q", m1.MsgCounter, "6")
	}

	// Third message
	m2 := messages[2]
	if m2.FunctionSet != "DeviceDiagnosisStateData" {
		t.Errorf("m2.FunctionSet = %q, want %q", m2.FunctionSet, "DeviceDiagnosisStateData")
	}
}

// SHIP-framed log lines (full {"data":[{"header":...},{"payload":{"datagram":...}}]} envelope)
// must parse into a fully populated SPINE message, not fall through with an empty
// CmdClassifier/FunctionSet and a "could not parse SPINE datagram" error.
const testShipFramedLogData = `1 [11:52:30.315] RECV from ship_Brusa_ICS_GPM_0x0000 MSG: {"data":[{"header":[{"protocolId":"ee1.0"}]},{"payload":{"datagram":[{"header":[{"specificationVersion":"1.3.0"},{"addressSource":[{"device":"d:_i:Brusa_ICS_GPM"},{"entity":[0]},{"feature":0}]},{"addressDestination":[{"entity":[0]},{"feature":0}]},{"msgCounter":1},{"cmdClassifier":"read"},{"ackRequest":true}]},{"payload":[{"cmd":[[{"nodeManagementDetailedDiscoveryData":[]}]]}]}]}}]}`

func TestImportLogFile_ShipFramed(t *testing.T) {
	_, messages, err := ImportLogFile(strings.NewReader(testShipFramedLogData), "ship-framed")
	if err != nil {
		t.Fatalf("ImportLogFile failed: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(messages))
	}
	m := messages[0]
	if m.ParseError != "" {
		t.Errorf("m.ParseError = %q, want empty", m.ParseError)
	}
	if m.CmdClassifier != "read" {
		t.Errorf("m.CmdClassifier = %q, want %q", m.CmdClassifier, "read")
	}
	if m.FunctionSet != "NodeManagementDetailedDiscoveryData" {
		t.Errorf("m.FunctionSet = %q, want %q", m.FunctionSet, "NodeManagementDetailedDiscoveryData")
	}
	if m.MsgCounter != "1" {
		t.Errorf("m.MsgCounter = %q, want %q", m.MsgCounter, "1")
	}
	if m.DeviceSource != "d:_i:Brusa_ICS_GPM" {
		t.Errorf("m.DeviceSource = %q, want %q", m.DeviceSource, "d:_i:Brusa_ICS_GPM")
	}
	if m.ShipMsgType != model.ShipMsgTypeData {
		t.Errorf("m.ShipMsgType = %q, want %q", m.ShipMsgType, model.ShipMsgTypeData)
	}
}

func TestImportLogFile_EmptyFile(t *testing.T) {
	_, _, err := ImportLogFile(strings.NewReader(""), "empty")
	if err == nil {
		t.Error("expected error for empty file")
	}
}

func TestImportLogFile_AllMalformed(t *testing.T) {
	data := "not a valid line\nalso not valid\n"
	_, _, err := ImportLogFile(strings.NewReader(data), "bad")
	if err == nil {
		t.Error("expected error for all malformed lines")
	}
}

func TestExtractPeerDevice(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"ship_Volvo-CEM-400000270_0xaff223b8", "Volvo-CEM-400000270"},
		{"ship_MyDevice_0x12345678", "MyDevice"},
		{"plain-peer", "plain-peer"},
	}
	for _, tt := range tests {
		got := parser.ExtractPeerDevice(tt.input)
		if got != tt.want {
			t.Errorf("ExtractPeerDevice(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

const testEEBusTesterLogData = `[20260206 11:54:06.816] - INFO - TESTER - + END KW: Init Loggers (37 ms)
[20260206 11:54:07.811] - DEBUG - USECASE - Tester_EG - Successfully added Feature /0/0
[20260206 11:54:15.338] - INFO - DATAGRAM - Tester_EG - Send message to 'ship_ghostONE-Wallbox-00001616_0x76ce84000df0': {"datagram":[{"header":[{"specificationVersion":"1.3.0"},{"addressSource":[{"device":"d:_i:46925_EEBUS-Tester"},{"entity":[0]},{"feature":0}]},{"addressDestination":[{"entity":[0]},{"feature":0}]},{"msgCounter":1},{"cmdClassifier":"read"},{"ackRequest":true}]},{"payload":[{"cmd":[[{"nodeManagementDetailedDiscoveryData":[]}]]}]}]}
[20260206 11:54:15.450] - INFO - DATAGRAM - Tester_EG - Received message from 'ship_ghostONE-Wallbox-00001616_0x76ce84000df0': {"datagram":[{"header":[{"specificationVersion":"1.3.0"},{"addressSource":[{"device":"d:_i:60745_ghostONE-00001616"},{"entity":[0]},{"feature":0}]},{"addressDestination":[{"entity":[0]},{"feature":0}]},{"msgCounter":2810},{"cmdClassifier":"read"},{"ackRequest":true}]},{"payload":[{"cmd":[[{"nodeManagementDetailedDiscoveryData":[]}]]}]}]}
[20260206 11:54:16.100] - INFO - DATAGRAM - Tester_EG - Send message to 'ship_ghostONE-Wallbox-00001616_0x76ce84000df0': {"datagram":[{"header":[{"specificationVersion":"1.3.0"},{"addressSource":[{"device":"d:_i:46925_EEBUS-Tester"},{"entity":[0]},{"feature":0}]},{"addressDestination":[{"device":"d:_i:60745_ghostONE-00001616"},{"entity":[0]},{"feature":0}]},{"msgCounter":2},{"cmdClassifier":"reply"}]},{"payload":[{"cmd":[[{"nodeManagementDetailedDiscoveryData":[]}]]}]}]}`

func TestImportEEBusTesterLogFile(t *testing.T) {
	trace, messages, err := ImportEEBusTesterLogFile(strings.NewReader(testEEBusTesterLogData), "eebustester-test")
	if err != nil {
		t.Fatalf("ImportEEBusTesterLogFile failed: %v", err)
	}

	if trace.Name != "eebustester-test" {
		t.Errorf("trace name = %q, want %q", trace.Name, "eebustester-test")
	}
	if trace.MessageCount != 3 {
		t.Errorf("trace.MessageCount = %d, want 3", trace.MessageCount)
	}
	if len(messages) != 3 {
		t.Fatalf("len(messages) = %d, want 3", len(messages))
	}

	// First message: Send → outgoing, auto-generated seq=1
	m0 := messages[0]
	if m0.SequenceNum != 1 {
		t.Errorf("m0.SequenceNum = %d, want 1", m0.SequenceNum)
	}
	if m0.Direction != model.DirectionOutgoing {
		t.Errorf("m0.Direction = %q, want %q", m0.Direction, model.DirectionOutgoing)
	}
	if m0.CmdClassifier != "read" {
		t.Errorf("m0.CmdClassifier = %q, want %q", m0.CmdClassifier, "read")
	}
	if m0.FunctionSet != "NodeManagementDetailedDiscoveryData" {
		t.Errorf("m0.FunctionSet = %q, want %q", m0.FunctionSet, "NodeManagementDetailedDiscoveryData")
	}
	if m0.MsgCounter != "1" {
		t.Errorf("m0.MsgCounter = %q, want %q", m0.MsgCounter, "1")
	}
	if m0.DeviceSource != "d:_i:46925_EEBUS-Tester" {
		t.Errorf("m0.DeviceSource = %q, want %q", m0.DeviceSource, "d:_i:46925_EEBUS-Tester")
	}
	if m0.ShipMsgType != model.ShipMsgTypeData {
		t.Errorf("m0.ShipMsgType = %q, want %q", m0.ShipMsgType, model.ShipMsgTypeData)
	}

	// Verify timestamp has embedded date
	if m0.Timestamp.Year() != 2026 || m0.Timestamp.Month() != 2 || m0.Timestamp.Day() != 6 {
		t.Errorf("m0.Timestamp date = %v, want 2026-02-06", m0.Timestamp.Format("2006-01-02"))
	}

	// Second message: Received → incoming, auto-generated seq=2
	m1 := messages[1]
	if m1.SequenceNum != 2 {
		t.Errorf("m1.SequenceNum = %d, want 2", m1.SequenceNum)
	}
	if m1.Direction != model.DirectionIncoming {
		t.Errorf("m1.Direction = %q, want %q", m1.Direction, model.DirectionIncoming)
	}
	if m1.MsgCounter != "2810" {
		t.Errorf("m1.MsgCounter = %q, want %q", m1.MsgCounter, "2810")
	}

	// Third message: Send → outgoing, seq=3
	m2 := messages[2]
	if m2.SequenceNum != 3 {
		t.Errorf("m2.SequenceNum = %d, want 3", m2.SequenceNum)
	}
	if m2.CmdClassifier != "reply" {
		t.Errorf("m2.CmdClassifier = %q, want %q", m2.CmdClassifier, "reply")
	}
}

func TestImportEEBusTesterLogFile_EmptyFile(t *testing.T) {
	_, _, err := ImportEEBusTesterLogFile(strings.NewReader(""), "empty")
	if err == nil {
		t.Error("expected error for empty file")
	}
}

func TestImportEEBusTesterLogFile_NoDatagram(t *testing.T) {
	data := "[20260206 11:54:06.816] - INFO - TESTER - no datagrams here\n[20260206 11:54:07.811] - DEBUG - USECASE - Tester_EG - line\n"
	_, _, err := ImportEEBusTesterLogFile(strings.NewReader(data), "no-datagram")
	if err == nil {
		t.Error("expected error when no DATAGRAM lines found")
	}
}

func TestImportLogFileAutoDetect_EEBusGo(t *testing.T) {
	trace, messages, err := ImportLogFileAutoDetect(strings.NewReader(testLogData), "auto-eebusgo")
	if err != nil {
		t.Fatalf("ImportLogFileAutoDetect failed: %v", err)
	}
	if trace.Name != "auto-eebusgo" {
		t.Errorf("trace name = %q, want %q", trace.Name, "auto-eebusgo")
	}
	if len(messages) != 3 {
		t.Errorf("len(messages) = %d, want 3", len(messages))
	}
}

func TestImportLogFileAutoDetect_EEBusTester(t *testing.T) {
	trace, messages, err := ImportLogFileAutoDetect(strings.NewReader(testEEBusTesterLogData), "auto-tester")
	if err != nil {
		t.Fatalf("ImportLogFileAutoDetect failed: %v", err)
	}
	if trace.Name != "auto-tester" {
		t.Errorf("trace name = %q, want %q", trace.Name, "auto-tester")
	}
	if len(messages) != 3 {
		t.Errorf("len(messages) = %d, want 3", len(messages))
	}
}

func TestImportLogFileAutoDetect_UnknownFormat(t *testing.T) {
	_, _, err := ImportLogFileAutoDetect(strings.NewReader("random content\nno log format\n"), "unknown")
	if err == nil {
		t.Error("expected error for unknown format")
	}
}

const testCEasierLoggerData = `[11:38:26.008] SEND to ship_Volvo-CEM-400000270_0xaff223b8 MSG: {"datagram":[{"header":[{"specificationVersion":"1.3.0"},{"addressSource":[{"device":"d:_i:_Volvo-00000122"},{"entity":[0]},{"feature":0}]},{"addressDestination":[{"device":"d:_i:37916_CEM-400000270"},{"entity":[0]},{"feature":0}]},{"msgCounter":21},{"cmdClassifier":"read"},{"ackRequest":true}]},{"payload":[{"cmd":[[{"nodeManagementDetailedDiscoveryData":[]}]]}]}]}
[11:38:26.016] RECV from ship_Volvo-CEM-400000270_0xaff223b8 MSG: {"datagram":[{"header":[{"specificationVersion":"1.3.0"},{"addressSource":[{"device":"d:_i:37916_CEM-400000270"},{"entity":[2]},{"feature":3}]},{"addressDestination":[{"device":"d:_i:_Volvo-00000122"},{"entity":[1]},{"feature":1}]},{"msgCounter":6},{"cmdClassifier":"read"}]},{"payload":[{"cmd":[[{"deviceClassificationManufacturerData":[]}]]}]}]}
[11:38:26.030] RECV from ship_Volvo-CEM-400000270_0xaff223b8 MSG: {"datagram":[{"header":[{"specificationVersion":"1.3.0"},{"addressSource":[{"device":"d:_i:37916_CEM-400000270"},{"entity":[2]},{"feature":5}]},{"addressDestination":[{"device":"d:_i:_Volvo-00000122"},{"entity":[1]},{"feature":2}]},{"msgCounter":7},{"cmdClassifier":"read"}]},{"payload":[{"cmd":[[{"deviceDiagnosisStateData":[]}]]}]}]}`

func TestImportLogFile_NoSequenceNumbers(t *testing.T) {
	trace, messages, err := ImportLogFile(strings.NewReader(testCEasierLoggerData), "ceasierlogger-test")
	if err != nil {
		t.Fatalf("ImportLogFile failed: %v", err)
	}

	if trace.Name != "ceasierlogger-test" {
		t.Errorf("trace name = %q, want %q", trace.Name, "ceasierlogger-test")
	}
	if trace.MessageCount != 3 {
		t.Errorf("trace.MessageCount = %d, want 3", trace.MessageCount)
	}
	if len(messages) != 3 {
		t.Fatalf("len(messages) = %d, want 3", len(messages))
	}

	// Verify auto-generated sequence numbers (1, 2, 3)
	for i, msg := range messages {
		wantSeq := i + 1
		if msg.SequenceNum != wantSeq {
			t.Errorf("messages[%d].SequenceNum = %d, want %d", i, msg.SequenceNum, wantSeq)
		}
	}

	// First message: SEND → outgoing
	m0 := messages[0]
	if m0.Direction != model.DirectionOutgoing {
		t.Errorf("m0.Direction = %q, want %q", m0.Direction, model.DirectionOutgoing)
	}
	if m0.CmdClassifier != "read" {
		t.Errorf("m0.CmdClassifier = %q, want %q", m0.CmdClassifier, "read")
	}
	if m0.FunctionSet != "NodeManagementDetailedDiscoveryData" {
		t.Errorf("m0.FunctionSet = %q, want %q", m0.FunctionSet, "NodeManagementDetailedDiscoveryData")
	}

	// Second message: RECV → incoming
	m1 := messages[1]
	if m1.Direction != model.DirectionIncoming {
		t.Errorf("m1.Direction = %q, want %q", m1.Direction, model.DirectionIncoming)
	}
	if m1.FunctionSet != "DeviceClassificationManufacturerData" {
		t.Errorf("m1.FunctionSet = %q, want %q", m1.FunctionSet, "DeviceClassificationManufacturerData")
	}

	// Third message
	m2 := messages[2]
	if m2.FunctionSet != "DeviceDiagnosisStateData" {
		t.Errorf("m2.FunctionSet = %q, want %q", m2.FunctionSet, "DeviceDiagnosisStateData")
	}
}

func TestImportLogFileAutoDetect_CEasierLogger(t *testing.T) {
	trace, messages, err := ImportLogFileAutoDetect(strings.NewReader(testCEasierLoggerData), "auto-ceasier")
	if err != nil {
		t.Fatalf("ImportLogFileAutoDetect failed: %v", err)
	}
	if trace.Name != "auto-ceasier" {
		t.Errorf("trace name = %q, want %q", trace.Name, "auto-ceasier")
	}
	if len(messages) != 3 {
		t.Errorf("len(messages) = %d, want 3", len(messages))
	}
	// Verify auto-generated sequence numbers
	for i, msg := range messages {
		if msg.SequenceNum != i+1 {
			t.Errorf("messages[%d].SequenceNum = %d, want %d", i, msg.SequenceNum, i+1)
		}
	}
}

const testEEBusHubLogData = `2026-03-16 05:19:57    [Send] 1adbb6152b3902b028b2f4c1b3855777f19fb4f7{"data":[{"header":[{"protocolId":"ee1.0"}]},{"payload":{"datagram":[{"header":[{"specificationVersion":"1.3.0"},{"addressSource":[{"device":"d:_i:_HEMS-01"},{"entity":[0]},{"feature":0}]},{"addressDestination":[{"device":"d:_i:_Wallbox-01"},{"entity":[0]},{"feature":0}]},{"msgCounter":21},{"cmdClassifier":"read"},{"ackRequest":true}]},{"payload":[{"cmd":[[{"nodeManagementDetailedDiscoveryData":[]}]]}]}]}}]}
2026-03-16 05:19:58    [Recv] 1adbb6152b3902b028b2f4c1b3855777f19fb4f7{"data":[{"header":[{"protocolId":"ee1.0"}]},{"payload":{"datagram":[{"header":[{"specificationVersion":"1.3.0"},{"addressSource":[{"device":"d:_i:_Wallbox-01"},{"entity":[2]},{"feature":3}]},{"addressDestination":[{"device":"d:_i:_HEMS-01"},{"entity":[1]},{"feature":1}]},{"msgCounter":6},{"cmdClassifier":"read"}]},{"payload":[{"cmd":[[{"deviceClassificationManufacturerData":[]}]]}]}]}}]}
2026-03-16 05:20:01    [Recv] 1adbb6152b3902b028b2f4c1b3855777f19fb4f7{"data":[{"header":[{"protocolId":"ee1.0"}]},{"payload":{"datagram":[{"header":[{"specificationVersion":"1.3.0"},{"addressSource":[{"device":"d:_i:_Wallbox-01"},{"entity":[2]},{"feature":5}]},{"addressDestination":[{"device":"d:_i:_HEMS-01"},{"entity":[1]},{"feature":2}]},{"msgCounter":7},{"cmdClassifier":"read"}]},{"payload":[{"cmd":[[{"deviceDiagnosisStateData":[]}]]}]}]}}]}
This is a malformed line that should be skipped`

func TestImportEEBusHubLogFile(t *testing.T) {
	trace, messages, err := ImportEEBusHubLogFile(strings.NewReader(testEEBusHubLogData), "eebushub-test")
	if err != nil {
		t.Fatalf("ImportEEBusHubLogFile failed: %v", err)
	}

	if trace.Name != "eebushub-test" {
		t.Errorf("trace name = %q, want %q", trace.Name, "eebushub-test")
	}
	if trace.MessageCount != 3 {
		t.Errorf("trace.MessageCount = %d, want 3", trace.MessageCount)
	}
	if len(messages) != 3 {
		t.Fatalf("len(messages) = %d, want 3", len(messages))
	}

	// First message: Send → outgoing, auto-generated seq=1
	m0 := messages[0]
	if m0.SequenceNum != 1 {
		t.Errorf("m0.SequenceNum = %d, want 1", m0.SequenceNum)
	}
	if m0.Direction != model.DirectionOutgoing {
		t.Errorf("m0.Direction = %q, want %q", m0.Direction, model.DirectionOutgoing)
	}
	if m0.CmdClassifier != "read" {
		t.Errorf("m0.CmdClassifier = %q, want %q", m0.CmdClassifier, "read")
	}
	if m0.FunctionSet != "NodeManagementDetailedDiscoveryData" {
		t.Errorf("m0.FunctionSet = %q, want %q", m0.FunctionSet, "NodeManagementDetailedDiscoveryData")
	}
	if m0.MsgCounter != "21" {
		t.Errorf("m0.MsgCounter = %q, want %q", m0.MsgCounter, "21")
	}
	if m0.DeviceSource != "d:_i:_HEMS-01" {
		t.Errorf("m0.DeviceSource = %q, want %q", m0.DeviceSource, "d:_i:_HEMS-01")
	}
	if m0.DeviceDest != "d:_i:_Wallbox-01" {
		t.Errorf("m0.DeviceDest = %q, want %q", m0.DeviceDest, "d:_i:_Wallbox-01")
	}
	if m0.ShipMsgType != model.ShipMsgTypeData {
		t.Errorf("m0.ShipMsgType = %q, want %q", m0.ShipMsgType, model.ShipMsgTypeData)
	}

	// Verify timestamp has embedded date
	if m0.Timestamp.Year() != 2026 || m0.Timestamp.Month() != 3 || m0.Timestamp.Day() != 16 {
		t.Errorf("m0.Timestamp date = %v, want 2026-03-16", m0.Timestamp.Format("2006-01-02"))
	}
	if m0.Timestamp.Hour() != 5 || m0.Timestamp.Minute() != 19 || m0.Timestamp.Second() != 57 {
		t.Errorf("m0.Timestamp time = %v, want 05:19:57", m0.Timestamp.Format("15:04:05"))
	}

	// Second message: Recv → incoming, auto-generated seq=2
	m1 := messages[1]
	if m1.SequenceNum != 2 {
		t.Errorf("m1.SequenceNum = %d, want 2", m1.SequenceNum)
	}
	if m1.Direction != model.DirectionIncoming {
		t.Errorf("m1.Direction = %q, want %q", m1.Direction, model.DirectionIncoming)
	}
	if m1.FunctionSet != "DeviceClassificationManufacturerData" {
		t.Errorf("m1.FunctionSet = %q, want %q", m1.FunctionSet, "DeviceClassificationManufacturerData")
	}
	if m1.MsgCounter != "6" {
		t.Errorf("m1.MsgCounter = %q, want %q", m1.MsgCounter, "6")
	}

	// Third message: seq=3
	m2 := messages[2]
	if m2.SequenceNum != 3 {
		t.Errorf("m2.SequenceNum = %d, want 3", m2.SequenceNum)
	}
	if m2.FunctionSet != "DeviceDiagnosisStateData" {
		t.Errorf("m2.FunctionSet = %q, want %q", m2.FunctionSet, "DeviceDiagnosisStateData")
	}
}

func TestImportEEBusHubLogFile_Empty(t *testing.T) {
	_, _, err := ImportEEBusHubLogFile(strings.NewReader(""), "empty")
	if err == nil {
		t.Error("expected error for empty file")
	}
}

func TestImportEEBusHubLogFile_NoMatch(t *testing.T) {
	data := "some random text\nno log format here\n"
	_, _, err := ImportEEBusHubLogFile(strings.NewReader(data), "bad")
	if err == nil {
		t.Error("expected error for all non-matching lines")
	}
}

func TestImportLogFileAutoDetect_EEBusHub(t *testing.T) {
	trace, messages, err := ImportLogFileAutoDetect(strings.NewReader(testEEBusHubLogData), "auto-hub")
	if err != nil {
		t.Fatalf("ImportLogFileAutoDetect failed: %v", err)
	}
	if trace.Name != "auto-hub" {
		t.Errorf("trace name = %q, want %q", trace.Name, "auto-hub")
	}
	if len(messages) != 3 {
		t.Errorf("len(messages) = %d, want 3", len(messages))
	}
}

func TestImportFileAutoDetect_EEBusHub(t *testing.T) {
	trace, messages, err := ImportFileAutoDetect(strings.NewReader(testEEBusHubLogData), "auto-hub")
	if err != nil {
		t.Fatalf("ImportFileAutoDetect failed: %v", err)
	}
	if trace.Name != "auto-hub" {
		t.Errorf("trace name = %q, want %q", trace.Name, "auto-hub")
	}
	if len(messages) != 3 {
		t.Errorf("len(messages) = %d, want 3", len(messages))
	}
}

func TestImportFileAutoDetect_EEBusGo(t *testing.T) {
	trace, messages, err := ImportFileAutoDetect(strings.NewReader(testLogData), "auto-go")
	if err != nil {
		t.Fatalf("ImportFileAutoDetect failed: %v", err)
	}
	if trace.Name != "auto-go" {
		t.Errorf("trace name = %q, want %q", trace.Name, "auto-go")
	}
	if len(messages) != 3 {
		t.Errorf("len(messages) = %d, want 3", len(messages))
	}
}

// Excerpt from a real Porsche CEM DLT text export. Includes:
//   - a Porsche Send line (outgoing SHIP-framed EEBus JSON)
//   - a Porsche Received line (incoming SHIP-framed EEBus JSON)
//   - a truncated line (must be skipped)
//   - non-EEBus telemetry (SVC, HEMS ShipTransport) that must be skipped
const testDLTTextLogData = `7390 2026/07/22 11:52:30.315139 162336.0780 252 ECU1 CEM CEM 668 log info verbose 1 : [Session 38099] Send: {"data":[{"header":[{"protocolId":"ee1.0"}]},{"payload":{"datagram":[{"header":[{"specificationVersion":"1.3.0"},{"addressSource":[{"device":"d:_i:19667_Porsche-CEM"},{"entity":[0]},{"feature":0}]},{"addressDestination":[{"device":"d:_i:Brusa_ICS_GPM"},{"entity":[0]},{"feature":0}]},{"msgCounter":2},{"msgCounterReference":1},{"cmdClassifier":"reply"}]},{"payload":[{"cmd":[[{"nodeManagementDetailedDiscoveryData":[]}]]}]}]}}]}
7413 2026/07/22 11:52:30.548651 162336.2020 17 ECU1 CEM CEM 668 log info verbose 1 : [ConnectionWorker 38099] Received 654 Data bytes during ConnectionDataExchange: {"data":[{"header":[{"protocolId":"ee1.0"}]},{"payload":{"datagram":[{"header":[{"specificationVersion":"1.3.0"},{"addressSource":[{"device":"d:_i:Brusa_ICS_GPM"},{"entity":[0]},{"feature":0}]},{"addressDestination":[{"device":"d:_i:19667_Porsche-CEM"},{"entity":[0]},{"feature":0}]},{"msgCounter":5},{"cmdClassifier":"call"},{"ackRequest":true}]},{"payload":[{"cmd":[[{"nodeManagementSubscriptionRequestCall":[]}]]}]}]}}]}
7415 2026/07/22 11:52:30.548651 162336.2923 19 ECU1 CEM CEM 668 log info verbose 1 : [ConnectionWorker 38099] Received 1949 Data bytes during ConnectionDataExchange: {"data":[{"header":[{"protocolId":"ee1.0"}]}]} <<Message truncated, too long>>
7770 2026/07/22 11:52:50.916676 162356.5740 86 ECU1 CEM SVC 668 log info verbose 1 #rtm powTot=9839
7941 2026/07/22 11:53:03.862024 162369.6056 108 ECU1 HEMS HEMS 434 log info verbose 1 : [ShipTransport] tryReconnect`

func TestImportDLTTextLogFile(t *testing.T) {
	trace, messages, err := ImportDLTTextLogFile(strings.NewReader(testDLTTextLogData), "porsche")
	if err != nil {
		t.Fatalf("ImportDLTTextLogFile failed: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("len(messages) = %d, want 2 (truncated + SVC + ShipTransport must be skipped)", len(messages))
	}
	if trace.Name != "porsche" {
		t.Errorf("trace name = %q, want porsche", trace.Name)
	}

	m0 := messages[0]
	if m0.Direction != model.DirectionOutgoing {
		t.Errorf("m0.Direction = %q, want outgoing", m0.Direction)
	}
	if m0.CmdClassifier != "reply" {
		t.Errorf("m0.CmdClassifier = %q, want reply", m0.CmdClassifier)
	}
	if m0.DeviceSource != "d:_i:19667_Porsche-CEM" {
		t.Errorf("m0.DeviceSource = %q", m0.DeviceSource)
	}

	m1 := messages[1]
	if m1.Direction != model.DirectionIncoming {
		t.Errorf("m1.Direction = %q, want incoming", m1.Direction)
	}
	if m1.CmdClassifier != "call" {
		t.Errorf("m1.CmdClassifier = %q, want call", m1.CmdClassifier)
	}
	// Sanity: timestamp resolution should be microseconds.
	if m0.Timestamp.Nanosecond() != 315139000 {
		t.Errorf("m0 nanos = %d, want 315139000", m0.Timestamp.Nanosecond())
	}
}

func TestImportDLTTextLogFile_NoEEBus(t *testing.T) {
	// Only telemetry, no extractable EEBus content.
	data := `7770 2026/07/22 11:52:50.916676 162356.5740 86 ECU1 CEM SVC 668 log info verbose 1 #rtm powTot=9839`
	_, _, err := ImportDLTTextLogFile(strings.NewReader(data), "empty")
	if err == nil {
		t.Error("expected error when no EEBus messages found")
	}
}

func TestImportFileAutoDetect_DLTText(t *testing.T) {
	trace, messages, err := ImportFileAutoDetect(strings.NewReader(testDLTTextLogData), "auto-dlt")
	if err != nil {
		t.Fatalf("ImportFileAutoDetect failed: %v", err)
	}
	if trace.Name != "auto-dlt" {
		t.Errorf("trace name = %q", trace.Name)
	}
	if len(messages) != 2 {
		t.Errorf("len(messages) = %d, want 2", len(messages))
	}
}

func TestImportDLTBinaryFile_SkipTruncated(t *testing.T) {
	// A frame whose string arg was cut off mid-JSON by DLT's fixed width.
	// Must be skipped entirely rather than stored with a parse error,
	// AND counted in trace.SkippedTruncated so the user sees "something was
	// here" in the trace header.
	truncated := `{"data":{"header":{"protocolId":"ee1.0"},"payload":{"datagram":{"header":{"specificationVersion":"1.3.0","cmdCla`
	f1 := buildDLTFrame(t, "CEM", "CEM", truncated)
	// A second truncated frame — count should be 2.
	f1b := buildDLTFrame(t, "CEM", "CEM", truncated)
	// A complete frame alongside them should still import.
	complete := `[Session 1] Send: {"datagram":[{"header":[{"specificationVersion":"1.3.0"},{"addressSource":[{"device":"d:_i:A"},{"entity":[0]},{"feature":0}]},{"addressDestination":[{"device":"d:_i:B"},{"entity":[0]},{"feature":0}]},{"msgCounter":1},{"cmdClassifier":"read"}]},{"payload":[{"cmd":[[{"nodeManagementDetailedDiscoveryData":[]}]]}]}]}`
	f2 := buildDLTFrame(t, "CEM", "CEM", complete)

	buf := append([]byte{}, f1...)
	buf = append(buf, f1b...)
	buf = append(buf, f2...)

	trace, messages, err := ImportDLTBinaryFile(bytes.NewReader(buf), "trunc")
	if err != nil {
		t.Fatalf("ImportDLTBinaryFile: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1 (truncated frames must be skipped)", len(messages))
	}
	if messages[0].CmdClassifier != "read" {
		t.Errorf("CmdClassifier = %q, want read", messages[0].CmdClassifier)
	}
	if trace.SkippedTruncated != 2 {
		t.Errorf("trace.SkippedTruncated = %d, want 2", trace.SkippedTruncated)
	}
}

func TestImportDLTTextLogFile_CountsTruncated(t *testing.T) {
	// The DLT text export appends "<<Message truncated, too long>>" to lines
	// whose payload exceeded the column width. Those must be counted too.
	data := `7390 2026/07/22 11:52:30.315139 162336.0780 252 ECU1 CEM CEM 668 log info verbose 1 : [Session 38099] Send: {"data":[{"header":[{"protocolId":"ee1.0"}]},{"payload":{"datagram":[{"header":[{"specificationVersion":"1.3.0"},{"addressSource":[{"device":"d:_i:CEM"},{"entity":[0]},{"feature":0}]},{"addressDestination":[{"device":"d:_i:EV"},{"entity":[0]},{"feature":0}]},{"msgCounter":1},{"cmdClassifier":"read"}]},{"payload":[{"cmd":[[{"nodeManagementDetailedDiscoveryData":[]}]]}]}]}}]}
7391 2026/07/22 11:52:30.316000 162336.0790 253 ECU1 CEM CEM 668 log info verbose 1 : [Session 38099] Send: {"data":[{"header":[{"protocolId":"ee1.0"}]}]} <<Message truncated, too long>>
7392 2026/07/22 11:52:30.317000 162336.0800 254 ECU1 CEM CEM 668 log info verbose 1 : [Session 38099] Send: {"data":[{"header":[{"protocolId":"ee1.0"}]},{"payload":{"datagram":[{"header":[{"specificationVersion":"1.3.0"},{"addressSource":[{"device":"d:_i:CEM"},{"entity":[0]},{"feature":0}]},{"addressDestination":[{"device":"d:_i:EV"},{"entity":[0]},{"feature":0}]},{"msgCounter":2},{"cmdClassifier":"read"}]},{"payload":[{"cmd":[[{"nodeManagementDetailedDiscoveryData":[]}]]}]}]}}]}`

	trace, messages, err := ImportDLTTextLogFile(strings.NewReader(data), "text-trunc")
	if err != nil {
		t.Fatalf("ImportDLTTextLogFile: %v", err)
	}
	if len(messages) != 2 {
		t.Errorf("len(messages) = %d, want 2", len(messages))
	}
	if trace.SkippedTruncated != 1 {
		t.Errorf("trace.SkippedTruncated = %d, want 1", trace.SkippedTruncated)
	}
}

func TestImportDLTBinaryFile(t *testing.T) {
	// Build two synthetic DLT frames: one with EEBus JSON, one with noise.
	eebusPayload := `[Session 1] Send: {"datagram":[{"header":[{"specificationVersion":"1.3.0"},{"addressSource":[{"device":"d:_i:CEM"},{"entity":[0]},{"feature":0}]},{"addressDestination":[{"device":"d:_i:EV"},{"entity":[0]},{"feature":0}]},{"msgCounter":1},{"cmdClassifier":"read"}]},{"payload":[{"cmd":[[{"nodeManagementDetailedDiscoveryData":[]}]]}]}]}`
	f1 := buildDLTFrame(t, "CEM", "CEM", eebusPayload)
	f2 := buildDLTFrame(t, "CEM", "SVC", `#rtm powTot=9839`)

	buf := append([]byte{}, f1...)
	buf = append(buf, f2...)

	trace, messages, err := ImportDLTBinaryFile(bytes.NewReader(buf), "dlt-bin")
	if err != nil {
		t.Fatalf("ImportDLTBinaryFile: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1 (SVC line must be skipped)", len(messages))
	}
	if trace.Name != "dlt-bin" {
		t.Errorf("trace name = %q", trace.Name)
	}
	m := messages[0]
	if m.Direction != model.DirectionOutgoing {
		t.Errorf("Direction = %q", m.Direction)
	}
	if m.CmdClassifier != "read" {
		t.Errorf("CmdClassifier = %q", m.CmdClassifier)
	}
	if m.DeviceSource != "d:_i:CEM" {
		t.Errorf("DeviceSource = %q", m.DeviceSource)
	}
}

func TestImportFileAutoDetect_DLTBinary(t *testing.T) {
	eebusPayload := `[ConnectionWorker 1] Received 100 Data bytes during ConnectionDataExchange: {"datagram":[{"header":[{"specificationVersion":"1.3.0"},{"addressSource":[{"device":"d:_i:EV"},{"entity":[0]},{"feature":0}]},{"addressDestination":[{"device":"d:_i:CEM"},{"entity":[0]},{"feature":0}]},{"msgCounter":42},{"cmdClassifier":"reply"}]},{"payload":[{"cmd":[[{"nodeManagementDetailedDiscoveryData":[]}]]}]}]}`
	frame := buildDLTFrame(t, "CEM", "CEM", eebusPayload)

	trace, messages, err := ImportFileAutoDetect(bytes.NewReader(frame), "auto-dlt-bin")
	if err != nil {
		t.Fatalf("ImportFileAutoDetect: %v", err)
	}
	if trace.Name != "auto-dlt-bin" {
		t.Errorf("trace name = %q", trace.Name)
	}
	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(messages))
	}
	if messages[0].Direction != model.DirectionIncoming {
		t.Errorf("Direction = %q, want incoming", messages[0].Direction)
	}
	if messages[0].CmdClassifier != "reply" {
		t.Errorf("CmdClassifier = %q", messages[0].CmdClassifier)
	}
}
