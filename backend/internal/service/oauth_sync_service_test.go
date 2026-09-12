package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type durableSyncMemory struct {
	op          *OAuthSyncOperation
	commits     int
	progressErr error
}

func (r *durableSyncMemory) InstanceID(context.Context) (string, error) {
	return "fixture-instance", nil
}
func (r *durableSyncMemory) Get(_ context.Context, scope, key string) (*OAuthSyncOperation, error) {
	if r.op == nil || r.op.Scope != scope || r.op.OperationID != key {
		return nil, nil
	}
	cp := *r.op
	return &cp, nil
}
func (r *durableSyncMemory) Latest(ctx context.Context, scope string, id int64) (*OAuthSyncOperation, error) {
	if r.op == nil || r.op.AccountID != id {
		return nil, nil
	}
	return r.Get(ctx, scope, r.op.OperationID)
}
func (r *durableSyncMemory) Commit(_ context.Context, op *OAuthSyncOperation, _ *Account, _ map[string]any) (*OAuthSyncOperation, bool, error) {
	r.commits++
	cp := *op
	cp.ID = 1
	cp.State = "pending"
	cp.CreatedAt = time.Now()
	cp.UpdatedAt = cp.CreatedAt
	r.op = &cp
	return &cp, false, nil
}
func (r *durableSyncMemory) Claim(context.Context) (*OAuthSyncOperation, error) {
	if r.op == nil || r.op.State != "pending" {
		return nil, nil
	}
	r.op.Attempts++
	cp := *r.op
	cp.LeaseID = "fixture-lease"
	return &cp, nil
}
func (r *durableSyncMemory) Progress(_ context.Context, _ *OAuthSyncOperation, cache, scheduler bool, code string, _, review bool) error {
	if r.progressErr != nil {
		return r.progressErr
	}
	r.op.CacheDone, r.op.SchedulerDone, r.op.LastError = cache, scheduler, code
	if cache && scheduler {
		r.op.State = "completed"
	}
	if review {
		r.op.State = "needs_review"
	}
	return nil
}
func (r *durableSyncMemory) Retry(context.Context, string, string) error { return nil }

type durableSyncCache struct {
	err       error
	calls     int
	seenToken string
}

func (c *durableSyncCache) InvalidateTokenStrict(_ context.Context, a *Account) error {
	c.calls++
	c.seenToken = a.GetCredential("access_token")
	return c.err
}

type durableSyncProjection struct {
	err   error
	calls int
}

func (p *durableSyncProjection) RefreshOAuthSyncProjection(context.Context, int64) error {
	p.calls++
	return p.err
}
func durableSyncFixture() (*OAuthSyncService, *durableSyncMemory, *oauthSyncAccountRepo, *SyncOAuthCredentialsRequest) {
	accounts, req := oauthSyncFixture()
	req.ExpectedInstanceID = "fixture-instance"
	repo := &durableSyncMemory{}
	svc := &OAuthSyncService{repo: repo, accounts: accounts, invalidator: &durableSyncCache{}, projector: &durableSyncProjection{}, wake: make(chan struct{}, 1)}
	return svc, repo, accounts, req
}
func TestOAuthDurableSyncReplayAndFingerprint(t *testing.T) {
	svc, repo, accounts, req := durableSyncFixture()
	ctx := context.Background()
	first, replay, err := svc.Submit(ctx, "admin-one:42", 42, req)
	require.NoError(t, err)
	require.False(t, replay)
	require.Equal(t, "pending", first.State)
	// Replay must precede snapshot validation, even if the account changed later.
	accounts.account.UpdatedAt = time.Now()
	_, replay, err = svc.Submit(ctx, "admin-one:42", 42, req)
	require.NoError(t, err)
	require.True(t, replay)
	require.Equal(t, 1, repo.commits)
	req.Credentials["access_token"] = "different"
	_, _, err = svc.Submit(ctx, "admin-one:42", 42, req)
	require.ErrorIs(t, err, ErrIdempotencyKeyConflict)
	unknown, err := svc.Operation(ctx, "another-admin:42", 42, req.OperationID)
	require.NoError(t, err)
	require.Nil(t, unknown)
}
func TestOAuthDurableSyncResumesOnlyUnfinishedStepsAfterRestart(t *testing.T) {
	svc, repo, accounts, req := durableSyncFixture()
	ctx := context.Background()
	_, _, err := svc.Submit(ctx, "admin:42", 42, req)
	require.NoError(t, err)
	projection := svc.projector.(*durableSyncProjection)
	projection.err = errors.New("fixture failure")
	_, err = svc.ProcessNext(ctx)
	require.NoError(t, err)
	require.True(t, repo.op.CacheDone)
	require.False(t, repo.op.SchedulerDone)
	require.Equal(t, "pending", repo.op.State)
	newCache := &durableSyncCache{err: errors.New("must not repeat successful step")}
	restarted := &OAuthSyncService{repo: repo, accounts: accounts, invalidator: newCache, projector: &durableSyncProjection{}}
	_, err = restarted.ProcessNext(ctx)
	require.NoError(t, err)
	require.Zero(t, newCache.calls)
	require.Equal(t, "completed", repo.op.State)
	require.Equal(t, 1, repo.commits)
	require.Zero(t, accounts.writes)
	require.False(t, accounts.account.Schedulable)
	require.Equal(t, StatusError, accounts.account.Status)
}
func TestOAuthDurableSyncCacheFailureAndLeaseLossNeverAdvanceScheduler(t *testing.T) {
	svc, repo, _, req := durableSyncFixture()
	_, _, err := svc.Submit(context.Background(), "admin:42", 42, req)
	require.NoError(t, err)
	cache := svc.invalidator.(*durableSyncCache)
	cache.err = errors.New("fixture secret")
	_, err = svc.ProcessNext(context.Background())
	require.NoError(t, err)
	require.False(t, repo.op.CacheDone)
	require.Equal(t, "token_cache_failed", repo.op.LastError)
	require.Zero(t, svc.projector.(*durableSyncProjection).calls)
	cache.err = nil
	repo.progressErr = errors.New("lease lost")
	_, err = svc.ProcessNext(context.Background())
	require.Error(t, err)
	require.Zero(t, svc.projector.(*durableSyncProjection).calls)
}
func TestOAuthDurableSyncUsesCurrentAccountAndRedactsState(t *testing.T) {
	svc, repo, accounts, req := durableSyncFixture()
	_, _, err := svc.Submit(context.Background(), "admin:42", 42, req)
	require.NoError(t, err)
	accounts.account.Credentials["access_token"] = "fixture-newer-secret"
	_, err = svc.ProcessNext(context.Background())
	require.NoError(t, err)
	require.Equal(t, "fixture-newer-secret", svc.invalidator.(*durableSyncCache).seenToken)
	snapshot, err := svc.Snapshot(context.Background(), "admin:42", 42)
	require.NoError(t, err)
	require.Contains(t, snapshot["remaining_blockers"], "schedulable_off")
	require.Equal(t, "not_verified", snapshot["availability"])
	encoded, err := json.Marshal(snapshot)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "fixture-newer-secret")
	require.NotContains(t, string(encoded), "refresh_token")
	require.Equal(t, "completed", repo.op.State)
}
func TestOAuthDurableSyncMissingAccountRequiresReview(t *testing.T) {
	svc, repo, accounts, req := durableSyncFixture()
	_, _, err := svc.Submit(context.Background(), "admin:42", 42, req)
	require.NoError(t, err)
	accounts.account = nil
	_, err = svc.ProcessNext(context.Background())
	require.NoError(t, err)
	require.Equal(t, "needs_review", repo.op.State)
	require.Zero(t, svc.invalidator.(*durableSyncCache).calls)
}
func TestOAuthDurableSyncWrongInstanceNeverWrites(t *testing.T) {
	svc, repo, _, req := durableSyncFixture()
	req.ExpectedInstanceID = "other-instance"
	_, _, err := svc.Submit(context.Background(), "admin:42", 42, req)
	require.Error(t, err)
	require.Zero(t, repo.commits)
}
