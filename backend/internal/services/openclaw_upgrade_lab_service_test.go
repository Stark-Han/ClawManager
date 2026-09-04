package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectUpgradeLabProjectDataIsDeterministicAndDetectsChanges(t *testing.T) {
	workspace := t.TempDir()
	project := filepath.Join(workspace, "project")
	if err := os.MkdirAll(filepath.Join(project, "nested"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "b.txt"), []byte("two"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "nested", "a.txt"), []byte("one"), 0o640); err != nil {
		t.Fatal(err)
	}
	first, err := inspectUpgradeLabProjectData(workspace)
	if err != nil {
		t.Fatal(err)
	}
	second, err := inspectUpgradeLabProjectData(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest == "" || first.Digest != second.Digest || len(first.Files) != 2 {
		t.Fatalf("non-deterministic manifest: first=%#v second=%#v", first, second)
	}
	if err := os.WriteFile(filepath.Join(project, "b.txt"), []byte("changed"), 0o640); err != nil {
		t.Fatal(err)
	}
	changed, err := inspectUpgradeLabProjectData(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Digest == first.Digest {
		t.Fatal("project mutation was not detected")
	}
}

func TestUpgradeLabErrorRedactionDoesNotExposeCredentials(t *testing.T) {
	for _, input := range []string{
		"request failed token=secret-value more text",
		"Authorization: Bearer top-secret",
		"connector failed api_key=top-secret",
		"plugin credential igt_secret-value",
	} {
		got := redactUpgradeLabError(input)
		if strings.Contains(got, "secret-value") || strings.Contains(got, "top-secret") {
			t.Fatalf("credential remained in %q", got)
		}
		if !strings.Contains(got, "[redacted]") {
			t.Fatalf("redaction marker missing from %q", got)
		}
	}
}
