package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"clawreef/internal/models"
)

// A terminal upgrade can retain a creating SQL reservation without ever
// publishing its successor. Expose explicit recovery only after independently
// checking the finalized source and every later publication, never from age or
// a creating/error label. Execution remains in the ordinary scoped child rollout.
func (s *RuntimeUpgradeService) verifiedUnpublishedUpgradeRecovery(ctx context.Context, d *RuntimeUpgradeDetails, id, generation int, b *models.InstanceRuntimeBinding) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	denied := fmt.Errorf("%w: pending upgrade successor is unconfirmed", ErrUpgradeSourceState)
	if d == nil || d.Rollout == nil || d.Rollout.Status != "error" || d.Rollout.Phase != "gateway_restart" || b == nil || b.InstanceID != id || b.RuntimeType != "openclaw" || b.Generation != generation || b.State != "creating" || b.GatewayPID != nil || b.GatewayID != fmt.Sprintf("pending-%d-%d", id, generation) {
		return denied
	}
	var item *models.RuntimeUpgradeItem
	for i := range d.Items {
		if d.Items[i].InstanceID == id {
			item = &d.Items[i]
		}
	}
	if item == nil || item.State != "restart_ready" || stringValue(item.RecoveryStatus) != "needs_attention" || item.SourceGeneration == nil || *item.SourceGeneration+1 != generation || item.RestartTargetPodUID == nil || item.PostflightJSON == nil || item.SnapshotRef == nil {
		return denied
	}
	var receipt struct {
		Migration *RuntimeAgentSessionSQLiteMigration `json:"migration"`
	}
	if json.Unmarshal([]byte(*item.PostflightJSON), &receipt) != nil || !validSessionSQLiteMigration(receipt.Migration) || receipt.Migration.InstanceID != id {
		return denied
	}
	pod, err := s.pods.GetByID(ctx, b.RuntimePodID)
	if err != nil {
		return err
	}
	if pod == nil || stringValue(pod.PodUID) != *item.RestartTargetPodUID {
		return denied
	}
	reader, ok := s.deployments.(teamStoppedSourceReader)
	if !ok {
		return denied
	}
	var user int
	var workspace string
	row, err := s.sess.SQL().QueryRowContext(ctx, `SELECT user_id,workspace_path FROM instances WHERE id=? AND runtime_generation=?`, id, generation)
	if err != nil {
		return err
	}
	if err = row.Scan(&user, &workspace); err != nil {
		return err
	}
	if workspace == "" || workspace != b.WorkspacePath {
		return denied
	}
	proof, err := reader.ConfirmedTeamStoppedSource(ctx, id, user, generation, workspace)
	if err != nil {
		return err
	}
	if proof.InstanceID != id || proof.UserID != user || proof.Generation < 1 || proof.Generation >= generation || proof.WorkspacePath != workspace || proof.Phase != "completed" || proof.StorageIdentity == "" || proof.PodUID == "" || proof.RuntimePodID <= 0 || proof.GatewayID == "" || proof.Endpoint == "" {
		return denied
	}
	return nil
}
