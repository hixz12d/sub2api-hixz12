package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Credential CAS and durable scheduler invalidation share a PostgreSQL statement.
// No account configuration or runtime blocker is changed by this operation.
func (r *accountRepository) UpdateOAuthCredentialsIfUnchanged(ctx context.Context, id int64, expectedAt time.Time, expected, credentials map[string]any) (bool, error) {
	if r == nil || r.sql == nil {
		return false, errors.New("credential storage is unavailable")
	}
	expectedJSON, err := json.Marshal(normalizeJSONMap(expected))
	if err != nil {
		return false, err
	}
	credentialsJSON, err := json.Marshal(normalizeJSONMap(credentials))
	if err != nil {
		return false, err
	}
	result, err := r.sql.ExecContext(ctx, `
		WITH updated AS (
			UPDATE accounts AS a
			SET credentials = $1::jsonb,
				updated_at = GREATEST(clock_timestamp(), a.updated_at + INTERVAL '1 microsecond')
			WHERE a.id = $2 AND a.deleted_at IS NULL
				AND a.platform = $3 AND a.type = $4
				AND a.updated_at = $5 AND a.credentials = $6::jsonb
			RETURNING a.id
		)
		INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)
		SELECT $7, updated.id, NULL, NULL FROM updated
	`, string(credentialsJSON), id, service.PlatformOpenAI, service.AccountTypeOAuth,
		expectedAt, string(expectedJSON), service.SchedulerOutboxEventAccountChanged)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	if err != nil || count == 0 {
		return false, err
	}
	r.syncSchedulerAccountSnapshotDetached(ctx, id)
	return true, nil
}
