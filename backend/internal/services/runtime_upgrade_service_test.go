package services

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"clawreef/internal/models"
)

func TestImageDigestFromReferenceRequiresImmutableSHA256(t *testing.T) {
	digest := strings.Repeat("a", 64)
	if got := imageDigestFromReference("registry.example/openclaw@sha256:" + digest); got != "sha256:"+digest {
		t.Fatalf("digest = %q", got)
	}
	for _, invalid := range []string{"registry/openclaw:2026.8.1", "registry/openclaw@sha256:abc", "registry/openclaw@sha256:" + strings.Repeat("z", 64)} {
		if got := imageDigestFromReference(invalid); got != "" {
			t.Fatalf("invalid reference %q produced %q", invalid, got)
		}
	}
}

func TestImmutableRuntimeImagePinsTagToObservedDigest(t *testing.T) {
	digest := "sha256:" + strings.Repeat("b", 64)
	got, err := immutableRuntimeImage("10.130.14.23:5000/agentsruntime/openclaw-lite:legacy", digest)
	if err != nil {
		t.Fatal(err)
	}
	want := "10.130.14.23:5000/agentsruntime/openclaw-lite@" + digest
	if got != want {
		t.Fatalf("immutable image = %q, want %q", got, want)
	}
	if _, err := immutableRuntimeImage("registry/openclaw:legacy", ""); err == nil {
		t.Fatal("missing source digest was accepted")
	}
}

func TestLiteClassificationHonorsExplicitInstanceMode(t *testing.T) {
	for _, test := range []struct {
		name    string
		mode    string
		backend string
		want    bool
	}{
		{name: "lite gateway", mode: InstanceModeLite, backend: RuntimeBackendGateway, want: true},
		{name: "pro desktop", mode: InstanceModePro, backend: RuntimeBackendDesktop, want: false},
		{name: "explicit pro wins over conflicting gateway", mode: InstanceModePro, backend: RuntimeBackendGateway, want: false},
		{name: "legacy gateway fallback", mode: "", backend: RuntimeBackendGateway, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			instance := &models.Instance{InstanceMode: test.mode, RuntimeType: test.backend}
			if got := isLiteRuntimeInstance(instance); got != test.want {
				t.Fatalf("isLiteRuntimeInstance() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestLocalUpgradeSnapshotPreservesOpenClawWorkspaceBytes(t *testing.T) {
	root := t.TempDir()
	workspace := RuntimeWorkspacePathWithRoot(root, RuntimeTypeOpenClaw, 45, 123)
	if err := os.MkdirAll(filepath.Join(workspace, ".openclaw", "sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".openclaw", "settings.json"), []byte(`{"version":"2026.7.1-2"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	validSQLite := append([]byte("SQLite format 3\x00"), make([]byte, 64)...)
	if err := os.WriteFile(filepath.Join(workspace, ".openclaw", "sessions", "sessions.sqlite"), validSQLite, 0o600); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(workspace, ".openclaw", "tmp", "openclaw-200075", "device-identity.98ce393a.lock.sqlite")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	service := &RuntimeUpgradeService{workspaceRoot: root}
	ref, inventory, err := service.createLocalSnapshot(77, runtimeUpgradeCandidate{InstanceID: 123, UserID: 45, WorkspacePath: workspace})
	if err != nil {
		t.Fatal(err)
	}
	if !inventory.SQLiteHeaderValid || inventory.SQLiteFileCount != 1 || inventory.ManifestSHA256 == "" {
		t.Fatalf("inventory = %#v", inventory)
	}
	archivePath := strings.TrimPrefix(ref, "local:")
	if err := verifyLocalSnapshotArchive(root, ref); err != nil {
		t.Fatalf("verifyLocalSnapshotArchive() error = %v", err)
	}
	file, err := os.Open(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	foundSettings, foundSQLite, foundLock := false, false, false
	for {
		header, err := tr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatal(err)
		}
		switch header.Name {
		case ".openclaw/settings.json":
			foundSettings = true
		case ".openclaw/sessions/sessions.sqlite":
			foundSQLite = true
		case ".openclaw/tmp/openclaw-200075/device-identity.98ce393a.lock.sqlite":
			foundLock = true
		}
	}
	if !foundSettings || !foundSQLite || !foundLock {
		t.Fatalf("snapshot entries settings=%v sqlite=%v lock=%v", foundSettings, foundSQLite, foundLock)
	}
	if _, err := os.Stat(workspace); err != nil {
		t.Fatalf("snapshot modified active workspace: %v", err)
	}
}

func TestSQLiteMainDatabaseClassification(t *testing.T) {
	for _, test := range []struct {
		name string
		want bool
	}{
		{name: "sessions.sqlite", want: true},
		{name: "state.SQLITE3", want: true},
		{name: "cache.db", want: true},
		{name: "device-identity.98ce393a.lock.sqlite", want: false},
		{name: "sessions.sqlite-wal", want: false},
		{name: "sessions.sqlite-shm", want: false},
		{name: "sessions.sqlite-journal", want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := isSQLiteMainDatabase(test.name); got != test.want {
				t.Fatalf("isSQLiteMainDatabase(%q) = %v, want %v", test.name, got, test.want)
			}
		})
	}
}

func TestRuntimeWorkspaceSQLitePreflightIgnoresLockAndCompanionFiles(t *testing.T) {
	root := t.TempDir()
	validSQLite := append([]byte("SQLite format 3\x00"), make([]byte, 64)...)
	for name, data := range map[string][]byte{
		"sessions.sqlite":                  validSQLite,
		"device-identity.test.lock.sqlite": nil,
		"sessions.sqlite-wal":              []byte("wal bytes"),
		"sessions.sqlite-shm":              []byte("shm bytes"),
		"sessions.sqlite-journal":          []byte("journal bytes"),
	} {
		if err := os.WriteFile(filepath.Join(root, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := validateRuntimeWorkspaceSQLite(root); err != nil {
		t.Fatalf("preflight rejected SQLite companion file: %v", err)
	}
	inventory, err := inspectRuntimeWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	if inventory.FileCount != 5 || inventory.SQLiteFileCount != 1 {
		t.Fatalf("inventory = %#v", inventory)
	}
}

func TestInspectRuntimeWorkspaceRejectsInvalidSQLiteWithoutChangingIt(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "broken.sqlite")
	original := []byte("not a sqlite database")
	if err := os.WriteFile(dbPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := inspectRuntimeWorkspace(root); err == nil {
		t.Fatal("invalid SQLite header was accepted")
	}
	got, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatal("preflight changed invalid database bytes")
	}
}

func TestRuntimeUpgradeCandidateOrderKeepsLeaderLastWithinEachTeam(t *testing.T) {
	teamOne, teamTwo := 11, 12
	candidates := []runtimeUpgradeCandidate{
		{InstanceID: 1, TeamID: &teamOne, Role: "leader"},
		{InstanceID: 4, TeamID: &teamTwo, Role: "leader"},
		{InstanceID: 3, TeamID: &teamOne, Role: "worker"},
		{InstanceID: 2, TeamID: &teamOne, Role: "worker"},
		{InstanceID: 5, TeamID: &teamTwo, Role: "worker"},
	}
	sortRuntimeUpgradeCandidates(candidates)
	want := []int{2, 3, 1, 5, 4}
	for index, instanceID := range want {
		if candidates[index].InstanceID != instanceID {
			t.Fatalf("candidate order = %#v, want %v", candidates, want)
		}
	}
}
