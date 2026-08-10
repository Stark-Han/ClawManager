package handlers

import (
	"net/http"

	"clawreef/internal/services"

	"github.com/gin-gonic/gin"
)

// WebSocketHandler handles WebSocket connections
type WebSocketHandler struct {
	hub *services.Hub
}

// NewWebSocketHandler creates a new WebSocket handler
func NewWebSocketHandler(hub *services.Hub) *WebSocketHandler {
	return &WebSocketHandler{
		hub: hub,
	}
}

// HandleWebSocket handles WebSocket upgrade requests
func (h *WebSocketHandler) HandleWebSocket(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	roleRaw, _ := c.Get("userRole")
	role, _ := roleRaw.(string)
	topic := services.WebSocketTopicUser
	if c.Query("topic") == string(services.WebSocketTopicRuntimeAdmin) {
		if role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
			return
		}
		topic = services.WebSocketTopicRuntimeAdmin
	}

	// Upgrade HTTP connection to WebSocket
	services.ServeWSWithOptions(h.hub, c.Writer, c.Request, userID.(int), role, topic)
}

// GetConnectionCount returns the number of active WebSocket connections
func (h *WebSocketHandler) GetConnectionCount(c *gin.Context) {
	count := h.hub.GetClientCount()
	c.JSON(http.StatusOK, gin.H{
		"count": count,
	})
}
