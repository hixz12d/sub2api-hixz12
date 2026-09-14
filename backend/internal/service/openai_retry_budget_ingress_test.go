package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIRetryBudgetLegacySlowIngressAllowsInitialAttempt(t *testing.T) {
	for _, mode := range []string{"default", "empty", "legacy"} {
		for _, path := range []string{"/v1/responses", "/v1/messages"} {
			t.Run(mode+path, func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, path, nil)
				// A large conversation took longer to upload than the legacy
				// retry window, but no upstream request has been sent yet.
				ingress := time.Now().Add(-time.Minute)
				RecordOpenAILogicalStart(c, ingress)
				var cfg *config.Config
				if mode != "default" {
					cfg = &config.Config{}
					if mode == "legacy" {
						cfg.Gateway.OpenAIPreoutputRecoveryMode = mode
					}
				}
				body := []byte(`{"model":"gpt-5","input":"hello","stream":true}`)
				account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
				budget := PrepareOpenAIRetryBudgetWithConfig(c, body, cfg)
				require.Same(t, budget, EnsureOpenAIRetryBudget(c, account, body))
				require.NoError(t, ReserveOpenAIUpstreamAttempt(c, account.ID))
				require.Equal(t, 1, budget.Snapshot().Attempts)
				require.Equal(t, 20*time.Second, budget.Snapshot().MaxElapsed)

				// Re-entering preparation must not give retries a fresh window.
				budget.startedAt = time.Now().Add(-21 * time.Second)
				started := budget.Snapshot().StartedAt
				require.Same(t, budget, PrepareOpenAIRetryBudgetWithConfig(c, body, cfg))
				require.Same(t, budget, EnsureOpenAIRetryBudget(c, account, body))
				require.Equal(t, started, budget.Snapshot().StartedAt)
				require.ErrorIs(t, ReserveOpenAIUpstreamAttempt(c, account.ID), ErrOpenAIRetryBudgetExhausted)
				require.Equal(t, 1, budget.Snapshot().Attempts)
			})
		}
	}
}

func TestOpenAIRetryBudgetBoundedIngressStillLimitsInitialAttempt(t *testing.T) {
	for _, path := range []string{"/v1/responses", "/v1/messages", "/v1/responses/compact"} {
		t.Run(path, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, path, nil)
			ingress := time.Now().Add(-111 * time.Second)
			RecordOpenAILogicalStart(c, ingress)
			cfg := &config.Config{Gateway: config.GatewayConfig{OpenAIPreoutputRecoveryMode: "bounded_preoutput"}}
			body := []byte(`{"model":"gpt-5","input":"hello","stream":true}`)
			account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
			budget := PrepareOpenAIRetryBudgetWithConfig(c, body, cfg)
			require.Same(t, budget, EnsureOpenAIRetryBudget(c, account, body))
			require.Equal(t, ingress, budget.Snapshot().StartedAt)
			require.ErrorIs(t, ReserveOpenAIUpstreamAttempt(c, account.ID), ErrOpenAIRetryBudgetExhausted)
			require.Zero(t, budget.Snapshot().Attempts)
		})
	}
}
