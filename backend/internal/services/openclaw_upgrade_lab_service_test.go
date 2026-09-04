package services

import (
	"crypto/sha256"
	"encoding/hex"
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

func TestInspectUpgradeLabLegacySessionsRequiresManualConversation(t *testing.T) {
	workspace := t.TempDir()
	sessions := filepath.Join(workspace, "home", ".openclaw", "agents", "main", "sessions")
	if err := os.MkdirAll(sessions, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessions, "sessions.json"), []byte(`{"agent:main:main":{"sessionId":"session-1"}}`), 0o640); err != nil {
		t.Fatal(err)
	}
	transcript := strings.Join([]string{
		`{"type":"message","message":{"role":"user","content":"[OpenClaw heartbeat poll]"}}`,
		`{"type":"message","message":{"role":"assistant","content":[{"type":"text","text":"ok"}]}}`,
		`{"type":"message","message":{"role":"user","content":[{"type":"text","text":"请记住这个测试"}]}}`,
		`{"type":"message","message":{"role":"assistant","content":"已记住"}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(sessions, "session-1.jsonl"), []byte(transcript), 0o640); err != nil {
		t.Fatal(err)
	}
	manifest, err := inspectUpgradeLabLegacySessions(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SessionCount != 1 || manifest.UserMessageCount != 2 || manifest.InteractiveUserMessageCount != 1 || manifest.AssistantMessageCount != 2 {
		t.Fatalf("unexpected session evidence: %#v", manifest)
	}
	if manifest.CatalogSHA256 == "" || manifest.SourceFilesSHA256 == "" || len(manifest.SourceFiles) != 2 {
		t.Fatalf("incomplete session evidence: %#v", manifest)
	}
}

func TestVerifyUpgradeLabSessionArchiveMatchesBytesNotNames(t *testing.T) {
	workspace := t.TempDir()
	archive := filepath.Join(workspace, "home", ".openclaw", "agents", "main", "session-sqlite-import-archive", "run-1")
	if err := os.MkdirAll(archive, 0o750); err != nil {
		t.Fatal(err)
	}
	raw := []byte("legacy transcript\n")
	if err := os.WriteFile(filepath.Join(archive, "renamed.imported-1.jsonl"), raw, 0o640); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	ok, matched, err := verifyUpgradeLabSessionArchive(workspace, map[string]string{"old/name.jsonl": hex.EncodeToString(digest[:])})
	if err != nil {
		t.Fatal(err)
	}
	if !ok || matched != 1 {
		t.Fatalf("archive verification = %v, %d", ok, matched)
	}
}
