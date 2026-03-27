package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteJSON(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		value      interface{}
		wantStatus int
		wantBody   string
	}{
		{
			name:       "200 with map",
			status:     http.StatusOK,
			value:      map[string]string{"key": "val"},
			wantStatus: 200,
			wantBody:   `{"key":"val"}`,
		},
		{
			name:       "201 with struct",
			status:     http.StatusCreated,
			value:      struct{ ID int `json:"id"` }{42},
			wantStatus: 201,
			wantBody:   `{"id":42}`,
		},
		{
			name:       "empty slice",
			status:     http.StatusOK,
			value:      []string{},
			wantStatus: 200,
			wantBody:   `[]`,
		},
		{
			name:       "nil becomes null",
			status:     http.StatusOK,
			value:      nil,
			wantStatus: 200,
			wantBody:   `null`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			writeJSON(w, tt.status, tt.value)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			ct := w.Header().Get("Content-Type")
			if ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			// json.Encoder appends a newline
			got := w.Body.String()
			got = got[:len(got)-1] // trim trailing newline
			if got != tt.wantBody {
				t.Errorf("body = %q, want %q", got, tt.wantBody)
			}
		})
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusBadRequest, "something went wrong")

	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["error"] != "something went wrong" {
		t.Errorf("error = %q, want %q", body["error"], "something went wrong")
	}
}

func TestParseID(t *testing.T) {
	tests := []struct {
		name    string
		pathVal string
		want    int64
		wantErr bool
	}{
		{"valid", "42", 42, false},
		{"zero", "0", 0, false},
		{"large", "9999999999", 9999999999, false},
		{"negative", "-1", -1, false},
		{"empty", "", 0, true},
		{"non-numeric", "abc", 0, true},
		{"float", "1.5", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/api/traces/"+tt.pathVal, http.NoBody)
			r.SetPathValue("id", tt.pathVal)

			got, err := parseID(r, "id")
			if (err != nil) != tt.wantErr {
				t.Errorf("parseID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseID() = %d, want %d", got, tt.want)
			}
		})
	}
}
