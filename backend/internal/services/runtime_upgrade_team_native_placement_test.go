package services

import (
	"clawreef/internal/models"
	"clawreef/internal/repository"
	"context"
	"strings"
	"testing"
)

func TestConfirmedColdTeamSourceRequiresCompleteGenerationChain(t *testing.T) {
	s := models.InstanceIdleStopOperation{InstanceID: 7, UserID: 2, Generation: 1, WorkspacePath: "/workspaces/openclaw/user-2/instance-7", Phase: "completed", RuntimePodID: 42, PodUID: "source", GatewayID: "gw-7-1", Endpoint: "http://10.0.0.1:19090"}
	zero := s
	zero.Generation = 2
	zero.RuntimePodID = 0
	zero.PodUID = ""
	zero.GatewayID = ""
	zero.Endpoint = ""
	for _, tc := range []struct {
		name string
		rows []models.InstanceIdleStopOperation
		good bool
	}{
		{"confirmed source", []models.InstanceIdleStopOperation{zero, s}, true},
		{"missing generation", []models.InstanceIdleStopOperation{s}, false},
		{"pending stop", []models.InstanceIdleStopOperation{func() models.InstanceIdleStopOperation { v := zero; v.Phase = "stopping"; return v }(), s}, false},
		{"wrong workspace", []models.InstanceIdleStopOperation{func() models.InstanceIdleStopOperation { v := zero; v.WorkspacePath = "other"; return v }(), s}, false},
		{"partial source identity", []models.InstanceIdleStopOperation{func() models.InstanceIdleStopOperation { v := zero; v.PodUID = "orphan"; return v }(), s}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := confirmedColdTeamSource(7, 2, 3, s.WorkspacePath, tc.rows)
			if ok != tc.good {
				t.Fatalf("accepted=%v", ok)
			}
		})
	}
}

func TestColdTeamNativePlacementMySQLPreservesVersionAndUpgradeFences(t *testing.T) {
	sess := upgradeFlowDB(t)
	ctx := context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := sess.SQL().Exec(q, args...); e != nil {
			t.Fatal(e)
		}
	}
	exec(`ALTER TABLE instances ADD runtime_security_profile VARCHAR(64) NOT NULL DEFAULT ''`)
	exec(`ALTER TABLE teams ADD preview_security_profile VARCHAR(64) NOT NULL DEFAULT ''`)
	exec(`ALTER TABLE team_members ADD instance_id INT, ADD team_id INT, ADD status VARCHAR(32)`)
	exec(`CREATE TABLE team_runtime_lifecycle(team_id INT PRIMARY KEY,state VARCHAR(32),desired_state VARCHAR(32),members_json LONGTEXT)`)
	workspace := "/workspaces/openclaw/user-2/instance-7"
	exec(`INSERT INTO instances(id,user_id,type,runtime_type,instance_mode,status,runtime_generation,workspace_path,runtime_security_profile) VALUES(7,2,'openclaw','gateway','lite','creating',3,?,?)`, workspace, RuntimeSecurityProfile)
	exec(`INSERT INTO teams(id,preview_security_profile) VALUES(9,?)`, models.TeamPreviewSecurityProfile)
	exec(`INSERT INTO team_members(id,instance_id,team_id,status) VALUES(8,7,9,'offline')`)
	exec(`INSERT INTO team_runtime_lifecycle VALUES(9,'starting','running',?)`, `[{"instance_id":7,"member_id":8,"generation":2,"workspace":"`+workspace+`"}]`)
	exec(`INSERT INTO instance_idle_stop_operations(id,instance_id,user_id,generation,runtime_pod_id,gateway_id,pod_uid,endpoint,workspace_path,storage_identity,phase,created_at,updated_at) VALUES('source',7,2,1,42,'gw-7-1','source','http://10.0.0.1:19090',?,'','completed',NOW()-INTERVAL 1 MINUTE,NOW()),('unbound',7,2,2,0,'','','',?,'','completed',NOW(),NOW())`, workspace, workspace)
	oldDigest, newDigest := "sha256:"+strings.Repeat("a", 64), "sha256:"+strings.Repeat("b", 64)
	oldImage, newImage := "repo@"+oldDigest, "repo@"+newDigest
	exec(`INSERT INTO runtime_pods(id,runtime_type,namespace,pod_name,deployment_name,image_ref,image_digest,pod_uid,agent_endpoint,state,capacity,used_slots,created_at,updated_at) VALUES(42,'openclaw','tenant','source-pod','source',?,?,'source','http://10.0.0.1:19090','ready',2,0,NOW(),NOW())`, oldImage, oldDigest)
	old := elasticManagementPod(42)
	old.RuntimeType = "openclaw"
	old.Namespace = "tenant"
	old.DeploymentName = "source"
	old.ImageRef = oldImage
	old.ImageDigest = &oldDigest
	old.PodUID = stringPtrOrNil("source")
	old.RegisteredPodUID = old.PodUID
	old.KubernetesPodUID = old.PodUID
	old.KubernetesReady = true
	old.WorkspaceStorageIdentity = runtimeStorageIdentityForTest(2, "pvc:tenant/shared:pvc-uid:pv-name", "/data", "", false, false)
	target := old
	target.ID = 43
	target.ImageRef = newImage
	target.ImageDigest = &newDigest
	target.PodUID = stringPtrOrNil("target")
	target.RegisteredPodUID = target.PodUID
	target.KubernetesPodUID = target.PodUID
	inventory := recoveryInventory{old, target}
	service := NewRuntimeUpgradeService(sess, repository.NewRuntimeRolloutRepository(sess), repository.NewRuntimePodRepository(sess), repository.NewInstanceRuntimeBindingRepository(sess), nil, "", "")
	service.deployments = inventory
	version := "2026.10.2"
	inspect := func(_ context.Context, image string, _ map[string]string, _ bool) (*OpenClawTargetClassification, error) {
		v := "2026.10.2"
		if image == newImage {
			v = version
		}
		return &OpenClawTargetClassification{ImageDigest: imageDigestFromReference(image), RuntimeVersion: v}, nil
	}
	check := func(good bool) {
		t.Helper()
		got, ok, e := service.coldTeamNativePlacement(ctx, 7, []models.RuntimePod{target}, inspect)
		if e != nil || ok != good || (good && (len(got) != 1 || got[0].ID != 43)) {
			t.Fatalf("got=%v ok=%v err=%v", got, ok, e)
		}
	}
	check(true)
	version = "2026.11.1"
	check(false)
	version = "2026.10.2"
	target.WorkspaceStorageIdentity = runtimeStorageIdentityForTest(2, "pvc:tenant/other:other:other", "/data", "", false, false)
	service.deployments = recoveryInventory{old, target}
	check(false)
	target.WorkspaceStorageIdentity = old.WorkspaceStorageIdentity
	service.deployments = inventory
	exec(`UPDATE instance_idle_stop_operations SET phase='stopping' WHERE id='unbound'`)
	check(false)
	exec(`UPDATE instance_idle_stop_operations SET phase='completed' WHERE id='unbound'`)
	exec(`UPDATE team_runtime_lifecycle SET desired_state='stopped'`)
	check(false)
	exec(`UPDATE team_runtime_lifecycle SET desired_state='running'`)
	exec(`INSERT INTO runtime_rollouts(id,runtime_type,target_image_ref,status,phase,batch_size,preflight_id,preflight_json) VALUES(2,'openclaw',?,'finished','completed',1,'excluded','{"exclude_instance_ids":[7]}')`, newImage)
	check(false)
	exec(`DELETE FROM runtime_rollouts WHERE id=2`)
	exec(`INSERT INTO runtime_rollouts(id,runtime_type,target_image_ref,status,phase,batch_size) VALUES(1,'openclaw',?,'finished','completed',1)`, newImage)
	exec(`INSERT INTO runtime_upgrade_items(rollout_id,instance_id,state,created_at,updated_at) VALUES(1,7,'verified',NOW(),NOW())`)
	check(false)
}
