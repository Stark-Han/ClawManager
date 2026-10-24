package systembackup

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Recover invokes the same restore implementation against an explicitly
// configured disaster-side control DB. No source job record is required.
func (s *TeamDataService) Recover(ctx context.Context, origin, id, commit string) (TeamDataJob, error) {
	if !s.recoveryOnly || origin != s.cfg.InstallationID {
		return TeamDataJob{}, errors.New("validation_failed")
	}
	a, e := OpenPublishedArtifact(ctx, s.store, origin, id, s.catalog().PublicKey, s.cfg.KeyVersion)
	if e != nil {
		return TeamDataJob{}, e
	}
	if a.CommitReference.SHA256 != commit {
		return TeamDataJob{}, errors.New("artifact_conflict")
	}
	if e = s.catalog().Import(ctx, a, 1, false); e != nil {
		return TeamDataJob{}, e
	}
	key, e := RandomID("offline_")
	if e != nil {
		return TeamDataJob{}, e
	}
	j, e := s.Create(ctx, 1, key, "restore", id)
	if e != nil {
		return j, e
	}
	_, e = s.db.ExecContext(ctx, "UPDATE system_team_data_backup_jobs SET status='running',owner_id=?,started_at=UTC_TIMESTAMP(6) WHERE public_id=? AND status='pending'", s.owner, j.ID)
	if e != nil {
		return j, e
	}
	r, operationErr := s.restore(ctx, j)
	status, phase, code := "succeeded", "complete", ""
	if operationErr != nil {
		status = "failed"
		actual, _ := s.Get(context.Background(), j.ID)
		phase = actual.Phase
		code = teamDataFailure(operationErr)
	}
	raw, e := json.Marshal(r)
	if e != nil {
		return j, e
	}
	finish, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, e = s.db.ExecContext(finish, "UPDATE system_team_data_backup_jobs SET status=?,phase=?,error_code=?,report_json=?,finished_at=UTC_TIMESTAMP(6) WHERE public_id=?", status, phase, code, raw, j.ID)
	if e != nil {
		return j, e
	}
	j, e = s.Get(finish, j.ID)
	if operationErr != nil {
		return j, operationErr
	}
	return j, e
}
func (s *TeamDataService) CleanupRecovery(ctx context.Context, id string) error {
	if !s.recoveryOnly {
		return errors.New("validation_failed")
	}
	return s.cleanup(ctx, id)
}
