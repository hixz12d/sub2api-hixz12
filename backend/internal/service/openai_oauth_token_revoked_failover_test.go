//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type openAITokenRevokedRepo struct {
	openAIRefreshFailureRepo
	setErrorCalls int
	tempCalls     int
}

func (r *openAITokenRevokedRepo) SetError(_ context.Context, id int64, msg string) error {
	r.setErrorCalls++
	if r.account != nil && r.account.ID == id {
		r.account.Status = StatusError
		r.account.Schedulable = false
		r.account.ErrorMessage = msg
	}
	return nil
}

func (r *openAITokenRevokedRepo) SetTempUnschedulable(_ context.Context, id int64, _ time.Time, msg string) error {
	r.tempCalls++
	if r.account != nil && r.account.ID == id {
		r.account.Schedulable = false
		r.account.ErrorMessage = msg
	}
	return nil
}

func TestOpenAIForwardTokenRevokedMarksAndSwitchesWithoutRefresh(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.4","input":[{"type":"message","role":"user","content":"hello"}],"stream":true}`)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))

	account := newTestOAuthAccount(2964, nil)
	account.Status, account.Schedulable = StatusActive, true
	account.Credentials = map[string]any{
		"access_token":  "dead",
		"refresh_token": "still-present",
		"expires_at":    time.Now().Add(time.Hour).Format(time.RFC3339),
	}
	repo := &openAITokenRevokedRepo{openAIRefreshFailureRepo: openAIRefreshFailureRepo{account: account}}
	cache := newOpenAITokenCacheStub()
	cache.tokens[OpenAITokenCacheKey(account)] = "dead"
	executor := &openAIRefreshFailureExecutor{}
	provider := NewOpenAITokenProvider(repo, cache, nil)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), executor)

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusUnauthorized,
			Header:     http.Header{},
			Body: io.NopCloser(strings.NewReader(
				`{"error":{"message":"Encountered invalidated oauth token for user, failing request","type":null,"code":"token_revoked","param":null},"status":401}`,
			)),
		},
	}}
	rl := NewRateLimitService(repo, nil, &config.Config{RateLimit: config.RateLimitConfig{OAuth401CooldownMinutes: 10}}, nil, nil)
	svc := &OpenAIGatewayService{
		cfg:                 &config.Config{},
		accountRepo:         repo,
		openAITokenProvider: provider,
		httpUpstream:        upstream,
		rateLimitService:    rl,
	}

	_, err := svc.Forward(context.Background(), c, account, body)
	var failure *UpstreamFailoverError
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ShouldRetryNextAccount(), "token_revoked must switch accounts immediately")
	require.False(t, failure.RetryableOnSameAccount)
	require.Equal(t, 0, executor.calls, "permanent revoke must not call refresh")
	require.Equal(t, 1, repo.setErrorCalls, "token_revoked must SetError immediately")
	require.Equal(t, StatusError, account.Status)
	require.False(t, account.Schedulable)
}

func TestIsOpenAIPermanentOAuthUnauthorized(t *testing.T) {
	body := []byte(`{"error":{"code":"token_revoked","message":"Encountered invalidated oauth token for user, failing request"}}`)
	require.True(t, isOpenAIPermanentOAuthUnauthorized(http.StatusUnauthorized, body))
	require.False(t, isOpenAIPermanentOAuthUnauthorized(http.StatusUnauthorized, []byte(`{"error":{"code":"token_expired"}}`)))
}
