package services

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveWorkspacePathRejectsTraversalAndAbsolutePaths(t *testing.T) {
	root := t.TempDir()

	for _, relativePath := range []string{
		"../secret.txt",
		"docs/../../secret.txt",
		"/etc/passwd",
		`C:\Windows\System32\drivers\etc\hosts`,
		"C:/Windows/System32/drivers/etc/hosts",
	} {
		t.Run(relativePath, func(t *testing.T) {
			_, err := ResolveWorkspacePath(root, relativePath, false)
			if !errors.Is(err, ErrWorkspacePathEscape) && !errors.Is(err, ErrWorkspacePathInvalid) {
				t.Fatalf("ResolveWorkspacePath(%q) error = %v, want path safety error", relativePath, err)
			}
		})
	}
}

func TestResolveWorkspacePathAllowsMissingChildInsideWorkspace(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "docs"), 0750); err != nil {
		t.Fatal(err)
	}

	resolved, err := ResolveWorkspacePath(root, "docs/new.txt", true)
	if err != nil {
		t.Fatalf("ResolveWorkspacePath returned error: %v", err)
	}

	if resolved.RelativePath != "docs/new.txt" {
		t.Fatalf("relative path = %q, want docs/new.txt", resolved.RelativePath)
	}
	if !strings.HasSuffix(filepath.ToSlash(resolved.Path), "/docs/new.txt") {
		t.Fatalf("target path = %q, want path under docs/new.txt", resolved.Path)
	}
}

func TestResolveWorkspacePathRejectsExistingSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "leak.txt")); err != nil {
		t.Skipf("symlink creation is not available: %v", err)
	}

	_, err := ResolveWorkspacePath(root, "leak.txt", false)
	if !errors.Is(err, ErrWorkspacePathEscape) {
		t.Fatalf("ResolveWorkspacePath through symlink error = %v, want ErrWorkspacePathEscape", err)
	}
}

func TestResolveWorkspacePathRejectsMissingTargetUnderEscapingSymlinkParent(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Skipf("symlink creation is not available: %v", err)
	}

	_, err := ResolveWorkspacePath(root, "linked/new.txt", true)
	if !errors.Is(err, ErrWorkspacePathEscape) {
		t.Fatalf("ResolveWorkspacePath through symlink parent error = %v, want ErrWorkspacePathEscape", err)
	}
}
