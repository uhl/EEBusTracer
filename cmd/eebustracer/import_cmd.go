package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/eebustracer/eebustracer/internal/store"
)

var importCmd = &cobra.Command{
	Use:   "import <file>",
	Short: "Import a .eet or .log trace file",
	Args:  cobra.ExactArgs(1),
	RunE:  runImport,
}

func init() {
	rootCmd.AddCommand(importCmd)
}

func runImport(cmd *cobra.Command, args []string) error {
	filePath := args[0]

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	name := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	trace, messages, err := store.ImportFileAutoDetect(f, name)
	if err != nil {
		return fmt.Errorf("parse trace file: %w", err)
	}

	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	traceRepo := store.NewTraceRepo(db)
	if err := traceRepo.CreateTrace(trace); err != nil {
		return fmt.Errorf("create trace: %w", err)
	}

	if len(messages) > 0 {
		msgRepo := store.NewMessageRepo(db)
		for _, m := range messages {
			m.TraceID = trace.ID
		}
		if err := msgRepo.InsertMessages(messages); err != nil {
			return fmt.Errorf("insert messages: %w", err)
		}
	}

	fmt.Printf("Imported trace %q (ID: %d, %d messages)\n", trace.Name, trace.ID, len(messages))
	return nil
}
