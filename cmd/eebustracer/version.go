package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version is set by the build process via -ldflags.
var Version = "0.4.0"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("eebustracer %s\n", Version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
