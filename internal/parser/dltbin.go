package parser

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
)

// DLT binary format constants. Reference: AUTOSAR PRS_LogAndTraceProtocol.
const (
	dltStorageMagic  = "DLT\x01"
	dltStorageHeader = 16 // 4 magic + 4 sec + 4 usec + 4 ecu

	// Standard header htyp flag bits.
	htypUEH  = 1 << 0 // Use Extended Header
	htypMSBF = 1 << 1 // MSB First (big-endian payload)
	htypWEID = 1 << 2 // With ECU ID
	htypWSID = 1 << 3 // With Session ID
	htypWTMS = 1 << 4 // With Timestamp

	// MSIN bit 0 = verbose.
	msinVerbose = 1 << 0

	// Argument type info bits (bits 0..3 = TYLE length; bit 9 = TYPE_STRING).
	typeInfoStringMask = 0x00000200
)

// DLTBinaryMessage is a decoded DLT frame relevant to EEBus extraction.
type DLTBinaryMessage struct {
	Timestamp time.Time // storage-header wall clock (from file) or zero on wire
	ECUID     string    // from extended header if present, else standard header
	APID      string    // extended header (empty if UEH bit clear)
	CTID      string    // extended header
	Verbose   bool
	// Args is the concatenation of all string arguments in the payload,
	// separated by spaces. Non-string args are skipped. This matches how DLT
	// Viewer renders the plain-text payload column.
	Args string
}

// ReadDLTMessage decodes a single DLT frame from r, expecting a leading
// 16-byte storage header (`.dlt` file format). Returns io.EOF cleanly at
// end-of-stream. Returns a non-EOF error on truncated / malformed frames.
func ReadDLTMessage(r io.Reader) (*DLTBinaryMessage, error) {
	var storage [dltStorageHeader]byte
	if _, err := io.ReadFull(r, storage[:]); err != nil {
		return nil, err
	}
	if string(storage[0:4]) != dltStorageMagic {
		return nil, fmt.Errorf("dlt: bad storage magic %x", storage[0:4])
	}
	// Storage header is little-endian per most DLT tooling.
	secs := binary.LittleEndian.Uint32(storage[4:8])
	usecs := binary.LittleEndian.Uint32(storage[8:12])
	ts := time.Unix(int64(secs), int64(usecs)*int64(time.Microsecond)).UTC()
	ecuID := trimZero(storage[12:16])

	msg, err := readStandardAndPayload(r)
	if err != nil {
		return nil, err
	}
	msg.Timestamp = ts
	if msg.ECUID == "" {
		msg.ECUID = ecuID
	}
	return msg, nil
}

// DLTFrameToMessage converts a decoded DLT frame into an EEBus model.Message
// if the frame carries EEBus content. Returns:
//   - (msg, false, nil) — good message extracted, no truncation
//   - (nil, true, nil)  — EEBus-shaped payload but truncated by DLT; caller
//     should bump its "skipped truncated" counter
//   - (nil, false, nil) — frame carries no EEBus content, skip silently
//
// This is the single point of truth used by both file importers and the live
// DLT stream source so extractor rules stay consistent.
func DLTFrameToMessage(p *Parser, frame *DLTBinaryMessage, seqNum int) (msg *model.Message, truncated bool, _ error) {
	if !frame.Verbose || frame.Args == "" {
		return nil, false, nil
	}
	extract := ExtractEEBusFromDLTPayload(frame.APID, frame.CTID, frame.Args)
	if extract == nil {
		return nil, false, nil
	}
	if extract.Truncated {
		return nil, true, nil
	}
	if !IsCompleteJSON([]byte(extract.JSON)) {
		return nil, true, nil
	}
	normalized := NormalizeEEBUSJSON([]byte(extract.JSON))
	return BuildMessageFromJSON(p, normalized, seqNum, frame.Timestamp, extract.Direction, ""), false, nil
}

// BuildMessageFromJSON constructs a model.Message from a normalized JSON
// payload, accepting either a bare SPINE datagram (`{"datagram":…}`) or a
// full SHIP frame (`{"data":[…]}`). Extracted here so it can be shared by
// log-file importers and live sources without pulling in the store package.
func BuildMessageFromJSON(p *Parser, normalized []byte, seqNum int, ts time.Time, dir model.Direction, peerDevice string) *model.Message {
	msg := &model.Message{
		SequenceNum:    seqNum,
		Timestamp:      ts,
		Direction:      dir,
		NormalizedJSON: json.RawMessage(normalized),
		ShipMsgType:    model.ShipMsgTypeData,
	}

	if spineMsg := p.ParseSpineFromJSON(normalized); spineMsg != nil {
		FillFromSpine(msg, spineMsg)
	} else if shipResult, err := p.ParseShipFromJSON(normalized); err == nil {
		msg.ShipMsgType = shipResult.ShipMsgType
		msg.ShipPayload = shipResult.ShipPayload
		if shipResult.Spine != nil {
			FillFromSpine(msg, shipResult.Spine)
		} else if shipResult.ShipMsgType == model.ShipMsgTypeData {
			msg.ParseError = "could not parse SPINE datagram from log line"
		}
	} else {
		msg.ParseError = "could not parse SPINE datagram from log line"
	}

	if dir == model.DirectionOutgoing && msg.DeviceDest == "" {
		msg.DeviceDest = peerDevice
	} else if dir == model.DirectionIncoming && msg.DeviceSource == "" {
		msg.DeviceSource = peerDevice
	}
	return msg
}

// FillFromSpine copies SPINE-parsed fields into a Message.
func FillFromSpine(msg *model.Message, s *SpineResult) {
	msg.SpinePayload = s.SpinePayload
	msg.CmdClassifier = s.CmdClassifier
	msg.FunctionSet = s.FunctionSet
	msg.MsgCounter = s.MsgCounter
	msg.MsgCounterRef = s.MsgCounterRef
	msg.DeviceSource = s.DeviceSource
	msg.DeviceDest = s.DeviceDest
	msg.EntitySource = s.EntitySource
	msg.EntityDest = s.EntityDest
	msg.FeatureSource = s.FeatureSource
	msg.FeatureDest = s.FeatureDest
}

// ReadDLTMessageStream decodes a single DLT frame from a live TCP stream.
// Unlike the `.dlt` file format, wire frames have no storage header — the
// message starts directly with the standard header. Because the wire also
// carries no absolute wall-clock timestamp, callers pass a fallback (typically
// time.Now()) that becomes the message's Timestamp.
//
// Returns io.EOF cleanly at end-of-stream. Returns a non-EOF error on
// truncated / malformed frames.
func ReadDLTMessageStream(r io.Reader, fallbackTS time.Time) (*DLTBinaryMessage, error) {
	msg, err := readStandardAndPayload(r)
	if err != nil {
		return nil, err
	}
	msg.Timestamp = fallbackTS
	return msg, nil
}

// readStandardAndPayload decodes standard header + optional extended header +
// payload starting at the first byte after any storage header.
func readStandardAndPayload(r io.Reader) (*DLTBinaryMessage, error) {
	var std [4]byte
	if _, err := io.ReadFull(r, std[:]); err != nil {
		// A clean EOF between messages is fine; a partial read is not.
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, err
		}
		return nil, err
	}
	htyp := std[0]
	msgLen := binary.BigEndian.Uint16(std[2:4]) // standard-header length is BE

	if msgLen < 4 {
		return nil, fmt.Errorf("dlt: bogus message length %d", msgLen)
	}

	// Read the rest of the message (msgLen includes the 4 std bytes already read).
	rest := make([]byte, int(msgLen)-4)
	if _, err := io.ReadFull(r, rest); err != nil {
		return nil, err
	}

	msg := &DLTBinaryMessage{}
	pos := 0

	if htyp&htypWEID != 0 {
		if pos+4 > len(rest) {
			return nil, io.ErrUnexpectedEOF
		}
		msg.ECUID = trimZero(rest[pos : pos+4])
		pos += 4
	}
	if htyp&htypWSID != 0 {
		if pos+4 > len(rest) {
			return nil, io.ErrUnexpectedEOF
		}
		pos += 4 // session id, unused
	}
	if htyp&htypWTMS != 0 {
		if pos+4 > len(rest) {
			return nil, io.ErrUnexpectedEOF
		}
		pos += 4 // 0.1ms timestamp, unused (we use storage-header time)
	}

	if htyp&htypUEH != 0 {
		if pos+10 > len(rest) {
			return nil, io.ErrUnexpectedEOF
		}
		msin := rest[pos]
		// rest[pos+1] is noar (arg count) — inferred from payload walk
		msg.APID = trimZero(rest[pos+2 : pos+6])
		msg.CTID = trimZero(rest[pos+6 : pos+10])
		msg.Verbose = msin&msinVerbose != 0
		pos += 10
	}

	// Payload endianness: MSBF flag toggles it. String length is 2 bytes
	// in payload endianness.
	payload := rest[pos:]
	payloadBE := htyp&htypMSBF != 0

	if !msg.Verbose {
		// Non-verbose messages need a Fibex file to decode. Skip.
		return msg, nil
	}

	msg.Args = decodeVerboseStringArgs(payload, payloadBE)
	return msg, nil
}

// decodeVerboseStringArgs walks the argument list and concatenates all string
// arguments. Non-string args are skipped by inspecting the TYLE length field.
// This intentionally accepts partial parses: DLT string args in the wild
// sometimes have off-by-one lengths, and we'd rather recover something than
// abort the whole message.
func decodeVerboseStringArgs(p []byte, be bool) string {
	order := binary.LittleEndian.Uint32
	order16 := binary.LittleEndian.Uint16
	if be {
		order = binary.BigEndian.Uint32
		order16 = binary.BigEndian.Uint16
	}

	var out []byte
	pos := 0
	for pos+4 <= len(p) {
		typeInfo := order(p[pos : pos+4])
		pos += 4

		isString := typeInfo&typeInfoStringMask != 0
		tyle := int(typeInfo & 0xF) // length field size class (usually irrelevant for strings)
		_ = tyle

		if isString {
			if pos+2 > len(p) {
				break
			}
			sl := int(order16(p[pos : pos+2]))
			pos += 2
			if sl < 0 || pos+sl > len(p) {
				break
			}
			s := p[pos : pos+sl]
			// Strings are nul-terminated.
			if n := indexNul(s); n >= 0 {
				s = s[:n]
			}
			if len(out) > 0 {
				out = append(out, ' ')
			}
			out = append(out, s...)
			pos += sl
			continue
		}

		// Non-string arg: skip by TYLE size. We don't know the exact size
		// without a full type table, so we bail — most DLT frames of interest
		// (EEBus) contain a single string arg anyway.
		break
	}
	return string(out)
}

func trimZero(b []byte) string {
	n := indexNul(b)
	if n < 0 {
		return string(b)
	}
	return string(b[:n])
}

func indexNul(b []byte) int {
	for i, c := range b {
		if c == 0 {
			return i
		}
	}
	return -1
}
