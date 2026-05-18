package k8s

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConfigMapService handles Kubernetes ConfigMap writes used for non-sensitive runtime config.
type ConfigMapService struct {
	client           *Client
	namespaceService *NamespaceService
}

// NewConfigMapService creates a new ConfigMap service.
func NewConfigMapService() *ConfigMapService {
	return &ConfigMapService{
		client:           globalClient,
		namespaceService: NewNamespaceService(),
	}
}

// UpsertConfigMap creates or updates a ConfigMap in the user's namespace.
func (s *ConfigMapService) UpsertConfigMap(ctx context.Context, userID int, name string, data map[string]string, labels map[string]string) error {
	if s.client == nil || s.client.Clientset == nil {
		return fmt.Errorf("k8s client not initialized")
	}

	if _, err := s.namespaceService.EnsureNamespace(ctx, userID); err != nil {
		return fmt.Errorf("failed to ensure namespace: %w", err)
	}

	namespace := s.client.GetNamespace(userID)
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Data: data,
	}

	existing, err := s.client.Clientset.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if err == nil && existing != nil {
		existing.Data = data
		if existing.Labels == nil {
			existing.Labels = map[string]string{}
		}
		for key, value := range labels {
			existing.Labels[key] = value
		}
		if _, err := s.client.Clientset.CoreV1().ConfigMaps(namespace).Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("failed to update configmap %s/%s: %w", namespace, name, err)
		}
		return nil
	}
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to inspect configmap %s/%s: %w", namespace, name, err)
	}

	if _, err := s.client.Clientset.CoreV1().ConfigMaps(namespace).Create(ctx, configMap, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create configmap %s/%s: %w", namespace, name, err)
	}
	return nil
}

// DeleteConfigMap deletes a ConfigMap from the user's namespace. Missing maps are treated as already deleted.
func (s *ConfigMapService) DeleteConfigMap(ctx context.Context, userID int, name string) error {
	if s.client == nil || s.client.Clientset == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	namespace := s.client.GetNamespace(userID)
	if err := s.client.Clientset.CoreV1().ConfigMaps(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to delete configmap %s/%s: %w", namespace, name, err)
	}
	return nil
}
