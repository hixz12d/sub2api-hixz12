//go:build unit

package service

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type openAIRefreshFailureRepo struct {
	AccountRepository
	account   *Account
	attempted map[string]any
	casCalls  int
	casErr    error
	beforeCAS func()
}

func (r *openAIRefreshFailureRepo) GetByID(context.Context, int64) (*Account, error) {
	copy := *r.account
	copy.Credentials = maps.Clone(r.account.Credentials)
	return &copy, nil
}

func (r *openAIRefreshFailureRepo) SetOpenAIOAuthErrorIfCredentialsUnchanged(_ context.Context, _ int64, expected map[string]any, _ string) (bool, error) {
	r.casCalls++
	r.attempted = maps.Clone(expected)
	if r.beforeCAS != nil {
		r.beforeCAS()
	}
	if r.casErr != nil {
		return false, r.casErr
	}
	if r.account.Status != StatusActive || !reflect.DeepEqual(r.account.Credentials, expected) {
		return false, nil
	}
	r.account.Status = StatusError
	r.account.Schedulable = false
	return true, nil
}

type openAIRefreshFailureExecutor struct {
	err   error
	calls int
}

func (e *openAIRefreshFailureExecutor) CanRefresh(*Account) bool                  { return true }
func (e *openAIRefreshFailureExecutor) NeedsRefresh(*Account, time.Duration) bool { return false }
func (e *openAIRefreshFailureExecutor) CacheKey(a *Account) string                { return OpenAITokenCacheKey(a) }
func (e *openAIRefreshFailureExecutor) Refresh(context.Context, *Account) (map[string]any, error) {
	e.calls++
	return nil, e.err
}

func TestOpenAIForcedRefreshQuarantineIsConditional(t *testing.T) {
	for _, tc := range []struct {
		name        string
		refreshErr  error
		reauthorize bool
		casErr      error
		quarantined bool
	}{
		{"invalidated", errors.New("refresh_token_invalidated"), false, nil, true},
		{"invalid_grant", errors.New("invalid_grant"), false, nil, true},
		{"concurrent_reauthorization", errors.New("refresh_token_invalidated"), true, nil, false},
		{"temporary", errors.New("token refresh failed: status 503"), false, nil, false},
		{"provider_config", errors.New("invalid_client"), false, nil, false},
		{"persistence_failure", errors.New("refresh_token_invalidated"), false, errors.New("database unavailable"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := &Account{ID: 77, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true,
				Credentials: map[string]any{"access_token": "old", "refresh_token": "refresh", "_token_version": int64(1)}}
			repo := &openAIRefreshFailureRepo{account: account, casErr: tc.casErr}
			cache := newOpenAITokenCacheStub()
			cache.tokens[OpenAITokenCacheKey(account)] = "old"
			if tc.reauthorize {
				repo.beforeCAS = func() {
					account.Credentials = map[string]any{"access_token": "new", "refresh_token": "new-refresh", "_token_version": int64(2)}
					cache.tokens[OpenAITokenCacheKey(account)] = "new"
				}
			}
			executor := &openAIRefreshFailureExecutor{err: tc.refreshErr}
			provider := NewOpenAITokenProvider(repo, cache, nil)
			provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), executor)
			token, err := provider.RefreshAfterUnauthorized(context.Background(), account, "old")
			require.ErrorIs(t, err, tc.refreshErr)
			require.Empty(t, token)
			require.Equal(t, 1, executor.calls)
			require.Equal(t, tc.quarantined, account.Status == StatusError)
			require.Equal(t, !tc.quarantined, account.Schedulable)
			cached, err := cache.GetAccessToken(context.Background(), OpenAITokenCacheKey(account))
			require.NoError(t, err)
			if tc.quarantined {
				require.Empty(t, cached)
			}
			if tc.reauthorize {
				require.Equal(t, "new", cached)
				require.Equal(t, "old", repo.attempted["access_token"])
			}
			if !openAIPermanentRefreshRejection(tc.refreshErr) {
				require.Zero(t, repo.casCalls)
			}
		})
	}
}

func TestOpenAIRefreshFailurePreservesRetryAndOutputBoundaries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name                                            string
		stateful, output, exhausted, provider, canceled bool
		retry                                           bool
	}{
		{name: "replayable", retry: true},
		{name: "stateful", stateful: true},
		{name: "output", output: true},
		{name: "budget_exhausted", exhausted: true},
		{name: "provider_config", provider: true},
		{name: "client_canceled", canceled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
			body := []byte(`{"input":"hello"}`)
			if tc.stateful {
				body = []byte(`{"previous_response_id":"resp_old"}`)
			}
			account := &Account{ID: 11, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
			budget := EnsureOpenAIRetryBudget(c, account, body)
			require.NoError(t, budget.Reserve(11))
			RecordOpenAIRetryFailure(c, http.StatusUnauthorized, nil)
			require.True(t, budget.UseRefresh())
			if tc.output {
				MarkOpenAISemanticOutputStarted(c)
				budget.MarkBytesEmitted()
			}
			if tc.exhausted {
				require.NoError(t, budget.Reserve(11))
			}
			if tc.canceled {
				cancel()
			}
			refreshErr := errors.New("refresh_token_invalidated secret-token")
			if tc.provider {
				refreshErr = errors.New("invalid_client secret-token")
			}
			err := (&OpenAIGatewayService{}).handleOpenAIRefreshFailure(ctx, c, account, refreshErr, false)
			if tc.canceled {
				require.ErrorIs(t, err, context.Canceled)
				return
			}
			var failure *UpstreamFailoverError
			require.ErrorAs(t, err, &failure)
			require.Equal(t, tc.retry, failure.ShouldRetryNextAccount())
			require.False(t, failure.RetryableOnSameAccount)
			require.NotContains(t, failure.ClientMessage, "secret-token")
			require.Empty(t, failure.ResponseBody)
			if tc.stateful {
				require.Equal(t, OpenAIConversationRecoveryRequiredReason, failure.Reason)
			}
			if tc.provider {
				require.Equal(t, GatewayFailureScopeProvider, failure.Scope)
			}
			err = budget.Reserve(12)
			if tc.retry {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrOpenAIRetryBudgetExhausted)
			}
			require.ErrorIs(t, budget.Reserve(11), ErrOpenAIRetryBudgetExhausted)
			require.False(t, c.Writer.Written())
		})
	}
}

func TestOpenAIRetryStateDetectionUsesJSONStructure(t *testing.T) {
	for _, body := range []string{
		"{\"input\":[{\"type\" :\n\t\"function_call_output\",\"output\":\"ok\"}]}",
		`{"input":[{"ty\u0070e":"function_call_output"}]}`,
		`{"input":[{"encrypted_content":"secret"}]}`,
		`{"input":[{"type":"custom_tool_call_output","output":"done"}]}`,
		`{"input":[{"type":"tool_search_output","output":"done"}]}`,
		`{"input":[{"type":"item_reference","id":"msg_old"}]}`,
		`{"input":[{"type":"mcp_approval_response","approve":true}]}`,
		`[]`,
	} {
		require.True(t, openAIRetryRequestIsStateful([]byte(body)), body)
	}
	require.False(t, openAIRetryRequestIsStateful([]byte(`{"input":"hello"}`)))
}
