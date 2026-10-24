package handlers

import (
	"errors"
	"net/http"

	"clawreef/internal/systembackup"
	"github.com/gin-gonic/gin"
)

// Routes are registered on the application's existing authenticated admin
// group. Ordinary Team data backup does not impersonate formal acceptance.
type TeamDataBackupHandler struct{ service *systembackup.TeamDataService }

func NewTeamDataBackupHandler(s *systembackup.TeamDataService) *TeamDataBackupHandler {
	return &TeamDataBackupHandler{service: s}
}
func (h *TeamDataBackupHandler) Register(r *gin.RouterGroup) {
	r.GET("/team-data/overview", h.overview)
	r.GET("/team-data/jobs", h.list)
	r.POST("/team-data/jobs", h.create)
	r.GET("/team-data/jobs/:id", h.detail)
	r.POST("/team-data/jobs/:id/cancel", h.cancel)
}
func (h *TeamDataBackupHandler) available(c *gin.Context) bool {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	if c.GetInt("userID") < 1 {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "code": "authentication_required"})
		return false
	}
	if h.service == nil {
		backupError(c, errors.New("not_ready"))
		return false
	}
	return true
}
func (h *TeamDataBackupHandler) overview(c *gin.Context) {
	if c.GetInt("userID") < 1 {
		c.JSON(401, gin.H{"success": false, "code": "authentication_required"})
		return
	}
	c.Header("Cache-Control", "no-store")
	if h.service == nil {
		c.JSON(200, gin.H{"success": true, "data": gin.H{"configured": false}})
		return
	}
	data, e := h.service.Overview(c.Request.Context())
	if e != nil {
		backupError(c, e)
		return
	}
	c.JSON(200, gin.H{"success": true, "data": data})
}
func (h *TeamDataBackupHandler) list(c *gin.Context) {
	if !h.available(c) {
		return
	}
	items, e := h.service.List(c.Request.Context())
	if e != nil {
		backupError(c, e)
		return
	}
	c.JSON(200, gin.H{"success": true, "data": gin.H{"items": items}})
}
func (h *TeamDataBackupHandler) detail(c *gin.Context) {
	if !h.available(c) {
		return
	}
	j, e := h.service.Get(c.Request.Context(), c.Param("id"))
	if e != nil {
		backupError(c, e)
		return
	}
	c.JSON(200, gin.H{"success": true, "data": j})
}
func (h *TeamDataBackupHandler) create(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var input struct {
		Operation string `json:"operation"`
		SourceID  string `json:"source_backup_id,omitempty"`
	}
	if !backupBody(c, &input) {
		return
	}
	j, e := h.service.Create(c.Request.Context(), c.GetInt("userID"), c.GetHeader("Idempotency-Key"), input.Operation, input.SourceID)
	if e != nil {
		backupError(c, e)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"success": true, "data": j})
}
func (h *TeamDataBackupHandler) cancel(c *gin.Context) {
	if !h.available(c) {
		return
	}
	if e := h.service.Cancel(c.Request.Context(), c.Param("id")); e != nil {
		backupError(c, e)
		return
	}
	j, e := h.service.Get(c.Request.Context(), c.Param("id"))
	if e != nil {
		backupError(c, e)
		return
	}
	c.JSON(200, gin.H{"success": true, "data": j})
}
