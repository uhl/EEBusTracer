# EEBusTracer - Roadmap

An open-source, cross-platform trace recording and analysis tool for the EEBus
protocol stack (SHIP + SPINE).

---

## Project Vision

There is no open-source, dedicated EEBus trace analysis tool available today.
Wireshark lacks SHIP/SPINE dissectors, and existing solutions (EEBUS Tester,
EEBUS Hub) are proprietary and subscription-based. EEBusTracer fills this gap
by providing a free, developer-friendly tool for capturing, decoding, and
analyzing EEBus communication — on macOS, Linux, and Windows.

---

## Technology Decisions

| Concern           | Choice                              | Rationale                                                        |
| ----------------- | ----------------------------------- | ---------------------------------------------------------------- |
| Language          | Go                                  | Cross-platform, direct reuse of eebus-go ecosystem               |
| SHIP parsing      | `github.com/enbility/ship-go`       | Battle-tested SHIP types and EEBUS JSON normalization            |
| SPINE parsing     | `github.com/enbility/spine-go`      | All ~110 function set types, datagram model, CmdType already done |
| UI                | Web UI (Go backend + JS frontend)   | Rich data tables, charts; works everywhere                       |
| HTTP framework    | `net/http` + gorilla/websocket      | Stdlib for REST API, websocket for live message streaming        |
| Frontend          | Vanilla JS                          | No build step, no framework dependency                           |
| Persistence       | SQLite (via modernc.org/sqlite)     | Pure Go, no CGO, cross-platform, fast queries on large traces    |
| Trace file format | `.eet` (JSON-based)                 | Human-readable, versionable                                      |
| CLI framework     | cobra                               | Standard Go CLI tooling                                          |

### Relationship to eebus-go

EEBusTracer imports `spine-go` and `ship-go` as **Go module dependencies**.
This gives us:

- All SPINE model types (`model.Datagram`, `model.CmdType`, all function sets)
- SHIP message types (`model.ConnectionHello`, `model.ShipData`, etc.)
- EEBUS JSON normalization (`JsonFromEEBUSJson` / `JsonIntoEEBUSJson`)
- `fixupSliceFields()` for malformed device output
- Future model updates come in via `go get -u`

The project lives as a **standalone repository** that depends on the enbility
modules. No forking required.

---

## Completed (v0.0.1 – v0.4.0)

### Foundation & Core (v0.0.1)

- [x] Go module, folder layout, CI (GitLab CI/CD), linter, Makefile
- [x] Data model: Trace, Message, Device, Connection
- [x] SQLite persistence with WAL mode, schema migrations, repository layer
- [x] UDP capture engine with batch insert and WebSocket fan-out
- [x] SHIP/SPINE parsing pipeline (enbility types, EEBUS JSON normalization)
- [x] REST API (traces CRUD, messages, import/export, capture control)
- [x] WebSocket live streaming hub
- [x] Web UI: trace sidebar, message table, detail panel (JSON + headers),
      drag-and-drop import, status bar
- [x] CLI: `serve`, `capture`, `import`, `version`
- [x] `.eet` JSON trace file format
- [x] Pluggable capture Source interface (UDP, log tail, TCP)
- [x] Live log tail capture (eebus-go, eebustester, CEasierLogger formats)
- [x] TCP log server capture (CNetLogServer)
- [x] EEBus Hub log format import with auto-detection
- [x] Keyboard shortcuts (j/k, Ctrl+F, Ctrl+L, Ctrl+G)
- [x] Recent capture targets with autocomplete
- [x] Resizable detail panel, collapsible sidebar
- [x] Light/dark theme toggle

### Analysis & Filtering (v0.0.1)

- [x] Full-text search (SQLite FTS5)
- [x] Filters: time range, device, entity, feature, function set, classifier,
      direction, SHIP type
- [x] Named filter presets (save/load/delete)
- [x] Device discovery view with entity/feature trees
- [x] Connection state timeline with anomaly detection
- [x] Message correlation (request/response matching by msgCounter)
- [x] Bookmarks and annotations with color-coded labels

### Charts & Visualization (v0.0.2 – v0.0.3)

- [x] Measurement, load control, and setpoint timeseries charts (Chart.js)
- [x] Multi-y-axis support for mixed units
- [x] CSV export for chart data
- [x] Custom chart builder with auto-discovery of chartable data
- [x] Chart definition persistence (create, edit, delete)
- [x] Built-in chart presets (Measurement, Load Control, Setpoint)
- [x] Description context enrichment (phase, scope labels)

### Protocol Intelligence (v0.0.3 – v0.4.0)

- [x] Use case detection from `nodeManagementUseCaseData` (36+ use cases)
- [x] Subscription and binding lifecycle tracking with staleness detection
- [x] Heartbeat jitter analysis (mean/stddev/min/max per device pair)
- [x] Heartbeat metrics export (CSV/JSON)
- [x] Intelligence dashboard page
- [x] `eebustracer analyze` CLI command

### Capture & Discovery (v0.0.1 – v0.4.0)

- [x] mDNS device discovery (`_ship._tcp` via zeroconf)
- [x] mDNS discovery web page and API
- [x] `eebustracer discover` CLI command
- [x] Three capture modes: UDP, TCP, Log Tail
- [x] Three log format parsers: eebus-go, eebustester, EEBus Hub
- [x] SHIP message overview renderers (connectionHello, handshake, pinState,
      accessMethods, connectionClose, init)
- [x] Overview tab as default detail view with per-message summaries
- [x] MIT license

---

## Completed (v0.5.0)

### Correlation & Context (v0.5.0)

- [x] Correlated message highlighting (request/response partners)
- [x] Orphaned request detection with red dot indicator and warning banner
- [x] Use-case-context filtering (filter messages by EEBus use case)
- [x] Richer correlation types: `read-reply`, `write-result`, `call-result`
- [x] Result status badges (accepted/rejected) for SPINE reply messages
- [x] Feature conversation grouping by device pair + function set
- [x] Response latency in Related tab
- [x] DB indexes on `msg_counter` and `msg_counter_ref` (schema v5)

### UI Polish (v0.5.0)

- [x] "Oscillograph" dark theme with amber/gold accents, phosphor grain overlay
- [x] "Blueprint" light theme with warm cream/navy palette
- [x] Self-hosted Space Grotesk + JetBrains Mono variable fonts
- [x] Active state awareness in charts (dashed lines for inactive, ON/OFF annotations)
- [x] Sequence number column in message table
- [x] Classifier color-coding in message rows
- [x] Descriptive tooltips on all UI controls
- [x] About page with version, dependency, and system info
- [x] Collapsible capture controls on trace pages
- [x] Filter active indicator in status bar

---

## Completed (v0.6.0)

### Virtual Scroll & Performance

- [x] Virtual scroll message table: all summaries loaded, only visible rows rendered
- [x] `MessageSummary` lightweight projection (no payloads, ~300 bytes each)
- [x] `GET /api/traces/{id}/messages/summaries` endpoint
- [x] On-demand detail loading with LRU cache (50 entries)
- [x] WebSocket broadcasts use `MessageSummary` instead of full `Message`
- [x] Find-in-view (Ctrl+F) searches all messages in virtual scroll array
- [x] Jump-to-message (Ctrl+G) uses summary array by sequence/counter
- [x] Boolean search operators (OR, AND, NOT) in FTS5 search
- [x] Time range filter inputs in message toolbar

### Protocol Intelligence — New Tabs

- [x] **Dependency tree view**: per-device entity/feature trees with use case pills,
      subscription/binding edges; DOM tree replacing Cytoscape.js force graph
- [x] **Write tracking**: LoadControl/Setpoint write history with result correlation,
      latency, duration, and effective state cards
- [x] **Lifecycle checklist**: 5-step setup verification per device+UC pair (SHIP
      handshake, feature discovery, UC announced, subscriptions, bindings);
      grouped-by-device card layout
- [x] Entity-type filtering for feature discovery and dependency graph (EVSE vs EV)
- [x] DBEVC use case abbreviation and function set mappings

### Protocol Decoding Enhancements

- [x] DeviceConfiguration overview with human-readable key names, typed values
- [x] Entity/feature addressing in message detail overview
- [x] Lifecycle checklist details list specific missing items (not just counts)

### API Endpoints (v0.6.0)

- [x] `GET /api/traces/{id}/depgraph` — dependency tree
- [x] `GET /api/traces/{id}/writetracking` — write tracking
- [x] `GET /api/traces/{id}/lifecycle` — lifecycle checklist

### Pending

- [ ] Click a failed lifecycle step to jump to the relevant message

---

## Future Work

### Sequence Diagram View

- [ ] Message flow diagram between devices (vertical timeline, horizontal arrows)
- [ ] Color-coded by cmdClassifier
- [ ] Click arrow to jump to message detail
- [ ] Time scale (relative / absolute)
- [ ] Filter to show only selected devices or function sets

### Timeline / Swimlane View

- [ ] Horizontal timeline with swimlanes per device
- [ ] Message events as dots on the timeline
- [ ] Zoom and pan, time range selection
- [ ] Overlay: heartbeat intervals, subscription lifetimes

### Enhanced Message Correlation

- [ ] Visual grouping of correlated messages (expand/collapse)

### WebSocket Proxy Capture

- [ ] MITM WebSocket proxy between two EEBus devices
- [ ] TLS termination with configurable certificates
- [ ] Transparent forwarding — no protocol alteration

### PCAP / PCAPNG Import

- [ ] Import Wireshark/tcpdump captures via `github.com/google/gopacket`
- [ ] Extract WebSocket frames from TCP streams
- [ ] TLS decryption via SSLKEYLOGFILE
- [ ] Reassemble fragmented WebSocket messages

### Export & Reporting

- [ ] Export filtered message set to JSON / CSV
- [ ] Generate summary report (HTML/PDF): devices found, use cases,
      errors, connection timeline, key measurements
- [ ] Shareable `.eet` trace files with annotations included

### Comparison Mode

- [ ] Load two traces side-by-side
- [ ] Diff message sequences (useful for regression testing)
- [ ] Highlight added/removed/changed messages

### Release Packaging

- [ ] Cross-compile binaries via `goreleaser`
- [ ] Homebrew formula (`brew install eebustracer`)
- [ ] Docker image for headless capture / CI usage

### CLI Enhancements

- [ ] `eebustracer export trace.eet --format csv` — export to CSV
- [ ] `eebustracer devices trace.eet` — list discovered devices
- [ ] CI/CD integration for EEBus device testing pipelines

### Wireshark Dissector (Companion)

- [ ] Lua dissector for SHIP protocol in Wireshark
- [ ] Lua dissector for SPINE datagrams in Wireshark
- [ ] Register on default SHIP WebSocket port

### Plugin / Extension System

- [ ] Plugin API for custom decoders (vendor-specific extensions)
- [ ] Plugin API for custom analysis checks

---

## Non-Goals (Explicitly Out of Scope)

- **Device simulation** — EEBusTracer is a passive observer, not a protocol
  endpoint
- **Certificate management** — no PKI/trust management UI
- **Cloud connectivity** — all data stays local
- **Protocol modification** — the proxy is transparent, no message rewriting
