package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/eebustracer/eebustracer/internal/mdns"
)

var discoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Discover EEBus devices on the local network via mDNS",
	RunE:  runDiscover,
}

var (
	discoverTimeout time.Duration
	discoverJSON    bool
)

func init() {
	discoverCmd.Flags().DurationVar(&discoverTimeout, "timeout", 10*time.Second, "discovery duration")
	discoverCmd.Flags().BoolVar(&discoverJSON, "json", false, "output as JSON")
	rootCmd.AddCommand(discoverCmd)
}

func runDiscover(cmd *cobra.Command, args []string) error {
	logger := newLogger()
	monitor := mdns.NewMonitor(logger)

	ctx, cancel := context.WithTimeout(context.Background(), discoverTimeout)
	defer cancel()

	if err := monitor.Start(ctx); err != nil {
		return fmt.Errorf("start mDNS discovery: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Discovering EEBus devices for %s...\n", discoverTimeout)

	<-ctx.Done()
	monitor.Stop()

	devices := monitor.Devices()

	if discoverJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(devices)
	}

	if len(devices) == 0 {
		fmt.Println("No EEBus devices found.")
		return nil
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tADDRESS\tPORT\tBRAND\tMODEL\tTYPE\tSKI")
	for _, d := range devices {
		addrs := strings.Join(d.Addresses, ",")
		fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\t%s\t%s\n",
			d.InstanceName, addrs, d.Port,
			d.Brand, d.Model, d.DeviceType, d.SKI,
		)
	}
	tw.Flush()

	fmt.Fprintf(os.Stderr, "\nFound %d device(s)\n", len(devices))
	return nil
}
