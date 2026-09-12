package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type oauthSyncAccountRepo struct {
	AccountRepository
	account *Account
	applied bool
	writes  int
	written map[string]any
}

func (r *oauthSyncAccountRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}
func (r *oauthSyncAccountRepo) UpdateOAuthCredentialsIfUnchanged(_ context.Context, _ int64, _ time.Time, _, credentials map[string]any) (bool, error) {
	r.writes++
	r.written = credentials
	return r.applied, nil
}

func oauthSyncFixture() (*oauthSyncAccountRepo, *SyncOAuthCredentialsRequest) {
	stamp := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)
	repo := &oauthSyncAccountRepo{applied: true, account: &Account{
		ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusError, ErrorMessage: "oauth 401 unauthorized", Schedulable: false, UpdatedAt: stamp,
		Credentials: map[string]any{"email": "fixture@example.invalid", "access_token": "old", "refresh_token": "old-rt", "client_id": "fixture-client"},
	}}
	req := &SyncOAuthCredentialsRequest{ContractVersion: 1, OperationID: "fixture-operation",
		ExpectedUpdatedAt: stamp.Format(time.RFC3339Nano), RecoveryMode: "credentials_only",
		ExpectedIdentity: map[string]any{"email": "fixture@example.invalid", "workspace_id": nil},
		Credentials:      map[string]any{"access_token": "fixture-at", "refresh_token": "fixture-rt", "client_id": "fixture-client"},
	}
	return repo, req
}

func TestOAuthCredentialSyncPreservesBlockersAndAdvancesVersion(t *testing.T) {
	repo, req := oauthSyncFixture()
	svc := &adminServiceImpl{accountRepo: repo}
	result, _, err := svc.SyncOpenAIOAuthCredentials(context.Background(), 42, req)
	require.NoError(t, err)
	require.Equal(t, 1, repo.writes)
	require.Equal(t, "succeeded", result.CredentialWrite)
	require.Equal(t, "pending", result.TokenCacheInvalidation)
	require.Equal(t, "skipped", result.AuthRecovery)
	require.False(t, result.Schedulable)
	require.Equal(t, "not_assessed", result.SchedulingAssessment)
	require.Equal(t, StatusError, repo.account.Status)
	require.Equal(t, "old", repo.account.Credentials["access_token"])
	require.Equal(t, "fixture-at", repo.written["access_token"])
	require.Equal(t, "fixture-client", repo.written["client_id"])
	require.NotZero(t, repo.written["_token_version"])
}

func TestOAuthCredentialSyncRejectsBeforeMutation(t *testing.T) {
	cases := map[string]func(*oauthSyncAccountRepo, *SyncOAuthCredentialsRequest){
		"auth_only":            func(_ *oauthSyncAccountRepo, r *SyncOAuthCredentialsRequest) { r.RecoveryMode = "auth_only" },
		"missing_operation":    func(_ *oauthSyncAccountRepo, r *SyncOAuthCredentialsRequest) { r.OperationID = "" },
		"missing_precondition": func(_ *oauthSyncAccountRepo, r *SyncOAuthCredentialsRequest) { r.ExpectedUpdatedAt = "" },
		"stale_precondition": func(repo *oauthSyncAccountRepo, _ *SyncOAuthCredentialsRequest) {
			repo.account.UpdatedAt = repo.account.UpdatedAt.Add(time.Second)
		},
		"missing_remote_email": func(repo *oauthSyncAccountRepo, _ *SyncOAuthCredentialsRequest) {
			delete(repo.account.Credentials, "email")
		},
		"different_email": func(_ *oauthSyncAccountRepo, r *SyncOAuthCredentialsRequest) {
			r.ExpectedIdentity["email"] = "other@example.invalid"
		},
		"missing_context": func(_ *oauthSyncAccountRepo, r *SyncOAuthCredentialsRequest) {
			delete(r.ExpectedIdentity, "workspace_id")
		},
		"different_workspace": func(_ *oauthSyncAccountRepo, r *SyncOAuthCredentialsRequest) {
			r.ExpectedIdentity["workspace_id"] = "other-workspace"
		},
		"different_client": func(_ *oauthSyncAccountRepo, r *SyncOAuthCredentialsRequest) {
			r.Credentials["client_id"] = "other-client"
		},
		"missing_client":        func(_ *oauthSyncAccountRepo, r *SyncOAuthCredentialsRequest) { delete(r.Credentials, "client_id") },
		"mixed_grant":           func(_ *oauthSyncAccountRepo, r *SyncOAuthCredentialsRequest) { delete(r.Credentials, "refresh_token") },
		"null_access_token":     func(_ *oauthSyncAccountRepo, r *SyncOAuthCredentialsRequest) { r.Credentials["access_token"] = nil },
		"config_in_credentials": func(_ *oauthSyncAccountRepo, r *SyncOAuthCredentialsRequest) { r.Credentials["schedulable"] = true },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			repo, req := oauthSyncFixture()
			mutate(repo, req)
			svc := &adminServiceImpl{accountRepo: repo}
			_, _, err := svc.SyncOpenAIOAuthCredentials(context.Background(), 42, req)
			require.Error(t, err)
			require.Zero(t, repo.writes)
			require.NotContains(t, err.Error(), "fixture-rt")
		})
	}
}

func TestOAuthCredentialSyncVersionSurvivesJSONNumberAndClockSkew(t *testing.T) {
	repo, req := oauthSyncFixture()
	old := time.Now().Add(time.Hour).UnixMilli()
	repo.account.Credentials["_token_version"] = float64(old)
	svc := &adminServiceImpl{accountRepo: repo}
	_, _, err := svc.SyncOpenAIOAuthCredentials(context.Background(), 42, req)
	require.NoError(t, err)
	require.Equal(t, old+1, repo.written["_token_version"])
}

func TestOAuthCredentialSyncConcurrentChangeIsConflict(t *testing.T) {
	repo, req := oauthSyncFixture()
	repo.applied = false
	svc := &adminServiceImpl{accountRepo: repo}
	_, _, err := svc.SyncOpenAIOAuthCredentials(context.Background(), 42, req)
	require.ErrorIs(t, err, ErrOAuthSyncConflict)
}

func TestOAuthCredentialSyncReceiptReplayAndUnknown(t *testing.T) {
	repo := newInMemoryIdempotencyRepo()
	coordinator := NewIdempotencyCoordinator(repo, DefaultIdempotencyConfig())
	previous := DefaultIdempotencyCoordinator()
	SetDefaultIdempotencyCoordinator(coordinator)
	t.Cleanup(func() { SetDefaultIdempotencyCoordinator(previous) })
	opts := IdempotencyExecuteOptions{Scope: "oauth-sync:admin:1:42", Method: "POST", Route: "/sync", ActorScope: "admin:1",
		IdempotencyKey: "fixture-operation", Payload: map[string]any{"access_token": "fixture-at"}}
	calls := 0
	for i := 0; i < 2; i++ {
		_, err := coordinator.Execute(context.Background(), opts, func(context.Context) (any, error) {
			calls++
			return &SyncOAuthCredentialsResult{ContractVersion: 1, OperationID: "fixture-operation", RemoteAccountID: 42, CredentialWrite: "succeeded", Partial: true}, nil
		})
		require.NoError(t, err)
	}
	require.Equal(t, 1, calls)
	receipt, err := LookupOAuthSyncOperation(context.Background(), opts.Scope, opts.IdempotencyKey)
	require.NoError(t, err)
	require.Equal(t, "recorded", receipt["state"])
	require.NotContains(t, receipt, "credentials")
	unknown, err := LookupOAuthSyncOperation(context.Background(), "oauth-sync:admin:2:42", opts.IdempotencyKey)
	require.NoError(t, err)
	require.Equal(t, "unknown", unknown["state"])
	opts.Payload = map[string]any{"access_token": "different"}
	_, err = coordinator.Execute(context.Background(), opts, func(context.Context) (any, error) { t.Fatal("must not run"); return nil, nil })
	require.Error(t, err)
}
