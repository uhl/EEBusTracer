package parser

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
	"time"
)

// buildDLTFrame constructs a synthetic .dlt file frame:
//   - storage header (magic + secs + usecs + ecu)
//   - standard header htyp=UEH|WEID|WTMS, mcnt=0, len (BE)
//   - WEID ecu id (4B)
//   - WTMS timestamp (4B, unused)
//   - extended header: msin=verbose, noar=1, apid, ctid
//   - one string arg: type_info=STRING, length, bytes+nul
func buildDLTFrame(t *testing.T, apid, ctid, s string) []byte {
	t.Helper()

	// Payload: 4B type_info + 2B length + string bytes + 1B nul
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

	// Extended header (10B)
	ext := make([]byte, 10)
	ext[0] = 0x01 // MSIN: verbose bit
	ext[1] = 1    // noar
	copy(ext[2:6], padTo4(apid))
	copy(ext[6:10], padTo4(ctid))

	// WEID ecu (4B)
	ecu := padTo4("ECU1")
	// WTMS timestamp (4B) = 0
	wtms := []byte{0, 0, 0, 0}

	// Standard header: 4B (htyp + mcnt + len)
	stdBody := append([]byte{}, ecu...)
	stdBody = append(stdBody, wtms...)
	stdBody = append(stdBody, ext...)
	stdBody = append(stdBody, payload...)

	msgLen := uint16(4 + len(stdBody))
	std := make([]byte, 4)
	std[0] = htypUEH | htypWEID | htypWTMS
	std[1] = 0 // mcnt
	binary.BigEndian.PutUint16(std[2:4], msgLen)

	// Storage header (16B)
	storage := make([]byte, 16)
	copy(storage[0:4], []byte(dltStorageMagic))
	binary.LittleEndian.PutUint32(storage[4:8], 1700000000)
	binary.LittleEndian.PutUint32(storage[8:12], 123456)
	copy(storage[12:16], padTo4("ECU1"))

	frame := append([]byte{}, storage...)
	frame = append(frame, std...)
	frame = append(frame, stdBody...)
	return frame
}

func padTo4(s string) []byte {
	b := make([]byte, 4)
	copy(b, s)
	return b
}

func TestReadDLTMessage_VerboseString(t *testing.T) {
	frame := buildDLTFrame(t, "CEM", "CEM", `[Session 42] Send: {"data":[{"header":[{"protocolId":"ee1.0"}]}]}`)
	msg, err := ReadDLTMessage(bytes.NewReader(frame))
	if err != nil {
		t.Fatalf("ReadDLTMessage: %v", err)
	}
	if msg.APID != "CEM" || msg.CTID != "CEM" {
		t.Errorf("apid/ctid = %q/%q, want CEM/CEM", msg.APID, msg.CTID)
	}
	if !msg.Verbose {
		t.Errorf("expected verbose")
	}
	if msg.ECUID != "ECU1" {
		t.Errorf("ecuid = %q", msg.ECUID)
	}
	if msg.Args[:11] != "[Session 42" {
		t.Errorf("args = %q...", msg.Args[:20])
	}
}

func TestReadDLTMessage_MultipleFrames(t *testing.T) {
	f1 := buildDLTFrame(t, "CEM", "CEM", `first`)
	f2 := buildDLTFrame(t, "HEMS", "HEMS", `second`)
	buf := bytes.NewReader(append(f1, f2...))

	m1, err := ReadDLTMessage(buf)
	if err != nil {
		t.Fatalf("first read: %v", err)
	}
	if m1.Args != "first" {
		t.Errorf("m1.Args = %q", m1.Args)
	}
	m2, err := ReadDLTMessage(buf)
	if err != nil {
		t.Fatalf("second read: %v", err)
	}
	if m2.Args != "second" || m2.APID != "HEMS" {
		t.Errorf("m2 = %+v", m2)
	}

	// Third read should hit EOF.
	if _, err := ReadDLTMessage(buf); err != io.EOF {
		t.Errorf("expected io.EOF, got %v", err)
	}
}

// buildDLTStreamFrame is like buildDLTFrame but omits the storage header, as
// used on live TCP wire (dlt-daemon default port 3490).
func buildDLTStreamFrame(t *testing.T, apid, ctid, s string) []byte {
	t.Helper()
	full := buildDLTFrame(t, apid, ctid, s)
	// Strip the 16-byte storage header.
	return full[16:]
}

func TestReadDLTMessageStream_VerboseString(t *testing.T) {
	frame := buildDLTStreamFrame(t, "CEM", "CEM", `[Session 42] Send: hi`)
	fallback := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	msg, err := ReadDLTMessageStream(bytes.NewReader(frame), fallback)
	if err != nil {
		t.Fatalf("ReadDLTMessageStream: %v", err)
	}
	if msg.APID != "CEM" || msg.CTID != "CEM" {
		t.Errorf("apid/ctid = %q/%q", msg.APID, msg.CTID)
	}
	if !msg.Verbose {
		t.Errorf("expected verbose")
	}
	if !msg.Timestamp.Equal(fallback) {
		t.Errorf("Timestamp = %v, want fallback %v", msg.Timestamp, fallback)
	}
	if msg.Args != "[Session 42] Send: hi" {
		t.Errorf("Args = %q", msg.Args)
	}
}

func TestReadDLTMessageStream_MultipleFrames(t *testing.T) {
	f1 := buildDLTStreamFrame(t, "CEM", "CEM", `first`)
	f2 := buildDLTStreamFrame(t, "HEMS", "HEMS", `second`)
	buf := bytes.NewReader(append(f1, f2...))
	ts := time.Now().UTC()

	m1, err := ReadDLTMessageStream(buf, ts)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if m1.Args != "first" {
		t.Errorf("m1.Args = %q", m1.Args)
	}
	m2, err := ReadDLTMessageStream(buf, ts)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if m2.Args != "second" || m2.APID != "HEMS" {
		t.Errorf("m2 = %+v", m2)
	}
	if _, err := ReadDLTMessageStream(buf, ts); err != io.EOF {
		t.Errorf("expected io.EOF, got %v", err)
	}
}

func TestReadDLTMessage_BadMagic(t *testing.T) {
	// 16 bytes of garbage → invalid magic error.
	buf := bytes.NewReader(bytes.Repeat([]byte{'x'}, 16))
	if _, err := ReadDLTMessage(buf); err == nil {
		t.Error("expected error on bad magic")
	}
}
