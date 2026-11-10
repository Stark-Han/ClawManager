package repository

import (
	"clawreef/internal/models"
	"errors"
	"github.com/upper/db/v4"
	"sync"
	"testing"
	"time"
)

func teamIdleFixture(t *testing.T) (db.Session, *TeamLifecycleRepository) {
	t.Helper()
	s := idleStopMySQL(t)
	for _, q := range []string{
		`INSERT INTO teams VALUES(9,'running','northbound')`,
		`UPDATE instances SET workspace_path=CONCAT('/runtime/openclaw/user-7/instance-',id)`,
		`INSERT INTO team_members VALUES(90,9,1,'leader','leader','idle'),(91,9,2,'worker','worker','idle')`,
		`INSERT INTO team_tasks VALUES(100,9,7,'succeeded',UTC_TIMESTAMP()-INTERVAL 3 DAY,NULL)`,
		`UPDATE instances SET started_at=UTC_TIMESTAMP()-INTERVAL 3 DAY WHERE id=2`,
	} {
		if _, e := s.SQL().Exec(q); e != nil {
			t.Fatal(e)
		}
	}
	a := NewInstanceAutoStopRepository(s)
	if e := a.Initialize(t.Context()); e != nil {
		t.Fatal(e)
	}
	r := NewTeamLifecycleRepository(s)
	if e := r.Initialize(t.Context()); e != nil {
		t.Fatal(e)
	}
	for _, q := range []string{`UPDATE instance_model_activity SET monitoring_since=UTC_TIMESTAMP()-INTERVAL 3 DAY`, `UPDATE team_runtime_lifecycle SET observed_since=UTC_TIMESTAMP()-INTERVAL 3 DAY`} {
		if _, e := s.SQL().Exec(q); e != nil {
			t.Fatal(e)
		}
	}
	return s, r
}

func TestTeamIdleMySQLWholeTeamAdmission(t *testing.T) {
	s, r := teamIdleFixture(t)
	ctx := t.Context()
	// Fresh member heartbeats/updated_at are deliberately irrelevant.
	if _, e := s.SQL().Exec(`ALTER TABLE team_members ADD last_seen_at DATETIME(6),ADD updated_at DATETIME(6)`); e != nil {
		t.Fatal(e)
	}
	if _, e := s.SQL().Exec(`UPDATE team_members SET last_seen_at=UTC_TIMESTAMP(),updated_at=UTC_TIMESTAMP()`); e != nil {
		t.Fatal(e)
	}
	a := NewInstanceAutoStopRepository(s)
	ordinary, e := a.Candidates(ctx, time.Now().UTC(), 1, 0, 100)
	if e != nil || len(ordinary) != 0 {
		t.Fatalf("Team member leaked into ordinary scan: %+v %v", ordinary, e)
	}
	op, e := r.Request(ctx, 9, "stopped", "idle_timeout", 1)
	if e != nil {
		t.Fatal(e)
	}
	if op.State != "stopping" || len(op.Members) != 2 {
		t.Fatalf("not a whole Team operation: %+v", op)
	}
	same, e := r.Request(ctx, 9, "stopped", "idle_timeout", 1)
	if e != nil || same.OperationID != op.OperationID {
		t.Fatalf("duplicate request: %+v %v", same, e)
	}
	if _, e = a.BeginModelActivity(ctx, 7, 1, 2, "gw-1-2"); !errors.Is(e, models.ErrInstanceStopping) {
		t.Fatalf("model admitted during stop: %v", e)
	}
	if _, e = a.ClaimStop(ctx, 1, 0, models.InstanceIdleStopOperation{InstanceID: 1, UserID: 7, Generation: 2}); e == nil {
		t.Fatal("single member stop bypassed Team operation")
	}
	if e = r.Result(ctx, op, 1, errors.New("unconfirmed worker stop")); e != nil {
		t.Fatal(e)
	}
	state, e := r.Get(ctx, 9)
	if e != nil || state.State != "recovery_required" || state.Progress != 1 {
		t.Fatalf("partial failure became success: %+v %v", state, e)
	}
}

func TestTeamIdleMySQLBusinessBlockers(t *testing.T) {
	for _, test := range []struct{ name, q string }{
		{"last_user_not_complete", `UPDATE team_tasks SET status='waiting',finished_at=NULL`},
		{"last_user_failed", `UPDATE team_tasks SET status='failed'`},
		{"fresh_business_activity", `UPDATE instance_model_activity SET last_gateway_activity_at=UTC_TIMESTAMP() WHERE instance_id=2`},
		{"unfinished_request", `INSERT INTO instance_model_activity_requests(request_id,instance_id,generation,started_at) VALUES('active',2,1,UTC_TIMESTAMP()-INTERVAL 3 DAY)`},
		{"waiting_review", `INSERT INTO team_work_items VALUES(9,'awaiting_review',NULL)`},
		{"pending_notification", `INSERT INTO team_event_outbox VALUES(9,'pending')`},
		{"unknown_status", `UPDATE team_tasks SET status='unrecognized'`},
		{"new_observation_grace", `UPDATE team_runtime_lifecycle SET observed_since=UTC_TIMESTAMP()`},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, r := teamIdleFixture(t)
			if _, e := s.SQL().Exec(test.q); e != nil {
				t.Fatal(e)
			}
			if _, e := r.Request(t.Context(), 9, "stopped", "idle_timeout", 1); e == nil {
				t.Fatal("unsafe idle stop admitted")
			}
			v, e := r.Get(t.Context(), 9)
			if e != nil || v.State != "running" {
				t.Fatalf("blocked request mutated state: %+v %v", v, e)
			}
		})
	}
}

func TestTeamIdleMySQLConcurrentAdmission(t *testing.T) {
	s, r := teamIdleFixture(t)
	ctx := t.Context()
	var wg sync.WaitGroup
	wg.Add(2)
	errs := make(chan error, 2)
	go func() { defer wg.Done(); _, e := r.Request(ctx, 9, "stopped", "idle_timeout", 1); errs <- e }()
	go func() {
		defer wg.Done()
		e := s.Tx(func(tx db.Session) error {
			if e := RequireTeamRunning(ctx, tx, 9); e != nil {
				return e
			}
			_, e := tx.SQL().Exec(`INSERT INTO team_tasks VALUES(101,9,7,'pending',NULL,NULL)`)
			return e
		})
		errs <- e
	}()
	wg.Wait()
	close(errs)
	success := 0
	for e := range errs {
		if e == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("task and stop must not both be admitted: successes=%d", success)
	}
}

func TestTeamIdleMySQLAdoptsConfirmedColdTeamAndRetriesOriginalStart(t *testing.T) {
	s, r := teamIdleFixture(t)
	for _, q := range []string{`DELETE FROM team_runtime_lifecycle`, `UPDATE instances SET status='stopped' WHERE id IN (1,2)`, `DELETE FROM instance_runtime_bindings`} {
		if _, e := s.SQL().Exec(q); e != nil {
			t.Fatal(e)
		}
	}
	if e := r.Initialize(t.Context()); e != nil {
		t.Fatal(e)
	}
	v, e := r.Get(t.Context(), 9)
	if e != nil || v.State != "stopped" {
		t.Fatalf("cold Team not startable: %+v %v", v, e)
	}
	op, e := r.Request(t.Context(), 9, "running", "manual", 0)
	if e != nil {
		t.Fatal(e)
	}
	if op.State != "starting" || len(op.Members) != 2 {
		t.Fatalf("original member snapshot lost: %+v", op)
	}
	if e = r.Result(t.Context(), op, 1, errors.New("capacity unavailable")); e != nil {
		t.Fatal(e)
	}
	retry, e := r.Request(t.Context(), 9, "running", "manual", 0)
	if e != nil || retry.OperationID != op.OperationID {
		t.Fatalf("retry replaced operation: %+v %v", retry, e)
	}
}
