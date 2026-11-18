package k8s

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const RuntimeFreshPoolPurposePrefix = "fresh-instance-"
const freshPoolSourceUID = "clawmanager.io/fresh-source-uid"
const freshPoolActivationUID = "clawmanager.io/fresh-activation-uid"

type FreshRuntimePoolScope struct {
	Source           RuntimePodScaleTarget
	SourceGeneration int64
	Name, Image      string
	InstanceID       int
}

type FreshRuntimePoolDeploymentService interface {
	EnsureFreshRuntimePool(context.Context, FreshRuntimePoolScope) (*appsv1.Deployment, error)
	ActivateFreshRuntimePool(context.Context, FreshRuntimePoolScope, string) error
}

func (s *runtimeDeploymentService) freshRuntimeTemplates(ctx context.Context, scope FreshRuntimePoolScope) (*appsv1.Deployment, error) {
	source, err := s.client.AppsV1().Deployments(scope.Source.Namespace).Get(ctx, scope.Source.DeploymentName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if source.UID == "" || string(source.UID) != scope.Source.DeploymentUID || scope.SourceGeneration < 1 || source.Generation != scope.SourceGeneration || source.DeletionTimestamp != nil || source.Labels["clawmanager.io/runtime-type"] != scope.Source.RuntimeType || scope.InstanceID < 1 || scope.Name == "" || scope.Name == source.Name || source.Spec.Template.Labels["clawmanager.io/runtime-protocol"] != "container-slots-v1" {
		return nil, fmt.Errorf("fresh Runtime source identity or protocol changed")
	}
	coordinator := runtimeContainerImage(source.Spec.Template.Spec.Containers)
	if runtimeImageDigest(scope.Image) == "" || runtimeImageDigest(coordinator) == "" || strings.SplitN(scope.Image, "@", 2)[0] != strings.SplitN(coordinator, "@", 2)[0] {
		return nil, fmt.Errorf("fresh Runtime image requires the approved source repository and an immutable digest")
	}
	purpose := RuntimeFreshPoolPurposePrefix + strconv.Itoa(scope.InstanceID)
	target, err := buildUpgradeRuntimePool(source, source.Namespace, source.Name, scope.Name, scope.Image, purpose, nil)
	if err != nil {
		return nil, err
	}
	one := int32(1)
	target.Spec.Replicas = &one
	target.Annotations = map[string]string{freshPoolSourceUID: scope.Source.DeploymentUID}
	for _, labels := range []map[string]string{target.Labels, target.Spec.Template.Labels, target.Spec.Selector.MatchLabels} {
		delete(labels, runtimeUpgradeIDLabel)
		delete(labels, runtimePoolRoleLabel)
		delete(labels, runtimeSourceLabel)
		labels[upgradeLabPurposeLabel] = purpose
	}
	for i := range target.Spec.Template.Spec.TopologySpreadConstraints {
		target.Spec.Template.Spec.TopologySpreadConstraints[i].LabelSelector = target.Spec.Selector.DeepCopy()
	}
	for i := range target.Spec.Template.Spec.Containers {
		removeEnvVar(&target.Spec.Template.Spec.Containers[i], "CLAWMANAGER_RUNTIME_UPGRADE_ID")
	}
	return target, nil
}

func freshRuntimeMatches(current, desired *appsv1.Deployment) bool {
	return current.UID != "" && current.DeletionTimestamp == nil && current.Annotations[freshPoolSourceUID] == desired.Annotations[freshPoolSourceUID] && current.Labels[upgradeLabPurposeLabel] == desired.Labels[upgradeLabPurposeLabel] && current.Labels["clawmanager.io/runtime-type"] == desired.Labels["clawmanager.io/runtime-type"] && apiequality.Semantic.DeepEqual(current.Spec, desired.Spec)
}

func (s *runtimeDeploymentService) EnsureFreshRuntimePool(ctx context.Context, scope FreshRuntimePoolScope) (*appsv1.Deployment, error) {
	desired, err := s.freshRuntimeTemplates(ctx, scope)
	if err != nil {
		return nil, err
	}
	deployments := s.client.AppsV1().Deployments(scope.Source.Namespace)
	current, err := deployments.Get(ctx, scope.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return deployments.Create(ctx, desired, metav1.CreateOptions{})
	}
	if err != nil {
		return nil, err
	}
	if !freshRuntimeMatches(current, desired) {
		return nil, fmt.Errorf("fresh Runtime controller has a different scope or template")
	}
	return current, nil
}

func (s *runtimeDeploymentService) ActivateFreshRuntimePool(ctx context.Context, scope FreshRuntimePoolScope, uid string) error {
	desired, err := s.freshRuntimeTemplates(ctx, scope)
	if err != nil {
		return err
	}
	deployments := s.client.AppsV1().Deployments(scope.Source.Namespace)
	current, err := deployments.Get(ctx, scope.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if !freshRuntimeMatches(current, desired) || string(current.UID) != uid || current.Status.ReadyReplicas != 1 || current.Status.ObservedGeneration < current.Generation {
		return fmt.Errorf("fresh Runtime controller changed or is not ready")
	}
	if current.Annotations[freshPoolActivationUID] == uid {
		return nil
	}
	current.Annotations[freshPoolActivationUID] = uid
	current.Labels[runtimeSchedulingLabel] = "true"
	_, err = deployments.Update(ctx, current, metav1.UpdateOptions{})
	return err
}
