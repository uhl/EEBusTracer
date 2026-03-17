# EEBus Tracer Integration Guide

This guide explains how to connect your EEBus device or application to
EEBus Tracer for live tracing and offline analysis.

## Overview

EEBus Tracer supports three capture methods:

| Method   | Use Case                                    | Direction    |
|----------|---------------------------------------------|--------------|
| **UDP**  | Direct connection to an EEBus stack debug port | Tracer connects to your device |
| **TCP**  | Connect to a TCP log server | Tracer connects to your server |
| **Log Tail** | Watch an eebus-go / spine-go log file in real time | Tracer reads your log file |

You can also **import log files** (drag-and-drop or CLI) for offline analysis
without a live connection.

---

## Method 1: UDP Capture

### How It Works

Your EEBus stack exposes a UDP debug port. EEBus Tracer connects to it, sends
a single registration byte (`0x00`), and then receives raw SHIP frames as UDP
packets.

### Packet Format

Each UDP packet is a binary SHIP frame:

```
[header_byte] [json_payload]
```

| Header Byte | Meaning | Payload |
|-------------|---------|---------|
| `0x00`      | SHIP Init | None |
| `0x01`      | CMI (SHIP message) | EEBUS JSON |

#### Example: Init Message

```
0x00
```

Single byte, no payload.

#### Example: SHIP Data Message

```
0x01{"datagram":[{"header":[{"specificationVersion":"1.3.0"},{"addressSource":[{"device":"d:_i:MyDevice"},{"entity":[1]},{"feature":0}]},{"addressDestination":[{"device":"d:_i:OtherDevice"},{"entity":[1]},{"feature":0}]},{"msgCounter":42},{"cmdClassifier":"read"}]},{"payload":[{"cmd":[[{"nodeManagementDetailedDiscoveryData":[]}]]}]}]}
```

Header byte `0x01` followed immediately by the JSON body (no separator).

### JSON Format

The payload uses **EEBUS JSON encoding** — arrays of single-key objects instead
of regular JSON objects. EEBus Tracer normalizes this automatically.

EEBUS JSON:
```json
[{"key1": "value1"}, {"key2": "value2"}]
```

Standard JSON equivalent:
```json
{"key1": "value1", "key2": "value2"}
```

Both formats are accepted. If your stack emits standard JSON, that works too.

### What You Need to Implement

Expose a UDP listener in your EEBus stack that:

1. Accepts incoming UDP connections on a configurable port (default: `4712`)
2. Reads a single `0x00` byte as registration from the tracer
3. Sends every outgoing and incoming SHIP frame as a UDP packet to the
   registered client, using the binary format above

### Starting a UDP Capture

**Web UI:** Select "UDP", enter host and port, click "Start Capture"

**CLI:**
```bash
eebustracer capture --target 192.168.1.100:4712
```

---

## Method 2: TCP Log Server

### How It Works

Your device runs a TCP log server that streams log lines to connected clients.
EEBus Tracer connects and parses the log output in real time.

### Log Line Format

Each message is a text line with this structure:

```
[HH:MM:SS.mmm] DIRECTION to|from PEER MSG: JSON_PAYLOAD
```

| Field | Format | Examples |
|-------|--------|----------|
| Timestamp | `[HH:MM:SS.mmm]` | `[11:38:26.008]` |
| Direction | `SEND` or `RECV` | `SEND` |
| Preposition | `to` (for SEND) or `from` (for RECV) | `to` |
| Peer | Device identifier string | `ship_MyDevice_0xaff223b8` |
| Separator | `MSG:` followed by a space | `MSG: ` |
| Payload | EEBUS JSON or standard JSON | `{"datagram":[...]}` |

#### Full Example

```
[11:38:26.008] SEND to ship_Volvo-CEM-400000270_0xaff223b8 MSG: {"datagram":[{"header":[{"specificationVersion":"1.3.0"},{"addressSource":[{"device":"d:_i:HEMS"},{"entity":[1]},{"feature":0}]},{"addressDestination":[{"device":"d:_i:Volvo-CEM-400000270"},{"entity":[1]},{"feature":0}]},{"msgCounter":21},{"cmdClassifier":"read"}]},{"payload":[{"cmd":[[{"nodeManagementDetailedDiscoveryData":[]}]]}]}]}
[11:38:26.016] RECV from ship_Volvo-CEM-400000270_0xaff223b8 MSG: {"datagram":[{"header":[{"specificationVersion":"1.3.0"},{"addressSource":[{"device":"d:_i:Volvo-CEM-400000270"},{"entity":[1]},{"feature":0}]},{"addressDestination":[{"device":"d:_i:HEMS"},{"entity":[1]},{"feature":0}]},{"msgCounter":5},{"msgCounterReference":21},{"cmdClassifier":"reply"}]},{"payload":[{"cmd":[[{"nodeManagementDetailedDiscoveryData":[...]}]]}]}]}
```

### Regex Pattern

The exact regex used to parse each line:

```
[(\d{2}:\d{2}:\d{2}\.\d{3})]\s+(SEND|RECV)\s+(?:to|from)\s+(\S+)\s+MSG:\s+(.+)
```

Capture groups:
1. Timestamp: `HH:MM:SS.mmm`
2. Direction: `SEND` or `RECV`
3. Peer identifier
4. JSON payload (everything after `MSG: `)

### Peer Identifier Format

The peer string identifies the remote device. If it follows the SHIP naming
convention, EEBus Tracer extracts a clean device name:

```
ship_<DeviceName>_<HexAddress>
```

| Peer String | Extracted Device Name |
|-------------|----------------------|
| `ship_Volvo-CEM-400000270_0xaff223b8` | `Volvo-CEM-400000270` |
| `ship_MyWallbox_0x12345678` | `MyWallbox` |
| `plain-peer-name` | `plain-peer-name` |

Any string works as a peer identifier. The `ship_..._0x...` format is optional
but provides cleaner device names in the UI.

### Handling Binary Padding

If your TCP server uses fixed-size C buffers, EEBus Tracer
handles:

- Multiple messages concatenated in a single buffer
- Null bytes (`0x00`) and binary garbage between messages
- Partial messages split across TCP reads

No special formatting is needed on your side — just send the log lines.

### What You Need to Implement

A TCP server that:

1. Listens on a configurable port
2. Accepts incoming connections
3. Streams log lines in the format above for every SHIP message sent/received
4. Each line contains timestamp, direction, peer, and the full JSON payload

### Starting a TCP Capture

**Web UI:** Select "TCP", enter host and port, click "Start Capture"

**CLI:**
```bash
eebustracer capture --tcp 192.168.20.41:54546
```

---

## Method 3: Log File Tail

### How It Works

Your application writes SHIP messages to a log file. EEBus Tracer watches the
file and processes new lines as they are appended — similar to `tail -f`.

### Supported Log Formats

#### eebus-go Format

Used by the enbility `eebus-go` / `spine-go` stack:

```
SEQ [HH:MM:SS.mmm] DIRECTION to|from PEER MSG: JSON_PAYLOAD
```

The sequence number prefix is optional:

```
15 [11:38:26.008] SEND to ship_Device_0xabc MSG: {"datagram":[...]}
[11:38:26.016] RECV from ship_Device_0xabc MSG: {"datagram":[...]}
```

Regex:
```
^(?:(\d+)\s+)?\[(\d{2}:\d{2}:\d{2}\.\d{3})\]\s+(SEND|RECV)\s+(?:to|from)\s+(\S+)\s+MSG:\s+(.+)$
```

#### eebustester Format

Used by the EEBus Living Lab eebustester tool:

```
[YYYYMMDD HH:MM:SS.mmm] - INFO - DATAGRAM - SENDER - Send|Received message to|from 'PEER': JSON_PAYLOAD
```

Example:
```
[20260206 11:54:15.338] - INFO - DATAGRAM - Tester_EG - Send message to 'ship_Device_0xabc': {"datagram":[...]}
[20260206 11:54:15.450] - INFO - DATAGRAM - Tester_EG - Received message from 'ship_Device_0xabc': {"datagram":[...]}
```

Regex:
```
^\[(\d{8})\s+(\d{2}:\d{2}:\d{2}\.\d{3})\]\s+-\s+INFO\s+-\s+DATAGRAM\s+-\s+\S+\s+-\s+(Send|Received)\s+message\s+(?:to|from)\s+'([^']+)':\s+(.+)$
```

### Format Auto-Detection

EEBus Tracer reads the first 4 KB of the file and detects the format
automatically. Detection order:

1. **eebustester** — line starts with `[YYYYMMDD HH:MM:SS`
2. **eebus-go** — line starts with optional number + `[HH:MM:SS`

### What You Need to Implement

Write every SHIP message to a log file using one of the formats above. The
simplest format to implement is eebus-go without sequence numbers:

```
[HH:MM:SS.mmm] SEND to PEER MSG: JSON_PAYLOAD
[HH:MM:SS.mmm] RECV from PEER MSG: JSON_PAYLOAD
```

### Starting a Log Tail Capture

**Web UI:** Select "Log Tail", enter the file path, click "Start Capture"

**CLI:**
```bash
eebustracer capture --log-file /var/log/eebus.log
```

---

## Method 4: File Import (Offline Analysis)

You can import existing log files without a live connection.

### Supported File Types

| Extension | Format |
|-----------|--------|
| `.eet` | EEBus Tracer native format (export/import) |
| `.log` | eebus-go or eebustester log file (auto-detected) |

### Import Methods

**Web UI:** Drag and drop a `.eet` or `.log` file onto the page, or click
"Load from file" on the welcome page.

**CLI:**
```bash
eebustracer import trace.log
eebustracer import capture.eet
```

---

## SPINE Message Structure

For EEBus Tracer to extract meaningful data (classifier, function set, devices,
message counters), your JSON payloads should contain standard SPINE datagrams:

```json
{
  "datagram": {
    "header": {
      "specificationVersion": "1.3.0",
      "addressSource": {
        "device": "d:_i:SourceDevice",
        "entity": [1],
        "feature": 0
      },
      "addressDestination": {
        "device": "d:_i:DestDevice",
        "entity": [1],
        "feature": 0
      },
      "msgCounter": 42,
      "cmdClassifier": "read"
    },
    "payload": {
      "cmd": [
        {"nodeManagementDetailedDiscoveryData": {}}
      ]
    }
  }
}
```

The tool also recognizes SHIP-level messages (outside of datagrams):

| SHIP Message | Top-Level JSON Key |
|-------------|-------------------|
| Connection Hello | `connectionHello` |
| Protocol Handshake | `messageProtocolHandshake` |
| Pin State | `connectionPinState` |
| Access Methods | `accessMethods` or `accessMethodsRequest` |
| Connection Close | `connectionClose` |
| Data (SPINE) | `data` → contains the `datagram` |

---

## Quick Start: Minimal Integration

The fastest way to get started is **log file output**. Add this to your
application's SHIP message handler:

```
// On every SHIP message sent or received, append a line:
[HH:MM:SS.mmm] SEND to PEER_NAME MSG: {json_payload}
[HH:MM:SS.mmm] RECV from PEER_NAME MSG: {json_payload}
```

Where:
- `HH:MM:SS.mmm` is the current time with milliseconds
- `SEND` / `RECV` indicates direction
- `PEER_NAME` is any identifier for the remote device
- `{json_payload}` is the raw SHIP/SPINE JSON as sent/received on the wire

Then either:
- Point EEBus Tracer at the file with Log Tail for live tracing
- Import the file later for offline analysis

### Example Implementation (Go)

```go
func logMessage(direction, peer string, payload []byte) {
    ts := time.Now().Format("15:04:05.000")
    dir := "SEND"
    prep := "to"
    if direction == "incoming" {
        dir = "RECV"
        prep = "from"
    }
    fmt.Fprintf(logFile, "[%s] %s %s %s MSG: %s\n", ts, dir, prep, peer, payload)
}
```

### Example Implementation (C/C++)

```c
void log_message(const char* direction, const char* peer, const char* json) {
    struct timespec ts;
    clock_gettime(CLOCK_REALTIME, &ts);
    struct tm* tm = localtime(&ts.tv_sec);
    fprintf(log_file, "[%02d:%02d:%02d.%03ld] %s %s %s MSG: %s\n",
        tm->tm_hour, tm->tm_min, tm->tm_sec, ts.tv_nsec / 1000000,
        strcmp(direction, "incoming") == 0 ? "RECV" : "SEND",
        strcmp(direction, "incoming") == 0 ? "from" : "to",
        peer, json);
    fflush(log_file);
}
```

### Example Implementation (Python)

```python
from datetime import datetime

def log_message(direction: str, peer: str, payload: str):
    ts = datetime.now().strftime("%H:%M:%S.%f")[:-3]
    if direction == "incoming":
        line = f"[{ts}] RECV from {peer} MSG: {payload}"
    else:
        line = f"[{ts}] SEND to {peer} MSG: {payload}"
    log_file.write(line + "\n")
    log_file.flush()
```
