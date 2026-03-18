package api

import (
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"runtime"
	"runtime/debug"

	"github.com/eebustracer/eebustracer/internal/store"
)

// TemplateRenderer handles template loading and rendering.
type TemplateRenderer struct {
	pages    map[string]*template.Template
	staticFS fs.FS
}

// NewTemplateRenderer creates a renderer from an embedded filesystem.
// Each page template is parsed separately with the layout and partials so that
// multiple pages can define the same block name (e.g. "content") without
// colliding.
func NewTemplateRenderer(templatesFS, staticFS fs.FS) (*TemplateRenderer, error) {
	// Collect shared templates (layout + partials)
	shared := []string{"layout.html", "partials/*.html"}

	// Page templates are top-level HTML files excluding layout
	pageFiles, err := fs.Glob(templatesFS, "*.html")
	if err != nil {
		return nil, err
	}

	pages := make(map[string]*template.Template)
	for _, pf := range pageFiles {
		if pf == "layout.html" {
			continue
		}
		patterns := append([]string{pf}, shared...)
		tmpl, err := template.ParseFS(templatesFS, patterns...)
		if err != nil {
			return nil, err
		}
		pages[pf] = tmpl
	}

	return &TemplateRenderer{
		pages:    pages,
		staticFS: staticFS,
	}, nil
}

// Render executes a named page template with the given data.
func (t *TemplateRenderer) Render(w io.Writer, name string, data interface{}) error {
	tmpl, ok := t.pages[name]
	if !ok {
		return fmt.Errorf("template %q not found", name)
	}
	return tmpl.ExecuteTemplate(w, name, data)
}

// StaticFS returns the filesystem for static assets.
func (t *TemplateRenderer) StaticFS() fs.FS {
	return t.staticFS
}

func (s *Server) handleIndexPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	traces, err := s.traceRepo.ListTraces()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Traces":    traces,
		"Capturing": s.engine.IsCapturing(),
		"TraceID":   s.engine.TraceID(),
		"Version":   s.version,
	}

	if s.templates == nil {
		writeJSON(w, http.StatusOK, data)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.Render(w, "index.html", data); err != nil {
		s.logger.Error("render index", "error", err)
	}
}

func (s *Server) handleVizPage(w http.ResponseWriter, r *http.Request, templateName string) {
	traceID, err := parseID(r, "id")
	if err != nil {
		http.Error(w, "invalid trace ID", http.StatusBadRequest)
		return
	}

	trace, err := s.traceRepo.GetTrace(traceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if trace == nil {
		http.NotFound(w, r)
		return
	}

	data := map[string]interface{}{
		"Trace":     trace,
		"Capturing": s.engine.IsCapturing(),
		"TraceID":   s.engine.TraceID(),
		"Version":   s.version,
	}

	if s.templates == nil {
		writeJSON(w, http.StatusOK, data)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.Render(w, templateName, data); err != nil {
		s.logger.Error("render "+templateName, "error", err)
	}
}

func (s *Server) handleChartsPage(w http.ResponseWriter, r *http.Request) {
	s.handleVizPage(w, r, "charts.html")
}

func (s *Server) handleIntelligencePage(w http.ResponseWriter, r *http.Request) {
	s.handleVizPage(w, r, "intelligence.html")
}

func (s *Server) handleDiscoveryPage(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"Title":     "Discovery",
		"Capturing": s.engine.IsCapturing(),
		"TraceID":   s.engine.TraceID(),
		"Version":   s.version,
	}

	if s.templates == nil {
		writeJSON(w, http.StatusOK, data)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.Render(w, "discovery.html", data); err != nil {
		s.logger.Error("render discovery", "error", err)
	}
}

func (s *Server) handleTracePage(w http.ResponseWriter, r *http.Request) {
	traceID, err := parseID(r, "id")
	if err != nil {
		http.Error(w, "invalid trace ID", http.StatusBadRequest)
		return
	}

	trace, err := s.traceRepo.GetTrace(traceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if trace == nil {
		http.NotFound(w, r)
		return
	}

	messages, err := s.msgRepo.ListMessages(traceID, store.MessageFilter{Limit: 100000})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	traces, err := s.traceRepo.ListTraces()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Trace":     trace,
		"Traces":    traces,
		"Messages":  messages,
		"Capturing": s.engine.IsCapturing(),
		"TraceID":   s.engine.TraceID(),
		"Version":   s.version,
	}

	if s.templates == nil {
		writeJSON(w, http.StatusOK, data)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.Render(w, "trace.html", data); err != nil {
		s.logger.Error("render trace", "error", err)
	}
}

func (s *Server) handleAboutPage(w http.ResponseWriter, r *http.Request) {
	type dep struct {
		Name    string
		Version string
	}

	// Collect direct dependencies from build info.
	directDeps := map[string]bool{
		"github.com/enbility/ship-go":    true,
		"github.com/enbility/spine-go":   true,
		"github.com/gorilla/websocket":   true,
		"github.com/spf13/cobra":         true,
		"github.com/mattn/go-sqlite3":    true,
		"modernc.org/sqlite":             true,
		"github.com/grandcat/zeroconf":   true,
	}

	var deps []dep
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, d := range info.Deps {
			if directDeps[d.Path] {
				deps = append(deps, dep{Name: d.Path, Version: d.Version})
			}
		}
	}
	if deps == nil {
		deps = []dep{}
	}

	data := map[string]interface{}{
		"Title":     "About",
		"Capturing": s.engine.IsCapturing(),
		"TraceID":   s.engine.TraceID(),
		"Version":   s.version,
		"Project": map[string]string{
			"Name":        "EEBus Tracer",
			"Version":     s.version,
			"Description": "Cross-platform trace recording and analysis tool for the EEBus protocol stack (SHIP + SPINE)",
			"License":     "MIT",
			"Author":      "Andreas Ertel",
		},
		"Dependencies": deps,
		"System": map[string]interface{}{
			"GoVersion": runtime.Version(),
			"OS":        runtime.GOOS,
			"Arch":      runtime.GOARCH,
			"CPUs":      runtime.NumCPU(),
		},
	}

	if s.templates == nil {
		writeJSON(w, http.StatusOK, data)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.Render(w, "about.html", data); err != nil {
		s.logger.Error("render about", "error", err)
	}
}
