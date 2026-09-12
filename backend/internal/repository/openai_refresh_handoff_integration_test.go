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

func handoffFixture(t *testing.T) (*accountRepository, *service.Account, *service.OpenAIRefreshHandoffRequest, string) {
	repo, before, rt := refreshFenceFixture(t)
	_, err := integrationDB.Exec(`UPDATE accounts SET credentials=credentials || '{"client_id":"fixture-client","_token_version":7}'::jsonb WHERE id=$1`, before.ID)
	require.NoError(t, err)
	before, err = repo.GetByID(context.Background(), before.ID)
	require.NoError(t, err)
	request := &service.OpenAIRefreshHandoffRequest{Action: "prepare", OperationID: fmt.Sprintf("handoff-%d", before.ID), ExpectedVersion: 7, ExpectedUpdatedAt: before.UpdatedAt.Format(time.RFC3339Nano)}
	t.Cleanup(func() {
		_, _ = integrationDB.Exec("DELETE FROM openai_refresh_delegated_accounts WHERE account_id=$1", before.ID)
	})
	return repo, before, request, rt
}
func TestOpenAIRefreshHandoffDrainsBeforeReleasingAndFencesOldTicket(t *testing.T) {
	repo, before, req, rt := handoffFixture(t)
	ctx := context.Background()
	ticket, err := repo.BeginOpenAIRefresh(ctx, rt)
	require.NoError(t, err)
	pending, err := repo.OpenAIRefreshHandoff(ctx, before, "scope", req)
	require.NoError(t, err)
	require.Equal(t, "draining", pending["state"])
	_, err = repo.BeginOpenAIRefresh(ctx, rt)
	require.ErrorIs(t, err, service.ErrOpenAIRefreshFenced)
	req.Action = "read"
	_, err = repo.OpenAIRefreshHandoff(ctx, before, "scope", req)
	require.ErrorIs(t, err, service.ErrOpenAIRefreshFenced)
	credentials := service.MergeCredentials(before.Credentials, map[string]any{"access_token": "handoff-new-AT", "refresh_token": rt + "-new"})
	applied, err := repo.FinishOpenAIRefresh(ctx, ticket, before, credentials, rt+"-new")
	require.NoError(t, err)
	require.True(t, applied)
	fresh, err := repo.GetByID(ctx, before.ID)
	require.NoError(t, err)
	req.Action = "prepare"
	ready, err := repo.OpenAIRefreshHandoff(ctx, fresh, "scope", req)
	require.NoError(t, err)
	require.Equal(t, "ready", ready["state"])
	require.NotContains(t, ready, "credentials")
	req.Action = "read"
	read, err := repo.OpenAIRefreshHandoff(ctx, fresh, "scope", req)
	require.NoError(t, err)
	require.Equal(t, rt+"-new", read["credentials"].(map[string]any)["refresh_token"])
	after, err := repo.GetByID(ctx, before.ID)
	require.NoError(t, err)
	require.Empty(t, after.GetCredential("refresh_token"))
	require.False(t, after.Schedulable)
	require.Equal(t, before.ErrorMessage, after.ErrorMessage)
	_, err = repo.BeginOpenAIRefresh(ctx, rt+"-new")
	require.ErrorIs(t, err, service.ErrOpenAIRefreshFenced)
	_, err = repo.FinishOpenAIRefresh(ctx, ticket, before, credentials, rt+"-new")
	require.ErrorIs(t, err, service.ErrOpenAIRefreshFenced)
	_, err = integrationDB.ExecContext(ctx, `UPDATE accounts SET credentials=credentials || '{"refresh_token":"attempted-reinsert"}'::jsonb WHERE id=$1`, before.ID)
	require.Error(t, err)
	req.Action = "ack"
	ack, err := repo.OpenAIRefreshHandoff(ctx, after, "scope", req)
	require.NoError(t, err)
	require.Equal(t, "acknowledged", ack["state"])
	req.Action = "read"
	_, err = repo.OpenAIRefreshHandoff(ctx, after, "scope", req)
	require.ErrorIs(t, err, service.ErrOpenAIRefreshFenced)
}
func TestOpenAIRefreshHandoffUncertainAndForeignScopeNeverReturnSecrets(t *testing.T) {
	repo, before, req, rt := handoffFixture(t)
	ctx := context.Background()
	ticket, err := repo.BeginOpenAIRefresh(ctx, rt)
	require.NoError(t, err)
	require.NoError(t, repo.MarkOpenAIRefreshUncertain(ctx, ticket))
	pending, err := repo.OpenAIRefreshHandoff(ctx, before, "scope", req)
	require.NoError(t, err)
	require.Equal(t, "draining", pending["state"])
	require.Equal(t, true, pending["uncertain"])
	_, err = repo.OpenAIRefreshHandoff(ctx, before, "foreign", req)
	require.ErrorIs(t, err, service.ErrOAuthSyncConflict)
	req.Action = "read"
	_, err = repo.OpenAIRefreshHandoff(ctx, before, "scope", req)
	require.ErrorIs(t, err, service.ErrOpenAIRefreshFenced)
}
func TestOpenAIRefreshHandoffOutboxFailureRollsBackRTAndOwnership(t *testing.T) {
	repo, before, req, _ := handoffFixture(t)
	ctx := context.Background()
	constraint := fmt.Sprintf("fixture_handoff_outbox_%d", before.ID)
	_, err := integrationDB.ExecContext(ctx, fmt.Sprintf("ALTER TABLE scheduler_outbox ADD CONSTRAINT %s CHECK(account_id <> %d)", constraint, before.ID))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, "ALTER TABLE scheduler_outbox DROP CONSTRAINT "+constraint)
	})
	_, err = repo.OpenAIRefreshHandoff(ctx, before, "scope", req)
	require.Error(t, err)
	after, err := repo.GetByID(ctx, before.ID)
	require.NoError(t, err)
	require.Equal(t, before.Credentials, after.Credentials)
	var count int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT count(*) FROM openai_refresh_handoffs WHERE operation_id=$1", req.OperationID).Scan(&count))
	require.Zero(t, count)
}
