# Architecture

## Overview

EEBusTracer is a Go application that captures EEBus protocol messages,
decodes them using the enbility `spine-go` and `ship-go` libraries, persists
them in SQLite, and serves a web-based UI for analysis.

## System Architecture

```
┌───────────────────────────────────────────────────────────────┐
│                         EEBusTracer                           │
│                                                               │
│  ┌──────────────────────────┐                                 │
│  │   Capture Engine         │    ┌─────────────┐              │
│  │                          │───▶│   Parser    │              │
│  │  Source interface:       │    │(SHIP/SPINE) │              │
│  │  ├─ UDPSource            │    └─────────────┘              │
│  │  ├─ LogTailSource        │                                 │
│  │  └─ (future sources)     │    ┌─────────────┐              │
│  └──────────┬───────────────┘───▶│    Store    │              │
│             │                    │  (SQLite)   │              │
│             │ OnMessage          └──────┬──────┘              │
│             ▼                          │                      │
│  ┌─────────────┐    ┌──────────┐ ┌─────┴──────┐              │
│  │  WebSocket   │◀───│  mDNS   │ │  HTTP API  │              │
│  │  Hub (typed) │    │ Monitor │ │  (Go 1.22) │              │
│  └──────┬──────┘    └──────────┘ └─────┬──────┘              │
│         │                              │                      │
└─────────┼──────────────────────────────┼──────────────────────┘
          │                              │
          └──────────┬───────────────────┘
                     │
              ┌──────┴──────┐
              │   Web UI    │
              │  (Browser)  │
              └─────────────┘
```

## Package Structure

```
cmd/eebustracer/       Main entrypoint, CLI commands (cobra)
                       - root.go: DB/logger initialization, --db/--verbose flags
                       - serve.go: HTTP server with embedded web UI
                       - capture.go: headless UDP capture with .eet export
                       - import_cmd.go: .eet file import
                       - version.go: build version

internal/model/        Domain types (independent of persistence/protocol)
                       - Trace, Message, Device structs
                       - Direction, ShipMsgType enums
                       - ChartDefinition, ChartSource (custom chart configs)

internal/capture/      Capture engine with pluggable sources
                       - source.go: Source interface (Name, Run)
                       - source_udp.go: UDPSource (connects to EEBus stack)
                       - source_logtail.go: LogTailSource (tails log files)
                       - capture.go: Engine with StartWithSource, context-based stop
                       - Batch insert: 100 messages or 500ms flush
                       - OnMessage callbacks for WebSocket fan-out
                       - CaptureStats with atomic counters

internal/parser/       Protocol decoding
                       - normalize.go: EEBUS JSON → standard JSON
                         (wraps ship.JsonFromEEBUSJson + fixupSliceFields)
                       - ship.go: SHIP message classification by top-level key
                       - spine.go: SPINE datagram field extraction
                         (with fixupSpineDatagram for array restoration)
                       - parser.go: Parser.Parse() orchestrates the pipeline
                       - logformat.go: shared log line parsing helpers
                         (LogLineRegex, ParseLogTimestamp, ExtractPeerDevice)

internal/store/        Data persistence
                       - db.go: SQLite connection with WAL + foreign keys
                       - migrations.go: schema DDL v1 (traces, messages, devices)
                         + v2 (FTS5, bookmarks, filter_presets)
                         + v3 (mdns_devices)
                         + v4 (chart_definitions with built-in seeds)
                       - chart_repo.go: CRUD for chart definitions
                       - mdns_device_repo.go: upsert/list mDNS devices
                       - trace_repo.go: CRUD for traces
                       - message_repo.go: insert/batch/paginate/filter messages,
                         FTS search, FindByMsgCounter/FindByMsgCounterRef
                       - device_repo.go: upsert/list devices
                       - preset_repo.go: CRUD for filter presets
                       - bookmark_repo.go: CRUD for bookmarks
                       - fileio.go: .eet JSON export/import

internal/api/          Web interface backend
                       - server.go: route registration (Go 1.22 mux)
                       - traces.go, messages.go: REST endpoints
                       - capture_handler.go: start/stop/status (UDP + log tail)
                       - mdns_handler.go: mDNS discovery start/stop/devices
                       - fileio_handler.go: import/export
                       - presets.go: filter preset endpoints
                       - bookmarks.go: bookmark endpoints
                       - devices.go: device discovery with entity/feature trees
                       - connections.go: SHIP connection state timeline
                       - timeseries.go: generic timeseries extraction via
                         ExtractionDescriptor (measurement, load control, setpoint)
                       - discovery.go: auto-discovery of chartable SPINE data
                         (introspects payloads for ScaledNumber values)
                       - charts.go: chart definition CRUD + data rendering endpoint
                       - correlation.go: message correlation by msgCounter
                       - usecases.go: use case API handler
                       - subscriptions.go: subscription/binding API handlers
                       - metrics.go: heartbeat accuracy metrics API handler + CSV export
                       - hub.go: WebSocket fan-out hub with typed events
                       - websocket.go: WS upgrade handler
                       - templates.go: HTML template rendering
                       - response.go: JSON helpers

internal/analysis/     Protocol intelligence and analysis
                       - usecases.go: detect use cases from
                         nodeManagementUseCaseData (36+ abbreviation mappings)
                       - subscriptions.go: subscription & binding lifecycle
                         tracking with staleness detection
                       - metrics.go: heartbeat accuracy metrics (jitter
                         statistics per device pair, CSV export)

internal/mdns/         mDNS device discovery
                       - monitor.go: Browse _ship._tcp, parse TXT records,
                         track online/offline, callbacks for WS broadcast

web/                   Frontend assets (embedded via embed.FS)
                       - templates/: Go HTML templates (layout, index, trace,
                         charts, intelligence, discovery)
                       - static/css/: dark theme CSS with visualization styles
                       - static/js/common.js: shared utilities (CMD_COLORS, etc.)
                       - static/js/app.js: main UI logic (capture mode selector,
                         typed WS events, overview registry)
                         See docs/OVERVIEW_RENDERERS.md for adding new decoders
                       - static/js/discovery.js: mDNS discovery page logic
                       - static/js/charts.js: Chart.js measurement/load charts
                       - static/js/intelligence.js: protocol intelligence page
                         (use cases, subscriptions, heartbeat accuracy)
                       - static/js/vendor/: Chart.js v4,
                         chartjs-plugin-zoom, Hammer.js (vendored, no CDN)
```

## Data Flow

### Recording (via Source interface)
```
Source.Run(ctx, emit) → emit(msg)
  │
  ├── UDPSource: UDP packet → raw bytes msg
  │     → Engine.runSource → Parser.Parse(raw, traceID, seqNum, timestamp)
  │       → NormalizeEEBUSJSON (if EEBUS format detected)
  │       → classifyShipMessage (top-level key matching)
  │       → parseSpineDatagram (if SHIP type = "data")
  │
  └── LogTailSource: poll file → readNewLines → LogLineRegex match
        → NormalizeEEBUSJSON → ParseSpineFromJSON
        → pre-parsed model.Message

  → model.Message populated
  → OnMessage callbacks (→ Hub.BroadcastEvent("message") → WebSocket clients)
  → Batch buffer → InsertMessages (SQLite)
```

### File Import
```
.eet JSON file → store.ImportTrace (parse + validate version)
  → TraceRepo.CreateTrace
  → MessageRepo.InsertMessages
```

### Analysis (UI)
```
Browser → GET /api/traces/{id}/messages?search=Measurement&cmdClassifier=read&device=devA&limit=50
  → MessageRepo.ListMessages (FTS JOIN + parameterized SQL query)
  → JSON response → rendered in browser table
  → Click message → GET /api/traces/{id}/messages/{mid}
  → Detail panel: decoded JSON, raw hex, headers, related messages
  → GET /api/traces/{id}/messages/{mid}/related
  → Correlation by msgCounter/msgCounterRef
```

### Device Discovery
```
GET /api/traces/{id}/devices
  → DeviceRepo.ListDevices
  → For each device: find NodeManagementDetailedDiscoveryData reply
  → Parse entity/feature tree from SPINE payload
  → Enriched response with entities[] and features[]
```

### Connection State
```
GET /api/traces/{id}/connections
  → Fetch all messages ordered by sequence
  → Walk SHIP message types to build state machine per device pair
  → Detect anomalies (missing states, unexpected close)
  → Response: connection pairs with state timeline
```

### Time Series Extraction
```
GET /api/traces/{id}/timeseries?type=measurement|loadcontrol|setpoint
  → Resolve type to ExtractionDescriptor (cmdKey, dataArrayKey, idField, classifiers)
  → Query messages by functionSet + cmdClassifier
  → extractGenericData: parse SPINE payload → data items with ScaledNumber conversion
  → extractGenericSeries: group by ID, enrich labels from descriptions
  → Response: { type, series: [{ id, label, dataPoints: [{timestamp, value}] }] }

GET /api/traces/{id}/charts/{cid}/data
  → Load ChartDefinition from chart_definitions table
  → Parse sources JSON → []ChartSource (each with extraction parameters)
  → For each source: build ExtractionDescriptor, query messages, extract series
  → Merge all series, return TimeseriesResponse

GET /api/traces/{id}/timeseries/discover
  → ListDistinctFunctionSets (data messages with non-empty function_set)
  → For each: sample messages, introspect SPINE payload for ScaledNumber values
  → Return discovered sources with cmdKey, dataArrayKey, idField, sampleIds
```

### Protocol Intelligence
```
GET /api/traces/{id}/usecases
  → Query messages with functionSet=NodeManagementUseCaseData
  → Parse SPINE payload → extract useCaseInformation[]
  → Map use case names to abbreviations (LPC, MPC, etc.)
  → Response: [{ deviceAddr, actor, useCases: [{ name, abbreviation, available }] }]

GET /api/traces/{id}/subscriptions
  → Walk subscription management messages in sequence
  → Track add/remove lifecycle, count notify messages per subscription
  → Detect stale subscriptions (no notify within threshold)
  → Response: [{ clientDevice, serverDevice, active, stale, notifyCount }]

GET /api/traces/{id}/metrics
  → Compute heartbeat interval jitter statistics per device pair
  → Response: { heartbeatJitter: [{ devicePair, meanIntervalMs, stdDevMs, minIntervalMs, maxIntervalMs, sampleCount }] }

eebustracer analyze <file> --check all
  → Parse .eet file → run analysis functions from internal/analysis/
  → Text output: formatted tables with severity badges
  → JSON output: raw result structs
```

### mDNS Discovery
```
POST /api/mdns/start
  → Monitor.Start(ctx) → zeroconf.Browse("_ship._tcp", "local.")
  → handleEntry: parse TXT records (ski, brand, model, type, id)
  → Upsert DiscoveredDevice in memory map
  → OnDevice callbacks → Hub.BroadcastEvent("mdns_device") → WebSocket clients
  → sweepLoop: every 15s, mark devices offline if not seen for 60s
```

## Key Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Reuse eebus-go types | Import `spine-go` + `ship-go` | Avoid re-implementing ~110 SPINE types; stay in sync with spec |
| Selective normalization | Only apply `JsonFromEEBUSJson` for EEBUS-format JSON | Prevents destruction of legitimate arrays in standard JSON |
| SPINE array fixup | `fixupSpineDatagram` restores cmd/entity arrays | `JsonFromEEBUSJson` flattens all `[{...}]` patterns; known array fields need restoration |
| Pure Go SQLite | `modernc.org/sqlite` (no CGO) | Cross-compile without C toolchain; driver name `"sqlite"` |
| WAL mode | Enabled on DB open | Concurrent reads during capture writes |
| Batch inserts | Buffer 100 msgs or 500ms flush | Reduces SQLite write amplification during high-throughput capture |
| Embedded web assets | `embed.FS` in `web/` package | Single binary distribution |
| Go 1.22 routing | `mux.HandleFunc("GET /api/...")` | Built-in method+pattern routing, no gorilla/mux needed |
| WebSocket fan-out | Buffered channel per client, drop on full | Prevents slow clients from blocking capture |
| .eet format | Simple JSON with version field | Human-readable, easy to inspect/modify |
| FTS5 search | SQLite FTS5 virtual table with triggers | Fast full-text search across message payloads; triggers keep index in sync |
| Schema migration | Version-based incremental migration | v1→v2 with automatic FTS backfill; idempotent re-runs |
| Connection state | Computed on-the-fly, not stored | No schema changes needed; always consistent with message data |
| SVG visualizations | D3.js v7 (vendored) | Industry standard for data-driven SVG; no build step needed |
| Chart rendering | Chart.js v4 (vendored) | Lightweight, interactive charts; time axis support built-in |
| Vendored JS libs | No CDN, minified files in repo | Offline-capable, no external dependencies at runtime |
| Viz page layout | Separate pages per visualization | Avoids overloading trace page; each loads only needed JS |
| Timeseries extraction | Generic ExtractionDescriptor | Parameterized by cmdKey/dataArrayKey/idField; supports any SPINE function set with ScaledNumber values |
| Chart definitions | SQLite table with JSON sources | Persist user-created charts; built-in presets seeded on migration; flexible source configuration |
| Source interface | Pluggable capture sources | Decouples engine from UDP; enables log tail, future WebSocket proxy, PCAP |
| Log tail polling | 100ms time.Ticker, no fsnotify | Avoids new dependency; portable across OS; simple and reliable |
| mDNS library | `github.com/grandcat/zeroconf` | Pure Go, no CGO, well-maintained; supports browsing and TXT record parsing |
| Typed WS events | `{type, payload}` envelope | Allows UI to distinguish message events from mDNS events without separate WS connections |
| Separate capture endpoints | `/api/capture/start` + `/api/capture/start/logtail` | Clearer validation per mode; backward compatible |
| Analysis package | Separate `internal/analysis/` | Reusable by API handlers and CLI; keeps business logic out of HTTP layer |
| Use case detection | Computed on-the-fly | Consistent with connections/topology; small data volume; no schema changes |
| Subscription tracking | Walk messages in sequence | Same pattern as `buildConnectionStates()`; messages are source of truth |
| Performance metrics | Per-request computation | Fast enough for typical traces; time range filter narrows window for large ones |
| Conformance checks | Five independent check functions | Each returns `[]Violation`; easy to add new checks; clear severity levels |

## External Dependencies

```
github.com/enbility/spine-go    SPINE protocol types and model
github.com/enbility/ship-go     SHIP protocol types, JSON helpers
modernc.org/sqlite               Pure Go SQLite driver (no CGO)
github.com/spf13/cobra           CLI framework
github.com/gorilla/websocket     WebSocket connections
github.com/grandcat/zeroconf     mDNS/DNS-SD browsing (pure Go)
```
