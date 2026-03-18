package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAnalyzeCommand_InvalidFile(t *testing.T) {
	analyzeCheck = "all"
	analyzeOutput = "text"

	err := runAnalyze(nil, []string{"/nonexistent/file.eet"})
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestAnalyzeCommand_ValidFile(t *testing.T) {
	// Create a temporary .eet file
	eebt := map[string]interface{}{
		"version": "1.0",
		"trace": map[string]interface{}{
			"name":      "test-trace",
			"startedAt": "2024-01-01T12:00:00Z",
		},
		"messages": []map[string]interface{}{
			{
				"sequenceNum": 1,
				"timestamp":   "2024-01-01T12:00:01Z",
				"direction":   "incoming",
				"rawHex":      "00",
				"shipMsgType": "init",
				"sourceAddr":  "A",
				"destAddr":    "B",
			},
			{
				"sequenceNum":   2,
				"timestamp":     "2024-01-01T12:00:02Z",
				"direction":     "incoming",
				"rawHex":        "00",
				"shipMsgType":   "data",
				"sourceAddr":    "A",
				"destAddr":      "B",
				"msgCounter":    "1",
				"cmdClassifier": "read",
				"deviceSource":  "devA",
				"deviceDest":    "devB",
			},
			{
				"sequenceNum":   3,
				"timestamp":     "2024-01-01T12:00:02Z",
				"direction":     "outgoing",
				"rawHex":        "00",
				"shipMsgType":   "data",
				"sourceAddr":    "B",
				"destAddr":      "A",
				"msgCounter":    "1",
				"msgCounterRef": "1",
				"cmdClassifier": "reply",
				"deviceSource":  "devB",
				"deviceDest":    "devA",
			},
		},
	}

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.eet")
	f, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	json.NewEncoder(f).Encode(eebt)
	f.Close()

	analyzeCheck = "all"
	analyzeOutput = "text"

	err = runAnalyze(nil, []string{filePath})
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
}

func TestAnalyzeCommand_JSONOutput(t *testing.T) {
	eebt := map[string]interface{}{
		"version": "1.0",
		"trace": map[string]interface{}{
			"name":      "json-test",
			"startedAt": "2024-01-01T12:00:00Z",
		},
		"messages": []map[string]interface{}{},
	}

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.eet")
	f, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	json.NewEncoder(f).Encode(eebt)
	f.Close()

	// Redirect stdout to capture JSON output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	analyzeCheck = "metrics"
	analyzeOutput = "json"

	err = runAnalyze(nil, []string{filePath})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(r).Decode(&result); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if _, ok := result["metrics"]; !ok {
		t.Error("expected metrics key in JSON output")
	}
}
