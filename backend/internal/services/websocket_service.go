package services

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"clawreef/internal/models"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// Client represents a WebSocket client
type Client struct {
	ID     int
	UserID int
	Role   string
	Topic  WebSocketTopic
	Conn   *websocket.Conn
	Send   chan []byte
	hub    *Hub
}

type WebSocketTopic string

const (
	WebSocketTopicUser         WebSocketTopic = "user"
	WebSocketTopicRuntimeAdmin WebSocketTopic = "runtime_admin"
)

// Hub maintains the set of active clients and broadcasts messages
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan *Message
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
	stop       chan struct{}
}

// Message represents a WebSocket message
type Message struct {
	Type       string      `json:"type"`
	UserID     int         `json:"user_id,omitempty"`
	InstanceID int         `json:"instance_id,omitempty"`
	Data       interface{} `json:"data"`
	Timestamp  time.Time   `json:"timestamp"`
}

// InstanceStatusUpdate represents an instance status update
type InstanceStatusUpdate struct {
	InstanceID int    `json:"instance_id"`
	Status     string `json:"status"`
	PodName    string `json:"pod_name,omitempty"`
	PodIP      string `json:"pod_ip,omitempty"`
	UpdatedAt  string `json:"updated_at"`
}

var (
	hub     *Hub
	hubOnce sync.Once
)

// GetHub returns the global hub instance
func GetHub() *Hub {
	hubOnce.Do(func() {
		hub = NewHub()
		go hub.Run()
	})
	return hub
}

// NewHub creates a new Hub instance
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan *Message),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		stop:       make(chan struct{}),
	}
}

// Run starts the hub's main loop
func (h *Hub) Run() {
	for {
		select {
		case <-h.stop:
			h.mu.Lock()
			for client := range h.clients {
				close(client.Send)
				delete(h.clients, client)
			}
			h.mu.Unlock()
			log.Println("WebSocket hub stopped")
			return

		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Printf("WebSocket client registered: user=%d", client.UserID)

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.Send)
			}
			h.mu.Unlock()
			log.Printf("WebSocket client unregistered: user=%d", client.UserID)

		case message := <-h.broadcast:
			h.mu.RLock()
			clients := make([]*Client, 0, len(h.clients))
			for client := range h.clients {
				// Filter by user ID if specified
				if message.UserID == 0 || client.UserID == message.UserID {
					clients = append(clients, client)
				}
			}
			h.mu.RUnlock()

			for _, client := range clients {
				select {
				case client.Send <- mustEncode(message):
				default:
					// Client's send channel is full, close it
					h.mu.Lock()
					close(client.Send)
					delete(h.clients, client)
					h.mu.Unlock()
				}
			}
		}
	}
}

// mustEncode encodes a message to JSON, panics on error
func mustEncode(msg *Message) []byte {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Failed to encode message: %v", err)
		return []byte(`{"type":"error","data":"encoding error"}`)
	}
	return data
}

// BroadcastInstanceStatus broadcasts instance status update to relevant clients
func (h *Hub) BroadcastInstanceStatus(userID int, instance *models.Instance) {
	update := InstanceStatusUpdate{
		InstanceID: instance.ID,
		Status:     instance.Status,
		UpdatedAt:  instance.UpdatedAt.Format(time.RFC3339),
	}

	if instance.PodName != nil {
		update.PodName = *instance.PodName
	}
	if instance.PodIP != nil {
		update.PodIP = *instance.PodIP
	}

	msg := &Message{
		Type:      "instance_status",
		UserID:    userID,
		Data:      update,
		Timestamp: time.Now(),
	}
	h.broadcast <- msg
}

// BroadcastToAll broadcasts a message to all connected clients
func (h *Hub) BroadcastToAll(msgType string, data interface{}) {
	msg := &Message{
		Type:      msgType,
		Data:      data,
		Timestamp: time.Now(),
	}
	h.broadcast <- msg
}

func (h *Hub) BroadcastRuntimeAdmin(msgType string, data interface{}) {
	msg := &Message{
		Type:      msgType,
		Data:      data,
		Timestamp: time.Now(),
	}
	h.broadcastRuntimeAdminMessage(msg)
}

func (h *Hub) broadcastRuntimeAdminMessage(msg *Message) {
	h.mu.RLock()
	clients := make([]*Client, 0, len(h.clients))
	for client := range h.clients {
		if client.Role == "admin" && client.Topic == WebSocketTopicRuntimeAdmin {
			clients = append(clients, client)
		}
	}
	h.mu.RUnlock()

	encoded := mustEncode(msg)
	for _, client := range clients {
		select {
		case client.Send <- encoded:
		default:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				close(client.Send)
				delete(h.clients, client)
			}
			h.mu.Unlock()
		}
	}
}

// ServeWS handles WebSocket connections
func ServeWS(hub *Hub, w http.ResponseWriter, r *http.Request, userID int) {
	ServeWSWithOptions(hub, w, r, userID, "", WebSocketTopicUser)
}

func ServeWSWithOptions(hub *Hub, w http.ResponseWriter, r *http.Request, userID int, role string, topic WebSocketTopic) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade WebSocket: %v", err)
		return
	}
	if topic == "" {
		topic = WebSocketTopicUser
	}

	client := &Client{
		UserID: userID,
		Role:   strings.TrimSpace(role),
		Topic:  topic,
		Conn:   conn,
		Send:   make(chan []byte, 256),
		hub:    hub,
	}

	client.hub.register <- client

	// Start goroutines for reading and writing
	go client.writePump()
	go client.readPump()
}

func StartRuntimeAdminEventBridge(ctx context.Context, events RuntimeEventService, hub *Hub) {
	if events == nil || hub == nil {
		return
	}
	go func() {
		lastID := "$"
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			messages, err := events.Read(ctx, lastID, 5*time.Second)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("runtime admin event bridge read failed: %v", err)
				sleepOrDone(ctx, time.Second)
				continue
			}
			if len(messages) == 0 {
				sleepOrDone(ctx, 200*time.Millisecond)
				continue
			}
			for _, message := range messages {
				if strings.TrimSpace(message.ID) != "" {
					lastID = message.ID
				}
				if msg, ok := runtimeEventWebSocketMessage(message); ok {
					hub.broadcastRuntimeAdminMessage(msg)
				}
			}
		}
	}()
}

func runtimeEventWebSocketMessage(message redisStreamMessage) (*Message, bool) {
	eventType := strings.TrimSpace(message.Fields["type"])
	if eventType == "" {
		var event RuntimeEvent
		if raw := strings.TrimSpace(message.Fields["event"]); raw != "" && json.Unmarshal([]byte(raw), &event) == nil {
			eventType = strings.TrimSpace(event.Type)
			if eventType == "" {
				return nil, false
			}
			return &Message{
				Type:      eventType,
				Data:      json.RawMessage(event.Payload),
				Timestamp: event.CreatedAt,
			}, true
		}
		return nil, false
	}

	createdAt := time.Now().UTC()
	if rawCreatedAt := strings.TrimSpace(message.Fields["created_at"]); rawCreatedAt != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, rawCreatedAt); err == nil {
			createdAt = parsed
		}
	}
	var data interface{} = map[string]any{}
	if rawPayload := strings.TrimSpace(message.Fields["payload"]); rawPayload != "" {
		if json.Valid([]byte(rawPayload)) {
			data = json.RawMessage(rawPayload)
		} else {
			data = rawPayload
		}
	}
	return &Message{
		Type:      eventType,
		Data:      data,
		Timestamp: createdAt,
	}, true
}

func sleepOrDone(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

// readPump pumps messages from the websocket connection to the hub
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.Conn.Close()
	}()

	c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, _, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}
		// Process incoming messages if needed
	}
}

// writePump pumps messages from the hub to the websocket connection
func (c *Client) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Add queued messages to the current websocket message
			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.Send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// GetClientCount returns the number of connected clients
func (h *Hub) GetClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// Stop gracefully shuts down the hub, closing all client connections.
func (h *Hub) Stop() {
	close(h.stop)
}
