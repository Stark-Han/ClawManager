package services

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"clawreef/internal/config"
	"clawreef/internal/models"
	"clawreef/internal/repository"
	"clawreef/internal/services/k8s"

	"github.com/upper/db/v4"
)

const (
	OpenClawUpgradeLabBaselineTag      = "master-20260824-737ad4c"
	OpenClawUpgradeLabBaselineDigest   = "sha256:c0905d813cdf22f5ed357d9bd6f61a6798020f1d099022f0aaec4a96c83df125"
	openClawUpgradeLabDescription      = "openclaw-upgrade-lab:"
	openClawUpgradeLabMaxCases         = 8
	openClawUpgradeLabReadyTimeout     = 3 * time.Minute
	openClawUpgradeLabProvisionTimeout = 12 * time.Minute
	openClawUpgradeLabCleanupTimeout   = 10 * time.Second
	openClawUpgradeLabCleanupQuiet     = 250 * time.Millisecond
)

type upgradeLabGatewayEnvBuilder interface {
	BuildGatewayEnv(*models.Instance) (map[string]string, error)
}

type OpenClawUpgradeLabService struct {
	sess         db.Session
	instances    repository.InstanceRepository
	pods         repository.RuntimePodRepository
	bindings     repository.InstanceRuntimeBindingRepository
	agent        RuntimeAgentClient
	deployments  k8s.RuntimeUpgradeLabDeploymentService
	inventory    RuntimeDeploymentInventoryProvider
	upgrade      *RuntimeUpgradeService
	scheduler    *RuntimeScheduler
	envBuilder   upgradeLabGatewayEnvBuilder
	conversation upgradeLabConversationClient
	cfg          config.RuntimePoolConfig
	// workspaceRemover is injected only by focused cleanup tests. Production
	// calls removeUpgradeLabWorkspace, which is rooted beneath WorkspaceRoot.
	workspaceRemover func(context.Context, string) error
}

type OpenClawUpgradeLabCheck struct {
	Name     string `json:"name"`
	Passed   bool   `json:"passed"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
	Message  string `json:"message,omitempty"`
}

type OpenClawUpgradeLabView struct {
	Run              *models.OpenClawUpgradeLabRun        `json:"run"`
	InstanceIDs      []int                                `json:"instance_ids"`
	Instances        []models.Instance                    `json:"instances"`
	Checks           []OpenClawUpgradeLabCheck            `json:"checks"`
	Rollout          *RuntimeUpgradeDetails               `json:"rollout,omitempty"`
	BaselineTag      string                               `json:"baseline_tag"`
	BaselineEvidence []OpenClawUpgradeLabBaselineEvidence `json:"baseline_evidence"`
}

type upgradeLabDataManifest struct {
	Files    map[string]string         `json:"files"`
	Digest   string                    `json:"digest"`
	Sessions upgradeLabSessionManifest `json:"sessions"`
}

type upgradeLabSessionManifest struct {
	SessionCount                int               `json:"session_count"`
	UserMessageCount            int               `json:"user_message_count"`
	AssistantMessageCount       int               `json:"assistant_message_count"`
	InteractiveUserMessageCount int               `json:"interactive_user_message_count"`
	CatalogSHA256               string            `json:"catalog_sha256"`
	SourceFiles                 map[string]string `json:"source_files"`
	SourceFilesSHA256           string            `json:"source_files_sha256"`
	AncillaryFiles              map[string]string `json:"ancillary_files,omitempty"`
	AncillaryFilesSHA256        string            `json:"ancillary_files_sha256,omitempty"`
}

type OpenClawUpgradeLabBaselineEvidence struct {
	InstanceID                  int    `json:"instance_id"`
	ProjectFileCount            int    `json:"project_file_count"`
	ManualProjectFileCount      int    `json:"manual_project_file_count"`
	SessionCount                int    `json:"session_count"`
	UserMessageCount            int    `json:"user_message_count"`
	AssistantMessageCount       int    `json:"assistant_message_count"`
	InteractiveUserMessageCount int    `json:"interactive_user_message_count"`
	ProjectSHA256               string `json:"project_sha256"`
	SessionCatalogSHA256        string `json:"session_catalog_sha256"`
	SessionSourceSHA256         string `json:"session_source_sha256"`
}

type upgradeLabProvisionPlan struct {
	InstanceCount int `json:"instance_count"`
}

func NewOpenClawUpgradeLabService(sess db.Session, instances repository.InstanceRepository, pods repository.RuntimePodRepository, bindings repository.InstanceRuntimeBindingRepository, agent RuntimeAgentClient, deployments k8s.RuntimeUpgradeLabDeploymentService, inventory RuntimeDeploymentInventoryProvider, upgrade *RuntimeUpgradeService, scheduler *RuntimeScheduler, envBuilder upgradeLabGatewayEnvBuilder, cfg config.RuntimePoolConfig) *OpenClawUpgradeLabService {
	return &OpenClawUpgradeLabService{sess: sess, instances: instances, pods: pods, bindings: bindings, agent: agent, deployments: deployments, inventory: inventory, upgrade: upgrade, scheduler: scheduler, envBuilder: envBuilder, conversation: openClawUpgradeLabConversationClient{}, cfg: cfg}
}

func (s *OpenClawUpgradeLabService) Latest(ctx context.Context) (*OpenClawUpgradeLabView, error) {
	var run models.OpenClawUpgradeLabRun
	if err := s.sess.Collection(run.TableName()).Find().OrderBy("-id").One(&run); err != nil {
		if errors.Is(err, db.ErrNoMoreRows) {
			return &OpenClawUpgradeLabView{BaselineTag: OpenClawUpgradeLabBaselineTag, InstanceIDs: []int{}, Instances: []models.Instance{}, Checks: []OpenClawUpgradeLabCheck{}}, nil
		}
		return nil, err
	}
	run = *s.recoverProvisionedRun(ctx, &run)
	return s.view(ctx, &run)
}

func (s *OpenClawUpgradeLabService) Get(ctx context.Context, id int64) (*OpenClawUpgradeLabView, error) {
	run, err := s.getRun(ctx, id)
	if err != nil || run == nil {
		return nil, err
	}
	run = s.recoverProvisionedRun(ctx, run)
	return s.view(ctx, run)
}

func (s *OpenClawUpgradeLabService) CreateBaseline(ctx context.Context, actorUserID, count int) (*OpenClawUpgradeLabView, error) {
	if s == nil || s.sess == nil || s.instances == nil || s.pods == nil || s.bindings == nil || s.agent == nil || s.deployments == nil || s.inventory == nil || s.envBuilder == nil {
		return nil, fmt.Errorf("OpenClaw upgrade lab is not configured")
	}
	if actorUserID <= 0 || count <= 0 || count > openClawUpgradeLabMaxCases {
		return nil, fmt.Errorf("test instance count must be between 1 and %d", openClawUpgradeLabMaxCases)
	}
	var active int
	row, err := s.sess.SQL().QueryRowContext(ctx, `SELECT COUNT(*) FROM openclaw_upgrade_lab_runs WHERE status <> 'cleaned'`)
	if err == nil {
		err = row.Scan(&active)
	}
	if err != nil {
		return nil, err
	}
	if active > 0 {
		return nil, fmt.Errorf("an OpenClaw upgrade lab run is already active; reset or clean it first")
	}
	template, baselineTagRef, baselineRef, err := s.resolveBaseline(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	planRaw, _ := json.Marshal(upgradeLabProvisionPlan{InstanceCount: count})
	planJSON := string(planRaw)
	run := &models.OpenClawUpgradeLabRun{ActorUserID: &actorUserID, Status: "provisioning", Phase: "baseline_pool", BaselineImageRef: baselineTagRef, BaselineImageDigest: OpenClawUpgradeLabBaselineDigest, BeforeJSON: &planJSON, CreatedAt: now, UpdatedAt: now}
	inserted, err := s.sess.Collection(run.TableName()).Insert(run)
	if err != nil {
		return nil, err
	}
	if id, ok := inserted.ID().(int64); ok {
		run.ID = id
	} else if id, ok := inserted.ID().(int); ok {
		run.ID = int64(id)
	} else {
		return nil, fmt.Errorf("upgrade lab run id was not returned")
	}
	// Provisioning belongs to the durable lab run, not to the browser request.
	// Navigating away must not cancel a successfully recorded setup half-way.
	provisionCtx, cancelProvision := context.WithTimeout(context.Background(), openClawUpgradeLabProvisionTimeout)
	defer cancelProvision()
	runToken := upgradeLabRunToken(run.ID)
	run.SourceDeployment = fmt.Sprintf("%sr%d-source", openClawUpgradeLabDeploymentPrefix, run.ID)
	if _, err := s.sess.SQL().ExecContext(provisionCtx, `UPDATE openclaw_upgrade_lab_runs SET source_deployment = ?, updated_at = ? WHERE id = ?`, run.SourceDeployment, time.Now().UTC(), run.ID); err != nil {
		return nil, err
	}
	if err := s.deployments.EnsureUpgradeLabPool(provisionCtx, template.Namespace, template.DeploymentName, run.SourceDeployment, baselineRef, runToken, 1); err != nil {
		return nil, s.failRun(provisionCtx, run.ID, "BASELINE_POOL_CREATE_FAILED", err)
	}
	pod, err := s.waitForLabPod(provisionCtx, run.SourceDeployment, openClawUpgradeLabReadyTimeout)
	if err != nil {
		return nil, s.failRun(provisionCtx, run.ID, "BASELINE_RUNTIME_NOT_READY", err)
	}
	instanceIDs := make([]int, 0, count)
	instances := make([]*models.Instance, 0, count)
	for index := 0; index < count; index++ {
		instance, err := s.createBaselineInstance(provisionCtx, run, pod, index)
		if err != nil {
			return nil, s.failRun(provisionCtx, run.ID, "BASELINE_GATEWAY_CREATE_FAILED", err)
		}
		instanceIDs = append(instanceIDs, instance.ID)
		instances = append(instances, instance)
		// Persist ownership after every successful fixture.  A later failure can
		// therefore be cleaned without scanning or guessing across user data.
		idsRaw, _ := json.Marshal(instanceIDs)
		if _, err := s.sess.SQL().ExecContext(provisionCtx, `UPDATE openclaw_upgrade_lab_runs SET instance_ids_json = ?, updated_at = ? WHERE id = ?`, string(idsRaw), time.Now().UTC(), run.ID); err != nil {
			return nil, s.failRun(provisionCtx, run.ID, "BASELINE_OWNERSHIP_RECORD_FAILED", err)
		}
	}
	if _, err := s.sess.SQL().ExecContext(provisionCtx, `UPDATE openclaw_upgrade_lab_runs SET phase = 'fixture_seeding', updated_at = ? WHERE id = ? AND status = 'provisioning'`, time.Now().UTC(), run.ID); err != nil {
		return nil, s.failRun(provisionCtx, run.ID, "BASELINE_FIXTURE_STATE_FAILED", err)
	}
	if err := s.seedBaselineFixtures(provisionCtx, run, pod, instances); err != nil {
		return nil, s.failRun(provisionCtx, run.ID, "BASELINE_FIXTURE_CREATE_FAILED", err)
	}
	before, err := s.captureAndValidateBaseline(run.ID, instanceIDs)
	if err != nil {
		return nil, s.failRun(provisionCtx, run.ID, "BASELINE_CAPTURE_FAILED", err)
	}
	beforeRaw, _ := json.Marshal(before)
	idsRaw, _ := json.Marshal(instanceIDs)
	if _, err := s.sess.SQL().ExecContext(provisionCtx, `UPDATE openclaw_upgrade_lab_runs SET status = 'ready', phase = 'baseline_captured', instance_ids_json = ?, before_json = ?, error_code = NULL, error_message = NULL, updated_at = ? WHERE id = ?`, string(idsRaw), string(beforeRaw), time.Now().UTC(), run.ID); err != nil {
		return nil, err
	}
	return s.Get(provisionCtx, run.ID)
}

func (s *OpenClawUpgradeLabService) StartUpgrade(ctx context.Context, id int64, actorUserID int, targetImage string, batchSize int) (*OpenClawUpgradeLabView, error) {
	run, err := s.getRun(ctx, id)
	if err != nil || run == nil {
		return nil, err
	}
	if run.ActorUserID == nil || *run.ActorUserID != actorUserID || run.Status != "ready" {
		return nil, fmt.Errorf("upgrade lab run is not ready for this administrator")
	}
	var before map[int]upgradeLabDataManifest
	if run.Phase != "baseline_captured" || run.BeforeJSON == nil || json.Unmarshal([]byte(*run.BeforeJSON), &before) != nil {
		return nil, fmt.Errorf("capture and validate the 7.1 baseline before starting the upgrade")
	}
	ids := decodeLabInstanceIDs(run.InstanceIDsJSON)
	// Re-capture immediately before claiming the rollout. Automated fixtures
	// must still be present, while optional tester conversations/files added
	// after provisioning become part of the protected baseline automatically.
	current, err := s.captureAndValidateBaseline(run.ID, ids)
	if err != nil {
		return nil, err
	}
	before = current
	claim, err := s.sess.SQL().ExecContext(ctx, `UPDATE openclaw_upgrade_lab_runs SET status = 'preflighting', phase = 'data_capture', updated_at = ? WHERE id = ? AND actor_user_id = ? AND status = 'ready'`, time.Now().UTC(), run.ID, actorUserID)
	if err != nil {
		return nil, err
	}
	if affected, _ := claim.RowsAffected(); affected != 1 {
		return nil, fmt.Errorf("upgrade lab run changed concurrently")
	}
	if len(ids) == 0 {
		return nil, s.failRun(ctx, run.ID, "BASELINE_INSTANCES_MISSING", errors.New("upgrade lab run has no test instances"))
	}
	if s.upgrade == nil || s.scheduler == nil {
		return nil, s.failRun(ctx, run.ID, "UPGRADE_ENGINE_UNAVAILABLE", errors.New("shared OpenClaw upgrade engine is unavailable"))
	}
	beforeRaw, _ := json.Marshal(before)
	result, err := s.upgrade.Preflight(ctx, RuntimeUpgradePreflightRequest{TargetImageRef: strings.TrimSpace(targetImage), BatchSize: minInt(maxInt(batchSize, 1), openClawUpgradeLabMaxCases), MaxUnavailable: 0, AutoRollback: true, ActorUserID: &actorUserID, UpgradeLabRunID: &run.ID, CandidateInstanceIDs: ids})
	if err != nil {
		return nil, s.failRun(ctx, run.ID, "UPGRADE_PREFLIGHT_FAILED", err)
	}
	if !result.Passed || result.Rollout == nil || result.Rollout.PreflightID == nil {
		return nil, s.failRun(ctx, run.ID, "UPGRADE_PREFLIGHT_BLOCKED", errors.New(strings.Join(result.Blockers, "; ")))
	}
	rollout, err := s.upgrade.ConfirmPreflight(ctx, *result.Rollout.PreflightID, result.TargetImageRef, &actorUserID)
	if err != nil {
		return nil, s.failRun(ctx, run.ID, "UPGRADE_CONFIRM_FAILED", err)
	}
	if _, err := s.sess.SQL().ExecContext(ctx, `UPDATE openclaw_upgrade_lab_runs SET status = 'upgrading', phase = 'preflight', target_image_ref = ?, rollout_id = ?, before_json = ?, error_code = NULL, error_message = NULL, updated_at = ? WHERE id = ?`, rollout.TargetImageRef, rollout.ID, string(beforeRaw), time.Now().UTC(), run.ID); err != nil {
		return nil, err
	}
	if err := s.scheduler.StartRollout(ctx, rollout.ID); err != nil {
		return nil, s.failRun(ctx, run.ID, "UPGRADE_START_FAILED", err)
	}
	return s.Get(ctx, run.ID)
}

func (s *OpenClawUpgradeLabService) CaptureBaseline(ctx context.Context, id int64, actorUserID int) (*OpenClawUpgradeLabView, error) {
	run, err := s.getRun(ctx, id)
	if err != nil || run == nil {
		return nil, err
	}
	if run.ActorUserID == nil || *run.ActorUserID != actorUserID || run.Status != "ready" {
		return nil, fmt.Errorf("upgrade lab run is not ready for baseline capture")
	}
	ids := decodeLabInstanceIDs(run.InstanceIDsJSON)
	if len(ids) == 0 {
		return nil, fmt.Errorf("upgrade lab run has no test instances")
	}
	before, err := s.captureAndValidateBaseline(run.ID, ids)
	if err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(before)
	if _, err := s.sess.SQL().ExecContext(ctx, `UPDATE openclaw_upgrade_lab_runs SET phase = 'baseline_captured', before_json = ?, checks_json = NULL, error_code = NULL, error_message = NULL, updated_at = ? WHERE id = ? AND actor_user_id = ? AND status = 'ready'`, string(raw), time.Now().UTC(), run.ID, actorUserID); err != nil {
		return nil, err
	}
	return s.Get(ctx, run.ID)
}

func (s *OpenClawUpgradeLabService) Reset(ctx context.Context, id int64, actorUserID, count int) (*OpenClawUpgradeLabView, error) {
	if err := s.Cleanup(ctx, id, actorUserID); err != nil {
		return nil, err
	}
	return s.CreateBaseline(ctx, actorUserID, count)
}

func (s *OpenClawUpgradeLabService) Cleanup(ctx context.Context, id int64, actorUserID int) error {
	run, err := s.getRun(ctx, id)
	if err != nil || run == nil {
		return err
	}
	if run.ActorUserID == nil || *run.ActorUserID != actorUserID {
		return fmt.Errorf("upgrade lab run belongs to another administrator")
	}
	if run.Status == "cleaned" {
		return nil
	}
	if run.Status == "provisioning" || run.Status == "preflighting" {
		return fmt.Errorf("cannot clean an active upgrade lab setup")
	}
	if run.RolloutID != nil {
		details, detailErr := s.upgrade.Details(ctx, *run.RolloutID)
		if detailErr != nil {
			return detailErr
		}
		if details != nil && details.Rollout != nil && details.Rollout.Status != "finished" && details.Rollout.Status != "error" && details.Rollout.Status != "cancelled" {
			return fmt.Errorf("cannot clean an active upgrade lab rollout")
		}
		if details != nil && details.Rollout != nil {
			rollbackStatus := strings.ToLower(strings.TrimSpace(stringValue(details.Rollout.RollbackStatus)))
			if rollbackStatus == "starting" || rollbackStatus == "waiting" {
				return fmt.Errorf("cannot clean while the upgrade lab rollback is active")
			}
		}
	}
	ids := decodeLabInstanceIDs(run.InstanceIDsJSON)
	for _, instanceID := range ids {
		instance, getErr := s.instances.GetByID(instanceID)
		if getErr != nil {
			return getErr
		}
		if instance == nil {
			// Older cleanup attempts could delete the database row before an NFS
			// RemoveAll finished. Recover that exact, run-owned canonical path so
			// retries do not leave an orphan workspace behind.
			workspace := RuntimeWorkspacePathWithRoot(s.cfg.WorkspaceRoot, RuntimeTypeOpenClaw, *run.ActorUserID, instanceID)
			if removeErr := s.removeLabWorkspace(ctx, workspace); removeErr != nil {
				return fmt.Errorf("delete orphaned upgrade lab workspace for instance %d: %w", instanceID, removeErr)
			}
			continue
		}
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(stringValue(instance.Description))), openClawUpgradeLabDescription) {
			return fmt.Errorf("refusing to clean non-lab instance %d", instanceID)
		}
		if deleteErr := s.deleteLabInstance(ctx, instance); deleteErr != nil {
			return fmt.Errorf("delete upgrade lab instance %d: %w", instanceID, deleteErr)
		}
	}
	runToken := upgradeLabRunToken(run.ID)
	if run.RolloutID != nil {
		target := runtimeUpgradeTargetDeploymentName(run.SourceDeployment, *run.RolloutID)
		if err := s.deployments.DeleteUpgradeLabPool(ctx, s.cfg.Namespace, target, runToken); err != nil {
			return err
		}
	}
	if err := s.deployments.DeleteUpgradeLabPool(ctx, s.cfg.Namespace, run.SourceDeployment, runToken); err != nil {
		return err
	}
	now := time.Now().UTC()
	_, err = s.sess.SQL().ExecContext(ctx, `UPDATE openclaw_upgrade_lab_runs SET status = 'cleaned', phase = 'cleaned', finished_at = COALESCE(finished_at, ?), updated_at = ? WHERE id = ?`, now, now, run.ID)
	return err
}

func (s *OpenClawUpgradeLabService) EnsureUpgradeLabGateway(ctx context.Context, rollout *models.RuntimeRollout, instanceID int, targetPods []models.RuntimePod) error {
	scope := runtimeUpgradeScopeFromRollout(rollout)
	if scope.UpgradeLabRunID == nil {
		return fmt.Errorf("rollout is not an upgrade lab run")
	}
	run, err := s.getRun(ctx, *scope.UpgradeLabRunID)
	if err != nil || run == nil {
		return err
	}
	ids := decodeLabInstanceIDs(run.InstanceIDsJSON)
	index := -1
	for i, id := range ids {
		if id == instanceID {
			index = i
			break
		}
	}
	if index < 0 || len(targetPods) == 0 {
		return fmt.Errorf("instance is outside the upgrade lab run")
	}
	if binding, err := s.bindings.GetRunningByInstanceID(ctx, instanceID); err != nil {
		return err
	} else if binding != nil {
		return nil
	}
	instance, err := s.instances.GetByID(instanceID)
	if err != nil || instance == nil {
		return fmt.Errorf("upgrade lab instance %d is unavailable", instanceID)
	}
	pod := targetPods[index%len(targetPods)]
	return s.startGateway(ctx, run, instance, pod, strconv.FormatInt(rollout.ID, 10), index)
}

func (s *OpenClawUpgradeLabService) createBaselineInstance(ctx context.Context, run *models.OpenClawUpgradeLabRun, pod models.RuntimePod, index int) (instance *models.Instance, err error) {
	now := time.Now().UTC()
	description := fmt.Sprintf("%s%d", openClawUpgradeLabDescription, run.ID)
	image := run.BaselineImageRef
	imageTag := OpenClawUpgradeLabBaselineTag
	if run.ActorUserID == nil || *run.ActorUserID <= 0 {
		return nil, fmt.Errorf("upgrade lab owner is unavailable")
	}
	instance = &models.Instance{UserID: *run.ActorUserID, Name: fmt.Sprintf("OpenClaw升级测试-%d-%d", run.ID, index+1), Description: &description, Type: RuntimeTypeOpenClaw, RuntimeType: RuntimeBackendGateway, InstanceMode: InstanceModeLite, Status: "stopped", CPUCores: 1, MemoryGB: 2, DiskGB: 5, OSType: "linux", OSVersion: "openclaw-2026.7.1-2", ImageRegistry: &image, ImageTag: &imageTag, StorageClass: "shared", MountPath: s.cfg.WorkspaceRoot, RuntimeGeneration: 1, CreatedAt: now, UpdatedAt: now}
	if err := s.instances.Create(instance); err != nil {
		return nil, err
	}
	workspace := RuntimeWorkspacePathWithRoot(s.cfg.WorkspaceRoot, RuntimeTypeOpenClaw, instance.UserID, instance.ID)
	defer func() {
		if err == nil {
			return
		}
		var cleanupErr error
		cleanupErr = s.deleteLabInstance(context.Background(), instance)
		if cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("clean partial upgrade lab instance %d: %w", instance.ID, cleanupErr))
		}
	}()
	instance.WorkspacePath = &workspace
	if err = s.instances.SetWorkspacePath(ctx, instance.ID, workspace); err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Join(workspace, "project"), 0o750); err != nil {
		return nil, err
	}
	if err = s.startGateway(ctx, run, instance, pod, upgradeLabRunToken(run.ID), index); err != nil {
		return nil, err
	}
	fixturePath := filepath.Join(workspace, "project", "upgrade-lab-continuity.txt")
	fixture := []byte(fmt.Sprintf("openclaw-upgrade-lab run=%d case=%d baseline=2026.7.1-2\n", run.ID, index+1))
	if err = os.WriteFile(fixturePath, fixture, 0o640); err != nil {
		return nil, err
	}
	_ = os.Chown(fixturePath, RuntimeLinuxID(instance.ID), RuntimeLinuxID(instance.ID))
	return instance, nil
}

func (s *OpenClawUpgradeLabService) seedBaselineFixtures(ctx context.Context, run *models.OpenClawUpgradeLabRun, pod models.RuntimePod, instances []*models.Instance) error {
	if s.conversation == nil {
		return errors.New("upgrade lab conversation client is unavailable")
	}
	seedCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	sem := make(chan struct{}, minInt(4, len(instances)))
	var wg sync.WaitGroup
	var firstErr error
	var errMu sync.Mutex
	for index, instance := range instances {
		index, instance := index, instance
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-seedCtx.Done():
				return
			}
			if err := s.seedBaselineFixture(seedCtx, run, pod, instance, index); err != nil {
				errMu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("instance %d: %w", instance.ID, err)
					cancel()
				}
				errMu.Unlock()
			}
		}()
	}
	wg.Wait()
	return firstErr
}

func (s *OpenClawUpgradeLabService) seedBaselineFixture(ctx context.Context, run *models.OpenClawUpgradeLabRun, pod models.RuntimePod, instance *models.Instance, index int) error {
	if instance == nil || pod.AgentEndpoint == nil {
		return errors.New("baseline gateway is unavailable")
	}
	env, err := s.envBuilder.BuildGatewayEnv(instance)
	if err != nil {
		return err
	}
	token := strings.TrimSpace(env["CLAWMANAGER_INSTANCE_TOKEN"])
	if token == "" {
		return errors.New("baseline gateway credential is unavailable")
	}
	marker := upgradeLabFixtureMarker(run.ID, index)
	prompt := fmt.Sprintf("这是一次升级连续性测试。请用一句简短中文确认你已收到测试标识 %s。", marker)
	sessionKey := fmt.Sprintf("agent:main:upgrade-lab-%d-%d", run.ID, index+1)
	port := s.cfg.GatewayPortStart + index
	if err := s.conversation.SendMessage(ctx, strings.TrimSpace(*pod.AgentEndpoint), port, instance.ID, token, sessionKey, prompt); err != nil {
		return err
	}
	deadline := time.Now().Add(2 * time.Minute)
	workspace := strings.TrimSpace(stringValue(instance.WorkspacePath))
	for {
		completed, inspectErr := upgradeLabConversationCompleted(workspace, marker)
		if inspectErr != nil {
			return inspectErr
		}
		if completed {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("baseline assistant reply did not complete within 2 minutes")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func (s *OpenClawUpgradeLabService) captureAndValidateBaseline(runID int64, ids []int) (map[int]upgradeLabDataManifest, error) {
	manifests, err := s.captureDataManifests(ids)
	if err != nil {
		return nil, err
	}
	for index, instanceID := range ids {
		manifest := manifests[instanceID]
		if _, ok := manifest.Files["upgrade-lab-continuity.txt"]; !ok {
			return nil, fmt.Errorf("instance %d automated project fixture is missing", instanceID)
		}
		if manifest.Sessions.SessionCount < 1 || manifest.Sessions.InteractiveUserMessageCount < 1 || manifest.Sessions.AssistantMessageCount < 1 {
			return nil, fmt.Errorf("instance %d automated conversation fixture is incomplete", instanceID)
		}
		instance, getErr := s.instances.GetByID(instanceID)
		if getErr != nil || instance == nil {
			return nil, fmt.Errorf("upgrade lab instance %d is unavailable", instanceID)
		}
		completed, inspectErr := upgradeLabConversationCompleted(stringValue(instance.WorkspacePath), upgradeLabFixtureMarker(runID, index))
		if inspectErr != nil {
			return nil, inspectErr
		}
		// Older runs did not use an automatic marker. They remain usable when
		// they already contain a complete real conversation and fixture file.
		if !completed && !upgradeLabAnyCompletedConversation(stringValue(instance.WorkspacePath)) {
			return nil, fmt.Errorf("instance %d has no completed baseline conversation", instanceID)
		}
	}
	return manifests, nil
}

func upgradeLabFixtureMarker(runID int64, index int) string {
	return fmt.Sprintf("openclaw-upgrade-lab:%d:%d", runID, index+1)
}

// deleteLabInstance is intentionally narrower than the normal instance
// lifecycle.  Lab fixtures use the legacy 7.1 Runtime Agent, whose watcher can
// win the stop response race.  We accept only a verified absent/stopped
// gateway, release exactly its binding, delete exactly its fixture row, and
// remove only its canonical workspace. The instance row is deliberately
// deleted last: stale Runtime reports can still refer to it while an NFS
// writer is settling, and a failed filesystem cleanup must remain retryable.
// No serving Deployment is touched.
func (s *OpenClawUpgradeLabService) deleteLabInstance(ctx context.Context, instance *models.Instance) error {
	if instance == nil || !strings.HasPrefix(strings.ToLower(strings.TrimSpace(stringValue(instance.Description))), openClawUpgradeLabDescription) {
		return fmt.Errorf("instance is not owned by the OpenClaw upgrade lab")
	}
	workspace := strings.TrimSpace(stringValue(instance.WorkspacePath))
	expectedWorkspace := RuntimeWorkspacePathWithRoot(s.cfg.WorkspaceRoot, RuntimeTypeOpenClaw, instance.UserID, instance.ID)
	if workspace != "" && (!sameCleanPath(workspace, expectedWorkspace) || !pathWithin(s.cfg.WorkspaceRoot, workspace)) {
		return fmt.Errorf("lab workspace left its canonical path")
	}
	binding, err := s.bindings.GetByInstanceID(ctx, instance.ID)
	if err != nil {
		return err
	}
	if binding != nil {
		pod, podErr := s.pods.GetByID(ctx, binding.RuntimePodID)
		if podErr != nil {
			return podErr
		}
		if pod != nil && pod.AgentEndpoint != nil && strings.TrimSpace(*pod.AgentEndpoint) != "" && binding.GatewayID != "" {
			endpoint := strings.TrimSpace(*pod.AgentEndpoint)
			deleteErr := s.agent.DeleteGateway(ctx, endpoint, binding.GatewayID)
			confirmed := errors.Is(deleteErr, ErrRuntimeAgentNotFound)
			if reader, ok := s.agent.(gatewayStateReader); ok {
				var confirmErr error
				confirmed, confirmErr = waitForGatewayStopped(ctx, reader, endpoint, binding.GatewayID, 5*time.Second)
				if !confirmed {
					return errors.Join(deleteErr, confirmErr)
				}
			}
			if deleteErr != nil && !errors.Is(deleteErr, ErrRuntimeAgentNotFound) && !confirmed {
				return deleteErr
			}
		}
		if err := s.bindings.DeleteByInstanceIDAndReleaseSlot(ctx, instance.ID, binding.RuntimePodID); err != nil {
			return err
		}
	}
	if workspace != "" {
		if err := s.removeLabWorkspace(ctx, workspace); err != nil {
			return err
		}
	}
	if err := s.instances.Delete(instance.ID); err != nil && !strings.Contains(strings.ToLower(err.Error()), "not found") {
		return err
	}
	return nil
}

type upgradeLabWorkspaceRoot interface {
	Lstat(name string) (os.FileInfo, error)
	RemoveAll(name string) error
}

func (s *OpenClawUpgradeLabService) removeLabWorkspace(ctx context.Context, workspace string) error {
	rootPath := filepath.Clean(strings.TrimSpace(s.cfg.WorkspaceRoot))
	targetPath := filepath.Clean(strings.TrimSpace(workspace))
	if rootPath == "." || targetPath == "." || targetPath == rootPath || !pathWithin(rootPath, targetPath) {
		return fmt.Errorf("lab workspace failed cleanup safety validation")
	}
	relativePath, err := filepath.Rel(rootPath, targetPath)
	if err != nil || relativePath == "." || filepath.IsAbs(relativePath) || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return fmt.Errorf("lab workspace failed rooted cleanup validation")
	}
	if s.workspaceRemover != nil {
		return s.workspaceRemover(ctx, targetPath)
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return fmt.Errorf("open upgrade lab workspace root: %w", err)
	}
	defer root.Close()
	if err := removeUpgradeLabWorkspaceWithRetry(ctx, root, relativePath, openClawUpgradeLabCleanupTimeout, openClawUpgradeLabCleanupQuiet); err != nil {
		return fmt.Errorf("remove upgrade lab workspace: %w", err)
	}
	return nil
}

// removeUpgradeLabWorkspaceWithRetry handles the short ENOTEMPTY window seen
// when a confirmed-stopped process finishes flushing an NFSv4 workspace. It
// requires two absent observations separated by a quiet period, never retries
// permission or path-safety failures, and remains bounded by both context and
// deadline.
func removeUpgradeLabWorkspaceWithRetry(ctx context.Context, root upgradeLabWorkspaceRoot, relativePath string, timeout, quiet time.Duration) error {
	if timeout <= 0 {
		timeout = openClawUpgradeLabCleanupTimeout
	}
	if quiet <= 0 {
		quiet = openClawUpgradeLabCleanupQuiet
	}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		removeErr := root.RemoveAll(relativePath)
		if removeErr != nil {
			lastErr = removeErr
			if !retryableUpgradeLabRemoveError(removeErr) {
				return removeErr
			}
		} else {
			absent, statErr := upgradeLabWorkspaceAbsent(root, relativePath)
			if statErr != nil {
				return statErr
			}
			if absent {
				if err := waitUpgradeLabCleanup(ctx, quiet, deadline); err != nil {
					return err
				}
				absent, statErr = upgradeLabWorkspaceAbsent(root, relativePath)
				if statErr != nil {
					return statErr
				}
				if absent {
					return nil
				}
				lastErr = errors.New("workspace reappeared during cleanup quiet period")
			}
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("workspace did not become quiescent within %s: %w", timeout, lastErr)
		}
		if err := waitUpgradeLabCleanup(ctx, quiet, deadline); err != nil {
			return err
		}
	}
}

func upgradeLabWorkspaceAbsent(root upgradeLabWorkspaceRoot, relativePath string) (bool, error) {
	_, err := root.Lstat(relativePath)
	if os.IsNotExist(err) {
		return true, nil
	}
	return false, err
}

func retryableUpgradeLabRemoveError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return os.IsExist(err) || strings.Contains(message, "directory not empty") || strings.Contains(message, "resource busy")
}

func waitUpgradeLabCleanup(ctx context.Context, delay time.Duration, deadline time.Time) error {
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return fmt.Errorf("upgrade lab workspace cleanup timed out")
	}
	if delay > remaining {
		delay = remaining
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (s *OpenClawUpgradeLabService) startGateway(ctx context.Context, run *models.OpenClawUpgradeLabRun, instance *models.Instance, pod models.RuntimePod, upgradeID string, index int) error {
	if pod.AgentEndpoint == nil || strings.TrimSpace(*pod.AgentEndpoint) == "" {
		return fmt.Errorf("upgrade lab Runtime pod has no agent endpoint")
	}
	env, err := s.envBuilder.BuildGatewayEnv(instance)
	if err != nil {
		return err
	}
	port := s.cfg.GatewayPortStart + index
	if port <= 0 || port > s.cfg.GatewayPortEnd {
		return fmt.Errorf("upgrade lab gateway port range is exhausted")
	}
	workspace := strings.TrimSpace(stringValue(instance.WorkspacePath))
	binding := &models.InstanceRuntimeBinding{InstanceID: instance.ID, RuntimePodID: pod.ID, RuntimeType: RuntimeTypeOpenClaw, GatewayPort: port, WorkspacePath: workspace, State: "starting", Generation: instance.RuntimeGeneration}
	if err := s.bindings.Create(ctx, binding); err != nil {
		if current, currentErr := s.bindings.GetByInstanceID(ctx, instance.ID); currentErr != nil || current == nil || current.RuntimePodID != pod.ID || current.Generation != instance.RuntimeGeneration {
			return err
		}
	}
	response, err := s.agent.CreateGateway(ctx, strings.TrimSpace(*pod.AgentEndpoint), RuntimeAgentCreateGatewayRequest{GatewayPort: port, InstanceID: instance.ID, UserID: instance.UserID, AgentType: RuntimeTypeOpenClaw, WorkspacePath: workspace, UID: RuntimeLinuxID(instance.ID), GID: RuntimeLinuxID(instance.ID), CPUCores: instance.CPUCores, MemoryMB: instance.MemoryGB * 1024, DiskQuotaMB: instance.DiskGB * 1024, Generation: instance.RuntimeGeneration, UpgradeID: upgradeID, Environment: env})
	if err != nil {
		_ = s.bindings.DeleteByInstanceID(ctx, instance.ID)
		return err
	}
	if err := s.bindings.UpdateGatewayAssignment(ctx, instance.ID, instance.RuntimeGeneration, response.GatewayID, response.PID, response.Status, nil); err != nil {
		return err
	}
	if NormalizeRuntimeGatewayLifecycle(response.Status, nil).Running {
		return s.commitLabGatewayRunning(ctx, instance, response)
	}
	reader, ok := s.agent.(gatewayStateReader)
	deadline := time.Now().Add(2 * time.Minute)
	lastObservation := strings.TrimSpace(response.Status)
	for {
		if ok {
			state, stateErr := reader.GatewayState(ctx, strings.TrimSpace(*pod.AgentEndpoint), response.GatewayID)
			if stateErr == nil && state != nil {
				lifecycle := NormalizeRuntimeGatewayLifecycle(state.State, nil)
				lastObservation = state.State
				if lifecycle.Running {
					return s.commitLabGatewayRunning(ctx, instance, response)
				}
				if lifecycle.Recognized && lifecycle.BindingState == RuntimeGatewayBindingError {
					return fmt.Errorf("gateway entered %s", state.State)
				}
			} else if stateErr != nil && !errors.Is(stateErr, ErrRuntimeAgentUnsupported) && !errors.Is(stateErr, ErrRuntimeAgentNotFound) {
				lastObservation = stateErr.Error()
			}
		}
		// The fixed 7.1 baseline Agent predates GET /v1/gateways/{id}. Its
		// heartbeat is nevertheless authoritative and is already consumed by
		// the normal binding reconciler. Falling back to that shared state keeps
		// baseline creation compatible without weakening 8.1 verification.
		binding, bindingErr := s.bindings.GetByInstanceID(ctx, instance.ID)
		if bindingErr != nil {
			return bindingErr
		}
		if binding != nil && binding.RuntimePodID == pod.ID && binding.Generation == instance.RuntimeGeneration && binding.GatewayID == response.GatewayID {
			lastObservation = binding.State
			if NormalizeRuntimeGatewayLifecycle(binding.State, nil).Running {
				return s.commitLabGatewayRunning(ctx, instance, response)
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("gateway did not become running within 2 minutes (last observation %s)", lastObservation)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func (s *OpenClawUpgradeLabService) commitLabGatewayRunning(ctx context.Context, instance *models.Instance, response *RuntimeAgentCreateGatewayResponse) error {
	if err := s.bindings.UpdateRunning(ctx, instance.ID, instance.RuntimeGeneration, response.GatewayID, response.Port, response.PID); err != nil {
		return err
	}
	return s.instances.UpdateRuntimeState(ctx, instance.ID, "running", instance.RuntimeGeneration, nil)
}

// recoverProvisionedRun closes the only safe crash window in baseline setup:
// the isolated instance and binding are already running, but the browser or
// application stopped before their ownership was committed to the lab row.
// It never adopts an ordinary instance or a binding from another Deployment.
func (s *OpenClawUpgradeLabService) recoverProvisionedRun(ctx context.Context, run *models.OpenClawUpgradeLabRun) *models.OpenClawUpgradeLabRun {
	if s == nil || run == nil || run.Status != "provisioning" || strings.TrimSpace(run.SourceDeployment) == "" {
		return run
	}
	desired := 1
	if run.BeforeJSON != nil {
		var plan upgradeLabProvisionPlan
		if json.Unmarshal([]byte(*run.BeforeJSON), &plan) == nil && plan.InstanceCount >= 1 && plan.InstanceCount <= openClawUpgradeLabMaxCases {
			desired = plan.InstanceCount
		}
	}
	description := fmt.Sprintf("%s%d", openClawUpgradeLabDescription, run.ID)
	rows, err := s.sess.SQL().QueryContext(ctx, `SELECT id FROM instances WHERE description = ? ORDER BY id`, description)
	if err != nil {
		return run
	}
	ids := make([]int, 0, desired)
	for rows.Next() {
		var id int
		if rows.Scan(&id) != nil {
			_ = rows.Close()
			return run
		}
		ids = append(ids, id)
	}
	_ = rows.Close()
	if time.Since(run.UpdatedAt) < openClawUpgradeLabProvisionTimeout {
		return run
	}
	if len(ids) != desired {
		if time.Since(run.UpdatedAt) >= openClawUpgradeLabProvisionTimeout {
			idsRaw, _ := json.Marshal(ids)
			_, _ = s.sess.SQL().ExecContext(ctx, `UPDATE openclaw_upgrade_lab_runs SET instance_ids_json = ?, updated_at = ? WHERE id = ? AND status = 'provisioning'`, string(idsRaw), time.Now().UTC(), run.ID)
			_ = s.failRun(ctx, run.ID, "BASELINE_PROVISIONING_INTERRUPTED", fmt.Errorf("baseline setup stopped after %d of %d test instances", len(ids), desired))
			if failed, getErr := s.getRun(ctx, run.ID); getErr == nil && failed != nil {
				return failed
			}
		}
		return run
	}
	for index, id := range ids {
		instance, getErr := s.instances.GetByID(id)
		if getErr != nil || instance == nil || instance.Status != "running" || instance.UserID <= 0 {
			return run
		}
		expectedWorkspace := RuntimeWorkspacePathWithRoot(s.cfg.WorkspaceRoot, RuntimeTypeOpenClaw, instance.UserID, instance.ID)
		if !sameCleanPath(strings.TrimSpace(stringValue(instance.WorkspacePath)), expectedWorkspace) {
			return run
		}
		binding, bindingErr := s.bindings.GetByInstanceID(ctx, id)
		if bindingErr != nil || binding == nil || binding.State != RuntimeGatewayBindingRunning || binding.Generation != instance.RuntimeGeneration {
			return run
		}
		pod, podErr := s.pods.GetByID(ctx, binding.RuntimePodID)
		if podErr != nil || pod == nil || pod.DeploymentName != run.SourceDeployment || pod.Draining {
			return run
		}
		fixturePath := filepath.Join(expectedWorkspace, "project", "upgrade-lab-continuity.txt")
		fixture := []byte(fmt.Sprintf("openclaw-upgrade-lab run=%d case=%d baseline=2026.7.1-2\n", run.ID, index+1))
		if writeErr := os.WriteFile(fixturePath, fixture, 0o640); writeErr != nil {
			return run
		}
		_ = os.Chown(fixturePath, RuntimeLinuxID(instance.ID), RuntimeLinuxID(instance.ID))
	}
	before, captureErr := s.captureAndValidateBaseline(run.ID, ids)
	if captureErr != nil {
		_ = s.failRun(ctx, run.ID, "BASELINE_PROVISIONING_INTERRUPTED", captureErr)
		if failed, getErr := s.getRun(ctx, run.ID); getErr == nil && failed != nil {
			return failed
		}
		return run
	}
	idsRaw, _ := json.Marshal(ids)
	beforeRaw, _ := json.Marshal(before)
	result, err := s.sess.SQL().ExecContext(ctx, `UPDATE openclaw_upgrade_lab_runs SET status = 'ready', phase = 'baseline_captured', instance_ids_json = ?, before_json = ?, error_code = NULL, error_message = NULL, updated_at = ? WHERE id = ? AND status = 'provisioning'`, string(idsRaw), string(beforeRaw), time.Now().UTC(), run.ID)
	if err != nil {
		return run
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return run
	}
	recovered, err := s.getRun(ctx, run.ID)
	if err != nil || recovered == nil {
		return run
	}
	return recovered
}

func (s *OpenClawUpgradeLabService) resolveBaseline(ctx context.Context) (models.RuntimePod, string, string, error) {
	pods, err := s.inventory.RuntimeDeploymentPods(ctx, RuntimeTypeOpenClaw)
	if err != nil {
		return models.RuntimePod{}, "", "", err
	}
	for _, pod := range pods {
		if isOpenClawUpgradeLabDeployment(pod.DeploymentName) || strings.TrimSpace(pod.DeploymentName) == "" || strings.TrimSpace(pod.Namespace) == "" {
			continue
		}
		host, repositoryName, _, splitErr := splitRegistryImage(pod.ImageRef)
		if splitErr != nil {
			continue
		}
		tagRef := host + "/" + repositoryName + ":" + OpenClawUpgradeLabBaselineTag
		sources := map[string]string{pod.Namespace + "/" + pod.DeploymentName: pod.ImageRef}
		resolved, resolveErr := resolveRuntimeImageReference(ctx, tagRef, sources)
		if resolveErr != nil {
			return models.RuntimePod{}, "", "", resolveErr
		}
		if imageDigestFromReference(resolved) != OpenClawUpgradeLabBaselineDigest {
			return models.RuntimePod{}, "", "", fmt.Errorf("configured 7.1 baseline tag resolved to unexpected digest %s", imageDigestFromReference(resolved))
		}
		return pod, tagRef, resolved, nil
	}
	return models.RuntimePod{}, "", "", fmt.Errorf("no serving OpenClaw Runtime deployment is available as a lab template")
}

func (s *OpenClawUpgradeLabService) waitForLabPod(ctx context.Context, deployment string, timeout time.Duration) (models.RuntimePod, error) {
	deadline := time.Now().Add(timeout)
	for {
		pods, err := s.pods.List(ctx, RuntimeTypeOpenClaw)
		if err != nil {
			return models.RuntimePod{}, err
		}
		for _, pod := range pods {
			if pod.DeploymentName == deployment && pod.AgentEndpoint != nil && strings.TrimSpace(*pod.AgentEndpoint) != "" && !pod.Draining && (pod.State == "standby" || pod.State == "ready") {
				return pod, nil
			}
		}
		if time.Now().After(deadline) {
			return models.RuntimePod{}, fmt.Errorf("upgrade lab Runtime %s did not register within %s", deployment, timeout)
		}
		select {
		case <-ctx.Done():
			return models.RuntimePod{}, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func (s *OpenClawUpgradeLabService) view(ctx context.Context, run *models.OpenClawUpgradeLabRun) (*OpenClawUpgradeLabView, error) {
	ids := decodeLabInstanceIDs(run.InstanceIDsJSON)
	view := &OpenClawUpgradeLabView{Run: run, InstanceIDs: ids, Instances: []models.Instance{}, Checks: decodeLabChecks(run.ChecksJSON), BaselineTag: OpenClawUpgradeLabBaselineTag, BaselineEvidence: decodeLabBaselineEvidence(run.BeforeJSON)}
	// A cleaned run intentionally retains instance ids and manifests as an
	// audit receipt. Those instances no longer exist and must never be loaded or
	// postflight-validated again.
	if run.Status == "cleaned" {
		return view, nil
	}
	for _, id := range ids {
		instance, err := s.instances.GetByID(id)
		if err != nil {
			return nil, err
		}
		if instance != nil {
			view.Instances = append(view.Instances, *instance)
		}
	}
	if run.RolloutID != nil && s.upgrade != nil {
		details, err := s.upgrade.Details(ctx, *run.RolloutID)
		if err != nil {
			return nil, err
		}
		view.Rollout = details
		if details != nil && details.Rollout != nil && !terminalUpgradeLabStatus(run.Status) {
			run.Phase = details.Rollout.Phase
			switch details.Rollout.Status {
			case "finished":
				checks, checkErr := s.finalChecks(run, ids, details)
				if checkErr != nil {
					message := redactUpgradeLabError(checkErr.Error())
					checks = []OpenClawUpgradeLabCheck{{Name: "postflight_data_verification", Passed: false, Message: message}}
					checksRaw, _ := json.Marshal(checks)
					now := time.Now().UTC()
					code := "POSTFLIGHT_DATA_CHECK_FAILED"
					_, _ = s.sess.SQL().ExecContext(ctx, `UPDATE openclaw_upgrade_lab_runs SET status = 'verification_failed', phase = 'postflight', checks_json = ?, error_code = ?, error_message = ?, finished_at = COALESCE(finished_at, ?), updated_at = ? WHERE id = ?`, string(checksRaw), code, message, now, now, run.ID)
					run.Status, run.Phase, run.ErrorCode, run.ErrorMessage = "verification_failed", "postflight", &code, &message
					view.Checks = checks
					break
				}
				view.Checks = checks
				allPassed := true
				for _, check := range checks {
					allPassed = allPassed && check.Passed
				}
				status := "finished"
				if !allPassed {
					status = "verification_failed"
				}
				checksRaw, _ := json.Marshal(checks)
				now := time.Now().UTC()
				_, _ = s.sess.SQL().ExecContext(ctx, `UPDATE openclaw_upgrade_lab_runs SET status = ?, phase = 'postflight', checks_json = ?, finished_at = COALESCE(finished_at, ?), updated_at = ? WHERE id = ?`, status, string(checksRaw), now, now, run.ID)
				run.Status = status
			case "error", "cancelled":
				rollbackStatus := strings.ToLower(strings.TrimSpace(stringValue(details.Rollout.RollbackStatus)))
				if rollbackStatus == "starting" || rollbackStatus == "waiting" || (details.Rollout.AutoRollback && rollbackStatus == "") {
					run.Status = "upgrading"
					run.Phase = "rollback_restore"
					_, _ = s.sess.SQL().ExecContext(ctx, `UPDATE openclaw_upgrade_lab_runs SET status = 'upgrading', phase = 'rollback_restore', updated_at = ? WHERE id = ?`, time.Now().UTC(), run.ID)
					break
				}
				message := stringValue(details.Rollout.ErrorMessage)
				if message == "" {
					message = stringValue(details.Rollout.RollbackError)
				}
				code, phase, display := describeUpgradeLabRolloutFailure(details.Rollout, message)
				_, _ = s.sess.SQL().ExecContext(ctx, `UPDATE openclaw_upgrade_lab_runs SET status = 'failed', phase = ?, error_code = ?, error_message = ?, updated_at = ? WHERE id = ?`, phase, code, display, time.Now().UTC(), run.ID)
				run.Status, run.Phase, run.ErrorCode, run.ErrorMessage = "failed", phase, &code, stringPtrOrNil(display)
			}
		}
	}
	return view, nil
}

func terminalUpgradeLabStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "finished", "verification_failed", "failed", "cleaned":
		return true
	default:
		return false
	}
}

func describeUpgradeLabRolloutFailure(rollout *models.RuntimeRollout, message string) (string, string, string) {
	message = redactUpgradeLabError(message)
	if rollout == nil {
		return "UPGRADE_EXECUTION_FAILED", "upgrade_failed", message
	}
	rollbackStatus := strings.ToLower(strings.TrimSpace(stringValue(rollout.RollbackStatus)))
	rollbackError := redactUpgradeLabError(stringValue(rollout.RollbackError))
	if rollbackError != "" || rollbackStatus == "error" || rollbackStatus == "failed" {
		if rollbackError != "" {
			message = strings.TrimSpace(message + "; rollback: " + rollbackError)
		}
		return "ROLLBACK_RESTORE_FAILED", "rollback_failed", message
	}
	if rollbackStatus == "restored" || rollbackStatus == "finished" || rollbackStatus == "complete" || rollbackStatus == "completed" {
		return "UPGRADE_FAILED_BASELINE_RESTORED", "upgrade_failed_restored", "升级失败，但7.1基线已恢复。原因：" + message
	}
	return "UPGRADE_EXECUTION_FAILED", "upgrade_failed", message
}

func (s *OpenClawUpgradeLabService) finalChecks(run *models.OpenClawUpgradeLabRun, ids []int, details *RuntimeUpgradeDetails) ([]OpenClawUpgradeLabCheck, error) {
	var before map[int]upgradeLabDataManifest
	if run.BeforeJSON == nil || json.Unmarshal([]byte(*run.BeforeJSON), &before) != nil {
		return nil, fmt.Errorf("upgrade lab baseline manifest is unavailable")
	}
	after, err := s.captureDataManifests(ids)
	if err != nil {
		return nil, err
	}
	checks := []OpenClawUpgradeLabCheck{}
	for _, id := range ids {
		checks = append(checks, OpenClawUpgradeLabCheck{Name: fmt.Sprintf("instance_%d_workspace", id), Passed: before[id].Digest != "" && before[id].Digest == after[id].Digest, Expected: before[id].Digest, Actual: after[id].Digest, Message: "project files must remain byte-identical"})
	}
	for _, item := range details.Items {
		baseline := before[item.InstanceID].Sessions
		instance, getErr := s.instances.GetByID(item.InstanceID)
		workspace := ""
		if getErr == nil && instance != nil {
			workspace = stringValue(instance.WorkspacePath)
		}
		archiveOK, archived, archiveErr := verifyUpgradeLabSessionArchive(workspace, baseline.SourceFiles)
		archiveMessage := "every 7.1 session source file must remain byte-identical in the official import archive"
		if archiveErr != nil {
			archiveMessage = archiveErr.Error()
		}
		checks = append(checks, OpenClawUpgradeLabCheck{Name: fmt.Sprintf("instance_%d_session_archive", item.InstanceID), Passed: archiveOK, Expected: strconv.Itoa(len(baseline.SourceFiles)), Actual: strconv.Itoa(archived), Message: archiveMessage})
		ancillaryOK := maps.Equal(baseline.AncillaryFiles, after[item.InstanceID].Sessions.AncillaryFiles)
		checks = append(checks, OpenClawUpgradeLabCheck{Name: fmt.Sprintf("instance_%d_session_auxiliary", item.InstanceID), Passed: ancillaryOK, Expected: baseline.AncillaryFilesSHA256, Actual: after[item.InstanceID].Sessions.AncillaryFilesSHA256, Message: "session prompt caches and auxiliary files must remain byte-identical in place"})
		var evidence struct {
			Migration RuntimeAgentSessionSQLiteMigration `json:"migration"`
		}
		if item.PostflightJSON != nil {
			_ = json.Unmarshal([]byte(*item.PostflightJSON), &evidence)
		}
		catalogOK := evidence.Migration.Status == "validated" && evidence.Migration.SessionCount == baseline.SessionCount && evidence.Migration.SessionCatalogSHA256 == baseline.CatalogSHA256
		checks = append(checks, OpenClawUpgradeLabCheck{Name: fmt.Sprintf("instance_%d_session_catalog", item.InstanceID), Passed: catalogOK, Expected: fmt.Sprintf("%d:%s", baseline.SessionCount, baseline.CatalogSHA256), Actual: fmt.Sprintf("%d:%s", evidence.Migration.SessionCount, evidence.Migration.SessionCatalogSHA256), Message: "8.1 official Session Catalog must contain the same session keys and ids as 7.1"})
		checks = append(checks, OpenClawUpgradeLabCheck{Name: fmt.Sprintf("instance_%d_gateway", item.InstanceID), Passed: item.State == "verified" || item.State == "gateway_verified", Actual: item.State, Message: "migrated gateway must pass the shared upgrade restart verification"})
	}
	return checks, nil
}

func (s *OpenClawUpgradeLabService) captureDataManifests(ids []int) (map[int]upgradeLabDataManifest, error) {
	result := make(map[int]upgradeLabDataManifest, len(ids))
	for _, id := range ids {
		instance, err := s.instances.GetByID(id)
		if err != nil || instance == nil {
			return nil, fmt.Errorf("upgrade lab instance %d is unavailable", id)
		}
		manifest, err := inspectUpgradeLabWorkspace(stringValue(instance.WorkspacePath))
		if err != nil {
			return nil, err
		}
		result[id] = manifest
	}
	return result, nil
}

func inspectUpgradeLabWorkspace(workspace string) (upgradeLabDataManifest, error) {
	manifest, err := inspectUpgradeLabProjectData(workspace)
	if err != nil {
		return manifest, err
	}
	manifest.Sessions, err = inspectUpgradeLabLegacySessions(workspace)
	return manifest, err
}

func inspectUpgradeLabProjectData(workspace string) (upgradeLabDataManifest, error) {
	root := filepath.Join(filepath.Clean(workspace), "project")
	manifest := upgradeLabDataManifest{Files: map[string]string{}}
	if !pathWithin(workspace, root) {
		return manifest, fmt.Errorf("upgrade lab project path escaped its workspace")
	}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("upgrade lab project contains unsupported entry %s", path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		hash := sha256.New()
		_, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		manifest.Files[filepath.ToSlash(relative)] = hex.EncodeToString(hash.Sum(nil))
		return nil
	})
	if err != nil {
		return manifest, err
	}
	keys := make([]string, 0, len(manifest.Files))
	for key := range manifest.Files {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	hash := sha256.New()
	for _, key := range keys {
		_, _ = io.WriteString(hash, key+"\x00"+manifest.Files[key]+"\n")
	}
	manifest.Digest = hex.EncodeToString(hash.Sum(nil))
	return manifest, nil
}

func inspectUpgradeLabLegacySessions(workspace string) (upgradeLabSessionManifest, error) {
	manifest := upgradeLabSessionManifest{SourceFiles: map[string]string{}, AncillaryFiles: map[string]string{}}
	root := filepath.Join(filepath.Clean(workspace), "home", ".openclaw", "agents")
	if !pathWithin(workspace, root) {
		return manifest, errors.New("upgrade lab session path escaped its workspace")
	}
	catalog := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, os.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if info.IsDir() || !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		slash := filepath.ToSlash(rel)
		lower := strings.ToLower(slash)
		if strings.Contains(lower, "session-sqlite-import-archive/") || strings.Contains(lower, "session-sqlite-migration-runs/") || (!strings.Contains(lower, "/sessions/") && !strings.HasSuffix(lower, "/sessions.json")) {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(raw)
		digestHex := hex.EncodeToString(digest[:])
		if isUpgradeLabLegacySessionSource(lower) {
			manifest.SourceFiles[slash] = digestHex
		} else {
			manifest.AncillaryFiles[slash] = digestHex
		}
		if strings.HasSuffix(lower, "sessions.json") {
			var entries map[string]struct {
				SessionID string `json:"sessionId"`
			}
			if json.Unmarshal(raw, &entries) == nil {
				for key, entry := range entries {
					if strings.TrimSpace(key) != "" && strings.TrimSpace(entry.SessionID) != "" {
						catalog[strings.TrimSpace(key)] = strings.TrimSpace(entry.SessionID)
					}
				}
			}
		}
		if strings.HasSuffix(lower, ".jsonl") && !strings.Contains(lower, ".trajectory") {
			if err := countUpgradeLabMessages(raw, &manifest); err != nil {
				return fmt.Errorf("read upgrade lab session transcript %s: %w", slash, err)
			}
		}
		return nil
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return manifest, err
	}
	manifest.SessionCount, manifest.CatalogSHA256 = canonicalUpgradeLabCatalog(catalog)
	manifest.SourceFilesSHA256 = digestUpgradeLabFiles(manifest.SourceFiles)
	manifest.AncillaryFilesSHA256 = digestUpgradeLabFiles(manifest.AncillaryFiles)
	return manifest, nil
}

func isUpgradeLabLegacySessionSource(lowerPath string) bool {
	lowerPath = strings.TrimSpace(filepath.ToSlash(lowerPath))
	base := strings.ToLower(filepath.Base(lowerPath))
	if base == "sessions.json" {
		return true
	}
	return strings.Contains(lowerPath, "/sessions/") && (strings.HasSuffix(lowerPath, ".jsonl") || strings.HasSuffix(lowerPath, ".json"))
}

func countUpgradeLabMessages(raw []byte, manifest *upgradeLabSessionManifest) error {
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var record struct {
			Type    string `json:"type"`
			Message struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(scanner.Bytes(), &record) != nil || record.Type != "message" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(record.Message.Role)) {
		case "user":
			manifest.UserMessageCount++
			if !strings.HasPrefix(strings.TrimSpace(upgradeLabMessageText(record.Message.Content)), "[OpenClaw heartbeat poll]") {
				manifest.InteractiveUserMessageCount++
			}
		case "assistant":
			manifest.AssistantMessageCount++
		}
	}
	return scanner.Err()
}

func upgradeLabConversationCompleted(workspace, marker string) (bool, error) {
	return inspectUpgradeLabCompletedConversation(workspace, func(text string) bool {
		return strings.Contains(text, marker)
	})
}

func upgradeLabAnyCompletedConversation(workspace string) bool {
	completed, _ := inspectUpgradeLabCompletedConversation(workspace, func(text string) bool {
		return !strings.HasPrefix(strings.TrimSpace(text), "[OpenClaw heartbeat poll]")
	})
	return completed
}

func inspectUpgradeLabCompletedConversation(workspace string, matchesUser func(string) bool) (bool, error) {
	root := filepath.Join(filepath.Clean(workspace), "home", ".openclaw", "agents")
	if !pathWithin(workspace, root) {
		return false, errors.New("upgrade lab session path escaped its workspace")
	}
	completed := false
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, os.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if completed || info.IsDir() || !info.Mode().IsRegular() || !strings.HasSuffix(strings.ToLower(path), ".jsonl") || strings.Contains(strings.ToLower(filepath.ToSlash(path)), "session-sqlite-import-archive/") {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		seenUser := false
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			role, text, ok := upgradeLabMessageRecord(scanner.Bytes())
			if !ok {
				continue
			}
			if role == "user" && matchesUser(text) {
				seenUser = true
				continue
			}
			if seenUser && role == "assistant" && strings.TrimSpace(text) != "" {
				completed = true
				break
			}
		}
		return scanner.Err()
	})
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return completed, err
}

func upgradeLabMessageRecord(raw []byte) (string, string, bool) {
	var record struct {
		Type    string `json:"type"`
		Message struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(raw, &record) != nil || record.Type != "message" {
		return "", "", false
	}
	role := strings.ToLower(strings.TrimSpace(record.Message.Role))
	if role != "user" && role != "assistant" {
		return "", "", false
	}
	return role, upgradeLabMessageText(record.Message.Content), true
}

func upgradeLabMessageText(content any) string {
	switch value := content.(type) {
	case string:
		return value
	case []any:
		var parts []string
		for _, item := range value {
			if object, ok := item.(map[string]any); ok {
				if text, ok := object["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

func canonicalUpgradeLabCatalog(entries map[string]string) (int, string) {
	lines := make([]string, 0, len(entries))
	for key, sessionID := range entries {
		lines = append(lines, key+"\x00"+sessionID+"\n")
	}
	sort.Strings(lines)
	hash := sha256.New()
	for _, line := range lines {
		_, _ = io.WriteString(hash, line)
	}
	return len(lines), hex.EncodeToString(hash.Sum(nil))
}

func digestUpgradeLabFiles(files map[string]string) string {
	keys := make([]string, 0, len(files))
	for key := range files {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	hash := sha256.New()
	for _, key := range keys {
		_, _ = io.WriteString(hash, key+"\x00"+files[key]+"\n")
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func verifyUpgradeLabSessionArchive(workspace string, sourceFiles map[string]string) (bool, int, error) {
	if strings.TrimSpace(workspace) == "" {
		return false, 0, errors.New("upgrade lab workspace is unavailable")
	}
	want := map[string]int{}
	for _, digest := range sourceFiles {
		want[digest]++
	}
	found := map[string]int{}
	root := filepath.Join(filepath.Clean(workspace), "home", ".openclaw", "agents")
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || !info.Mode().IsRegular() || !strings.Contains(strings.ToLower(filepath.ToSlash(path)), "/session-sqlite-import-archive/") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(raw)
		found[hex.EncodeToString(digest[:])]++
		return nil
	})
	if err != nil {
		return false, 0, err
	}
	matched := 0
	for digest, count := range want {
		available := found[digest]
		if available < count {
			return false, matched + available, nil
		}
		matched += count
	}
	return len(sourceFiles) > 0, matched, nil
}

func manualUpgradeLabProjectFileCount(files map[string]string) int {
	count := 0
	for name := range files {
		if filepath.ToSlash(name) != "upgrade-lab-continuity.txt" {
			count++
		}
	}
	return count
}

func decodeLabBaselineEvidence(raw *string) []OpenClawUpgradeLabBaselineEvidence {
	var manifests map[int]upgradeLabDataManifest
	if raw == nil || json.Unmarshal([]byte(*raw), &manifests) != nil {
		return []OpenClawUpgradeLabBaselineEvidence{}
	}
	ids := make([]int, 0, len(manifests))
	for id := range manifests {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	result := make([]OpenClawUpgradeLabBaselineEvidence, 0, len(ids))
	for _, id := range ids {
		manifest := manifests[id]
		result = append(result, OpenClawUpgradeLabBaselineEvidence{InstanceID: id, ProjectFileCount: len(manifest.Files), ManualProjectFileCount: manualUpgradeLabProjectFileCount(manifest.Files), SessionCount: manifest.Sessions.SessionCount, UserMessageCount: manifest.Sessions.UserMessageCount, AssistantMessageCount: manifest.Sessions.AssistantMessageCount, InteractiveUserMessageCount: manifest.Sessions.InteractiveUserMessageCount, ProjectSHA256: manifest.Digest, SessionCatalogSHA256: manifest.Sessions.CatalogSHA256, SessionSourceSHA256: manifest.Sessions.SourceFilesSHA256})
	}
	return result
}

func (s *OpenClawUpgradeLabService) getRun(ctx context.Context, id int64) (*models.OpenClawUpgradeLabRun, error) {
	var run models.OpenClawUpgradeLabRun
	if err := s.sess.Collection(run.TableName()).Find(db.Cond{"id": id}).One(&run); err != nil {
		if errors.Is(err, db.ErrNoMoreRows) {
			return nil, nil
		}
		return nil, err
	}
	return &run, nil
}

func (s *OpenClawUpgradeLabService) failRun(ctx context.Context, id int64, code string, cause error) error {
	message := redactUpgradeLabError(cause.Error())
	_, _ = s.sess.SQL().ExecContext(ctx, `UPDATE openclaw_upgrade_lab_runs SET status = 'failed', phase = 'failed', error_code = ?, error_message = ?, updated_at = ? WHERE id = ?`, code, message, time.Now().UTC(), id)
	return fmt.Errorf("%s: %s", code, message)
}

func decodeLabInstanceIDs(raw *string) []int {
	var ids []int
	if raw != nil {
		_ = json.Unmarshal([]byte(*raw), &ids)
	}
	return normalizedPositiveIDs(ids)
}

func decodeLabChecks(raw *string) []OpenClawUpgradeLabCheck {
	checks := []OpenClawUpgradeLabCheck{}
	if raw != nil {
		_ = json.Unmarshal([]byte(*raw), &checks)
	}
	return checks
}

func upgradeLabRunToken(id int64) string { return fmt.Sprintf("lab-r%d", id) }

func redactUpgradeLabError(value string) string {
	value = strings.TrimSpace(value)
	for _, marker := range []string{"igt_", "Bearer ", "token=", "api_key="} {
		if index := strings.Index(strings.ToLower(value), strings.ToLower(marker)); index >= 0 {
			value = value[:index] + marker + "[redacted]"
		}
	}
	if len(value) > 2000 {
		value = value[:2000]
	}
	return value
}
