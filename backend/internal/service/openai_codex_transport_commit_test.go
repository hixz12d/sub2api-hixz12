package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIWSTransportScopeParticipatesInClientAndHandshakeKeys(t *testing.T) {
	profileA := tlsfingerprint.BuiltinChromeAutoProfile()
	profileA.CacheScopeKey = "account-1-credential-a-profile-cli"
	profileB := tlsfingerprint.BuiltinChromeAutoProfile()
	profileB.CacheScopeKey = "account-1-credential-b-profile-cli"
	require.NotEqual(t, openAIWSHTTPClientCacheKey("", profileA), openAIWSHTTPClientCacheKey("", profileB))

	headers := http.Header{"OpenAI-Beta": {"responses_websockets=2026-02-06"}}
	first := normalizeOpenAIWSHandshakeCompatibilityForAccount(nil, headers, "session", "proxy", profileA.CacheScopeKey)
	second := normalizeOpenAIWSHandshakeCompatibilityForAccount(nil, headers, "session", "proxy", profileB.CacheScopeKey)
	require.NotEqual(t, first, second)
}

func TestCodexCommitGuardAllowsHeartbeatButBlocksSemanticOutputAndOwnership(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	account := newTestOAuthAccount(501, nil)
	PrepareOpenAIRetryBudget(c, []byte(`{"model":"gpt-5"}`))
	EnsureOpenAIRetryBudget(c, account, []byte(`{"model":"gpt-5"}`))
	guard := NewCodexCommitGuard(c)

	require.NoError(t, guard.CanStartAttempt(account.ID))
	guard.MarkHeartbeat()
	require.NoError(t, guard.CanStartAttempt(account.ID), "transport-only heartbeat remains replayable")
	guard.MarkSemanticOutput()
	require.ErrorContains(t, guard.CanStartAttempt(account.ID), "semantic output")

	c2, _ := gin.CreateTestContext(httptest.NewRecorder())
	c2.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	PrepareOpenAIRetryBudget(c2, []byte(`{"model":"gpt-5"}`))
	EnsureOpenAIRetryBudget(c2, account, []byte(`{"model":"gpt-5"}`))
	TrackOpenAIResponseID(c2, "resp_owned")
	require.ErrorContains(t, NewCodexCommitGuard(c2).CanStartAttempt(account.ID), "response ownership")
}

func TestCodexCommitGuardBlocksStatefulCrossAccountRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	body := []byte(`{"model":"gpt-5","previous_response_id":"resp_owned"}`)
	account := newTestOAuthAccount(601, nil)
	PrepareOpenAIRetryBudget(c, body)
	EnsureOpenAIRetryBudget(c, account, body)

	guard := NewCodexCommitGuard(c)
	require.NoError(t, guard.CanStartAttempt(account.ID))
	require.ErrorContains(t, guard.CanStartAttempt(602), "stateful request cannot switch accounts")
}

func TestRelayKernelHTTPUsesProfiledTLSExactlyOnce(t *testing.T) {
	plan := mustCodexPlanForTest(t, "transport-logical", "transport-conversation", CodexTransportHTTP, time.Unix(1_800_000_000, 0))
	state, err := FinalizeCodexAttempt(plan, CodexAttemptInput{
		AccountID:         701,
		CredentialVersion: "credential-v1",
		ProfileID:         CodexProfileCLI,
		FingerprintMode:   string(codexFingerprintSession),
	}, testCodexRelaySecret)
	require.NoError(t, err)
	upstream := &codexTransportCountingUpstream{}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	account := newTestOAuthAccount(701, nil)
	req := httptest.NewRequest(http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", nil)
	req = req.WithContext(ContextWithCodexAttemptState(req.Context(), state))

	_, err = svc.doOpenAIUpstream(req, "", account)
	require.Error(t, err)
	require.Zero(t, upstream.plainCalls.Load())
	require.Equal(t, int32(1), upstream.tlsCalls.Load())
}

type codexTransportCountingUpstream struct {
	plainCalls atomic.Int32
	tlsCalls   atomic.Int32
}

func (u *codexTransportCountingUpstream) Do(*http.Request, string, int64, int) (*http.Response, error) {
	u.plainCalls.Add(1)
	return nil, errors.New("stop")
}

func (u *codexTransportCountingUpstream) DoWithTLS(*http.Request, string, int64, int, *tlsfingerprint.Profile) (*http.Response, error) {
	u.tlsCalls.Add(1)
	return nil, errors.New("stop")
}
