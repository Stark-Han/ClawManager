package handlers

import "testing"

func TestValidateImportedUserAllowsEmptyPassword(t *testing.T) {
	if err := validateImportedUser("alice", "alice@example.com", "", "user"); err != "" {
		t.Fatalf("expected empty import password to be allowed, got %q", err)
	}
}

func TestValidateImportedUserRejectsShortExplicitPassword(t *testing.T) {
	if err := validateImportedUser("alice", "alice@example.com", "user123", "user"); err != "Password must be at least 8 characters" {
		t.Fatalf("expected short explicit password error, got %q", err)
	}
}
