package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func newTestHub(t *testing.T) *Hub {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewHub(logger)
}

func TestHub_NewHub(t *testing.T) {
	hub := newTestHub(t)
	if hub == nil {
		t.Fatal("NewHub() returned nil")
	}
	if hub.ClientCount() != 0 {
		t.Errorf("ClientCount() = %d, want 0", hub.ClientCount())
	}
}

func TestHub_RegisterAndClientCount(t *testing.T) {
	hub := newTestHub(t)

	// Set up a WebSocket server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		hub.Register(conn)
	}))
	defer srv.Close()

	// Connect a client
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Give the Register goroutines time to start
	time.Sleep(50 * time.Millisecond)

	if hub.ClientCount() != 1 {
		t.Errorf("ClientCount() = %d, want 1", hub.ClientCount())
	}
}

func TestHub_BroadcastEvent(t *testing.T) {
	hub := newTestHub(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		hub.Register(conn)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)

	// Broadcast a typed event
	hub.BroadcastEvent("test_event", map[string]string{"key": "value"})

	// Read the message from the client
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var event wsEvent
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if event.Type != "test_event" {
		t.Errorf("event.Type = %q, want %q", event.Type, "test_event")
	}
}

func TestHub_BroadcastToMultipleClients(t *testing.T) {
	hub := newTestHub(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		hub.Register(conn)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	const numClients = 3
	conns := make([]*websocket.Conn, numClients)
	for i := 0; i < numClients; i++ {
		c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("dial client %d: %v", i, err)
		}
		conns[i] = c
	}
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()

	time.Sleep(50 * time.Millisecond)

	if hub.ClientCount() != numClients {
		t.Errorf("ClientCount() = %d, want %d", hub.ClientCount(), numClients)
	}

	hub.BroadcastEvent("ping", "hello")

	var wg sync.WaitGroup
	for i, c := range conns {
		wg.Add(1)
		go func(idx int, conn *websocket.Conn) {
			defer wg.Done()
			conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			_, data, err := conn.ReadMessage()
			if err != nil {
				t.Errorf("client %d read: %v", idx, err)
				return
			}
			var event wsEvent
			if err := json.Unmarshal(data, &event); err != nil {
				t.Errorf("client %d unmarshal: %v", idx, err)
				return
			}
			if event.Type != "ping" {
				t.Errorf("client %d event.Type = %q, want %q", idx, event.Type, "ping")
			}
		}(i, c)
	}
	wg.Wait()
}

func TestHub_UnregisterOnClose(t *testing.T) {
	hub := newTestHub(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		hub.Register(conn)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if hub.ClientCount() != 1 {
		t.Errorf("ClientCount() = %d, want 1 before close", hub.ClientCount())
	}

	conn.Close()

	// Wait for readPump to detect the close and unregister
	deadline := time.Now().Add(2 * time.Second)
	for hub.ClientCount() != 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}

	if hub.ClientCount() != 0 {
		t.Errorf("ClientCount() = %d, want 0 after close", hub.ClientCount())
	}
}

func TestHub_BroadcastNoClients(t *testing.T) {
	hub := newTestHub(t)

	// Should not panic with no clients
	hub.BroadcastEvent("test", "data")

	if hub.ClientCount() != 0 {
		t.Errorf("ClientCount() = %d, want 0", hub.ClientCount())
	}
}
