package api

import (
	"log/slog"
	"net/http"

	"github.com/gorilla/websocket"

	"github.com/eebustracer/eebustracer/internal/capture"
	"github.com/eebustracer/eebustracer/internal/mdns"
	"github.com/eebustracer/eebustracer/internal/store"
)

// Server is the HTTP API server.
type Server struct {
	traceRepo    *store.TraceRepo
	msgRepo      *store.MessageRepo
	deviceRepo   *store.DeviceRepo
	presetRepo   *store.PresetRepo
	bookmarkRepo *store.BookmarkRepo
	chartRepo    *store.ChartRepo
	engine       *capture.Engine
	hub          *Hub
	mdnsMonitor  *mdns.Monitor
	version      string
	templates    *TemplateRenderer
	logger       *slog.Logger
	upgrader     websocket.Upgrader
}

// NewServer creates a new API server.
func NewServer(
	traceRepo *store.TraceRepo,
	msgRepo *store.MessageRepo,
	deviceRepo *store.DeviceRepo,
	presetRepo *store.PresetRepo,
	bookmarkRepo *store.BookmarkRepo,
	chartRepo *store.ChartRepo,
	engine *capture.Engine,
	hub *Hub,
	mdnsMonitor *mdns.Monitor,
	version string,
	templates *TemplateRenderer,
	logger *slog.Logger,
) *Server {
	return &Server{
		traceRepo:    traceRepo,
		msgRepo:      msgRepo,
		deviceRepo:   deviceRepo,
		presetRepo:   presetRepo,
		bookmarkRepo: bookmarkRepo,
		chartRepo:    chartRepo,
		engine:       engine,
		hub:          hub,
		mdnsMonitor:  mdnsMonitor,
		version:      version,
		templates:    templates,
		logger:       logger,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

// Handler returns the HTTP handler with all routes registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("GET /api/traces", s.handleListTraces)
	mux.HandleFunc("POST /api/traces", s.handleCreateTrace)
	mux.HandleFunc("GET /api/traces/{id}", s.handleGetTrace)
	mux.HandleFunc("PATCH /api/traces/{id}", s.handleRenameTrace)
	mux.HandleFunc("DELETE /api/traces/{id}", s.handleDeleteTrace)

	mux.HandleFunc("GET /api/traces/{id}/messages", s.handleListMessages)
	mux.HandleFunc("GET /api/traces/{id}/messages/{mid}", s.handleGetMessage)

	mux.HandleFunc("GET /api/capture/status", s.handleCaptureStatus)
	mux.HandleFunc("POST /api/capture/start", s.handleCaptureStart)
	mux.HandleFunc("POST /api/capture/start/logtail", s.handleCaptureStartLogTail)
	mux.HandleFunc("POST /api/capture/start/tcp", s.handleCaptureStartTCP)
	mux.HandleFunc("POST /api/capture/stop", s.handleCaptureStop)

	mux.HandleFunc("POST /api/traces/import", s.handleImport)
	mux.HandleFunc("GET /api/traces/{id}/export", s.handleExport)

	mux.HandleFunc("GET /api/traces/{id}/live", s.handleWebSocket)

	// Filter presets
	mux.HandleFunc("GET /api/presets", s.handleListPresets)
	mux.HandleFunc("POST /api/presets", s.handleCreatePreset)
	mux.HandleFunc("DELETE /api/presets/{id}", s.handleDeletePreset)

	// Bookmarks
	mux.HandleFunc("GET /api/traces/{id}/bookmarks", s.handleListBookmarks)
	mux.HandleFunc("POST /api/traces/{id}/bookmarks", s.handleCreateBookmark)
	mux.HandleFunc("DELETE /api/bookmarks/{id}", s.handleDeleteBookmark)

	// Device discovery
	mux.HandleFunc("GET /api/traces/{id}/devices", s.handleListDevices)
	mux.HandleFunc("GET /api/traces/{id}/devices/{did}", s.handleGetDevice)

	// Connection state
	mux.HandleFunc("GET /api/traces/{id}/connections", s.handleListConnections)

	// Message correlation
	mux.HandleFunc("GET /api/traces/{id}/messages/{mid}/related", s.handleRelatedMessages)
	mux.HandleFunc("GET /api/traces/{id}/messages/{mid}/conversation", s.handleConversation)
	mux.HandleFunc("GET /api/traces/{id}/orphaned-requests", s.handleOrphanedRequests)
	mux.HandleFunc("GET /api/traces/{id}/usecase-context", s.handleUseCaseContext)

	// Timeseries & description APIs
	mux.HandleFunc("GET /api/traces/{id}/timeseries", s.handleTimeseries)
	mux.HandleFunc("GET /api/traces/{id}/timeseries/discover", s.handleDiscoverTimeseries)
	mux.HandleFunc("GET /api/traces/{id}/descriptions", s.handleDescriptions)

	// Chart definitions
	mux.HandleFunc("GET /api/traces/{id}/charts", s.handleListCharts)
	mux.HandleFunc("POST /api/traces/{id}/charts", s.handleCreateChart)
	mux.HandleFunc("GET /api/charts/{id}", s.handleGetChart)
	mux.HandleFunc("PATCH /api/charts/{id}", s.handleUpdateChart)
	mux.HandleFunc("DELETE /api/charts/{id}", s.handleDeleteChart)
	mux.HandleFunc("GET /api/traces/{id}/charts/{cid}/data", s.handleChartData)

	// Protocol intelligence APIs
	mux.HandleFunc("GET /api/traces/{id}/usecases", s.handleListUseCases)
	mux.HandleFunc("GET /api/traces/{id}/subscriptions", s.handleListSubscriptions)
	mux.HandleFunc("GET /api/traces/{id}/bindings", s.handleListBindings)
	mux.HandleFunc("GET /api/traces/{id}/metrics", s.handleMetrics)
	mux.HandleFunc("GET /api/traces/{id}/metrics/export", s.handleMetricsExport)

	// mDNS discovery
	mux.HandleFunc("GET /api/mdns/devices", s.handleMDNSDevices)
	mux.HandleFunc("GET /api/mdns/status", s.handleMDNSStatus)
	mux.HandleFunc("POST /api/mdns/start", s.handleMDNSStart)
	mux.HandleFunc("POST /api/mdns/stop", s.handleMDNSStop)

	// Web UI pages
	mux.HandleFunc("GET /", s.handleIndexPage)
	mux.HandleFunc("GET /traces/{id}", s.handleTracePage)
	mux.HandleFunc("GET /traces/{id}/charts", s.handleChartsPage)
	mux.HandleFunc("GET /traces/{id}/intelligence", s.handleIntelligencePage)
	mux.HandleFunc("GET /discovery", s.handleDiscoveryPage)
	mux.HandleFunc("GET /about", s.handleAboutPage)

	// Static assets
	if s.templates != nil {
		mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(s.templates.StaticFS()))))
	}

	return mux
}
