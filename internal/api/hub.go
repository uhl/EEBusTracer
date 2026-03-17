package api

import (
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/gorilla/websocket"

	"github.com/eebustracer/eebustracer/internal/model"
)

// Hub manages WebSocket client connections and broadcasts messages.
type Hub struct {
	mu      sync.RWMutex
	clients map[*wsClient]bool
	logger  *slog.Logger
}

type wsClient struct {
	conn *websocket.Conn
	send chan []byte
}

const clientBufferSize = 256

// wsEvent is the typed envelope sent over WebSocket connections.
type wsEvent struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// NewHub creates a new Hub.
func NewHub(logger *slog.Logger) *Hub {
	return &Hub{
		clients: make(map[*wsClient]bool),
		logger:  logger,
	}
}

// Register adds a WebSocket client.
func (h *Hub) Register(conn *websocket.Conn) {
	client := &wsClient{
		conn: conn,
		send: make(chan []byte, clientBufferSize),
	}

	h.mu.Lock()
	h.clients[client] = true
	h.mu.Unlock()

	go h.writePump(client)
	go h.readPump(client)
}

// Broadcast sends a message to all connected clients wrapped in a typed event.
func (h *Hub) Broadcast(msg *model.Message) {
	h.BroadcastEvent("message", msg)
}

// BroadcastEvent sends a typed event to all connected clients.
func (h *Hub) BroadcastEvent(eventType string, payload interface{}) {
	data, err := json.Marshal(wsEvent{
		Type:    eventType,
		Payload: payload,
	})
	if err != nil {
		h.logger.Error("marshal broadcast event", "error", err)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		select {
		case client.send <- data:
		default:
			// Client is slow, drop message
			h.logger.Debug("dropping message for slow client")
		}
	}
}

// ClientCount returns the number of connected clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

func (h *Hub) unregister(client *wsClient) {
	h.mu.Lock()
	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)
		close(client.send)
	}
	h.mu.Unlock()
}

func (h *Hub) writePump(client *wsClient) {
	defer client.conn.Close()

	for msg := range client.send {
		if err := client.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			h.unregister(client)
			return
		}
	}
}

func (h *Hub) readPump(client *wsClient) {
	defer h.unregister(client)
	for {
		if _, _, err := client.conn.ReadMessage(); err != nil {
			return
		}
	}
}
