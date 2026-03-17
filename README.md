# EEBusTracer

A cross-platform trace recording and analysis tool for the
[EEBus](https://www.eebus.org/) protocol stack (SHIP + SPINE).

## Status

**Phase 5 implemented** — Protocol intelligence (use case detection, subscription
tracking, heartbeat accuracy metrics) on top of Phase 4 advanced capture,
Phase 3 visualizations, and Phase 2 analysis features.

## What is EEBusTracer?

EEBusTracer captures, decodes, and visualizes EEBus protocol communication.
It helps developers and integrators debug, analyze, and validate EEBus device
interactions.

**Key features:**
- Connect to an EEBus stack and capture SHIP frames via UDP
- Live log tail: watch eebus-go log files in real time and capture new messages
- mDNS device discovery: browse `_ship._tcp` services to find EEBus devices
- Decode SHIP and SPINE protocol layers using the enbility eebus-go ecosystem
- Identify SHIP message types (connectionHello, data, accessMethods, etc.)
- Extract SPINE datagram fields: cmdClassifier, function set, addresses, msgCounter
- Full-text search across message content (FTS5)
- Filter by time range, device, entity, feature, function set, direction
- Save and recall named filter presets
- Device discovery view with entity/feature trees from discovery data
- Connection state timeline with anomaly detection
- Message correlation: match request/response pairs by msgCounter
- Bookmarks and annotations on messages with custom labels and colors
- Measurement, load control, and setpoint charts with CSV export
- Custom chart builder: create charts from any SPINE data with ScaledNumber values
- Auto-discovery of chartable data sources in a trace
- Use case detection from `nodeManagementUseCaseData` (LPC, MPC, MGCP, etc.)
- Subscription and binding lifecycle tracking with staleness detection
- Heartbeat accuracy metrics (jitter statistics per device pair)
- Intelligence dashboard with use case pills, subscription tracking, and heartbeat analysis
- Web-based UI for browsing, filtering, and inspecting messages (dark theme)
- Drag-and-drop `.eet`/`.log` file import
- Import log files from eebus-go, eebustester, and EEBus Hub with auto-detection
- Import/export traces in `.eet` JSON format
- SQLite persistence with WAL mode for concurrent access
- Cross-platform: macOS, Linux, Windows — single binary, no CGO

## Requirements

- Go 1.22 or later

## Building from Source

```bash
git clone https://github.com/<org>/eebustracer.git
cd eebustracer
go build ./cmd/eebustracer
```

Or use the Makefile:

```bash
make build        # Build binary
make test         # Run tests
make test-race    # Run tests with race detector
make lint         # Run linter
```

## Usage

### Web UI

```bash
# Start the web UI on port 8080
./eebustracer serve --port 8080
```

Then open http://localhost:8080 in your browser. Enter the EEBus stack's IP
address and port in the top bar, then click "Start Capture". The tracer connects
to the EEBus stack via UDP, and SHIP frames appear in real time via WebSocket.

### Headless Capture

```bash
# Connect to EEBus stack and capture, press Ctrl+C to stop
./eebustracer capture --target 192.168.1.100:4712

# Capture and export to file
./eebustracer capture --target 192.168.1.100:4712 -o trace.eet

# Tail an eebus-go log file
./eebustracer capture --log-file /var/log/eebus.log
```

### mDNS Discovery

```bash
# Discover EEBus devices on the network (10s scan)
./eebustracer discover

# Longer scan with JSON output
./eebustracer discover --timeout 30s --json
```

### Import/Export

```bash
# Import a .eet trace file
./eebustracer import trace.eet

# Export via the web UI or REST API
curl http://localhost:8080/api/traces/1/export > trace.eet
```

### Protocol Analysis

```bash
# Run all analysis checks on a trace file
./eebustracer analyze trace.eet --check all

# Run heartbeat metrics only, JSON output
./eebustracer analyze trace.eet --check metrics --output json

# Run specific checks
./eebustracer analyze trace.eet --check usecases,metrics
```

### Global Options

```bash
# Use a custom database location (default: ~/.eebustracer/traces.db)
./eebustracer --db /path/to/my/traces.db serve

# Enable verbose/debug output
./eebustracer -v serve
```

### Other Commands

```bash
./eebustracer version      # Print version
./eebustracer --help       # Show help
```

## REST API

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/traces` | List all traces |
| POST | `/api/traces` | Create a new trace |
| GET | `/api/traces/{id}` | Get trace by ID |
| DELETE | `/api/traces/{id}` | Delete trace |
| GET | `/api/traces/{id}/messages` | List messages (paginated, filterable, searchable) |
| GET | `/api/traces/{id}/messages/{mid}` | Get single message |
| GET | `/api/traces/{id}/messages/{mid}/related` | Get correlated messages |
| GET | `/api/traces/{id}/devices` | List devices with entity/feature tree |
| GET | `/api/traces/{id}/devices/{did}` | Get device detail |
| GET | `/api/traces/{id}/connections` | Connection state timeline |
| GET | `/api/traces/{id}/timeseries` | Measurement/load control time series |
| GET | `/api/traces/{id}/timeseries/discover` | Discover chartable data sources |
| GET | `/api/traces/{id}/charts` | List chart definitions |
| POST | `/api/traces/{id}/charts` | Create chart definition |
| GET | `/api/charts/{id}` | Get chart definition |
| PATCH | `/api/charts/{id}` | Update chart definition |
| DELETE | `/api/charts/{id}` | Delete chart definition |
| GET | `/api/traces/{id}/charts/{cid}/data` | Render chart data |
| GET | `/api/traces/{id}/usecases` | Detected use cases per device |
| GET | `/api/traces/{id}/subscriptions` | Subscription tracker |
| GET | `/api/traces/{id}/bindings` | Binding tracker |
| GET | `/api/traces/{id}/metrics` | Heartbeat accuracy metrics |
| GET | `/api/traces/{id}/metrics/export` | Export heartbeat metrics as CSV/JSON |
| GET | `/api/traces/{id}/bookmarks` | List bookmarks |
| POST | `/api/traces/{id}/bookmarks` | Create bookmark |
| DELETE | `/api/bookmarks/{id}` | Delete bookmark |
| GET | `/api/presets` | List filter presets |
| POST | `/api/presets` | Save filter preset |
| DELETE | `/api/presets/{id}` | Delete filter preset |
| GET | `/api/capture/status` | Capture engine status |
| POST | `/api/capture/start` | Start UDP capture |
| POST | `/api/capture/start/logtail` | Start log tail capture |
| POST | `/api/capture/stop` | Stop capture |
| GET | `/api/mdns/devices` | List mDNS-discovered devices |
| GET | `/api/mdns/status` | mDNS monitor status |
| POST | `/api/mdns/start` | Start mDNS discovery |
| POST | `/api/mdns/stop` | Stop mDNS discovery |
| POST | `/api/traces/import` | Import .eet file |
| GET | `/api/traces/{id}/export` | Export trace as .eet |
| GET | `/api/traces/{id}/live` | WebSocket live stream |

### Message Filter Parameters

The `/api/traces/{id}/messages` endpoint supports these query parameters:

| Parameter | Description |
|-----------|-------------|
| `search` | Full-text search across message content |
| `cmdClassifier` | Filter by command classifier (read, reply, write, call, notify) |
| `functionSet` | Filter by function set name |
| `shipMsgType` | Filter by SHIP message type |
| `direction` | Filter by direction (incoming, outgoing) |
| `device` | Filter by device address (matches source OR destination) |
| `deviceSource` | Filter by source device address |
| `deviceDest` | Filter by destination device address |
| `entitySource` | Filter by source entity address |
| `entityDest` | Filter by destination entity address |
| `featureSource` | Filter by source feature address |
| `featureDest` | Filter by destination feature address |
| `timeFrom` | Filter messages after this timestamp (RFC3339) |
| `timeTo` | Filter messages before this timestamp (RFC3339) |
| `limit` | Page size (default: 100) |
| `offset` | Pagination offset |

## Documentation

- [ROADMAP.md](ROADMAP.md) — Feature roadmap and milestones
- [CHANGELOG.md](CHANGELOG.md) — Version history
- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — System architecture
- [docs/INTEGRATION.md](docs/INTEGRATION.md) — Integration guide
- [docs/OVERVIEW_RENDERERS.md](docs/OVERVIEW_RENDERERS.md) — Adding Overview tab decoders

## License

MIT — see [LICENSE](LICENSE) for details.
