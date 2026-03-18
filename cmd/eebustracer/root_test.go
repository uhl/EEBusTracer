package main

import (
	"bytes"
	"testing"
)

func TestRootCmd_Help(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"--help"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("help failed: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("expected help output")
	}
}

func TestRootCmd_UnknownSubcommand(t *testing.T) {
	rootCmd.SetArgs([]string{"nonexistent"})
	err := rootCmd.Execute()
	if err == nil {
		t.Error("expected error for unknown subcommand")
	}
}
