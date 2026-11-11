package services

import (
	"clawreef/internal/models"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

type nativePlacementInspector func(context.Context, string, map[string]string, bool) (*OpenClawTargetClassification, error)

// This is continuation of a confirmed whole-Team cold adoption, not permission
// to upgrade application data. The old global legacy-pool default is superseded
// only by actual stop provenance, an exact frozen member and the SAME immutable
// image-reported OpenClaw version. Native assignment/image authority still runs.
func (s *RuntimeUpgradeService) coldTeamNativePlacement(ctx context.Context, id int, pods []models.RuntimePod, inspect nativePlacementInspector) ([]models.RuntimePod, bool, error) {
	var user, generation int
	var workspace, profile, status, teamProfile, state, desired, raw string
	row, err := s.sess.SQL().QueryRowContext(ctx, `SELECT i.user_id,i.runtime_generation,COALESCE(i.workspace_path,''),i.runtime_security_profile,i.status,t.preview_security_profile,l.state,l.desired_state,l.members_json FROM instances i JOIN team_members m ON m.instance_id=i.id JOIN teams t ON t.id=m.team_id JOIN team_runtime_lifecycle l ON l.team_id=t.id WHERE i.id=? AND m.status NOT IN ('deleted','deleting') AND (SELECT COUNT(*) FROM team_members own WHERE own.instance_id=i.id AND own.status NOT IN ('deleted','deleting'))=1 AND NOT EXISTS (SELECT 1 FROM runtime_upgrade_items ui WHERE ui.instance_id=i.id) AND NOT EXISTS (SELECT 1 FROM runtime_rollouts r WHERE r.preflight_id IS NOT NULL AND JSON_CONTAINS(JSON_EXTRACT(r.preflight_json,'$.exclude_instance_ids'),JSON_ARRAY(i.id)))`, id)
	if err != nil {
		return nil, false, err
	}
	err = row.Scan(&user, &generation, &workspace, &profile, &status, &teamProfile, &state, &desired, &raw)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if profile != RuntimeSecurityProfile || teamProfile != models.TeamPreviewSecurityProfile || status != "creating" || desired != "running" || (state != "starting" && state != "recovery_required") {
		return nil, false, nil
	}
	var members []models.TeamLifecycleMember
	if err = json.Unmarshal([]byte(raw), &members); err != nil {
		return nil, false, err
	}
	frozen := false
	for _, m := range members {
		if m.InstanceID == id && m.Workspace == workspace && m.Generation == generation-1 {
			frozen = true
		}
	}
	if !frozen {
		return nil, false, nil
	}
	rows, err := s.sess.SQL().QueryContext(ctx, `SELECT instance_id,user_id,generation,runtime_pod_id,gateway_id,pod_uid,endpoint,workspace_path,phase FROM instance_idle_stop_operations WHERE instance_id=? ORDER BY created_at DESC LIMIT 64`, id)
	if err != nil {
		return nil, false, err
	}
	var stops []models.InstanceIdleStopOperation
	for rows.Next() {
		var v models.InstanceIdleStopOperation
		if err = rows.Scan(&v.InstanceID, &v.UserID, &v.Generation, &v.RuntimePodID, &v.GatewayID, &v.PodUID, &v.Endpoint, &v.WorkspacePath, &v.Phase); err != nil {
			rows.Close()
			return nil, false, err
		}
		stops = append(stops, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, false, err
	}
	source, ok := confirmedColdTeamSource(id, user, generation, workspace, stops)
	if !ok {
		return nil, false, nil
	}
	if s.pods == nil || s.deployments == nil {
		return nil, false, nil
	}
	history, err := s.pods.GetByID(ctx, source.RuntimePodID)
	if err != nil {
		return nil, false, err
	}
	if history == nil || history.RuntimeType != RuntimeTypeOpenClaw || stringValue(history.PodUID) != source.PodUID || stringValue(history.AgentEndpoint) != source.Endpoint {
		return nil, false, nil
	}
	oldImage, err := immutableRuntimeImage(history.ImageRef, stringValue(history.ImageDigest))
	if err != nil {
		return nil, false, err
	}
	live, err := s.deployments.RuntimeDeploymentPods(ctx, RuntimeTypeOpenClaw)
	if err != nil {
		return nil, false, err
	}
	var old *models.RuntimePod
	for n := range live {
		p := &live[n]
		if p.ID == history.ID && stringValue(p.PodUID) == source.PodUID && p.KubernetesReady && stringValue(p.RegisteredPodUID) == source.PodUID && stringValue(p.KubernetesPodUID) == source.PodUID && runtimePodMatchesImage(*p, oldImage) {
			old = p
		}
	}
	if old == nil {
		return nil, false, nil
	}
	refs := map[string]string{old.Namespace + "/" + old.DeploymentName: oldImage}
	before, err := inspect(ctx, oldImage, refs, false)
	if err != nil {
		return nil, false, err
	}
	if before == nil || before.ImageDigest != imageDigestFromReference(oldImage) || before.RuntimeVersion == "" {
		return nil, false, fmt.Errorf("cold Team source image is not verifiable")
	}
	result := []models.RuntimePod{}
	for _, p := range pods {
		uid := stringValue(p.PodUID)
		matched := false
		for _, v := range live {
			if v.ID == p.ID && len(v.Slots) > 0 && v.KubernetesReady && uid != "" && stringValue(v.PodUID) == uid && stringValue(v.RegisteredPodUID) == uid && stringValue(v.KubernetesPodUID) == uid && runtimeWorkspaceStorageCompatible(old.WorkspaceStorageIdentity, v.WorkspaceStorageIdentity) {
				matched = true
			}
		}
		if !matched {
			continue
		}
		image, e := immutableRuntimeImage(p.ImageRef, stringValue(p.ImageDigest))
		if e != nil {
			continue
		}
		after, e := inspect(ctx, image, refs, false)
		if e != nil {
			return nil, false, e
		}
		if after != nil && after.ImageDigest == imageDigestFromReference(image) && after.RuntimeVersion == before.RuntimeVersion {
			result = append(result, p)
		}
	}
	return result, len(result) > 0, nil
}

// An unbound failed start is not a new source. Every intervening generation
// must have a real completed stop; missing or uncertain history never qualifies.
func confirmedColdTeamSource(id, user, generation int, workspace string, stops []models.InstanceIdleStopOperation) (models.InstanceIdleStopOperation, bool) {
	want := generation - 1
	for _, s := range stops {
		if s.Generation > want || s.InstanceID != id || s.UserID != user || s.WorkspacePath != workspace || s.Phase != "completed" {
			return s, false
		}
		if s.Generation != want {
			return s, false
		}
		if s.RuntimePodID > 0 {
			return s, s.PodUID != "" && s.GatewayID != "" && s.Endpoint != ""
		}
		if s.PodUID != "" || s.GatewayID != "" || s.Endpoint != "" {
			return s, false
		}
		want--
		if want < 1 {
			return s, false
		}
	}
	return models.InstanceIdleStopOperation{}, false
}
