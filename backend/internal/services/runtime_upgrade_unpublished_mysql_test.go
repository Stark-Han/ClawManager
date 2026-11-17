package services

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"clawreef/internal/models"
	"clawreef/internal/repository"
)

func TestUpgradeMySQLUnpublishedReservationRetiresOnlyUnderClaimAndPublicationFence(t *testing.T) {
	for _, name := range []string{"retire", "low-counter", "publication-race", "missing-proof", "wrong-owner", "missing-storage", "missing-claim", "expired-claim", "native-pid", "changed-binding"} {
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
			exec(`UPDATE runtime_rollouts SET status='running',phase='session_migration' WHERE id=?`, p.Rollout.ID)
			exec(`UPDATE instance_runtime_bindings SET gateway_id='pending-481-8',state='creating',gateway_pid=NULL WHERE instance_id=481`)
			exec(`UPDATE runtime_pods SET used_slots=3 WHERE id=319`)
			if name == "low-counter" {
				exec(`UPDATE runtime_pods SET used_slots=1 WHERE id=319`)
			}
			b, err := u.bindings.GetByInstanceID(ctx, 481)
			if err != nil {
				t.Fatal(err)
			}
			proof := &retainedColdSourceInventory{recoveryInventory: u.deployments.(recoveryInventory), source: models.InstanceIdleStopOperation{InstanceID: 481, UserID: 1, Generation: 7, RuntimePodID: 319, GatewayID: "gw-481-7", PodUID: "source-uid", Endpoint: "http://10.1.2.2:19090", WorkspacePath: b.WorkspacePath, StorageIdentity: "verified-storage", Phase: "completed"}}
			u.deployments = proof
			u.agent = &slotRuntimeAgentClient{}
			c := runtimeUpgradeCandidate{InstanceID: 481, UserID: 1, RuntimePodID: &b.RuntimePodID, Generation: 8, GatewayID: b.GatewayID, WorkspacePath: b.WorkspacePath, BindingState: "creating"}
			switch name {
			case "missing-proof":
				proof.err = errors.New("finalized source unavailable")
			case "wrong-owner":
				proof.source.UserID++
			case "missing-storage":
				proof.source.StorageIdentity = ""
			case "native-pid":
				exec(`UPDATE instance_runtime_bindings SET gateway_pid=123 WHERE instance_id=481`)
			case "changed-binding":
				c.Generation++
			}
			var stopped bool
			run := func(ctx context.Context) error {
				if name == "expired-claim" {
					exec(`UPDATE runtime_upgrade_items SET operation_deadline_at=NOW()-INTERVAL 1 SECOND WHERE rollout_id=?`, p.Rollout.ID)
				}
				var e error
				stopped, e = u.stopCandidateGateway(ctx, c)
				return e
			}
			if name == "publication-race" {
				// Directly exercise the production repository callback at the fence.
				var done chan error
				err = u.withUpgradeClaim(ctx, p.Rollout.ID, 481, "session_migration", func(ctx context.Context) error {
					store := u.bindings.(interface {
						RetireUnpublishedUpgradeReservation(context.Context, models.InstanceRuntimeBinding, string, func(context.Context, int, string) error) (bool, error)
					})
					var e error
					stopped, e = store.RetireUnpublishedUpgradeReservation(ctx, *b, ctx.Value(upgradeClaimKey{}).(string), func(ctx context.Context, _ int, _ string) error {
						done = make(chan error, 1)
						pod, _ := u.pods.GetByID(ctx, 319)
						go func() {
							done <- repository.WithRuntimePodAssignmentAdmission(context.Background(), sess, *pod, 481, 8, func() error { return errors.New("publication crossed fence") })
						}()
						select {
						case e := <-done:
							t.Fatalf("publication escaped admission: %v", e)
						case <-time.After(50 * time.Millisecond):
						}
						return nil
					})
					return e
				})
				if done == nil {
					t.Fatalf("fenced verifier not called: %v", err)
				}
				if e := <-done; e == nil || !strings.Contains(e.Error(), "current durable generation binding") {
					t.Fatalf("late publisher did not reject retired reservation: %v", e)
				}
			} else if name == "missing-claim" {
				err = run(ctx)
			} else {
				err = u.withUpgradeClaim(ctx, p.Rollout.ID, 481, "session_migration", run)
			}
			want := name == "retire" || name == "low-counter" || name == "publication-race"
			after, _ := u.bindings.GetByInstanceID(ctx, 481)
			pod, _ := u.pods.GetByID(ctx, 319)
			if want && (err != nil || !stopped || after != nil || pod.UsedSlots != 2) || !want && (err == nil || stopped || after == nil || pod.UsedSlots != 3) {
				t.Fatalf("retirement=%v binding=%+v slots=%d err=%v", stopped, after, pod.UsedSlots, err)
			}
		})
	}
}
