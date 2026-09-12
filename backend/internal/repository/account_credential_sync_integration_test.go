//go:build integration

package repository

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOAuthCredentialSyncPostgresConcurrentCASPreservesRuntime(t *testing.T) {
	ctx := context.Background()
	account := mustCreateAccount(t, integrationEntClient, &service.Account{
		Name: "credential-sync-concurrency-fixture", Platform: service.PlatformOpenAI,
		Credentials: map[string]any{"email": "fixture@example.invalid", "access_token": "old", "refresh_token": "old-rt"},
	})
	t.Cleanup(func() {
		// The audit outbox has no account FK; remove only this fixture's events.
		_, err := integrationDB.ExecContext(ctx, "DELETE FROM scheduler_outbox WHERE account_id = $1", account.ID)
		require.NoError(t, err)
		require.NoError(t, integrationEntClient.Account.DeleteOneID(account.ID).Exec(ctx))
	})
	reset := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
	_, err := integrationEntClient.Account.UpdateOneID(account.ID).
		SetSchedulable(false).SetStatus(service.StatusError).SetErrorMessage("oauth 401 unauthorized").
		SetRateLimitResetAt(reset).SetTempUnschedulableUntil(reset).Save(ctx)
	require.NoError(t, err)
	repo := newAccountRepositoryWithSQL(integrationEntClient, integrationDB, nil)
	before, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make([]bool, 2)
	errors := make([]error, 2)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], errors[i] = repo.UpdateOAuthCredentialsIfUnchanged(ctx, account.ID, before.UpdatedAt, before.Credentials,
				map[string]any{"access_token": []string{"winner-a", "winner-b"}[i], "refresh_token": "new-rt", "_token_version": 7})
		}(i)
	}
	close(start)
	wg.Wait()
	require.NoError(t, errors[0])
	require.NoError(t, errors[1])
	require.NotEqual(t, results[0], results[1], "exactly one writer may consume the original snapshot")
	after, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.True(t, after.UpdatedAt.After(before.UpdatedAt))
	require.False(t, after.Schedulable)
	require.Equal(t, before.Status, after.Status)
	require.Equal(t, before.ErrorMessage, after.ErrorMessage)
	require.True(t, after.RateLimitResetAt.Equal(reset))
	require.True(t, after.TempUnschedulableUntil.Equal(reset))
	require.Equal(t, before.Concurrency, after.Concurrency)
	require.Equal(t, before.Priority, after.Priority)
	var count int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT count(*) FROM scheduler_outbox WHERE account_id = $1", account.ID).Scan(&count))
	require.Equal(t, 1, count, "only the committed credential version emits a scheduler event")
}
