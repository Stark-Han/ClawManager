package k8s

import (
	"context"
	"testing"

	"k8s.io/client-go/kubernetes/fake"
)

func TestRolloutTargetsExcludeHistoricalAndExperimentalPools(t *testing.T) {
	live := BuildRuntimeDeployment(RuntimeDeploymentSpec{Name: "openclaw-runtime", Namespace: "tenant", RuntimeType: "openclaw", Image: "stable", Replicas: 1})
	lab := live.DeepCopy()
	lab.Name = "openclaw-upgrade-lab-r6-source"
	zero := live.DeepCopy()
	zero.Name = "old-pool"
	n := int32(0)
	zero.Spec.Replicas = &n
	disabled := live.DeepCopy()
	disabled.Name = "disabled"
	disabled.Labels["clawmanager.io/scheduling-enabled"] = "false"
	service := NewRuntimeDeploymentService(fake.NewSimpleClientset(live, lab, zero, disabled)).(RuntimeRolloutInventory)
	targets, err := service.RolloutTargets(context.Background(), "tenant", "openclaw")
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].Name != "openclaw-runtime" {
		t.Fatalf("unexpected targets: %#v", targets)
	}
}

func TestRolloutInventoryRejectsMalformedLiveDeploymentBeforeMutation(t *testing.T) {
	d := BuildRuntimeDeployment(RuntimeDeploymentSpec{Name: "broken", Namespace: "tenant", RuntimeType: "openclaw", Image: "stable", Replicas: 1})
	d.Spec.Template.Spec.Containers[0].Name = "unexpected"
	client := fake.NewSimpleClientset(d)
	service := NewRuntimeDeploymentService(client).(RuntimeRolloutInventory)
	if _, err := service.RolloutTargets(context.Background(), "tenant", "openclaw"); err == nil {
		t.Fatal("expected preflight failure")
	}
	for _, a := range client.Actions() {
		if a.GetVerb() != "list" {
			t.Fatalf("unexpected mutation: %#v", a)
		}
	}
}
