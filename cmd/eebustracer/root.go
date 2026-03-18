package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/eebustracer/eebustracer/internal/store"
)

var (
	dbPath  string
	verbose bool
)

var rootCmd = &cobra.Command{
	Use:   "eebustracer",
	Short: "EEBus protocol trace recorder and analyzer",
	Long:  "EEBusTracer captures, decodes, and visualizes EEBus protocol communication (SHIP + SPINE).",
}

func init() {
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", "", "database file path (default: ~/.eebustracer/traces.db)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")
}

func Execute() error {
	return rootCmd.Execute()
}

func resolveDBPath() string {
	if dbPath != "" {
		return dbPath
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "traces.db"
	}
	dir := filepath.Join(home, ".eebustracer")
	_ = os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, "traces.db")
}

func openDB() (*store.DB, error) {
	path := resolveDBPath()
	db, err := store.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open database at %s: %w", path, err)
	}
	if err := db.Migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}
	return db, nil
}

func newLogger() *slog.Logger {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}
