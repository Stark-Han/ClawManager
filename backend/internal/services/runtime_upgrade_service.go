package services

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"clawreef/internal/models"
	"clawreef/internal/repository"

	"github.com/upper/db/v4"
)

const targetOpenClawUpgradeVersion = "2026.8.1"

var openClawUpgradeRequiredCapabilities = []string{
	"openclaw.state.sqlite",
	"openclaw.automation.rpc",
	"openclaw.backup.sqlite",
	"openclaw.database.preflight",
	"openclaw.plugin.capability-consent",
	"openclaw.gateway.stop-confirm",
	"openclaw.workspace.atomic-restore",
	"openclaw.workspace.snapshot-v1",
	"openclaw.workspace.writer-lease",
	"openclaw.session-sqlite-migrate-v1",
	"redis-team.group-hooks-v1",
}

type RuntimeUpgradePreflightRequest struct {
	TargetImageRef string `json:"target_image_ref"`
	BatchSize      int    `json:"batch_size"`
	MaxUnavailable int    `json:"max_unavailable"`
	AutoRollback   bool   `json:"auto_rollback"`
	ActorUserID    *int   `json:"-"`
}

type RuntimeUpgradePreflightResult struct {
	Rollout                 *models.RuntimeRollout `json:"rollout"`
	Passed                  bool                   `json:"passed"`
	Blockers                []string               `json:"blockers"`
	Warnings                []string               `json:"warnings"`
	InstanceCount           int                    `json:"instance_count"`
	TeamCount               int                    `json:"team_count"`
	OpenClawTeamMemberCount int                    `json:"openclaw_team_member_count"`
	HermesTeamMemberCount   int                    `json:"hermes_team_member_count"`
	RequiredCapabilities    []string               `json:"required_capabilities"`
}

type RuntimeUpgradeDetails struct {
	Rollout *models.RuntimeRollout       `json:"rollout"`
	Items   []models.RuntimeUpgradeItem  `json:"items"`
	Audits  []models.RuntimeUpgradeAudit `json:"audits"`
}

type runtimeUpgradeCandidate struct {
	InstanceID    int
	UserID        int
	RuntimePodID  *int64
	GatewayID     string
	Generation    int
	BindingState  string
	WorkspacePath string
	TeamID        *int
	TeamMemberID  *int
	MemberKey     string
	Role          string
	RuntimeType   string
	Availability  string
	MemberStatus  string
	SourceVersion string
	AgentEndpoint string
	Capabilities  []string
}

type runtimeWorkspaceInventory struct {
	FileCount         int64  `json:"file_count"`
	DirectoryCount    int64  `json:"directory_count"`
	SymlinkCount      int64  `json:"symlink_count"`
	TotalBytes        int64  `json:"total_bytes"`
	SQLiteFileCount   int    `json:"sqlite_file_count"`
	SQLiteHeaderValid bool   `json:"sqlite_header_valid"`
	ManifestSHA256    string `json:"manifest_sha256"`
}

type RuntimeUpgradeService struct {
	sess            db.Session
	rollouts        repository.RuntimeRolloutRepository
	pods            repository.RuntimePodRepository
	bindings        repository.InstanceRuntimeBindingRepository
	agent           RuntimeAgentClient
	workspaceRoot   string
	redisURL        string
	teamMaintenance TeamUpgradeMaintenanceController
	deployments     RuntimeDeploymentInventoryProvider
}

type TeamUpgradeMaintenanceController interface {
	SetRuntimeUpgradeMaintenance(ctx context.Context, teamIDs []int, rolloutID int64, enabled bool) error
}

type RuntimeDeploymentInventoryProvider interface {
	RuntimeDeploymentPods(ctx context.Context, runtimeType string) ([]models.RuntimePod, error)
}

func (s *RuntimeUpgradeService) SetTeamMaintenanceController(controller TeamUpgradeMaintenanceController) {
	s.teamMaintenance = controller
}

func (s *RuntimeUpgradeService) SetDeploymentInventoryProvider(provider RuntimeDeploymentInventoryProvider) {
	s.deployments = provider
}

func NewRuntimeUpgradeService(sess db.Session, rollouts repository.RuntimeRolloutRepository, pods repository.RuntimePodRepository, bindings repository.InstanceRuntimeBindingRepository, agent RuntimeAgentClient, workspaceRoot, redisURL string) *RuntimeUpgradeService {
	return &RuntimeUpgradeService{
		sess:          sess,
		rollouts:      rollouts,
		pods:          pods,
		bindings:      bindings,
		agent:         agent,
		workspaceRoot: filepath.Clean(strings.TrimSpace(workspaceRoot)),
		redisURL:      strings.TrimSpace(redisURL),
	}
}

func (s *RuntimeUpgradeService) Preflight(ctx context.Context, req RuntimeUpgradePreflightRequest) (*RuntimeUpgradePreflightResult, error) {
	if s == nil || s.sess == nil || s.rollouts == nil {
		return nil, fmt.Errorf("runtime upgrade service is not configured")
	}
	target := strings.TrimSpace(req.TargetImageRef)
	result := &RuntimeUpgradePreflightResult{RequiredCapabilities: append([]string(nil), openClawUpgradeRequiredCapabilities...)}
	if target == "" {
		result.Blockers = append(result.Blockers, "target_image_ref is required")
	}
	digest := imageDigestFromReference(target)
	if digest == "" {
		result.Blockers = append(result.Blockers, "target image must be immutable and include @sha256:<64 hex>")
	}
	if !strings.Contains(strings.ToLower(target), "openclaw") {
		result.Blockers = append(result.Blockers, "target image must be an OpenClaw runtime image")
	}

	candidates, sourceImages, warnings, blockers, err := s.inspectCandidates(ctx)
	if err != nil {
		return nil, err
	}
	result.Warnings = append(result.Warnings, warnings...)
	result.Blockers = append(result.Blockers, blockers...)
	if len(sourceImages) == 0 {
		result.Blockers = append(result.Blockers, "no current OpenClaw deployment image is available for rollback")
	}
	if len(candidates) == 0 {
		result.Blockers = append(result.Blockers, "no OpenClaw Lite instance was discovered; refusing an empty data-safe rollout")
	}
	teamIDs := map[int]struct{}{}
	for _, candidate := range candidates {
		if candidate.TeamID != nil {
			teamIDs[*candidate.TeamID] = struct{}{}
			result.OpenClawTeamMemberCount++
		}
	}
	result.InstanceCount = len(candidates)
	result.TeamCount = len(teamIDs)
	hermesCount, teamBlockers, err := s.inspectTeamConsistency(ctx, teamIDs)
	if err != nil {
		return nil, err
	}
	result.HermesTeamMemberCount = hermesCount
	result.Blockers = append(result.Blockers, teamBlockers...)
	result.Blockers = uniqueSortedStrings(result.Blockers)
	result.Warnings = uniqueSortedStrings(result.Warnings)

	preflightID, err := randomUpgradeID()
	if err != nil {
		return nil, err
	}
	planFingerprint := runtimeUpgradeFingerprint(target, candidates, sourceImages)
	preflightSummary, _ := json.Marshal(map[string]any{
		"passed":                     len(result.Blockers) == 0,
		"blockers":                   result.Blockers,
		"warnings":                   result.Warnings,
		"instance_count":             result.InstanceCount,
		"team_count":                 result.TeamCount,
		"openclaw_team_member_count": result.OpenClawTeamMemberCount,
		"hermes_team_member_count":   result.HermesTeamMemberCount,
	})
	sourceImagesJSON, _ := json.Marshal(sourceImages)
	requiredJSON, _ := json.Marshal(openClawUpgradeRequiredCapabilities)
	fingerprint := planFingerprint
	preflight := preflightID
	preflightRaw := string(preflightSummary)
	sourceRaw := string(sourceImagesJSON)
	requiredRaw := string(requiredJSON)
	rollout := &models.RuntimeRollout{
		RuntimeType:              RuntimeTypeOpenClaw,
		TargetImageRef:           target,
		SourceImagesJSON:         &sourceRaw,
		TargetImageDigest:        stringPtrOrNil(digest),
		Status:                   map[bool]string{true: "preflight_passed", false: "preflight_blocked"}[len(result.Blockers) == 0],
		Phase:                    "preflight",
		PreflightID:              &preflight,
		PlanFingerprint:          &fingerprint,
		PreflightJSON:            &preflightRaw,
		RequiredCapabilitiesJSON: &requiredRaw,
		BatchSize:                maxInt(req.BatchSize, 1),
		MaxUnavailable:           0,
		StartedBy:                req.ActorUserID,
		AutoRollback:             req.AutoRollback,
	}
	if err := s.rollouts.Create(ctx, rollout); err != nil {
		return nil, err
	}
	if err := s.insertUpgradeItems(ctx, rollout.ID, candidates); err != nil {
		return nil, err
	}
	result.Rollout = rollout
	result.Passed = len(result.Blockers) == 0
	if err := s.audit(ctx, &rollout.ID, req.ActorUserID, "preflight", "preflight", map[bool]string{true: "passed", false: "blocked"}[result.Passed], map[string]any{
		"plan_fingerprint":    planFingerprint,
		"target_image_digest": digest,
		"blocker_count":       len(result.Blockers),
		"warning_count":       len(result.Warnings),
		"instance_count":      len(candidates),
	}); err != nil {
		return nil, fmt.Errorf("persist runtime upgrade preflight audit: %w", err)
	}
	return result, nil
}

func (s *RuntimeUpgradeService) ConfirmPreflight(ctx context.Context, preflightID, targetImage string, actor *int) (*models.RuntimeRollout, error) {
	preflightID = strings.TrimSpace(preflightID)
	if preflightID == "" {
		return nil, fmt.Errorf("preflight_id is required for OpenClaw rollout")
	}
	var rollout models.RuntimeRollout
	if err := s.sess.Collection("runtime_rollouts").Find(db.Cond{"preflight_id": preflightID}).One(&rollout); err != nil {
		if err == db.ErrNoMoreRows {
			return nil, fmt.Errorf("runtime upgrade preflight not found")
		}
		return nil, err
	}
	if rollout.Status != "preflight_passed" || rollout.Phase != "preflight" {
		return nil, fmt.Errorf("runtime upgrade preflight is not executable: %s/%s", rollout.Status, rollout.Phase)
	}
	if strings.TrimSpace(targetImage) != "" && strings.TrimSpace(targetImage) != strings.TrimSpace(rollout.TargetImageRef) {
		return nil, fmt.Errorf("target image changed after preflight")
	}
	if rollout.PlanFingerprint == nil || strings.TrimSpace(*rollout.PlanFingerprint) == "" {
		return nil, fmt.Errorf("runtime upgrade preflight fingerprint is missing")
	}
	err := s.sess.TxContext(ctx, func(tx db.Session) error {
		result, err := tx.SQL().ExecContext(ctx, `UPDATE runtime_rollouts SET status = 'pending', started_by = ?, updated_at = ? WHERE id = ? AND status = 'preflight_passed'`, actor, time.Now().UTC(), rollout.ID)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil || affected != 1 {
			return fmt.Errorf("runtime upgrade preflight was already consumed")
		}
		return insertRuntimeUpgradeAudit(ctx, tx, &rollout.ID, actor, "confirm", "preflight", "accepted", map[string]any{"preflight_id": preflightID, "plan_fingerprint": *rollout.PlanFingerprint})
	}, nil)
	if err != nil {
		return nil, err
	}
	rollout.Status = "pending"
	rollout.StartedBy = actor
	return &rollout, nil
}

func (s *RuntimeUpgradeService) Details(ctx context.Context, rolloutID int64) (*RuntimeUpgradeDetails, error) {
	rollout, err := s.rollouts.GetByID(ctx, rolloutID)
	if err != nil || rollout == nil {
		return nil, err
	}
	items, err := s.listUpgradeItems(ctx, rolloutID)
	if err != nil {
		return nil, err
	}
	var audits []models.RuntimeUpgradeAudit
	if err := s.sess.Collection("runtime_upgrade_audits").Find(db.Cond{"rollout_id": rolloutID}).OrderBy("created_at", "id").All(&audits); err != nil {
		return nil, err
	}
	return &RuntimeUpgradeDetails{Rollout: rollout, Items: items, Audits: audits}, nil
}

func (s *RuntimeUpgradeService) Prepare(ctx context.Context, rollout *models.RuntimeRollout) error {
	if rollout == nil || rollout.PreflightID == nil || rollout.PlanFingerprint == nil {
		return fmt.Errorf("OpenClaw rollout requires a successful persisted preflight")
	}
	if rollout.Status != "preflight_passed" && rollout.Status != "pending" && rollout.Status != "running" {
		return fmt.Errorf("OpenClaw rollout preflight is not executable: %s", rollout.Status)
	}
	candidates, sourceImages, _, blockers, err := s.inspectCandidates(ctx)
	if err != nil {
		return err
	}
	teamIDs := map[int]struct{}{}
	for _, candidate := range candidates {
		if candidate.TeamID != nil {
			teamIDs[*candidate.TeamID] = struct{}{}
		}
	}
	_, teamBlockers, err := s.inspectTeamConsistency(ctx, teamIDs)
	if err != nil {
		return err
	}
	blockers = append(blockers, teamBlockers...)
	if len(blockers) > 0 {
		return fmt.Errorf("runtime upgrade state changed after preflight: %s", strings.Join(uniqueSortedStrings(blockers), "; "))
	}
	if got := runtimeUpgradeFingerprint(rollout.TargetImageRef, candidates, sourceImages); got != *rollout.PlanFingerprint {
		return fmt.Errorf("runtime upgrade inventory changed after preflight; run preflight again")
	}
	if err := s.updateRolloutPhase(ctx, rollout.ID, "maintenance"); err != nil {
		return err
	}
	if err := s.setTeamMaintenance(ctx, teamIDs, rollout.ID, true); err != nil {
		return err
	}
	prepared := false
	defer func() {
		if !prepared {
			_ = s.setTeamMaintenance(context.Background(), teamIDs, rollout.ID, false)
		}
	}()
	// Close the preflight-to-maintenance race: once dispatch is fenced, re-read
	// every durable Team ledger before stopping a single gateway.
	if _, fencedBlockers, err := s.inspectTeamConsistency(ctx, teamIDs); err != nil {
		return err
	} else if len(fencedBlockers) > 0 {
		return fmt.Errorf("Team state changed while entering runtime maintenance: %s", strings.Join(uniqueSortedStrings(fencedBlockers), "; "))
	}

	sortRuntimeUpgradeCandidates(candidates)
	for _, candidate := range candidates {
		if err := s.stopCandidateGateway(ctx, candidate); err != nil {
			return fmt.Errorf("stop instance %d before snapshot: %w", candidate.InstanceID, err)
		}
	}
	if err := s.updateRolloutPhase(ctx, rollout.ID, "snapshot"); err != nil {
		return err
	}
	existingItems, err := s.listUpgradeItems(ctx, rollout.ID)
	if err != nil {
		return err
	}
	existingByInstance := make(map[int]models.RuntimeUpgradeItem, len(existingItems))
	for _, item := range existingItems {
		existingByInstance[item.InstanceID] = item
	}
	for _, candidate := range candidates {
		if existing, ok := existingByInstance[candidate.InstanceID]; ok && existing.State == "snapshotted" && existing.SnapshotRef != nil && existing.WorkspaceManifestSHA256 != nil {
			current, err := inspectRuntimeWorkspace(candidate.WorkspacePath)
			if err != nil {
				return fmt.Errorf("resume snapshot validation for instance %d: %w", candidate.InstanceID, err)
			}
			if current.ManifestSHA256 != strings.TrimSpace(*existing.WorkspaceManifestSHA256) {
				return fmt.Errorf("instance %d workspace changed after its persisted snapshot", candidate.InstanceID)
			}
			if err := verifyLocalSnapshotArchive(s.workspaceRoot, *existing.SnapshotRef); err != nil {
				return fmt.Errorf("resume snapshot verification for instance %d: %w", candidate.InstanceID, err)
			}
			continue
		}
		snapshot, inventory, err := s.snapshotCandidate(ctx, rollout.ID, candidate)
		if err != nil {
			return fmt.Errorf("snapshot instance %d: %w", candidate.InstanceID, err)
		}
		if err := s.updateUpgradeItemSnapshot(ctx, rollout.ID, candidate.InstanceID, snapshot, inventory); err != nil {
			return err
		}
	}
	prepared = true
	if err := s.audit(ctx, &rollout.ID, rollout.StartedBy, "prepare", "snapshot", "passed", map[string]any{"instance_count": len(candidates), "team_count": len(teamIDs)}); err != nil {
		return err
	}
	return s.updateRolloutPhase(ctx, rollout.ID, "image_rollout")
}

func (s *RuntimeUpgradeService) ValidateTargetRuntime(ctx context.Context, rollout *models.RuntimeRollout, pods []models.RuntimePod) (bool, error) {
	if rollout == nil {
		return false, fmt.Errorf("rollout is required")
	}
	readyTarget := 0
	for _, pod := range pods {
		if pod.RuntimeType != RuntimeTypeOpenClaw || pod.State != "ready" || pod.Draining || strings.TrimSpace(pod.ImageRef) != strings.TrimSpace(rollout.TargetImageRef) {
			continue
		}
		readyTarget++
		if value := stringValue(pod.OpenClawVersion); value != targetOpenClawUpgradeVersion {
			return false, fmt.Errorf("target pod %s reports OpenClaw %q, want %s", pod.PodName, value, targetOpenClawUpgradeVersion)
		}
		if value := stringValue(pod.SessionStore); value != "sqlite" {
			return false, fmt.Errorf("target pod %s reports session store %q, want sqlite", pod.PodName, value)
		}
		if value := stringValue(pod.AgentProtocolVersion); value != "openclaw-upgrade-v1" {
			return false, fmt.Errorf("target pod %s reports agent protocol %q, want openclaw-upgrade-v1", pod.PodName, value)
		}
		if value := stringValue(pod.TeamPluginVersion); value != "0.3.0" {
			return false, fmt.Errorf("target pod %s reports Redis Team plugin %q, want 0.3.0", pod.PodName, value)
		}
		if rollout.TargetImageDigest != nil && stringValue(pod.ImageDigest) != strings.TrimSpace(*rollout.TargetImageDigest) {
			return false, fmt.Errorf("target pod %s reports image digest %q, want %s", pod.PodName, stringValue(pod.ImageDigest), strings.TrimSpace(*rollout.TargetImageDigest))
		}
		for _, capability := range openClawUpgradeRequiredCapabilities {
			if !containsString(pod.Capabilities(), capability) {
				return false, fmt.Errorf("target pod %s lacks required capability %s", pod.PodName, capability)
			}
		}
	}
	if readyTarget == 0 {
		return false, nil
	}
	items, err := s.listUpgradeItems(ctx, rollout.ID)
	if err != nil {
		return false, err
	}
	if len(items) == 0 {
		return false, fmt.Errorf("OpenClaw data-safe rollout has no persisted upgrade items")
	}
	if rollout.Phase != "session_migration" && rollout.Phase != "gateway_restart" && rollout.Phase != "postflight" {
		for _, item := range items {
			workspace, err := s.workspaceForInstance(item.InstanceID)
			if err != nil {
				return false, err
			}
			inventory, err := inspectRuntimeWorkspace(workspace)
			if err != nil {
				return false, fmt.Errorf("pre-restart workspace validation for instance %d: %w", item.InstanceID, err)
			}
			if item.WorkspaceManifestSHA256 == nil || inventory.ManifestSHA256 != strings.TrimSpace(*item.WorkspaceManifestSHA256) {
				return false, fmt.Errorf("instance %d workspace changed between snapshot and target restart", item.InstanceID)
			}
		}
		if err := s.updateRolloutPhase(ctx, rollout.ID, "session_migration"); err != nil {
			return false, err
		}
		rollout.Phase = "session_migration"
	}
	if rollout.Phase != "gateway_restart" && rollout.Phase != "postflight" {
		var targetPod *models.RuntimePod
		for index := range pods {
			pod := &pods[index]
			if pod.RuntimeType == RuntimeTypeOpenClaw && pod.State == "ready" && !pod.Draining && strings.TrimSpace(pod.ImageRef) == strings.TrimSpace(rollout.TargetImageRef) && stringValue(pod.AgentEndpoint) != "" {
				targetPod = pod
				break
			}
		}
		if targetPod == nil {
			return false, nil
		}
		upgradeAgent, ok := s.agent.(RuntimeUpgradeAgentClient)
		if !ok {
			return false, fmt.Errorf("target Runtime Agent does not implement session SQLite migration")
		}
		for _, item := range items {
			if item.State == "migrated" || item.State == "restart_ready" || item.State == "gateway_verified" || item.State == "verified" {
				continue
			}
			if item.State != "snapshotted" {
				return false, fmt.Errorf("instance %d has unexpected migration state %q", item.InstanceID, item.State)
			}
			var userID, generation int
			row, queryErr := s.sess.SQL().QueryRowContext(ctx, `SELECT user_id, runtime_generation FROM instances WHERE id = ?`, item.InstanceID)
			if queryErr != nil {
				return false, queryErr
			}
			if scanErr := row.Scan(&userID, &generation); scanErr != nil {
				return false, scanErr
			}
			leaseToken, tokenErr := randomUpgradeID()
			if tokenErr != nil {
				return false, tokenErr
			}
			request := RuntimeAgentWorkspaceRequest{RolloutID: strconv.FormatInt(rollout.ID, 10), UserID: userID, InstanceID: item.InstanceID, Generation: generation, UID: RuntimeLinuxID(item.InstanceID), GID: RuntimeLinuxID(item.InstanceID), LeaseToken: leaseToken}
			lease := RuntimeAgentWriterLeaseRequest{RuntimeAgentWorkspaceRequest: request, Token: leaseToken, TTLSeconds: 3600}
			if err := upgradeAgent.AcquireWriterLease(ctx, stringValue(targetPod.AgentEndpoint), lease); err != nil {
				return false, fmt.Errorf("acquire migration writer lease for instance %d: %w", item.InstanceID, err)
			}
			migration, migrateErr := upgradeAgent.MigrateSessionSQLite(ctx, stringValue(targetPod.AgentEndpoint), request)
			releaseErr := upgradeAgent.ReleaseWriterLease(ctx, stringValue(targetPod.AgentEndpoint), lease)
			if migrateErr != nil {
				return false, fmt.Errorf("migrate instance %d session store: %w", item.InstanceID, migrateErr)
			}
			if releaseErr != nil {
				return false, fmt.Errorf("release migration writer lease for instance %d: %w", item.InstanceID, releaseErr)
			}
			if migration == nil || migration.Status != "validated" || strings.TrimSpace(migration.OutputSHA256) == "" {
				return false, fmt.Errorf("instance %d session migration did not return validated evidence", item.InstanceID)
			}
			postflight, _ := json.Marshal(migration)
			result, updateErr := s.sess.SQL().ExecContext(ctx, `UPDATE runtime_upgrade_items SET state = 'migrated', postflight_json = ?, updated_at = ? WHERE rollout_id = ? AND instance_id = ? AND state = 'snapshotted'`, string(postflight), time.Now().UTC(), rollout.ID, item.InstanceID)
			if updateErr != nil {
				return false, updateErr
			}
			if affected, _ := result.RowsAffected(); affected != 1 {
				return false, fmt.Errorf("instance %d migration state changed concurrently", item.InstanceID)
			}
			return false, nil
		}
		if err := s.updateRolloutPhase(ctx, rollout.ID, "gateway_restart"); err != nil {
			return false, err
		}
		rollout.Phase = "gateway_restart"
		items, err = s.listUpgradeItems(ctx, rollout.ID)
		if err != nil {
			return false, err
		}
	}
	// Target agents are now capability-verified. Release exactly one instance
	// at a time. Items are ordered OpenClaw workers first and the Leader last;
	// Team maintenance remains active until every recreated binding is healthy.
	if rollout.Phase == "gateway_restart" && len(items) > 0 && items[0].State == "migrated" {
		if err := s.markUpgradeItemRestartReady(ctx, rollout.ID, items[0].InstanceID); err != nil {
			return false, err
		}
		return false, nil
	}
	if rollout.Phase != "postflight" {
		for index, item := range items {
			if item.State == "gateway_verified" || item.State == "verified" {
				continue
			}
			if item.State == "migrated" {
				// Resume safely after a controller restart between releasing two
				// serial members (or between the phase update and first release).
				if err := s.markUpgradeItemRestartReady(ctx, rollout.ID, item.InstanceID); err != nil {
					return false, err
				}
				return false, nil
			}
			if item.State != "restart_ready" {
				return false, fmt.Errorf("instance %d has unexpected restart state %q", item.InstanceID, item.State)
			}
			binding, err := s.bindings.GetRunningByInstanceID(ctx, item.InstanceID)
			if err != nil {
				return false, err
			}
			if binding == nil {
				return false, nil
			}
			var expectedGeneration int
			row, err := s.sess.SQL().QueryRowContext(ctx, `SELECT runtime_generation FROM instances WHERE id = ?`, item.InstanceID)
			if err != nil {
				return false, err
			}
			if err := row.Scan(&expectedGeneration); err != nil {
				return false, err
			}
			if binding.Generation != expectedGeneration {
				return false, fmt.Errorf("instance %d binding generation %d does not match %d", item.InstanceID, binding.Generation, expectedGeneration)
			}
			targetPod := false
			for _, pod := range pods {
				if pod.ID == binding.RuntimePodID && strings.TrimSpace(pod.ImageRef) == strings.TrimSpace(rollout.TargetImageRef) && pod.State == "ready" && !pod.Draining {
					targetPod = true
					break
				}
			}
			if !targetPod {
				return false, fmt.Errorf("instance %d running binding is not on a ready target pod", item.InstanceID)
			}
			if err := s.markUpgradeItemGatewayVerified(ctx, rollout.ID, item.InstanceID); err != nil {
				return false, err
			}
			if err := s.audit(ctx, &rollout.ID, rollout.StartedBy, "instance_gateway_verified", "gateway_restart", "passed", map[string]any{"instance_id": item.InstanceID, "is_team_leader": item.IsTeamLeader}); err != nil {
				return false, err
			}
			if index+1 < len(items) {
				if err := s.markUpgradeItemRestartReady(ctx, rollout.ID, items[index+1].InstanceID); err != nil {
					return false, err
				}
				return false, nil
			}
			break
		}
	}
	if err := s.updateRolloutPhase(ctx, rollout.ID, "postflight"); err != nil {
		return false, err
	}
	teamIDs := map[int]struct{}{}
	for _, item := range items {
		if item.TeamID != nil {
			teamIDs[*item.TeamID] = struct{}{}
		}
	}
	if err := s.audit(ctx, &rollout.ID, rollout.StartedBy, "postflight", "postflight", "passed", map[string]any{"instance_count": len(items)}); err != nil {
		return false, err
	}
	if err := s.setTeamMaintenance(ctx, teamIDs, rollout.ID, false); err != nil {
		return false, err
	}
	now := time.Now().UTC()
	_, _ = s.sess.SQL().ExecContext(ctx, `UPDATE runtime_upgrade_items SET state = 'verified', finished_at = ?, updated_at = ? WHERE rollout_id = ?`, now, now, rollout.ID)
	return true, nil
}

func (s *RuntimeUpgradeService) workspaceForInstance(instanceID int) (string, error) {
	var userID int
	row, err := s.sess.SQL().QueryRow(`SELECT user_id FROM instances WHERE id = ?`, instanceID)
	if err != nil {
		return "", err
	}
	if err := row.Scan(&userID); err != nil {
		return "", err
	}
	return RuntimeWorkspacePathWithRoot(s.workspaceRoot, RuntimeTypeOpenClaw, userID, instanceID), nil
}

func (s *RuntimeUpgradeService) Rollback(ctx context.Context, rollout *models.RuntimeRollout) error {
	if rollout == nil {
		return nil
	}
	_ = s.updateRolloutPhase(ctx, rollout.ID, "rollback_restore")
	items, err := s.listUpgradeItems(ctx, rollout.ID)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return fmt.Errorf("rollback refused: rollout %d has no persisted upgrade items", rollout.ID)
	}
	var errs []error
	teamIDs := map[int]struct{}{}
	for _, item := range items {
		if item.TeamID != nil {
			teamIDs[*item.TeamID] = struct{}{}
		}
		if item.SnapshotRef == nil || strings.TrimSpace(*item.SnapshotRef) == "" {
			errs = append(errs, fmt.Errorf("restore instance %d: persisted snapshot is missing", item.InstanceID))
			continue
		}
		if item.WorkspaceManifestSHA256 == nil || strings.TrimSpace(*item.WorkspaceManifestSHA256) == "" {
			errs = append(errs, fmt.Errorf("restore instance %d: persisted workspace manifest is missing", item.InstanceID))
			continue
		}
		if err := verifyLocalSnapshotArchive(s.workspaceRoot, *item.SnapshotRef); err != nil {
			errs = append(errs, fmt.Errorf("verify instance %d snapshot: %w", item.InstanceID, err))
			continue
		}
		if err := s.restoreLocalSnapshot(item.InstanceID, *item.SnapshotRef); err != nil {
			errs = append(errs, fmt.Errorf("restore instance %d: %w", item.InstanceID, err))
			continue
		}
		workspace, pathErr := s.workspaceForInstance(item.InstanceID)
		if pathErr != nil {
			errs = append(errs, fmt.Errorf("verify restored instance %d workspace: %w", item.InstanceID, pathErr))
			continue
		}
		inventory, inspectErr := inspectRuntimeWorkspace(workspace)
		if inspectErr != nil {
			errs = append(errs, fmt.Errorf("verify restored instance %d manifest: %w", item.InstanceID, inspectErr))
			continue
		}
		if inventory.ManifestSHA256 != strings.TrimSpace(*item.WorkspaceManifestSHA256) {
			errs = append(errs, fmt.Errorf("verify restored instance %d manifest: got %s want %s", item.InstanceID, inventory.ManifestSHA256, strings.TrimSpace(*item.WorkspaceManifestSHA256)))
		}
	}
	if len(errs) == 0 {
		if err := s.audit(ctx, &rollout.ID, rollout.StartedBy, "rollback", "rollback_restore", "passed", map[string]any{"instance_count": len(items)}); err != nil {
			return err
		}
		if err := s.setTeamMaintenance(ctx, teamIDs, rollout.ID, false); err != nil {
			return err
		}
		status := "restored"
		_, _ = s.sess.SQL().ExecContext(ctx, `UPDATE runtime_rollouts SET rollback_status = ?, rollback_error = NULL, updated_at = ? WHERE id = ?`, status, time.Now().UTC(), rollout.ID)
		return nil
	}
	message := errors.Join(errs...).Error()
	_, _ = s.sess.SQL().ExecContext(ctx, `UPDATE runtime_rollouts SET rollback_status = 'error', rollback_error = ?, updated_at = ? WHERE id = ?`, message, time.Now().UTC(), rollout.ID)
	return errors.Join(errs...)
}

func (s *RuntimeUpgradeService) BeginRollback(ctx context.Context, rolloutID int64) error {
	_, err := s.sess.SQL().ExecContext(ctx, `UPDATE runtime_rollouts SET phase = 'rollback_image', rollback_status = 'starting', rollback_error = NULL, updated_at = ? WHERE id = ?`, time.Now().UTC(), rolloutID)
	return err
}

func (s *RuntimeUpgradeService) InstanceBlocked(ctx context.Context, instanceID int) bool {
	if s == nil || s.sess == nil || instanceID <= 0 {
		return false
	}
	var count int
	row, err := s.sess.SQL().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM runtime_upgrade_items i
		JOIN runtime_rollouts r ON r.id = i.rollout_id
		WHERE i.instance_id = ? AND (
		  (r.status IN ('pending','running') AND r.phase IN ('maintenance','snapshot','image_rollout','session_migration','postflight','rollback_restore'))
		  OR (r.status IN ('pending','running') AND r.phase = 'gateway_restart' AND i.state <> 'restart_ready')
		  OR r.rollback_status IN ('starting','waiting','error')
		)
	`, instanceID)
	if err == nil {
		err = row.Scan(&count)
	}
	return err == nil && count > 0
}

func (s *RuntimeUpgradeService) inspectCandidates(ctx context.Context) ([]runtimeUpgradeCandidate, map[string]string, []string, []string, error) {
	pods, err := s.pods.List(ctx, RuntimeTypeOpenClaw)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	podByID := map[int64]models.RuntimePod{}
	sourceImages := map[string]string{}
	var candidates []runtimeUpgradeCandidate
	var warnings, blockers []string
	for _, pod := range pods {
		podByID[pod.ID] = pod
	}
	if s.deployments == nil {
		return nil, nil, nil, nil, fmt.Errorf("live runtime deployment inventory is not configured")
	}
	livePods, err := s.deployments.RuntimeDeploymentPods(ctx, RuntimeTypeOpenClaw)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("inspect live OpenClaw deployments: %w", err)
	}
	for _, pod := range livePods {
		if strings.TrimSpace(pod.DeploymentName) == "" || strings.TrimSpace(pod.Namespace) == "" {
			continue
		}
		key := strings.TrimSpace(pod.Namespace) + "/" + strings.TrimSpace(pod.DeploymentName)
		pinned, pinErr := immutableRuntimeImage(pod.ImageRef, stringValue(pod.ImageDigest))
		if pinErr != nil {
			return nil, nil, nil, nil, fmt.Errorf("deployment %s rollback image is not immutable: %w", key, pinErr)
		}
		if prior, exists := sourceImages[key]; exists && prior != pinned {
			return nil, nil, nil, nil, fmt.Errorf("deployment %s has pods with conflicting image digests", key)
		}
		sourceImages[key] = pinned
	}
	rows, err := s.sess.SQL().QueryContext(ctx, `
		SELECT i.id, i.user_id, i.workspace_path,
		       b.runtime_pod_id, b.gateway_id, b.generation, b.state,
		       tm.id, tm.team_id, tm.member_key, tm.role, tm.runtime_type, tm.availability, tm.status
		FROM instances i
		LEFT JOIN instance_runtime_bindings b ON b.instance_id = i.id
		LEFT JOIN team_members tm ON tm.instance_id = i.id AND tm.status NOT IN ('deleted','deleting')
		WHERE LOWER(TRIM(i.type)) = 'openclaw'
		  AND (CASE
		    WHEN LOWER(TRIM(i.instance_mode)) IN ('lite','pro') THEN LOWER(TRIM(i.instance_mode))
		    WHEN LOWER(TRIM(i.runtime_type)) = 'gateway' THEN 'lite'
		    ELSE 'pro'
		  END) = 'lite'
		ORDER BY COALESCE(tm.team_id, 0), i.id
	`)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	defer rows.Close()
	seen := map[int]struct{}{}
	for rows.Next() {
		var instanceID, userID int
		var workspace sql.NullString
		var podID sql.NullInt64
		var gatewayID sql.NullString
		var generation sql.NullInt64
		var bindingState sql.NullString
		var memberID, teamID sql.NullInt64
		var memberKey, role, runtimeType, availability, status sql.NullString
		if err := rows.Scan(&instanceID, &userID, &workspace, &podID, &gatewayID, &generation, &bindingState, &memberID, &teamID, &memberKey, &role, &runtimeType, &availability, &status); err != nil {
			return nil, nil, nil, nil, err
		}
		if _, duplicate := seen[instanceID]; duplicate {
			blockers = append(blockers, fmt.Sprintf("instance %d belongs to more than one active Team", instanceID))
			continue
		}
		seen[instanceID] = struct{}{}
		expected := RuntimeWorkspacePathWithRoot(s.workspaceRoot, RuntimeTypeOpenClaw, userID, instanceID)
		actual := filepath.Clean(strings.TrimSpace(workspace.String))
		if actual == "." || actual == "" {
			actual = expected
		}
		if !sameCleanPath(actual, expected) || !pathWithin(s.workspaceRoot, actual) {
			blockers = append(blockers, fmt.Sprintf("instance %d workspace is outside the fixed managed path", instanceID))
		}
		if _, err := os.Lstat(actual); err != nil {
			blockers = append(blockers, fmt.Sprintf("instance %d workspace is unavailable: %v", instanceID, err))
		}
		candidate := runtimeUpgradeCandidate{InstanceID: instanceID, UserID: userID, GatewayID: strings.TrimSpace(gatewayID.String), Generation: int(generation.Int64), BindingState: strings.ToLower(strings.TrimSpace(bindingState.String)), WorkspacePath: actual, MemberKey: memberKey.String, Role: role.String, RuntimeType: runtimeType.String, Availability: availability.String, MemberStatus: status.String}
		if podID.Valid {
			value := podID.Int64
			candidate.RuntimePodID = &value
			if pod, ok := podByID[value]; ok {
				if pod.State != "ready" || pod.Draining {
					blockers = append(blockers, fmt.Sprintf("instance %d source Runtime pod is not ready", instanceID))
				}
				candidate.SourceVersion = stringValue(pod.OpenClawVersion)
				candidate.AgentEndpoint = stringValue(pod.AgentEndpoint)
				candidate.Capabilities = pod.Capabilities()
				if candidate.SourceVersion == "" {
					warnings = append(warnings, fmt.Sprintf("instance %d is a legacy 7.1-compatible source; local immutable workspace snapshot will be used", instanceID))
				}
			}
		}
		if memberID.Valid {
			value := int(memberID.Int64)
			candidate.TeamMemberID = &value
		}
		if teamID.Valid {
			value := int(teamID.Int64)
			candidate.TeamID = &value
			if strings.EqualFold(candidate.Availability, models.TeamMemberAvailabilityBusy) || strings.EqualFold(candidate.MemberStatus, models.TeamMemberStatusBusy) {
				blockers = append(blockers, fmt.Sprintf("Team %d OpenClaw member %d is busy", value, candidate.TeamMemberIDValue()))
			}
			if !strings.EqualFold(strings.TrimSpace(candidate.RuntimeType), RuntimeTypeOpenClaw) {
				blockers = append(blockers, fmt.Sprintf("Team %d member %d runtime type %q conflicts with its OpenClaw instance", value, candidate.TeamMemberIDValue(), candidate.RuntimeType))
			}
		}
		if candidate.GatewayID != "" {
			if candidate.AgentEndpoint == "" {
				blockers = append(blockers, fmt.Sprintf("instance %d has a running gateway but no reachable Runtime Agent", instanceID))
			} else if !containsString(candidate.Capabilities, "openclaw.gateway.stop-confirm") {
				warnings = append(warnings, fmt.Sprintf("instance %d uses the legacy 7.1 synchronous stop path; snapshot consistency will be verified twice", instanceID))
			}
		}
		candidates = append(candidates, candidate)
	}
	return candidates, sourceImages, warnings, blockers, rows.Err()
}

func (c runtimeUpgradeCandidate) TeamMemberIDValue() int {
	if c.TeamMemberID == nil {
		return 0
	}
	return *c.TeamMemberID
}

func sortRuntimeUpgradeCandidates(candidates []runtimeUpgradeCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		leftTeam, rightTeam := 0, 0
		if candidates[i].TeamID != nil {
			leftTeam = *candidates[i].TeamID
		}
		if candidates[j].TeamID != nil {
			rightTeam = *candidates[j].TeamID
		}
		if leftTeam != rightTeam {
			return leftTeam < rightTeam
		}
		leftLeader := isTeamLeaderRole(candidates[i].Role)
		rightLeader := isTeamLeaderRole(candidates[j].Role)
		if leftLeader != rightLeader {
			return !leftLeader
		}
		return candidates[i].InstanceID < candidates[j].InstanceID
	})
}

func (s *RuntimeUpgradeService) inspectTeamConsistency(ctx context.Context, teamIDs map[int]struct{}) (int, []string, error) {
	hermesCount := 0
	var blockers []string
	for teamID := range teamIDs {
		var activeTasks int
		row, err := s.sess.SQL().QueryRowContext(ctx, `SELECT COUNT(*) FROM team_tasks WHERE team_id = ? AND status IN ('pending','dispatched','running','waiting')`, teamID)
		if err != nil {
			return 0, nil, err
		}
		if err := row.Scan(&activeTasks); err != nil {
			return 0, nil, err
		}
		if activeTasks > 0 {
			blockers = append(blockers, fmt.Sprintf("Team %d has %d active task(s)", teamID, activeTasks))
		}
		var activeWorkItems int
		row, err = s.sess.SQL().QueryRowContext(ctx, `SELECT COUNT(*) FROM team_work_items WHERE team_id = ? AND status IN ('dispatched','running','waiting')`, teamID)
		if err != nil {
			return 0, nil, err
		}
		if err := row.Scan(&activeWorkItems); err != nil {
			return 0, nil, err
		}
		if activeWorkItems > 0 {
			blockers = append(blockers, fmt.Sprintf("Team %d has %d active assignment(s)", teamID, activeWorkItems))
		}
		var pendingOutbox int
		row, err = s.sess.SQL().QueryRowContext(ctx, `SELECT COUNT(*) FROM team_event_outbox WHERE team_id = ? AND status <> 'delivered'`, teamID)
		if err != nil {
			return 0, nil, err
		}
		if err := row.Scan(&pendingOutbox); err != nil {
			return 0, nil, err
		}
		if pendingOutbox > 0 {
			blockers = append(blockers, fmt.Sprintf("Team %d has %d pending outbox event(s)", teamID, pendingOutbox))
		}
		rows, err := s.sess.SQL().QueryContext(ctx, `SELECT id, runtime_type, role, availability, status FROM team_members WHERE team_id = ? AND status NOT IN ('deleted','deleting')`, teamID)
		if err != nil {
			return 0, nil, err
		}
		leaderCount := 0
		openClawLeaderCount := 0
		for rows.Next() {
			var id int
			var runtimeType, role, availability, status string
			if err := rows.Scan(&id, &runtimeType, &role, &availability, &status); err != nil {
				rows.Close()
				return 0, nil, err
			}
			if isTeamLeaderRole(role) {
				leaderCount++
				if strings.EqualFold(runtimeType, RuntimeTypeOpenClaw) {
					openClawLeaderCount++
				}
			}
			if strings.EqualFold(runtimeType, RuntimeTypeHermes) {
				hermesCount++
				if strings.EqualFold(availability, models.TeamMemberAvailabilityBusy) || strings.EqualFold(status, models.TeamMemberStatusBusy) {
					blockers = append(blockers, fmt.Sprintf("Team %d Hermes member %d is busy", teamID, id))
				}
			}
		}
		rows.Close()
		if leaderCount != 1 {
			blockers = append(blockers, fmt.Sprintf("Team %d must have exactly one Leader", teamID))
		} else if openClawLeaderCount != 1 {
			blockers = append(blockers, fmt.Sprintf("Team %d Leader must remain an OpenClaw intermediary", teamID))
		}
	}
	return hermesCount, blockers, nil
}

func (s *RuntimeUpgradeService) stopCandidateGateway(ctx context.Context, candidate runtimeUpgradeCandidate) error {
	if candidate.GatewayID == "" {
		if candidate.RuntimePodID != nil && candidate.BindingState != "running" {
			return s.bindings.DeleteByInstanceIDAndReleaseSlot(ctx, candidate.InstanceID, *candidate.RuntimePodID)
		}
		return nil
	}
	if candidate.AgentEndpoint == "" || s.agent == nil {
		return fmt.Errorf("running gateway cannot be stopped because its Runtime Agent is unavailable")
	}
	if err := s.agent.DeleteGateway(ctx, candidate.AgentEndpoint, candidate.GatewayID); err != nil && !errors.Is(err, ErrRuntimeAgentNotFound) {
		return err
	}
	if upgradeAgent, ok := s.agent.(RuntimeUpgradeAgentClient); ok && containsString(candidate.Capabilities, "openclaw.gateway.stop-confirm") {
		state, err := upgradeAgent.GatewayState(ctx, candidate.AgentEndpoint, candidate.GatewayID)
		if err != nil && !errors.Is(err, ErrRuntimeAgentNotFound) {
			return fmt.Errorf("verify confirmed gateway stop: %w", err)
		}
		if err == nil && state != nil {
			return fmt.Errorf("gateway is still registered after confirmed stop (state %s)", state.State)
		}
	}
	if candidate.RuntimePodID != nil {
		if candidate.BindingState == "running" {
			deleted, err := s.bindings.DeleteRunningByInstanceIDGenerationAndReleaseSlot(ctx, candidate.InstanceID, *candidate.RuntimePodID, candidate.Generation)
			if err != nil {
				return fmt.Errorf("release stopped runtime binding: %w", err)
			}
			if !deleted {
				return fmt.Errorf("stopped gateway binding changed concurrently")
			}
		} else if err := s.bindings.DeleteByInstanceIDAndReleaseSlot(ctx, candidate.InstanceID, *candidate.RuntimePodID); err != nil {
			return fmt.Errorf("release inactive runtime binding: %w", err)
		}
	}
	return nil
}

func (s *RuntimeUpgradeService) snapshotCandidate(ctx context.Context, rolloutID int64, candidate runtimeUpgradeCandidate) (string, runtimeWorkspaceInventory, error) {
	localInventory, localInventoryErr := inspectRuntimeWorkspace(candidate.WorkspacePath)
	if localInventoryErr != nil {
		return "", localInventory, localInventoryErr
	}
	if upgradeAgent, ok := s.agent.(RuntimeUpgradeAgentClient); ok && candidate.AgentEndpoint != "" && containsString(candidate.Capabilities, "openclaw.workspace.snapshot-v1") {
		if !containsString(candidate.Capabilities, "openclaw.workspace.writer-lease") {
			return "", localInventory, fmt.Errorf("Runtime Agent advertises snapshots without the required writer lease capability")
		}
		leaseToken, err := randomUpgradeID()
		if err != nil {
			return "", localInventory, err
		}
		request := RuntimeAgentWorkspaceRequest{
			RolloutID: strconv.FormatInt(rolloutID, 10), SnapshotID: fmt.Sprintf("instance-%d-generation-%d", candidate.InstanceID, candidate.Generation),
			UserID: candidate.UserID, InstanceID: candidate.InstanceID, Generation: candidate.Generation,
			LeaseToken: leaseToken, OfficialDBCheck: true,
		}
		leaseRequest := RuntimeAgentWriterLeaseRequest{RuntimeAgentWorkspaceRequest: request, Token: leaseToken, TTLSeconds: 3600}
		if err := upgradeAgent.AcquireWriterLease(ctx, candidate.AgentEndpoint, leaseRequest); err != nil {
			return "", localInventory, fmt.Errorf("acquire workspace writer lease: %w", err)
		}
		release := func() error { return upgradeAgent.ReleaseWriterLease(ctx, candidate.AgentEndpoint, leaseRequest) }
		if _, err := upgradeAgent.PreflightWorkspace(ctx, candidate.AgentEndpoint, request); err != nil {
			_ = release()
			return "", localInventory, fmt.Errorf("Runtime Agent workspace preflight: %w", err)
		}
		snapshot, err := upgradeAgent.CreateWorkspaceSnapshot(ctx, candidate.AgentEndpoint, RuntimeAgentSnapshotRequest{RuntimeAgentWorkspaceRequest: request})
		if err != nil {
			_ = release()
			return "", localInventory, fmt.Errorf("Runtime Agent workspace snapshot: %w", err)
		}
		verifyRequest := request
		verifyRequest.SnapshotID = snapshot.SnapshotID
		if err := upgradeAgent.VerifyWorkspaceSnapshot(ctx, candidate.AgentEndpoint, RuntimeAgentSnapshotVerifyRequest{RuntimeAgentWorkspaceRequest: verifyRequest}); err != nil {
			_ = release()
			return "", localInventory, fmt.Errorf("verify Runtime Agent workspace snapshot: %w", err)
		}
		if !pathWithin(filepath.Join(s.workspaceRoot, ".clawmanager-upgrades"), snapshot.ArchivePath) {
			_ = release()
			return "", localInventory, fmt.Errorf("Runtime Agent snapshot path is outside the shared managed upgrade root")
		}
		if _, err := os.Stat(snapshot.ArchivePath); err != nil {
			_ = release()
			return "", localInventory, fmt.Errorf("Runtime Agent snapshot is not visible to the controller: %w", err)
		}
		if err := release(); err != nil {
			return "", localInventory, fmt.Errorf("release workspace writer lease: %w", err)
		}
		return "local:" + snapshot.ArchivePath, localInventory, nil
	}
	return s.createLocalSnapshot(rolloutID, candidate)
}

func (s *RuntimeUpgradeService) createLocalSnapshot(rolloutID int64, candidate runtimeUpgradeCandidate) (string, runtimeWorkspaceInventory, error) {
	inventory, err := inspectRuntimeWorkspace(candidate.WorkspacePath)
	if err != nil {
		return "", inventory, err
	}
	destination := filepath.Join(s.workspaceRoot, ".clawmanager-upgrades", fmt.Sprintf("rollout-%d", rolloutID))
	if err := os.MkdirAll(destination, 0o700); err != nil {
		return "", inventory, err
	}
	archivePath := filepath.Join(destination, fmt.Sprintf("controller-instance-%d.tar.gz", candidate.InstanceID))
	tmpPath := archivePath + ".tmp"
	if _, err := os.Lstat(tmpPath); err == nil {
		return "", inventory, fmt.Errorf("stale snapshot temp file exists")
	}
	file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", inventory, err
	}
	failed := true
	defer func() {
		_ = file.Close()
		if failed {
			_ = os.Remove(tmpPath)
		}
	}()
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	err = filepath.Walk(candidate.WorkspacePath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(candidate.WorkspacePath, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		var link string
		if info.Mode()&os.ModeSymlink != 0 {
			link, err = os.Readlink(path)
			if err != nil {
				return err
			}
		}
		header, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		source, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tw, source)
		closeErr := source.Close()
		return errors.Join(copyErr, closeErr)
	})
	if closeErr := tw.Close(); err == nil {
		err = closeErr
	}
	if closeErr := gz.Close(); err == nil {
		err = closeErr
	}
	if closeErr := file.Sync(); err == nil {
		err = closeErr
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", inventory, err
	}
	after, err := inspectRuntimeWorkspace(candidate.WorkspacePath)
	if err != nil {
		return "", inventory, err
	}
	if after.ManifestSHA256 != inventory.ManifestSHA256 {
		return "", inventory, fmt.Errorf("workspace changed while snapshot was being created")
	}
	if err := os.Rename(tmpPath, archivePath); err != nil {
		return "", inventory, err
	}
	failed = false
	return "local:" + archivePath, inventory, nil
}

func (s *RuntimeUpgradeService) restoreLocalSnapshot(instanceID int, snapshotRef string) error {
	if strings.HasPrefix(snapshotRef, "agent:") {
		return fmt.Errorf("agent snapshot restore requires the original runtime agent and is not available through local fallback")
	}
	archivePath := strings.TrimPrefix(snapshotRef, "local:")
	if !pathWithin(filepath.Join(s.workspaceRoot, ".clawmanager-upgrades"), archivePath) {
		return fmt.Errorf("snapshot path is outside the managed upgrade root")
	}
	workspace, err := s.workspaceForInstance(instanceID)
	if err != nil {
		return err
	}
	staging := workspace + fmt.Sprintf(".restore-%d", time.Now().UTC().UnixNano())
	preserved := workspace + fmt.Sprintf(".preserved-%d", time.Now().UTC().UnixNano())
	if err := os.MkdirAll(staging, 0o700); err != nil {
		return err
	}
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		target := filepath.Join(staging, filepath.FromSlash(header.Name))
		if !pathWithin(staging, target) {
			return fmt.Errorf("snapshot contains an unsafe path")
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if filepath.IsAbs(header.Linkname) {
				return fmt.Errorf("snapshot contains an absolute symlink")
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				return err
			}
			if err := os.Symlink(header.Linkname, target); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(out, tr)
			closeErr := out.Close()
			if err := errors.Join(copyErr, closeErr); err != nil {
				return err
			}
		default:
			return fmt.Errorf("snapshot contains unsupported entry type")
		}
	}
	if err := os.Rename(workspace, preserved); err != nil {
		return err
	}
	if err := os.Rename(staging, workspace); err != nil {
		_ = os.Rename(preserved, workspace)
		return err
	}
	return nil
}

func verifyLocalSnapshotArchive(workspaceRoot, snapshotRef string) error {
	if strings.HasPrefix(snapshotRef, "agent:") {
		return fmt.Errorf("agent-only snapshot reference cannot be verified locally")
	}
	archivePath := strings.TrimPrefix(strings.TrimSpace(snapshotRef), "local:")
	if !pathWithin(filepath.Join(workspaceRoot, ".clawmanager-upgrades"), archivePath) {
		return fmt.Errorf("snapshot path is outside the managed upgrade root")
	}
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		clean := filepath.Clean(filepath.FromSlash(header.Name))
		if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return fmt.Errorf("snapshot contains an unsafe path")
		}
		if header.Typeflag == tar.TypeReg || header.Typeflag == tar.TypeRegA {
			if _, err := io.CopyN(io.Discard, reader, header.Size); err != nil {
				return err
			}
		}
	}
}

func inspectRuntimeWorkspace(root string) (runtimeWorkspaceInventory, error) {
	var inventory runtimeWorkspaceInventory
	manifest := sha256.New()
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			inventory.SymlinkCount++
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			fmt.Fprintf(manifest, "%s|symlink|%s\n", filepath.ToSlash(rel), link)
			return nil
		}
		if info.IsDir() {
			inventory.DirectoryCount++
			fmt.Fprintf(manifest, "%s|dir|%o\n", filepath.ToSlash(rel), info.Mode().Perm())
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported workspace entry %s", rel)
		}
		inventory.FileCount++
		inventory.TotalBytes += info.Size()
		fileHash := sha256.New()
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(fileHash, file)
		closeErr := file.Close()
		if err := errors.Join(copyErr, closeErr); err != nil {
			return err
		}
		fmt.Fprintf(manifest, "%s|file|%o|%d|%x\n", filepath.ToSlash(rel), info.Mode().Perm(), info.Size(), fileHash.Sum(nil))
		lower := strings.ToLower(info.Name())
		if strings.HasSuffix(lower, ".sqlite") || strings.HasSuffix(lower, ".sqlite3") || strings.HasSuffix(lower, ".db") {
			inventory.SQLiteFileCount++
			dbFile, err := os.Open(path)
			if err != nil {
				return err
			}
			header := make([]byte, 16)
			_, readErr := io.ReadFull(dbFile, header)
			_ = dbFile.Close()
			if readErr != nil || string(header) != "SQLite format 3\x00" {
				return fmt.Errorf("SQLite header check failed for %s", filepath.ToSlash(rel))
			}
		}
		return nil
	})
	if err != nil {
		return inventory, err
	}
	inventory.SQLiteHeaderValid = true
	inventory.ManifestSHA256 = hex.EncodeToString(manifest.Sum(nil))
	return inventory, nil
}

func (s *RuntimeUpgradeService) setTeamMaintenance(ctx context.Context, teamIDs map[int]struct{}, rolloutID int64, enabled bool) error {
	if len(teamIDs) == 0 {
		return nil
	}
	ids := make([]int, 0, len(teamIDs))
	for teamID := range teamIDs {
		ids = append(ids, teamID)
	}
	sort.Ints(ids)
	if s.teamMaintenance != nil {
		return s.teamMaintenance.SetRuntimeUpgradeMaintenance(ctx, ids, rolloutID, enabled)
	}
	if s.redisURL == "" {
		return fmt.Errorf("Team maintenance requires PLATFORM_REDIS_URL/TEAM_REDIS_URL")
	}
	bus, err := newRedisBus(s.redisURL)
	if err != nil {
		return err
	}
	for teamID := range teamIDs {
		key := fmt.Sprintf("claw:team:%d:maintenance", teamID)
		if enabled {
			value, _ := json.Marshal(map[string]any{"enabled": true, "rolloutId": rolloutID, "reason": "openclaw_runtime_upgrade", "updatedAt": time.Now().UTC().Format(time.RFC3339Nano)})
			if err := bus.Set(ctx, key, string(value), 0); err != nil {
				return err
			}
		} else if err := bus.Del(ctx, key); err != nil {
			return err
		}
	}
	return nil
}

func (s *RuntimeUpgradeService) insertUpgradeItems(ctx context.Context, rolloutID int64, candidates []runtimeUpgradeCandidate) error {
	now := time.Now().UTC()
	for _, candidate := range candidates {
		leader := isTeamLeaderRole(candidate.Role)
		order := candidate.InstanceID
		item := &models.RuntimeUpgradeItem{RolloutID: rolloutID, TeamID: candidate.TeamID, TeamMemberID: candidate.TeamMemberID, InstanceID: candidate.InstanceID, RuntimePodID: candidate.RuntimePodID, MemberOrder: order, IsTeamLeader: leader, SourceRuntimeVersion: stringPtrOrNil(candidate.SourceVersion), TargetRuntimeVersion: stringPtrOrNil(targetOpenClawUpgradeVersion), State: "pending", CreatedAt: now, UpdatedAt: now}
		if _, err := s.sess.Collection("runtime_upgrade_items").Insert(item); err != nil {
			return err
		}
	}
	return nil
}

func (s *RuntimeUpgradeService) listUpgradeItems(ctx context.Context, rolloutID int64) ([]models.RuntimeUpgradeItem, error) {
	var items []models.RuntimeUpgradeItem
	if err := s.sess.Collection("runtime_upgrade_items").Find(db.Cond{"rollout_id": rolloutID}).OrderBy("team_id", "is_team_leader", "member_order", "id").All(&items); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *RuntimeUpgradeService) updateUpgradeItemSnapshot(ctx context.Context, rolloutID int64, instanceID int, snapshot string, inventory runtimeWorkspaceInventory) error {
	raw, _ := json.Marshal(inventory)
	now := time.Now().UTC()
	_, err := s.sess.SQL().ExecContext(ctx, `UPDATE runtime_upgrade_items SET state = 'snapshotted', snapshot_ref = ?, workspace_manifest_sha256 = ?, preflight_json = ?, started_at = COALESCE(started_at, ?), updated_at = ? WHERE rollout_id = ? AND instance_id = ?`, snapshot, inventory.ManifestSHA256, string(raw), now, now, rolloutID, instanceID)
	return err
}

func (s *RuntimeUpgradeService) markUpgradeItemRestartReady(ctx context.Context, rolloutID int64, instanceID int) error {
	result, err := s.sess.SQL().ExecContext(ctx, `UPDATE runtime_upgrade_items SET state = 'restart_ready', updated_at = ? WHERE rollout_id = ? AND instance_id = ? AND state IN ('migrated','gateway_verified')`, time.Now().UTC(), rolloutID, instanceID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return fmt.Errorf("instance %d could not enter restart_ready", instanceID)
	}
	return nil
}

func (s *RuntimeUpgradeService) markUpgradeItemGatewayVerified(ctx context.Context, rolloutID int64, instanceID int) error {
	result, err := s.sess.SQL().ExecContext(ctx, `UPDATE runtime_upgrade_items SET state = 'gateway_verified', updated_at = ? WHERE rollout_id = ? AND instance_id = ? AND state = 'restart_ready'`, time.Now().UTC(), rolloutID, instanceID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return fmt.Errorf("instance %d gateway verification state changed concurrently", instanceID)
	}
	return nil
}

func (s *RuntimeUpgradeService) updateRolloutPhase(ctx context.Context, rolloutID int64, phase string) error {
	_, err := s.sess.SQL().ExecContext(ctx, `UPDATE runtime_rollouts SET phase = ?, updated_at = ? WHERE id = ?`, phase, time.Now().UTC(), rolloutID)
	return err
}

func (s *RuntimeUpgradeService) audit(ctx context.Context, rolloutID *int64, actor *int, action, phase, outcome string, detail map[string]any) error {
	return insertRuntimeUpgradeAudit(ctx, s.sess, rolloutID, actor, action, phase, outcome, detail)
}

func insertRuntimeUpgradeAudit(ctx context.Context, sess db.Session, rolloutID *int64, actor *int, action, phase, outcome string, detail map[string]any) error {
	raw, _ := json.Marshal(detail)
	_, err := sess.SQL().ExecContext(ctx, `INSERT INTO runtime_upgrade_audits (rollout_id, actor_user_id, action, phase, outcome, detail_json, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, rolloutID, actor, action, phase, outcome, string(raw), time.Now().UTC())
	return err
}

func imageDigestFromReference(value string) string {
	index := strings.LastIndex(strings.TrimSpace(value), "@sha256:")
	if index < 0 {
		return ""
	}
	digest := value[index+1:]
	hexPart := strings.TrimPrefix(digest, "sha256:")
	if len(hexPart) != 64 {
		return ""
	}
	if _, err := hex.DecodeString(hexPart); err != nil {
		return ""
	}
	return digest
}

func immutableRuntimeImage(imageRef, digest string) (string, error) {
	imageRef = strings.TrimSpace(imageRef)
	digest = strings.TrimSpace(digest)
	if imageRef == "" {
		return "", fmt.Errorf("image reference is empty")
	}
	if existing := imageDigestFromReference(imageRef); existing != "" {
		return imageRef, nil
	}
	if !strings.HasPrefix(digest, "sha256:") || imageDigestFromReference("image@"+digest) == "" {
		return "", fmt.Errorf("runtime did not report a valid sha256 image digest")
	}
	if at := strings.Index(imageRef, "@"); at >= 0 {
		imageRef = imageRef[:at]
	}
	lastSlash := strings.LastIndex(imageRef, "/")
	if colon := strings.LastIndex(imageRef, ":"); colon > lastSlash {
		imageRef = imageRef[:colon]
	}
	if strings.TrimSpace(imageRef) == "" {
		return "", fmt.Errorf("image repository is empty")
	}
	return imageRef + "@" + digest, nil
}

func runtimeUpgradeFingerprint(target string, candidates []runtimeUpgradeCandidate, sourceImages map[string]string) string {
	type fingerprintCandidate struct {
		InstanceID int
		UserID     int
		Workspace  string
		TeamID     int
		Member     string
		Role       string
		Generation int
		Binding    string
	}
	values := make([]fingerprintCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		teamID := 0
		if candidate.TeamID != nil {
			teamID = *candidate.TeamID
		}
		values = append(values, fingerprintCandidate{candidate.InstanceID, candidate.UserID, filepath.Clean(candidate.WorkspacePath), teamID, candidate.MemberKey, candidate.Role, candidate.Generation, candidate.BindingState})
	}
	sort.Slice(values, func(i, j int) bool { return values[i].InstanceID < values[j].InstanceID })
	payload, _ := json.Marshal(map[string]any{"target": strings.TrimSpace(target), "candidates": values, "sources": sourceImages})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func randomUpgradeID() (string, error) {
	value := make([]byte, 24)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func uniqueSortedStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func pathWithin(root, target string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(target))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func sameCleanPath(left, right string) bool {
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}
func stringPtrOrNil(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
