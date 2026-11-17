package services

import (
	"context"
	"testing"

	"clawreef/internal/runtimeauth"
)

func TestUpgradeMySQLPreviewApprovesOnlyFrozenMigratedSuccessor(t *testing.T) {
	for _, name := range []string{"reserved-target", "retrying", "held", "wrong-user", "wrong-pod", "wrong-generation", "not-activated", "failed", "isolated", "bad-receipt", "wrong-migration-owner", "wrong-digest"} {
		t.Run(name, func(t *testing.T) {
			sess, u, target := recoveryActionsDB(t)
			ctx := context.Background()
			p, err := u.Preflight(ctx, RuntimeUpgradePreflightRequest{TargetImageRef: target, CandidateInstanceIDs: []int{481}, RecoveryMode: "retry"})
			if err != nil || !p.Passed {
				t.Fatalf("preflight %+v %v", p, err)
			}
			exec := func(q string, args ...any) {
				t.Helper()
				if _, e := sess.SQL().Exec(q, args...); e != nil {
					t.Fatal(e)
				}
			}
			exec(`UPDATE runtime_rollouts SET status='running',phase='gateway_restart' WHERE id=?`, p.Rollout.ID)
			exec(`UPDATE runtime_upgrade_items SET state='restart_ready',recovery_status='starting',restart_target_pod_uid='target-uid',postflight_json=? WHERE rollout_id=?`, `{"migration":{"instance_id":481,"status":"validated","output_sha256":"verified-output","session_catalog_sha256":"verified-catalog","session_count":0}}`, p.Rollout.ID)
			exec(`UPDATE runtime_pods SET pod_uid='target-uid' WHERE id=319`)
			exec(`UPDATE instances SET runtime_generation=9 WHERE id=481`)
			exec(`UPDATE instance_runtime_bindings SET generation=9,gateway_id='pending-481-9' WHERE instance_id=481`)
			id := runtimeauth.Identity{InstanceID: 481, UserID: 1, Generation: 9, PodUID: "target-uid"}
			switch name {
			case "retrying":
				exec(`UPDATE runtime_upgrade_items SET recovery_status='retrying' WHERE rollout_id=?`, p.Rollout.ID)
			case "held":
				exec(`UPDATE runtime_upgrade_items SET recovery_status='needs_attention' WHERE rollout_id=?`, p.Rollout.ID)
			case "wrong-user":
				id.UserID++
			case "wrong-pod":
				id.PodUID = "other"
			case "wrong-generation":
				id.Generation++
			case "not-activated":
				exec(`UPDATE runtime_rollouts SET phase='session_migration' WHERE id=?`, p.Rollout.ID)
			case "failed":
				exec(`UPDATE runtime_rollouts SET status='error' WHERE id=?`, p.Rollout.ID)
			case "isolated":
				exec(`UPDATE runtime_upgrade_items SET isolated_at=NOW() WHERE rollout_id=?`, p.Rollout.ID)
			case "bad-receipt":
				exec(`UPDATE runtime_upgrade_items SET postflight_json='{}' WHERE rollout_id=?`, p.Rollout.ID)
			case "wrong-migration-owner":
				exec(`UPDATE runtime_upgrade_items SET postflight_json=JSON_SET(postflight_json,'$.migration.instance_id',482) WHERE rollout_id=?`, p.Rollout.ID)
			case "wrong-digest":
				exec(`UPDATE runtime_rollouts SET target_image_digest='sha256:wrong' WHERE id=?`, p.Rollout.ID)
			}
			image, err := u.ManagedPreviewUpgradeImage(ctx, id)
			if name == "reserved-target" || name == "retrying" {
				if err != nil || image != p.Rollout.TargetImageRef {
					t.Fatalf("admitted successor not approved: %q %v", image, err)
				}
			} else if image != "" {
				t.Fatalf("unproven target approved: %q", image)
			}
		})
	}
}
