//go:build unit

package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type oauthRefreshHandlerRepo struct{ grokCredentialHandlerRepo }

func (r *oauthRefreshHandlerRepo) SetOpenAIOAuthErrorIfCredentialsUnchanged(_ context.Context, id int64, expected map[string]any, message string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.accounts {
		a := &r.accounts[i]
		if a.ID == id && a.Status == service.StatusActive && reflect.DeepEqual(a.Credentials, expected) {
			a.Status, a.Schedulable, a.ErrorMessage = service.StatusError, false, message
			r.setErrorIDs = append(r.setErrorIDs, id)
			return true, nil
		}
	}
	return false, nil
}

type oauthRefreshHandlerExecutor struct{ calls int }

func (*oauthRefreshHandlerExecutor) CanRefresh(*service.Account) bool                  { return true }
func (*oauthRefreshHandlerExecutor) NeedsRefresh(*service.Account, time.Duration) bool { return false }
func (*oauthRefreshHandlerExecutor) CacheKey(*service.Account) string                  { return "openai:test:refresh" }
func (e *oauthRefreshHandlerExecutor) Refresh(context.Context, *service.Account) (map[string]any, error) {
	e.calls++
	return nil, errors.New("refresh_token_invalidated secret-must-not-leak")
}

type oauthRefreshHandlerUpstream struct {
	service.HTTPUpstream
	accountIDs []int64
}

func (u *oauthRefreshHandlerUpstream) Do(_ *http.Request, _ string, id int64, _ int) (*http.Response, error) {
	u.accountIDs = append(u.accountIDs, id)
	if id == 1 {
		return &http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"token_revoked"}}`))}, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_healthy\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"))}, nil
}

func TestResponsesHandlerRefreshRejectionSwitchesAccounts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, passthrough := range []bool{false, true} {
		name := "normal"
		if passthrough {
			name = "passthrough"
		}
		t.Run(name, func(t *testing.T) {
			repo := &oauthRefreshHandlerRepo{}
			for id := int64(1); id <= 2; id++ {
				repo.accounts = append(repo.accounts, service.Account{ID: id, Name: "test-account", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
					Status: service.StatusActive, Schedulable: true, Priority: int(id),
					Extra:       map[string]any{"openai_passthrough": passthrough},
					Credentials: map[string]any{"access_token": "token", "refresh_token": "refresh", "expires_at": time.Now().Add(time.Hour).Format(time.RFC3339)}})
			}
			executor := &oauthRefreshHandlerExecutor{}
			provider := service.NewOpenAITokenProvider(repo, nil, nil)
			provider.SetRefreshAPI(service.NewOAuthRefreshAPI(repo, nil), executor)
			cfg := &config.Config{RunMode: config.RunModeSimple}
			upstream := &oauthRefreshHandlerUpstream{}
			gateway := service.NewOpenAIGatewayService(repo, nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil,
				upstream, nil, provider, nil, nil, nil, nil, nil, nil)
			billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
			t.Cleanup(billing.Stop)
			h := NewOpenAIGatewayHandler(gateway, service.NewConcurrencyService(nil), billing,
				service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg), nil, nil, nil, nil, cfg)
			h.maxAccountSwitches = 10
			c, rec := newOpenAIResponsesFailoverTestContext(t, context.Background())
			h.Responses(c)
			require.Equal(t, []int64{1, 2}, upstream.accountIDs)
			require.Equal(t, []int64{1}, repo.setErrorIDs)
			require.Equal(t, 1, executor.calls)
			require.Equal(t, http.StatusOK, rec.Code)
			require.Contains(t, rec.Body.String(), "resp_healthy")
			require.NotContains(t, rec.Body.String(), "secret-must-not-leak")
			budget := service.OpenAIRetryBudgetFromContext(c)
			require.NotNil(t, budget)
			require.Equal(t, 2, budget.Snapshot().Attempts)
			require.Equal(t, 2, budget.Snapshot().DistinctAccounts)
		})
	}
}
