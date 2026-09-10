package services

import (
	"context"
	"fmt"
)

// ResolveHermesRolloutImage pins the manifest before changing any deployment.
// Both old and desktop-web Hermes accept digests; the latter requires one.
func ResolveHermesRolloutImage(ctx context.Context, target string) (string, string, error) {
	host, repository, reference, err := splitRegistryImage(target)
	if err != nil {
		return "", "", err
	}
	if digest := imageDigestFromReference(target); digest != "" {
		return host + "/" + repository + "@" + digest, digest, nil
	}
	if reference == "" {
		return "", "", fmt.Errorf("Hermes image tag or digest is required")
	}
	client, baseURL, err := safeRegistryClient(host, repository)
	if err != nil {
		return "", "", err
	}
	_, digest, err := fetchRegistryManifest(ctx, client, baseURL, reference)
	if err != nil {
		return "", "", fmt.Errorf("resolve Hermes image: %w", err)
	}
	if imageDigestFromReference("image@"+digest) == "" {
		return "", "", fmt.Errorf("Hermes registry returned an invalid manifest digest")
	}
	return host + "/" + repository + "@" + digest, digest, nil
}
