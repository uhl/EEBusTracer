# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.5.0] - 2026-03-19

### Added
- Correlated message highlighting: selecting a message highlights its
  request/response partners in the message table with a teal left border
- Orphaned request detection: requests with no response are marked with a red
  dot in the sequence column and a warning banner in the detail overview
- `GET /api/traces/{id}/orphaned-requests` endpoint returning IDs of
  unanswered request messages
- Use-case-context filtering: new dropdown in filter toolbar filters messages
  by EEBus use case (e.g., LPC shows LoadControlLimit messages from
  participating devices)
- `GET /api/traces/{id}/usecase-context` endpoint returning detected use cases
  with associated devices and function sets
- Multi-value function set filtering (comma-separated) in message list API
- Response latency in Related tab (time delta between request and reply)
- Richer correlation types: `read-reply`, `write-result`, `call-result`
  (previously all `request-response`)
- Result status badges (accepted/rejected) for SPINE reply messages containing
  `resultData`
- Feature conversation grouping: "Conversation" section in Related tab shows all
  messages between the same device pair and function set
- `GET /api/traces/{id}/messages/{mid}/conversation` endpoint with pagination
- DB indexes on `msg_counter` and `msg_counter_ref` columns (schema v5)

### Changed
- Related tab redesigned with "Direct Correlation" and "Conversation" sections

### Changed
- Capture controls auto-collapse on trace pages when not actively capturing,
  reducing visual clutter; a compact "Capture" button expands them on demand;
  controls auto-expand when a capture is active and re-collapse when stopped
- Filter preset icon changed from gear (&#9881;) to star (&#9733;) to better
  convey "saved presets" rather than "settings"
- Filter and Find toolbar sections are now visually distinct: the Filter section
  has a contained panel background with border radius, and the Find input is
  narrower, preventing confusion between the server-side filter (FTS round-trip)
  and client-side find (highlight in view)
- Messages list API endpoint (`GET /api/traces/{id}/messages`) now returns an
  `X-Total-Count` response header with the total number of matching messages,
  enabling pagination-aware UIs
- Trace page initial load capped at 2,000 messages (was 100,000); a "Load more"
  button fetches additional pages of 2,000 rows using offset pagination, avoiding
  browser lockups on large traces

### Fixed
- Filter active indicator: when a filter reduces the visible message set, the
  filter toolbar section shows an accent border and the status bar displays
  "Showing X of Y messages" in accent color; clearing the filter restores
  the normal state
- Message table no longer renders up to 100K DOM rows on filter; capped at
  2,000 per page with load-more pagination

### Added
- Sequence number column (`#`) in the message table, making `Ctrl+G` "Jump to
  message" discoverable and usable
- Classifier color-coding in message table rows: read (teal), reply (green),
  write (amber), call (purple), notify (blue) — matching the existing detail
  panel badge colors
- Delete button on filter presets: hover a preset in the dropdown to reveal a
  `×` button that deletes it via the existing `DELETE /api/presets/{id}` endpoint
- Descriptive tooltips on all UI controls: toolbar inputs, nav tabs, column
  headers, detail panel tabs, chart controls, intelligence tabs, discovery
  buttons, and capture controls; uses native HTML `title` attributes for
  browser-native hover hints
- Light theme toggle: "Blueprint" light theme with warm cream/navy palette,
  togglable via sun/moon button in top bar; preference persists via localStorage;
  auto-detects system dark/light preference via `prefers-color-scheme` when no
  explicit choice is stored, and follows live system theme changes;
  all CSS custom properties switch via `[data-theme="light"]` override block;
  charts, syntax highlighting, and detail panel re-render on theme change

### Changed
- Unified filter & find toolbar on the trace page: merged the separate filter
  bar and hidden find bar (Ctrl+F) into a single always-visible toolbar with
  labeled FILTER and FIND sections separated by a vertical divider; filter
  presets consolidated from a select dropdown + save button into a single
  gear-icon dropdown with saved presets and a "Save current filter..." action;
  find input is always visible (Ctrl+F focuses it); arrows use triangular
  glyphs matching the oscillograph aesthetic
- Complete visual redesign: dark-only "Oscillograph" aesthetic replacing the
  dual light/dark Catppuccin theme; warm-tinted dark palette inspired by
  high-end protocol analyzers and oscilloscopes (amber/gold accent, teal data,
  phosphor green success, neon chart colors)
- Self-hosted variable fonts: Space Grotesk (headings, nav, buttons) and
  JetBrains Mono (tables, JSON, timestamps, data) as embedded WOFF2 assets,
  replacing system monospace fallbacks
- Atmospheric effects: CRT phosphor grain noise texture overlay, oscilloscope
  graticule grid pattern on main content area, amber glow accents on focused
  and hovered elements, gradient edge lines on sidebar and detail panel
- Inset shadow on all input and select elements for recessed instrument control
  feel; refined thin scrollbar with amber-tinted thumb
- Page load animations: staggered reveal-up sequence on all pages (top bar,
  main container, status bar, page sections) using CSS animation delays
- Hover states: message rows and trace items gain translateY(-1px) lift with
  amber glow left border; buttons gain glow ring; cards lift with shadow
- New message arrival animation: slideInLeft on dynamically inserted table rows
  during live capture
- Auto-scroll pulse redesigned with amber glow instead of blue
- Discovery cards render with staggered entrance animation
- Removed theme toggle button and light theme; UI is now dark-only

### Added
- Active state awareness in charts: when a Load Control Limit or Setpoint has
  `isActive = false` in the SPINE payload, the chart renders dashed line segments
  for inactive values, vertical annotation lines with ON/OFF labels at state
  transitions, and active/inactive status in tooltips
- `IsActive *bool` field on `TimeseriesDataPoint` (JSON: `isActive`, omitted
  when nil for backward compatibility)
- `ActiveField` on `ExtractionDescriptor` for parameterized active-state
  extraction (`isLimitActive` for loadcontrol, `isSetpointActive` for setpoint)
- CSS custom properties `--chart-annotation-line` and `--chart-annotation-text`
  for theme-aware annotation styling (light and dark themes)
- About page (`/about`) in the web UI showing project info (name, version,
  description, license, author), dependency versions (from Go build info), and
  system/runtime information (Go version, OS, architecture, CPUs)
- "About" navigation link in the header bar

### Changed
- Function Set column now shows the SHIP message type (e.g. connectionHello,
  messageProtocolHandshake, accessMethods) for non-data SHIP messages, making
  the SHIP connection establishment sequence visible in the message table
- ROADMAP.md cleaned up: removed incorrectly checked items (sequence diagram,
  swimlane view were never implemented), updated CLI items already done,
  reorganized into "Completed" and "Future Work" sections with accurate
  version references, removed stale milestones table

### Fixed
- Detail panel tab highlight now resets to "Overview" when selecting a new
  message; previously the previously active tab (e.g. "Decoded JSON") kept its
  highlight even though the Overview content was shown

## [0.4.0] - 2026-03-16

### Changed
- Improved SHIP handshake overview display in the web UI:
  - Color-coded SHIP type badges (init=gray, connectionHello=blue,
    handshake=purple, pinState=orange, accessMethods=teal, close=red)
  - Phase/state indicators with colored dots (connectionHello, connectionClose,
    connectionPinState)
  - Protocol version formatted as "1.0" instead of raw JSON `{"major":1,"minor":0}`
  - Formats displayed as comma-separated list instead of JSON array
  - Handshake type (announceMax/select) shown as badge
  - accessMethods distinguishes request vs response with badge, shows DNS URI
  - connectionPinState shows `inputPermission` field
  - connectionClose shows `reason` and `maxTime` with unit
  - Init frame shows CMI header byte description

### Removed
- Raw Hex tab from the message detail panel

### Added
- EEBus Hub log format support: import `.log` files from EEBus Hub with
  content-based auto-detection; the format includes full SHIP data messages
  (not just SPINE datagrams) and uses SKI (Subject Key Identifier) as peer
  identifier
- `EEBusHubLogRegex`, `ParseEEBusHubTimestamp` in `internal/parser/logformat.go`
  for parsing the EEBus Hub log format
- `ParseShipFromJSON` method on Parser for classifying full SHIP JSON messages
  and extracting SPINE fields (used by EEBus Hub importer and future formats
  that include SHIP framing)
- `ImportEEBusHubLogFile` in `internal/store/logimport.go` for importing EEBus
  Hub logs with auto-generated sequence numbers and SKI-based peer identification
- `ImportLogFileAutoDetect` now detects and routes EEBus Hub format files
  automatically

### Fixed
- Heartbeat jitter metrics now track each direction separately (A → B, B → A)
  instead of merging both directions into a single bidirectional entry; previously
  interleaved timestamps from both directions produced incorrect interval
  calculations and only one entry was shown

## [0.0.3] - 2026-03-10

### Added
- MIT LICENSE file (Copyright 2026 Andreas Ertel)

### Removed
- "Use Case" column from LoadControlLimitListData overview table; the column
  used a hardcoded scopeType-to-use-case mapping that was incorrect (e.g.
  `selfConsumption` was mapped to OSCEV instead of LPC). The scope information
  is already shown in the "Scope" column.

### Added
- Multi-y-axis support for charts: when series have different units (e.g. W and
  A), each unit group gets its own y-axis with labeled title; first unit on the
  left axis, additional units on right axes with grid lines only on the primary
  axis
- Series toggle labels now show the unit in parentheses (e.g. "Current Phase A (A)")
- Chart tooltips now display the unit after the value (e.g. "Power: 2300 W")
- Custom chart builder: create user-defined timeseries charts from any SPINE data
  containing ScaledNumber values, not just the three hardcoded types
- Auto-discovery of chartable data: `GET /api/traces/{id}/timeseries/discover`
  scans a trace to find function sets with numeric values and reports available
  data sources with sample IDs and message counts
- Chart definition persistence: save, update, and delete chart configurations
  in SQLite (`chart_definitions` table, schema v4); charts can be global or
  trace-specific
- Chart definition CRUD API: `GET/POST /api/traces/{id}/charts`,
  `GET/PATCH/DELETE /api/charts/{id}`,
  `GET /api/traces/{id}/charts/{cid}/data` (render timeseries from saved definition)
- Chart builder UI: "New Chart" button opens a dialog that discovers available
  data sources, lets the user select sources and chart type (line/step), and
  saves as a reusable chart definition
- Three built-in chart presets (Measurements, Load Control, Setpoints) seeded
  on migration; built-in charts cannot be deleted

### Changed
- Timeseries extraction refactored from three copy-paste extractors to a single
  generic `extractGenericData`/`extractGenericSeries` parameterized by
  `ExtractionDescriptor` (cmdKey, dataArrayKey, idField, classifiers)
- Charts page: hardcoded `<select>` replaced with a dynamic chart selector
  populated from the chart definitions API; custom charts show a delete button

## [0.0.2] - 2026-03-09

### Changed
- Trace page navigation (Messages, Charts, Insights) moved to right side of
  header as a fixed-width segmented button group; "Intelligence" renamed to
  "Insights"
- Export and Delete actions moved into a dropdown menu (⋮ button) to declutter
  the header

### Fixed
- Clearing a search filter now restores all messages instead of showing only 100
- Live capture messages are now filtered client-side to match the active filter;
  previously new WebSocket messages bypassed the filter and appeared unfiltered

## [0.0.1] - 2026-03-09

### Changed
- Unified trace page navigation: Messages, Charts, and Intelligence pages now
  share the same header layout with tab navigation, trace name, and Export/Delete
  actions; removed separate `.viz-header` and `.viz-nav` styling in favor of a
  single `.trace-nav` component
- Simplified filter bar: removed SHIP Type, Direction, Device, and Function Set
  filters (redundant with full-text search); kept Search, Classifier, Reset, and
  Presets

### Added
- Version number displayed in the status bar (bottom-right), overridable at
  build time via `-ldflags "-X main.Version=..."`
- Resizable detail panel: drag the handle between the message table and the
  detail panel to adjust their relative sizes; height persists across sessions
  via localStorage

### Added
- Auto-scroll toggle button on trace page: during live capture, auto-scroll
  pauses when scrolling up to inspect earlier messages and resumes when scrolling
  back to bottom or clicking the floating arrow button; a pulsing indicator
  signals new messages arrived while scrolled away
- Inline rename and delete buttons on sidebar trace items (hover to reveal);
  rename updates the trace name in-place without page reload, delete removes the
  trace from the sidebar (redirects to index if deleting the active trace)
- `PATCH /api/traces/{id}` endpoint for renaming traces (accepts `{"name": "..."}`)

### Fixed
- TCP capture: rewrote TCP source to handle CNetLogServer's fixed-size C buffers
  which contain multiple messages concatenated with binary garbage (no newlines).
  Replaced `bufio.Scanner` with raw `conn.Read()` and regex-based multi-message
  extraction, fixing silently dropped messages and scanner hangs during live
  TCP capture of large initial buffer dumps (244KB+)
- TCP/LogTail capture: unmatched lines are now logged at DEBUG level (visible
  with `-v`) instead of being silently dropped, aiding diagnosis of format issues

### Added
- Devices sidebar tab on trace page showing mDNS-discovered devices and recent
  capture targets; click any entry to populate the capture mode/host/port fields
  in the header bar for quick capture start

### Changed
- Trace page sidebar: removed unused Devices and Connections tabs (data was
  never populated); trace list is now server-rendered with the current trace
  highlighted, matching the index page layout
- Renamed trace file extension from `.eebt` to `.eet` across the codebase;
  old `.eebt` files remain importable for backward compatibility
- Display title changed from "EEBusTracer" to "EEBus Tracer" (with space) in
  browser tab, header bar, welcome page, and CLI startup message
- Internal file format types renamed: `EEBTFile` → `EETFile`,
  `EEBTTrace` → `EETTrace`, `EEBTMessage` → `EETMessage`

### Added
- EET favicon: SVG logo with "EET" monogram on a blue gradient background and
  a glowing fingertip on the T (E.T. movie reference), displayed in browser tabs
- Recent capture targets: previously used connection targets (host:port, file
  path) appear as autocomplete suggestions in capture input fields, persisted
  in browser localStorage (max 10 entries, auto-fills port when host is selected)
- TCP capture source: connect to CEasierLogger `CNetLogServer` (or any TCP log
  server) and receive newline-delimited trace lines in real time
  (`internal/capture/source_tcp.go`)
- `POST /api/capture/start/tcp` endpoint for starting TCP capture via the web UI
- `--tcp` flag on `eebustracer capture` for headless TCP log server capture
  (e.g. `eebustracer capture --tcp 192.168.20.41:54546`)
- TCP capture mode in web UI header: select "TCP", enter host and port, start
  capture
- CEasierLogger format support: `LogLineRegex` now accepts lines without a
  sequence number prefix (e.g. `[HH:MM:SS.zzz] SEND to ...`), with
  auto-generated sequence numbers when absent
- Tests: TCP source (connect, read, malformed lines, shutdown, connection
  refused), parser regex with no-sequence-number lines, logimport with
  CEasierLogger format

### Fixed
- Drag-and-drop file import: replaced drop-zone-specific handlers (broken by
  CSS `pointer-events: none` interfering with drag events) with document-level
  drop handling that works on all pages, with a full-page overlay for visual
  feedback

### Added
- GitLab CI/CD pipeline (`.gitlab-ci.yml`): lint, test, and cross-compile build
  stages using `golang:1.24` Docker image with `golangci-lint` v2, race-detector
  tests, and 6 parallel matrix jobs for Linux/macOS/Windows on amd64/arm64
- eebustester log format support: import `.log` files from the EEBus Living Lab
  eebustester tool with content-based auto-detection between eebus-go and
  eebustester formats
- `EEBusTesterLogRegex`, `ParseEEBusTesterTimestamp`, `DetectLogFormat` in
  `internal/parser/logformat.go` for parsing the eebustester log format
- `ImportEEBusTesterLogFile` and `ImportLogFileAutoDetect` in
  `internal/store/logimport.go` for importing eebustester logs with
  auto-generated sequence numbers
- CLI `import` and `analyze` commands now accept `.log` files with auto-detection
- Web UI drag-and-drop import handles eebustester `.log` files automatically

### Changed
- Web UI redesigned with a fresh, light "Soft Modern" theme as default;
  dark Catppuccin theme preserved as toggleable alternative via header button
  (persisted in localStorage)
- All hardcoded colors replaced with CSS custom properties for consistent
  theming across badges, charts, syntax highlighting, and overlays

### Added
- Keyboard shortcuts for trace page: j/k to navigate messages, Ctrl+F to
  focus search, Ctrl+L to focus filters, Ctrl+G to jump to a message by
  sequence number, arrow keys for message navigation, ? to show keyboard
  help overlay

### Changed
- Sidebar is now collapsible: click the toggle arrow to hide/show the
  Traces/Devices/Connections/Bookmarks panel; state persists across sessions
  via localStorage

### Changed
- Overview tab renderers refactored to use a registry pattern: adding a new
  decoder is now a single `registerOverview()` call + a render function, with no
  switch statement or manual HTML table construction needed
- New `overviewTable()` and `overviewKV()` builder helpers eliminate repetitive
  HTML construction across all overview renderers
- New `fmtScaled()` helper for displaying ScaledNumber values with optional units
- Pattern-based renderer matching via `registerOverviewPattern()` for substring
  matches (Subscription, Binding)

### Added
- `docs/OVERVIEW_RENDERERS.md`: guide for adding new Overview tab decoders with
  step-by-step examples for table, KV, and pattern-based renderers

### Added
- Overview tab in message detail panel as the default view, replacing raw JSON
  as the first thing users see when clicking a message
- Per-message summaries for all SPINE function sets: LoadControl (with Use Case
  column), Measurement, Setpoint, Heartbeat, DiagnosisState, Manufacturer,
  description list types, NodeManagement discovery/use cases/subscriptions/bindings
- Result data badge: green "Accepted" / red "Rejected" with error number for
  SPINE reply messages containing `resultData`
- SHIP message overviews: connectionHello (phase, waiting), handshake (type,
  version, formats), pinState, accessMethods, connectionClose (phase, reason)
- Classifier badges with color-coded styling (read/reply/write/call/notify)
- Generic fallback overview with direction, classifier, function set, and device
  path for unknown message types

### Changed
- Decoded JSON tab is now pure JSON only (enriched tables moved to Overview tab)
- Overview tab is the default active tab when selecting a message

### Removed
- Topology map page (`/traces/{id}/topology`), API endpoint
  (`GET /api/traces/{id}/topology`), and D3.js vendor dependency
- Topology button from trace page and navigation links from charts/intelligence

### Fixed
- Subscription notify counting: notifies were never matched to subscriptions
  because the feature address format differed between subscription entries
  (entity+feature combined, e.g., "1.2") and message fields (entity and feature
  stored separately). All subscriptions appeared stale with zero notify count.
- Subscription staleness: subscriptions are no longer incorrectly marked stale
  when matching notifies exist in the trace

### Added
- Description context enrichment: join `ElectricalConnectionParameterDescriptionListData`,
  `MeasurementDescriptionListData`, and `LoadControlLimitDescriptionListData` to
  provide phase (A/B/C/Total) and scope (Overload Protection, Self Consumption,
  Discharge) labels for measurements and limits (`internal/api/descriptions.go`)
- `GET /api/traces/{id}/descriptions` — unified description context API endpoint
- Enriched detail view in web UI: LoadControlLimitListData and MeasurementListData
  messages now show a summary table with phase, scope, value, and active status
  above the JSON display
- LoadControl timeseries now includes `write` messages (CEM writing limits to
  EVSE), which are the most important for analysis

### Changed
- Measurement chart labels now show enriched format: "Current Phase A [A]"
  instead of "current (acCurrent) [A]"
- LoadControl chart labels now show enriched format: "Overload Protection Phase A
  [A]" instead of "Limit 1"
- Timeseries series now include a `unit` field from description context
- Use case detection: parse `nodeManagementUseCaseData` to identify active use
  cases per device (LPC, MPC, MGCP, EV charging, etc.) with abbreviation mapping
  for 36+ SPINE use case names (`internal/analysis/usecases.go`)
- Subscription & binding tracker: lifecycle tracking from
  `nodeManagementSubscriptionData`/`RequestCall`/`DeleteCall` messages, staleness
  detection based on notify activity (`internal/analysis/subscriptions.go`)
- Heartbeat accuracy metrics: heartbeat jitter statistics (mean/stddev/min/max)
  per device pair (`internal/analysis/metrics.go`)
- New `internal/analysis/` package: reusable protocol analysis logic independent
  of HTTP layer
- `GET /api/traces/{id}/usecases` — detected use cases per device
- `GET /api/traces/{id}/subscriptions` — subscription tracker with staleness
- `GET /api/traces/{id}/bindings` — binding tracker
- `GET /api/traces/{id}/metrics` — heartbeat accuracy metrics
- `GET /api/traces/{id}/metrics/export` — export heartbeat metrics as CSV or JSON
- `GET /traces/{id}/intelligence` — web UI intelligence page with tabs for use
  cases, subscriptions, and heartbeat accuracy
- `eebustracer analyze <file>` CLI command: run protocol analysis on `.eet`
  files with `--check usecases|subscriptions|metrics|all` and
  `--output text|json` flags
- Intelligence page with use case pills, subscription/binding status indicators,
  heartbeat jitter table with CSV/JSON export
- Intelligence navigation link on charts and trace pages
- 30+ new tests: use case detection, subscription lifecycle, heartbeat metrics,
  API endpoints, CLI analyze command
- Pluggable capture Source interface: decouple engine from UDP transport,
  enabling log tail and future capture modes (`internal/capture/source.go`)
- Live log tail capture: watch an eebus-go log file in real time and parse new
  lines as they appear (`internal/capture/source_logtail.go`)
- `POST /api/capture/start/logtail` endpoint for starting log tail capture via
  the web UI
- `--log-file` flag on `eebustracer capture` for headless log tail capture
- Capture mode selector in web UI header: switch between UDP and Log Tail modes
- mDNS device discovery: browse `_ship._tcp` services on the local network via
  `github.com/grandcat/zeroconf` (`internal/mdns/`)
- mDNS API endpoints: `GET /api/mdns/devices`, `GET /api/mdns/status`,
  `POST /api/mdns/start`, `POST /api/mdns/stop`
- mDNS device persistence: `mdns_devices` table (schema v3)
- mDNS discovery web page at `/discovery` with device cards, online/offline
  badges, and "Connect" button to pre-fill capture target
- `eebustracer discover` CLI command: discover EEBus devices on the local
  network with `--timeout` and `--json` flags
- Typed WebSocket events: `{type, payload}` envelope format distinguishing
  `message` events from `mdns_device` events
- Shared log parsing helpers extracted to `internal/parser/logformat.go`:
  `LogLineRegex`, `ParseLogTimestamp`, `ExtractPeerDevice`
- Navigation link to Discovery page in header
- `SourceType` field in capture stats and status API response
- 15+ new tests: Source interface, UDP source, log tail source (new lines,
  malformed lines, shutdown, file not found), mDNS monitor (handle entry,
  update, callbacks, device list, online/offline, TXT parsing), logformat
  parsing
- Schema migration v3: `mdns_devices` table for persisting discovered devices

### Changed
- Capture engine refactored to use context-based cancellation instead of
  connection close for shutdown
- `Engine.Start()` now delegates to `Engine.StartWithSource()` via `UDPSource`
- `Hub.Broadcast()` now wraps messages in typed `{type: "message", payload}`
  envelope via `BroadcastEvent()`
- `NewServer` constructor updated with `*mdns.Monitor` parameter
- Web UI WebSocket handler now parses typed events and dispatches by type
- Log import (`store/logimport.go`) updated to use shared `parser.LogLineRegex`,
  `parser.ParseLogTimestamp`, `parser.ExtractPeerDevice`

- Device topology map: D3.js force-directed graph with nodes sized by message
  count, edges colored by connection state, drag/zoom/pan interaction
- Measurement, load control & setpoint charts: Chart.js line/step charts with
  per-series toggles, zoom via chartjs-plugin-zoom, CSV export
- `GET /api/traces/{id}/topology` endpoint returning nodes and edges with
  message counts, connection states, and function sets
- `GET /api/traces/{id}/timeseries` endpoint extracting measurement, load
  control, and setpoint values from SPINE payloads with ScaledNumber conversion
- Measurement description enrichment from `MeasurementDescriptionListData`
  replies for human-readable series labels
- Setpoint description enrichment from `SetpointDescriptionListData` replies
  for human-readable series labels
- Visualization page templates: `/traces/{id}/topology`, `/traces/{id}/charts`
- Direct Topology and Charts buttons on trace page
- Navigation bar on visualization pages linking between views
- Shared JavaScript utilities in `common.js` (CMD_COLORS, escapeHtml,
  syntaxHighlight, formatHex, apiFetch, formatTimestamp)
- Vendored D3.js v7, Chart.js v4, chartjs-plugin-zoom v2, Hammer.js v2
  (no CDN dependencies)
- 20+ new tests: topology builder, ScaledNumber conversion, measurement
  extraction, load control extraction, timeseries API, description enrichment,
  time range filter, visualization page routes
- Full-text search across message content using SQLite FTS5 virtual table
- Extended message filtering: time range, device (source/dest/either), entity,
  feature, and combined filters with FTS search
- Filter presets: save, load, and delete named filter configurations via
  `GET/POST/DELETE /api/presets`
- Device discovery view: parse `nodeManagementDetailedDiscoveryData` replies to
  build entity/feature trees, `GET /api/traces/{id}/devices` and
  `GET /api/traces/{id}/devices/{did}` endpoints
- Connection state view: compute SHIP connection lifecycle from message history,
  detect anomalies (missing hello, unexpected close, out-of-order transitions),
  `GET /api/traces/{id}/connections`
- Message correlation: match request/response pairs by `msgCounter`/`msgCounterRef`,
  classify relationships (request-response, call-result, subscription-notify),
  `GET /api/traces/{id}/messages/{mid}/related`
- Bookmarks and annotations: create, list, and delete bookmarks on messages with
  custom labels and colors, `GET/POST /api/traces/{id}/bookmarks` and
  `DELETE /api/bookmarks/{id}`
- Web UI filter toolbar with search input, dropdown filters (cmdClassifier,
  shipMsgType, direction), device and function set text filters, and clear button
- Web UI filter preset save/load controls
- Web UI sidebar tabs for Devices, Connections, and Bookmarks panels
- Web UI device tree panel: click device to filter messages to/from it
- Web UI connection timeline panel with state markers and anomaly indicators
- Web UI bookmark panel with jump-to-message navigation
- Web UI "Related Messages" tab in detail panel showing correlated messages
- Web UI bookmark button in message detail panel
- Web UI "Load More" pagination button for message table
- Web UI drag-and-drop .eet/.log file import on welcome page
- Web UI "Load from file" button on welcome page for file picker import
- `.log` file import: parse eebus-go log format (`<seq> [HH:MM:SS.mmm] SEND|RECV to|from <peer> MSG: <json>`)
  with EEBUS JSON normalization and SPINE field extraction
- `ParseSpineFromJSON` method on Parser for parsing standalone SPINE datagrams
  without SHIP framing (used by log importer)
- Schema migration v2: FTS5 virtual table with triggers, bookmarks table,
  filter_presets table, automatic backfill of existing messages into FTS index
- `FindByMsgCounter` and `FindByMsgCounterRef` queries in MessageRepo
- 20+ new tests covering FTS search, time range filtering, device filtering,
  combined filters, preset CRUD, bookmark CRUD with cascade delete, connection
  state building, anomaly detection, and message correlation

### Changed
- `MessageFilter` struct extended with Search, TimeFrom, TimeTo, DeviceSource,
  DeviceDest, Device, EntitySource, EntityDest, FeatureSource, FeatureDest fields
- `ListMessages` query builder now uses table alias and supports FTS JOIN
- `Server` struct and `NewServer` constructor updated with `PresetRepo` and
  `BookmarkRepo` parameters
- Message table now includes bookmark indicator column
- Detail panel now includes "Related" tab and bookmark action button

- Project scaffold: Go module, directory layout, Makefile, linter config
- Domain types: Trace, Message, Device, Direction/ShipMsgType enums (`internal/model/`)
- SHIP message parser: EEBUS JSON normalization, SHIP message classification by
  top-level JSON key (connectionHello, messageProtocolHandshake, connectionPinState,
  accessMethods, data, connectionClose) (`internal/parser/`)
- SPINE datagram parser: extract cmdClassifier, function set (via `CmdType.DataName()`),
  addresses, msgCounter from SPINE datagrams (`internal/parser/`)
- SQLite persistence with WAL mode: schema migration, traces/messages/devices tables
  with indexes (`internal/store/`)
- Repository layer: TraceRepo, MessageRepo (with batch insert and filtered pagination),
  DeviceRepo (with upsert) (`internal/store/`)
- UDP capture engine: goroutine-based packet receiver, batch insert (100 msgs / 500ms),
  per-message callbacks for WebSocket fan-out (`internal/capture/`)
- REST API using Go 1.22 routing: full CRUD for traces, paginated/filtered message
  listing, capture start/stop/status endpoints (`internal/api/`)
- WebSocket live streaming hub: fan-out to multiple clients, slow-client message
  dropping (`internal/api/`)
- File I/O: `.eet` JSON-based trace export/import format with version checking
  (`internal/store/`)
- Web UI: embedded Go templates + static assets, dark Catppuccin-inspired theme,
  trace sidebar, message table, detail panel with JSON syntax highlighting,
  raw hex view, and headers tab (`web/`)
- CLI commands via cobra: `serve`, `capture`, `import`, `version` with persistent
  `--db` and `--verbose` flags (`cmd/eebustracer/`)
- Comprehensive test suite: 30+ tests covering model, parser (normalize, SHIP, SPINE),
  store (DB, migrations, repos, fileio), capture engine, API endpoints, and CLI
- Project roadmap (ROADMAP.md)
- Project rules and conventions (CLAUDE.md)
- This changelog
