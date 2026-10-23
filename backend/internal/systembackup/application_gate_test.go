package systembackup

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"clawreef/internal/backupgate"
	"clawreef/internal/config"
	appdb "clawreef/internal/db"
	"clawreef/internal/repository"
	upper "github.com/upper/db/v4"
)

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
