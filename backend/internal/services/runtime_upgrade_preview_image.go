package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"clawreef/internal/runtimeauth"
)

// A scoped upgrade may activate its frozen target before changing the fleet
// default. Approve only its exact reserved successor and validated migration;
// VerifyManagedPreviewImage still independently checks every live container.
func (s *RuntimeUpgradeService) ManagedPreviewUpgradeImage(ctx context.Context, id runtimeauth.Identity) (string, error) {
	var image, digest, receipt string
	row, err := s.sess.SQL().QueryRowContext(ctx, `SELECT r.target_image_ref,r.target_image_digest,ui.postflight_json FROM runtime_upgrade_items ui JOIN runtime_rollouts r ON r.id=ui.rollout_id JOIN instances i ON i.id=ui.instance_id JOIN instance_runtime_bindings b ON b.instance_id=i.id JOIN runtime_pods p ON p.id=b.runtime_pod_id WHERE i.id=? AND i.user_id=? AND i.runtime_generation=? AND b.generation=? AND ui.source_generation+1=? AND BINARY p.pod_uid=BINARY ? AND BINARY ui.restart_target_pod_uid=BINARY ? AND i.type='openclaw' AND b.runtime_type='openclaw' AND p.runtime_type='openclaw' AND r.runtime_type='openclaw' AND r.preflight_id IS NOT NULL AND r.status IN ('running','finished') AND r.phase IN ('gateway_restart','postflight') AND ui.state IN ('restart_ready','gateway_verified','verified') AND ui.isolated_at IS NULL AND ui.recovery_status IN ('starting','verified') AND ui.postflight_json IS NOT NULL ORDER BY r.id DESC LIMIT 1`, id.InstanceID, id.UserID, id.Generation, id.Generation, id.Generation, id.PodUID, id.PodUID)
	if err != nil {
		return "", err
	}
	if err = row.Scan(&image, &digest, &receipt); errors.Is(err, sql.ErrNoRows) {
		return "", nil
	} else if err != nil {
		return "", err
	}
	var evidence struct {
		Migration *RuntimeAgentSessionSQLiteMigration `json:"migration"`
	}
	if json.Unmarshal([]byte(receipt), &evidence) != nil || !validSessionSQLiteMigration(evidence.Migration) || evidence.Migration.InstanceID != id.InstanceID || digest == "" || imageDigestFromReference(image) != digest {
		return "", errRuntimeSecurity
	}
	return image, nil
}
