package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/eebustracer/eebustracer/internal/capture"
	"github.com/eebustracer/eebustracer/internal/model"
	"github.com/eebustracer/eebustracer/internal/parser"
	"github.com/eebustracer/eebustracer/internal/store"
)

var captureCmd = &cobra.Command{
	Use:   "capture",
	Short: "Connect to an EEBus stack and capture SHIP traffic",
	RunE:  runCapture,
}

var (
	captureTarget  string
	captureOutput  string
	captureLogFile string
	captureTCP     string
)

func init() {
	captureCmd.Flags().StringVar(&captureTarget, "target", "", "EEBus stack address to connect to (host:port, e.g. 192.168.1.100:4712)")
	captureCmd.Flags().StringVarP(&captureOutput, "output", "o", "", "output file path (.eet)")
	captureCmd.Flags().StringVar(&captureLogFile, "log-file", "", "tail an eebus-go log file instead of connecting via UDP")
	captureCmd.Flags().StringVar(&captureTCP, "tcp", "", "connect to a TCP log server (host:port, e.g. 192.168.20.41:54546)")
	rootCmd.AddCommand(captureCmd)
}

func runCapture(cmd *cobra.Command, args []string) error {
	sources := 0
	if captureTarget != "" {
		sources++
	}
	if captureLogFile != "" {
		sources++
	}
	if captureTCP != "" {
		sources++
	}
	if sources == 0 {
		return fmt.Errorf("one of --target, --log-file, or --tcp is required")
	}
	if sources > 1 {
		return fmt.Errorf("--target, --log-file, and --tcp are mutually exclusive")
	}

	logger := newLogger()

	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	traceRepo := store.NewTraceRepo(db)
	msgRepo := store.NewMessageRepo(db)
	p := parser.New()
	engine := capture.NewEngine(p, msgRepo, logger)

	trace := &model.Trace{
		Name:      fmt.Sprintf("Capture %s", time.Now().Format("2006-01-02 15:04:05")),
		StartedAt: time.Now(),
		CreatedAt: time.Now(),
	}
	if err := traceRepo.CreateTrace(trace); err != nil {
		return fmt.Errorf("create trace: %w", err)
	}

	if captureLogFile != "" {
		src := capture.NewLogTailSource(captureLogFile, p, logger)
		if err := engine.StartWithSource(trace.ID, src, captureLogFile); err != nil {
			return fmt.Errorf("start log tail: %w", err)
		}
		fmt.Printf("Tailing %s (trace ID: %d)\n", captureLogFile, trace.ID)
		fmt.Println("Watching for new log lines... Press Ctrl+C to stop.")
	} else if captureTCP != "" {
		src := capture.NewTCPSource(captureTCP, p, logger)
		if err := engine.StartWithSource(trace.ID, src, captureTCP); err != nil {
			return fmt.Errorf("start TCP capture: %w", err)
		}
		fmt.Printf("Connected to TCP %s (trace ID: %d)\n", captureTCP, trace.ID)
		fmt.Println("Receiving log lines... Press Ctrl+C to stop.")
	} else {
		if err := engine.Start(trace.ID, captureTarget); err != nil {
			return fmt.Errorf("start capture: %w", err)
		}
		fmt.Printf("Connected to %s (trace ID: %d)\n", captureTarget, trace.ID)
		fmt.Println("Receiving SHIP frames... Press Ctrl+C to stop.")
	}

	// Wait for interrupt
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Println("\nStopping capture...")

	stats := engine.Stats()
	if err := engine.Stop(); err != nil {
		return fmt.Errorf("stop capture: %w", err)
	}

	stopTime := time.Now()
	if err := traceRepo.StopTrace(trace.ID, stopTime, int(stats.PacketsParsed)); err != nil {
		logger.Error("failed to update trace", "error", err)
	}

	fmt.Printf("Captured %d packets (%d bytes)\n", stats.PacketsReceived, stats.BytesReceived)

	// Export if output file specified
	if captureOutput != "" {
		messages, err := msgRepo.ListMessages(trace.ID, store.MessageFilter{Limit: 100000})
		if err != nil {
			return fmt.Errorf("list messages for export: %w", err)
		}

		trace.StoppedAt = &stopTime
		trace.MessageCount = len(messages)

		f, err := os.Create(captureOutput)
		if err != nil {
			return fmt.Errorf("create output file: %w", err)
		}
		defer f.Close()

		if err := store.ExportTrace(f, trace, messages); err != nil {
			return fmt.Errorf("export trace: %w", err)
		}
		fmt.Printf("Exported to %s\n", captureOutput)
	}

	return nil
}
