package main

import (
	"testing"
)

func TestServeCmd_Flags(t *testing.T) {
	f := serveCmd.Flags()
	if f.Lookup("port") == nil {
		t.Error("expected --port flag")
	}
}
