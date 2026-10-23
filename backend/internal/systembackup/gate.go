package systembackup

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
)

var ErrMaintenance = errors.New("backup maintenance in progress")
var ErrFence = errors.New("backup fencing token lost")

type Gate struct{ DB *sql.DB }
type MutationLease struct {
	ID, Scope  string
	Generation uint64
}
type GateOwnership struct {
	Scope, Owner      string
	Generation, Token uint64
}

func RandomID(prefix string) (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(b[:]), nil
}

func (g Gate) EnsureScope(ctx context.Context, scope string) error {
	if scope == "" || len(scope) > 128 {
		return errors.New("invalid maintenance scope")
	}
	_, err := g.DB.ExecContext(ctx, "INSERT INTO system_backup_gate(scope) VALUES (?) ON DUPLICATE KEY UPDATE scope=VALUES(scope)", scope)
	return err
}

func (g Gate) AcquireMutation(ctx context.Context, scope string, ttl time.Duration) (MutationLease, error) {
	if ttl < time.Second || ttl > 15*time.Minute {
		return MutationLease{}, errors.New("invalid mutation lease TTL")
	}
	id, err := RandomID("sml_")
	if err != nil {
		return MutationLease{}, err
	}
	tx, err := g.DB.BeginTx(ctx, nil)
	if err != nil {
		return MutationLease{}, err
	}
	defer tx.Rollback()
	var phase string
	var generation uint64
	if err = tx.QueryRowContext(ctx, "SELECT phase,generation FROM system_backup_gate WHERE scope=? FOR UPDATE", scope).Scan(&phase, &generation); err != nil {
		return MutationLease{}, err
	}
	if phase != "idle" {
		return MutationLease{}, ErrMaintenance
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO system_backup_mutation_leases(public_id,scope,generation,expires_at) VALUES (?,?,?,TIMESTAMPADD(MICROSECOND,?,UTC_TIMESTAMP(6)))", id, scope, generation, ttl.Microseconds())
	if err != nil {
		return MutationLease{}, err
	}
	if err = tx.Commit(); err != nil {
		return MutationLease{}, err
	}
	return MutationLease{id, scope, generation}, nil
}

func (g Gate) ReleaseMutation(ctx context.Context, lease MutationLease) error {
	// Expired leases remain blockers until the writer explicitly confirms stop.
	_, err := g.DB.ExecContext(ctx, "UPDATE system_backup_mutation_leases SET released_at=COALESCE(released_at,UTC_TIMESTAMP(6)) WHERE public_id=? AND scope=? AND generation=?", lease.ID, lease.Scope, lease.Generation)
	return err
}

// WriteWithLease holds the gate row before business rows. It verifies ownership
// and expiry in the transaction which performs the business write. External
// filesystem/object writers must use registered participant pause instead.
func (g Gate) WriteWithLease(ctx context.Context, lease MutationLease, write func(*sql.Tx) error) error {
	tx, err := g.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var phase string
	var generation uint64
	if err = tx.QueryRowContext(ctx, "SELECT phase,generation FROM system_backup_gate WHERE scope=? FOR UPDATE", lease.Scope).Scan(&phase, &generation); err != nil {
		return err
	}
	// An existing writer may finish while acquiring; only its original lease
	// generation (immediately before gate generation) is accepted then.
	if !((phase == "idle" && generation == lease.Generation) || (phase == "acquiring" && generation == lease.Generation+1)) {
		return ErrFence
	}
	var valid bool
	if err = tx.QueryRowContext(ctx, "SELECT expires_at>UTC_TIMESTAMP(6) AND released_at IS NULL FROM system_backup_mutation_leases WHERE public_id=? AND scope=? AND generation=? FOR UPDATE", lease.ID, lease.Scope, lease.Generation).Scan(&valid); err != nil {
		return err
	}
	if !valid {
		return ErrFence
	}
	if err = write(tx); err != nil {
		return err
	}
	if err = tx.QueryRowContext(ctx, "SELECT expires_at>UTC_TIMESTAMP(6) AND released_at IS NULL FROM system_backup_mutation_leases WHERE public_id=?", lease.ID).Scan(&valid); err != nil {
		return err
	}
	if !valid {
		return ErrFence
	}
	return tx.Commit()
}

func (g Gate) Begin(ctx context.Context, scope, owner, registryHash string, acquire, hold time.Duration) (GateOwnership, error) {
	if owner == "" || len(owner) > 128 || len(registryHash) != 64 || acquire <= 0 || hold <= acquire || hold > 15*time.Minute {
		return GateOwnership{}, errors.New("invalid gate acquisition")
	}
	tx, err := g.DB.BeginTx(ctx, nil)
	if err != nil {
		return GateOwnership{}, err
	}
	defer tx.Rollback()
	var phase string
	var generation, token uint64
	// Never queue an exclusive lock behind the complete-request shared lock.
	// Its admitted handler can still need another SQL transaction on this row;
	// queuing FOR UPDATE would block that SQL and prevent the handler draining.
	// The controller retries a busy gate within its absolute acquisition budget.
	if err = tx.QueryRowContext(ctx, "SELECT phase,generation,fencing_token FROM system_backup_gate WHERE scope=? FOR UPDATE NOWAIT", scope).Scan(&phase, &generation, &token); err != nil {
		var busy *mysql.MySQLError
		if errors.As(err, &busy) && busy.Number == 3572 {
			return GateOwnership{}, ErrMaintenance
		}
		return GateOwnership{}, err
	}
	if phase != "idle" {
		return GateOwnership{}, ErrMaintenance
	}
	var outstanding int
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM system_backup_participants WHERE scope=? AND (state<>'running' OR resume_result NOT IN ('none','succeeded','forced'))", scope).Scan(&outstanding); err != nil {
		return GateOwnership{}, err
	}
	if outstanding != 0 {
		return GateOwnership{}, ErrMaintenance
	}
	_, err = tx.ExecContext(ctx, "UPDATE system_backup_gate SET phase='acquiring',generation=generation+1,owner=?,registry_hash=?,acquire_deadline=TIMESTAMPADD(MICROSECOND,?,UTC_TIMESTAMP(6)),hold_deadline=TIMESTAMPADD(MICROSECOND,?,UTC_TIMESTAMP(6)),updated_at=UTC_TIMESTAMP(6) WHERE scope=?", owner, registryHash, acquire.Microseconds(), hold.Microseconds(), scope)
	if err != nil {
		return GateOwnership{}, err
	}
	_, err = tx.ExecContext(ctx, "UPDATE system_backup_participants SET generation=?,state='pausing',resume_result='none',checkpoint_hash=NULL,updated_at=UTC_TIMESTAMP(6) WHERE scope=?", generation+1, scope)
	if err != nil {
		return GateOwnership{}, err
	}
	if err = tx.Commit(); err != nil {
		return GateOwnership{}, err
	}
	return GateOwnership{scope, owner, generation + 1, token}, nil
}

// Hold requires every registered writer to have stopped, including expired
// mutation leases. A TTL expiration alone never establishes consistency.
func (g Gate) Hold(ctx context.Context, ownership GateOwnership) (GateOwnership, error) {
	tx, err := g.DB.BeginTx(ctx, nil)
	if err != nil {
		return ownership, err
	}
	defer tx.Rollback()
	var phase, owner string
	var generation, token uint64
	var timely bool
	if err = tx.QueryRowContext(ctx, "SELECT phase,COALESCE(owner,''),generation,fencing_token,UTC_TIMESTAMP(6)<acquire_deadline AND UTC_TIMESTAMP(6)<hold_deadline FROM system_backup_gate WHERE scope=? FOR UPDATE", ownership.Scope).Scan(&phase, &owner, &generation, &token, &timely); err != nil {
		return ownership, err
	}
	if phase != "acquiring" || owner != ownership.Owner || generation != ownership.Generation || !timely {
		return ownership, ErrFence
	}
	var active int
	if err = tx.QueryRowContext(ctx, "SELECT (SELECT COUNT(*) FROM system_backup_mutation_leases WHERE scope=? AND released_at IS NULL) + (SELECT COUNT(*) FROM system_backup_participants WHERE scope=? AND (generation<>? OR state<>'paused' OR resume_result<>'none'))", ownership.Scope, ownership.Scope, generation).Scan(&active); err != nil {
		return ownership, err
	}
	if active != 0 {
		return ownership, ErrMaintenance
	}
	_, err = tx.ExecContext(ctx, "UPDATE system_backup_gate SET phase='held',fencing_token=fencing_token+1,updated_at=UTC_TIMESTAMP(6) WHERE scope=?", ownership.Scope)
	if err != nil {
		return ownership, err
	}
	if err = tx.Commit(); err != nil {
		return ownership, err
	}
	ownership.Token = token + 1
	return ownership, nil
}

func (g Gate) BeginRelease(ctx context.Context, ownership GateOwnership) error {
	r, err := g.DB.ExecContext(ctx, "UPDATE system_backup_gate SET phase='releasing',updated_at=UTC_TIMESTAMP(6) WHERE scope=? AND owner=? AND generation=? AND phase IN ('acquiring','held','releasing')", ownership.Scope, ownership.Owner, ownership.Generation)
	return requireRow(r, err)
}
func (g Gate) FinishRelease(ctx context.Context, ownership GateOwnership) error {
	tx, err := g.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var phase, owner string
	var generation uint64
	if err = tx.QueryRowContext(ctx, "SELECT phase,COALESCE(owner,''),generation FROM system_backup_gate WHERE scope=? FOR UPDATE", ownership.Scope).Scan(&phase, &owner, &generation); err != nil {
		return err
	}
	if phase != "releasing" || owner != ownership.Owner || generation != ownership.Generation {
		return ErrFence
	}
	var active int
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM system_backup_participants WHERE scope=? AND (state<>'running' OR resume_result NOT IN ('succeeded','forced'))", ownership.Scope).Scan(&active); err != nil {
		return err
	}
	if active != 0 {
		return ErrMaintenance
	}
	_, err = tx.ExecContext(ctx, "UPDATE system_backup_gate SET phase='idle',owner=NULL,acquire_deadline=NULL,hold_deadline=NULL,updated_at=UTC_TIMESTAMP(6) WHERE scope=?", ownership.Scope)
	if err != nil {
		return err
	}
	return tx.Commit()
}
func requireRow(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrFence
	}
	return nil
}

func (g Gate) RegisterParticipant(ctx context.Context, scope, id string) error {
	if id == "" || len(id) > 128 {
		return errors.New("invalid participant identity")
	}
	tx, err := g.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var phase string
	var generation uint64
	if err = tx.QueryRowContext(ctx, "SELECT phase,generation FROM system_backup_gate WHERE scope=? FOR UPDATE", scope).Scan(&phase, &generation); err != nil {
		return err
	}
	if phase != "idle" {
		return ErrMaintenance
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO system_backup_participants(scope,participant_id,generation,state) VALUES (?,?,?,'running') ON DUPLICATE KEY UPDATE participant_id=VALUES(participant_id)", scope, id, generation)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (g Gate) AckPaused(ctx context.Context, ownership GateOwnership, id, checkpointHash string) error {
	if len(checkpointHash) != 64 {
		return errors.New("participant checkpoint evidence required")
	}
	tx, err := g.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var phase, owner string
	var generation uint64
	var timely bool
	if err = tx.QueryRowContext(ctx, "SELECT phase,COALESCE(owner,''),generation,UTC_TIMESTAMP(6)<acquire_deadline FROM system_backup_gate WHERE scope=? FOR UPDATE", ownership.Scope).Scan(&phase, &owner, &generation, &timely); err != nil {
		return err
	}
	if phase != "acquiring" || owner != ownership.Owner || generation != ownership.Generation || !timely {
		return ErrFence
	}
	r, err := tx.ExecContext(ctx, "UPDATE system_backup_participants SET state='paused',checkpoint_hash=?,updated_at=UTC_TIMESTAMP(6) WHERE scope=? AND participant_id=? AND generation=? AND state IN ('pausing','paused') AND resume_result='none'", checkpointHash, ownership.Scope, id, generation)
	if err = requireRow(r, err); err != nil {
		return err
	}
	return tx.Commit()
}

func (g Gate) AckResumed(ctx context.Context, ownership GateOwnership, id string, forced bool) error {
	result := "succeeded"
	if forced {
		result = "forced"
	}
	tx, err := g.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var phase, owner string
	var generation uint64
	var expired bool
	if err = tx.QueryRowContext(ctx, "SELECT phase,COALESCE(owner,''),generation,COALESCE(UTC_TIMESTAMP(6)>=hold_deadline,0) FROM system_backup_gate WHERE scope=? FOR UPDATE", ownership.Scope).Scan(&phase, &owner, &generation, &expired); err != nil {
		return err
	}
	if owner != ownership.Owner || generation != ownership.Generation || (!forced && phase != "releasing") || (forced && !expired) {
		return ErrFence
	}
	r, err := tx.ExecContext(ctx, "UPDATE system_backup_participants SET state='running',resume_result=?,updated_at=UTC_TIMESTAMP(6) WHERE scope=? AND participant_id=? AND generation=?", result, ownership.Scope, id, generation)
	if err = requireRow(r, err); err != nil {
		return err
	}
	if forced {
		if _, err = tx.ExecContext(ctx, "UPDATE system_backup_gate SET phase='releasing',updated_at=UTC_TIMESTAMP(6) WHERE scope=?", ownership.Scope); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (g Gate) ValidateCapture(ctx context.Context, ownership GateOwnership) error {
	var valid bool
	err := g.DB.QueryRowContext(ctx, "SELECT phase='held' AND owner=? AND generation=? AND fencing_token=? AND UTC_TIMESTAMP(6)<hold_deadline AND NOT EXISTS (SELECT 1 FROM system_backup_participants p WHERE p.scope=g.scope AND (p.generation<>g.generation OR p.state<>'paused' OR p.resume_result<>'none')) FROM system_backup_gate g WHERE scope=?", ownership.Owner, ownership.Generation, ownership.Token, ownership.Scope).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return ErrFence
	}
	return nil
}
