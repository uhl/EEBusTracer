package main

import (
	"testing"
)

func TestImportCmd_MissingFile(t *testing.T) {
	rootCmd.SetArgs([]string{"import", "/nonexistent/file.eet"})
	err := rootCmd.Execute()
	if err == nil {
		t.Error("expected error for missing file")
	}
}
