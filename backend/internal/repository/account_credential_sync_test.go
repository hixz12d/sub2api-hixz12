package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOAuthCredentialSyncCASAndOutboxAreAtomic(t *testing.T) {
	for _, affected := range []int64{0, 1} {
		exec := &recordingSQLExecutor{result: rowsAffectedResult(affected)}
		repo := newAccountRepositoryWithSQL(nil, exec, nil)
		stamp := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)
		applied, err := repo.UpdateOAuthCredentialsIfUnchanged(context.Background(), 42, stamp,
			map[string]any{"access_token": "old"}, map[string]any{"access_token": "new", "_token_version": 7})
		require.NoError(t, err)
		require.Equal(t, affected == 1, applied)
		require.Len(t, exec.execQueries, 1)
		query := normalizeSQLWhitespace(exec.execQueries[0])
		for _, fragment := range []string{"WITH updated AS", "a.deleted_at IS NULL", "a.platform = $3", "a.type = $4", "a.updated_at = $5", "a.credentials = $6::jsonb", "INSERT INTO scheduler_outbox", "FROM updated", "clock_timestamp()"} {
			require.Contains(t, query, fragment)
		}
		require.NotContains(t, query, "schedulable =")
		require.NotContains(t, query, "status =")
		require.Equal(t, stamp, exec.execArgs[0][4])
		require.Equal(t, service.PlatformOpenAI, exec.execArgs[0][2])
		original, ok := exec.execArgs[0][5].(string)
		require.True(t, ok)
		require.JSONEq(t, `{"access_token":"old"}`, original)
	}
}
