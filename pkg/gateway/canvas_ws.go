package gateway

import (
	"sync"

	"github.com/anyclaw/anyclaw/pkg/canvas"
	"github.com/gorilla/websocket"
)

type CanvasHub struct {
	mu        sync.RWMutex
	clients   map[*websocket.Conn]bool
	broadcast chan *canvas.CanvasEntry
}

func NewCanvasHub() *CanvasHub {
	return &CanvasHub{
		clients:   make(map[*websocket.Conn]bool),
		broadcast: make(chan *canvas.CanvasEntry, 64),
	}
}

func (h *CanvasHub) Register(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[conn] = true
}

func (h *CanvasHub) Unregister(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[conn] {
		delete(h.clients, conn)
		conn.Close()
	}
}

func (h *CanvasHub) Broadcast(entry *canvas.CanvasEntry) {
	select {
	case h.broadcast <- entry:
	default:
	}
}

func (h *CanvasHub) Run() {
	for entry := range h.broadcast {
		h.mu.RLock()
		clients := make([]*websocket.Conn, 0, len(h.clients))
		for conn := range h.clients {
			clients = append(clients, conn)
		}
		h.mu.RUnlock()

		for _, conn := range clients {
			if err := conn.WriteJSON(map[string]any{
				"type":       "canvas_update",
				"entry_id":   entry.ID,
				"name":       entry.Name,
				"entry_type": entry.Type,
				"version":    entry.Version,
				"updated":    entry.UpdatedAt,
			}); err != nil {
				h.Unregister(conn)
			}
		}
	}
}
