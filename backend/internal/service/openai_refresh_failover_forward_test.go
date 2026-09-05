//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIForwardRefreshFailureCanSwitchWithinOriginalBudget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, passthrough := range []bool{false, true} {
		name := "normal"
		if passthrough {
			name = "passthrough"
		}
		t.Run(name, func(t *testing.T) {
			body := []byte(`{"model":"gpt-5.4","input":"hello","stream":true}`)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
			account := newTestOAuthAccount(11, map[string]any{"openai_passthrough": passthrough})
			account.Status, account.Schedulable = StatusActive, true
			account.Credentials = map[string]any{"access_token": "old", "refresh_token": "refresh", "expires_at": time.Now().Add(time.Hour).Format(time.RFC3339)}
			repo := &openAIRefreshFailureRepo{account: account}
			cache := newOpenAITokenCacheStub()
			cache.tokens[OpenAITokenCacheKey(account)] = "old"
			executor := &openAIRefreshFailureExecutor{err: errors.New("refresh_token_invalidated")}
			provider := NewOpenAITokenProvider(repo, cache, nil)
			provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), executor)
			upstream := &httpUpstreamRecorder{responses: []*http.Response{
				{StatusCode: http.StatusUnauthorized, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"token_revoked"}}`))},
				{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_healthy\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"))},
			}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, accountRepo: repo, openAITokenProvider: provider, httpUpstream: upstream}
			_, err := svc.Forward(context.Background(), c, account, body)
			var failure *UpstreamFailoverError
			require.ErrorAs(t, err, &failure)
			require.Equal(t, OpenAIOAuthRefreshFailedReason, failure.Reason)
			require.True(t, failure.ShouldRetryNextAccount())
			require.False(t, c.Writer.Written())
			require.Equal(t, StatusError, account.Status)
			require.Len(t, upstream.requests, 1)
			budget := OpenAIRetryBudgetFromContext(c)
			require.Equal(t, 1, budget.Snapshot().Attempts)
			require.True(t, budget.Snapshot().RefreshUsed)

			healthy := newTestOAuthAccount(12, maps.Clone(account.Extra))
			healthy.Status, healthy.Schedulable = StatusActive, true
			healthy.Credentials = map[string]any{"access_token": "healthy", "refresh_token": "healthy-refresh"}
			cache.tokens[OpenAITokenCacheKey(healthy)] = "healthy"
			require.NoError(t, svc.ResetForAccountSwitch(c.Request.Context(), c, healthy.ID))
			result, err := svc.Forward(c.Request.Context(), c, healthy, body)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, "resp_healthy", result.ResponseID)
			require.Len(t, upstream.requests, 2)
			require.Equal(t, "Bearer old", upstream.requests[0].Header.Get("Authorization"))
			require.Equal(t, "Bearer healthy", upstream.requests[1].Header.Get("Authorization"))
			require.Same(t, budget, OpenAIRetryBudgetFromContext(c))
			require.Equal(t, 2, budget.Snapshot().Attempts)
			require.Equal(t, 2, budget.Snapshot().DistinctAccounts)
			require.Equal(t, 1, executor.calls)
			require.ErrorIs(t, budget.Reserve(13), ErrOpenAIRetryBudgetExhausted)
		})
	}
}
