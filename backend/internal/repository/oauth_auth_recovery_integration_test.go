//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOAuthRecoveryPostgresPreservesOtherBlockersAndRejectsLate401(t *testing.T) {
	repo, before, op := durableSyncPostgresFixture(t)
	ctx := context.Background()
	_, err := integrationDB.ExecContext(ctx, `UPDATE accounts SET status='active', error_message=NULL, credentials=credentials || '{"_token_version":7}'::jsonb WHERE id=$1`, before.ID)
	require.NoError(t, err)
	accounts := newAccountRepositoryWithSQL(integrationEntClient, integrationDB, nil)
	before, err = accounts.GetByID(ctx, before.ID)
	require.NoError(t, err)
	recorder := accounts
	require.NoError(t, recorder.RecordOAuthUnauthorized(ctx, before, "fixture-old", true, time.Now().Add(time.Hour)))
	failed, err := accounts.GetByID(ctx, before.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusError, failed.Status)
	now := time.Now().UTC()
	op.AuthRecovery = "cleared"
	op.ValidationScope = service.OAuthValidationScope
	op.ValidatedAt = &now
	creds := map[string]any{"access_token": "fixture-new", "refresh_token": "fixture-new-rt", "_token_version": op.CredentialVersion}
	saved, _, err := repo.Commit(ctx, op, failed, creds)
	require.NoError(t, err)
	require.Equal(t, "cleared", saved.AuthRecovery)
	after, err := accounts.GetByID(ctx, before.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusActive, after.Status)
	require.Empty(t, after.ErrorMessage)
	require.False(t, after.Schedulable)
	require.True(t, before.RateLimitResetAt.Equal(*after.RateLimitResetAt))
	require.NoError(t, recorder.RecordOAuthUnauthorized(ctx, before, "fixture-old", true, time.Now().Add(time.Hour)))
	after, err = accounts.GetByID(ctx, before.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusActive, after.Status)
	var markers int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT count(*) FROM oauth_auth_errors WHERE account_id=$1", before.ID).Scan(&markers))
	require.Zero(t, markers)
}

func TestOAuthRecoveryPostgresRefreshRejectionUsesItsCredentialSnapshot(t *testing.T) {
	repo, before, op := durableSyncPostgresFixture(t)
	ctx := context.Background()
	_, err := integrationDB.ExecContext(ctx, `UPDATE accounts SET status='active',error_message=NULL,credentials=credentials || '{"_token_version":7}'::jsonb WHERE id=$1`, before.ID)
	require.NoError(t, err)
	accounts := newAccountRepositoryWithSQL(integrationEntClient, integrationDB, nil)
	before, err = accounts.GetByID(ctx, before.ID)
	require.NoError(t, err)
	applied, err := accounts.SetOpenAIOAuthErrorIfCredentialsUnchanged(ctx, before.ID, before.Credentials, "OpenAI OAuth refresh credential rejected; reauthorize the account")
	require.NoError(t, err)
	require.True(t, applied)
	failed, err := accounts.GetByID(ctx, before.ID)
	require.NoError(t, err)
	now := time.Now().UTC()
	op.AuthRecovery = "cleared"
	op.ValidationScope = service.OAuthValidationScope
	op.ValidatedAt = &now
	_, _, err = repo.Commit(ctx, op, failed, map[string]any{"access_token": "fixture-new", "_token_version": op.CredentialVersion})
	require.NoError(t, err)
	after, err := accounts.GetByID(ctx, before.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusActive, after.Status)
	require.False(t, after.Schedulable)
}

func TestOAuthRecoveryPostgresUnknownErrorRollsBackEntireOperation(t *testing.T) {
	repo, before, op := durableSyncPostgresFixture(t)
	now := time.Now().UTC()
	op.AuthRecovery = "cleared"
	op.ValidationScope = service.OAuthValidationScope
	op.ValidatedAt = &now
	_, _, err := repo.Commit(context.Background(), op, before, map[string]any{"access_token": "must-not-write", "_token_version": op.CredentialVersion})
	require.ErrorIs(t, err, service.ErrOAuthAuthUnattributed)
	stored, err := repo.Get(context.Background(), op.Scope, op.OperationID)
	require.NoError(t, err)
	require.Nil(t, stored)
	accounts := newAccountRepositoryWithSQL(integrationEntClient, integrationDB, nil)
	after, err := accounts.GetByID(context.Background(), before.ID)
	require.NoError(t, err)
	require.Equal(t, before.Credentials, after.Credentials)
	require.Equal(t, before.ErrorMessage, after.ErrorMessage)
}

func TestOAuthRecoveryPostgresSameTokenCannotRecoverAndOutboxFailureRollsBack(t *testing.T) {
	repo, before, op := durableSyncPostgresFixture(t)
	ctx := context.Background()
	_, err := integrationDB.ExecContext(ctx, `UPDATE accounts SET status='active', error_message=NULL, credentials=credentials || '{"_token_version":7}'::jsonb WHERE id=$1`, before.ID)
	require.NoError(t, err)
	accounts := newAccountRepositoryWithSQL(integrationEntClient, integrationDB, nil)
	before, err = accounts.GetByID(ctx, before.ID)
	require.NoError(t, err)
	require.NoError(t, accounts.RecordOAuthUnauthorized(ctx, before, "fixture-old", false, time.Now().Add(time.Hour)))
	before, err = accounts.GetByID(ctx, before.ID)
	require.NoError(t, err)
	now := time.Now().UTC()
	op.AuthRecovery = "cleared"
	op.ValidationScope = service.OAuthValidationScope
	op.ValidatedAt = &now
	_, _, err = repo.Commit(ctx, op, before, map[string]any{"access_token": "fixture-old", "_token_version": op.CredentialVersion})
	require.ErrorIs(t, err, service.ErrOAuthAuthUnattributed)
	_, err = integrationDB.ExecContext(ctx, `CREATE FUNCTION fixture_recovery_outbox_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture outbox failure'; END $$; CREATE TRIGGER fixture_recovery_fail BEFORE INSERT ON scheduler_outbox FOR EACH ROW EXECUTE FUNCTION fixture_recovery_outbox_failure()`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, `DROP TRIGGER fixture_recovery_fail ON scheduler_outbox; DROP FUNCTION fixture_recovery_outbox_failure()`)
	})
	_, _, err = repo.Commit(ctx, op, before, map[string]any{"access_token": "fixture-new", "_token_version": op.CredentialVersion})
	require.Error(t, err)
	require.Contains(t, err.Error(), "fixture outbox failure")
	after, err := accounts.GetByID(ctx, before.ID)
	require.NoError(t, err)
	require.Equal(t, before.Credentials, after.Credentials)
	require.Equal(t, before.TempUnschedulableReason, after.TempUnschedulableReason)
	var markers int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT count(*) FROM oauth_auth_errors WHERE account_id=$1", before.ID).Scan(&markers))
	require.Equal(t, 1, markers)
	stored, err := repo.Get(ctx, op.Scope, op.OperationID)
	require.NoError(t, err)
	require.Nil(t, stored)
}

func TestOAuthRecoveryPostgresTemporaryEvidenceCannotClearManualPause(t *testing.T) {
	repo, before, op := durableSyncPostgresFixture(t)
	ctx := context.Background()
	_, err := integrationDB.ExecContext(ctx, `UPDATE accounts SET status='active', error_message=NULL, credentials=credentials || '{"_token_version":7}'::jsonb WHERE id=$1`, before.ID)
	require.NoError(t, err)
	accounts := newAccountRepositoryWithSQL(integrationEntClient, integrationDB, nil)
	before, err = accounts.GetByID(ctx, before.ID)
	require.NoError(t, err)
	recorder := accounts
	require.NoError(t, recorder.RecordOAuthUnauthorized(ctx, before, "fixture-old", false, time.Now().Add(time.Hour)))
	_, err = integrationDB.ExecContext(ctx, `UPDATE accounts SET temp_unschedulable_reason='manual quota hold', updated_at=clock_timestamp() WHERE id=$1`, before.ID)
	require.NoError(t, err)
	before, err = accounts.GetByID(ctx, before.ID)
	require.NoError(t, err)
	now := time.Now().UTC()
	op.AuthRecovery = "cleared"
	op.ValidationScope = service.OAuthValidationScope
	op.ValidatedAt = &now
	_, _, err = repo.Commit(ctx, op, before, map[string]any{"access_token": "fixture-new", "_token_version": op.CredentialVersion})
	require.ErrorIs(t, err, service.ErrOAuthAuthUnattributed)
	after, err := accounts.GetByID(ctx, before.ID)
	require.NoError(t, err)
	require.Equal(t, "manual quota hold", after.TempUnschedulableReason)
	require.Equal(t, before.Credentials, after.Credentials)
}
