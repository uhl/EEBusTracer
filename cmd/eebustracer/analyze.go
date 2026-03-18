package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/eebustracer/eebustracer/internal/analysis"
	"github.com/eebustracer/eebustracer/internal/model"
	"github.com/eebustracer/eebustracer/internal/store"
)

var analyzeCmd = &cobra.Command{
	Use:   "analyze <file>",
	Short: "Run protocol analysis on a trace file",
	Long: `Run protocol analysis checks on a .eet or .log trace file.

Checks:
  usecases       Detect active use cases per device
  subscriptions  Track subscription and binding lifecycle
  metrics        Heartbeat accuracy metrics (jitter statistics)
  all            Run all checks`,
	Args: cobra.ExactArgs(1),
	RunE: runAnalyze,
}

var (
	analyzeCheck  string
	analyzeOutput string
)

func init() {
	analyzeCmd.Flags().StringVar(&analyzeCheck, "check", "all", "check to run: usecases|subscriptions|metrics|all")
	analyzeCmd.Flags().StringVar(&analyzeOutput, "output", "text", "output format: text|json")
	rootCmd.AddCommand(analyzeCmd)
}

func runAnalyze(cmd *cobra.Command, args []string) error {
	filePath := args[0]

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	var trace *model.Trace
	var messages []*model.Message

	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".log":
		name := strings.TrimSuffix(filepath.Base(filePath), ext)
		trace, messages, err = store.ImportLogFileAutoDetect(f, name)
	default:
		trace, messages, err = store.ImportTrace(f)
	}
	if err != nil {
		return fmt.Errorf("parse trace file: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Analyzing trace %q (%d messages)\n\n", trace.Name, len(messages))

	checks := strings.Split(analyzeCheck, ",")
	runAll := contains(checks, "all")

	type result struct {
		name string
		data interface{}
	}
	var results []result

	if runAll || contains(checks, "usecases") {
		usecases := analysis.DetectUseCases(messages)
		results = append(results, result{"usecases", usecases})
	}

	if runAll || contains(checks, "subscriptions") {
		subs := analysis.TrackSubscriptionsAndBindings(messages, 5*time.Minute)
		results = append(results, result{"subscriptions", subs})
	}

	if runAll || contains(checks, "metrics") {
		metrics := analysis.ComputeHeartbeatMetrics(messages)
		results = append(results, result{"metrics", metrics})
	}

	if analyzeOutput == "json" {
		output := map[string]interface{}{}
		for _, r := range results {
			output[r.name] = r.data
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	}

	// Text output
	for _, r := range results {
		printTextResult(r.name, r.data)
	}
	return nil
}

func printTextResult(name string, data interface{}) {
	fmt.Printf("=== %s ===\n\n", strings.ToUpper(name))

	switch v := data.(type) {
	case []analysis.DeviceUseCases:
		if len(v) == 0 {
			fmt.Println("  No use cases detected.")
		}
		for _, duc := range v {
			fmt.Printf("  Device: %s (Actor: %s)\n", duc.DeviceAddr, duc.Actor)
			for _, uc := range duc.UseCases {
				status := "available"
				if !uc.Available {
					status = "unavailable"
				}
				fmt.Printf("    [%s] %s (%s)\n", uc.Abbreviation, uc.UseCaseName, status)
			}
			fmt.Println()
		}

	case analysis.SubscriptionBindingResult:
		fmt.Printf("  Subscriptions: %d | Bindings: %d\n\n", len(v.Subscriptions), len(v.Bindings))
		for _, sub := range v.Subscriptions {
			status := "active"
			if !sub.Active {
				status = "removed"
			}
			if sub.Stale {
				status = "STALE"
			}
			fmt.Printf("  [%s] %s:%s -> %s:%s (notifies: %d)\n",
				status, sub.ClientDevice, sub.ClientFeature,
				sub.ServerDevice, sub.ServerFeature, sub.NotifyCount)
		}
		for _, b := range v.Bindings {
			status := "active"
			if !b.Active {
				status = "removed"
			}
			fmt.Printf("  [%s] binding %s:%s -> %s:%s\n",
				status, b.ClientDevice, b.ClientFeature,
				b.ServerDevice, b.ServerFeature)
		}

	case analysis.HeartbeatMetrics:
		fmt.Printf("  Heartbeat jitter entries: %d\n", len(v.HeartbeatJitter))
		for _, j := range v.HeartbeatJitter {
			fmt.Printf("  Heartbeat %s: mean=%.0fms stddev=%.0fms min=%.0fms max=%.0fms (%d samples)\n",
				j.DevicePair, j.MeanMs, j.StdDevMs, j.MinMs, j.MaxMs, j.SampleCount)
		}
	}

	fmt.Println()
}

func contains(list []string, item string) bool {
	for _, v := range list {
		if strings.TrimSpace(v) == item {
			return true
		}
	}
	return false
}
