package services

import (
	"context"
	"fmt"
	"testing"
)

func TestTeamManualLegacyStopDoesNotWeakenAutomaticPreflight(t *testing.T) {
	if !legacyTeamManualStop("manual", fmt.Errorf("missing route: %w", ErrRuntimeAgentNotFound)) {
		t.Fatal("legacy confirmed manual stop blocked")
	}
	for _, reason := range []string{"manual", "idle_timeout"} {
		for _, err := range []error{context.DeadlineExceeded, fmt.Errorf("authorization failed"), nil} {
			if legacyTeamManualStop(reason, err) {
				t.Fatalf("uncertain stop admitted: %s %v", reason, err)
			}
		}
	}
	if legacyTeamManualStop("idle_timeout", ErrRuntimeAgentNotFound) {
		t.Fatal("automatic stop bypassed missing execution proof")
	}
}
