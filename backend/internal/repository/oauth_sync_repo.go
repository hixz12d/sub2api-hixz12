package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type oauthSyncRepository struct{ db *sql.DB }

func NewOAuthSyncRepository(db *sql.DB) service.OAuthSyncRepository {
	return &oauthSyncRepository{db: db}
}

const oauthSyncColumns = "id, scope, operation_id, request_hash, account_id, credential_version, cache_done, scheduler_done, state, attempts, COALESCE(lease_id::text, ''), last_error, created_at, updated_at, next_attempt_at, auth_recovery, validation_scope, validated_at"

type oauthSyncScanner interface{ Scan(...any) error }

func scanOAuthSync(row oauthSyncScanner) (*service.OAuthSyncOperation, error) {
	op := &service.OAuthSyncOperation{}
	err := row.Scan(&op.ID, &op.Scope, &op.OperationID, &op.RequestHash, &op.AccountID, &op.CredentialVersion, &op.CacheDone, &op.SchedulerDone, &op.State, &op.Attempts, &op.LeaseID, &op.LastError, &op.CreatedAt, &op.UpdatedAt, &op.NextAttemptAt, &op.AuthRecovery, &op.ValidationScope, &op.ValidatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return op, nil
}
func (r *oauthSyncRepository) InstanceID(ctx context.Context) (string, error) {
	var id string
	err := r.db.QueryRowContext(ctx, "SELECT instance_id::text FROM integration_identity WHERE singleton = TRUE").Scan(&id)
	return id, err
}
func (r *oauthSyncRepository) Get(ctx context.Context, scope, key string) (*service.OAuthSyncOperation, error) {
	return scanOAuthSync(r.db.QueryRowContext(ctx, "SELECT "+oauthSyncColumns+" FROM oauth_sync_operations WHERE scope = $1 AND operation_id = $2", scope, key))
}
func (r *oauthSyncRepository) Latest(ctx context.Context, scope string, id int64) (*service.OAuthSyncOperation, error) {
	return scanOAuthSync(r.db.QueryRowContext(ctx, "SELECT "+oauthSyncColumns+" FROM oauth_sync_operations WHERE scope = $1 AND account_id = $2 ORDER BY id DESC LIMIT 1", scope, id))
}

// Insert the metadata receipt, compare-and-swap credentials and enqueue the
// scheduler event in ONE transaction. A failed CAS rolls back the new receipt.
func (r *oauthSyncRepository) Commit(ctx context.Context, op *service.OAuthSyncOperation, before *service.Account, credentials map[string]any) (*service.OAuthSyncOperation, bool, error) {
	oldJSON, err := json.Marshal(normalizeJSONMap(before.Credentials))
	if err != nil {
		return nil, false, err
	}
	newJSON, err := json.Marshal(normalizeJSONMap(credentials))
	if err != nil {
		return nil, false, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()
	inserted, err := tx.ExecContext(ctx, `INSERT INTO oauth_sync_operations (scope, operation_id, request_hash, account_id, credential_version, auth_recovery, validation_scope, validated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT (scope, operation_id) DO NOTHING`, op.Scope, op.OperationID, op.RequestHash, op.AccountID, op.CredentialVersion, defaultRecovery(op.AuthRecovery), op.ValidationScope, op.ValidatedAt)
	if err != nil {
		return nil, false, err
	}
	count, err := inserted.RowsAffected()
	if err != nil {
		return nil, false, err
	}
	saved, err := scanOAuthSync(tx.QueryRowContext(ctx, "SELECT "+oauthSyncColumns+" FROM oauth_sync_operations WHERE scope = $1 AND operation_id = $2 FOR UPDATE", op.Scope, op.OperationID))
	if err != nil {
		return nil, false, err
	}
	if saved == nil {
		return nil, false, service.ErrIdempotencyStoreUnavail
	}
	if saved.RequestHash != op.RequestHash || saved.AccountID != op.AccountID {
		return nil, false, service.ErrIdempotencyKeyConflict
	}
	if count == 0 {
		return saved, true, tx.Commit()
	}
	updated, err := tx.ExecContext(ctx, `UPDATE accounts SET credentials = $1::jsonb,
        updated_at = GREATEST(clock_timestamp(), updated_at + INTERVAL '1 microsecond')
        WHERE id = $2 AND deleted_at IS NULL AND platform = $3 AND type = $4
        AND updated_at = $5 AND credentials = $6::jsonb`, string(newJSON), op.AccountID, service.PlatformOpenAI, service.AccountTypeOAuth, before.UpdatedAt, string(oldJSON))
	if err != nil {
		return nil, false, err
	}
	count, err = updated.RowsAffected()
	if err != nil {
		return nil, false, err
	}
	if count != 1 {
		return nil, false, service.ErrOAuthSyncConflict
	}
	if op.AuthRecovery == "cleared" {
		if err := recoverVersionedOAuthError(ctx, tx, op, before, credentials); err != nil {
			return nil, false, err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO scheduler_outbox (event_type, account_id, payload) VALUES ($1, $2, NULL)`, service.SchedulerOutboxEventAccountChanged, op.AccountID)
	if err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return saved, false, nil
}

func (r *oauthSyncRepository) Claim(ctx context.Context) (*service.OAuthSyncOperation, error) {
	// This single statement owns a short row lock, then a persistent fenced lease.
	// No database transaction remains open during Redis calls.
	return scanOAuthSync(r.db.QueryRowContext(ctx, `WITH candidate AS (
        SELECT id FROM oauth_sync_operations WHERE state = 'pending'
        AND next_attempt_at <= clock_timestamp() AND (lease_until IS NULL OR lease_until < clock_timestamp())
        ORDER BY next_attempt_at, id FOR UPDATE SKIP LOCKED LIMIT 1
    ), claimed AS (
        UPDATE oauth_sync_operations o SET lease_id = gen_random_uuid(), lease_until = clock_timestamp() + INTERVAL '60 seconds',
        attempts = attempts + 1, updated_at = clock_timestamp()
        FROM candidate c WHERE o.id = c.id RETURNING o.*
    ) SELECT `+oauthSyncColumns+` FROM claimed`))
}
func (r *oauthSyncRepository) Progress(ctx context.Context, op *service.OAuthSyncOperation, cacheDone, schedulerDone bool, code string, release, review bool) error {
	if op.LeaseID == "" {
		return errors.New("oauth sync lease missing")
	}
	exponent := op.Attempts
	if exponent > 9 {
		exponent = 9
	}
	if exponent < 0 {
		exponent = 0
	}
	delay := 5 * (1 << exponent)
	result, err := r.db.ExecContext(ctx, `UPDATE oauth_sync_operations
        SET cache_done = cache_done OR $3, scheduler_done = scheduler_done OR $4,
        state = CASE WHEN $7 THEN 'needs_review' WHEN (cache_done OR $3) AND (scheduler_done OR $4) THEN 'completed' ELSE 'pending' END,
        last_error = $5, updated_at = clock_timestamp(),
        next_attempt_at = CASE WHEN $6 THEN clock_timestamp() + ($8 * INTERVAL '1 second') ELSE next_attempt_at END,
        lease_id = CASE WHEN $6 THEN NULL ELSE lease_id END,
        lease_until = CASE WHEN $6 THEN NULL ELSE lease_until END
        WHERE id = $1 AND lease_id = $2::uuid AND lease_until > clock_timestamp() AND state = 'pending'`,
		op.ID, op.LeaseID, cacheDone, schedulerDone, code, release, review, delay)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errors.New("oauth sync lease lost")
	}
	return nil
}
func (r *oauthSyncRepository) Retry(ctx context.Context, scope, key string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE oauth_sync_operations SET next_attempt_at = clock_timestamp()
        WHERE scope = $1 AND operation_id = $2 AND state = 'pending'
        AND (lease_until IS NULL OR lease_until < clock_timestamp())
        AND updated_at < clock_timestamp() - INTERVAL '2 seconds'`, scope, key)
	return err
}
