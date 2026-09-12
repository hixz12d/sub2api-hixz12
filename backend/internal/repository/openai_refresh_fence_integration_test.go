//go:build integration

package repository

import (
	"context"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func refreshFenceFixture(t *testing.T) (*accountRepository, *service.Account, string) {
	_, before, _ := durableSyncPostgresFixture(t)
	rt := fmt.Sprintf("fixture-refresh-%d", before.ID)
	_, err := integrationDB.ExecContext(context.Background(), `UPDATE accounts SET status='active',credentials=jsonb_set(credentials,'{refresh_token}',to_jsonb($1::text)) WHERE id=$2`, rt, before.ID)
	require.NoError(t, err)
	repo := newAccountRepositoryWithSQL(integrationEntClient, integrationDB, nil)
	before, err = repo.GetByID(context.Background(), before.ID)
	require.NoError(t, err)
	t.Cleanup(func() {
		var grant string
		if integrationDB.QueryRow("SELECT grant_id::text FROM openai_refresh_tokens WHERE token_hash=$1", service.OAuthAccessTokenHash(rt)).Scan(&grant) == nil {
			_, _ = integrationDB.Exec("DELETE FROM openai_refresh_handoffs WHERE grant_id=$1", grant)
			_, _ = integrationDB.Exec("DELETE FROM openai_refresh_tokens WHERE grant_id=$1", grant)
			_, _ = integrationDB.Exec("DELETE FROM openai_refresh_grants WHERE id=$1", grant)
		}
	})
	return repo, before, rt
}
func TestOpenAIRefreshFenceCrossReplicaAndRotation(t *testing.T) {
	repo, before, rt := refreshFenceFixture(t)
	ctx := context.Background()
	first, err := repo.BeginOpenAIRefresh(ctx, rt)
	require.NoError(t, err)
	second := newAccountRepositoryWithSQL(integrationEntClient, integrationDB, nil)
	_, err = second.BeginOpenAIRefresh(ctx, rt)
	require.ErrorIs(t, err, service.ErrOpenAIRefreshFenced)
	nextRT := rt + "-next"
	applied, err := repo.FinishOpenAIRefresh(ctx, first, before, map[string]any{"access_token": "new", "refresh_token": nextRT}, nextRT)
	require.NoError(t, err)
	require.True(t, applied)
	_, err = second.BeginOpenAIRefresh(ctx, rt)
	require.ErrorIs(t, err, service.ErrOpenAIRefreshFenced)
	next, err := second.BeginOpenAIRefresh(ctx, nextRT)
	require.NoError(t, err)
	require.Equal(t, first.GrantID, next.GrantID)
	require.NoError(t, second.MarkOpenAIRefreshUncertain(ctx, next))
	after, err := repo.GetByID(ctx, before.ID)
	require.NoError(t, err)
	require.False(t, after.Schedulable)
	require.Equal(t, before.ErrorMessage, after.ErrorMessage)
}
func TestOpenAIRefreshFenceOldReplyCannotOverwriteNewCredentials(t *testing.T) {
	repo, before, rt := refreshFenceFixture(t)
	ctx := context.Background()
	ticket, err := repo.BeginOpenAIRefresh(ctx, rt)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `UPDATE accounts SET credentials=credentials || '{"access_token":"new-auth","_token_version":9999999999999}'::jsonb WHERE id=$1`, before.ID)
	require.NoError(t, err)
	applied, err := repo.FinishOpenAIRefresh(ctx, ticket, before, map[string]any{"access_token": "stale-reply", "refresh_token": rt + "next"}, rt+"next")
	require.NoError(t, err)
	require.False(t, applied)
	after, err := repo.GetByID(ctx, before.ID)
	require.NoError(t, err)
	require.Equal(t, "new-auth", after.GetCredential("access_token"))
	_, err = repo.FinishOpenAIRefresh(ctx, ticket, before, map[string]any{}, rt)
	require.ErrorIs(t, err, service.ErrOpenAIRefreshFenced)
}
func TestOpenAIRefreshFenceUncertainNeverExpiresIntoAnotherAttempt(t *testing.T) {
	repo, before, rt := refreshFenceFixture(t)
	ctx := context.Background()
	ticket, err := repo.BeginOpenAIRefresh(ctx, rt)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, "UPDATE openai_refresh_grants SET attempt_started_at=$1 WHERE id=$2", time.Now().Add(-24*time.Hour), ticket.GrantID)
	require.NoError(t, err)
	_, err = repo.BeginOpenAIRefresh(ctx, rt)
	require.ErrorIs(t, err, service.ErrOpenAIRefreshFenced)
	require.NoError(t, repo.MarkOpenAIRefreshUncertain(ctx, ticket))
	_, err = repo.FinishOpenAIRefresh(ctx, ticket, before, map[string]any{"access_token": "late"}, rt)
	require.ErrorIs(t, err, service.ErrOpenAIRefreshFenced)
}
