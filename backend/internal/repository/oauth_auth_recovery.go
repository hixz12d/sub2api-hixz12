package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func defaultRecovery(value string) string {
	if value == "" {
		return "skipped"
	}
	return value
}

// RecordOAuthUnauthorized never upgrades a stale request or a manual/permission
// error into recoverable auth evidence. Account mutation and evidence are atomic.
func (r *accountRepository) RecordOAuthUnauthorized(ctx context.Context, before *service.Account, token string, permanent bool, until time.Time) error {
	if before == nil || token == "" || token != before.GetCredential("access_token") {
		return nil
	}
	creds, err := json.Marshal(normalizeJSONMap(before.Credentials))
	if err != nil {
		return err
	}
	kind := "temporary"
	if permanent {
		kind = "error"
	}
	message := "OAuth authentication failed (versioned 401)"
	if before.GetCredentialAsInt64("_token_version") <= 0 {
		// Legacy imports still need durable quarantine. Without a version, do not
		// create evidence that a later credential sync could automatically clear.
		message = "OAuth authentication failed (401; unversioned credential)"
	}
	_, err = r.sql.ExecContext(ctx, `WITH changed AS (
        UPDATE accounts SET
            status = CASE WHEN $4 THEN 'error' ELSE status END,
            error_message = CASE WHEN $4 THEN $5 ELSE error_message END,
            temp_unschedulable_until = CASE WHEN $4 THEN temp_unschedulable_until ELSE $6 END,
            temp_unschedulable_reason = CASE WHEN $4 THEN temp_unschedulable_reason ELSE $5 END,
            updated_at = GREATEST(clock_timestamp(), updated_at + INTERVAL '1 microsecond')
        WHERE id = $1 AND deleted_at IS NULL AND platform = 'openai' AND type = 'oauth'
            AND status = 'active' AND COALESCE(error_message, '') = '' AND credentials = $2::jsonb
            AND ($4 OR temp_unschedulable_until IS NULL OR temp_unschedulable_until <= clock_timestamp()
                OR EXISTS (SELECT 1 FROM oauth_auth_errors e WHERE e.account_id = accounts.id AND e.kind = 'temporary'
                    AND e.message = temp_unschedulable_reason AND e.blocked_until = temp_unschedulable_until))
        RETURNING id
    ), evidence AS (
        INSERT INTO oauth_auth_errors (account_id, kind, credential_version, token_hash, message, blocked_until)
        SELECT id, $3, $7, $8, $5, CASE WHEN $4 THEN NULL ELSE $6 END FROM changed WHERE $7::bigint > 0
        ON CONFLICT (account_id, kind) DO UPDATE SET credential_version = EXCLUDED.credential_version,
            token_hash = EXCLUDED.token_hash, message = EXCLUDED.message, blocked_until = EXCLUDED.blocked_until, observed_at = clock_timestamp()
        RETURNING account_id
    ) INSERT INTO scheduler_outbox (event_type, account_id, payload)
        SELECT $9, id, NULL FROM changed`, before.ID, string(creds), kind, permanent, message, until,
		before.GetCredentialAsInt64("_token_version"), service.OAuthAccessTokenHash(token), service.SchedulerOutboxEventAccountChanged)
	return err
}

func recoverVersionedOAuthError(ctx context.Context, tx *sql.Tx, op *service.OAuthSyncOperation, before *service.Account, credentials map[string]any) error {
	if op.ValidatedAt == nil || op.ValidationScope != service.OAuthValidationScope || time.Since(*op.ValidatedAt) > 30*time.Second || op.ValidatedAt.After(time.Now()) {
		return service.ErrOAuthValidationFailed
	}
	token, _ := credentials["access_token"].(string)
	// Exact evidence fields must still own the field being cleared. No general
	// status reset, schedulable write, cooldown reset or quota mutation occurs.
	var recovered int
	err := tx.QueryRowContext(ctx, `WITH eligible AS (
        SELECT e.* FROM oauth_auth_errors e JOIN accounts a ON a.id = e.account_id
        WHERE e.account_id = $1 AND e.credential_version <= $2 AND e.credential_version < $3 AND e.token_hash <> $4
        AND ((e.kind = 'error' AND a.status = 'error' AND a.error_message = e.message)
          OR (e.kind = 'temporary' AND a.temp_unschedulable_until = e.blocked_until AND a.temp_unschedulable_reason = e.message))
        FOR UPDATE OF e
    ), cleared AS (
        UPDATE accounts SET
            status = CASE WHEN EXISTS (SELECT 1 FROM eligible WHERE kind = 'error') THEN 'active' ELSE status END,
            error_message = CASE WHEN EXISTS (SELECT 1 FROM eligible WHERE kind = 'error') THEN NULL ELSE error_message END,
            temp_unschedulable_until = CASE WHEN EXISTS (SELECT 1 FROM eligible WHERE kind = 'temporary') THEN NULL ELSE temp_unschedulable_until END,
            temp_unschedulable_reason = CASE WHEN EXISTS (SELECT 1 FROM eligible WHERE kind = 'temporary') THEN NULL ELSE temp_unschedulable_reason END
        WHERE id = $1 AND EXISTS (SELECT 1 FROM eligible) RETURNING id
    ), removed AS (
        DELETE FROM oauth_auth_errors WHERE account_id = $1 AND kind IN (SELECT kind FROM eligible)
            AND EXISTS (SELECT 1 FROM cleared) RETURNING kind
    ) SELECT count(*) FROM removed`, before.ID, before.GetCredentialAsInt64("_token_version"), op.CredentialVersion, service.OAuthAccessTokenHash(token)).Scan(&recovered)
	if err != nil {
		return err
	}
	if recovered == 0 {
		return service.ErrOAuthAuthUnattributed
	}
	return nil
}
