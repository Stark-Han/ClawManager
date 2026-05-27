package handlers

import "testing"

func TestWorkspaceArchiveMaxMiB(t *testing.T) {
	t.Setenv(workspaceArchiveMaxMiBEnv, "")
	if got := workspaceArchiveMaxMiB(); got != defaultWorkspaceArchiveMaxMiB {
		t.Fatalf("expected default archive limit %d MiB, got %d", defaultWorkspaceArchiveMaxMiB, got)
	}

	t.Setenv(workspaceArchiveMaxMiBEnv, "750")
	if got := workspaceArchiveMaxMiB(); got != 750 {
		t.Fatalf("expected env archive limit 750 MiB, got %d", got)
	}

	t.Setenv(workspaceArchiveMaxMiBEnv, "0")
	if got := workspaceArchiveMaxMiB(); got != defaultWorkspaceArchiveMaxMiB {
		t.Fatalf("expected invalid archive limit to fall back to %d MiB, got %d", defaultWorkspaceArchiveMaxMiB, got)
	}

	t.Setenv(workspaceArchiveMaxMiBEnv, "not-a-number")
	if got := workspaceArchiveMaxMiB(); got != defaultWorkspaceArchiveMaxMiB {
		t.Fatalf("expected unparsable archive limit to fall back to %d MiB, got %d", defaultWorkspaceArchiveMaxMiB, got)
	}
}
