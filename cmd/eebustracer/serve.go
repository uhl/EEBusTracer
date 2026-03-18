package main

import (
	"fmt"
	"io/fs"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/eebustracer/eebustracer/internal/api"
	"github.com/eebustracer/eebustracer/internal/capture"
	"github.com/eebustracer/eebustracer/internal/mdns"
	"github.com/eebustracer/eebustracer/internal/parser"
	"github.com/eebustracer/eebustracer/internal/store"
	"github.com/eebustracer/eebustracer/web"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the web UI server",
	RunE:  runServe,
}

var httpPort int

func init() {
	serveCmd.Flags().IntVar(&httpPort, "port", 8080, "HTTP server port")
	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, args []string) error {
	logger := newLogger()

	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	traceRepo := store.NewTraceRepo(db)
	msgRepo := store.NewMessageRepo(db)
	deviceRepo := store.NewDeviceRepo(db)
	presetRepo := store.NewPresetRepo(db)
	bookmarkRepo := store.NewBookmarkRepo(db)
	chartRepo := store.NewChartRepo(db)
	p := parser.New()
	engine := capture.NewEngine(p, msgRepo, logger)
	hub := api.NewHub(logger)
	monitor := mdns.NewMonitor(logger)

	// Load templates
	templatesFS, err := fs.Sub(web.FS, "templates")
	if err != nil {
		return fmt.Errorf("load templates: %w", err)
	}
	staticFS, err := fs.Sub(web.FS, "static")
	if err != nil {
		return fmt.Errorf("load static assets: %w", err)
	}
	templates, err := api.NewTemplateRenderer(templatesFS, staticFS)
	if err != nil {
		return fmt.Errorf("parse templates: %w", err)
	}

	srv := api.NewServer(traceRepo, msgRepo, deviceRepo, presetRepo, bookmarkRepo, chartRepo, engine, hub, monitor, Version, templates, logger)

	addr := fmt.Sprintf(":%d", httpPort)
	logger.Info("starting server", "addr", addr)
	fmt.Printf("EEBus Tracer is running at http://localhost:%d\n", httpPort)

	return http.ListenAndServe(addr, srv.Handler())
}
