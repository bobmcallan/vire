package jobmanager

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// JobWSHub manages WebSocket clients and broadcasts job events.
type JobWSHub struct {
	clients    map[*JobWSClient]bool
	broadcast  chan models.JobEvent
	register   chan *JobWSClient
	unregister chan *JobWSClient
	done       chan struct{}
	mu         sync.RWMutex
	logger     *common.Logger
}

// JobWSClient represents a connected WebSocket client.
type JobWSClient struct {
	hub  *JobWSHub
	conn *websocket.Conn
	send chan []byte
}

// NewJobWSHub creates a new WebSocket hub.
func NewJobWSHub(logger *common.Logger) *JobWSHub {
	return &JobWSHub{
		clients:    make(map[*JobWSClient]bool),
		broadcast:  make(chan models.JobEvent, 256),
		register:   make(chan *JobWSClient),
		unregister: make(chan *JobWSClient),
		done:       make(chan struct{}),
		logger:     logger,
	}
}

// Run starts the hub's main event loop. Should be called as a goroutine.
func (h *JobWSHub) Run() {
	for {
		select {
		case <-h.done:
			return

		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			h.logger.Debug().Int("clients", len(h.clients)).Msg("WebSocket client connected")

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			h.logger.Debug().Int("clients", len(h.clients)).Msg("WebSocket client disconnected")

		case event := <-h.broadcast:
			data, err := json.Marshal(event)
			if err != nil {
				h.logger.Warn().Err(err).Msg("Failed to marshal job event")
				continue
			}

			h.mu.RLock()
			var slow []*JobWSClient
			for client := range h.clients {
				select {
				case client.send <- data:
				default:
					slow = append(slow, client)
				}
			}
			h.mu.RUnlock()

			if len(slow) > 0 {
				h.mu.Lock()
				for _, c := range slow {
					delete(h.clients, c)
					close(c.send)
				}
				h.mu.Unlock()
			}
		}
	}
}

// Stop signals the hub's event loop to exit.
func (h *JobWSHub) Stop() {
	select {
	case <-h.done:
		// Already stopped
	default:
		close(h.done)
	}
}

// Broadcast sends a job event to all connected clients.
func (h *JobWSHub) Broadcast(event models.JobEvent) {
	select {
	case h.broadcast <- event:
	default:
		h.logger.Warn().Msg("WebSocket broadcast channel full, dropping event")
	}
}

// ServeWS upgrades an HTTP connection to WebSocket and registers the client.
func (h *JobWSHub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Warn().Err(err).Msg("WebSocket upgrade failed")
		return
	}

	client := &JobWSClient{
		hub:  h,
		conn: conn,
		send: make(chan []byte, 256),
	}

	h.register <- client

	go client.writePump()
	go client.readPump()
}

// ClientCount returns the number of connected clients.
func (h *JobWSHub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// writePump sends messages from the send channel to the WebSocket connection.
func (c *JobWSClient) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// readPump reads messages from the WebSocket connection (mainly to detect close).
func (c *JobWSClient) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			break
		}
	}
}
