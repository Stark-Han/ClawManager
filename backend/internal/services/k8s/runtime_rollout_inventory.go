package k8s

import (
	"context"
	"fmt"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// RuntimeRolloutInventory is separate from the historical registration ledger.
// Inventory is read before any image mutation; API/RBAC errors are not treated
// as an empty pool or permission to fall back to guessed deployment names.
type RuntimeRolloutTarget struct {
	Namespace string
	Name      string
	UID       string
	Image     string
	Ready     bool
	Upgrade   bool
}

type RuntimeRolloutInventory interface {
	RolloutTargets(context.Context, string, string) ([]RuntimeRolloutTarget, error)
}

func ordinaryRuntimeDeployment(d appsv1.Deployment) bool {
	if d.DeletionTimestamp != nil || (d.Spec.Replicas != nil && *d.Spec.Replicas == 0) {
		return false
	}
	meta := d.Labels
	if strings.HasPrefix(d.Name, "openclaw-upgrade-lab-") ||
		meta["clawmanager.io/pool-purpose"] == "openclaw-upgrade-lab" ||
		meta["clawmanager.io/pool-role"] == "upgrade-lab" ||
		meta["clawmanager.io/scheduling-enabled"] == "false" {
		return false
	}
	return meta["clawmanager.io/pool-role"] != "upgrade-target" || meta["clawmanager.io/scheduling-enabled"] == "true"
}

func (s *runtimeDeploymentService) RolloutTargets(ctx context.Context, namespace, runtimeType string) ([]RuntimeRolloutTarget, error) {
	if s == nil || s.client == nil || strings.TrimSpace(namespace) == "" || strings.TrimSpace(runtimeType) == "" {
		return nil, fmt.Errorf("runtime rollout inventory requires client, namespace and type")
	}
	items, err := s.client.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.Set{"clawmanager.io/runtime-type": runtimeType}.String(),
	})
	if err != nil {
		return nil, fmt.Errorf("inspect live runtime deployments: %w", err)
	}
	result := make([]RuntimeRolloutTarget, 0, len(items.Items))
	for _, d := range items.Items {
		if !ordinaryRuntimeDeployment(d) {
			continue
		}
		image := ""
		for _, c := range d.Spec.Template.Spec.Containers {
			if c.Name == "runtime" {
				image = strings.TrimSpace(c.Image)
			}
		}
		if image == "" {
			return nil, fmt.Errorf("runtime deployment %s/%s has no runtime image", namespace, d.Name)
		}
		desired := int32(1)
		if d.Spec.Replicas != nil {
			desired = *d.Spec.Replicas
		}
		ready := d.Status.ObservedGeneration >= d.Generation && d.Status.UpdatedReplicas == desired && d.Status.Replicas == desired && d.Status.AvailableReplicas == desired
		upgrade := d.Labels["clawmanager.io/pool-role"] == "upgrade-target"
		for _, c := range d.Spec.Template.Spec.Containers {
			if c.Name != "runtime" {
				continue
			}
			for _, e := range c.Env {
				if e.Name == "CLAWMANAGER_RUNTIME_UPGRADE_ID" && e.Value != "" {
					upgrade = true
				}
			}
		}
		result = append(result, RuntimeRolloutTarget{Namespace: namespace, Name: d.Name, UID: string(d.UID), Image: image, Ready: ready, Upgrade: upgrade})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}
