package config

import "testing"

func TestLoadRuntimeDefaults(t *testing.T) {
	for _, key := range []string{
		"HERMES_RUNTIME_IMAGE",
		"OPENCLAW_RUNTIME_IMAGE",
		"RUNTIME_NAMESPACE",
		"K8S_NAMESPACE",
		"HOSTNAME",
		"PLATFORM_REDIS_URL",
		"TEAM_REDIS_URL",
	} {
		t.Setenv(key, "")
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if got, want := cfg.Runtime.HermesImage, "ghcr.io/yuan-lab-llm/agentsruntime/hermes-lite:latest"; got != want {
		t.Fatalf("expected Hermes default image %q, got %q", want, got)
	}
	if got, want := cfg.Runtime.OpenClawImage, "ghcr.io/yuan-lab-llm/agentsruntime/openclaw-lite:latest"; got != want {
		t.Fatalf("expected OpenClaw default image %q, got %q", want, got)
	}
}
