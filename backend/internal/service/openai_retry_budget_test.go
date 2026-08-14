package service

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIRetryBudgetBoundsAttemptsAndDistinctAccounts(t *testing.T) {
	budget := NewOpenAIRetryBudget(false)
	require.NoError(t, budget.Reserve(11))
	require.NoError(t, budget.Reserve(12))
	require.ErrorIs(t, budget.Reserve(13), ErrOpenAIRetryBudgetExhausted)

	snapshot := budget.Snapshot()
	require.Equal(t, 2, snapshot.Attempts)
	require.Equal(t, 2, snapshot.DistinctAccounts)
}

func TestOpenAIRetryBudgetStatefulStaysOnOneAccount(t *testing.T) {
	budget := NewOpenAIRetryBudget(true)
	require.NoError(t, budget.Reserve(11))
	require.ErrorIs(t, budget.Reserve(12), ErrOpenAIRetryBudgetExhausted)
	require.NoError(t, budget.Reserve(11))
	require.ErrorIs(t, budget.Reserve(11), ErrOpenAIRetryBudgetExhausted)
}

func TestOpenAIRetryBudgetOutputClosesReplay(t *testing.T) {
	budget := NewOpenAIRetryBudget(false)
	require.NoError(t, budget.Reserve(11))
	budget.MarkBytesEmitted()
	require.ErrorIs(t, budget.Reserve(11), ErrOpenAIRetryBudgetExhausted)

	snapshot := budget.Snapshot()
	require.True(t, snapshot.StreamStarted)
	require.True(t, snapshot.BytesEmitted)
	require.False(t, snapshot.ReplaySafe)
}

func TestOpenAIRetryBudgetCredentialRecoveryCannotChangeAccount(t *testing.T) {
	budget := NewOpenAIRetryBudget(false)
	require.NoError(t, budget.Reserve(11))
	budget.RecordFailure(ClassifyOpenAIRetryFailure(context.Background(), http.StatusUnauthorized, nil, false, true))
	require.ErrorIs(t, budget.Reserve(12), ErrOpenAIRetryBudgetExhausted)
	require.NoError(t, budget.Reserve(11))
	require.True(t, budget.UseRefresh())
	require.False(t, budget.UseRefresh())
}

func TestOpenAIRetryBudgetFailurePolicyBlocksForbiddenRetry(t *testing.T) {
	budget := NewOpenAIRetryBudget(false)
	require.NoError(t, budget.Reserve(11))
	budget.RecordFailure(ClassifyOpenAIRetryFailure(context.Background(), http.StatusForbidden, nil, false, true))
	require.ErrorIs(t, budget.Reserve(11), ErrOpenAIRetryBudgetExhausted)
	require.ErrorIs(t, budget.Reserve(12), ErrOpenAIRetryBudgetExhausted)
}

func TestOpenAIRetryBudgetPreviousRecoveryOnceAndFunctionStateDetection(t *testing.T) {
	budget := NewOpenAIRetryBudget(true)
	require.True(t, budget.UsePreviousResponseRecovery())
	require.False(t, budget.UsePreviousResponseRecovery())
	require.True(t, openAIRetryRequestIsStateful([]byte(`{"input":[{"type":"function_call_output","call_id":"call_1","output":"ok"}]}`)))
	require.True(t, openAIRetryRequestIsStateful([]byte(`{"previous_response_id":"resp_1"}`)))
	require.False(t, openAIRetryRequestIsStateful([]byte(`{"input":"hello"}`)))
}

func TestOpenAIRetryFailureClassifier(t *testing.T) {
	require.Equal(t, OpenAIRetryFailureRequest, ClassifyOpenAIRetryFailure(context.Background(), http.StatusBadRequest, nil, false, true).Class)
	require.False(t, ClassifyOpenAIRetryFailure(context.Background(), http.StatusForbidden, nil, false, true).RetrySameAccount)

	transient := ClassifyOpenAIRetryFailure(context.Background(), http.StatusServiceUnavailable, nil, false, true)
	require.True(t, transient.RetrySameAccount)
	require.True(t, transient.RetryOtherAccount)
	require.False(t, ClassifyOpenAIRetryFailure(context.Background(), http.StatusServiceUnavailable, nil, true, true).RetryOtherAccount)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.Equal(t, OpenAIRetryFailureCanceled, ClassifyOpenAIRetryFailure(ctx, 0, errors.New("transport"), false, true).Class)
	require.Equal(t, OpenAIRetryFailureState, ClassifyOpenAIRetryFailure(context.Background(), http.StatusServiceUnavailable, nil, false, false).Class)
}

func TestOpenAIRetryBudgetFeatureFlagAndAPIKeyControl(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	oauth := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{openAIRetryBudgetV2ExtraKey: true}}
	require.NotNil(t, EnsureOpenAIRetryBudget(c, oauth, []byte(`{"input":"hello"}`)))

	c2, _ := gin.CreateTestContext(nil)
	apiKey := &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{openAIRetryBudgetV2ExtraKey: true}}
	require.Nil(t, EnsureOpenAIRetryBudget(c2, apiKey, []byte(`{"input":"hello"}`)))

	c3, _ := gin.CreateTestContext(nil)
	legacyOAuth := &Account{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	require.Nil(t, EnsureOpenAIRetryBudget(c3, legacyOAuth, []byte(`{"input":"hello"}`)))
}

func TestOpenAIRetryBudgetConcurrentReserveIsBounded(t *testing.T) {
	budget := NewOpenAIRetryBudget(false)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = budget.Reserve(11)
		}()
	}
	wg.Wait()
	require.Equal(t, 2, budget.Snapshot().Attempts)
}

type retryBudgetRefreshRepo struct {
	AccountRepository
	mu      sync.Mutex
	account *Account
}

func (r *retryBudgetRefreshRepo) GetByID(context.Context, int64) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	copyAccount := *r.account
	copyAccount.Credentials = make(map[string]any, len(r.account.Credentials))
	for key, value := range r.account.Credentials {
		copyAccount.Credentials[key] = value
	}
	return &copyAccount, nil
}

func (r *retryBudgetRefreshRepo) UpdateCredentials(_ context.Context, _ int64, credentials map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.account.Credentials = make(map[string]any, len(credentials))
	for key, value := range credentials {
		r.account.Credentials[key] = value
	}
	return nil
}

type retryBudgetRefreshExecutor struct {
	mu    sync.Mutex
	calls int
}

func (e *retryBudgetRefreshExecutor) CanRefresh(*Account) bool                  { return true }
func (e *retryBudgetRefreshExecutor) NeedsRefresh(*Account, time.Duration) bool { return false }
func (e *retryBudgetRefreshExecutor) CacheKey(*Account) string                  { return "openai:retry-budget:401" }
func (e *retryBudgetRefreshExecutor) Refresh(context.Context, *Account) (map[string]any, error) {
	e.mu.Lock()
	e.calls++
	e.mu.Unlock()
	time.Sleep(20 * time.Millisecond)
	return map[string]any{"access_token": "new-token", "refresh_token": "new-refresh"}, nil
}

func TestOAuthRefreshAPIForced401ConcurrentUsesOneRefresh(t *testing.T) {
	base := &Account{
		ID: 77, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive,
		Credentials: map[string]any{"access_token": "old-token", "refresh_token": "old-refresh", "_token_version": int64(1)},
	}
	repo := &retryBudgetRefreshRepo{account: base}
	executor := &retryBudgetRefreshExecutor{}
	api := NewOAuthRefreshAPI(repo, nil)

	oldA, oldB := *base, *base
	oldA.Credentials = map[string]any{"access_token": "old-token", "refresh_token": "old-refresh", "_token_version": int64(1)}
	oldB.Credentials = map[string]any{"access_token": "old-token", "refresh_token": "old-refresh", "_token_version": int64(1)}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, snapshot := range []*Account{&oldA, &oldB} {
		wg.Add(1)
		go func(index int, account *Account) {
			defer wg.Done()
			_, errs[index] = api.RefreshIfNeeded(context.Background(), account, executor, 0, true)
		}(i, snapshot)
	}
	wg.Wait()
	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	executor.mu.Lock()
	require.Equal(t, 1, executor.calls)
	executor.mu.Unlock()
}
