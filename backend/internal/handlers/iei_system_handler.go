package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"clawreef/internal/config"
	"clawreef/internal/models"
	"clawreef/internal/services"
	"clawreef/internal/utils"

	"github.com/gin-gonic/gin"
)

const (
	ieiSystemSessionCookie        = "clawmanager_iei_session"
	ieiSystemSessionBindingPrefix = "iei:"
)

type IEISystemHandler struct {
	cfg             config.IEISystemConfig
	sso             *services.IEISSOService
	instances       services.InstanceService
	instanceHandler *InstanceHandler
	workspace       *WorkspaceFileHandler
}

type ieiSessionExchangeRequest struct {
	Token string `json:"token" binding:"required,max=4096"`
}

type ieiInstanceView struct {
	ID             int        `json:"id"`
	Owner          string     `json:"owner"`
	Name           string     `json:"name"`
	Description    *string    `json:"description,omitempty"`
	Type           string     `json:"type"`
	RuntimeType    string     `json:"runtime_type"`
	RuntimeVariant string     `json:"runtime_variant,omitempty"`
	InstanceMode   string     `json:"instance_mode"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
}

func NewIEISystemHandler(
	cfg config.IEISystemConfig,
	sso *services.IEISSOService,
	instances services.InstanceService,
	instanceHandler *InstanceHandler,
) *IEISystemHandler {
	return &IEISystemHandler{cfg: cfg, sso: sso, instances: instances, instanceHandler: instanceHandler}
}

func (h *IEISystemHandler) SetWorkspaceFileHandler(workspace *WorkspaceFileHandler) {
	h.workspace = workspace
}

func (h *IEISystemHandler) ExchangeSession(c *gin.Context) {
	h.noStore(c)
	if h.sso == nil || !h.sso.Enabled() {
		utils.Error(c, http.StatusServiceUnavailable, "IEI system SSO is not configured")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8<<10)
	var request ieiSessionExchangeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		utils.Error(c, http.StatusBadRequest, "token is required")
		return
	}
	session, err := h.sso.ExchangeExternalToken(request.Token)
	if err != nil {
		h.clearSessionCookie(c)
		utils.Error(c, http.StatusUnauthorized, "IEI system token is invalid or expired")
		return
	}
	h.setSessionCookie(c, session.Token, session.ExpiresAt)
	utils.Success(c, http.StatusOK, "IEI system session established", gin.H{
		"owner":      session.Email,
		"expires_at": session.ExpiresAt,
	})
}

func (h *IEISystemHandler) GetSession(c *gin.Context) {
	h.noStore(c)
	session, ok := h.requireSession(c)
	if !ok {
		return
	}
	utils.Success(c, http.StatusOK, "IEI system session is valid", gin.H{
		"owner":      session.Email,
		"expires_at": session.ExpiresAt,
	})
}

func (h *IEISystemHandler) DeleteSession(c *gin.Context) {
	h.noStore(c)
	h.clearSessionCookie(c)
	utils.Success(c, http.StatusOK, "IEI system session cleared", nil)
}

func (h *IEISystemHandler) ListInstances(c *gin.Context) {
	h.noStore(c)
	session, ok := h.requireSession(c)
	if !ok {
		return
	}
	page := positiveQueryInt(c, "page", 1)
	limit := positiveQueryInt(c, "limit", 100)
	if limit > 100 {
		limit = 100
	}
	ownerService, ok := h.instances.(services.IEISystemInstanceService)
	if !ok {
		utils.Error(c, http.StatusServiceUnavailable, "IEI system instance lookup is unavailable")
		return
	}
	instances, total, err := ownerService.GetSupportedByOwnerEmail(session.Email, (page-1)*limit, limit)
	if err != nil {
		utils.HandleError(c, err)
		return
	}
	views := make([]ieiInstanceView, 0, len(instances))
	for idx := range instances {
		if h.ownedSupportedInstance(&instances[idx], session.Email) {
			views = append(views, newIEIInstanceView(&instances[idx]))
		}
	}
	utils.Success(c, http.StatusOK, "IEI instances retrieved", gin.H{
		"instances": views,
		"total":     total,
		"page":      page,
		"limit":     limit,
	})
}

func (h *IEISystemHandler) GetInstance(c *gin.Context) {
	h.noStore(c)
	session, ok := h.requireSession(c)
	if !ok {
		return
	}
	instance, ok := h.requireOwnedSupportedInstance(c, session.Email)
	if !ok {
		return
	}
	utils.Success(c, http.StatusOK, "IEI instance retrieved", gin.H{
		"instance": newIEIInstanceView(instance),
	})
}

// RestartInstance restarts an IEI-owned instance through the existing runtime
// lifecycle service. The dedicated IEI session and exact owner match are
// required; the normal user JWT handler must not be reused for this surface.
func (h *IEISystemHandler) RestartInstance(c *gin.Context) {
	h.noStore(c)
	session, ok := h.requireSession(c)
	if !ok {
		return
	}
	instance, ok := h.requireOwnedSupportedInstance(c, session.Email)
	if !ok {
		return
	}

	status := strings.ToLower(strings.TrimSpace(instance.Status))
	if status != "running" {
		utils.Error(c, http.StatusConflict, "Instance is not running")
		return
	}
	if err := h.instances.Restart(instance.ID); err != nil {
		// Runtime lifecycle errors are operational failures. Do not expose
		// Kubernetes resource names or internal endpoints on this public portal.
		utils.Error(c, http.StatusServiceUnavailable, "Unable to restart instance")
		return
	}

	utils.Success(c, http.StatusAccepted, "Instance restart submitted", gin.H{
		"instance_id": instance.ID,
		"status":      "restarting",
	})
}

func (h *IEISystemHandler) GenerateInstanceAccess(c *gin.Context) {
	h.noStore(c)
	session, ok := h.requireSession(c)
	if !ok {
		return
	}
	instance, ok := h.requireOwnedSupportedInstance(c, session.Email)
	if !ok {
		return
	}
	if instance.Status != "running" {
		utils.Error(c, http.StatusConflict, "Instance is not running")
		return
	}
	if strings.EqualFold(strings.TrimSpace(instance.RuntimeType), "shell") {
		utils.Error(c, http.StatusBadRequest, "Desktop access is not available for shell runtime instances")
		return
	}
	if h.instanceHandler == nil || h.instanceHandler.accessService == nil || h.instanceHandler.proxyService == nil {
		utils.Error(c, http.StatusServiceUnavailable, "Instance access is unavailable")
		return
	}

	accessURL := h.instanceHandler.proxyService.GetProxyURLForInstance(instance, "")
	if strings.TrimSpace(accessURL) == "" {
		utils.Error(c, http.StatusServiceUnavailable, "Unable to generate access URL")
		return
	}
	targetPort := h.instanceHandler.proxyService.GetTargetPortForInstance(instance)
	// IEI-bound tokens always use the control-plane proxy. Same-origin requests
	// revalidate the HttpOnly IEI session; dedicated OpenCode/DSH origins use the
	// signed instance-scoped handoff capability because browsers cannot send the
	// management-origin IEI cookie there. The handoff cannot outlive the IEI
	// session expiry used below.
	upstream := ""
	directProxyEnabled := false
	duration := time.Until(session.ExpiresAt)
	if duration > time.Hour {
		duration = time.Hour
	}
	if duration <= time.Second {
		h.clearSessionCookie(c)
		utils.Error(c, http.StatusUnauthorized, "IEI system session is invalid or expired")
		return
	}
	accessToken, err := h.instanceHandler.accessService.GenerateBoundToken(
		instance.UserID,
		instance.ID,
		instance.Type,
		accessURL,
		upstream,
		targetPort,
		duration,
		ieiSystemSessionBinding(session.SessionID),
	)
	if err != nil {
		utils.HandleError(c, err)
		return
	}
	proxyURL := h.instanceHandler.proxyService.GetProxyURLForInstance(instance, accessToken.Token)
	browserURL := browserAccessEntryURL(accessURL, proxyURL)
	workspaceAvailable := isDesktopWorkspaceInstance(instance) ||
		(instance.WorkspacePath != nil && strings.TrimSpace(*instance.WorkspacePath) != "")
	workspaceRoot := "Workspace"
	if isDesktopWorkspaceInstance(instance) {
		workspaceRoot = "/config"
	}
	h.setInstanceAccessCookie(c, instance.ID, accessToken.Token, accessToken.ExpiresAt)
	utils.Success(c, http.StatusOK, "IEI instance access granted", gin.H{
		"access_url":               browserURL,
		"expires_at":               accessToken.ExpiresAt,
		"desktop_proxy_mode":       desktopProxyMode(directProxyEnabled, upstream),
		"desktop_upstream_present": upstream != "",
		"workspace_available":      workspaceAvailable,
		"workspace_root":           workspaceRoot,
	})
}

func (h *IEISystemHandler) ListWorkspace(c *gin.Context) {
	h.handleWorkspace(c, (*WorkspaceFileHandler).List)
}

func (h *IEISystemHandler) PreviewWorkspace(c *gin.Context) {
	h.handleWorkspace(c, (*WorkspaceFileHandler).Preview)
}

func (h *IEISystemHandler) DownloadWorkspace(c *gin.Context) {
	h.handleWorkspace(c, (*WorkspaceFileHandler).Download)
}

func (h *IEISystemHandler) UploadWorkspace(c *gin.Context) {
	h.handleWorkspace(c, (*WorkspaceFileHandler).Upload)
}

func (h *IEISystemHandler) CreateWorkspaceFolder(c *gin.Context) {
	h.handleWorkspace(c, (*WorkspaceFileHandler).Mkdir)
}

func (h *IEISystemHandler) RenameWorkspaceEntry(c *gin.Context) {
	h.handleWorkspace(c, (*WorkspaceFileHandler).Rename)
}

func (h *IEISystemHandler) DeleteWorkspaceEntry(c *gin.Context) {
	h.handleWorkspace(c, (*WorkspaceFileHandler).Delete)
}

// handleWorkspace authenticates the dedicated IEI session and checks the
// instance owner before delegating to the existing workspace implementation.
// It deliberately does not create or validate a ShareLink session.
func (h *IEISystemHandler) handleWorkspace(c *gin.Context, next func(*WorkspaceFileHandler, *gin.Context)) {
	h.noStore(c)
	session, ok := h.requireSession(c)
	if !ok {
		return
	}
	instance, ok := h.requireOwnedSupportedInstance(c, session.Email)
	if !ok {
		return
	}
	if h.workspace == nil {
		utils.Error(c, http.StatusServiceUnavailable, "IEI workspace access is unavailable")
		return
	}

	// Pass only the instance that was authorized by the dedicated IEI owner
	// check. WorkspaceFileHandler uses this server-side context instead of the
	// normal JWT or ShareLink authentication paths.
	c.Set(ieiWorkspaceContextKey, instance)
	next(h.workspace, c)
}

func (h *IEISystemHandler) requireSession(c *gin.Context) (*services.IEISession, bool) {
	if h.sso == nil || !h.sso.Enabled() {
		utils.Error(c, http.StatusServiceUnavailable, "IEI system SSO is not configured")
		return nil, false
	}
	rawToken, err := c.Cookie(ieiSystemSessionCookie)
	if err != nil {
		utils.Error(c, http.StatusUnauthorized, "IEI system session is required")
		return nil, false
	}
	session, err := h.sso.ValidateSession(rawToken)
	if err != nil {
		h.clearSessionCookie(c)
		utils.Error(c, http.StatusUnauthorized, "IEI system session is invalid or expired")
		return nil, false
	}
	return session, true
}

func (h *IEISystemHandler) requireOwnedSupportedInstance(c *gin.Context, owner string) (*models.Instance, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		utils.Error(c, http.StatusBadRequest, "Invalid instance ID")
		return nil, false
	}
	instance, err := h.instances.GetByID(id)
	if err != nil {
		utils.HandleError(c, err)
		return nil, false
	}
	// Deliberately collapse missing, unsupported and wrong-owner cases so this
	// public endpoint cannot be used to enumerate other owners' instances.
	if !h.ownedSupportedInstance(instance, owner) {
		utils.Error(c, http.StatusNotFound, "Instance not found")
		return nil, false
	}
	return instance, true
}

func (h *IEISystemHandler) ownedSupportedInstance(instance *models.Instance, owner string) bool {
	if instance == nil || instance.Owner == nil || !strings.EqualFold(strings.TrimSpace(*instance.Owner), strings.TrimSpace(owner)) {
		return false
	}
	mode, ok := services.NormalizeInstanceMode(instance.InstanceMode)
	if !ok {
		return false
	}
	typeName := strings.ToLower(strings.TrimSpace(instance.Type))
	if mode == services.InstanceModeLite {
		switch typeName {
		case services.RuntimeTypeOpenClaw, services.RuntimeTypeHermes, "opencode", "deepseek-harness":
			return true
		default:
			return false
		}
	}
	if mode != services.InstanceModePro {
		return false
	}
	switch typeName {
	case services.RuntimeTypeOpenClaw, services.RuntimeTypeHermes, services.RuntimeTypeOpenCode:
		return true
	case "workbuddy":
		return strings.EqualFold(strings.TrimSpace(instance.RuntimeVariant), services.WorkbuddyRuntimeLinux)
	default:
		return false
	}
}

func newIEIInstanceView(instance *models.Instance) ieiInstanceView {
	owner := ""
	if instance.Owner != nil {
		owner = *instance.Owner
	}
	return ieiInstanceView{
		ID:             instance.ID,
		Owner:          owner,
		Name:           instance.Name,
		Description:    instance.Description,
		Type:           instance.Type,
		RuntimeType:    instance.RuntimeType,
		RuntimeVariant: instance.RuntimeVariant,
		InstanceMode:   instance.InstanceMode,
		Status:         instance.Status,
		CreatedAt:      instance.CreatedAt,
		UpdatedAt:      instance.UpdatedAt,
		StartedAt:      instance.StartedAt,
	}
}

func positiveQueryInt(c *gin.Context, name string, fallback int) int {
	value, err := strconv.Atoi(c.DefaultQuery(name, strconv.Itoa(fallback)))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func (h *IEISystemHandler) setSessionCookie(c *gin.Context, token string, expiresAt time.Time) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     ieiSystemSessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   max(1, int(time.Until(expiresAt).Seconds())),
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   h.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *IEISystemHandler) clearSessionCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     ieiSystemSessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
		HttpOnly: true,
		Secure:   h.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *IEISystemHandler) setInstanceAccessCookie(c *gin.Context, instanceID int, token string, expiresAt time.Time) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     fmt.Sprintf("instance_access_%d", instanceID),
		Value:    token,
		Path:     fmt.Sprintf("/api/v1/instances/%d/proxy", instanceID),
		MaxAge:   max(1, int(time.Until(expiresAt).Seconds())),
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   h.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *IEISystemHandler) noStore(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.Header("Referrer-Policy", "no-referrer")
}

func ieiSystemSessionBinding(sessionID string) string {
	digest := sha256.Sum256([]byte("clawmanager-ieisystem:" + strings.TrimSpace(sessionID)))
	return ieiSystemSessionBindingPrefix + hex.EncodeToString(digest[:])
}
