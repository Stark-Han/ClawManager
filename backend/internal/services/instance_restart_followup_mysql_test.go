package services

import (
	"clawreef/internal/repository"
	"testing"
)

func TestDurableLiteRestartPreservesSourceAndRecoversWithoutReplay(t *testing.T) {
	s := idleLifecycleDB(t)
	ctx := t.Context()
	for _, q := range []string{
		`INSERT INTO instances(id,user_id,type,runtime_type,instance_mode,status,runtime_generation,workspace_path,updated_at) VALUES(1,7,'hermes','gateway','lite','running',2,'/workspaces/hermes/user-7/instance-1',UTC_TIMESTAMP())`,
		`INSERT INTO runtime_pods(id,runtime_type,namespace,pod_name,pod_uid,deployment_name,image_ref,agent_endpoint,state,capacity,used_slots) VALUES(11,'hermes','runtime','pool','source-uid','pool','test','https://10.0.0.2:19090','ready',1,1)`,
		`INSERT INTO instance_runtime_bindings(instance_id,runtime_pod_id,runtime_type,gateway_id,gateway_port,workspace_path,state,generation) VALUES(1,11,'hermes','gw-1-2',20000,'/workspaces/hermes/user-7/instance-1','running',2)`,
	} {
		if _, err := s.SQL().Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	store := repository.NewInstanceAutoStopRepository(s)
	if err := store.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	agent := &manualStopFailureAgent{fakeRuntimeAgentClient: &fakeRuntimeAgentClient{}, store: store, fail: true, t: t}
	svc := &instanceService{instanceRepo: repository.NewInstanceRepository(s), bindingRepo: repository.NewInstanceRuntimeBindingRepository(s), runtimePodRepo: repository.NewRuntimePodRepository(s), agentClient: agent, autoStopRepo: store}
	configureLifecycleModels(t, svc)
	if err := svc.Restart(1); err == nil {
		t.Fatal("unconfirmed old source reported restarted")
	}
	i, err := svc.instanceRepo.GetByID(1)
	if err != nil {
		t.Fatal(err)
	}
	if i.RuntimeGeneration != 2 {
		t.Fatal("source generation changed before confirmed stop")
	}
	if err = svc.instanceRepo.UpdateRuntimeState(ctx, 1, "running", 2, nil); err == nil {
		t.Fatal("old reporter revived durable stop")
	}
	pending, err := store.PendingStops(ctx)
	if err != nil || len(pending) != 1 || pending[0].Kind != "restart" {
		t.Fatalf("restart intent %+v %v", pending, err)
	}
	op := pending[0]
	agent.fail = false
	// A new controller instance resumes the same persisted source operation.
	controller := NewInstanceAutoStopService(store, svc, nil)
	controller.resume(ctx, op)
	i, err = svc.instanceRepo.GetByID(1)
	if err != nil {
		t.Fatal(err)
	}
	if i.RuntimeGeneration != 3 || i.Status != "creating" {
		t.Fatalf("restart not accepted once: %+v", i)
	}
	b, _ := svc.bindingRepo.GetByInstanceID(ctx, 1)
	if b != nil {
		t.Fatal("old binding retained after confirmed stop")
	}
	controller.resume(ctx, op)
	i, _ = svc.instanceRepo.GetByID(1)
	if i.RuntimeGeneration != 3 {
		t.Fatal("late stop snapshot replayed Start")
	}
	// Simulate loss of the final progress write after Start was committed.
	if _, err = s.SQL().Exec(`UPDATE instance_idle_stop_operations SET phase='restart_ready' WHERE id=?`, op.ID); err != nil {
		t.Fatal(err)
	}
	if err = svc.resumeRestartFollowup(ctx, 1); err != nil {
		t.Fatal(err)
	}
	i, _ = svc.instanceRepo.GetByID(1)
	if i.RuntimeGeneration != 3 {
		t.Fatal("lost response replayed Start")
	}
	// Explicit Stop cancels an unstarted follow-up, including stale snapshots.
	if err = svc.stopV2InstanceForKind(ctx, i, "restart"); err != nil {
		t.Fatal(err)
	}
	follow, err := store.RestartFollowup(ctx, 1)
	if err != nil || follow == nil {
		t.Fatalf("missing follow-up %v", err)
	}
	if err = svc.Stop(1); err != nil {
		t.Fatal(err)
	}
	controller.resume(ctx, *follow)
	i, _ = svc.instanceRepo.GetByID(1)
	if i.RuntimeGeneration != 3 || i.Status != "stopped" {
		t.Fatal("cancelled restart restarted instance")
	}
}
