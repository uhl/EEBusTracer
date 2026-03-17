package parser

import (
	"encoding/json"
	"testing"
)

func TestNormalizeEEBUSJSON(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  string
	}{
		{
			"standard JSON unchanged",
			[]byte(`{"key":"value"}`),
			`{"key":"value"}`,
		},
		{
			"EEBUS array format to object",
			[]byte(`[{"key":"value"},{"other":"data"}]`),
			`{"key":"value","other":"data"}`,
		},
		{
			"strip null bytes",
			append([]byte(`{"key":"value"}`), 0x00, 0x00),
			`{"key":"value"}`,
		},
		{
			"empty input",
			[]byte{},
			``,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(NormalizeEEBUSJSON(tt.input))
			if got != tt.want {
				t.Errorf("NormalizeEEBUSJSON() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFixupSliceFields(t *testing.T) {
	type target struct {
		Items []string `json:"items"`
		Name  string   `json:"name"`
	}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			"single item not in array",
			`{"items":"hello","name":"test"}`,
			`{"items":["hello"],"name":"test"}`,
		},
		{
			"already an array",
			`{"items":["hello"],"name":"test"}`,
			`{"items":["hello"],"name":"test"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fixupSliceFields([]byte(tt.input), target{})
			var gotObj, wantObj map[string]json.RawMessage
			if err := json.Unmarshal(got, &gotObj); err != nil {
				t.Fatalf("unmarshal result: %v", err)
			}
			if err := json.Unmarshal([]byte(tt.want), &wantObj); err != nil {
				t.Fatalf("unmarshal want: %v", err)
			}
			if string(gotObj["items"]) != string(wantObj["items"]) {
				t.Errorf("items = %s, want %s", gotObj["items"], wantObj["items"])
			}
		})
	}
}
