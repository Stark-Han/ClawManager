package northbound

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"time"

	"clawreef/internal/config"
	"clawreef/internal/models"
	"clawreef/internal/repository"
	"clawreef/internal/services"
)

const (
	northboundWorkbuddyCPUCores = 4
	northboundWorkbuddyMemoryGB = 8
	northboundWorkbuddyDiskGB   = 40
	northboundProCPUCores       = 4
	northboundProMemoryGB       = 8
	northboundProDiskGB         = 50
	lifecycleReadyTimeout       = 5 * time.Minute
	lifecycleReadyPollInterval  = 2 * time.Second
	lifecycleWaitingCode        = "LIFECYCLE_WAITING"
)

type CoreService struct {
	repo            *repository.NorthboundRepository
	users           repository.UserRepository
	instances       coreInstanceService
	externalAccess  shareLinkService
	config          config.NorthboundConfig
	audit           repository.AuditEventRepository
	runtimeSettings RuntimeSettingsProvider
}

type coreInstanceService interface {
	Create(userID int, req services.CreateInstanceRequest) (*models.Instance, error)
	GetByID(id int) (*models.Instance, error)
	GetByUserID(userID int, offset, limit int) ([]models.Instance, int, error)
}

type shareLinkService interface {
	CreatePassword(ctx context.Context, instanceID, createdBy int, expiration services.ExternalAccessExpirationRequest) (*services.PasswordExternalAccessResult, error)
	ResetURL(ctx context.Context, instanceID, createdBy int) (*services.EnableShareLinkResult, error)
	ResetPassword(ctx context.Context, instanceID, createdBy int) (*services.PasswordExternalAccessResult, error)
}

func NewCoreService(repo *repository.NorthboundRepository, users repository.UserRepository, instances services.InstanceService, externalAccess services.InstanceExternalAccessService, cfg config.NorthboundConfig, providers ...RuntimeSettingsProvider) *CoreService {
	service := &CoreService{repo: repo, users: users, instances: instances, externalAccess: externalAccess, config: cfg}
	if len(providers) > 0 {
		service.runtimeSettings = providers[0]
	}
	return service
}

func (s *CoreService) settings() *models.NorthboundAdminSettings {
	if s.runtimeSettings != nil {
		return s.runtimeSettings.Current()
	}
	return defaultRuntimeSettings(s.config)
}

func (s *CoreService) ValidatePrincipal(principal Principal) error {
	if principal.UserID <= 0 || strings.TrimSpace(principal.SessionID) == "" {
		return apiError(401, "AUTH_INVALID", "Invalid internal identity", nil)
	}
	user, err := s.users.GetByID(principal.UserID)
	if err != nil {
		return err
	}
	if user == nil || !user.IsActive {
		return apiError(401, "AUTH_INVALID", "Invalid internal identity", nil)
	}
	session, err := s.repo.GetSessionByID(principal.SessionID)
	if err != nil {
		return err
	}
	if session == nil || session.UserID != principal.UserID || session.Status != "active" ||
		session.RefreshExpiresAt.Before(time.Now().UTC()) || user.UpdatedAt.After(session.CreatedAt) {
		return apiError(401, "AUTH_INVALID", "Invalid internal identity", nil)
	}
	var storedScopes []string
	if err := json.Unmarshal([]byte(session.ScopesJSON), &storedScopes); err != nil {
		return err
	}
	for _, claimed := range principal.Scopes {
		allowed := false
		for _, stored := range storedScopes {
			if claimed == stored {
				allowed = true
				break
			}
		}
		if !allowed {
			return apiError(403, "SCOPE_DENIED", "Invalid internal scope", nil)
		}
	}
	return nil
}

func (s *CoreService) SubmitCreate(principal Principal, idempotencyKey string, req CreateLiteInstanceRequest) (*models.NorthboundOperation, bool, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Type = strings.ToLower(strings.TrimSpace(req.Type))
	owner, ownerErr := services.NormalizeInstanceOwner(req.Owner)
	if ownerErr != nil {
		return nil, false, apiError(422, "VALIDATION_ERROR", ownerErr.Error(), ownerErr)
	}
	req.Owner = owner
	if len(req.Name) < 3 || len(req.Name) > 50 || !isSupportedNorthboundType(req.Type) {
		return nil, false, apiError(422, "VALIDATION_ERROR", "Invalid instance request", nil)
	}
	if !allowedType(s.settings().AllowedLiteTypes, req.Type) {
		return nil, false, apiError(422, "RUNTIME_DISABLED", "This Lite runtime is disabled by policy", nil)
	}
	if req.Description != nil && len(*req.Description) > 2000 {
		return nil, false, apiError(422, "VALIDATION_ERROR", "Description is too long", nil)
	}
	// A WorkBuddy request may be a retry of an operation originally submitted
	// through /pro-instances before the collections were unified. Reuse that
	// operation instead of provisioning a duplicate across an upgrade.
	if req.Type == "workbuddy" {
		existing, err := s.findLegacyProOperation(principal.UserID, idempotencyKey, req)
		if err != nil || existing != nil {
			return existing, existing != nil, err
		}
	}
	return s.submitCreateOperation(
		principal,
		idempotencyKey,
		req,
		OperationTypeLiteInstance,
		"Too many unfinished instance operations",
	)
}

func (s *CoreService) findLegacyProOperation(userID int, idempotencyKey string, req CreateLiteInstanceRequest) (*models.NorthboundOperation, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if len(idempotencyKey) < 8 || len(idempotencyKey) > 128 {
		return nil, apiError(400, "INVALID_REQUEST", "Idempotency-Key must contain 8 to 128 characters", nil)
	}
	payload, err := json.Marshal(CreateProInstanceRequest(req))
	if err != nil {
		return nil, err
	}
	existing, err := s.repo.GetOperationByIdempotency(userID, OperationTypeProInstance, sha256Hex(idempotencyKey))
	if err != nil || existing == nil {
		return existing, err
	}
	if existing.RequestHash != sha256Hex(string(payload)) {
		return nil, apiError(409, "IDEMPOTENCY_CONFLICT", "Idempotency-Key was already used for a different request", nil)
	}
	return existing, nil
}

func isSupportedNorthboundType(instanceType string) bool {
	switch strings.ToLower(strings.TrimSpace(instanceType)) {
	case services.RuntimeTypeOpenClaw,
		services.RuntimeTypeHermes,
		services.RuntimeTypeOpenCode,
		services.RuntimeTypeDeepSeekHarness,
		"workbuddy":
		return true
	default:
		return false
	}
}

func isSupportedNorthboundProType(instanceType string) bool {
	switch strings.ToLower(strings.TrimSpace(instanceType)) {
	case services.RuntimeTypeOpenClaw,
		services.RuntimeTypeHermes,
		services.RuntimeTypeOpenCode,
		services.RuntimeTypeDeepSeekHarness,
		"workbuddy":
		return true
	default:
		return false
	}
}

func (s *CoreService) SubmitProCreate(principal Principal, idempotencyKey string, req CreateProInstanceRequest) (*models.NorthboundOperation, bool, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Type = strings.ToLower(strings.TrimSpace(req.Type))
	owner, ownerErr := services.NormalizeInstanceOwner(req.Owner)
	if ownerErr != nil {
		return nil, false, apiError(422, "VALIDATION_ERROR", ownerErr.Error(), ownerErr)
	}
	req.Owner = owner
	if len(req.Name) < 3 || len(req.Name) > 50 || !isSupportedNorthboundProType(req.Type) {
		return nil, false, apiError(422, "VALIDATION_ERROR", "Invalid Pro instance request", nil)
	}
	if !allowedType(s.settings().AllowedProTypes, req.Type) {
		return nil, false, apiError(422, "RUNTIME_DISABLED", "This Pro runtime is disabled by policy", nil)
	}
	if req.Description != nil && len(*req.Description) > 2000 {
		return nil, false, apiError(422, "VALIDATION_ERROR", "Description is too long", nil)
	}
	// WorkBuddy remains in the canonical unified/Lite operation domain so old
	// and new clients cannot create duplicates by switching collection paths.
	if req.Type == "workbuddy" {
		return s.SubmitCreate(principal, idempotencyKey, CreateLiteInstanceRequest(req))
	}
	if _, ok := services.RuntimeImageForBackend(req.Type, services.RuntimeBackendDesktop); !ok {
		return nil, false, apiError(422, "PRO_IMAGE_NOT_CONFIGURED", "An enabled Pro image is not configured for this runtime", nil)
	}
	return s.submitCreateOperation(
		principal,
		idempotencyKey,
		req,
		OperationTypeProInstance,
		"Too many unfinished Pro instance operations",
	)
}

func (s *CoreService) SubmitLifecycle(principal Principal, idempotencyKey string, instanceID int, mode, action string) (*models.NorthboundOperation, bool, error) {
	if instanceID <= 0 {
		return nil, false, apiError(404, "INSTANCE_NOT_FOUND", "Instance not found", nil)
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	action = strings.ToLower(strings.TrimSpace(action))
	var (
		instance      *models.Instance
		operationType string
		err           error
	)
	switch mode {
	case services.InstanceModeLite:
		instance, err = s.GetInstance(principal.UserID, instanceID)
		if err == nil && !strings.EqualFold(strings.TrimSpace(instance.InstanceMode), services.InstanceModeLite) && !isWorkbuddyLinuxPro(instance) {
			err = apiError(404, "INSTANCE_NOT_FOUND", "Instance not found", nil)
		}
		if action == "restart" {
			operationType = OperationTypeLiteRestart
		} else if action == "reset" {
			operationType = OperationTypeLiteReset
		}
	case services.InstanceModePro:
		instance, err = s.GetProInstance(principal.UserID, instanceID)
		if action == "restart" {
			operationType = OperationTypeProRestart
		} else if action == "reset" {
			operationType = OperationTypeProReset
		}
		// WorkBuddy remains in the canonical Lite compatibility domain, matching
		// its create behavior and preventing duplicate lifecycle operations when
		// callers switch between the old and new collection paths.
		if err == nil && isWorkbuddyLinuxPro(instance) {
			if action == "restart" {
				operationType = OperationTypeLiteRestart
			} else if action == "reset" {
				operationType = OperationTypeLiteReset
			}
		}
	default:
		return nil, false, apiError(422, "VALIDATION_ERROR", "Invalid instance mode", nil)
	}
	if err != nil {
		return nil, false, err
	}
	if operationType == "" {
		return nil, false, apiError(422, "VALIDATION_ERROR", "Invalid lifecycle action", nil)
	}
	active, err := s.repo.GetActiveLifecycleOperation(principal.UserID, instanceID)
	if err != nil {
		return nil, false, err
	}
	if active != nil {
		if active.OperationType == operationType {
			return active, true, nil
		}
		return nil, false, apiError(409, "LIFECYCLE_IN_PROGRESS", "Another lifecycle operation is already in progress", nil)
	}
	status := strings.ToLower(strings.TrimSpace(instance.Status))
	if action == "restart" && status != "running" {
		return nil, false, apiError(409, "INVALID_INSTANCE_STATE", "Instance is not running", nil)
	}
	if action == "reset" && status != "running" && status != "stopped" && status != "error" {
		return nil, false, apiError(409, "INVALID_INSTANCE_STATE", "Instance cannot be reset while a lifecycle operation is in progress", nil)
	}
	return s.submitCreateOperation(principal, idempotencyKey, InstanceLifecycleRequest{InstanceID: instanceID}, operationType, "Too many unfinished instance operations")
}

func (s *CoreService) submitCreateOperation(
	principal Principal,
	idempotencyKey string,
	request any,
	operationType string,
	pendingMessage string,
) (*models.NorthboundOperation, bool, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if len(idempotencyKey) < 8 || len(idempotencyKey) > 128 {
		return nil, false, apiError(400, "INVALID_REQUEST", "Idempotency-Key must contain 8 to 128 characters", nil)
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, false, err
	}
	requestHash := sha256Hex(string(payload))
	keyHash := sha256Hex(idempotencyKey)
	existing, err := s.repo.GetOperationByIdempotency(principal.UserID, operationType, keyHash)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		if existing.RequestHash != requestHash {
			return nil, false, apiError(409, "IDEMPOTENCY_CONFLICT", "Idempotency-Key was already used for a different request", nil)
		}
		return existing, true, nil
	}
	pending, err := s.repo.CountPendingOperationsByUser(principal.UserID)
	if err != nil {
		return nil, false, err
	}
	if pending >= s.settings().MaxPendingOperations {
		return nil, false, apiError(429, "RATE_LIMITED", pendingMessage, nil)
	}
	operationID, err := randomToken("op_", 18)
	if err != nil {
		return nil, false, err
	}
	now := time.Now().UTC()
	item := &models.NorthboundOperation{
		OperationID:        operationID,
		UserID:             principal.UserID,
		SessionID:          principal.SessionID,
		OperationType:      operationType,
		IdempotencyKeyHash: keyHash,
		RequestHash:        requestHash,
		RequestPayload:     string(payload),
		Status:             "queued",
		AvailableAt:        now,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if lifecycleRequest, ok := request.(InstanceLifecycleRequest); ok && lifecycleRequest.InstanceID > 0 {
		instanceID := lifecycleRequest.InstanceID
		item.InstanceID = &instanceID
	}
	if err := s.repo.CreateOperation(item); err != nil {
		existing, getErr := s.repo.GetOperationByIdempotency(principal.UserID, operationType, keyHash)
		if getErr == nil && existing != nil {
			if existing.RequestHash != requestHash {
				return nil, false, apiError(409, "IDEMPOTENCY_CONFLICT", "Idempotency-Key was already used for a different request", nil)
			}
			return existing, true, nil
		}
		return nil, false, err
	}
	return item, false, nil
}

func (s *CoreService) GetOperation(userID int, operationID string) (*models.NorthboundOperation, error) {
	item, err := s.repo.GetOperationByID(strings.TrimSpace(operationID))
	if err != nil {
		return nil, err
	}
	if item == nil || item.UserID != userID {
		return nil, apiError(404, "OPERATION_NOT_FOUND", "Operation not found", nil)
	}
	return item, nil
}

func (s *CoreService) GetLatestLifecycleOperation(userID, instanceID int) (*models.NorthboundOperation, error) {
	item, err := s.repo.GetLatestLifecycleOperation(userID, instanceID)
	if err != nil {
		return nil, err
	}
	if item != nil && item.UserID != userID {
		return nil, apiError(404, "OPERATION_NOT_FOUND", "Operation not found", nil)
	}
	return item, nil
}

func (s *CoreService) GetInstance(userID, instanceID int) (*models.Instance, error) {
	item, err := s.instances.GetByID(instanceID)
	if err != nil {
		return nil, err
	}
	if item == nil || item.UserID != userID || !isSupportedNorthboundInstance(item) {
		return nil, apiError(404, "INSTANCE_NOT_FOUND", "Instance not found", nil)
	}
	return item, nil
}

func (s *CoreService) GetProInstance(userID, instanceID int) (*models.Instance, error) {
	item, err := s.instances.GetByID(instanceID)
	if err != nil {
		return nil, err
	}
	if item == nil || item.UserID != userID || !isSupportedNorthboundProInstance(item) {
		return nil, apiError(404, "INSTANCE_NOT_FOUND", "Instance not found", nil)
	}
	return item, nil
}

func (s *CoreService) EnableShareLinkPassword(ctx context.Context, principal Principal, instanceID int, req EnableShareLinkPasswordRequest) (*ShareLinkResetResponse, error) {
	if _, err := s.GetInstance(principal.UserID, instanceID); err != nil {
		return nil, err
	}
	if s.externalAccess == nil {
		return nil, apiError(503, "DEPENDENCY_UNAVAILABLE", "External access service is unavailable", nil)
	}
	expiration, err := normalizeShareLinkPasswordRequest(req)
	if err != nil {
		return nil, err
	}
	result, err := s.externalAccess.CreatePassword(ctx, instanceID, principal.UserID, expiration)
	if err != nil {
		return nil, err
	}
	if result == nil || result.Access == nil || strings.TrimSpace(result.ShareURL) == "" || strings.TrimSpace(result.Password) == "" {
		return nil, apiError(503, "DEPENDENCY_UNAVAILABLE", "External access service returned an invalid response", nil)
	}
	return shareLinkResetResponse(result.Access, result.ShareURL, result.Password), nil
}

func normalizeShareLinkPasswordRequest(req EnableShareLinkPasswordRequest) (services.ExternalAccessExpirationRequest, error) {
	mode := strings.ToLower(strings.TrimSpace(req.ExpiresMode))
	if mode == "" {
		mode = services.ExternalAccessExpirationPreset
	}
	preset := strings.ToLower(strings.TrimSpace(req.ExpiresPreset))
	workspaceAccess, err := services.NormalizeExternalWorkspaceAccess(req.WorkspaceAccess)
	if err != nil {
		return services.ExternalAccessExpirationRequest{}, apiError(422, "VALIDATION_ERROR", "Invalid ShareLink password request", err)
	}
	switch mode {
	case services.ExternalAccessExpirationPreset:
		if req.ExpiresAt != nil {
			return services.ExternalAccessExpirationRequest{}, apiError(422, "VALIDATION_ERROR", "Invalid ShareLink password request", nil)
		}
		if preset == "" {
			preset = services.ExternalAccessPreset24Hours
		}
		switch preset {
		case services.ExternalAccessPreset1Hour, services.ExternalAccessPreset24Hours, services.ExternalAccessPreset7Days, services.ExternalAccessPreset30Days:
		default:
			return services.ExternalAccessExpirationRequest{}, apiError(422, "VALIDATION_ERROR", "Invalid ShareLink password request", nil)
		}
	case services.ExternalAccessExpirationCustom:
		if preset != "" || req.ExpiresAt == nil || !req.ExpiresAt.After(time.Now().UTC()) {
			return services.ExternalAccessExpirationRequest{}, apiError(422, "VALIDATION_ERROR", "Invalid ShareLink password request", nil)
		}
	case services.ExternalAccessExpirationPermanent:
		if preset != "" || req.ExpiresAt != nil {
			return services.ExternalAccessExpirationRequest{}, apiError(422, "VALIDATION_ERROR", "Invalid ShareLink password request", nil)
		}
	default:
		return services.ExternalAccessExpirationRequest{}, apiError(422, "VALIDATION_ERROR", "Invalid ShareLink password request", nil)
	}
	return services.ExternalAccessExpirationRequest{
		Mode:            mode,
		Preset:          preset,
		ExpiresAt:       req.ExpiresAt,
		WorkspaceAccess: workspaceAccess,
	}, nil
}

func (s *CoreService) ResetShareLinkURL(ctx context.Context, principal Principal, instanceID int) (*ShareLinkResetResponse, error) {
	if _, err := s.GetInstance(principal.UserID, instanceID); err != nil {
		return nil, err
	}
	if s.externalAccess == nil {
		return nil, apiError(503, "DEPENDENCY_UNAVAILABLE", "External access service is unavailable", nil)
	}
	result, err := s.externalAccess.ResetURL(ctx, instanceID, principal.UserID)
	if err != nil {
		if errors.Is(err, services.ErrExternalAccessNotEnabled) {
			return nil, apiError(409, "SHARE_LINK_NOT_ENABLED", "Share link is not enabled", nil)
		}
		return nil, err
	}
	if result == nil || result.Access == nil || strings.TrimSpace(result.ShareURL) == "" {
		return nil, apiError(503, "DEPENDENCY_UNAVAILABLE", "External access service returned an invalid response", nil)
	}
	return shareLinkResetResponse(result.Access, result.ShareURL, ""), nil
}

func (s *CoreService) ResetShareLinkPassword(ctx context.Context, principal Principal, instanceID int) (*ShareLinkResetResponse, error) {
	if _, err := s.GetInstance(principal.UserID, instanceID); err != nil {
		return nil, err
	}
	if s.externalAccess == nil {
		return nil, apiError(503, "DEPENDENCY_UNAVAILABLE", "External access service is unavailable", nil)
	}
	result, err := s.externalAccess.ResetPassword(ctx, instanceID, principal.UserID)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrExternalAccessNotEnabled):
			return nil, apiError(409, "SHARE_LINK_NOT_ENABLED", "Share link is not enabled", nil)
		case errors.Is(err, services.ErrExternalAccessPasswordNotEnabled):
			return nil, apiError(409, "SHARE_LINK_PASSWORD_NOT_ENABLED", "Share link password authentication is not enabled", nil)
		default:
			return nil, err
		}
	}
	if result == nil || result.Access == nil || strings.TrimSpace(result.ShareURL) == "" || strings.TrimSpace(result.Password) == "" {
		return nil, apiError(503, "DEPENDENCY_UNAVAILABLE", "External access service returned an invalid response", nil)
	}
	return shareLinkResetResponse(result.Access, result.ShareURL, result.Password), nil
}

func shareLinkResetResponse(access *models.InstanceExternalAccess, shareURL, password string) *ShareLinkResetResponse {
	return &ShareLinkResetResponse{
		InstanceID:      access.InstanceID,
		AuthMode:        access.AuthMode,
		ShareURL:        shareURL,
		Password:        password,
		WorkspaceAccess: access.WorkspaceAccess,
		ExpiresAt:       access.ExpiresAt,
		UpdatedAt:       access.UpdatedAt,
	}
}

func (s *CoreService) ListInstances(userID int, owner string, page, limit int) ([]LiteInstanceResponse, int, error) {
	normalizedOwner, err := services.NormalizeInstanceOwner(owner)
	if err != nil {
		return nil, 0, apiError(400, "INVALID_REQUEST", err.Error(), err)
	}
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	ownerService, ok := s.instances.(services.InstanceOwnerService)
	if !ok {
		return nil, 0, fmt.Errorf("instance service does not support owner filtering")
	}
	items, total, err := ownerService.GetNorthboundByUserIDAndOwner(userID, normalizedOwner, (page-1)*limit, limit)
	if err != nil {
		return nil, 0, err
	}
	filtered := make([]LiteInstanceResponse, 0, len(items))
	for idx := range items {
		filtered = append(filtered, liteInstanceResponse(&items[idx]))
	}
	return filtered, total, nil
}

func (s *CoreService) ListProInstances(userID int, owner string, page, limit int) ([]LiteInstanceResponse, int, error) {
	normalizedOwner, err := services.NormalizeInstanceOwner(owner)
	if err != nil {
		return nil, 0, apiError(400, "INVALID_REQUEST", err.Error(), err)
	}
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	ownerService, ok := s.instances.(services.InstanceOwnerService)
	if !ok {
		return nil, 0, fmt.Errorf("instance service does not support owner filtering")
	}
	items, total, err := ownerService.GetProByUserIDAndOwner(userID, normalizedOwner, (page-1)*limit, limit)
	if err != nil {
		return nil, 0, err
	}
	filtered := make([]LiteInstanceResponse, 0, len(items))
	for idx := range items {
		if isSupportedNorthboundProInstance(&items[idx]) {
			filtered = append(filtered, liteInstanceResponse(&items[idx]))
		}
	}
	return filtered, total, nil
}

func isLite(item *models.Instance) bool {
	if item == nil {
		return false
	}
	if mode, ok := services.NormalizeInstanceMode(item.InstanceMode); ok {
		return mode == services.InstanceModeLite
	}
	return strings.EqualFold(strings.TrimSpace(item.RuntimeType), services.RuntimeBackendGateway)
}

func isWorkbuddyLinuxPro(item *models.Instance) bool {
	if item == nil || !strings.EqualFold(strings.TrimSpace(item.Type), "workbuddy") ||
		!strings.EqualFold(strings.TrimSpace(item.RuntimeVariant), services.WorkbuddyRuntimeLinux) {
		return false
	}
	mode, ok := services.NormalizeInstanceMode(item.InstanceMode)
	return ok && mode == services.InstanceModePro
}

func isSupportedNorthboundProInstance(item *models.Instance) bool {
	if item == nil || !isSupportedNorthboundProType(item.Type) {
		return false
	}
	mode, ok := services.NormalizeInstanceMode(item.InstanceMode)
	if !ok || mode != services.InstanceModePro {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(item.Type), "workbuddy") {
		return strings.EqualFold(strings.TrimSpace(item.RuntimeVariant), services.WorkbuddyRuntimeLinux)
	}
	return true
}

func isSupportedNorthboundInstance(item *models.Instance) bool {
	if item == nil {
		return false
	}
	if isWorkbuddyLinuxPro(item) {
		return true
	}
	if isSupportedNorthboundProInstance(item) {
		return true
	}
	return isLite(item) && isSupportedNorthboundType(item.Type) &&
		!strings.EqualFold(strings.TrimSpace(item.Type), "workbuddy")
}

type OperationWorker struct {
	service *CoreService
	owner   string
	mu      sync.Mutex
	cancel  context.CancelFunc
}

func NewOperationWorker(service *CoreService, owner string) *OperationWorker {
	return &OperationWorker{service: service, owner: strings.TrimSpace(owner)}
}

func (w *OperationWorker) Start(parent context.Context) {
	if w == nil || w.service == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	w.cancel = cancel
	go w.loop(ctx)
}

func (w *OperationWorker) Stop() {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
}

func (w *OperationWorker) loop(ctx context.Context) {
	for {
		tick := time.Duration(w.service.settings().OperationTickMilliseconds) * time.Millisecond
		if tick <= 0 {
			tick = time.Second
		}
		timer := time.NewTimer(tick)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			w.processOne(ctx)
		}
	}
}

func (w *OperationWorker) processOne(ctx context.Context) {
	claimID, err := randomToken(w.owner+"_", 8)
	if err != nil {
		return
	}
	now := time.Now().UTC()
	lease := time.Duration(w.service.settings().OperationLeaseSeconds) * time.Second
	if lease <= 0 {
		lease = 30 * time.Second
	}
	item, err := w.service.repo.ClaimNextOperation(ctx, claimID, now, now.Add(lease))
	if err != nil {
		log.Printf("northbound operation claim failed: %v", err)
		return
	}
	if item == nil {
		return
	}
	if isLifecycleOperation(item.OperationType) {
		w.processLifecycle(ctx, item)
		return
	}
	createRequest, auditPrefix, err := w.service.operationCreateRequest(item)
	if err != nil {
		_ = w.service.repo.MarkOperationFailed(ctx, item.OperationID, "INVALID_REQUEST", "Stored operation payload is invalid", time.Now().UTC())
		return
	}
	instance, createErr := w.service.instances.Create(item.UserID, createRequest)
	if createErr == nil {
		if err := w.service.repo.MarkOperationSucceeded(ctx, item.OperationID, instance.ID, time.Now().UTC()); err != nil {
			log.Printf("northbound operation %s completion failed: %v", item.OperationID, err)
			return
		}
		item.Status = "succeeded"
		item.InstanceID = &instance.ID
		w.service.auditOperation(item, auditPrefix+".succeeded", models.AuditSeverityInfo, auditOperationMessage(item.OperationID, "succeeded"), &instance.ID, "")
		return
	}
	code, message, retryable := classifyCreateError(createErr, createRequest.InstanceMode)
	maxAttempts := w.service.settings().OperationMaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	if !retryable || item.AttemptCount >= maxAttempts {
		_ = w.service.repo.MarkOperationFailed(ctx, item.OperationID, code, message, time.Now().UTC())
		item.Status = "failed"
		w.service.auditOperation(item, auditPrefix+".failed", models.AuditSeverityWarn, auditOperationMessage(item.OperationID, "failed"), nil, code)
		return
	}
	delay := time.Duration(math.Pow(2, float64(item.AttemptCount-1))) * time.Second
	if delay > time.Minute {
		delay = time.Minute
	}
	requeueAt := time.Now().UTC()
	_ = w.service.repo.RequeueOperation(ctx, item.OperationID, code, message, requeueAt.Add(delay), requeueAt)
	item.Status = "queued"
	w.service.auditOperation(item, auditPrefix+".retry", models.AuditSeverityWarn, auditOperationMessage(item.OperationID, "retry scheduled"), nil, code)
}

func isLifecycleOperation(operationType string) bool {
	switch operationType {
	case OperationTypeLiteRestart, OperationTypeLiteReset, OperationTypeProRestart, OperationTypeProReset:
		return true
	default:
		return false
	}
}

func (w *OperationWorker) processLifecycle(ctx context.Context, item *models.NorthboundOperation) {
	var request InstanceLifecycleRequest
	if err := json.Unmarshal([]byte(item.RequestPayload), &request); err != nil || request.InstanceID <= 0 {
		_ = w.service.repo.MarkOperationFailed(ctx, item.OperationID, "INVALID_REQUEST", "Stored operation payload is invalid", time.Now().UTC())
		return
	}
	var current *models.Instance
	var currentErr error
	if item.OperationType == OperationTypeProRestart || item.OperationType == OperationTypeProReset {
		current, currentErr = w.service.GetProInstance(item.UserID, request.InstanceID)
	} else {
		current, currentErr = w.service.GetInstance(item.UserID, request.InstanceID)
	}
	if currentErr != nil || current == nil {
		_ = w.service.repo.MarkOperationFailed(ctx, item.OperationID, "INSTANCE_NOT_FOUND", "Instance not found", time.Now().UTC())
		return
	}
	currentStatus := strings.ToLower(strings.TrimSpace(current.Status))
	if item.ErrorCode != nil && *item.ErrorCode == lifecycleWaitingCode {
		w.finishOrContinueLifecycle(ctx, item, request.InstanceID, currentStatus, lifecycleAuditPrefix(item.OperationType))
		return
	}
	if (item.OperationType == OperationTypeLiteRestart || item.OperationType == OperationTypeProRestart) && currentStatus != "running" {
		_ = w.service.repo.MarkOperationFailed(ctx, item.OperationID, "INVALID_INSTANCE_STATE", "Instance is not running", time.Now().UTC())
		return
	}
	if (item.OperationType == OperationTypeLiteReset || item.OperationType == OperationTypeProReset) && currentStatus != "running" && currentStatus != "stopped" && currentStatus != "error" {
		_ = w.service.repo.MarkOperationFailed(ctx, item.OperationID, "INVALID_INSTANCE_STATE", "Instance lifecycle operation is already in progress", time.Now().UTC())
		return
	}
	var (
		action      func() error
		auditPrefix string
	)
	switch item.OperationType {
	case OperationTypeLiteRestart:
		auditPrefix = "northbound.lite.restart"
		action = func() error { return restartInstance(w.service.instances, request.InstanceID) }
	case OperationTypeProRestart:
		auditPrefix = "northbound.pro.restart"
		action = func() error { return restartInstance(w.service.instances, request.InstanceID) }
	case OperationTypeLiteReset:
		auditPrefix = "northbound.lite.reset"
		action = func() error { return resetInstance(w.service.instances, request.InstanceID) }
	case OperationTypeProReset:
		auditPrefix = "northbound.pro.reset"
		action = func() error { return resetInstance(w.service.instances, request.InstanceID) }
	}
	actionErr := w.runLifecycleActionWithLease(ctx, item, action)
	if actionErr != nil {
		// Lifecycle operations are not retried automatically. A failed reset may
		// have already rebuilt an ephemeral workload; an automatic retry would
		// add risk without improving data safety. The same idempotency key returns
		// this terminal result and a caller must explicitly submit a new request.
		_ = w.service.repo.MarkOperationFailed(ctx, item.OperationID, "LIFECYCLE_FAILED", "Instance lifecycle operation failed; persistent workspace was retained", time.Now().UTC())
		item.Status = "failed"
		w.service.auditOperation(item, auditPrefix+".failed", models.AuditSeverityWarn, auditOperationMessage(item.OperationID, "failed"), &request.InstanceID, "LIFECYCLE_FAILED")
		return
	}
	refreshed, refreshErr := w.service.instances.GetByID(request.InstanceID)
	if refreshErr != nil || refreshed == nil {
		_ = w.service.repo.MarkOperationFailed(ctx, item.OperationID, "INSTANCE_NOT_FOUND", "Instance not found after lifecycle operation", time.Now().UTC())
		return
	}
	w.finishOrContinueLifecycle(ctx, item, request.InstanceID, strings.ToLower(strings.TrimSpace(refreshed.Status)), auditPrefix)
}

func lifecycleAuditPrefix(operationType string) string {
	switch operationType {
	case OperationTypeLiteRestart:
		return "northbound.lite.restart"
	case OperationTypeProRestart:
		return "northbound.pro.restart"
	case OperationTypeLiteReset:
		return "northbound.lite.reset"
	case OperationTypeProReset:
		return "northbound.pro.reset"
	default:
		return "northbound.lifecycle"
	}
}

func (w *OperationWorker) finishOrContinueLifecycle(ctx context.Context, item *models.NorthboundOperation, instanceID int, instanceStatus, auditPrefix string) {
	now := time.Now().UTC()
	if instanceStatus == "running" {
		if err := w.service.repo.MarkOperationSucceeded(ctx, item.OperationID, instanceID, now); err != nil {
			log.Printf("northbound operation %s completion failed: %v", item.OperationID, err)
			return
		}
		item.Status = "succeeded"
		item.InstanceID = &instanceID
		w.service.auditOperation(item, auditPrefix+".succeeded", models.AuditSeverityInfo, auditOperationMessage(item.OperationID, "succeeded"), &instanceID, "")
		return
	}
	if instanceStatus == "error" || instanceStatus == "deleting" {
		_ = w.service.repo.MarkOperationFailed(ctx, item.OperationID, "LIFECYCLE_FAILED", "Instance did not recover; persistent workspace was retained", now)
		item.Status = "failed"
		w.service.auditOperation(item, auditPrefix+".failed", models.AuditSeverityWarn, auditOperationMessage(item.OperationID, "failed"), &instanceID, "LIFECYCLE_FAILED")
		return
	}
	startedAt := item.StartedAt
	if startedAt != nil && now.Sub(*startedAt) >= lifecycleReadyTimeout {
		if recorder, ok := w.service.instances.(services.InstanceLifecycleFailureService); ok {
			_ = recorder.MarkLifecycleFailure(instanceID)
		}
		_ = w.service.repo.MarkOperationFailed(ctx, item.OperationID, "LIFECYCLE_TIMEOUT", "Instance did not become ready in time; persistent workspace was retained", now)
		item.Status = "failed"
		w.service.auditOperation(item, auditPrefix+".failed", models.AuditSeverityWarn, auditOperationMessage(item.OperationID, "timed out"), &instanceID, "LIFECYCLE_TIMEOUT")
		return
	}
	_ = w.service.repo.RequeueOperation(ctx, item.OperationID, lifecycleWaitingCode, "Waiting for the instance runtime to become ready", now.Add(lifecycleReadyPollInterval), now)
}

func (w *OperationWorker) runLifecycleActionWithLease(ctx context.Context, item *models.NorthboundOperation, action func() error) error {
	result := make(chan error, 1)
	go func() { result <- action() }()
	lease := time.Duration(w.service.settings().OperationLeaseSeconds) * time.Second
	if lease <= 0 {
		lease = 30 * time.Second
	}
	interval := lease / 3
	if interval < time.Second {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case err := <-result:
			return err
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if item.LeaseOwner == nil {
				continue
			}
			now := time.Now().UTC()
			if err := w.service.repo.RenewOperationLease(ctx, item.OperationID, *item.LeaseOwner, now.Add(lease), now); err != nil {
				log.Printf("northbound lifecycle operation %s lease renewal failed: %v", item.OperationID, err)
			}
		}
	}
}

func restartInstance(instances coreInstanceService, instanceID int) error {
	restarter, ok := instances.(interface{ Restart(int) error })
	if !ok {
		return errors.New("instance restart service is unavailable")
	}
	return restarter.Restart(instanceID)
}

func resetInstance(instances coreInstanceService, instanceID int) error {
	resetter, ok := instances.(services.InstanceResetService)
	if !ok {
		return errors.New("instance reset service is unavailable")
	}
	return resetter.Reset(instanceID)
}

func operationCreateRequest(item *models.NorthboundOperation) (services.CreateInstanceRequest, string, error) {
	return operationCreateRequestWithSettings(item, nil)
}

func (s *CoreService) operationCreateRequest(item *models.NorthboundOperation) (services.CreateInstanceRequest, string, error) {
	return operationCreateRequestWithSettings(item, s.settings())
}

func operationCreateRequestWithSettings(item *models.NorthboundOperation, settings *models.NorthboundAdminSettings) (services.CreateInstanceRequest, string, error) {
	if item == nil {
		return services.CreateInstanceRequest{}, "", errors.New("operation is required")
	}
	switch item.OperationType {
	case OperationTypeLiteInstance:
		var request CreateLiteInstanceRequest
		if err := json.Unmarshal([]byte(item.RequestPayload), &request); err != nil {
			return services.CreateInstanceRequest{}, "", err
		}
		if strings.EqualFold(strings.TrimSpace(request.Type), "workbuddy") {
			createRequest, err := proCreateRequestWithSettings(item, CreateProInstanceRequest(request), settings)
			return createRequest, "northbound.pro.create", err
		}
		return liteCreateRequestWithSettings(item, request, settings), "northbound.lite.create", nil
	case OperationTypeProInstance:
		var request CreateProInstanceRequest
		if err := json.Unmarshal([]byte(item.RequestPayload), &request); err != nil {
			return services.CreateInstanceRequest{}, "", err
		}
		if !isSupportedNorthboundProType(request.Type) {
			return services.CreateInstanceRequest{}, "", errors.New("unsupported Pro instance type")
		}
		createRequest, err := proCreateRequestWithSettings(item, request, settings)
		return createRequest, "northbound.pro.create", err
	default:
		return services.CreateInstanceRequest{}, "", fmt.Errorf("unsupported operation type %q", item.OperationType)
	}
}

func liteCreateRequest(item *models.NorthboundOperation, request CreateLiteInstanceRequest) services.CreateInstanceRequest {
	return liteCreateRequestWithSettings(item, request, nil)
}

func liteCreateRequestWithSettings(item *models.NorthboundOperation, request CreateLiteInstanceRequest, settings *models.NorthboundAdminSettings) services.CreateInstanceRequest {
	cpu, memory, disk := float64(2), 4, services.DefaultLiteDiskGB
	if settings != nil {
		cpu = settings.LiteCPUCores
		memory = settings.LiteMemoryGB
		disk = settings.LiteDiskGB
	}
	return services.CreateInstanceRequest{
		Name:                    request.Name,
		Owner:                   &request.Owner,
		Description:             request.Description,
		Type:                    request.Type,
		Mode:                    services.InstanceModeLite,
		InstanceMode:            services.InstanceModeLite,
		RuntimeType:             services.RuntimeBackendGateway,
		CPUCores:                cpu,
		MemoryGB:                memory,
		DiskGB:                  disk,
		GPUEnabled:              false,
		GPUCount:                0,
		OSType:                  request.Type,
		OSVersion:               "latest",
		ProvisioningOperationID: item.OperationID,
	}
}

func proCreateRequest(item *models.NorthboundOperation, request CreateProInstanceRequest) (services.CreateInstanceRequest, error) {
	return proCreateRequestWithSettings(item, request, nil)
}

func proCreateRequestWithSettings(item *models.NorthboundOperation, request CreateProInstanceRequest, settings *models.NorthboundAdminSettings) (services.CreateInstanceRequest, error) {
	instanceType := strings.ToLower(strings.TrimSpace(request.Type))
	if instanceType == "workbuddy" {
		cpu, memory, disk := float64(northboundWorkbuddyCPUCores), northboundWorkbuddyMemoryGB, northboundWorkbuddyDiskGB
		if settings != nil {
			cpu = settings.WorkBuddyProCPUCores
			memory = settings.WorkBuddyProMemoryGB
			disk = settings.WorkBuddyProDiskGB
		}
		image := services.LinuxWorkbuddyImage()
		return services.CreateInstanceRequest{
			Name:                    request.Name,
			Owner:                   &request.Owner,
			Description:             request.Description,
			Type:                    "workbuddy",
			RuntimeVariant:          services.WorkbuddyRuntimeLinux,
			Mode:                    services.InstanceModePro,
			InstanceMode:            services.InstanceModePro,
			RuntimeType:             services.RuntimeBackendDesktop,
			CPUCores:                cpu,
			MemoryGB:                memory,
			DiskGB:                  disk,
			GPUEnabled:              false,
			GPUCount:                0,
			OSType:                  "workbuddy",
			OSVersion:               "latest",
			ImageRegistry:           &image,
			ProvisioningOperationID: item.OperationID,
		}, nil
	}
	if !isSupportedNorthboundProType(instanceType) {
		return services.CreateInstanceRequest{}, errors.New("unsupported Pro instance type")
	}
	imageConfig, ok := services.RuntimeImageForBackend(instanceType, services.RuntimeBackendDesktop)
	if !ok || strings.TrimSpace(imageConfig.Image) == "" {
		return services.CreateInstanceRequest{}, fmt.Errorf("enabled Pro image is not configured for %s", instanceType)
	}
	image := strings.TrimSpace(imageConfig.Image)
	cpu, memory, disk := float64(northboundProCPUCores), northboundProMemoryGB, northboundProDiskGB
	if settings != nil {
		cpu = settings.ProCPUCores
		memory = settings.ProMemoryGB
		disk = settings.ProDiskGB
	}
	return services.CreateInstanceRequest{
		Name:                    request.Name,
		Owner:                   &request.Owner,
		Description:             request.Description,
		Type:                    instanceType,
		RuntimeVariant:          imageConfig.RuntimeVariant,
		Mode:                    services.InstanceModePro,
		InstanceMode:            services.InstanceModePro,
		RuntimeType:             services.RuntimeBackendDesktop,
		CPUCores:                cpu,
		MemoryGB:                memory,
		DiskGB:                  disk,
		GPUEnabled:              false,
		GPUCount:                0,
		OSType:                  instanceType,
		OSVersion:               "latest",
		ImageRegistry:           &image,
		ProvisioningOperationID: item.OperationID,
	}, nil
}

func classifyCreateError(err error, instanceMode string) (string, string, bool) {
	message := strings.ToLower(err.Error())
	modeName := "Lite"
	capacityCode := "LITE_CAPACITY_EXHAUSTED"
	if strings.EqualFold(strings.TrimSpace(instanceMode), services.InstanceModePro) || instanceMode == OperationTypeProInstance {
		modeName = "Pro"
		capacityCode = "PRO_CAPACITY_EXHAUSTED"
	}
	switch {
	case strings.Contains(message, "instance name already exists"):
		return "NAME_CONFLICT", "Instance name already exists", false
	case strings.Contains(message, "quota"), strings.Contains(message, "instance limit reached"):
		return "QUOTA_EXCEEDED", "Instance quota exceeded", false
	case strings.Contains(message, "capacity reached"):
		return capacityCode, modeName + " instance capacity is temporarily exhausted", true
	case strings.Contains(message, "invalid"), strings.Contains(message, "unsupported"):
		return "VALIDATION_ERROR", modeName + " instance request is invalid", false
	default:
		return "DEPENDENCY_UNAVAILABLE", "A provisioning dependency is temporarily unavailable", true
	}
}
