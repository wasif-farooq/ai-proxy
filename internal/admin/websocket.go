package admin

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"ai-proxy/internal/logger"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins in dev; restrict in production
	},
}

// Connection represents a single WebSocket client connection.
type Connection struct {
	ID      string
	ClientID string
	Send    chan []byte
}

// Hub maintains a set of active WebSocket connections and broadcasts messages.
type Hub struct {
	mu      sync.RWMutex
	connections map[string]*Connection
}

// NewHub creates a new WebSocket hub.
func NewHub() *Hub {
	return &Hub{
		connections: make(map[string]*Connection),
	}
}

// Register adds a connection to the hub.
func (h *Hub) Register(conn *Connection) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.connections[conn.ID] = conn
	logger.Default().Info("websocket client connected",
		slog.String("id", conn.ID),
		slog.Int("total", len(h.connections)),
	)
}

// Unregister removes a connection from the hub.
func (h *Hub) Unregister(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if conn, ok := h.connections[id]; ok {
		close(conn.Send)
		delete(h.connections, id)
		logger.Default().Info("websocket client disconnected",
			slog.String("id", id),
			slog.Int("total", len(h.connections)),
		)
	}
}

// Broadcast sends a message to all connected clients.
func (h *Hub) Broadcast(msg interface{}) {
	data, err := json.Marshal(msg)
	if err != nil {
		logger.Default().Error("websocket broadcast marshal", slog.String("error", err.Error()))
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, conn := range h.connections {
		select {
		case conn.Send <- data:
		default:
			// Skip slow clients
		}
	}
}

// Stats returns the number of connected clients.
func (h *Hub) Stats() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.connections)
}

// HandleWebSocket handles the WebSocket upgrade and manages the connection lifecycle.
func (h *Hub) HandleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.FromContext(c.Request.Context()).Error("websocket upgrade failed",
			slog.String("error", err.Error()),
		)
		return
	}

	id := fmt.Sprintf("ws-%d", time.Now().UnixNano())
	client := &Connection{
		ID:      id,
		Send:    make(chan []byte, 256),
	}

	h.Register(client)

	// Write pump: sends messages from the Send channel to the WebSocket
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		defer conn.Close()
		defer h.Unregister(id)

		for {
			select {
			case message, ok := <-client.Send:
				if !ok {
					_ = conn.WriteMessage(websocket.CloseMessage, []byte{})
					return
				}
				_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
					return
				}
			case <-ticker.C:
				_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	// Read pump: reads messages from the WebSocket (for future command support)
	conn.SetReadLimit(4096)
	_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}


