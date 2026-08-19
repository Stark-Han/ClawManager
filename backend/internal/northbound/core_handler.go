package northbound

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type CoreHandler struct {
	service        *CoreService
	internalSecret string
}

func NewCoreHandler(service *CoreService, internalSecret string) *CoreHandler {
	return &CoreHandler{service: service, internalSecret: internalSecret}
}

func (h *CoreHandler) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := strings.TrimSpace(c.GetHeader("Authorization"))
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			writeError(c, apiError(401, "AUTH_INVALID", "Invalid internal identity", nil))
			c.Abort()
			return
		}
		claims, err := parseInternalToken(h.internalSecret, strings.TrimSpace(parts[1]))
		if err != nil {
			writeError(c, apiError(401, "AUTH_INVALID", "Invalid internal identity", err))
			c.Abort()
			return
		}
		principal := &Principal{UserID: claims.UserID, SessionID: claims.SessionID, Scopes: claims.Scopes}
		if err := h.service.ValidatePrincipal(*principal); err != nil {
			writeError(c, err)
			c.Abort()
			return
		}
		c.Set("northboundPrincipal", principal)
		if strings.TrimSpace(claims.RequestID) != "" {
			c.Set("requestID", claims.RequestID)
		}
		c.Next()
	}
}

func (h *CoreHandler) SubmitCreate(c *gin.Context) {
	principal := currentPrincipal(c)
	if principal == nil || !principal.HasScope(ScopeLiteCreate) {
		writeError(c, apiError(403, "SCOPE_DENIED", "Insufficient permission", nil))
		return
	}
	var req CreateLiteInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, apiError(422, "VALIDATION_ERROR", "Invalid Lite instance request", err))
		return
	}
	item, replayed, err := h.service.SubmitCreate(*principal, c.GetHeader("Idempotency-Key"), req)
	if err != nil {
		writeError(c, err)
		return
	}
	if replayed {
		c.Header("Idempotent-Replayed", "true")
	}
	c.Header("Location", "/api/northbound/v1/operations/"+item.OperationID)
	c.JSON(http.StatusAccepted, operationResponse(item))
}

func (h *CoreHandler) GetOperation(c *gin.Context) {
	principal := currentPrincipal(c)
	item, err := h.service.GetOperation(principal.UserID, c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, operationResponse(item))
}

func (h *CoreHandler) GetInstance(c *gin.Context) {
	principal := currentPrincipal(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		writeError(c, apiError(404, "INSTANCE_NOT_FOUND", "Instance not found", nil))
		return
	}
	item, err := h.service.GetInstance(principal.UserID, id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, liteInstanceResponse(item))
}

func (h *CoreHandler) ListInstances(c *gin.Context) {
	principal := currentPrincipal(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	} else if limit > 100 {
		limit = 100
	}
	owner := strings.TrimSpace(c.Query("owner"))
	items, total, err := h.service.ListInstances(principal.UserID, owner, page, limit)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"instances": items, "owner": owner, "total": total, "page": page, "limit": limit})
}

func (h *CoreHandler) EnableShareLinkPassword(c *gin.Context) {
	principal := currentPrincipal(c)
	if principal == nil || !principal.HasScope(ScopeShareLinkManage) {
		writeError(c, apiError(403, "SCOPE_DENIED", "Insufficient permission", nil))
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		writeError(c, apiError(404, "INSTANCE_NOT_FOUND", "Instance not found", nil))
		return
	}
	var req EnableShareLinkPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, apiError(422, "VALIDATION_ERROR", "Invalid ShareLink password request", err))
		return
	}
	result, err := h.service.EnableShareLinkPassword(c.Request.Context(), *principal, id, req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, result)
}

func (h *CoreHandler) ResetShareLinkURL(c *gin.Context) {
	principal := currentPrincipal(c)
	if principal == nil || !principal.HasScope(ScopeShareLinkReset) {
		writeError(c, apiError(403, "SCOPE_DENIED", "Insufficient permission", nil))
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		writeError(c, apiError(404, "INSTANCE_NOT_FOUND", "Instance not found", nil))
		return
	}
	result, err := h.service.ResetShareLinkURL(c.Request.Context(), *principal, id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *CoreHandler) ResetShareLinkPassword(c *gin.Context) {
	principal := currentPrincipal(c)
	if principal == nil || !principal.HasScope(ScopeShareLinkReset) {
		writeError(c, apiError(403, "SCOPE_DENIED", "Insufficient permission", nil))
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		writeError(c, apiError(404, "INSTANCE_NOT_FOUND", "Instance not found", nil))
		return
	}
	result, err := h.service.ResetShareLinkPassword(c.Request.Context(), *principal, id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func RegisterCoreRoutes(router *gin.Engine, handler *CoreHandler) {
	group := router.Group("/internal/northbound/v1")
	group.Use(handler.Middleware())
	group.POST("/lite-instances", handler.SubmitCreate)
	group.GET("/lite-instances", handler.ListInstances)
	group.GET("/lite-instances/:id", handler.GetInstance)
	group.POST("/lite-instances/:id/external-access/password", handler.EnableShareLinkPassword)
	group.POST("/lite-instances/:id/external-access/share-link/reset", handler.ResetShareLinkURL)
	group.POST("/lite-instances/:id/external-access/password/reset", handler.ResetShareLinkPassword)
	group.GET("/operations/:id", handler.GetOperation)
}
