package services

import (
	"context"
	"errors"
	"testing"

	"clawreef/internal/models"
)

func TestUpgradeMySQLTerminalCreatingReservationOffersOnlyProvenRecovery(t *testing.T) {
	for _, name := range []string{"unpublished", "newer-publication", "no-receipt", "wrong-target", "wrong-owner", "running-binding", "active-upgrade"} {
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
			exec(`UPDATE runtime_rollouts SET status='error',phase='gateway_restart' WHERE id=?`, p.Rollout.ID)
			exec(`UPDATE runtime_upgrade_items SET state='restart_ready',recovery_status='needs_attention',restart_attempts=1,restart_target_pod_uid='source-uid',snapshot_ref='official-session-archive',postflight_json=? WHERE rollout_id=?`, `{"migration":{"instance_id":481,"status":"validated","output_sha256":"verified-output","session_catalog_sha256":"verified-catalog","session_count":0}}`, p.Rollout.ID)
			exec(`UPDATE instances SET status='creating',runtime_generation=9 WHERE id=481`)
			exec(`UPDATE instance_runtime_bindings SET gateway_id='pending-481-9',generation=9,state='creating',gateway_pid=NULL WHERE instance_id=481`)
			b, _ := u.bindings.GetByInstanceID(ctx, 481)
			proof := &retainedColdSourceInventory{recoveryInventory: u.deployments.(recoveryInventory), source: models.InstanceIdleStopOperation{InstanceID: 481, UserID: 1, Generation: 8, RuntimePodID: 319, GatewayID: "gw-481-8", PodUID: "source-uid", Endpoint: "http://10.1.2.2:19090", WorkspacePath: b.WorkspacePath, StorageIdentity: "verified-source", Phase: "completed"}}
			u.deployments = proof
			switch name {
			case "newer-publication":
				proof.err = errors.New("newer or unknown generation was published")
			case "no-receipt":
				exec(`UPDATE runtime_upgrade_items SET postflight_json='{}' WHERE rollout_id=?`, p.Rollout.ID)
			case "wrong-target":
				exec(`UPDATE runtime_upgrade_items SET restart_target_pod_uid='replacement' WHERE rollout_id=?`, p.Rollout.ID)
			case "wrong-owner":
				proof.source.UserID++
			case "running-binding":
				exec(`UPDATE instance_runtime_bindings SET gateway_id='gw-481-9',state='running' WHERE instance_id=481`)
			case "active-upgrade":
				exec(`UPDATE runtime_rollouts SET status='running' WHERE id=?`, p.Rollout.ID)
			}
			info, err := u.RecoveryInfo(ctx, 481)
			if err != nil {
				t.Fatal(err)
			}
			if info.CanRetry != (name == "unpublished") {
				t.Fatalf("creating recovery proposal %+v", info)
			}
			after, _ := u.bindings.GetByInstanceID(ctx, 481)
			if after == nil || after.ID != b.ID || after.Generation != 9 {
				t.Fatal("read-only proposal altered reservation")
			}
		})
	}
}
