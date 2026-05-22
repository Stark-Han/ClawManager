package k8s

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestHostPathPVNodeAffinitySelectsReadyNode(t *testing.T) {
	service := &PVCService{
		client: &Client{
			Clientset: fake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-b",
						Labels: map[string]string{
							"kubernetes.io/hostname": "node-b-host",
						},
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-a",
						Labels: map[string]string{
							"kubernetes.io/hostname": "node-a-host",
						},
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
						},
					},
				},
			),
		},
	}

	affinity, err := service.hostPathPVNodeAffinity(context.Background())
	if err != nil {
		t.Fatalf("hostPathPVNodeAffinity returned error: %v", err)
	}
	requireHostnameAffinity(t, affinity, "node-a-host")
}

func TestHostPathPVNodeAffinitySkipsUnschedulableAndNotReadyNodes(t *testing.T) {
	service := &PVCService{
		client: &Client{
			Clientset: fake.NewSimpleClientset(
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-a",
						Labels: map[string]string{
							"kubernetes.io/hostname": "node-a-host",
						},
					},
					Spec: corev1.NodeSpec{Unschedulable: true},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-b",
						Labels: map[string]string{
							"kubernetes.io/hostname": "node-b-host",
						},
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{Type: corev1.NodeReady, Status: corev1.ConditionFalse},
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-c",
						Labels: map[string]string{
							"kubernetes.io/hostname": "node-c-host",
						},
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
						},
					},
				},
			),
		},
	}

	affinity, err := service.hostPathPVNodeAffinity(context.Background())
	if err != nil {
		t.Fatalf("hostPathPVNodeAffinity returned error: %v", err)
	}
	requireHostnameAffinity(t, affinity, "node-c-host")
}

func requireHostnameAffinity(t *testing.T, affinity *corev1.VolumeNodeAffinity, hostname string) {
	t.Helper()
	if affinity == nil || affinity.Required == nil || len(affinity.Required.NodeSelectorTerms) != 1 {
		t.Fatalf("unexpected node affinity: %#v", affinity)
	}
	expressions := affinity.Required.NodeSelectorTerms[0].MatchExpressions
	if len(expressions) != 1 {
		t.Fatalf("unexpected node affinity expressions: %#v", expressions)
	}
	expression := expressions[0]
	if expression.Key != "kubernetes.io/hostname" || expression.Operator != corev1.NodeSelectorOpIn {
		t.Fatalf("unexpected node affinity expression: %#v", expression)
	}
	if len(expression.Values) != 1 || expression.Values[0] != hostname {
		t.Fatalf("expected hostname %q, got %#v", hostname, expression.Values)
	}
}
