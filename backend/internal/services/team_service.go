package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	posixpath "path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"clawreef/internal/models"
	"clawreef/internal/repository"
	"clawreef/internal/services/k8s"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

const (
	teamSharedMountPath     = "/team"
	teamConfigFileName      = "team.json"
	teamConfigMountDirPath  = "/etc/clawmanager/team"
	teamConfigMountPath     = teamConfigMountDirPath + "/" + teamConfigFileName
	teamHermesSoulMountPath = "/config/.hermes/SOUL.md"
	teamSharedUID           = 1000
	teamSharedGID           = 1000
	teamSharedUmask         = "0002"
	teamRedisURLSecretKey   = "CLAWMANAGER_TEAM_REDIS_URL"
	teamTokenSecretKey      = "CLAWMANAGER_TEAM_TOKEN"

	defaultTeamTaskStaleTimeout = 30 * time.Minute
	teamTaskStaleSweepInterval  = 30 * time.Second

	initialLeaderTaskIntent = "team_bootstrap_introduction"
	teamTaskCompletionTool  = "team_complete_task"
	teamTaskReplyTarget     = "clawmanager"
)

var (
	teamMemberKeyPattern                = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	teamMemberInstanceNameInvalidChars  = regexp.MustCompile(`[^a-z0-9-]+`)
	teamMemberInstanceNameRepeatedDashs = regexp.MustCompile(`-+`)
)

type TeamService interface {
	Start()
	Stop()
	CreateTeam(userID int, req CreateTeamRequest) (*TeamDetailsPayload, error)
	ListTeams(userID, offset, limit int) (*TeamListPayload, error)
	GetTeam(userID, teamID int) (*TeamDetailsPayload, error)
	ListTeamTasks(userID, teamID, beforeID, limit int) (*TeamTasksHistoryPayload, error)
	ListTeamEvents(userID, teamID, beforeID, limit int) (*TeamEventsHistoryPayload, error)
	ListWorkspaceFiles(ctx context.Context, userID, teamID int, relPath string) (*TeamWorkspaceListPayload, error)
	PreviewWorkspaceFile(ctx context.Context, userID, teamID int, relPath string) (*TeamWorkspacePreviewPayload, error)
	DownloadWorkspaceFile(ctx context.Context, userID, teamID int, relPath string) (*TeamWorkspaceDownloadPayload, error)
	CreateWorkspaceFolder(ctx context.Context, userID, teamID int, req TeamWorkspaceFolderRequest) error
	RenameWorkspaceEntry(ctx context.Context, userID, teamID int, req TeamWorkspaceRenameRequest) error
	DeleteWorkspaceEntry(ctx context.Context, userID, teamID int, relPath string) error
	UploadWorkspaceFiles(ctx context.Context, userID, teamID int, targetPath string, files []*multipart.FileHeader, relativePaths []string) error
	DispatchTask(userID, teamID int, req DispatchTeamTaskRequest) (*TeamTaskPayload, error)
	DeleteTeam(userID, teamID int) error
	DeleteMember(userID, teamID int, memberID string) error
}

type CreateTeamRequest struct {
	Name              string                    `json:"name"`
	Description       *string                   `json:"description,omitempty"`
	CommunicationMode string                    `json:"communication_mode,omitempty"`
	RedisURL          string                    `json:"redis_url,omitempty"`
	SharedStorageGB   int                       `json:"shared_storage_gb,omitempty"`
	StorageClass      string                    `json:"storage_class,omitempty"`
	Members           []CreateTeamMemberRequest `json:"members"`
}

type CreateTeamMemberRequest struct {
	MemberID             string              `json:"member_id,omitempty"`
	Name                 string              `json:"name,omitempty"`
	Role                 string              `json:"role"`
	Mode                 string              `json:"mode,omitempty"`
	InstanceMode         string              `json:"instance_mode,omitempty"`
	RuntimeType          string              `json:"runtime_type,omitempty"`
	Description          *string             `json:"description,omitempty"`
	CPUCores             float64             `json:"cpu_cores,omitempty"`
	MemoryGB             int                 `json:"memory_gb,omitempty"`
	DiskGB               int                 `json:"disk_gb,omitempty"`
	GPUEnabled           bool                `json:"gpu_enabled,omitempty"`
	GPUCount             int                 `json:"gpu_count,omitempty"`
	ImageRegistry        *string             `json:"image_registry,omitempty"`
	ImageTag             *string             `json:"image_tag,omitempty"`
	EnvironmentOverrides map[string]string   `json:"environment_overrides,omitempty"`
	OpenClawConfigPlan   *OpenClawConfigPlan `json:"openclaw_config_plan,omitempty"`
	IsLeader             bool                `json:"is_leader,omitempty"`
}

type DispatchTeamTaskRequest struct {
	TargetMemberID string                 `json:"target_member_id"`
	MessageID      string                 `json:"message_id,omitempty"`
	Payload        map[string]interface{} `json:"payload"`
}

type TeamListPayload struct {
	Teams []models.Team `json:"teams"`
	Total int           `json:"total"`
}

type TeamDetailsPayload struct {
	Team           *models.Team        `json:"team"`
	LeaderMemberID string              `json:"leader_member_id,omitempty"`
	Leader         *models.TeamMember  `json:"leader,omitempty"`
	Members        []models.TeamMember `json:"members"`
	Tasks          []TeamTaskPayload   `json:"tasks,omitempty"`
	Events         []TeamEventPayload  `json:"events,omitempty"`
}

type TeamTasksHistoryPayload struct {
	Tasks        []TeamTaskPayload `json:"tasks"`
	HasMore      bool              `json:"has_more"`
	NextBeforeID *int              `json:"next_before_id,omitempty"`
}

type TeamEventsHistoryPayload struct {
	Events       []TeamEventPayload `json:"events"`
	HasMore      bool               `json:"has_more"`
	NextBeforeID *int               `json:"next_before_id,omitempty"`
}

type TeamWorkspaceFileEntry struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Type        string `json:"type"`
	Size        int64  `json:"size"`
	ModifiedAt  string `json:"modified_at,omitempty"`
	Previewable bool   `json:"previewable"`
}

type TeamWorkspaceListPayload struct {
	Path    string                   `json:"path"`
	Root    string                   `json:"root"`
	Entries []TeamWorkspaceFileEntry `json:"entries"`
}

type TeamWorkspacePreviewPayload struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Content string `json:"content"`
}

type TeamWorkspaceDownloadPayload struct {
	Path        string
	Name        string
	ContentType string
	Data        []byte
}

type TeamWorkspaceFolderRequest struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

type TeamWorkspaceRenameRequest struct {
	Path    string `json:"path"`
	NewName string `json:"new_name"`
}

type TeamTaskPayload struct {
	models.TeamTask
	Payload map[string]interface{} `json:"payload,omitempty"`
	Result  map[string]interface{} `json:"result,omitempty"`
}

type TeamEventPayload struct {
	models.TeamEvent
	Payload map[string]interface{} `json:"payload,omitempty"`
}

type teamService struct {
	repo             repository.TeamRepository
	instanceService  InstanceService
	pvcService       *k8s.PVCService
	secretService    *k8s.SecretService
	configMapService *k8s.ConfigMapService
	podService       *k8s.PodService

	ctx                  context.Context
	cancel               context.CancelFunc
	mu                   sync.Mutex
	consumers            map[int]struct{}
	staleMonitorStarted  bool
	runtimeWorkspaceRoot string
}

type plannedTeamMember struct {
	Request      CreateTeamMemberRequest
	MemberKey    string
	DisplayName  string
	Role         string
	RuntimeType  string
	InstanceMode string
	IsLeader     bool
}

type teamRuntimeSecrets struct {
	RedisURL string
	Token    string
}

type TeamServiceOption func(*teamService)

func WithTeamRuntimeWorkspaceRoot(root string) TeamServiceOption {
	return func(s *teamService) {
		if strings.TrimSpace(root) != "" {
			s.runtimeWorkspaceRoot = strings.TrimSpace(root)
		}
	}
}

func NewTeamService(repo repository.TeamRepository, instanceService InstanceService, opts ...TeamServiceOption) TeamService {
	ctx, cancel := context.WithCancel(context.Background())
	service := &teamService{
		repo:                 repo,
		instanceService:      instanceService,
		pvcService:           k8s.NewPVCService(),
		secretService:        k8s.NewSecretService(),
		configMapService:     k8s.NewConfigMapService(),
		podService:           k8s.NewPodService(),
		ctx:                  ctx,
		cancel:               cancel,
		consumers:            map[int]struct{}{},
		runtimeWorkspaceRoot: "/workspaces",
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	return service
}

func (s *teamService) Start() {
	teams, err := s.repo.ListActiveTeams()
	if err != nil {
		fmt.Printf("Warning: failed to start Team event consumers: %v\n", err)
		return
	}
	for _, team := range teams {
		s.ensureConsumer(team.ID)
	}
	s.ensureStaleTaskMonitor()
}

func (s *teamService) Stop() {
	s.cancel()
}

func (s *teamService) CreateTeam(userID int, req CreateTeamRequest) (*TeamDetailsPayload, error) {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return nil, fmt.Errorf("team name is required")
	}
	if len(req.Members) == 0 {
		return nil, fmt.Errorf("team must include at least one member")
	}
	memberPlans, err := planTeamMembers(req.Name, req.Members)
	if err != nil {
		return nil, err
	}
	existingTeam, err := s.repo.GetTeamByUserIDAndName(userID, req.Name)
	if err != nil {
		return nil, err
	}
	if existingTeam != nil {
		if existingTeam.Status == models.TeamStatusFailed {
			if err := s.DeleteTeam(userID, existingTeam.ID); err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("team name already exists")
		}
	}

	communicationMode := strings.TrimSpace(req.CommunicationMode)
	if communicationMode == "" {
		communicationMode = "leader_mediated"
	}
	redisURL := strings.TrimSpace(req.RedisURL)
	if redisURL == "" {
		redisURL = defaultTeamRedisURL()
	}
	if redisURL == "" {
		return nil, fmt.Errorf("team redis url is required")
	}
	if _, err := newRedisBus(redisURL); err != nil {
		return nil, err
	}

	sharedStorageGB := req.SharedStorageGB
	if sharedStorageGB <= 0 {
		sharedStorageGB = 10
	}
	preflightTeam := &models.Team{
		ID:              0,
		Name:            req.Name,
		StorageClass:    optionalString(strings.TrimSpace(req.StorageClass)),
		SharedMountPath: teamSharedMountPath,
	}
	if err := s.instanceService.ValidateCreateRequests(userID, s.buildTeamMemberInstanceRequests(preflightTeam, memberPlans)); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	storageClass := optionalString(strings.TrimSpace(req.StorageClass))
	team := &models.Team{
		UserID:            userID,
		Name:              req.Name,
		Description:       req.Description,
		Status:            models.TeamStatusCreating,
		CommunicationMode: communicationMode,
		RedisEventsLastID: "0-0",
		SharedMountPath:   teamSharedMountPath,
		StorageClass:      storageClass,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := s.repo.CreateTeam(team); err != nil {
		return nil, err
	}
	if err := s.instanceService.ValidateCreateRequests(userID, s.buildTeamMemberInstanceRequests(team, memberPlans)); err != nil {
		return nil, s.rollbackTeamCreation(userID, team, err)
	}

	runtimeSecrets, err := s.provisionTeamK8s(userID, team, redisURL, sharedStorageGB, strings.TrimSpace(req.StorageClass))
	if err != nil {
		return nil, s.rollbackTeamCreation(userID, team, err)
	}
	rosterJSON, err := s.upsertTeamRosterConfig(userID, team, memberPlans)
	if err != nil {
		return nil, s.rollbackTeamCreation(userID, team, err)
	}

	for _, memberPlan := range memberPlans {
		member, err := s.createTeamMemberInstance(userID, team, memberPlan, runtimeSecrets, rosterJSON)
		if err != nil {
			return nil, s.rollbackTeamCreation(userID, team, err)
		}
		member.Status = models.TeamMemberStatusIdle
		member.UpdatedAt = time.Now().UTC()
		if err := s.repo.UpdateMember(member); err != nil {
			return nil, s.rollbackTeamCreation(userID, team, err)
		}
	}

	team.Status = models.TeamStatusRunning
	team.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateTeam(team); err != nil {
		return nil, err
	}
	s.ensureConsumer(team.ID)
	s.ensureStaleTaskMonitor()
	if err := s.dispatchInitialLeaderTask(userID, team); err != nil {
		fmt.Printf("Warning: failed to dispatch initial Team %d leader task: %v\n", team.ID, err)
		if recordErr := s.recordInitialLeaderTaskDispatchFailure(team.ID, err); recordErr != nil {
			fmt.Printf("Warning: failed to record Team %d initial leader task dispatch failure: %v\n", team.ID, recordErr)
		}
	}
	return s.GetTeam(userID, team.ID)
}

func (s *teamService) dispatchInitialLeaderTask(userID int, team *models.Team) error {
	if team == nil {
		return fmt.Errorf("team is required")
	}
	members, err := s.repo.ListMembersByTeamID(team.ID)
	if err != nil {
		return err
	}
	leader := findTeamLeader(activeTeamMembers(members))
	if leader == nil {
		return fmt.Errorf("team leader not found")
	}
	_, err = s.DispatchTask(userID, team.ID, DispatchTeamTaskRequest{
		TargetMemberID: leader.MemberKey,
		MessageID:      initialLeaderTaskMessageID(team.ID),
		Payload:        buildInitialLeaderTaskPayload(team.Name),
	})
	return err
}

func initialLeaderTaskMessageID(teamID int) string {
	return fmt.Sprintf("team-%d-bootstrap-introduction", teamID)
}

func (s *teamService) recordInitialLeaderTaskDispatchFailure(teamID int, cause error) error {
	now := time.Now().UTC()
	payload := map[string]interface{}{
		"v":         1,
		"event":     "bootstrap_dispatch_failed",
		"teamId":    strconv.Itoa(teamID),
		"intent":    initialLeaderTaskIntent,
		"messageId": initialLeaderTaskMessageID(teamID),
		"source":    "clawmanager",
	}
	if cause != nil {
		payload["diagnostic"] = cause.Error()
	}
	payloadJSON, err := marshalOptionalJSON(payload)
	if err != nil {
		return err
	}
	messageID := initialLeaderTaskMessageID(teamID)
	return s.repo.CreateEvent(&models.TeamEvent{
		TeamID:      teamID,
		MessageID:   &messageID,
		EventType:   "bootstrap_dispatch_failed",
		PayloadJSON: payloadJSON,
		OccurredAt:  &now,
		CreatedAt:   now,
	})
}

func buildTeamTaskEnvelope(teamID int, memberKey string, task *models.TeamTask, messageID string, taskPayload map[string]interface{}, memberContext map[string]string, now time.Time) map[string]interface{} {
	if taskPayload == nil {
		taskPayload = map[string]interface{}{}
	}
	taskID := 0
	if task != nil {
		taskID = task.ID
	}
	taskRef := fmt.Sprintf("team-%d-task-%d", teamID, taskID)
	prompt := eventString(taskPayload, "prompt", "goal", "instruction", "instructions")
	if prompt == "" {
		rawPayload, _ := marshalJSON(taskPayload)
		prompt = rawPayload
	}
	rawPrompt := prompt
	prompt = buildTeamRuntimePrompt(prompt, memberContext)
	envelope := map[string]interface{}{
		"v":                  1,
		"messageId":          messageID,
		"teamId":             strconv.Itoa(teamID),
		"from":               "clawmanager",
		"to":                 memberKey,
		"replyTo":            teamTaskReplyTarget,
		"requiresCompletion": true,
		"completionTool":     teamTaskCompletionTool,
		"resultSink": map[string]interface{}{
			"type":           "redis_stream",
			"eventsKey":      teamEventsKey(teamID),
			"successEvent":   "task_completed",
			"failureEvent":   "task_failed",
			"replyEvent":     "reply",
			"resultField":    "resultMarkdown",
			"summaryField":   "summary",
			"artifactField":  "artifactRefs",
			"completionTool": teamTaskCompletionTool,
		},
		"intent":        eventString(taskPayload, "intent"),
		"taskId":        taskRef,
		"title":         eventString(taskPayload, "title"),
		"prompt":        appendTeamTaskCompletionInstruction(prompt),
		"rawPrompt":     rawPrompt,
		"contextRefs":   normalizeContextRefs(taskPayload["contextRefs"]),
		"memberContext": memberContext,
		"systemPrompt":  memberContext["systemPrompt"],
		"metadata":      taskPayload,
		"createdAt":     now.Format(time.RFC3339Nano),
	}
	if envelope["intent"] == "" {
		envelope["intent"] = "run_task"
	}
	if envelope["title"] == "" {
		envelope["title"] = fmt.Sprintf("Team task %d", taskID)
	}
	return envelope
}

func appendTeamTaskCompletionInstruction(prompt string) string {
	base := strings.TrimSpace(prompt)
	if strings.Contains(base, teamTaskCompletionTool) && strings.Contains(base, "task_completed") {
		return base
	}
	instruction := strings.Join([]string{
		"Completion contract:",
		"- When the final result is ready, call team_complete_task with status=\"succeeded\", summary, and resultMarkdown.",
		"- If the task fails, call team_complete_task with status=\"failed\" and an error message.",
		"- Do not send the final answer as a normal message to clawmanager; ClawManager consumes task_completed/task_failed events from the Team Redis event stream.",
	}, "\n")
	if base == "" {
		return instruction
	}
	return base + "\n\n" + instruction
}

func (s *teamService) provisionTeamK8s(userID int, team *models.Team, redisURL string, sharedStorageGB int, storageClass string) (*teamRuntimeSecrets, error) {
	ctx := context.Background()
	pvc, err := s.pvcService.CreateTeamSharedPVC(ctx, userID, team.ID, sharedStorageGB, storageClass)
	if err != nil {
		return nil, err
	}
	secretName := s.pvcService.GetClient().GetTeamSecretName(team.ID)
	teamToken, err := generatePrefixedToken("team")
	if err != nil {
		return nil, fmt.Errorf("failed to generate Team token: %w", err)
	}
	if err := s.secretService.UpsertSecret(ctx, userID, secretName, map[string]string{
		teamRedisURLSecretKey: redisURL,
		teamTokenSecretKey:    teamToken,
	}, map[string]string{
		"app":        "clawreef",
		"managed-by": "clawreef",
		"team-id":    strconv.Itoa(team.ID),
	}); err != nil {
		return nil, err
	}

	team.RedisURLSecretName = &secretName
	team.RedisURLSecretKey = optionalString(teamRedisURLSecretKey)
	team.TeamTokenSecretName = &secretName
	team.TeamTokenSecretKey = optionalString(teamTokenSecretKey)
	team.SharedPVCName = &pvc.Name
	team.SharedPVCNamespace = &pvc.Namespace
	team.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateTeam(team); err != nil {
		return nil, err
	}
	return &teamRuntimeSecrets{RedisURL: redisURL, Token: teamToken}, nil
}

func (s *teamService) upsertTeamRosterConfig(userID int, team *models.Team, members []plannedTeamMember) (string, error) {
	rosterJSON, err := buildTeamRosterConfig(team, members)
	if err != nil {
		return "", err
	}
	data := buildTeamRosterConfigData(rosterJSON, members)
	if err := s.configMapService.UpsertConfigMap(context.Background(), userID, s.teamConfigMapName(team.ID), data, map[string]string{
		"app":        "clawreef",
		"managed-by": "clawreef",
		"team-id":    strconv.Itoa(team.ID),
	}); err != nil {
		return "", err
	}
	return rosterJSON, nil
}

func buildTeamRosterConfigData(rosterJSON string, members []plannedTeamMember) map[string]string {
	data := map[string]string{
		teamConfigFileName: rosterJSON,
	}
	for _, member := range members {
		if member.RuntimeType != "hermes" {
			continue
		}
		data[teamMemberSoulConfigKey(member.MemberKey)] = buildTeamMemberSoulMarkdown(member)
	}
	return data
}

func (s *teamService) teamConfigMapName(teamID int) string {
	client := k8s.GetClient()
	if client == nil {
		return fmt.Sprintf("clawreef-team-%d-config", teamID)
	}
	return client.GetTeamConfigMapName(teamID)
}

func (s *teamService) createTeamMemberInstance(userID int, team *models.Team, memberPlan plannedTeamMember, runtimeSecrets *teamRuntimeSecrets, rosterJSON string) (*models.TeamMember, error) {
	now := time.Now().UTC()
	member := &models.TeamMember{
		TeamID:       team.ID,
		UserID:       userID,
		MemberKey:    memberPlan.MemberKey,
		DisplayName:  memberPlan.DisplayName,
		Role:         memberPlan.Role,
		RuntimeType:  memberPlan.RuntimeType,
		InstanceMode: memberPlan.InstanceMode,
		Description:  optionalString(strings.TrimSpace(derefTeamString(memberPlan.Request.Description))),
		Status:       models.TeamMemberStatusCreating,
		Availability: models.TeamMemberAvailabilityUnknown,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.repo.CreateMember(member); err != nil {
		return nil, err
	}

	createReq := s.buildTeamMemberInstanceRequestWithSecrets(team, memberPlan, runtimeSecrets, rosterJSON)
	instance, err := s.instanceService.Create(userID, createReq)
	if err != nil {
		member.Status = models.TeamMemberStatusFailed
		member.UpdatedAt = time.Now().UTC()
		_ = s.repo.UpdateMember(member)
		return nil, err
	}
	member.InstanceID = &instance.ID
	member.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateMember(member); err != nil {
		return nil, err
	}
	return member, nil
}

func (s *teamService) buildTeamMemberInstanceRequests(team *models.Team, memberPlans []plannedTeamMember) []CreateInstanceRequest {
	requests := make([]CreateInstanceRequest, 0, len(memberPlans))
	for _, memberPlan := range memberPlans {
		requests = append(requests, s.buildTeamMemberInstanceRequest(team, memberPlan))
	}
	return requests
}

func (s *teamService) buildTeamMemberInstanceRequest(team *models.Team, memberPlan plannedTeamMember) CreateInstanceRequest {
	return s.buildTeamMemberInstanceRequestWithSecrets(team, memberPlan, nil, "")
}

func (s *teamService) buildTeamMemberInstanceRequestWithSecrets(team *models.Team, memberPlan plannedTeamMember, runtimeSecrets *teamRuntimeSecrets, rosterJSON string) CreateInstanceRequest {
	req := memberPlan.Request
	instanceMode := memberPlan.InstanceMode
	if instanceMode == "" {
		instanceMode = InstanceModeLite
	}
	runtimeBackendType, _ := RuntimeTypeForInstanceMode(instanceMode)
	memberEnv := s.teamMemberEnv(team, memberPlan)
	if instanceMode == InstanceModeLite {
		memberEnv["CLAWMANAGER_TEAM_SHARED_DIR"] = s.teamRuntimeSharedPath(team)
	}
	environmentOverrides := mergeEnvMaps(req.EnvironmentOverrides, memberEnv)
	if instanceMode == InstanceModeLite && runtimeSecrets != nil {
		environmentOverrides = mergeEnvMaps(environmentOverrides, map[string]string{
			teamRedisURLSecretKey: runtimeSecrets.RedisURL,
			teamTokenSecretKey:    runtimeSecrets.Token,
		})
		if strings.TrimSpace(rosterJSON) != "" {
			environmentOverrides["CLAWMANAGER_TEAM_CONFIG_JSON"] = rosterJSONWithSharedDir(rosterJSON, s.teamRuntimeSharedPath(team))
		}
	}
	return CreateInstanceRequest{
		Name:                 teamMemberInstanceName(team.Name, team.ID, memberPlan.MemberKey),
		Type:                 memberPlan.RuntimeType,
		Mode:                 instanceMode,
		InstanceMode:         instanceMode,
		RuntimeType:          runtimeBackendType,
		CPUCores:             defaultFloat(req.CPUCores, 2),
		MemoryGB:             defaultInt(req.MemoryGB, 4),
		DiskGB:               defaultInt(req.DiskGB, 20),
		GPUEnabled:           req.GPUEnabled,
		GPUCount:             req.GPUCount,
		OSType:               memberPlan.RuntimeType,
		OSVersion:            "latest",
		ImageRegistry:        req.ImageRegistry,
		ImageTag:             req.ImageTag,
		EnvironmentOverrides: environmentOverrides,
		StorageClass:         derefTeamString(team.StorageClass),
		OpenClawConfigPlan:   req.OpenClawConfigPlan,
		Team: &TeamInstanceConfig{
			Environment:      memberEnv,
			SecretName:       derefTeamString(team.TeamTokenSecretName),
			SharedPVCName:    derefTeamString(team.SharedPVCName),
			SharedMountPath:  team.SharedMountPath,
			ConfigMapName:    s.teamConfigMapName(team.ID),
			ConfigMountPath:  teamConfigMountDirPath,
			PersonaConfigKey: teamMemberPersonaConfigKey(memberPlan),
			SharedUID:        teamSharedUID,
			SharedGID:        teamSharedGID,
			SharedUmask:      teamSharedUmask,
		},
	}
}

func (s *teamService) teamRuntimeSharedPath(team *models.Team) string {
	if team == nil {
		return k8s.TeamSharedWorkspacePath(s.runtimeWorkspaceRoot, 0, 0)
	}
	return k8s.TeamSharedWorkspacePath(s.runtimeWorkspaceRoot, team.UserID, team.ID)
}

func (s *teamService) teamMemberEnv(team *models.Team, member plannedTeamMember) map[string]string {
	managerBaseURL, _ := defaultTeamManagerBaseURL()
	memberContext := buildPlannedTeamMemberTaskContext(member)
	env := map[string]string{
		"CLAWMANAGER_TEAM_ENABLED":        "true",
		"CLAWMANAGER_TEAM_ID":             strconv.Itoa(team.ID),
		"CLAWMANAGER_TEAM_MEMBER_ID":      member.MemberKey,
		"CLAWMANAGER_TEAM_ROLE":           member.Role,
		"CLAWMANAGER_TEAM_SHARED_DIR":     team.SharedMountPath,
		"CLAWMANAGER_TEAM_SHARED_UID":     strconv.Itoa(teamSharedUID),
		"CLAWMANAGER_TEAM_SHARED_GID":     strconv.Itoa(teamSharedGID),
		"CLAWMANAGER_TEAM_UMASK":          teamSharedUmask,
		"PUID":                            strconv.Itoa(teamSharedUID),
		"PGID":                            strconv.Itoa(teamSharedGID),
		"UMASK":                           teamSharedUmask,
		"CLAWMANAGER_TEAM_CONFIG_PATH":    teamConfigMountPath,
		"CLAWMANAGER_TEAM_AUTORUN":        "true",
		"CLAWMANAGER_TEAM_CONSUMER_GROUP": "team-members",
		"CLAWMANAGER_TEAM_INBOX_KEY":      teamInboxKey(team.ID, member.MemberKey),
		"CLAWMANAGER_TEAM_EVENTS_KEY":     teamEventsKey(team.ID),
		"CLAWMANAGER_TEAM_PRESENCE_KEY":   teamPresenceKey(team.ID),
		"CLAWMANAGER_TEAM_DLQ_KEY":        teamDLQKey(team.ID),
		"CLAWMANAGER_TEAM_MANAGER_URL":    managerBaseURL,
	}
	if description := strings.TrimSpace(memberContext["description"]); description != "" {
		env["CLAWMANAGER_TEAM_MEMBER_DESCRIPTION"] = description
	}
	if systemPrompt := strings.TrimSpace(memberContext["systemPrompt"]); systemPrompt != "" {
		env["CLAWMANAGER_TEAM_SYSTEM_PROMPT"] = systemPrompt
		env["HERMES_AGENT_HELP_GUIDANCE"] = systemPrompt
	}
	return env
}

func teamMemberPersonaConfigKey(member plannedTeamMember) string {
	if member.RuntimeType != "hermes" {
		return ""
	}
	return teamMemberSoulConfigKey(member.MemberKey)
}

func teamMemberSoulConfigKey(memberKey string) string {
	key := normalizeTeamMemberKeyForInstanceName(memberKey)
	if key == "" {
		key = "member"
	}
	return fmt.Sprintf("hermes-soul-%s.md", key)
}

func (s *teamService) ListTeams(userID, offset, limit int) (*TeamListPayload, error) {
	teams, err := s.repo.ListTeamsByUserID(userID, offset, limit)
	if err != nil {
		return nil, err
	}
	teams = activeTeams(teams)
	total, err := s.repo.CountTeamsByUserID(userID)
	if err != nil {
		return nil, err
	}
	return &TeamListPayload{Teams: teams, Total: total}, nil
}

func (s *teamService) GetTeam(userID, teamID int) (*TeamDetailsPayload, error) {
	team, err := s.requireOwnedTeam(userID, teamID)
	if err != nil {
		return nil, err
	}
	members, err := s.repo.ListMembersByTeamID(teamID)
	if err != nil {
		return nil, err
	}
	members = activeTeamMembers(members)
	tasks, err := s.repo.ListTasksByTeamID(teamID, 20)
	if err != nil {
		return nil, err
	}
	events, err := s.repo.ListEventsByTeamID(teamID, 50)
	if err != nil {
		return nil, err
	}
	leader := findTeamLeader(members)
	return &TeamDetailsPayload{
		Team:           team,
		LeaderMemberID: leaderMemberKey(leader),
		Leader:         leader,
		Members:        members,
		Tasks:          teamTaskPayloads(tasks),
		Events:         teamEventPayloads(events),
	}, nil
}

func (s *teamService) ListTeamTasks(userID, teamID, beforeID, limit int) (*TeamTasksHistoryPayload, error) {
	if _, err := s.requireOwnedTeam(userID, teamID); err != nil {
		return nil, err
	}
	limit = normalizeTeamHistoryLimit(limit, 20, 100)
	tasks, err := s.repo.ListTasksBeforeID(teamID, beforeID, limit+1)
	if err != nil {
		return nil, err
	}
	hasMore := len(tasks) > limit
	if hasMore {
		tasks = tasks[:limit]
	}
	payload := teamTaskPayloads(tasks)
	return &TeamTasksHistoryPayload{
		Tasks:        payload,
		HasMore:      hasMore,
		NextBeforeID: nextTeamTaskBeforeID(payload),
	}, nil
}

func (s *teamService) ListTeamEvents(userID, teamID, beforeID, limit int) (*TeamEventsHistoryPayload, error) {
	if _, err := s.requireOwnedTeam(userID, teamID); err != nil {
		return nil, err
	}
	limit = normalizeTeamHistoryLimit(limit, 50, 200)
	events, err := s.repo.ListEventsBeforeID(teamID, beforeID, limit+1)
	if err != nil {
		return nil, err
	}
	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}
	payload := teamEventPayloads(events)
	return &TeamEventsHistoryPayload{
		Events:       payload,
		HasMore:      hasMore,
		NextBeforeID: nextTeamEventBeforeID(payload),
	}, nil
}

func (s *teamService) ListWorkspaceFiles(ctx context.Context, userID, teamID int, relPath string) (*TeamWorkspaceListPayload, error) {
	team, proxyInstanceID, err := s.workspaceProxyInstance(userID, teamID)
	if err != nil {
		return nil, err
	}
	cleanPath, err := cleanTeamWorkspacePath(relPath)
	if err != nil {
		return nil, err
	}
	target := teamWorkspaceFullPath(team, cleanPath)
	script := fmt.Sprintf(`set -eu
target=%s
if [ ! -d "$target" ]; then exit 44; fi
for entry in "$target"/* "$target"/.[!.]* "$target"/..?*; do
  [ -e "$entry" ] || continue
  name="${entry##*/}"
  [ "$name" = "." ] && continue
  [ "$name" = ".." ] && continue
  if [ -d "$entry" ]; then kind=directory; size=0; else kind=file; size=$(stat -c %%s "$entry" 2>/dev/null || wc -c < "$entry" | tr -d ' '); fi
  mtime=$(stat -c %%Y "$entry" 2>/dev/null || echo 0)
  printf '%%s\t%%s\t%%s\t%%s\n' "$kind" "$name" "$size" "$mtime"
done`, shellQuote(target))
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := s.execTeamWorkspace(ctx, userID, proxyInstanceID, []string{"sh", "-lc", script}, nil, &stdout, &stderr); err != nil {
		return nil, formatExecError("list Team workspace", err, stderr.String())
	}
	entries := parseTeamWorkspaceList(cleanPath, stdout.String())
	return &TeamWorkspaceListPayload{
		Path:    cleanPath,
		Root:    team.SharedMountPath,
		Entries: entries,
	}, nil
}

func (s *teamService) PreviewWorkspaceFile(ctx context.Context, userID, teamID int, relPath string) (*TeamWorkspacePreviewPayload, error) {
	team, proxyInstanceID, err := s.workspaceProxyInstance(userID, teamID)
	if err != nil {
		return nil, err
	}
	cleanPath, err := cleanTeamWorkspacePath(relPath)
	if err != nil {
		return nil, err
	}
	if cleanPath == "" || !isPreviewableWorkspaceFile(cleanPath) {
		return nil, fmt.Errorf("only md and txt files can be previewed")
	}
	target := teamWorkspaceFullPath(team, cleanPath)
	script := fmt.Sprintf(`set -eu
target=%s
[ -f "$target" ] || exit 44
size=$(stat -c %%s "$target" 2>/dev/null || wc -c < "$target" | tr -d ' ')
[ "$size" -le 1048576 ] || exit 45
cat "$target"`, shellQuote(target))
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := s.execTeamWorkspace(ctx, userID, proxyInstanceID, []string{"sh", "-lc", script}, nil, &stdout, &stderr); err != nil {
		return nil, formatExecError("preview Team workspace file", err, stderr.String())
	}
	return &TeamWorkspacePreviewPayload{
		Path:    cleanPath,
		Name:    posixpath.Base(cleanPath),
		Content: stdout.String(),
	}, nil
}

func (s *teamService) DownloadWorkspaceFile(ctx context.Context, userID, teamID int, relPath string) (*TeamWorkspaceDownloadPayload, error) {
	team, proxyInstanceID, err := s.workspaceProxyInstance(userID, teamID)
	if err != nil {
		return nil, err
	}
	cleanPath, err := cleanTeamWorkspacePath(relPath)
	if err != nil {
		return nil, err
	}
	if cleanPath == "" {
		return nil, fmt.Errorf("file path is required")
	}
	target := teamWorkspaceFullPath(team, cleanPath)
	checkScript := fmt.Sprintf(`set -eu
target=%s
if [ -f "$target" ]; then
  printf file
elif [ -d "$target" ]; then
  printf directory
else
  exit 44
fi`, shellQuote(target))
	var checkStdout bytes.Buffer
	var checkStderr bytes.Buffer
	if err := s.execTeamWorkspace(ctx, userID, proxyInstanceID, []string{"sh", "-lc", checkScript}, nil, &checkStdout, &checkStderr); err != nil {
		return nil, formatExecError("inspect Team workspace entry", err, checkStderr.String())
	}
	entryType := strings.TrimSpace(checkStdout.String())
	if entryType == "directory" {
		script := fmt.Sprintf(`set -eu
target=%s
parent=$(dirname "$target")
name=$(basename "$target")
cd "$parent"
if command -v zip >/dev/null 2>&1; then
  zip -rq - "$name"
elif command -v python3 >/dev/null 2>&1; then
  python3 - "$name" <<'PY'
import os
import sys
import zipfile

root = sys.argv[1]
with zipfile.ZipFile(sys.stdout.buffer, "w", zipfile.ZIP_DEFLATED) as archive:
    for current, dirs, files in os.walk(root):
        dirs.sort()
        files.sort()
        if not files and not dirs:
            archive.write(current, current)
        for filename in files:
            path = os.path.join(current, filename)
            archive.write(path, path)
PY
else
  echo "zip or python3 is required to download folders" >&2
  exit 45
fi`, shellQuote(target))
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		if err := s.execTeamWorkspace(ctx, userID, proxyInstanceID, []string{"sh", "-lc", script}, nil, &stdout, &stderr); err != nil {
			return nil, formatExecError("download Team workspace folder", err, stderr.String())
		}
		return &TeamWorkspaceDownloadPayload{
			Path:        cleanPath,
			Name:        posixpath.Base(cleanPath) + ".zip",
			ContentType: "application/zip",
			Data:        stdout.Bytes(),
		}, nil
	}
	if entryType != "file" {
		return nil, fmt.Errorf("unsupported workspace entry type")
	}
	script := fmt.Sprintf(`set -eu
target=%s
[ -f "$target" ] || exit 44
cat "$target"`, shellQuote(target))
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := s.execTeamWorkspace(ctx, userID, proxyInstanceID, []string{"sh", "-lc", script}, nil, &stdout, &stderr); err != nil {
		return nil, formatExecError("download Team workspace file", err, stderr.String())
	}
	return &TeamWorkspaceDownloadPayload{
		Path:        cleanPath,
		Name:        posixpath.Base(cleanPath),
		ContentType: "application/octet-stream",
		Data:        stdout.Bytes(),
	}, nil
}

func (s *teamService) CreateWorkspaceFolder(ctx context.Context, userID, teamID int, req TeamWorkspaceFolderRequest) error {
	team, proxyInstanceID, err := s.workspaceProxyInstance(userID, teamID)
	if err != nil {
		return err
	}
	parent, err := cleanTeamWorkspacePath(req.Path)
	if err != nil {
		return err
	}
	name, err := cleanWorkspaceEntryName(req.Name)
	if err != nil {
		return err
	}
	target := teamWorkspaceFullPath(team, joinTeamWorkspacePath(parent, name))
	script := fmt.Sprintf(`set -eu
target=%s
mkdir -p "$target"
chown -R 1000:1000 "$target" 2>/dev/null || true`, shellQuote(target))
	var stderr bytes.Buffer
	if err := s.execTeamWorkspace(ctx, userID, proxyInstanceID, []string{"sh", "-lc", script}, nil, nil, &stderr); err != nil {
		return formatExecError("create Team workspace folder", err, stderr.String())
	}
	return nil
}

func (s *teamService) RenameWorkspaceEntry(ctx context.Context, userID, teamID int, req TeamWorkspaceRenameRequest) error {
	team, proxyInstanceID, err := s.workspaceProxyInstance(userID, teamID)
	if err != nil {
		return err
	}
	cleanPath, err := cleanTeamWorkspacePath(req.Path)
	if err != nil {
		return err
	}
	if cleanPath == "" {
		return fmt.Errorf("path is required")
	}
	newName, err := cleanWorkspaceEntryName(req.NewName)
	if err != nil {
		return err
	}
	parent := posixpath.Dir(cleanPath)
	if parent == "." {
		parent = ""
	}
	source := teamWorkspaceFullPath(team, cleanPath)
	target := teamWorkspaceFullPath(team, joinTeamWorkspacePath(parent, newName))
	script := fmt.Sprintf(`set -eu
source=%s
target=%s
[ -e "$source" ] || exit 44
[ ! -e "$target" ] || exit 46
mv "$source" "$target"`, shellQuote(source), shellQuote(target))
	var stderr bytes.Buffer
	if err := s.execTeamWorkspace(ctx, userID, proxyInstanceID, []string{"sh", "-lc", script}, nil, nil, &stderr); err != nil {
		return formatExecError("rename Team workspace entry", err, stderr.String())
	}
	return nil
}

func (s *teamService) DeleteWorkspaceEntry(ctx context.Context, userID, teamID int, relPath string) error {
	team, proxyInstanceID, err := s.workspaceProxyInstance(userID, teamID)
	if err != nil {
		return err
	}
	cleanPath, err := cleanTeamWorkspacePath(relPath)
	if err != nil {
		return err
	}
	if cleanPath == "" {
		return fmt.Errorf("cannot delete workspace root")
	}
	target := teamWorkspaceFullPath(team, cleanPath)
	script := fmt.Sprintf(`set -eu
target=%s
[ -e "$target" ] || exit 44
rm -rf -- "$target"`, shellQuote(target))
	var stderr bytes.Buffer
	if err := s.execTeamWorkspace(ctx, userID, proxyInstanceID, []string{"sh", "-lc", script}, nil, nil, &stderr); err != nil {
		return formatExecError("delete Team workspace entry", err, stderr.String())
	}
	return nil
}

func (s *teamService) UploadWorkspaceFiles(ctx context.Context, userID, teamID int, targetPath string, files []*multipart.FileHeader, relativePaths []string) error {
	if len(files) == 0 {
		return fmt.Errorf("no files uploaded")
	}
	team, proxyInstanceID, err := s.workspaceProxyInstance(userID, teamID)
	if err != nil {
		return err
	}
	basePath, err := cleanTeamWorkspacePath(targetPath)
	if err != nil {
		return err
	}
	for index, fileHeader := range files {
		if fileHeader == nil {
			continue
		}
		uploadName := fileHeader.Filename
		if index < len(relativePaths) && strings.TrimSpace(relativePaths[index]) != "" {
			uploadName = relativePaths[index]
		}
		uploadPath, err := cleanTeamWorkspacePath(uploadName)
		if err != nil {
			return err
		}
		if uploadPath == "" {
			return fmt.Errorf("uploaded file name is required")
		}
		destination := teamWorkspaceFullPath(team, joinTeamWorkspacePath(basePath, uploadPath))
		file, err := fileHeader.Open()
		if err != nil {
			return fmt.Errorf("failed to open uploaded file: %w", err)
		}
		script := fmt.Sprintf(`set -eu
target=%s
mkdir -p "$(dirname "$target")"
cat > "$target"
chown 1000:1000 "$target" 2>/dev/null || true`, shellQuote(destination))
		var stderr bytes.Buffer
		execErr := s.execTeamWorkspace(ctx, userID, proxyInstanceID, []string{"sh", "-lc", script}, file, nil, &stderr)
		_ = file.Close()
		if execErr != nil {
			return formatExecError("upload Team workspace file", execErr, stderr.String())
		}
	}
	return nil
}

func (s *teamService) DispatchTask(userID, teamID int, req DispatchTeamTaskRequest) (*TeamTaskPayload, error) {
	team, err := s.requireOwnedTeam(userID, teamID)
	if err != nil {
		return nil, err
	}
	memberKey := strings.TrimSpace(req.TargetMemberID)
	if memberKey == "" {
		members, err := s.repo.ListMembersByTeamID(teamID)
		if err != nil {
			return nil, err
		}
		memberKey = leaderMemberKey(findTeamLeader(activeTeamMembers(members)))
	}
	if memberKey == "" {
		return nil, fmt.Errorf("target member id is required")
	}
	if req.Payload == nil {
		return nil, fmt.Errorf("task payload is required")
	}
	member, err := s.repo.GetMemberByTeamKey(teamID, memberKey)
	if err != nil {
		return nil, err
	}
	if member == nil {
		return nil, fmt.Errorf("team member not found")
	}

	messageID := strings.TrimSpace(req.MessageID)
	if messageID == "" {
		messageID = fmt.Sprintf("team-%d-task-%d", teamID, time.Now().UTC().UnixNano())
	}
	existing, err := s.repo.GetTaskByMessageID(teamID, messageID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if existing.TargetMemberID != member.ID {
			return nil, fmt.Errorf("team task message id already exists")
		}
		if existing.Status != models.TeamTaskStatusPending || existing.RedisStreamID != nil {
			return teamTaskPayload(*existing)
		}
	} else {
		payloadJSON, err := marshalJSON(req.Payload)
		if err != nil {
			return nil, fmt.Errorf("failed to encode task payload: %w", err)
		}
		now := time.Now().UTC()
		existing = &models.TeamTask{
			TeamID:         teamID,
			TargetMemberID: member.ID,
			CreatedBy:      &userID,
			MessageID:      messageID,
			Status:         models.TeamTaskStatusPending,
			PayloadJSON:    payloadJSON,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := s.repo.CreateTask(existing); err != nil {
			return nil, err
		}
	}
	task := existing

	bus, err := s.redisBusForTeam(context.Background(), team)
	if err != nil {
		return nil, err
	}
	taskPayload := map[string]interface{}{}
	if strings.TrimSpace(task.PayloadJSON) != "" {
		if err := json.Unmarshal([]byte(task.PayloadJSON), &taskPayload); err != nil {
			return nil, fmt.Errorf("failed to decode task payload: %w", err)
		}
	}
	now := time.Now().UTC()
	memberInstance, _ := s.teamMemberInstance(member)
	memberContext := buildTeamMemberTaskContext(member, memberInstance)
	envelope := buildTeamTaskEnvelope(teamID, member.MemberKey, task, messageID, taskPayload, memberContext, now)
	envelopeJSON, err := marshalJSON(envelope)
	if err != nil {
		return nil, fmt.Errorf("failed to encode task envelope: %w", err)
	}
	streamID, err := bus.XAdd(context.Background(), teamInboxKey(team.ID, member.MemberKey), map[string]string{
		"payload":    envelopeJSON,
		"team_id":    strconv.Itoa(team.ID),
		"task_id":    strconv.Itoa(task.ID),
		"message_id": messageID,
		"member_id":  member.MemberKey,
	})
	if err != nil {
		return nil, err
	}
	task.Status = models.TeamTaskStatusDispatched
	task.RedisStreamID = &streamID
	task.DispatchedAt = &now
	task.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateTask(task); err != nil {
		return nil, err
	}
	return teamTaskPayload(*task)
}

func (s *teamService) teamMemberInstance(member *models.TeamMember) (*models.Instance, error) {
	if s == nil || s.instanceService == nil || member == nil || member.InstanceID == nil || *member.InstanceID <= 0 {
		return nil, nil
	}
	return s.instanceService.GetByID(*member.InstanceID)
}

func buildTeamMemberTaskContext(member *models.TeamMember, instance *models.Instance) map[string]string {
	if member == nil {
		return map[string]string{}
	}
	displayName := strings.TrimSpace(member.DisplayName)
	if displayName == "" {
		displayName = member.MemberKey
	}
	role := strings.TrimSpace(member.Role)
	if role == "" {
		role = "member"
	}
	description := derefTeamString(member.Description)
	personaSystemPrompt, personaDescription := teamMemberPersonaFromInstance(instance)
	if description == "" {
		description = personaDescription
	}
	systemPrompt := buildTeamMemberSystemPrompt(displayName, member.MemberKey, role, description, personaSystemPrompt)
	return map[string]string{
		"memberId":     member.MemberKey,
		"displayName":  displayName,
		"role":         role,
		"description":  description,
		"systemPrompt": systemPrompt,
	}
}

func buildPlannedTeamMemberTaskContext(member plannedTeamMember) map[string]string {
	displayName := strings.TrimSpace(member.DisplayName)
	if displayName == "" {
		displayName = member.MemberKey
	}
	role := strings.TrimSpace(member.Role)
	if role == "" {
		role = "member"
	}
	description := strings.TrimSpace(derefTeamString(member.Request.Description))
	personaSystemPrompt, personaDescription := teamMemberPersonaFromEnv(member.Request.EnvironmentOverrides)
	if description == "" {
		description = personaDescription
	}
	systemPrompt := buildTeamMemberSystemPrompt(displayName, member.MemberKey, role, description, personaSystemPrompt)
	return map[string]string{
		"memberId":     member.MemberKey,
		"displayName":  displayName,
		"role":         role,
		"description":  description,
		"systemPrompt": systemPrompt,
	}
}

func buildTeamMemberSystemPrompt(displayName, memberID, role, description, personaSystemPrompt string) string {
	systemPrompt := strings.TrimSpace(fmt.Sprintf(
		"You are Team member %q (member_id=%s, role=%s). Follow this role for this task. Role responsibilities: %s",
		displayName,
		memberID,
		role,
		description,
	))
	if strings.TrimSpace(description) == "" {
		systemPrompt = fmt.Sprintf(
			"You are Team member %q (member_id=%s, role=%s). Follow this role for this task.",
			displayName,
			memberID,
			role,
		)
	}
	if personaSystemPrompt != "" {
		systemPrompt = personaSystemPrompt + "\n\n" + systemPrompt
	}
	return systemPrompt
}

func buildTeamMemberSoulMarkdown(member plannedTeamMember) string {
	context := buildPlannedTeamMemberTaskContext(member)
	lines := []string{
		fmt.Sprintf("# %s", strings.TrimSpace(context["displayName"])),
		"",
		"You are running as a ClawManager Team member. Treat this file as persistent identity and role guidance.",
		"",
		"## Team Identity",
		fmt.Sprintf("- Member ID: %s", context["memberId"]),
		fmt.Sprintf("- Display name: %s", context["displayName"]),
		fmt.Sprintf("- Role: %s", context["role"]),
	}
	if description := strings.TrimSpace(context["description"]); description != "" {
		lines = append(lines, fmt.Sprintf("- Responsibilities: %s", description))
	}
	lines = append(lines,
		"",
		"## Role Instructions",
		strings.TrimSpace(context["systemPrompt"]),
		"",
		"## Collaboration Rules",
		"- Only handle tasks addressed to your Team member inbox.",
		"- Use /team for shared context, durable notes, and handoff artifacts.",
		"- Report progress, blockers, verification evidence, and final results through the Team channel.",
		"- If asked about your role, answer from this Team Identity and Role Instructions section.",
		"",
	)
	return strings.Join(lines, "\n")
}

func teamMemberPersonaFromInstance(instance *models.Instance) (string, string) {
	if instance == nil {
		return "", ""
	}
	overrides, err := parseEnvironmentOverridesJSON(instance.EnvironmentOverridesJSON)
	if err != nil || len(overrides) == 0 {
		return "", ""
	}
	return teamMemberPersonaFromEnv(overrides)
}

func teamMemberPersonaFromEnv(overrides map[string]string) (string, string) {
	if len(overrides) == 0 {
		return "", ""
	}
	systemPrompt := strings.TrimSpace(firstNonEmptyEnv(overrides,
		"CLAWMANAGER_AGENT_SYSTEM_PROMPT",
		"CLAWMANAGER_HERMES_SYSTEM_PROMPT",
		"CLAWMANAGER_RUNTIME_SYSTEM_PROMPT",
		"HERMES_SYSTEM_PROMPT",
	))
	description := ""
	for _, key := range []string{
		"CLAWMANAGER_AGENT_PERSONA_JSON",
		"CLAWMANAGER_HERMES_PERSONA_JSON",
		"CLAWMANAGER_RUNTIME_PERSONA_JSON",
	} {
		persona := parseTeamPersonaEnv(overrides[key])
		if persona == nil {
			continue
		}
		if systemPrompt == "" {
			systemPrompt = strings.TrimSpace(persona.SystemPrompt)
		}
		if description == "" {
			description = strings.TrimSpace(persona.Summary)
		}
	}
	for _, key := range []string{
		"CLAWMANAGER_HERMES_AGENTS_JSON",
		"CLAWMANAGER_RUNTIME_AGENTS_JSON",
		"CLAWMANAGER_OPENCLAW_AGENTS_JSON",
	} {
		agents := parseTeamAgentsEnv(overrides[key])
		if agents == nil {
			continue
		}
		if systemPrompt == "" {
			systemPrompt = strings.TrimSpace(agents.SystemPrompt)
		}
		if description == "" {
			description = strings.TrimSpace(agents.Summary)
		}
	}
	return systemPrompt, description
}

type teamPersonaEnv struct {
	SystemPrompt string `json:"systemPrompt"`
	Summary      string `json:"summary"`
}

func parseTeamPersonaEnv(raw string) *teamPersonaEnv {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var persona teamPersonaEnv
	if err := json.Unmarshal([]byte(raw), &persona); err != nil {
		return nil
	}
	return &persona
}

type teamAgentsEnv struct {
	Items []struct {
		Content struct {
			Config struct {
				SystemPrompt string `json:"systemPrompt"`
				Summary      string `json:"summary"`
			} `json:"config"`
		} `json:"content"`
	} `json:"items"`
}

func parseTeamAgentsEnv(raw string) *teamPersonaEnv {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var payload teamAgentsEnv
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil
	}
	for _, item := range payload.Items {
		systemPrompt := strings.TrimSpace(item.Content.Config.SystemPrompt)
		summary := strings.TrimSpace(item.Content.Config.Summary)
		if systemPrompt != "" || summary != "" {
			return &teamPersonaEnv{SystemPrompt: systemPrompt, Summary: summary}
		}
	}
	return nil
}

func firstNonEmptyEnv(values map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(values[key]); value != "" {
			return value
		}
	}
	return ""
}

func buildTeamRuntimePrompt(rawPrompt string, memberContext map[string]string) string {
	prompt := strings.TrimSpace(rawPrompt)
	if len(memberContext) == 0 {
		return prompt
	}
	systemPrompt := strings.TrimSpace(memberContext["systemPrompt"])
	if systemPrompt == "" {
		return prompt
	}
	if prompt == "" {
		return systemPrompt
	}
	return fmt.Sprintf("%s\n\nUser task:\n%s", systemPrompt, prompt)
}

func (s *teamService) DeleteTeam(userID, teamID int) error {
	team, err := s.requireOwnedTeam(userID, teamID)
	if err != nil {
		return err
	}
	if team.Status == models.TeamStatusDeleted {
		return nil
	}

	now := time.Now().UTC()
	team.Status = models.TeamStatusDeleting
	team.UpdatedAt = now
	if err := s.repo.UpdateTeam(team); err != nil {
		return err
	}

	members, err := s.repo.ListMembersByTeamID(teamID)
	if err != nil {
		return err
	}
	for idx := range members {
		member := members[idx]
		if member.Status == models.TeamMemberStatusDeleted {
			continue
		}
		member.Status = models.TeamMemberStatusDeleting
		member.UpdatedAt = time.Now().UTC()
		_ = s.repo.UpdateMember(&member)
		if member.InstanceID != nil && *member.InstanceID > 0 {
			if err := s.instanceService.Delete(*member.InstanceID); err != nil {
				fmt.Printf("Warning: failed to delete Team %d member %s instance %d: %v\n", teamID, member.MemberKey, *member.InstanceID, err)
			}
		}
		member.Status = models.TeamMemberStatusDeleted
		member.CurrentTaskID = nil
		member.UpdatedAt = time.Now().UTC()
		_ = s.repo.UpdateMember(&member)
	}

	ctx := context.Background()
	if strings.TrimSpace(derefTeamString(team.TeamTokenSecretName)) != "" {
		if err := s.secretService.DeleteSecret(ctx, userID, derefTeamString(team.TeamTokenSecretName)); err != nil {
			fmt.Printf("Warning: failed to delete Team %d secret: %v\n", teamID, err)
		}
	}
	if err := s.configMapService.DeleteConfigMap(ctx, userID, s.teamConfigMapName(teamID)); err != nil {
		fmt.Printf("Warning: failed to delete Team %d configmap: %v\n", teamID, err)
	}
	if err := s.pvcService.DeleteTeamSharedPVC(ctx, userID, teamID); err != nil {
		fmt.Printf("Warning: failed to delete Team %d shared PVC: %v\n", teamID, err)
	}

	team.Name = deletedTeamName(team.Name, team.ID)
	team.Status = models.TeamStatusDeleted
	team.UpdatedAt = time.Now().UTC()
	return s.repo.UpdateTeam(team)
}

func (s *teamService) DeleteMember(userID, teamID int, memberID string) error {
	team, err := s.requireOwnedTeam(userID, teamID)
	if err != nil {
		return err
	}
	member, err := s.findTeamMemberForDelete(teamID, memberID)
	if err != nil {
		return err
	}
	if member == nil {
		return fmt.Errorf("team member not found")
	}
	if member.UserID != userID || member.TeamID != teamID {
		return fmt.Errorf("access denied")
	}
	if member.Status == models.TeamMemberStatusDeleted {
		return nil
	}
	if isTeamLeaderRole(member.Role) {
		return fmt.Errorf("team leader cannot be deleted before assigning a new leader")
	}

	now := time.Now().UTC()
	member.Status = models.TeamMemberStatusDeleting
	member.UpdatedAt = now
	if err := s.repo.UpdateMember(member); err != nil {
		return err
	}
	if member.InstanceID != nil && *member.InstanceID > 0 {
		if err := s.instanceService.Delete(*member.InstanceID); err != nil {
			return err
		}
	}
	member.Status = models.TeamMemberStatusDeleted
	member.CurrentTaskID = nil
	member.Progress = 0
	member.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateMember(member); err != nil {
		return err
	}
	return s.refreshTeamRosterConfig(userID, team)
}

func (s *teamService) findTeamMemberForDelete(teamID int, memberID string) (*models.TeamMember, error) {
	value := strings.TrimSpace(memberID)
	if value == "" {
		return nil, fmt.Errorf("team member id is required")
	}
	if numericID, err := strconv.Atoi(value); err == nil && numericID > 0 {
		member, err := s.repo.GetMemberByID(numericID)
		if err != nil || member == nil || member.TeamID != teamID {
			return member, err
		}
		return member, nil
	}
	return s.repo.GetMemberByTeamKey(teamID, value)
}

func (s *teamService) refreshTeamRosterConfig(userID int, team *models.Team) error {
	members, err := s.repo.ListMembersByTeamID(team.ID)
	if err != nil {
		return err
	}
	rosterJSON, err := buildTeamRosterConfigFromMembers(team, activeTeamMembers(members))
	if err != nil {
		return err
	}
	configData := map[string]string{
		teamConfigFileName: rosterJSON,
	}
	for _, member := range activeTeamMembers(members) {
		if member.RuntimeType != "hermes" {
			continue
		}
		configData[teamMemberSoulConfigKey(member.MemberKey)] = buildTeamMemberSoulMarkdown(plannedTeamMember{
			Request: CreateTeamMemberRequest{
				Description: member.Description,
			},
			MemberKey:    member.MemberKey,
			DisplayName:  member.DisplayName,
			Role:         member.Role,
			RuntimeType:  member.RuntimeType,
			InstanceMode: member.InstanceMode,
			IsLeader:     isTeamLeaderRole(member.Role),
		})
	}
	return s.configMapService.UpsertConfigMap(context.Background(), userID, s.teamConfigMapName(team.ID), configData, map[string]string{
		"app":        "clawreef",
		"managed-by": "clawreef",
		"team-id":    strconv.Itoa(team.ID),
	})
}

func (s *teamService) requireOwnedTeam(userID, teamID int) (*models.Team, error) {
	team, err := s.repo.GetTeamByID(teamID)
	if err != nil {
		return nil, err
	}
	if team == nil {
		return nil, fmt.Errorf("team not found")
	}
	if team.UserID != userID {
		return nil, fmt.Errorf("access denied")
	}
	return team, nil
}

func (s *teamService) ensureConsumer(teamID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.consumers[teamID]; exists {
		return
	}
	s.consumers[teamID] = struct{}{}
	go s.consumeTeamEvents(teamID)
}

func (s *teamService) consumeTeamEvents(teamID int) {
	defer func() {
		s.mu.Lock()
		delete(s.consumers, teamID)
		s.mu.Unlock()
	}()

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		team, err := s.repo.GetTeamByID(teamID)
		if err != nil || team == nil {
			time.Sleep(5 * time.Second)
			continue
		}
		bus, err := s.redisBusForTeam(s.ctx, team)
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}
		lastID := strings.TrimSpace(team.RedisEventsLastID)
		if lastID == "" {
			lastID = "0-0"
		}
		messages, err := bus.XRead(s.ctx, teamEventsKey(teamID), lastID, 5*time.Second)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		for _, message := range messages {
			if err := s.projectTeamEvent(team, bus, message); err != nil {
				fmt.Printf("Warning: failed to project Team %d event %s: %v\n", teamID, message.ID, err)
			}
			team.RedisEventsLastID = message.ID
			team.UpdatedAt = time.Now().UTC()
			_ = s.repo.UpdateTeam(team)
		}
	}
}

func (s *teamService) ensureStaleTaskMonitor() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.staleMonitorStarted {
		return
	}
	s.staleMonitorStarted = true
	go s.monitorStaleTasks()
}

func (s *teamService) monitorStaleTasks() {
	ticker := time.NewTicker(teamTaskStaleSweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			if err := s.sweepStaleTasks(); err != nil {
				fmt.Printf("Warning: failed to sweep stale Team tasks: %v\n", err)
			}
		}
	}
}

func (s *teamService) sweepStaleTasks() error {
	timeout := teamTaskStaleTimeout()
	if timeout <= 0 {
		return nil
	}
	cutoff := time.Now().UTC().Add(-timeout)
	tasks, err := s.repo.ListStaleCandidateTasks(cutoff, 100)
	if err != nil {
		return err
	}
	for idx := range tasks {
		if err := s.markTaskStale(&tasks[idx], timeout); err != nil {
			fmt.Printf("Warning: failed to mark Team task %d stale: %v\n", tasks[idx].ID, err)
		}
	}
	return nil
}

func (s *teamService) markTaskStale(task *models.TeamTask, timeout time.Duration) error {
	if task == nil {
		return nil
	}
	if task.Status != models.TeamTaskStatusDispatched && task.Status != models.TeamTaskStatusRunning {
		return nil
	}
	lastUpdatedAt := task.UpdatedAt
	team, err := s.repo.GetTeamByID(task.TeamID)
	if err != nil {
		return err
	}
	if team == nil || team.Status == models.TeamStatusDeleted || team.Status == models.TeamStatusDeleting {
		return nil
	}

	now := time.Now().UTC()
	previousStatus := task.Status
	task.Status = models.TeamTaskStatusStale
	task.FinishedAt = &now
	message := fmt.Sprintf("Team task stale: no runtime event for %s since %s", timeout.String(), task.UpdatedAt.Format(time.RFC3339))
	task.ErrorMessage = &message
	task.UpdatedAt = now
	if err := s.repo.UpdateTask(task); err != nil {
		return err
	}

	member, err := s.repo.GetMemberByID(task.TargetMemberID)
	if err != nil {
		return err
	}
	if member != nil && member.TeamID == task.TeamID && member.CurrentTaskID != nil && *member.CurrentTaskID == task.ID {
		member.Status = models.TeamMemberStatusIdle
		member.CurrentTaskID = nil
		member.Availability = models.TeamMemberAvailabilityBlocked
		member.RuntimeTaskID = &task.MessageID
		member.RuntimeIntent = nil
		member.BlockedReason = &message
		member.LastSummary = &message
		member.Progress = 0
		member.UpdatedAt = now
		if err := s.repo.UpdateMember(member); err != nil {
			return err
		}
	}

	payload := map[string]interface{}{
		"v":                 1,
		"event":             "task_stale",
		"teamId":            strconv.Itoa(task.TeamID),
		"taskId":            fmt.Sprintf("team-%d-task-%d", task.TeamID, task.ID),
		"messageId":         task.MessageID,
		"previousStatus":    previousStatus,
		"staleAfterSeconds": int(timeout.Seconds()),
		"lastTaskUpdatedAt": lastUpdatedAt.Format(time.RFC3339Nano),
		"diagnostic":        message,
		"source":            "clawmanager",
	}
	payloadJSON, err := marshalOptionalJSON(payload)
	if err != nil {
		return err
	}
	event := &models.TeamEvent{
		TeamID:      task.TeamID,
		TaskID:      &task.ID,
		EventType:   "task_stale",
		MessageID:   &task.MessageID,
		PayloadJSON: payloadJSON,
		OccurredAt:  &now,
		CreatedAt:   now,
	}
	if member != nil && member.TeamID == task.TeamID {
		event.MemberID = &member.ID
	}
	return s.repo.CreateEvent(event)
}

func (s *teamService) redisBusForTeam(ctx context.Context, team *models.Team) (*redisBus, error) {
	redisURL := ""
	if team.RedisURLSecretName != nil && team.RedisURLSecretKey != nil {
		client := k8s.GetClient()
		if client == nil {
			return nil, fmt.Errorf("k8s client not initialized")
		}
		value, err := s.secretService.GetSecretValue(ctx, client.GetNamespace(team.UserID), *team.RedisURLSecretName, *team.RedisURLSecretKey)
		if err != nil {
			return nil, err
		}
		redisURL = strings.TrimSpace(value)
	}
	if redisURL == "" {
		redisURL = defaultTeamRedisURL()
	}
	if redisURL == "" {
		return nil, fmt.Errorf("team redis url is required")
	}
	return newRedisBus(redisURL)
}

type teamTaskProjectionResult struct {
	status  string
	changed bool
}

func projectTeamTaskRuntimeState(task *models.TeamTask, payload map[string]interface{}, eventType string, payloadJSON *string, now time.Time) teamTaskProjectionResult {
	if task == nil {
		return teamTaskProjectionResult{}
	}
	status := normalizedTeamTaskEventStatus(payload)
	completed := isTeamTaskCompletionSignal(eventType, status, payload)
	failed := isTeamTaskFailureSignal(eventType, status, payload)
	running := isTeamTaskRunningSignal(eventType, status, payload)

	result := teamTaskProjectionResult{}
	setStatus := func(next string) {
		result.status = next
		if task.Status != next {
			task.Status = next
			result.changed = true
		}
	}
	setStarted := func() {
		if task.StartedAt == nil {
			task.StartedAt = &now
			result.changed = true
		}
	}
	setFinished := func() {
		if task.FinishedAt == nil || !task.FinishedAt.Equal(now) {
			task.FinishedAt = &now
			result.changed = true
		}
	}

	if completed {
		setStatus(models.TeamTaskStatusSucceeded)
		setFinished()
		if payloadJSON != nil && (task.ResultJSON == nil || *task.ResultJSON != *payloadJSON) {
			task.ResultJSON = payloadJSON
			result.changed = true
		}
		if task.ErrorMessage != nil {
			task.ErrorMessage = nil
			result.changed = true
		}
		return result
	}

	if failed && task.Status != models.TeamTaskStatusSucceeded {
		setStatus(models.TeamTaskStatusFailed)
		setFinished()
		if errText := eventString(payload, "error_message", "error", "reason", "diagnostic", "lastSummary", "last_summary", "summary"); errText != "" {
			if task.ErrorMessage == nil || *task.ErrorMessage != errText {
				task.ErrorMessage = &errText
				result.changed = true
			}
		}
		return result
	}

	if isTerminalTeamTaskStatus(task.Status) {
		return result
	}

	switch eventType {
	case "task_received":
		if task.Status == models.TeamTaskStatusPending {
			setStatus(models.TeamTaskStatusDispatched)
		}
	case "task_started":
		setStatus(models.TeamTaskStatusRunning)
		setStarted()
	default:
		if running {
			setStatus(models.TeamTaskStatusRunning)
			setStarted()
		}
	}
	return result
}

func normalizedTeamTaskEventStatus(payload map[string]interface{}) string {
	raw := eventString(payload, "task_status", "taskStatus", "result_status", "resultStatus", "status", "state")
	raw = strings.ToLower(strings.TrimSpace(raw))
	raw = strings.ReplaceAll(raw, "-", "_")
	raw = strings.ReplaceAll(raw, " ", "_")
	return raw
}

func isTeamTaskCompletionSignal(eventType, status string, payload map[string]interface{}) bool {
	switch eventType {
	case "task_completed", "completion":
		return true
	}
	if !hasTeamTaskCompletionToolCall(payload) {
		return false
	}
	switch status {
	case "succeeded", "success", "completed", "complete", "done", "finished", "ok":
		return true
	default:
		return false
	}
}

func isTeamTaskFailureSignal(eventType, status string, payload map[string]interface{}) bool {
	switch eventType {
	case "task_failed", "message_failed":
		return true
	}
	if !hasTeamTaskCompletionToolCall(payload) {
		return false
	}
	switch status {
	case "failed", "failure", "error", "errored", "blocked":
		return true
	default:
		return false
	}
}

func isTeamTaskRunningSignal(eventType, status string, payload map[string]interface{}) bool {
	switch eventType {
	case "task_started", "task_progress", "progress":
		return true
	}
	switch status {
	case "running", "in_progress", "processing", "busy", "working":
		return true
	}
	progress := eventInt(payload, "progress")
	return progress > 0 && progress < 100
}

func isTerminalTeamTaskStatus(status string) bool {
	return status == models.TeamTaskStatusSucceeded ||
		status == models.TeamTaskStatusFailed ||
		status == models.TeamTaskStatusStale
}

func shouldAssociateEventWithCurrentMemberTask(eventType string, payload map[string]interface{}) bool {
	switch eventType {
	case "reply", "completion", "task_completed", "task_failed", "message_failed", "message_warning", "task_started", "task_progress", "progress":
		return true
	}
	if eventString(payload, "taskId", "task_id", "runtimeTaskId", "runtime_task_id", "messageId", "message_id") != "" {
		return true
	}
	return teamEventHasBody(payload)
}

func isNonAuthoritativeDispatchFailure(eventType string, payload map[string]interface{}) bool {
	if eventType != "task_failed" && eventType != "message_failed" {
		return false
	}
	text := strings.ToLower(strings.Join(strings.Fields(strings.Join([]string{
		eventString(payload, "error_message", "error", "reason", "diagnostic", "lastSummary", "last_summary", "summary", "text", "message"),
	}, " ")), " "))
	return strings.Contains(text, "dispatch finished without reply/completion") ||
		strings.Contains(text, "without reply/completion")
}

func isNonAuthoritativeDispatchWarning(eventType string, payload map[string]interface{}) bool {
	return eventType == "message_warning" && isNonAuthoritativeDispatchFailure(eventString(payload, "originalEvent"), payload)
}

func (s *teamService) projectTeamEvent(team *models.Team, bus *redisBus, message redisStreamMessage) error {
	if exists, err := s.repo.EventExistsByStreamID(team.ID, message.ID); err != nil || exists {
		return err
	}
	payload := mergeRedisEventPayload(message.Fields)
	eventType := eventString(payload, "event_type", "event", "type")
	if eventType == "" {
		eventType = "message"
	}
	messageID := eventString(payload, "message_id", "messageId")
	memberKey := eventString(payload, "member_id", "memberId", "member_key")
	if isOutboundTeamEvent(eventType) && messageID != "" && !teamEventHasBody(payload) && bus != nil {
		enriched, err := s.enrichOutboundEventFromInbox(team.ID, bus, payload, messageID)
		if err != nil {
			fmt.Printf("Warning: failed to enrich Team %d outbound event %s from inbox: %v\n", team.ID, messageID, err)
		} else {
			payload = enriched
		}
	}

	var member *models.TeamMember
	if memberKey != "" {
		found, err := s.repo.GetMemberByTeamKey(team.ID, memberKey)
		if err != nil {
			return err
		}
		member = found
	}

	var task *models.TeamTask
	if taskID := eventInt(payload, "task_id", "taskId"); taskID > 0 {
		found, err := s.repo.GetTaskByID(taskID)
		if err != nil {
			return err
		}
		if found != nil && found.TeamID == team.ID {
			task = found
		}
	}
	if task == nil && messageID != "" {
		found, err := s.repo.GetTaskByMessageID(team.ID, messageID)
		if err != nil {
			return err
		}
		task = found
	}
	if task == nil && member != nil && member.CurrentTaskID != nil && shouldAssociateEventWithCurrentMemberTask(eventType, payload) {
		found, err := s.repo.GetTaskByID(*member.CurrentTaskID)
		if err != nil {
			return err
		}
		if found != nil && found.TeamID == team.ID && found.TargetMemberID == member.ID && !isTerminalTeamTaskStatus(found.Status) {
			task = found
		}
	}
	if isNonAuthoritativeDispatchFailure(eventType, payload) {
		if eventString(payload, "originalEvent") == "" {
			payload["originalEvent"] = eventType
		}
		payload["event"] = "message_warning"
		payload["type"] = "message_warning"
		payload["status"] = "warning"
		eventType = "message_warning"
	}
	eventType = normalizeFinalReplyTaskEvent(eventType, payload, task, member)

	payloadJSON, err := marshalOptionalJSON(payload)
	if err != nil {
		return err
	}
	streamID := message.ID
	event := &models.TeamEvent{
		TeamID:        team.ID,
		EventType:     eventType,
		PayloadJSON:   payloadJSON,
		RedisStreamID: &streamID,
		OccurredAt:    eventTime(payload),
	}
	if member != nil {
		event.MemberID = &member.ID
	}
	if task != nil {
		event.TaskID = &task.ID
	}
	if messageID != "" {
		event.MessageID = &messageID
	}
	if err := s.repo.CreateEvent(event); err != nil {
		return err
	}

	now := time.Now().UTC()
	taskProjection := teamTaskProjectionResult{}
	if task != nil {
		taskProjection = projectTeamTaskRuntimeState(task, payload, eventType, payloadJSON, now)
		if taskProjection.changed {
			task.UpdatedAt = now
			if err := s.repo.UpdateTask(task); err != nil {
				return err
			}
		}
	}
	if member != nil {
		member.LastSeenAt = &now
		applyTeamMemberRuntimeProjection(member, payload, eventType)
		taskIsActive := task != nil && !isTerminalTeamTaskStatus(task.Status)
		if taskIsActive && (eventType == "task_received" || eventType == "task_started" || taskProjection.status == models.TeamTaskStatusRunning || taskProjection.status == models.TeamTaskStatusDispatched) {
			member.Status = models.TeamMemberStatusBusy
			if member.Availability == "" || member.Availability == models.TeamMemberAvailabilityUnknown {
				member.Availability = models.TeamMemberAvailabilityBusy
			}
			member.CurrentTaskID = &task.ID
			member.Progress = eventInt(payload, "progress")
		}
		taskProjectedTerminal := taskProjection.status == models.TeamTaskStatusSucceeded || taskProjection.status == models.TeamTaskStatusFailed
		terminalEventWithoutTask := task == nil && (eventType == "task_completed" || eventType == "completion" || eventType == "task_failed" || eventType == "message_failed")
		if taskProjectedTerminal || terminalEventWithoutTask {
			member.Status = models.TeamMemberStatusIdle
			member.CurrentTaskID = nil
			if taskProjection.status == models.TeamTaskStatusSucceeded || (task == nil && (eventType == "task_completed" || eventType == "completion")) {
				member.Progress = 100
				if member.Availability != models.TeamMemberAvailabilityBlocked {
					member.Availability = models.TeamMemberAvailabilityIdle
					member.BlockedReason = nil
				}
			} else {
				member.Progress = 0
				if member.Availability == "" || member.Availability == models.TeamMemberAvailabilityUnknown {
					member.Availability = models.TeamMemberAvailabilityBlocked
				}
				if member.BlockedReason == nil {
					if errText := eventString(payload, "error_message", "error", "reason", "diagnostic", "lastSummary", "last_summary"); errText != "" {
						member.BlockedReason = &errText
					}
				}
			}
		}
		if task != nil && task.Status == models.TeamTaskStatusSucceeded {
			member.Status = models.TeamMemberStatusIdle
			member.CurrentTaskID = nil
			member.Progress = 100
			member.Availability = models.TeamMemberAvailabilityIdle
			member.BlockedReason = nil
		}
		member.UpdatedAt = now
		if err := s.repo.UpdateMember(member); err != nil {
			return err
		}
	}
	return nil
}

func normalizeFinalReplyTaskEvent(eventType string, payload map[string]interface{}, task *models.TeamTask, member *models.TeamMember) string {
	if task == nil || !strings.EqualFold(strings.TrimSpace(eventType), "reply") {
		return eventType
	}
	hasCompletionTool := hasTeamTaskCompletionToolCall(payload)
	hasImplicitDirectCompletion := isImplicitDirectTaskCompletionReply(task, member, payload)
	if !hasCompletionTool && !hasImplicitDirectCompletion {
		return eventType
	}
	if hasCompletionTool && !eventBool(payload, "final", "isFinal", "complete", "completed", "taskCompleted") {
		return eventType
	}
	if !teamEventHasBody(payload) {
		return eventType
	}
	payload["originalEvent"] = eventType
	payload["event"] = "task_completed"
	payload["type"] = "task_completed"
	payload["status"] = "succeeded"
	payload["availability"] = models.TeamMemberAvailabilityIdle
	payload["runtimeStatus"] = "succeeded"
	if eventString(payload, "resultMarkdown") == "" {
		if text := eventString(payload, "text", "result", "summary"); text != "" {
			payload["resultMarkdown"] = text
		}
	}
	if eventString(payload, "summary") == "" {
		if text := eventString(payload, "text", "resultMarkdown", "result"); text != "" {
			payload["summary"] = text
		}
	}
	return "task_completed"
}

func isImplicitDirectTaskCompletionReply(task *models.TeamTask, member *models.TeamMember, payload map[string]interface{}) bool {
	if task == nil || member == nil || payload == nil {
		return false
	}
	if member.ID != task.TargetMemberID {
		return false
	}
	status := normalizedTeamTaskEventStatus(payload)
	if isTeamTaskFailureSignal("reply", status, payload) || isTeamTaskRunningSignal("reply", status, payload) {
		return false
	}
	resultText := directTaskCompletionReplyText(payload)
	if resultText == "" || isInterimOrDelegationReplyText(resultText) {
		return false
	}
	if eventBool(payload, "final", "isFinal", "complete", "completed", "taskCompleted") {
		return true
	}
	if eventString(payload, "resultMarkdown", "result_markdown", "result", "answer") != "" {
		return true
	}
	switch status {
	case "succeeded", "success", "completed", "complete", "done", "finished", "ok":
		return true
	}
	compact := strings.TrimSpace(resultText)
	compact = strings.Join(strings.Fields(compact), "")
	return len([]rune(compact)) >= 36 && looksLikeSubstantialFinalReply(resultText)
}

func directTaskCompletionReplyText(payload map[string]interface{}) string {
	for _, record := range eventRecordCandidates(payload) {
		if text := eventString(record, "resultMarkdown", "result_markdown", "result", "answer", "text", "message", "summary"); text != "" {
			return text
		}
	}
	return ""
}

func isInterimOrDelegationReplyText(text string) bool {
	normalized := strings.TrimSpace(text)
	if normalized == "" {
		return true
	}
	lower := strings.ToLower(normalized)
	compact := strings.ToLower(strings.Join(strings.Fields(normalized), ""))
	if len([]rune(compact)) <= 12 {
		return true
	}
	for _, prefix := range []string{
		"收到", "好的", "好，", "好,", "ok", "okay", "处理中", "正在", "准备", "我将", "让我", "先看", "稍等",
		"let me", "i will", "i'll", "checking", "working on",
	} {
		if strings.HasPrefix(lower, prefix) || strings.HasPrefix(normalized, prefix) {
			return true
		}
	}
	for _, marker := range []string{
		"正在整理", "现在整理", "稍后", "等待其", "等待他", "等待她", "等待worker", "等待 worker",
		"派单", "已派发", "派发给", "下发给", "转派给", "交给worker", "交给 worker",
		"让worker", "让 worker", "请worker", "请 worker", "worker在线空闲",
		"sent to worker", "assigned to worker", "waiting for worker", "handoff to worker",
	} {
		if strings.Contains(compact, strings.ReplaceAll(strings.ToLower(marker), " ", "")) || strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func looksLikeSubstantialFinalReply(text string) bool {
	normalized := strings.TrimSpace(text)
	if normalized == "" {
		return false
	}
	if strings.ContainsAny(normalized, "#*>|`") {
		return true
	}
	if strings.ContainsAny(normalized, "。；;：:\n") {
		return true
	}
	return strings.Contains(normalized, "完成") ||
		strings.Contains(normalized, "总结") ||
		strings.Contains(normalized, "报告") ||
		strings.Contains(normalized, "结果") ||
		strings.Contains(strings.ToLower(normalized), "completed") ||
		strings.Contains(strings.ToLower(normalized), "summary")
}

func hasTeamTaskCompletionToolCall(payload map[string]interface{}) bool {
	if payload == nil {
		return false
	}
	if eventString(payload, "toolCallName", "tool_call_name", "calledTool", "called_tool") == teamTaskCompletionTool {
		return true
	}
	for _, key := range []string{"toolCall", "tool_call", "function_call"} {
		record, ok := payload[key].(map[string]interface{})
		if !ok || record == nil {
			continue
		}
		if eventString(record, "name", "function", "functionName", "function_name", "tool", "toolName", "tool_name") == teamTaskCompletionTool {
			return true
		}
	}
	return false
}

func eventRecordCandidates(payload map[string]interface{}) []map[string]interface{} {
	if payload == nil {
		return nil
	}
	records := []map[string]interface{}{payload}
	for _, key := range []string{"sent", "metadata", "data", "envelope", "task", "toolCall", "tool_call", "function_call"} {
		if record, ok := payload[key].(map[string]interface{}); ok && record != nil {
			records = append(records, record)
		}
	}
	return records
}

func (s *teamService) enrichOutboundEventFromInbox(teamID int, bus *redisBus, payload map[string]interface{}, messageID string) (map[string]interface{}, error) {
	targetMember := eventString(payload, "to", "recipient", "target", "targetMemberId", "target_member_id")
	if targetMember == "" {
		return payload, nil
	}
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			time.Sleep(100 * time.Millisecond)
		}
		messages, err := bus.XRevRange(context.Background(), teamInboxKey(teamID, targetMember), 100)
		if err != nil {
			lastErr = err
			continue
		}
		for _, inboxMessage := range messages {
			if !redisStreamMessageMatches(inboxMessage, messageID) {
				continue
			}
			envelope := mergeRedisEventPayload(inboxMessage.Fields)
			return mergeMissingEventFields(payload, envelope), nil
		}
	}
	return payload, lastErr
}

func redisStreamMessageMatches(message redisStreamMessage, messageID string) bool {
	if strings.TrimSpace(message.Fields["message_id"]) == messageID {
		return true
	}
	payload := mergeRedisEventPayload(message.Fields)
	return eventString(payload, "message_id", "messageId") == messageID
}

func mergeMissingEventFields(base map[string]interface{}, extra map[string]interface{}) map[string]interface{} {
	merged := map[string]interface{}{}
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range extra {
		if existing, ok := merged[key]; !ok || isEmptyEventValue(existing) {
			merged[key] = value
		}
	}
	if metadata, ok := extra["metadata"].(map[string]interface{}); ok {
		for key, value := range metadata {
			if existing, ok := merged[key]; !ok || isEmptyEventValue(existing) {
				merged[key] = value
			}
		}
	}
	return merged
}

func isEmptyEventValue(value interface{}) bool {
	if value == nil {
		return true
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text) == ""
	}
	return false
}

func isOutboundTeamEvent(eventType string) bool {
	switch eventType {
	case "outbound", "task_assigned":
		return true
	default:
		return false
	}
}

func teamEventHasBody(payload map[string]interface{}) bool {
	if eventString(payload, "text", "title", "prompt", "instruction", "instructions", "summary", "resultMarkdown") != "" {
		return true
	}
	for idx, record := range eventRecordCandidates(payload) {
		if idx == 0 {
			continue
		}
		if eventString(record, "text", "title", "prompt", "instruction", "instructions", "summary", "resultMarkdown") != "" {
			return true
		}
	}
	return false
}

func (s *teamService) markTeamFailed(team *models.Team, cause error) error {
	team.Status = models.TeamStatusFailed
	team.UpdatedAt = time.Now().UTC()
	_ = s.repo.UpdateTeam(team)
	return cause
}

func (s *teamService) rollbackTeamCreation(userID int, team *models.Team, cause error) error {
	members, err := s.repo.ListMembersByTeamID(team.ID)
	if err != nil {
		fmt.Printf("Warning: failed to list Team %d members during create rollback: %v\n", team.ID, err)
	}
	for idx := range members {
		member := members[idx]
		if member.InstanceID != nil && *member.InstanceID > 0 {
			if err := s.instanceService.Delete(*member.InstanceID); err != nil {
				fmt.Printf("Warning: failed to delete Team %d member %s instance %d during create rollback: %v\n", team.ID, member.MemberKey, *member.InstanceID, err)
			}
		}
		member.Status = models.TeamMemberStatusDeleted
		member.CurrentTaskID = nil
		member.UpdatedAt = time.Now().UTC()
		_ = s.repo.UpdateMember(&member)
	}

	ctx := context.Background()
	if strings.TrimSpace(derefTeamString(team.TeamTokenSecretName)) != "" {
		if err := s.secretService.DeleteSecret(ctx, userID, derefTeamString(team.TeamTokenSecretName)); err != nil {
			fmt.Printf("Warning: failed to delete Team %d secret during create rollback: %v\n", team.ID, err)
		}
	}
	if err := s.configMapService.DeleteConfigMap(ctx, userID, s.teamConfigMapName(team.ID)); err != nil {
		fmt.Printf("Warning: failed to delete Team %d configmap during create rollback: %v\n", team.ID, err)
	}
	if err := s.pvcService.DeleteTeamSharedPVC(ctx, userID, team.ID); err != nil {
		fmt.Printf("Warning: failed to delete Team %d shared PVC during create rollback: %v\n", team.ID, err)
	}

	team.Name = deletedTeamName(team.Name, team.ID)
	team.Status = models.TeamStatusDeleted
	team.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateTeam(team); err != nil {
		fmt.Printf("Warning: failed to mark Team %d deleted during create rollback: %v\n", team.ID, err)
	}
	return cause
}

func teamTaskPayloads(tasks []models.TeamTask) []TeamTaskPayload {
	result := make([]TeamTaskPayload, 0, len(tasks))
	for _, task := range tasks {
		if payload, err := teamTaskPayload(task); err == nil {
			result = append(result, *payload)
		}
	}
	return result
}

func normalizeTeamHistoryLimit(limit, defaultLimit, maxLimit int) int {
	if limit <= 0 {
		return defaultLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}

func nextTeamTaskBeforeID(tasks []TeamTaskPayload) *int {
	if len(tasks) == 0 {
		return nil
	}
	next := tasks[len(tasks)-1].ID
	return &next
}

func nextTeamEventBeforeID(events []TeamEventPayload) *int {
	if len(events) == 0 {
		return nil
	}
	next := events[len(events)-1].ID
	return &next
}

func teamTaskPayload(task models.TeamTask) (*TeamTaskPayload, error) {
	payload := &TeamTaskPayload{TeamTask: task}
	if strings.TrimSpace(task.PayloadJSON) != "" {
		if err := json.Unmarshal([]byte(task.PayloadJSON), &payload.Payload); err != nil {
			return nil, err
		}
	}
	if task.ResultJSON != nil && strings.TrimSpace(*task.ResultJSON) != "" {
		if err := json.Unmarshal([]byte(*task.ResultJSON), &payload.Result); err != nil {
			return nil, err
		}
	}
	return payload, nil
}

func buildInitialLeaderTaskPayload(teamName string) map[string]interface{} {
	normalizedTeamName := strings.TrimSpace(teamName)
	if normalizedTeamName == "" {
		normalizedTeamName = "current"
	}
	prompt := fmt.Sprintf("请介绍`team %s`当前 Redis Team成员构成，包括各角色的职责分工、运行状态与技术能力边界。同时说明团队内部的协作与通信机制(team_send)，例如任务流转方式、消息同步方式、上下文共享方式以及可调用的方法、工具与操作能力，以便后续能够更高效地开展团队工作", normalizedTeamName)
	return map[string]interface{}{
		"intent": initialLeaderTaskIntent,
		"title":  "介绍当前 Redis Team 成员与协作机制",
		"prompt": prompt,
	}
}

func teamEventPayloads(events []models.TeamEvent) []TeamEventPayload {
	result := make([]TeamEventPayload, 0, len(events))
	for _, event := range events {
		payload := TeamEventPayload{TeamEvent: event}
		if event.PayloadJSON != nil && strings.TrimSpace(*event.PayloadJSON) != "" {
			_ = json.Unmarshal([]byte(*event.PayloadJSON), &payload.Payload)
		}
		result = append(result, payload)
	}
	return result
}

func mergeRedisEventPayload(fields map[string]string) map[string]interface{} {
	payload := map[string]interface{}{}
	if raw := strings.TrimSpace(fields["payload"]); raw != "" {
		_ = json.Unmarshal([]byte(raw), &payload)
	}
	for key, value := range fields {
		if _, exists := payload[key]; !exists {
			payload[key] = value
		}
	}
	return payload
}

func eventString(payload map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case string:
			return strings.TrimSpace(typed)
		case float64:
			return strconv.Itoa(int(typed))
		case int:
			return strconv.Itoa(typed)
		default:
			return strings.TrimSpace(fmt.Sprintf("%v", typed))
		}
	}
	return ""
}

func eventBool(payload map[string]interface{}, keys ...string) bool {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case bool:
			return typed
		case string:
			switch strings.ToLower(strings.TrimSpace(typed)) {
			case "1", "true", "yes", "y", "on":
				return true
			case "0", "false", "no", "n", "off":
				return false
			}
		case float64:
			return typed != 0
		case int:
			return typed != 0
		default:
			text := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", typed)))
			if text == "true" || text == "yes" || text == "1" {
				return true
			}
		}
	}
	return false
}

func applyTeamMemberRuntimeProjection(member *models.TeamMember, payload map[string]interface{}, eventType string) {
	if member == nil {
		return
	}
	nonAuthoritativeDispatchWarning := isNonAuthoritativeDispatchWarning(eventType, payload)
	availability := normalizeTeamAvailability(eventString(payload, "availability", "memberAvailability"))
	if nonAuthoritativeDispatchWarning && availability == models.TeamMemberAvailabilityBlocked {
		availability = ""
	}
	explicitlyBlocked := availability == models.TeamMemberAvailabilityBlocked
	if availability != "" {
		member.Availability = availability
	}
	if member.Availability == "" {
		member.Availability = models.TeamMemberAvailabilityUnknown
	}
	if runtimeStatus := eventString(payload, "runtime_status", "runtimeStatus", "runtime", "liveness"); runtimeStatus != "" {
		member.RuntimeStatus = &runtimeStatus
	}
	if runtimeTaskID := eventString(payload, "runtime_task_id", "runtimeTaskId", "current_task_id", "currentTaskId", "taskId"); runtimeTaskID != "" {
		member.RuntimeTaskID = &runtimeTaskID
	}
	if runtimeIntent := eventString(payload, "runtime_intent", "runtimeIntent", "current_intent", "currentIntent", "intent"); runtimeIntent != "" {
		member.RuntimeIntent = &runtimeIntent
	}
	if summary := eventString(payload, "last_summary", "lastSummary", "summary", "diagnostic"); summary != "" {
		member.LastSummary = &summary
	}
	if reason := eventString(payload, "blocked_reason", "blockedReason", "error_message", "error", "reason"); reason != "" {
		if !nonAuthoritativeDispatchWarning {
			member.BlockedReason = &reason
		}
	}
	switch eventType {
	case "presence", "member_presence", "status", "member_status":
		return
	case "task_completed":
		if !explicitlyBlocked {
			member.Availability = models.TeamMemberAvailabilityIdle
			member.BlockedReason = nil
		}
	case "task_failed", "message_failed":
		if member.Availability == "" || member.Availability == models.TeamMemberAvailabilityUnknown || member.Availability == models.TeamMemberAvailabilityBusy {
			member.Availability = models.TeamMemberAvailabilityBlocked
		}
	}
}

func normalizeTeamAvailability(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "idle", "available", "ready":
		return models.TeamMemberAvailabilityIdle
	case "busy", "running", "working":
		return models.TeamMemberAvailabilityBusy
	case "blocked", "error", "failed":
		return models.TeamMemberAvailabilityBlocked
	case "offline", "unavailable":
		return models.TeamMemberAvailabilityOffline
	case "unknown":
		return models.TeamMemberAvailabilityUnknown
	default:
		return ""
	}
}

func eventInt(payload map[string]interface{}, keys ...string) int {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case float64:
			return int(typed)
		case int:
			return typed
		case string:
			parsed, _ := strconv.Atoi(strings.TrimSpace(typed))
			return parsed
		}
	}
	return 0
}

func eventTime(payload map[string]interface{}) *time.Time {
	for _, key := range []string{"occurred_at", "occurredAt", "timestamp"} {
		raw := eventString(payload, key)
		if raw == "" {
			continue
		}
		if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
			return &parsed
		}
	}
	now := time.Now().UTC()
	return &now
}

func normalizeContextRefs(value interface{}) []string {
	rawItems, ok := value.([]interface{})
	if !ok {
		if typed, ok := value.([]string); ok {
			return typed
		}
		return nil
	}
	refs := make([]string, 0, len(rawItems))
	for _, item := range rawItems {
		ref := strings.TrimSpace(fmt.Sprintf("%v", item))
		if ref != "" {
			refs = append(refs, ref)
		}
	}
	return refs
}

func (s *teamService) workspaceProxyInstance(userID, teamID int) (*models.Team, int, error) {
	team, err := s.requireOwnedTeam(userID, teamID)
	if err != nil {
		return nil, 0, err
	}
	members, err := s.repo.ListMembersByTeamID(teamID)
	if err != nil {
		return nil, 0, err
	}
	for _, member := range activeTeamMembers(members) {
		if member.InstanceID == nil {
			continue
		}
		instance, err := s.instanceService.GetByID(*member.InstanceID)
		if err != nil || instance == nil || instance.UserID != userID {
			continue
		}
		if instance.Status == "running" || member.Status == models.TeamMemberStatusIdle || member.Status == models.TeamMemberStatusBusy {
			return team, instance.ID, nil
		}
	}
	for _, member := range activeTeamMembers(members) {
		if member.InstanceID != nil {
			instance, err := s.instanceService.GetByID(*member.InstanceID)
			if err == nil && instance != nil && instance.UserID == userID {
				return team, instance.ID, nil
			}
		}
	}
	return nil, 0, fmt.Errorf("no available Team member instance for workspace access")
}

func (s *teamService) execTeamWorkspace(ctx context.Context, userID, instanceID int, command []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if s.podService == nil || s.podService.GetClient() == nil || s.podService.GetClient().Clientset == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	pod, err := s.podService.GetPod(ctx, userID, instanceID)
	if err != nil {
		return fmt.Errorf("failed to get pod: %w", err)
	}
	req := s.podService.GetClient().Clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod.Name).
		Namespace(pod.Namespace).
		SubResource("exec")
	req.VersionedParams(&corev1.PodExecOptions{
		Container: "desktop",
		Command:   command,
		Stdin:     stdin != nil,
		Stdout:    stdout != nil,
		Stderr:    stderr != nil,
		TTY:       false,
	}, scheme.ParameterCodec)
	exec, err := remotecommand.NewSPDYExecutor(s.podService.GetClient().Config, "POST", req.URL())
	if err != nil {
		return fmt.Errorf("failed to initialize exec stream: %w", err)
	}
	return exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
		Tty:    false,
	})
}

func cleanTeamWorkspacePath(raw string) (string, error) {
	value := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	value = strings.TrimPrefix(value, "/")
	if value == "" || value == "." {
		return "", nil
	}
	cleaned := posixpath.Clean(value)
	if cleaned == "." {
		return "", nil
	}
	for _, segment := range strings.Split(cleaned, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("invalid workspace path")
		}
	}
	return cleaned, nil
}

func cleanWorkspaceEntryName(raw string) (string, error) {
	name := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if name == "" || strings.Contains(name, "/") || name == "." || name == ".." {
		return "", fmt.Errorf("invalid file or folder name")
	}
	return name, nil
}

func joinTeamWorkspacePath(base, child string) string {
	if strings.TrimSpace(base) == "" {
		return child
	}
	if strings.TrimSpace(child) == "" {
		return base
	}
	return posixpath.Clean(base + "/" + child)
}

func teamWorkspaceFullPath(team *models.Team, relPath string) string {
	root := strings.TrimRight(strings.TrimSpace(team.SharedMountPath), "/")
	if root == "" {
		root = teamSharedMountPath
	}
	if relPath == "" {
		return root
	}
	return root + "/" + relPath
}

func parseTeamWorkspaceList(parentPath, raw string) []TeamWorkspaceFileEntry {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	entries := make([]TeamWorkspaceFileEntry, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) != 4 {
			continue
		}
		size, _ := strconv.ParseInt(strings.TrimSpace(parts[2]), 10, 64)
		mtimeUnix, _ := strconv.ParseInt(strings.TrimSpace(parts[3]), 10, 64)
		name := parts[1]
		entryPath := joinTeamWorkspacePath(parentPath, name)
		modifiedAt := ""
		if mtimeUnix > 0 {
			modifiedAt = time.Unix(mtimeUnix, 0).UTC().Format(time.RFC3339)
		}
		entries = append(entries, TeamWorkspaceFileEntry{
			Name:        name,
			Path:        entryPath,
			Type:        parts[0],
			Size:        size,
			ModifiedAt:  modifiedAt,
			Previewable: parts[0] == "file" && isPreviewableWorkspaceFile(name),
		})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Type != entries[j].Type {
			return entries[i].Type == "directory"
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	return entries
}

func isPreviewableWorkspaceFile(path string) bool {
	name := strings.ToLower(strings.TrimSpace(path))
	return strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".txt")
}

func planTeamMembers(teamName string, members []CreateTeamMemberRequest) ([]plannedTeamMember, error) {
	plans := make([]plannedTeamMember, 0, len(members))
	memberKeys := map[string]struct{}{}
	leaderCount := 0
	for idx, memberReq := range members {
		role := normalizeTeamMemberRole(memberReq.Role, memberReq.IsLeader)
		memberKey, err := normalizeTeamMemberKey(memberReq.MemberID, role, idx)
		if err != nil {
			return nil, err
		}
		if _, exists := memberKeys[memberKey]; exists {
			return nil, fmt.Errorf("duplicate team member id: %s", memberKey)
		}
		memberKeys[memberKey] = struct{}{}
		runtimeType, err := normalizeTeamMemberRuntimeType(memberReq.RuntimeType)
		if err != nil {
			return nil, err
		}
		instanceMode, err := normalizeTeamMemberInstanceMode(memberReq.Mode, memberReq.InstanceMode)
		if err != nil {
			return nil, err
		}

		isLeader := memberReq.IsLeader || isTeamLeaderRole(role)
		if isLeader {
			leaderCount++
			role = "leader"
		}
		displayName := strings.TrimSpace(memberReq.Name)
		if displayName == "" {
			displayName = fmt.Sprintf("%s-%s", teamName, memberKey)
		}
		plans = append(plans, plannedTeamMember{
			Request:      memberReq,
			MemberKey:    memberKey,
			DisplayName:  displayName,
			Role:         role,
			RuntimeType:  runtimeType,
			InstanceMode: instanceMode,
			IsLeader:     isLeader,
		})
	}
	if leaderCount != 1 {
		return nil, fmt.Errorf("team must include exactly one leader")
	}
	return plans, nil
}

func teamMemberInstanceName(teamName string, teamID int, memberKey string) string {
	teamPart := normalizeTeamMemberKeyForInstanceName(teamName)
	if teamPart == "" {
		teamPart = "team"
	}
	memberPart := normalizeTeamMemberKeyForInstanceName(memberKey)
	if memberPart == "" {
		memberPart = "member"
	}
	const maxInstanceNameLength = 50
	idPart := fmt.Sprintf("%d", teamID)
	maxMemberLength := maxInstanceNameLength - len(idPart) - len("--t")
	if maxMemberLength < 1 {
		maxMemberLength = 1
	}
	if len(memberPart) > maxMemberLength {
		memberPart = strings.Trim(memberPart[:maxMemberLength], "-")
		if memberPart == "" {
			memberPart = "member"
		}
	}
	suffix := fmt.Sprintf("-%s-%s", idPart, memberPart)
	if len(teamPart)+len(suffix) <= maxInstanceNameLength {
		return teamPart + suffix
	}
	maxTeamLength := maxInstanceNameLength - len(suffix)
	if maxTeamLength < 1 {
		maxTeamLength = 1
	}
	return strings.Trim(teamPart[:maxTeamLength], "-") + suffix
}

func normalizeTeamMemberKeyForInstanceName(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, "_", "-")
	normalized = strings.ReplaceAll(normalized, " ", "-")
	normalized = teamMemberInstanceNameInvalidChars.ReplaceAllString(normalized, "")
	normalized = teamMemberInstanceNameRepeatedDashs.ReplaceAllString(normalized, "-")
	return strings.Trim(normalized, "-")
}

func normalizeTeamMemberRuntimeType(raw string) (string, error) {
	runtimeType := strings.ToLower(strings.TrimSpace(raw))
	if runtimeType == "" {
		return "openclaw", nil
	}
	switch runtimeType {
	case "openclaw", "hermes":
		return runtimeType, nil
	default:
		return "", fmt.Errorf("unsupported team member runtime type: %s", raw)
	}
}

func normalizeTeamMemberInstanceMode(rawMode, rawInstanceMode string) (string, error) {
	if mode, ok := NormalizeInstanceMode(rawMode); ok {
		return mode, nil
	}
	if strings.TrimSpace(rawMode) != "" {
		return "", fmt.Errorf("unsupported team member instance mode: %s", rawMode)
	}
	if mode, ok := NormalizeInstanceMode(rawInstanceMode); ok {
		return mode, nil
	}
	if strings.TrimSpace(rawInstanceMode) != "" {
		return "", fmt.Errorf("unsupported team member instance mode: %s", rawInstanceMode)
	}
	return InstanceModeLite, nil
}

func normalizeTeamMemberRole(raw string, isLeader bool) string {
	role := strings.TrimSpace(raw)
	if isLeader || isTeamLeaderRole(role) {
		return "leader"
	}
	if role == "" {
		return "member"
	}
	return role
}

func isTeamLeaderRole(role string) bool {
	normalized := strings.ToLower(strings.TrimSpace(role))
	normalized = strings.ReplaceAll(normalized, "_", "-")
	normalized = strings.ReplaceAll(normalized, " ", "-")
	return normalized == "leader" || normalized == "team-leader"
}

func findTeamLeader(members []models.TeamMember) *models.TeamMember {
	for idx := range members {
		if isTeamLeaderRole(members[idx].Role) {
			member := members[idx]
			return &member
		}
	}
	return nil
}

func leaderMemberKey(member *models.TeamMember) string {
	if member == nil {
		return ""
	}
	return member.MemberKey
}

type teamRosterConfig struct {
	Version           int                `json:"version"`
	TeamID            string             `json:"teamId"`
	LeaderMemberID    string             `json:"leaderMemberId"`
	CommunicationMode string             `json:"communicationMode"`
	SharedDir         string             `json:"sharedDir"`
	Members           []teamRosterMember `json:"members"`
	Redis             teamRosterRedis    `json:"redis"`
}

type teamRosterMember struct {
	MemberID     string `json:"memberId"`
	Role         string `json:"role"`
	RuntimeType  string `json:"runtimeType"`
	InstanceMode string `json:"instanceMode"`
	DisplayName  string `json:"displayName"`
	Description  string `json:"description,omitempty"`
	IsLeader     bool   `json:"isLeader"`
}

type teamRosterRedis struct {
	EventsKey   string `json:"eventsKey"`
	PresenceKey string `json:"presenceKey"`
	DLQKey      string `json:"dlqKey"`
}

func buildTeamRosterConfig(team *models.Team, members []plannedTeamMember) (string, error) {
	config := teamRosterConfig{
		Version:           1,
		TeamID:            strconv.Itoa(team.ID),
		CommunicationMode: team.CommunicationMode,
		SharedDir:         team.SharedMountPath,
		Members:           make([]teamRosterMember, 0, len(members)),
		Redis: teamRosterRedis{
			EventsKey:   teamEventsKey(team.ID),
			PresenceKey: teamPresenceKey(team.ID),
			DLQKey:      teamDLQKey(team.ID),
		},
	}
	for _, member := range members {
		if member.IsLeader {
			config.LeaderMemberID = member.MemberKey
		}
		config.Members = append(config.Members, teamRosterMember{
			MemberID:     member.MemberKey,
			Role:         member.Role,
			RuntimeType:  member.RuntimeType,
			InstanceMode: member.InstanceMode,
			DisplayName:  member.DisplayName,
			Description:  derefTeamString(member.Request.Description),
			IsLeader:     member.IsLeader,
		})
	}
	if config.LeaderMemberID == "" {
		return "", fmt.Errorf("team must include exactly one leader")
	}
	return marshalJSON(config)
}

func rosterJSONWithSharedDir(rosterJSON, sharedDir string) string {
	sharedDir = strings.TrimSpace(sharedDir)
	if sharedDir == "" {
		return rosterJSON
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(rosterJSON), &payload); err != nil {
		return rosterJSON
	}
	payload["sharedDir"] = sharedDir
	raw, err := json.Marshal(payload)
	if err != nil {
		return rosterJSON
	}
	return string(raw)
}

func buildTeamRosterConfigFromMembers(team *models.Team, members []models.TeamMember) (string, error) {
	config := teamRosterConfig{
		Version:           1,
		TeamID:            strconv.Itoa(team.ID),
		CommunicationMode: team.CommunicationMode,
		SharedDir:         team.SharedMountPath,
		Members:           make([]teamRosterMember, 0, len(members)),
		Redis: teamRosterRedis{
			EventsKey:   teamEventsKey(team.ID),
			PresenceKey: teamPresenceKey(team.ID),
			DLQKey:      teamDLQKey(team.ID),
		},
	}
	for _, member := range members {
		isLeader := isTeamLeaderRole(member.Role)
		runtimeType := strings.TrimSpace(member.RuntimeType)
		if runtimeType == "" {
			runtimeType = "openclaw"
		}
		instanceMode := strings.TrimSpace(member.InstanceMode)
		if instanceMode == "" {
			instanceMode = InstanceModeLite
		}
		if isLeader {
			config.LeaderMemberID = member.MemberKey
		}
		config.Members = append(config.Members, teamRosterMember{
			MemberID:     member.MemberKey,
			Role:         member.Role,
			RuntimeType:  runtimeType,
			InstanceMode: instanceMode,
			DisplayName:  member.DisplayName,
			Description:  derefTeamString(member.Description),
			IsLeader:     isLeader,
		})
	}
	if config.LeaderMemberID == "" {
		return "", fmt.Errorf("team must include exactly one leader")
	}
	return marshalJSON(config)
}

func activeTeamMembers(members []models.TeamMember) []models.TeamMember {
	active := make([]models.TeamMember, 0, len(members))
	for _, member := range members {
		if member.Status == models.TeamMemberStatusDeleted || member.Status == models.TeamMemberStatusDeleting {
			continue
		}
		active = append(active, member)
	}
	return active
}

func activeTeams(teams []models.Team) []models.Team {
	active := make([]models.Team, 0, len(teams))
	for _, team := range teams {
		if team.Status == models.TeamStatusDeleted {
			continue
		}
		active = append(active, team)
	}
	return active
}

func deletedTeamName(name string, teamID int) string {
	const maxTeamNameLength = 255
	suffix := fmt.Sprintf("__deleted_%d", teamID)
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		trimmed = "team"
	}
	if strings.HasSuffix(trimmed, suffix) {
		return trimmed
	}
	if len(trimmed)+len(suffix) <= maxTeamNameLength {
		return trimmed + suffix
	}
	runes := []rune(trimmed)
	maxPrefixLength := maxTeamNameLength - len(suffix)
	if len(runes) > maxPrefixLength {
		runes = runes[:maxPrefixLength]
	}
	return string(runes) + suffix
}

func normalizeTeamMemberKey(raw, role string, index int) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		value = strings.ToLower(strings.TrimSpace(role))
	}
	if value == "" {
		value = fmt.Sprintf("member-%d", index+1)
	}
	value = strings.ReplaceAll(value, "_", "-")
	value = strings.ReplaceAll(value, " ", "-")
	if !teamMemberKeyPattern.MatchString(value) {
		return "", fmt.Errorf("team member id is invalid")
	}
	return value, nil
}

func defaultTeamRedisURL() string {
	for _, key := range []string{"CLAWMANAGER_TEAM_REDIS_URL", "TEAM_REDIS_URL", "REDIS_URL"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	if value, ok := defaultTeamRedisServiceURL(); ok {
		return value
	}
	return ""
}

func defaultTeamRedisServiceURL() (string, bool) {
	systemNamespace := strings.TrimSpace(os.Getenv("CLAWMANAGER_SYSTEM_NAMESPACE"))
	if systemNamespace == "" {
		if client := k8s.GetClient(); client != nil {
			systemNamespace = client.GetSystemNamespace()
		} else if baseNamespace := strings.TrimSpace(os.Getenv("K8S_NAMESPACE")); baseNamespace != "" {
			systemNamespace = fmt.Sprintf("%s-system", baseNamespace)
		}
	}
	if systemNamespace == "" {
		return "", false
	}

	serviceName := strings.TrimSpace(os.Getenv("CLAWMANAGER_TEAM_REDIS_SERVICE_NAME"))
	if serviceName == "" {
		serviceName = strings.TrimSpace(os.Getenv("CLAWMANAGER_TEAM_REDIS_SERVICE"))
	}
	if serviceName == "" {
		serviceName = "clawmanager-team-redis"
	}

	port := normalizePortValue(
		strings.TrimSpace(os.Getenv("CLAWMANAGER_TEAM_REDIS_SERVICE_PORT")),
		strings.TrimSpace(os.Getenv("CLAWMANAGER_TEAM_REDIS_PORT")),
	)
	if port == "" {
		port = "6379"
	}

	db := strings.TrimSpace(os.Getenv("CLAWMANAGER_TEAM_REDIS_DB"))
	if db == "" {
		db = strings.TrimSpace(os.Getenv("TEAM_REDIS_DB"))
	}
	if db == "" {
		db = "0"
	}

	return fmt.Sprintf("redis://%s.%s.svc.cluster.local:%s/%s", serviceName, systemNamespace, port, db), true
}

func teamTaskStaleTimeout() time.Duration {
	raw := strings.TrimSpace(os.Getenv("CLAWMANAGER_TEAM_TASK_STALE_SECONDS"))
	if raw == "" {
		return defaultTeamTaskStaleTimeout
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil {
		return defaultTeamTaskStaleTimeout
	}
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

func defaultTeamManagerBaseURL() (string, bool) {
	if override := strings.TrimSpace(os.Getenv("CLAWMANAGER_TEAM_MANAGER_BASE_URL")); override != "" {
		return override, true
	}
	return defaultAgentControlBaseURL()
}

func teamInboxKey(teamID int, memberID string) string {
	return fmt.Sprintf("claw:team:%d:inbox:%s", teamID, memberID)
}

func teamEventsKey(teamID int) string {
	return fmt.Sprintf("claw:team:%d:events", teamID)
}

func teamPresenceKey(teamID int) string {
	return fmt.Sprintf("claw:team:%d:presence", teamID)
}

func teamDLQKey(teamID int) string {
	return fmt.Sprintf("claw:team:%d:dlq", teamID)
}

func defaultInt(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func defaultFloat(value, fallback float64) float64 {
	if value > 0 {
		return value
	}
	return fallback
}

func derefTeamString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
