package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"clawreef/internal/models"
	"github.com/upper/db/v4"
)

type NorthboundRepository struct{ sess db.Session }

func NewNorthboundRepository(sess db.Session) *NorthboundRepository {
	return &NorthboundRepository{sess: sess}
}

func (r *NorthboundRepository) GetAdminSettings() (*models.NorthboundAdminSettings, error) {
	var item models.NorthboundAdminSettings
	if err := r.sess.Collection(item.TableName()).Find(db.Cond{"id": 1}).One(&item); err != nil {
		return nil, fmt.Errorf("failed to get northbound admin settings: %w", err)
	}
	if err := json.Unmarshal([]byte(item.AllowedLiteTypesJSON), &item.AllowedLiteTypes); err != nil {
		return nil, fmt.Errorf("failed to decode allowed Lite runtimes: %w", err)
	}
	if err := json.Unmarshal([]byte(item.AllowedProTypesJSON), &item.AllowedProTypes); err != nil {
		return nil, fmt.Errorf("failed to decode allowed Pro runtimes: %w", err)
	}
	return &item, nil
}

func (r *NorthboundRepository) UpdateAdminSettings(ctx context.Context, item *models.NorthboundAdminSettings, expectedVersion int64, actorUserID int) (bool, error) {
	liteJSON, err := json.Marshal(item.AllowedLiteTypes)
	if err != nil {
		return false, err
	}
	proJSON, err := json.Marshal(item.AllowedProTypes)
	if err != nil {
		return false, err
	}
	now := time.Now().UTC()
	result, err := r.sess.SQL().ExecContext(ctx, `UPDATE northbound_admin_settings SET
		api_enabled=?, external_node_port=?, require_explicit_callers=?, challenge_ttl_seconds=?,
		access_token_ttl_seconds=?, refresh_token_ttl_seconds=?, core_request_timeout_seconds=?,
		challenge_rate_per_minute=?, login_rate_per_minute=?, account_login_rate_per_minute=?,
		create_rate_per_minute=?, query_rate_per_minute=?, share_rate_per_minute=?, max_pending_operations=?,
		operation_tick_milliseconds=?, operation_lease_seconds=?, operation_max_attempts=?,
		allowed_lite_types=?, allowed_pro_types=?, lite_cpu_cores=?, lite_memory_gb=?, lite_disk_gb=?,
		pro_cpu_cores=?, pro_memory_gb=?, pro_disk_gb=?, workbuddy_pro_cpu_cores=?,
		workbuddy_pro_memory_gb=?, workbuddy_pro_disk_gb=?, version=version+1, updated_by=?, updated_at=?
		WHERE id=1 AND version=?`,
		item.APIEnabled, item.ExternalNodePort, item.RequireExplicitCallers, item.ChallengeTTLSeconds,
		item.AccessTokenTTLSeconds, item.RefreshTokenTTLSeconds, item.CoreRequestTimeoutSeconds,
		item.ChallengeRatePerMinute, item.LoginRatePerMinute, item.AccountLoginRatePerMinute,
		item.CreateRatePerMinute, item.QueryRatePerMinute, item.ShareRatePerMinute, item.MaxPendingOperations,
		item.OperationTickMilliseconds, item.OperationLeaseSeconds, item.OperationMaxAttempts,
		string(liteJSON), string(proJSON), item.LiteCPUCores, item.LiteMemoryGB, item.LiteDiskGB,
		item.ProCPUCores, item.ProMemoryGB, item.ProDiskGB, item.WorkBuddyProCPUCores,
		item.WorkBuddyProMemoryGB, item.WorkBuddyProDiskGB, actorUserID, now, expectedVersion)
	if err != nil {
		return false, fmt.Errorf("failed to update northbound admin settings: %w", err)
	}
	count, _ := result.RowsAffected()
	return count == 1, nil
}

func (r *NorthboundRepository) RecordSettingsAudit(ctx context.Context, actorUserID int, action string, before, after any) error {
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	_, err := r.sess.SQL().ExecContext(ctx, `INSERT INTO northbound_settings_audit
		(actor_user_id, action, before_json, after_json, created_at) VALUES (?, ?, ?, ?, ?)`,
		actorUserID, action, nullableJSON(beforeJSON), nullableJSON(afterJSON), time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed to record northbound settings audit: %w", err)
	}
	return nil
}

func nullableJSON(value []byte) any {
	if string(value) == "null" {
		return nil
	}
	return string(value)
}

func (r *NorthboundRepository) GetCallerPolicy(userID int) (*models.NorthboundCallerPolicy, error) {
	var item models.NorthboundCallerPolicy
	err := r.sess.Collection(item.TableName()).Find(db.Cond{"user_id": userID}).One(&item)
	if err == db.ErrNoMoreRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get northbound caller policy: %w", err)
	}
	if err := json.Unmarshal([]byte(item.ScopesJSON), &item.Scopes); err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *NorthboundRepository) ListCallerPolicies() ([]models.NorthboundCallerPolicy, error) {
	rows, err := r.sess.SQL().Query(`SELECT p.user_id, u.username, u.email, p.enabled, p.scopes_json,
		p.created_at, p.updated_at, p.updated_by FROM northbound_caller_policies p JOIN users u ON u.id=p.user_id ORDER BY u.username`)
	if err != nil {
		return nil, fmt.Errorf("failed to list northbound caller policies: %w", err)
	}
	defer rows.Close()
	items := []models.NorthboundCallerPolicy{}
	for rows.Next() {
		var item models.NorthboundCallerPolicy
		if err := rows.Scan(&item.UserID, &item.Username, &item.Email, &item.Enabled, &item.ScopesJSON, &item.CreatedAt, &item.UpdatedAt, &item.UpdatedBy); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(item.ScopesJSON), &item.Scopes); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *NorthboundRepository) UpsertCallerPolicy(ctx context.Context, item *models.NorthboundCallerPolicy, actorUserID int) error {
	scopes, err := json.Marshal(item.Scopes)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	_, err = r.sess.SQL().ExecContext(ctx, `INSERT INTO northbound_caller_policies
		(user_id, enabled, scopes_json, created_at, updated_at, updated_by) VALUES (?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE enabled=VALUES(enabled), scopes_json=VALUES(scopes_json),
		updated_at=VALUES(updated_at), updated_by=VALUES(updated_by)`, item.UserID, item.Enabled, string(scopes), now, now, actorUserID)
	if err != nil {
		return fmt.Errorf("failed to save northbound caller policy: %w", err)
	}
	return nil
}

func (r *NorthboundRepository) RevokeActiveSessionsByUser(ctx context.Context, userID int) error {
	now := time.Now().UTC()
	_, err := r.sess.SQL().ExecContext(ctx, `UPDATE northbound_sessions SET status='revoked', revoked_at=?, updated_at=? WHERE user_id=? AND status='active'`, now, now, userID)
	return err
}

func (r *NorthboundRepository) GetOperationalStats() (*models.NorthboundOperationalStats, error) {
	var stats models.NorthboundOperationalStats
	row, err := r.sess.SQL().QueryRow(`SELECT
		(SELECT COUNT(*) FROM northbound_sessions WHERE status='active' AND refresh_expires_at>UTC_TIMESTAMP(6)),
		(SELECT COUNT(*) FROM northbound_operations WHERE status='queued'),
		(SELECT COUNT(*) FROM northbound_operations WHERE status='processing'),
		(SELECT COUNT(*) FROM northbound_operations WHERE status='failed')`)
	if err != nil {
		return nil, fmt.Errorf("failed to query northbound operational stats: %w", err)
	}
	if err := row.Scan(&stats.ActiveSessions, &stats.QueuedOperations, &stats.ProcessingOperations, &stats.FailedOperations); err != nil {
		return nil, fmt.Errorf("failed to get northbound operational stats: %w", err)
	}
	return &stats, nil
}

func (r *NorthboundRepository) ListSettingsAudit(limit int) ([]models.NorthboundSettingsAudit, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.sess.SQL().Query(`SELECT a.id,a.actor_user_id,COALESCE(u.username,''),a.action,a.before_json,a.after_json,a.created_at FROM northbound_settings_audit a LEFT JOIN users u ON u.id=a.actor_user_id ORDER BY a.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []models.NorthboundSettingsAudit{}
	for rows.Next() {
		var item models.NorthboundSettingsAudit
		if err := rows.Scan(&item.ID, &item.ActorUserID, &item.Actor, &item.Action, &item.BeforeJSON, &item.AfterJSON, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *NorthboundRepository) CreateChallenge(item *models.NorthboundAuthChallenge) error {
	ensureTimestamps(&item.CreatedAt, &item.UpdatedAt)
	res, err := r.sess.Collection(item.TableName()).Insert(item)
	if err != nil {
		return fmt.Errorf("failed to create northbound challenge: %w", err)
	}
	if id, ok := res.ID().(int64); ok {
		item.ID = id
	}
	return nil
}

func (r *NorthboundRepository) ClaimChallenge(ctx context.Context, challengeID string, now time.Time) (*models.NorthboundAuthChallenge, error) {
	result, err := r.sess.SQL().ExecContext(ctx, `
		UPDATE northbound_auth_challenges
		SET status = 'processing', updated_at = ?
		WHERE challenge_id = ? AND status = 'issued' AND expires_at > ?
	`, now, challengeID, now)
	if err != nil {
		return nil, fmt.Errorf("failed to claim northbound challenge: %w", err)
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return nil, nil
	}
	var item models.NorthboundAuthChallenge
	if err := r.sess.Collection(item.TableName()).Find(db.Cond{"challenge_id": challengeID}).One(&item); err != nil {
		return nil, fmt.Errorf("failed to load claimed northbound challenge: %w", err)
	}
	return &item, nil
}

func (r *NorthboundRepository) ConsumeChallenge(ctx context.Context, challengeID string, now time.Time) error {
	_, err := r.sess.SQL().ExecContext(ctx, `
		UPDATE northbound_auth_challenges
		SET status = 'consumed', used_at = ?, updated_at = ?
		WHERE challenge_id = ? AND status = 'processing'
	`, now, now, challengeID)
	if err != nil {
		return fmt.Errorf("failed to consume northbound challenge: %w", err)
	}
	return nil
}

func (r *NorthboundRepository) CreateSession(item *models.NorthboundSession) error {
	ensureTimestamps(&item.CreatedAt, &item.UpdatedAt)
	res, err := r.sess.Collection(item.TableName()).Insert(item)
	if err != nil {
		return fmt.Errorf("failed to create northbound session: %w", err)
	}
	if id, ok := res.ID().(int64); ok {
		item.ID = id
	}
	return nil
}

func (r *NorthboundRepository) GetSessionByID(sessionID string) (*models.NorthboundSession, error) {
	var item models.NorthboundSession
	if err := r.sess.Collection(item.TableName()).Find(db.Cond{"session_id": sessionID}).One(&item); err != nil {
		if err == db.ErrNoMoreRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get northbound session: %w", err)
	}
	return &item, nil
}

func (r *NorthboundRepository) GetActiveSessionByRefreshHash(hash string, now time.Time) (*models.NorthboundSession, error) {
	var item models.NorthboundSession
	if err := r.sess.Collection(item.TableName()).Find(db.Cond{
		"refresh_token_hash": hash,
		"status":             "active",
	}).And("refresh_expires_at > ?", now).One(&item); err != nil {
		if err == db.ErrNoMoreRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get northbound refresh session: %w", err)
	}
	return &item, nil
}

func (r *NorthboundRepository) GetActiveSessionByPreviousRefreshHash(hash string) (*models.NorthboundSession, error) {
	var item models.NorthboundSession
	if err := r.sess.Collection(item.TableName()).Find(db.Cond{"status": "active"}).And(
		"previous_refresh_token_hash = ? OR JSON_CONTAINS(refresh_token_history, JSON_QUOTE(?))",
		hash, hash,
	).One(&item); err != nil {
		if err == db.ErrNoMoreRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to detect northbound refresh token replay: %w", err)
	}
	return &item, nil
}

func (r *NorthboundRepository) RotateSession(ctx context.Context, sessionID, oldHash, newHash string, accessExpiry, refreshExpiry, now time.Time) (bool, error) {
	result, err := r.sess.SQL().ExecContext(ctx, `
		UPDATE northbound_sessions
		SET refresh_token_history = JSON_ARRAY_APPEND(refresh_token_history, '$', refresh_token_hash),
		    previous_refresh_token_hash = refresh_token_hash, refresh_token_hash = ?,
		    access_expires_at = ?, refresh_expires_at = ?,
		    last_used_at = ?, updated_at = ?
		WHERE session_id = ? AND refresh_token_hash = ? AND status = 'active' AND refresh_expires_at > ?
	`, newHash, accessExpiry, refreshExpiry, now, now, sessionID, oldHash, now)
	if err != nil {
		return false, fmt.Errorf("failed to rotate northbound session: %w", err)
	}
	count, _ := result.RowsAffected()
	return count == 1, nil
}

func (r *NorthboundRepository) RevokeSession(ctx context.Context, sessionID string, now time.Time) error {
	_, err := r.sess.SQL().ExecContext(ctx, `
		UPDATE northbound_sessions
		SET status = 'revoked', revoked_at = ?, updated_at = ?
		WHERE session_id = ? AND status = 'active'
	`, now, now, sessionID)
	if err != nil {
		return fmt.Errorf("failed to revoke northbound session: %w", err)
	}
	return nil
}

func (r *NorthboundRepository) CreateOperation(item *models.NorthboundOperation) error {
	ensureTimestamps(&item.CreatedAt, &item.UpdatedAt)
	res, err := r.sess.Collection(item.TableName()).Insert(item)
	if err != nil {
		return fmt.Errorf("failed to create northbound operation: %w", err)
	}
	if id, ok := res.ID().(int64); ok {
		item.ID = id
	}
	return nil
}

func (r *NorthboundRepository) GetOperationByID(operationID string) (*models.NorthboundOperation, error) {
	var item models.NorthboundOperation
	if err := r.sess.Collection(item.TableName()).Find(db.Cond{"operation_id": operationID}).One(&item); err != nil {
		if err == db.ErrNoMoreRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get northbound operation: %w", err)
	}
	return &item, nil
}

func (r *NorthboundRepository) GetOperationByIdempotency(userID int, operationType, keyHash string) (*models.NorthboundOperation, error) {
	var item models.NorthboundOperation
	if err := r.sess.Collection(item.TableName()).Find(db.Cond{
		"user_id":              userID,
		"operation_type":       operationType,
		"idempotency_key_hash": keyHash,
	}).One(&item); err != nil {
		if err == db.ErrNoMoreRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get idempotent northbound operation: %w", err)
	}
	return &item, nil
}

func (r *NorthboundRepository) CountPendingOperationsByUser(userID int) (int, error) {
	count, err := r.sess.Collection("northbound_operations").Find(db.Cond{
		"user_id":   userID,
		"status IN": []string{"queued", "processing"},
	}).Count()
	if err != nil {
		return 0, fmt.Errorf("failed to count pending northbound operations: %w", err)
	}
	return int(count), nil
}

func (r *NorthboundRepository) ClaimNextOperation(ctx context.Context, leaseOwner string, now, leaseUntil time.Time) (*models.NorthboundOperation, error) {
	result, err := r.sess.SQL().ExecContext(ctx, `
		UPDATE northbound_operations
		SET status = 'processing', lease_owner = ?, lease_expires_at = ?,
		    attempt_count = attempt_count + 1,
		    started_at = COALESCE(started_at, ?), updated_at = ?
		WHERE id = (
			SELECT id FROM (
				SELECT id FROM northbound_operations
				WHERE (status = 'queued' AND available_at <= ?)
				   OR (status = 'processing' AND lease_expires_at < ?)
				ORDER BY id
				LIMIT 1
			) candidate
		)
	`, leaseOwner, leaseUntil, now, now, now, now)
	if err != nil {
		return nil, fmt.Errorf("failed to claim northbound operation: %w", err)
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return nil, nil
	}
	var item models.NorthboundOperation
	if err := r.sess.Collection(item.TableName()).Find(db.Cond{"lease_owner": leaseOwner, "status": "processing"}).One(&item); err != nil {
		return nil, fmt.Errorf("failed to load claimed northbound operation: %w", err)
	}
	return &item, nil
}

func (r *NorthboundRepository) MarkOperationSucceeded(ctx context.Context, operationID string, instanceID int, now time.Time) error {
	_, err := r.sess.SQL().ExecContext(ctx, `
		UPDATE northbound_operations
		SET status = 'succeeded', instance_id = ?, error_code = NULL, error_message = NULL,
		    lease_owner = NULL, lease_expires_at = NULL, finished_at = ?, updated_at = ?
		WHERE operation_id = ?
	`, instanceID, now, now, operationID)
	if err != nil {
		return fmt.Errorf("failed to complete northbound operation: %w", err)
	}
	return nil
}

func (r *NorthboundRepository) MarkOperationFailed(ctx context.Context, operationID, code, message string, now time.Time) error {
	_, err := r.sess.SQL().ExecContext(ctx, `
		UPDATE northbound_operations
		SET status = 'failed', error_code = ?, error_message = ?, lease_owner = NULL,
		    lease_expires_at = NULL, finished_at = ?, updated_at = ?
		WHERE operation_id = ?
	`, code, message, now, now, operationID)
	if err != nil {
		return fmt.Errorf("failed to fail northbound operation: %w", err)
	}
	return nil
}

func (r *NorthboundRepository) RequeueOperation(ctx context.Context, operationID, code, message string, availableAt, now time.Time) error {
	_, err := r.sess.SQL().ExecContext(ctx, `
		UPDATE northbound_operations
		SET status = 'queued', error_code = ?, error_message = ?, lease_owner = NULL,
		    lease_expires_at = NULL, available_at = ?, updated_at = ?
		WHERE operation_id = ?
	`, code, message, availableAt, now, operationID)
	if err != nil {
		return fmt.Errorf("failed to requeue northbound operation: %w", err)
	}
	return nil
}
