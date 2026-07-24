package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/eebustracer/eebustracer/internal/analysis"
	"github.com/eebustracer/eebustracer/internal/model"
	"github.com/eebustracer/eebustracer/internal/spineparse"
	"github.com/eebustracer/eebustracer/internal/store"
)

var devicesCmd = &cobra.Command{
	Use:   "devices <file>",
	Short: "List devices, entities, features, and use cases in a trace",
	Long: `List every device seen in a trace, together with its entity /
feature tree (from NodeManagementDetailedDiscoveryData replies) and
the EEBus use cases it announced. Useful for a quick "what is in this
capture?" summary without loading the web UI.`,
	Args: cobra.ExactArgs(1),
	RunE: runDevices,
}

var devicesOutput string

func init() {
	devicesCmd.Flags().StringVar(&devicesOutput, "output", "text", "output format: text|json")
	rootCmd.AddCommand(devicesCmd)
}

type deviceReport struct {
	DeviceAddr string                       `json:"deviceAddr"`
	Actor      string                       `json:"actor,omitempty"`
	Entities   []spineparse.DiscoveryEntity `json:"entities,omitempty"`
	UseCases   []analysis.UseCaseInfo       `json:"useCases,omitempty"`
	MessageCnt int                          `json:"messageCount"`
}

func runDevices(cmd *cobra.Command, args []string) error {
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

	reports := buildDeviceReports(messages)

	if devicesOutput == "json" {
		out := map[string]interface{}{
			"trace":   trace.Name,
			"devices": reports,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	printDeviceReports(trace.Name, len(messages), reports)
	return nil
}

func buildDeviceReports(messages []*model.Message) []deviceReport {
	// Merge entities from all discovery reply/notify messages per device.
	entitiesByAddr := map[string][]spineparse.DiscoveryEntity{}
	msgCountByAddr := map[string]int{}
	deviceOrder := []string{}
	seen := map[string]bool{}

	for _, msg := range messages {
		for _, addr := range []string{msg.DeviceSource, msg.DeviceDest} {
			if addr == "" {
				continue
			}
			if !seen[addr] {
				seen[addr] = true
				deviceOrder = append(deviceOrder, addr)
			}
		}
		if msg.DeviceSource != "" {
			msgCountByAddr[msg.DeviceSource]++
		}
		if msg.FunctionSet == "NodeManagementDetailedDiscoveryData" &&
			(msg.CmdClassifier == "reply" || msg.CmdClassifier == "notify") &&
			msg.DeviceSource != "" && len(msg.SpinePayload) > 0 {
			if ents := spineparse.ParseDiscoveryEntities(msg.SpinePayload); len(ents) > 0 {
				entitiesByAddr[msg.DeviceSource] = mergeDiscoveryEntities(entitiesByAddr[msg.DeviceSource], ents)
			}
		}
	}

	// Group use cases by device address.
	ucByAddr := map[string][]analysis.UseCaseInfo{}
	actorByAddr := map[string]string{}
	for _, duc := range analysis.DetectUseCases(messages) {
		ucByAddr[duc.DeviceAddr] = append(ucByAddr[duc.DeviceAddr], duc.UseCases...)
		if actorByAddr[duc.DeviceAddr] == "" {
			actorByAddr[duc.DeviceAddr] = duc.Actor
		}
	}

	sort.Strings(deviceOrder)
	reports := make([]deviceReport, 0, len(deviceOrder))
	for _, addr := range deviceOrder {
		reports = append(reports, deviceReport{
			DeviceAddr: addr,
			Actor:      actorByAddr[addr],
			Entities:   entitiesByAddr[addr],
			UseCases:   ucByAddr[addr],
			MessageCnt: msgCountByAddr[addr],
		})
	}
	return reports
}

func mergeDiscoveryEntities(existing, incoming []spineparse.DiscoveryEntity) []spineparse.DiscoveryEntity {
	idx := map[string]int{}
	for i, e := range existing {
		idx[e.Address] = i
	}
	for _, ent := range incoming {
		if pos, ok := idx[ent.Address]; ok {
			// Merge features that aren't already present.
			featSeen := map[string]bool{}
			for _, f := range existing[pos].Features {
				featSeen[f.Address] = true
			}
			for _, f := range ent.Features {
				if !featSeen[f.Address] {
					existing[pos].Features = append(existing[pos].Features, f)
					featSeen[f.Address] = true
				}
			}
			if existing[pos].EntityType == "" && ent.EntityType != "" {
				existing[pos].EntityType = ent.EntityType
			}
		} else {
			existing = append(existing, ent)
			idx[ent.Address] = len(existing) - 1
		}
	}
	return existing
}

func printDeviceReports(traceName string, msgCount int, reports []deviceReport) {
	fmt.Fprintf(os.Stderr, "Trace: %s (%d messages, %d devices)\n\n", traceName, msgCount, len(reports))
	if len(reports) == 0 {
		fmt.Println("No devices observed.")
		return
	}
	for _, r := range reports {
		fmt.Printf("Device: %s", r.DeviceAddr)
		if r.Actor != "" {
			fmt.Printf("  (actor: %s)", r.Actor)
		}
		fmt.Printf("  [%d msgs from]\n", r.MessageCnt)

		if len(r.Entities) == 0 {
			fmt.Println("  (no discovery data observed)")
		} else {
			for _, ent := range r.Entities {
				label := ent.EntityType
				if label == "" {
					label = "(no entityType)"
				}
				fmt.Printf("  entity %s  type=%s  %d feature(s)\n", ent.Address, label, len(ent.Features))
				for _, f := range ent.Features {
					role := f.Role
					if role == "" {
						role = "?"
					}
					fmt.Printf("    feat %s  %s [%s]", f.Address, f.FeatureType, role)
					if len(f.Functions) > 0 {
						fmt.Printf("  fns=%d", len(f.Functions))
					}
					fmt.Println()
				}
			}
		}

		if len(r.UseCases) > 0 {
			fmt.Println("  use cases:")
			for _, uc := range r.UseCases {
				status := "available"
				if !uc.Available {
					status = "unavailable"
				}
				fmt.Printf("    [%s] %s (%s)\n", uc.Abbreviation, uc.UseCaseName, status)
			}
		}
		fmt.Println()
	}
}
