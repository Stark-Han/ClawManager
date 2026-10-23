package systembackup

import (
	"bytes"
	"context"
	"testing"
	"time"
)

func TestSQLTimeCellPreservesDeclaredPrecision(t *testing.T) {
	at := time.Date(2026, 10, 23, 1, 2, 3, 123400000, time.UTC)
	for _, tc := range []struct{ kind, want string }{{"datetime(6)", "2026-10-23 01:02:03.123400"}, {"timestamp(3)", "2026-10-23 01:02:03.123"}, {"datetime", "2026-10-23 01:02:03"}, {"date", "2026-10-23"}} {
		c, e := sqlCell(at, SQLColumn{Type: tc.kind})
		if e != nil || string(c.Data) != tc.want {
			t.Fatalf("%s: %s %v", tc.kind, c.Data, e)
		}
	}
}

func TestReadOnlyVerifierDetectsValueLossExtraRowsAndBudget(t *testing.T) {
	db := controlDB(t)
	ctx := context.Background()
	_, err := db.Exec(`CREATE TABLE team_events (id BIGINT PRIMARY KEY, payload LONGBLOB NULL, created_at DATETIME(6) NOT NULL) ENGINE=InnoDB`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("INSERT INTO team_events VALUES(1,?,'2026-10-23 01:02:03.123400')", []byte{0, 255, 10})
	if err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	if _, err = ExportMySQL(ctx, db, &archive, []string{"team_events"}); err != nil {
		t.Fatal(err)
	}
	if _, err = VerifyMySQL(ctx, db, bytes.NewReader(archive.Bytes()), 10); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("UPDATE team_events SET payload='lost' WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	if _, err = VerifyMySQL(ctx, db, bytes.NewReader(archive.Bytes()), 10); err == nil {
		t.Fatal("missed corruption")
	}
	if _, err = db.Exec("UPDATE team_events SET payload=? WHERE id=1", []byte{0, 255, 10}); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO team_events VALUES(2,NULL,'2026-10-23 01:02:03.123400')"); err != nil {
		t.Fatal(err)
	}
	if _, err = VerifyMySQL(ctx, db, bytes.NewReader(archive.Bytes()), 10); err == nil {
		t.Fatal("missed additional row")
	}
	archive.Reset()
	if _, err = ExportMySQL(ctx, db, &archive, []string{"team_events"}); err != nil {
		t.Fatal(err)
	}
	if _, err = VerifyMySQL(ctx, db, bytes.NewReader(archive.Bytes()), 1); err == nil {
		t.Fatal("ignored budget")
	}
}
