package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpenAIOAuthQuarantineMatchesCredentialsAndPublishesAtomically(t *testing.T) {
	for _, affected := range []int64{0, 1} {
		exec := &recordingSQLExecutor{result: rowsAffectedResult(affected)}
		repo := newAccountRepositoryWithSQL(nil, exec, nil)
		applied, err := repo.SetOpenAIOAuthErrorIfCredentialsUnchanged(context.Background(), 77,
			map[string]any{"access_token": "old", "refresh_token": "refresh", "_token_version": int64(7)}, "reauthorize")
		require.NoError(t, err)
		require.Equal(t, affected == 1, applied)
		require.Len(t, exec.execQueries, 1)
		query := normalizeSQLWhitespace(exec.execQueries[0])
		for _, fragment := range []string{"WITH updated AS", "a.deleted_at IS NULL", "a.platform = $4", "a.type = $5", "a.status = $6", "a.credentials = $7::jsonb", "schedulable = FALSE", "INSERT INTO scheduler_outbox", "FROM updated"} {
			require.Contains(t, query, fragment)
		}
		require.Equal(t, service.PlatformOpenAI, exec.execArgs[0][3])
		require.Equal(t, service.AccountTypeOAuth, exec.execArgs[0][4])
		require.Equal(t, service.StatusActive, exec.execArgs[0][5])
		require.JSONEq(t, `{"access_token":"old","refresh_token":"refresh","_token_version":7}`, exec.execArgs[0][6].(string))
		require.Equal(t, service.SchedulerOutboxEventAccountChanged, exec.execArgs[0][7])
	}
}

func TestOpenAIOAuthQuarantineCancellationRetainsAtomicOutbox(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	exec := &recordingSQLExecutor{result: rowsAffectedResult(1), afterExec: cancel}
	repo := newAccountRepositoryWithSQL(nil, exec, nil)
	applied, err := repo.SetOpenAIOAuthErrorIfCredentialsUnchanged(ctx, 77, map[string]any{"access_token": "old"}, "reauthorize")
	require.NoError(t, err)
	require.True(t, applied)
	require.True(t, errors.Is(ctx.Err(), context.Canceled))
	require.Len(t, exec.execQueries, 1)
	require.Contains(t, exec.execQueries[0], "INSERT INTO scheduler_outbox")
}
