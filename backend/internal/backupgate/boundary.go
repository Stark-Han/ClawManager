package backupgate

import (
	"context"
	"database/sql"
	"errors"
	"sync"
)

// HoldRequest fences the complete synchronous handler, including non-SQL
// side effects. Its transaction deliberately outlives request cancellation:
// cancellation is not proof that a handler ignoring its context has stopped.
// Release must be deferred until that handler actually returns. Detached work,
// reverse proxies and background workers need separate registered participants.
func HoldRequest(acquire context.Context, pool *sql.DB, scope string) (func(), error) {
	if pool == nil || !scopePattern.MatchString(scope) {
		return nil, errors.New("backup boundary unavailable")
	}
	conn, e := pool.Conn(acquire)
	if e != nil {
		return nil, e
	}
	tx, e := conn.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if e != nil {
		conn.Close()
		return nil, e
	}
	var once sync.Once
	release := func() { once.Do(func() { _ = tx.Rollback(); _ = conn.Close() }) }
	var phase string
	if e = tx.QueryRowContext(acquire, "SELECT phase FROM system_backup_gate WHERE scope=? LOCK IN SHARE MODE", scope).Scan(&phase); e != nil {
		release()
		return nil, e
	}
	if phase != "idle" {
		release()
		return nil, ErrMaintenance
	}
	return release, nil
}

// Application SQL and request fencing must observe the same physical gate.
// A second database with an identically named scope is not equivalent.
func RequireSameDatabase(ctx context.Context, business, control *sql.DB) error {
	if business == nil || control == nil {
		return errors.New("backup gate identity unavailable")
	}
	var aDB, aServer, bDB, bServer string
	if e := business.QueryRowContext(ctx, "SELECT DATABASE(),@@server_uuid").Scan(&aDB, &aServer); e != nil {
		return e
	}
	if e := control.QueryRowContext(ctx, "SELECT DATABASE(),@@server_uuid").Scan(&bDB, &bServer); e != nil {
		return e
	}
	if aDB == "" || aDB != bDB || aServer == "" || aServer != bServer {
		return errors.New("backup gate database identity mismatch")
	}
	return nil
}
