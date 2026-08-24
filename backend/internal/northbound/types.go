package northbound

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"clawreef/internal/models"
	"github.com/gin-gonic/gin"
)

func requireStrongSecret(name, value string) error {
	if len(value) < 32 {
		return fmt.Errorf("%s must contain at least 32 bytes", name)
	}
	return nil
}

const (
	ScopeLiteCreate      = "lite-instances:create"
	ScopeLiteRead        = "lite-instances:read"
	ScopeProCreate       = "pro-instances:create"
	ScopeProRead         = "pro-instances:read"
	ScopeShareLinkManage = "lite-instances:share-link:manage"
	ScopeShareLinkReset  = "lite-instances:share-link:reset"

	OperationTypeLiteInstance = "lite_instance"
	OperationTypeProInstance  = "pro_instance"
)

type APIError struct {
	Status     int
	Code       string
	Message    string
	Cause      error
	AuditEvent string
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause != nil {
		return e.Code + ": " + e.Cause.Error()
	}
	return e.Code + ": " + e.Message
}

func apiError(status int, code, message string, cause error) *APIError {
	return &APIError{Status: status, Code: code, Message: message, Cause: cause}
}

func writeError(c *gin.Context, err error) {
	var target *APIError
	if !errors.As(err, &target) {
		target = apiError(http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "A required dependency is temporarily unavailable", err)
	}
	c.Set("northboundErrorCode", target.Code)
	if target.AuditEvent != "" {
		c.Set("northboundAuditEvent", target.AuditEvent)
	}
	requestID, _ := c.Get("requestID")
	c.JSON(target.Status, gin.H{
		"code":       target.Code,
		"message":    target.Message,
		"request_id": requestID,
	})
}

type TokenResponse struct {
	UserID           int      `json:"-"`
	AccessToken      string   `json:"access_token"`
	RefreshToken     string   `json:"refresh_token"`
	TokenType        string   `json:"token_type"`
	ExpiresIn        int64    `json:"expires_in"`
	RefreshExpiresIn int64    `json:"refresh_expires_in"`
	Scopes           []string `json:"scopes"`
	SessionID        string   `json:"session_id"`
}

type Principal struct {
	UserID    int
	SessionID string
	Scopes    []string
}

func (p Principal) HasScope(required string) bool {
	for _, scope := range p.Scopes {
		if scope == required {
			return true
		}
	}
	return false
}

type CreateLiteInstanceRequest struct {
	Name        string  `json:"name" binding:"required,min=3,max=50"`
	Owner       string  `json:"owner" binding:"required,min=1,max=128"`
	Type        string  `json:"type" binding:"required,oneof=openclaw hermes opencode deepseek-harness workbuddy"`
	Description *string `json:"description,omitempty"`
}

// CreateProInstanceRequest intentionally mirrors the existing Lite request.
// The server owns the Linux runtime variant and resource preset so callers
// cannot accidentally request the Windows VM or oversized resources.
type CreateProInstanceRequest struct {
	Name        string  `json:"name" binding:"required,min=3,max=50"`
	Owner       string  `json:"owner" binding:"required,min=1,max=128"`
	Type        string  `json:"type" binding:"required,oneof=workbuddy"`
	Description *string `json:"description,omitempty"`
}

type EnableShareLinkPasswordRequest struct {
	ExpiresMode     string     `json:"expires_mode,omitempty" binding:"omitempty,oneof=preset custom permanent"`
	ExpiresPreset   string     `json:"expires_preset,omitempty" binding:"omitempty,oneof=1h 24h 7d 30d"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	WorkspaceAccess string     `json:"workspace_access,omitempty" binding:"omitempty,oneof=none read write"`
}

type OperationResponse struct {
	OperationID  string     `json:"operation_id"`
	Status       string     `json:"status"`
	ResourceType string     `json:"resource_type"`
	InstanceID   *int       `json:"instance_id,omitempty"`
	ErrorCode    *string    `json:"error_code,omitempty"`
	ErrorMessage *string    `json:"error_message,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func operationResponse(item *models.NorthboundOperation) OperationResponse {
	return OperationResponse{
		OperationID:  item.OperationID,
		Status:       item.Status,
		ResourceType: item.OperationType,
		InstanceID:   item.InstanceID,
		ErrorCode:    item.ErrorCode,
		ErrorMessage: item.ErrorMessage,
		CreatedAt:    item.CreatedAt,
		StartedAt:    item.StartedAt,
		FinishedAt:   item.FinishedAt,
		UpdatedAt:    item.UpdatedAt,
	}
}

type LiteInstanceResponse struct {
	ID           int        `json:"id"`
	Name         string     `json:"name"`
	Owner        string     `json:"owner"`
	Description  *string    `json:"description,omitempty"`
	Type         string     `json:"type"`
	Status       string     `json:"status"`
	Availability string     `json:"availability,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
}

type ShareLinkResetResponse struct {
	InstanceID      int        `json:"instance_id"`
	AuthMode        string     `json:"auth_mode"`
	ShareURL        string     `json:"share_url"`
	Password        string     `json:"password,omitempty"`
	WorkspaceAccess string     `json:"workspace_access"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func liteInstanceResponse(item *models.Instance) LiteInstanceResponse {
	availability := "unavailable"
	switch item.Status {
	case "running":
		availability = "available"
	case "creating":
		availability = "starting"
	}
	return LiteInstanceResponse{
		ID:           item.ID,
		Name:         item.Name,
		Owner:        instanceOwner(item),
		Description:  item.Description,
		Type:         item.Type,
		Status:       item.Status,
		Availability: availability,
		CreatedAt:    item.CreatedAt,
		UpdatedAt:    item.UpdatedAt,
		StartedAt:    item.StartedAt,
	}
}

func instanceOwner(item *models.Instance) string {
	if item == nil || item.Owner == nil {
		return ""
	}
	return *item.Owner
}
