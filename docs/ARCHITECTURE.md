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
│  │  ├─ TCPSource            │                                 │
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
                       - capture.go: headless UDP/TCP/log tail capture
                       - import_cmd.go: .eet/.log file import
                       - analyze.go: protocol analysis (usecases, metrics, etc.)
                       - mdns.go: mDNS device discovery CLI
                       - version.go: build version

internal/model/        Domain types (independent of persistence/protocol)
                       - Trace, Message, Device structs
                       - Direction, ShipMsgType enums
                       - ChartDefinition, ChartSource (custom chart configs)

internal/capture/      Capture engine with pluggable sources
                       - source.go: Source interface (Name, Run)
                       - source_udp.go: UDPSource (connects to EEBus stack)
                       - source_logtail.go: LogTailSource (tails log files)
                       - source_tcp.go: TCPSource (connects to CNetLogServer)
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
                         + v5 (msg_counter/msg_counter_ref indexes)
                         + v6 (setpoint chart definition fix)
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
                       - correlation.go: message correlation by msgCounter/Ref,
                         latency computation, result status extraction,
                         conversation grouping by device pair + function set,
                         orphaned request detection
                       - usecases.go: use case API handler + use-case-context
                         filtering endpoint (maps use cases to function sets)
                       - subscriptions.go: subscription/binding API handlers
                       - metrics.go: heartbeat accuracy metrics API handler + CSV export
                       - depgraph.go: dependency graph API handler
                         (aggregates devices, use cases, subscriptions, bindings)
                       - writetracking.go: write tracking API handler
                         (extracts LoadControl/Setpoint writes, correlates results,
                         computes durations, builds effective state)
                       - lifecycle.go: use case lifecycle checklist API handler
                         (evaluates 5 setup steps per device+UC pair)
                       - hub.go: WebSocket fan-out hub with typed events
                       - websocket.go: WS upgrade handler
                       - templates.go: HTML template rendering
                       - response.go: JSON helpers

internal/analysis/     Protocol intelligence and analysis
                       - usecases.go: detect use cases from
                         nodeManagementUseCaseData (36+ abbreviation mappings),
                         UseCaseFunctionSets mapping for context filtering
                       - subscriptions.go: subscription & binding lifecycle
                         tracking with staleness detection
                       - metrics.go: heartbeat accuracy metrics (jitter
                         statistics per device pair, CSV export)
                       - depgraph.go: dependency tree builder
                         (use cases → devices → entities → features,
                         subscription/binding edges)
                       - lifecycle.go: lifecycle spec map + evaluator
                         (5-step checklist per device+UC pair)

internal/mdns/         mDNS device discovery
                       - monitor.go: Browse _ship._tcp, parse TXT records,
                         track online/offline, callbacks for WS broadcast

web/                   Frontend assets (embedded via embed.FS)
                       - templates/: Go HTML templates (layout, index, trace,
                         charts, intelligence, discovery, about)
                       - static/css/: "Oscillograph" theme CSS with dark/light
                         toggle (dark default, "Blueprint" light theme via
                         [data-theme="light"]), self-hosted Space Grotesk +
                         JetBrains Mono fonts, CRT noise overlay, amber glow
                         accents, animations
                       - static/fonts/: Space Grotesk (variable, WOFF2) and
                         JetBrains Mono (variable, WOFF2) self-hosted fonts
                       - static/js/common.js: shared utilities (CMD_COLORS, etc.)
                       - static/js/app.js: main UI logic (capture mode selector,
                         typed WS events, overview registry)
                         See docs/OVERVIEW_RENDERERS.md for adding new decoders
                       - static/js/discovery.js: mDNS discovery page logic
                       - static/js/charts.js: Chart.js measurement/load charts
                       - static/js/intelligence.js: protocol intelligence page
                         (use cases, subscriptions, heartbeat accuracy,
                         dependency tree, write tracking, lifecycle checklist)
                       - static/js/virtual-scroll.js: virtual scroll engine
                         for message table (renders only visible rows)
                       - static/js/vendor/: Chart.js v4,
                         chartjs-plugin-zoom, Hammer.js
                         (vendored, no CDN)
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
  → OnMessage callbacks (→ msg.ToSummary() → Hub.BroadcastEvent("message") → WebSocket clients)
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
Browser → GET /api/traces/{id}/messages/summaries?search=Measurement&cmdClassifier=read
  → MessageRepo.ListMessageSummaries (FTS JOIN, no LIMIT — returns all matching summaries)
  → JSON array → stored in client-side JS array, rendered via VirtualScroll engine
  → Only ~50-60 visible rows in DOM at any time; spacer <tr> elements maintain scroll position
  → Click message → GET /api/traces/{id}/messages/{mid} (on-demand, LRU-cached)
  → Detail panel: decoded JSON, raw hex, headers, related messages
  → GET /api/traces/{id}/messages/{mid}/related
  → Direct correlation: latency, relationship type, result status
  → GET /api/traces/{id}/messages/{mid}/conversation
  → Conversation grouping: all SPINE data msgs for same device pair + function set
  → Selecting a message highlights correlated rows (teal border) via /related data
  → GET /api/traces/{id}/orphaned-requests
  → IDs of data messages with msgCounter but no matching msgCounterRef
  → Frontend marks orphaned rows with red dot, detail panel shows warning banner
  → GET /api/traces/{id}/usecase-context
  → Detected use cases with associated devices and SPINE function sets
  → Frontend dropdown filters message table by use case context
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
  → Resolve type to ExtractionDescriptor (cmdKey, dataArrayKey, idField, classifiers, activeField)
  → Query messages by functionSet + cmdClassifier
  → extractGenericData: parse SPINE payload → data items with ScaledNumber conversion
    + optional boolean active-state extraction (isLimitActive / isSetpointActive)
  → extractGenericSeries: group by ID, enrich labels from descriptions, propagate isActive
  → Response: { type, series: [{ id, label, dataPoints: [{timestamp, value, isActive?}] }] }

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

GET /api/traces/{id}/depgraph
  → Detect use cases, list devices with discovery, track subscriptions/bindings
  → BuildDependencyTree: create per-device entity/feature trees with UC annotations,
    subscription/binding edges as flat list
  → Response: { devices: [...], edges: [...] } for tree view rendering

GET /api/traces/{id}/writetracking
  → Query LoadControlLimitListData + SetpointListData write messages
  → For each write: extract values per phase/scope, enrich labels from descriptions
  → Correlate result via msgCounterRef (accepted/rejected/pending)
  → Compute duration until next write to same ID (effective active period)
  → Build effective state: latest value per limit/setpoint ID
  → Response: { writes: [...], effectiveState: [...] }

GET /api/traces/{id}/lifecycle
  → Detect use cases per device (from nodeManagementUseCaseData)
  → For each device+UC pair, evaluate 5 setup steps:
    1. SHIP handshake completed (connectionHello exchange)
    2. Feature discovery (required features present per UC spec)
    3. Use case announced and available
    4. Required subscriptions established
    5. Required bindings established (if applicable)
  → Each step: pass/fail/partial/pending/na with details and missing items
  → Response: [{ deviceAddr, shortName, useCase, abbreviation, overallStatus, steps: [...] }]

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
| Chart rendering | Chart.js v4 (vendored) | Lightweight, interactive charts; time axis support built-in |
| Vendored JS libs | No CDN, minified files in repo | Offline-capable, no external dependencies at runtime |
| Viz page layout | Separate pages per visualization | Avoids overloading trace page; each loads only needed JS |
| Timeseries extraction | Generic ExtractionDescriptor | Parameterized by cmdKey/dataArrayKey/idField/activeField; supports any SPINE function set with ScaledNumber values; optional active-state boolean propagated to frontend for dashed-line rendering |
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
| Virtual scroll | Fat client: all summaries in JS array, render visible rows | Eliminates 2000-msg page limit; ~300 bytes/summary; 100k msgs ≈ 30 MB; smooth 60fps scroll |
| MessageSummary | Lightweight projection of Message (no payloads) | Reduces transfer size by ~10x; WS broadcasts use summaries; detail fetched on-demand with LRU cache |
| Dependency tree | DOM tree view (no library) | Lightweight per-device entity/feature trees rendered as HTML; reuses existing card/pill styles; no external dependency |
| Tree computation | On-the-fly from existing data | No new schema; aggregates devices, use cases, subscriptions into tree structure; consistent with source of truth |

## External Dependencies

```
github.com/enbility/spine-go    SPINE protocol types and model
github.com/enbility/ship-go     SHIP protocol types, JSON helpers
modernc.org/sqlite               Pure Go SQLite driver (no CGO)
github.com/spf13/cobra           CLI framework
github.com/gorilla/websocket     WebSocket connections
github.com/grandcat/zeroconf     mDNS/DNS-SD browsing (pure Go)
```
