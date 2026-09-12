package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Keep quarantine and outbox publication atomic, and let newer credentials win.
func (r *accountRepository) SetOpenAIOAuthErrorIfCredentialsUnchanged(ctx context.Context, id int64, expectedCredentials map[string]any, errorMsg string) (bool, error) {
	if r == nil || r.sql == nil {
		return false, errors.New("account repository SQL executor is not configured")
	}
	expectedJSON, err := json.Marshal(normalizeJSONMap(expectedCredentials))
	if err != nil {
		return false, err
	}
	before := &service.Account{Credentials: expectedCredentials}
	attributed := errorMsg == "OpenAI OAuth refresh credential rejected; reauthorize the account" && before.GetCredentialAsInt64("_token_version") > 0 && before.GetCredential("access_token") != "" && before.GetCredential("refresh_token") != ""
	result, err := r.sql.ExecContext(ctx, `
		WITH updated AS (
		UPDATE accounts AS a
		SET status = $1, error_message = $2, schedulable = FALSE, updated_at = NOW()
		WHERE a.id = $3
			AND a.deleted_at IS NULL
			AND a.platform = $4
			AND a.type = $5
			AND a.status = $6
			AND a.credentials = $7::jsonb
		RETURNING a.id
        ), evidence AS (
            INSERT INTO oauth_auth_errors (account_id, kind, credential_version, token_hash, message)
            SELECT id, 'error', $10, $11, $2 FROM updated WHERE $9
            ON CONFLICT (account_id, kind) DO UPDATE SET credential_version=EXCLUDED.credential_version,
                token_hash=EXCLUDED.token_hash, message=EXCLUDED.message, blocked_until=NULL, observed_at=clock_timestamp()
		)
		INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)
		SELECT $8, updated.id, NULL, NULL FROM updated
	`, service.StatusError, errorMsg, id, service.PlatformOpenAI, service.AccountTypeOAuth,
		service.StatusActive, string(expectedJSON), service.SchedulerOutboxEventAccountChanged,
		attributed, before.GetCredentialAsInt64("_token_version"), service.OAuthAccessTokenHash(before.GetCredential("access_token")))
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil || affected == 0 {
		return false, err
	}
	r.syncSchedulerAccountSnapshotDetached(ctx, id)
	return true, nil
}

var _ service.OpenAIOAuthConditionalErrorRepository = (*accountRepository)(nil)
