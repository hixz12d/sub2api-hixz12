//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAICredentialPoolUnavailableDetection(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	for _, body := range []string{
		`{"error":{"message":"No healthy OpenAI OAuth credential is currently available"}}`,
		`{"detail":"No healthy OpenAI OAuth credential is currently available."}`,
		`{"message":"NO HEALTHY OPENAI OAUTH CREDENTIAL IS CURRENTLY AVAILABLE"}`,
		"No healthy OpenAI OAuth credential is currently available",
	} {
		require.True(t, isOpenAICredentialPoolUnavailable(account, 503, []byte(body)))
	}
	require.False(t, isOpenAICredentialPoolUnavailable(account, 503, []byte(`{"error":{"message":"Server overloaded"}}`)))
	require.False(t, isOpenAICredentialPoolUnavailable(account, 400, []byte(openAICredentialPoolUnavailableMessage)))
	require.False(t, isOpenAICredentialPoolUnavailable(account, 503, []byte(`{"prompt":"No healthy OpenAI OAuth credential is currently available"}`)))
	require.False(t, isOpenAICredentialPoolUnavailable(&Account{Platform: PlatformAnthropic}, 503, []byte(openAICredentialPoolUnavailableMessage)))
}

func TestOpenAICredentialPoolUnavailableStopsAllModels(t *testing.T) {
	for _, credentials := range []map[string]any{
		{}, {"pool_mode": true}, {"custom_error_codes_enabled": true, "custom_error_codes": []any{float64(401)}},
	} {
		repo := &rateLimitAccountRepoStub{}
		limits := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
		svc := &OpenAIGatewayService{rateLimitService: limits}
		limits.SetAccountRuntimeBlocker(svc)
		account := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: credentials}
		body := []byte(`{"error":{"message":"No healthy OpenAI OAuth credential is currently available"}}`)
		require.True(t, svc.handleOpenAIAccountUpstreamError(context.Background(), account, http.StatusServiceUnavailable, nil, body, "gpt-5.4"))
		require.Equal(t, 1, repo.tempCalls)
		require.Zero(t, repo.setErrorCalls, "recoverable upstream pool failure must not permanently disable the account")
		require.Contains(t, repo.lastTempReason, openAICredentialPoolUnavailableMessage)
		require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
		until, ok := svc.openaiAccountRuntimeBlockUntil.Load(account.ID)
		require.True(t, ok)
		require.WithinDuration(t, time.Now().Add(5*time.Minute), until.(time.Time), time.Second)
		failover := svc.newOpenAIAccountFailoverError(account, 503, nil, body, "unavailable", true, true)
		require.False(t, failover.RetryableOnSameAccount)
	}
}

func TestOpenAICredentialPoolUnavailablePolicyAndPersistenceFailure(t *testing.T) {
	repo := &rateLimitAccountRepoStub{tempErr: errors.New("database unavailable")}
	limits := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	svc := &OpenAIGatewayService{rateLimitService: limits}
	limits.SetAccountRuntimeBlocker(svc)
	account := &Account{ID: 47, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"pool_mode": true}}
	body := []byte(openAICredentialPoolUnavailableMessage)
	require.Equal(t, ErrorPolicyTempUnscheduled, limits.CheckErrorPolicy(context.Background(), account, 503, body, "gpt-5.4"))
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
	require.True(t, limits.HandleUpstreamError(context.Background(), account, 503, nil, body, "gpt-5.4"))
	require.Equal(t, 2, repo.tempCalls)
	require.Zero(t, repo.setErrorCalls)
}
