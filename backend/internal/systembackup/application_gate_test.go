package systembackup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"clawreef/internal/backupgate"
	"clawreef/internal/config"
	appdb "clawreef/internal/db"
	"clawreef/internal/repository"
	upper "github.com/upper/db/v4"
)

func TestRequestBoundaryDrainsExternalWritesDespiteClientCancellation(t *testing.T) {
	ctx := context.Background()
	raw := controlDB(t)
	gate := Gate{DB: raw}
	if e := gate.EnsureScope(ctx, "requests"); e != nil {
		t.Fatal(e)
	}
	requestCtx, cancel := context.WithCancel(ctx)
	release, e := backupgate.HoldRequest(requestCtx, raw, "requests")
	if e != nil {
		t.Fatal(e)
	}
	defer release()
	cancel() // The handler can still be writing despite its client disconnecting.
	wait, stop := context.WithTimeout(ctx, 150*time.Millisecond)
	if _, e = gate.Begin(wait, "requests", "capture", RegistryHash(), time.Second, 5*time.Second); e == nil {
		stop()
		t.Fatal("gate passed an unfinished HTTP side effect")
	}
	stop()
	file := filepath.Join(t.TempDir(), "partial-result")
	if e = os.WriteFile(file, []byte("complete handler write"), 0600); e != nil {
		t.Fatal(e)
	}
	release()
	owner, e := gate.Begin(ctx, "requests", "capture", RegistryHash(), time.Second, 5*time.Second)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = backupgate.HoldRequest(ctx, raw, "requests"); !errors.Is(e, backupgate.ErrMaintenance) {
		t.Fatal("new handler crossed maintenance", e)
	}
	if e = gate.BeginRelease(ctx, owner); e != nil {
		t.Fatal(e)
	}
	if e = gate.FinishRelease(ctx, owner); e != nil {
		t.Fatal(e)
	}
	if e = backupgate.RequireSameDatabase(ctx, raw, raw); e != nil {
		t.Fatal(e)
	}
	if e = backupgate.RequireSameDatabase(ctx, raw, fixtureDatabase(t)); e == nil {
		t.Fatal("different physical gate database accepted")
	}
}

func TestApplicationSQLGateCoversUpperRepositories(t *testing.T) {
	ctx := context.Background()
	raw := fullSchemaDB(t)
	var name string
	if e := raw.QueryRow("SELECT DATABASE()").Scan(&name); e != nil {
		t.Fatal(e)
	}
	gate := Gate{DB: raw}
	if e := gate.EnsureScope(ctx, "application"); e != nil {
		t.Fatal(e)
	}
	t.Setenv("SYSTEM_BACKUP_SQL_GATE_SCOPE", "application")
	port, _ := strconv.Atoi(os.Getenv("TEST_SYSTEM_BACKUP_MYSQL_PORT"))
	cfg := config.DatabaseConfig{Host: "127.0.0.1", Port: port, User: "root", Database: name, MaxOpenConns: 4, MaxIdleConns: 2}
	session, e := appdb.Initialize(cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer session.Close()
	if _, e = session.SQL().Exec("INSERT INTO teams(id,user_id,name,status) VALUES(991,1,'gate-fixture','stopped')"); e != nil {
		t.Fatal(e)
	}
	repo := repository.NewTeamRepository(session)
	if _, e = repo.GetTeamByID(991); e != nil {
		t.Fatal(e)
	}
	if e = session.Collection("teams").Find(upper.Cond{"id": 991}).Update(map[string]any{"name": "before-gate"}); e != nil {
		t.Fatal(e)
	}
	owner, e := gate.Begin(ctx, "application", "test-controller", RegistryHash(), time.Second, 5*time.Second)
	if e != nil {
		t.Fatal(e)
	}
	owner, e = gate.Hold(ctx, owner)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = repo.GetTeamByID(991); e != nil {
		t.Fatal("repository reads must remain available", e)
	}
	if e = session.Collection("teams").Find(upper.Cond{"id": 991}).Update(map[string]any{"name": "escaped"}); !errors.Is(e, backupgate.ErrMaintenance) {
		t.Fatal("ORM update escaped gate", e)
	}
	if _, e = appdb.Initialize(cfg); !errors.Is(e, backupgate.ErrMaintenance) {
		t.Fatal("business startup escaped maintenance", e)
	}
	if e = gate.BeginRelease(ctx, owner); e != nil {
		t.Fatal(e)
	}
	if e = gate.FinishRelease(ctx, owner); e != nil {
		t.Fatal(e)
	}
	if e = session.Collection("teams").Find(upper.Cond{"id": 991}).Update(map[string]any{"name": "after-gate"}); e != nil {
		t.Fatal(e)
	}
}
