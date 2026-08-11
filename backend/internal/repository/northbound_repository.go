package repository

import (
	"context"
	"fmt"
	"time"

	"clawreef/internal/models"
	"github.com/upper/db/v4"
)

type NorthboundRepository struct{ sess db.Session }

func NewNorthboundRepository(sess db.Session) *NorthboundRepository {
	return &NorthboundRepository{sess: sess}
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
