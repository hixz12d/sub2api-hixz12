//go:build integration

package repository

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func durableSyncPostgresFixture(t *testing.T) (*oauthSyncRepository, *service.Account, *service.OAuthSyncOperation) {
	t.Helper()
	ctx := context.Background()
	account := mustCreateAccount(t, integrationEntClient, &service.Account{Name: t.Name(), Platform: service.PlatformOpenAI,
		Credentials: map[string]any{"email": "fixture@example.invalid", "access_token": "fixture-old", "refresh_token": "fixture-old-rt"}})
	_, err := integrationEntClient.Account.UpdateOneID(account.ID).SetSchedulable(false).SetStatus(service.StatusError).
		SetErrorMessage("fixture preserved error").SetRateLimitResetAt(time.Now().Add(time.Hour)).Save(ctx)
	require.NoError(t, err)
	accounts := newAccountRepositoryWithSQL(integrationEntClient, integrationDB, nil)
	before, err := accounts.GetByID(ctx, account.ID)
	require.NoError(t, err)
	op := &service.OAuthSyncOperation{Scope: fmt.Sprintf("fixture-admin:%d", account.ID), OperationID: "fixture-operation", RequestHash: strings.Repeat("a", 64), AccountID: account.ID, CredentialVersion: time.Now().UnixMilli()}
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM oauth_sync_operations WHERE account_id = $1", account.ID)
		_, err := integrationDB.ExecContext(ctx, "DELETE FROM scheduler_outbox WHERE account_id = $1", account.ID)
		require.NoError(t, err)
		_ = integrationEntClient.Account.DeleteOneID(account.ID).Exec(ctx)
	})
	return &oauthSyncRepository{db: integrationDB}, before, op
}
func TestOAuthDurableSyncPostgresConcurrentReplayAndCAS(t *testing.T) {
	repo, before, op := durableSyncPostgresFixture(t)
	ctx := context.Background()
	creds := map[string]any{"access_token": "fixture-new", "_token_version": op.CredentialVersion}
	var wg sync.WaitGroup
	replay := make([]bool, 2)
	errs := make([]error, 2)
	for i := range replay {
		wg.Add(1)
		go func(i int) { defer wg.Done(); _, replay[i], errs[i] = repo.Commit(ctx, op, before, creds) }(i)
	}
	wg.Wait()
	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	require.NotEqual(t, replay[0], replay[1])
	restart := &oauthSyncRepository{db: integrationDB}
	saved, err := restart.Get(ctx, op.Scope, op.OperationID)
	require.NoError(t, err)
	require.NotNil(t, saved)
	require.Equal(t, "pending", saved.State)
	var outboxCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT count(*) FROM scheduler_outbox WHERE account_id = $1", before.ID).Scan(&outboxCount))
	require.Equal(t, 1, outboxCount)
	accounts := newAccountRepositoryWithSQL(integrationEntClient, integrationDB, nil)
	after, err := accounts.GetByID(ctx, before.ID)
	require.NoError(t, err)
	require.Equal(t, "fixture-new", after.GetCredential("access_token"))
	require.Equal(t, before.ErrorMessage, after.ErrorMessage)
	require.False(t, after.Schedulable)
	require.True(t, before.RateLimitResetAt.Equal(*after.RateLimitResetAt))
	changed := *op
	changed.OperationID = "stale-operation"
	_, _, err = repo.Commit(ctx, &changed, before, creds)
	require.ErrorIs(t, err, service.ErrOAuthSyncConflict)
	missing, err := repo.Get(ctx, changed.Scope, changed.OperationID)
	require.NoError(t, err)
	require.Nil(t, missing)
	changed = *op
	changed.RequestHash = strings.Repeat("b", 64)
	_, _, err = repo.Commit(ctx, &changed, before, creds)
	require.ErrorIs(t, err, service.ErrIdempotencyKeyConflict)
	var raw string
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT row_to_json(o)::text FROM oauth_sync_operations o WHERE id=$1", saved.ID).Scan(&raw))
	require.NotContains(t, raw, "fixture-new")
	require.NotContains(t, raw, "fixture-old-rt")
}
func TestOAuthDurableSyncPostgresOutboxFailureRollsBackReceiptAndCredentials(t *testing.T) {
	repo, before, op := durableSyncPostgresFixture(t)
	ctx := context.Background()
	constraint := fmt.Sprintf("fixture_sync_fail_%d", before.ID)
	_, err := integrationDB.ExecContext(ctx, fmt.Sprintf("ALTER TABLE scheduler_outbox ADD CONSTRAINT %s CHECK (account_id <> %d)", constraint, before.ID))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, "ALTER TABLE scheduler_outbox DROP CONSTRAINT "+constraint)
	})
	_, _, err = repo.Commit(ctx, op, before, map[string]any{"access_token": "must-not-commit"})
	require.Error(t, err)
	saved, err := repo.Get(ctx, op.Scope, op.OperationID)
	require.NoError(t, err)
	require.Nil(t, saved)
	accounts := newAccountRepositoryWithSQL(integrationEntClient, integrationDB, nil)
	after, err := accounts.GetByID(ctx, before.ID)
	require.NoError(t, err)
	require.Equal(t, before.Credentials, after.Credentials)
	require.True(t, before.UpdatedAt.Equal(after.UpdatedAt))
}
func TestOAuthDurableSyncPostgresLeaseRecoveryIsFenced(t *testing.T) {
	repo, before, op := durableSyncPostgresFixture(t)
	ctx := context.Background()
	_, _, err := repo.Commit(ctx, op, before, map[string]any{"access_token": "fixture-new"})
	require.NoError(t, err)
	first, err := repo.Claim(ctx)
	require.NoError(t, err)
	require.NotNil(t, first)
	blocked, err := repo.Claim(ctx)
	require.NoError(t, err)
	require.Nil(t, blocked)
	require.NoError(t, repo.Progress(ctx, first, true, false, "", false, false))
	_, err = integrationDB.ExecContext(ctx, "UPDATE oauth_sync_operations SET lease_until = clock_timestamp() - INTERVAL '1 second' WHERE id=$1", first.ID)
	require.NoError(t, err)
	recovered, err := repo.Claim(ctx)
	require.NoError(t, err)
	require.NotNil(t, recovered)
	require.NotEqual(t, first.LeaseID, recovered.LeaseID)
	require.True(t, recovered.CacheDone)
	require.Error(t, repo.Progress(ctx, first, true, true, "", true, false))
	require.NoError(t, repo.Progress(ctx, recovered, true, true, "", true, false))
	require.NoError(t, repo.Retry(ctx, op.Scope, op.OperationID))
	final, err := repo.Get(ctx, op.Scope, op.OperationID)
	require.NoError(t, err)
	require.Equal(t, "completed", final.State)
	none, err := repo.Claim(ctx)
	require.NoError(t, err)
	require.Nil(t, none)
	instance, err := repo.InstanceID(ctx)
	require.NoError(t, err)
	restarted := &oauthSyncRepository{db: integrationDB}
	same, err := restarted.InstanceID(ctx)
	require.NoError(t, err)
	require.Equal(t, instance, same)
}
