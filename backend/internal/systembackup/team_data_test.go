package systembackup

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func teamDataJobFixture(t *testing.T) *TeamDataService {
	t.Helper()
	db := fixtureDatabase(t)
	raw, e := os.ReadFile("../db/migrations/093_add_team_data_backup_jobs.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(string(raw)); e != nil {
		t.Fatal(e)
	}
	return &TeamDataService{db: db, configHash: strings.Repeat("a", 64), owner: "test-worker"}
}

func TestTeamDataWorkerStartsWithInstallationMutex(t *testing.T) {
	s := teamDataJobFixture(t)
	s.cfg.InstallationID = "fixture-installation"
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	err := s.runOwned(ctx)
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatal("worker did not start and wait without panicking", err)
	}
}

func TestTeamDataJobsIdempotencyBusyAndPendingCancel(t *testing.T) {
	s := teamDataJobFixture(t)
	ctx := context.Background()
	j, e := s.Create(ctx, 1, "same-request-0001", "backup", "")
	if e != nil {
		t.Fatal(e)
	}
	same, e := s.Create(ctx, 1, "same-request-0001", "backup", "")
	if e != nil || same.ID != j.ID {
		t.Fatal("retry created another backup", e)
	}
	if _, e = s.Create(ctx, 1, "other-request-0002", "backup", ""); e == nil || e.Error() != "conflict_active_job" {
		t.Fatal("parallel capture admitted", e)
	}
	if e = s.Cancel(ctx, j.ID); e != nil {
		t.Fatal(e)
	}
	j, e = s.Get(ctx, j.ID)
	if e != nil || j.Status != "canceled" || j.StartedAt != nil || j.FinishedAt == nil {
		t.Fatal("pending cancellation started work or lost finish time", e, j.Status)
	}
	if _, e = s.Create(ctx, 1, "same-request-0001", "restore", j.ID); e == nil || e.Error() != "idempotency_conflict" {
		t.Fatal("idempotency binding changed", e)
	}
	if _, e = s.Create(ctx, 1, "bad-restore-0003", "restore", j.ID); e == nil {
		t.Fatal("restored an incomplete backup")
	}
	if _, e = s.Create(ctx, 1, "next-backup-0004", "backup", ""); e != nil {
		t.Fatal("canceled pending job retained active slot", e)
	}
}

func TestTeamDataExportPreservesTeamResultsWithoutGlobalGatewayRows(t *testing.T) {
	db := fixtureDatabase(t)
	ctx := context.Background()
	for _, q := range []string{"CREATE TABLE team_tasks(id BIGINT PRIMARY KEY,status VARCHAR(30),result_json BLOB) ENGINE=InnoDB", "CREATE TABLE model_invocations(id BIGINT PRIMARY KEY,payload BLOB) ENGINE=InnoDB", "INSERT INTO team_tasks VALUES(1,'running',X'00FF70617274')", "INSERT INTO model_invocations VALUES(1,'unrelated global metering log')"} {
		if _, e := db.ExecContext(ctx, q); e != nil {
			t.Fatal(e)
		}
	}
	var archive bytes.Buffer
	result, e := ExportTeamDataMySQL(ctx, db, &archive, []string{"team_tasks", "model_invocations"})
	if e != nil {
		t.Fatal(e)
	}
	if result.Tables["team_tasks"] != 1 || result.Tables["model_invocations"] != 0 {
		t.Fatal("wrong data scope", result.Tables)
	}
	readback, e := ReadSQLSnapshot(bytes.NewReader(archive.Bytes()), nil, nil)
	if e != nil || readback.Tables["team_tasks"] != 1 {
		t.Fatal("archive unreadable", e)
	}
	if bytes.Contains(archive.Bytes(), []byte("unrelated global")) {
		t.Fatal("global gateway payload captured")
	}
	archive.Reset()
	full, e := ExportMySQL(ctx, db, &archive, []string{"model_invocations"})
	if e != nil || full.Tables["model_invocations"] != 1 {
		t.Fatal("existing full-system export scope changed", e)
	}
}

func TestTeamDataFileAliasesPreserveSharedLinksWithoutWideningNormalCapture(t *testing.T) {
	physical, logical, target := t.TempDir(), t.TempDir(), t.TempDir()
	shared, instance := filepath.Join(physical, "shared"), filepath.Join(physical, "instance")
	for _, dir := range []string{shared, instance} {
		if e := os.Mkdir(dir, 0700); e != nil {
			t.Fatal(e)
		}
	}
	if e := os.WriteFile(filepath.Join(shared, "partial.bin"), []byte("partial artifact"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(filepath.Join(logical, "shared"), filepath.Join(instance, "team")); e != nil {
		t.Skip("symlinks unavailable on this host")
	}
	roots := []FileRoot{{ID: "shared", Path: shared}, {ID: "instance", Path: instance}}
	var archive bytes.Buffer
	if _, e := CaptureFiles(context.Background(), &archive, roots, FileBudget{1 << 20, 100}); e == nil {
		t.Fatal("normal capture accepted an external logical alias")
	}
	archive.Reset()
	if _, e := captureFiles(context.Background(), &archive, roots, FileBudget{1 << 20, 100}, map[string]string{logical: physical}); e != nil {
		t.Fatal(e)
	}
	if _, e := ExtractFiles(context.Background(), bytes.NewReader(archive.Bytes()), target, FileBudget{1 << 20, 100}); e != nil {
		t.Fatal(e)
	}
	got, e := os.ReadFile(filepath.Join(target, "roots", "instance", "team", "partial.bin"))
	if e != nil || string(got) != "partial artifact" {
		t.Fatal("shared artifact link lost", e)
	}
}
