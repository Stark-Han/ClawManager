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

type CoreService struct {
	repo           *repository.NorthboundRepository
	users          repository.UserRepository
	instances      coreInstanceService
	externalAccess shareLinkService
	config         config.NorthboundConfig
	audit          repository.AuditEventRepository
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

func NewCoreService(repo *repository.NorthboundRepository, users repository.UserRepository, instances services.InstanceService, externalAccess services.InstanceExternalAccessService, cfg config.NorthboundConfig) *CoreService {
	return &CoreService{repo: repo, users: users, instances: instances, externalAccess: externalAccess, config: cfg}
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
	if len(req.Name) < 3 || len(req.Name) > 50 || !isSupportedNorthboundLiteType(req.Type) {
		return nil, false, apiError(422, "VALIDATION_ERROR", "Invalid Lite instance request", nil)
	}
	if req.Description != nil && len(*req.Description) > 2000 {
		return nil, false, apiError(422, "VALIDATION_ERROR", "Description is too long", nil)
	}
	return s.submitCreateOperation(
		principal,
		idempotencyKey,
		req,
		OperationTypeLiteInstance,
		"Too many unfinished Lite instance operations",
	)
}

func isSupportedNorthboundLiteType(instanceType string) bool {
	switch strings.ToLower(strings.TrimSpace(instanceType)) {
	case services.RuntimeTypeOpenClaw,
		services.RuntimeTypeHermes,
		services.RuntimeTypeOpenCode,
		services.RuntimeTypeDeepSeekHarness:
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
	if len(req.Name) < 3 || len(req.Name) > 50 || req.Type != "workbuddy" {
		return nil, false, apiError(422, "VALIDATION_ERROR", "Invalid Pro instance request", nil)
	}
	if req.Description != nil && len(*req.Description) > 2000 {
		return nil, false, apiError(422, "VALIDATION_ERROR", "Description is too long", nil)
	}
	return s.submitCreateOperation(
		principal,
		idempotencyKey,
		req,
		OperationTypeProInstance,
		"Too many unfinished Pro instance operations",
	)
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
	if pending >= 5 {
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

func (s *CoreService) GetInstance(userID, instanceID int) (*models.Instance, error) {
	item, err := s.instances.GetByID(instanceID)
	if err != nil {
		return nil, err
	}
	if item == nil || item.UserID != userID || !isLite(item) {
		return nil, apiError(404, "INSTANCE_NOT_FOUND", "Instance not found", nil)
	}
	return item, nil
}

func (s *CoreService) GetProInstance(userID, instanceID int) (*models.Instance, error) {
	item, err := s.instances.GetByID(instanceID)
	if err != nil {
		return nil, err
	}
	if item == nil || item.UserID != userID || !isWorkbuddyLinuxPro(item) {
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
	items, total, err := ownerService.GetLiteByUserIDAndOwner(userID, normalizedOwner, (page-1)*limit, limit)
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
	items, total, err := ownerService.GetWorkbuddyProByUserIDAndOwner(userID, normalizedOwner, (page-1)*limit, limit)
	if err != nil {
		return nil, 0, err
	}
	filtered := make([]LiteInstanceResponse, 0, len(items))
	for idx := range items {
		if isWorkbuddyLinuxPro(&items[idx]) {
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
	tick := w.service.config.OperationTick
	if tick <= 0 {
		tick = time.Second
	}
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
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
	lease := w.service.config.OperationLease
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
	createRequest, auditPrefix, err := operationCreateRequest(item)
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
	code, message, retryable := classifyCreateError(createErr, item.OperationType)
	maxAttempts := w.service.config.OperationMaxAttempts
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

func operationCreateRequest(item *models.NorthboundOperation) (services.CreateInstanceRequest, string, error) {
	if item == nil {
		return services.CreateInstanceRequest{}, "", errors.New("operation is required")
	}
	switch item.OperationType {
	case OperationTypeLiteInstance:
		var request CreateLiteInstanceRequest
		if err := json.Unmarshal([]byte(item.RequestPayload), &request); err != nil {
			return services.CreateInstanceRequest{}, "", err
		}
		return liteCreateRequest(item, request), "northbound.lite.create", nil
	case OperationTypeProInstance:
		var request CreateProInstanceRequest
		if err := json.Unmarshal([]byte(item.RequestPayload), &request); err != nil {
			return services.CreateInstanceRequest{}, "", err
		}
		if strings.ToLower(strings.TrimSpace(request.Type)) != "workbuddy" {
			return services.CreateInstanceRequest{}, "", errors.New("unsupported Pro instance type")
		}
		return proCreateRequest(item, request), "northbound.pro.create", nil
	default:
		return services.CreateInstanceRequest{}, "", fmt.Errorf("unsupported operation type %q", item.OperationType)
	}
}

func liteCreateRequest(item *models.NorthboundOperation, request CreateLiteInstanceRequest) services.CreateInstanceRequest {
	return services.CreateInstanceRequest{
		Name:                    request.Name,
		Owner:                   &request.Owner,
		Description:             request.Description,
		Type:                    request.Type,
		Mode:                    services.InstanceModeLite,
		InstanceMode:            services.InstanceModeLite,
		RuntimeType:             services.RuntimeBackendGateway,
		CPUCores:                2,
		MemoryGB:                4,
		DiskGB:                  20,
		GPUEnabled:              false,
		GPUCount:                0,
		OSType:                  request.Type,
		OSVersion:               "latest",
		ProvisioningOperationID: item.OperationID,
	}
}

func proCreateRequest(item *models.NorthboundOperation, request CreateProInstanceRequest) services.CreateInstanceRequest {
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
		CPUCores:                2,
		MemoryGB:                4,
		DiskGB:                  20,
		GPUEnabled:              false,
		GPUCount:                0,
		OSType:                  "workbuddy",
		OSVersion:               "latest",
		ImageRegistry:           &image,
		ProvisioningOperationID: item.OperationID,
	}
}

func classifyCreateError(err error, operationType string) (string, string, bool) {
	message := strings.ToLower(err.Error())
	modeName := "Lite"
	capacityCode := "LITE_CAPACITY_EXHAUSTED"
	if operationType == OperationTypeProInstance {
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
