//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type blueprintBlockingTokenCache struct {
	OpenAITokenCache
	entered bool
}

func (c *blueprintBlockingTokenCache) GetAccessToken(ctx context.Context, _ string) (string, error) {
	c.entered = true
	<-ctx.Done()
	return "", ctx.Err()
}

type blueprintUndispatchedUpstream struct {
	HTTPUpstream
	calls atomic.Int32
}

func (u *blueprintUndispatchedUpstream) Do(*http.Request, string, int64, int) (*http.Response, error) {
	u.calls.Add(1)
	return nil, errors.New("unexpected dispatch after preparation deadline")
}

func TestBlueprintV2CredentialPreparationRecoveryDeadline(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []string{"/v1/responses", "/v1/messages"} {
		t.Run(endpoint, func(t *testing.T) {
			cfg := &config.Config{Gateway: config.GatewayConfig{OpenAIPreoutputRecoveryMode: "bounded_preoutput", OpenAIPreoutputRecoveryMaxElapsedSeconds: 1}}
			cache := &blueprintBlockingTokenCache{}
			upstream := &blueprintUndispatchedUpstream{}
			svc := &OpenAIGatewayService{cfg: cfg, openAITokenProvider: NewOpenAITokenProvider(nil, cache, nil), httpUpstream: upstream}
			account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "fixture", "expires_at": time.Now().Add(time.Hour).Format(time.RFC3339)}}
			body := []byte(`{"model":"gpt-5.4","stream":true,"input":"hello"}`)
			if endpoint == "/v1/messages" {
				body = []byte(`{"model":"gpt-5.4","stream":true,"max_tokens":16,"messages":[{"role":"user","content":"hello"}]}`)
			}
			parent, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(string(body))).WithContext(parent)
			RecordOpenAILogicalStart(c, time.Now().Add(-500*time.Millisecond))
			budget := PrepareOpenAIRetryBudgetWithConfig(c, body, cfg)
			started := time.Now()
			var err error
			if endpoint == "/v1/messages" {
				_, err = svc.ForwardAsAnthropic(parent, c, account, body, "", "")
			} else {
				_, err = svc.Forward(parent, c, account, body)
			}
			require.ErrorIs(t, err, ErrOpenAIRecoveryDeadline)
			var failure *UpstreamFailoverError
			require.ErrorAs(t, err, &failure)
			require.Equal(t, http.StatusGatewayTimeout, failure.StatusCode)
			require.True(t, cache.entered, "must block inside the real token-provider path")
			require.Less(t, time.Since(started), 1500*time.Millisecond)
			require.Zero(t, upstream.calls.Load())
			require.Zero(t, budget.Snapshot().Attempts)
			require.Empty(t, rec.Body.String())
			require.NoError(t, parent.Err())
		})
	}
}

func TestBlueprintV2PreparationCeilingAndScope(t *testing.T) {
	cfg := &config.Config{Gateway: config.GatewayConfig{OpenAIPreoutputRecoveryMode: "bounded_preoutput", OpenAIPreoutputRecoveryMaxElapsedSeconds: 1, OpenAIPreoutputRecoveryHighEffortMaxElapsedSeconds: 3}}
	started := time.Now().Add(-1500 * time.Millisecond)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	RecordOpenAILogicalStart(c, started)
	PrepareOpenAIRetryBudgetWithConfig(c, []byte(`{"stream":true,"input":"hello"}`), cfg)
	parent := context.Background()
	for _, account := range []*Account{
		{Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
		{Platform: PlatformAnthropic, Type: AccountTypeAPIKey},
		{Platform: PlatformGemini, Type: AccountTypeAPIKey},
	} {
		ctx, release := OpenAIRecoveryPreparationContext(c, parent, account)
		require.Equal(t, parent, ctx)
		release()
	}
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	ctx, release := OpenAIRecoveryPreparationContext(c, parent, account)
	defer release()
	deadline, ok := ctx.Deadline()
	require.True(t, ok)
	require.Equal(t, started.Add(3*time.Second), deadline)
	require.NoError(t, ctx.Err())
	svc := &OpenAIGatewayService{cfg: cfg}
	_, guard, err := svc.beginOpenAIHTTPOutputPhase(parent, c, account, "low")
	require.ErrorIs(t, err, ErrOpenAIRecoveryDeadline)
	require.Nil(t, guard)
}
