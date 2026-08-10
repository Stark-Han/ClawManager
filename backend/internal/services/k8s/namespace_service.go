package k8s

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NamespaceService handles namespace operations
type NamespaceService struct {
	client *Client
}

// NewNamespaceService creates a new namespace service
func NewNamespaceService() *NamespaceService {
	return &NamespaceService{
		client: globalClient,
	}
}

// EnsureNamespace ensures a namespace exists, creates it if not
func (s *NamespaceService) EnsureNamespace(ctx context.Context, userID int) (*corev1.Namespace, error) {
	if s.client == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}

	namespace := s.client.GetNamespace(userID)

	// Try to get the namespace
	ns, err := s.client.Clientset.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err == nil {
		// Namespace already exists
		return ns, nil
	}

	// If not found, create it
	if errors.IsNotFound(err) {
		newNs := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
				Labels: map[string]string{
					"app":        "clawreef",
					"user-id":    fmt.Sprintf("%d", userID),
					"managed-by": "clawreef",
				},
			},
		}

		createdNs, err := s.client.Clientset.CoreV1().Namespaces().Create(ctx, newNs, metav1.CreateOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to create namespace %s: %w", namespace, err)
		}

		return createdNs, nil
	}

	return nil, fmt.Errorf("failed to get namespace %s: %w", namespace, err)
}

// DeleteNamespace deletes a namespace
func (s *NamespaceService) DeleteNamespace(ctx context.Context, userID int) error {
	if s.client == nil {
		return fmt.Errorf("k8s client not initialized")
	}

	namespace := s.client.GetNamespace(userID)

	err := s.client.Clientset.CoreV1().Namespaces().Delete(ctx, namespace, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete namespace %s: %w", namespace, err)
	}

	return nil
}

// NamespaceExists checks if a namespace exists
func (s *NamespaceService) NamespaceExists(ctx context.Context, userID int) (bool, error) {
	if s.client == nil {
		return false, fmt.Errorf("k8s client not initialized")
	}

	namespace := s.client.GetNamespace(userID)

	_, err := s.client.Clientset.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check namespace %s: %w", namespace, err)
	}

	return true, nil
}
