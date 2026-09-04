package service

import (
	"context"
	"maps"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCodexClientProfileCatalogValidatesAndReturnsCopies(t *testing.T) {
	profiles := CodexClientProfiles()
	require.Len(t, profiles, 6)
	for _, profile := range profiles {
		require.NoError(t, ValidateCodexClientProfile(profile), profile.ID)
	}

	profile, err := ResolveCodexClientProfile(CodexProfileCLI)
	require.NoError(t, err)
	require.True(t, profile.Supports(CodexCapabilityWebSocket))
	require.Equal(t, CodexProfileFidelityDegraded, profile.Fidelity)
	profile.Transport.HeaderOrder[0] = "mutated"

	fresh, err := ResolveCodexClientProfile(CodexProfileCLI)
	require.NoError(t, err)
	require.Equal(t, "host", fresh.Transport.HeaderOrder[0])
	require.Equal(t, tlsfingerprint.HelloPresetChromeAuto, fresh.Transport.TLSProfileID)
	require.Equal(t, tlsfingerprint.ChromeHTTP2ProfileID, fresh.Transport.HTTP2ProfileID)
	require.Equal(t, tlsfingerprint.ChromeHTTP2HeaderOrder(), fresh.Transport.HeaderOrder)
}

func TestCodexAutoProfileSelectsClaimedClientAndDefaultsToPassthrough(t *testing.T) {
	tests := []struct {
		name       string
		originator string
		userAgent  string
		want       string
	}{
		{name: "exec", originator: "codex_exec", want: CodexProfileExec},
		{name: "desktop", userAgent: "codex-desktop/1.0", want: CodexProfileDesktop},
		{name: "opencode", userAgent: "opencode/1.0", want: CodexProfileOpenCode},
		{name: "pi", userAgent: "pi-coding-agent/1.0", want: CodexProfilePi},
		{name: "cli", originator: "codex_cli_rs", want: CodexProfileCLI},
		{name: "unknown", userAgent: "custom-client/1.0", want: CodexProfilePassthrough},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := make(http.Header)
			header.Set("originator", tt.originator)
			header.Set("user-agent", tt.userAgent)
			profile, err := ResolveCodexClientProfileForRequest(CodexProfileAuto, header)
			require.NoError(t, err)
			require.Equal(t, tt.want, profile.ID)
		})
	}
}

func TestValidateCodexClientProfileRejectsInconsistentIdentityAndCapabilities(t *testing.T) {
	profile, err := ResolveCodexClientProfile(CodexProfileExec)
	require.NoError(t, err)
	profile.App.Originator = "codex_cli_rs"
	require.ErrorContains(t, ValidateCodexClientProfile(profile), "inconsistent")

	piProfile, err := ResolveCodexClientProfile(CodexProfilePi)
	require.NoError(t, err)
	piProfile.Capabilities |= CodexCapabilityWebSocket
	require.ErrorContains(t, ValidateCodexClientProfile(piProfile), "cannot claim WebSocket")
}

func TestResolveCodexRelaySettingsRequiresV2ForKernel(t *testing.T) {
	legacy, err := ResolveCodexRelaySettings(nil)
	require.NoError(t, err)
	require.Equal(t, CodexRelayModeLegacy, legacy.Mode)
	require.Equal(t, CodexIdentityPolicyV1, legacy.PolicyVersion)
	require.Equal(t, CodexProfileCLI, legacy.ProfileID)

	account := newTestOAuthAccount(920, map[string]any{
		CodexRelayModeExtraKey:             string(CodexRelayModeKernel),
		CodexIdentityPolicyVersionExtraKey: CodexIdentityPolicyV1,
	})
	_, err = ResolveCodexRelaySettings(account)
	require.ErrorContains(t, err, "requires codex identity policy v2")

	account.Extra[CodexIdentityPolicyVersionExtraKey] = CodexIdentityPolicyV2
	account.Extra[CodexClientProfileExtraKey] = CodexProfileOpenCode
	settings, err := ResolveCodexRelaySettings(account)
	require.NoError(t, err)
	require.Equal(t, CodexProfileOpenCode, settings.ProfileID)
}

func TestValidateCodexRelayAccountExtra(t *testing.T) {
	valid := map[string]any{
		CodexRelayModeExtraKey:             string(CodexRelayModeKernel),
		CodexIdentityPolicyVersionExtraKey: CodexIdentityPolicyV2,
		CodexClientProfileExtraKey:         CodexProfileAuto,
		CodexRelayShadowEnabledExtraKey:    false,
		codexFingerprintModeExtraKey:       string(codexFingerprintSession),
	}
	require.NoError(t, ValidateCodexRelayAccountExtra(PlatformOpenAI, AccountTypeOAuth, valid, testCodexRelaySecret))
	require.ErrorContains(t, ValidateCodexRelayAccountExtra(PlatformAnthropic, AccountTypeOAuth, valid, testCodexRelaySecret), "OpenAI OAuth")
	require.ErrorContains(t, ValidateCodexRelayAccountExtra(PlatformOpenAI, AccountTypeOAuth, valid, "short"), "at least")

	invalidProfile := maps.Clone(valid)
	invalidProfile[CodexClientProfileExtraKey] = "invented-client"
	require.ErrorContains(t, ValidateCodexRelayAccountExtra(PlatformOpenAI, AccountTypeOAuth, invalidProfile, testCodexRelaySecret), "unknown codex client profile")

	invalidShadow := maps.Clone(valid)
	invalidShadow[CodexRelayShadowEnabledExtraKey] = "true"
	require.ErrorContains(t, ValidateCodexRelayAccountExtra(PlatformOpenAI, AccountTypeOAuth, invalidShadow, testCodexRelaySecret), "must be a boolean")

	missingIdentity := maps.Clone(valid)
	missingIdentity[codexFingerprintModeExtraKey] = string(codexFingerprintOff)
	require.ErrorContains(t, ValidateCodexRelayAccountExtra(PlatformOpenAI, AccountTypeOAuth, missingIdentity, testCodexRelaySecret), "requires a managed")
}

func TestValidateCodexRelayAccountExtraIgnoresUnconfiguredAccounts(t *testing.T) {
	require.NoError(t, ValidateCodexRelayAccountExtra(PlatformAnthropic, AccountTypeAPIKey, nil, testCodexRelaySecret))
	require.NoError(t, ValidateCodexRelayAccountExtra(PlatformOpenAI, AccountTypeAPIKey, map[string]any{"foo": "bar"}, testCodexRelaySecret))
}

func TestCodexRelaySensitiveExtraIsNotPersistable(t *testing.T) {
	input := map[string]any{
		CodexRelayModeExtraKey:             string(CodexRelayModeLegacy),
		"codex_relay_secret":               "do-not-store",
		"codex_identity_derivation_secret": "do-not-store",
		"codex_identity_transport_key":     "do-not-store",
	}
	sanitized := sanitizedCodexFingerprintExtraUpdates(input)
	require.Equal(t, string(CodexRelayModeLegacy), sanitized[CodexRelayModeExtraKey])
	for _, key := range codexRelaySensitiveExtraKeys {
		require.NotContains(t, sanitized, key)
	}
}

func TestRelayKernelFinalizerStagesV2AttemptWithoutChangingLegacyDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	cfg.Gateway.OpenAIAffinity.Secret = testCodexRelaySecret
	registry := &codexRegistryGatewayCacheStub{}
	svc := &OpenAIGatewayService{cfg: cfg, cache: registry}
	account := newTestOAuthAccount(921, map[string]any{
		CodexRelayModeExtraKey:             string(CodexRelayModeKernel),
		CodexIdentityPolicyVersionExtraKey: CodexIdentityPolicyV2,
		CodexClientProfileExtraKey:         CodexProfileExec,
		codexFingerprintModeExtraKey:       string(codexFingerprintWindow40),
	})
	account.Credentials = map[string]any{"access_token": "secret-token"}
	account.UpdatedAt = time.Unix(1_800_000_000, 0)

	plan := mustCodexPlanForTest(t, "logical-kernel", "conversation-kernel", CodexTransportHTTP, time.Unix(1_800_000_000, 0))
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	req = req.WithContext(ContextWithCodexRequestPlan(req.Context(), plan))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	snapshot, err := svc.finalizeCodexOAuthIdentity(account, c, req.Header, "")
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	state, ok := CodexAttemptStateFromContext(c.Request.Context())
	require.True(t, ok)
	require.Equal(t, CodexIdentityPolicyV2, state.PolicyVersion())
	require.Equal(t, CodexProfileExec, state.Profile().ID)
	require.Equal(t, snapshot, state.Identity())

	headers := make(http.Header)
	svc.finalizeCodexOAuthHeaders(c.Request.Context(), c, account, headers, snapshot, "")
	require.Equal(t, state.Profile().App.UserAgent, headers.Get("User-Agent"))
	require.Equal(t, state.Profile().App.Originator, headers.Get("originator"))
	require.Equal(t, state.Profile().App.Version, headers.Get("x-openai-client-version"))
	require.NoError(t, svc.CommitCodexConversation(c.Request.Context()))
	committed, err := registry.GetCodexConversation(c.Request.Context(), plan.ConversationDigest())
	require.NoError(t, err)
	require.True(t, committed.Committed)

	otherAccount := newTestOAuthAccount(923, account.Extra)
	otherAccount.Credentials = map[string]any{"access_token": "other-secret-token"}
	otherAccount.UpdatedAt = account.UpdatedAt
	otherRequest := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	otherRequest = otherRequest.WithContext(ContextWithCodexRequestPlan(otherRequest.Context(), plan))
	otherContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	otherContext.Request = otherRequest
	_, err = svc.finalizeCodexOAuthIdentity(otherAccount, otherContext, otherRequest.Header, "")
	require.ErrorContains(t, err, "bound to account")
}

func TestRelayKernelAllowsCASFailoverBeforeSemanticCommit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	cfg.Gateway.OpenAIAffinity.Secret = testCodexRelaySecret
	registry := &codexRegistryGatewayCacheStub{}
	svc := &OpenAIGatewayService{cfg: cfg, cache: registry}
	extra := map[string]any{
		CodexRelayModeExtraKey:             string(CodexRelayModeKernel),
		CodexIdentityPolicyVersionExtraKey: CodexIdentityPolicyV2,
		CodexClientProfileExtraKey:         CodexProfileCLI,
		codexFingerprintModeExtraKey:       string(codexFingerprintSession),
	}
	plan := mustCodexPlanForTest(t, "logical-failover", "conversation-failover", CodexTransportHTTP, time.Unix(1_800_000_000, 0))
	finalize := func(accountID int64, token string) *CodexAttemptState {
		account := newTestOAuthAccount(accountID, maps.Clone(extra))
		account.Credentials = map[string]any{"access_token": token}
		req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		req = req.WithContext(ContextWithCodexRequestPlan(req.Context(), plan))
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = req
		_, err := svc.finalizeCodexOAuthIdentity(account, c, req.Header, "")
		require.NoError(t, err)
		state, ok := CodexAttemptStateFromContext(c.Request.Context())
		require.True(t, ok)
		return state
	}

	first := finalize(931, "first-token")
	second := finalize(932, "second-token")
	require.Equal(t, int64(931), first.AccountID())
	require.Equal(t, int64(932), second.AccountID())
	stored, err := registry.GetCodexConversation(context.Background(), plan.ConversationDigest())
	require.NoError(t, err)
	require.Equal(t, int64(932), stored.AccountID)
	require.False(t, stored.Committed)
}

func TestRelayKernelFinalizerFailsClosedWithoutPlanOrSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := newTestOAuthAccount(922, map[string]any{
		CodexRelayModeExtraKey:             string(CodexRelayModeKernel),
		CodexIdentityPolicyVersionExtraKey: CodexIdentityPolicyV2,
	})
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	_, err := svc.finalizeCodexOAuthIdentity(account, c, nil, "")
	require.ErrorContains(t, err, "openai_affinity.secret")

	svc.cfg.Gateway.OpenAIAffinity.Secret = testCodexRelaySecret
	_, err = svc.finalizeCodexOAuthIdentity(account, c, nil, "")
	require.ErrorContains(t, err, "request plan")
}

func TestCodexRelayShadowComparisonDoesNotSendSecondUpstreamRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	cfg.Gateway.OpenAIAffinity.Secret = testCodexRelaySecret
	upstream := &openAIEgressCountingUpstream{}
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
	account := newTestOAuthAccount(924, map[string]any{
		CodexRelayShadowEnabledExtraKey: true,
		CodexClientProfileExtraKey:      CodexProfileCLI,
		codexFingerprintModeExtraKey:    string(codexFingerprintSession),
	})
	account.Credentials = map[string]any{"access_token": "shadow-token"}
	plan := mustCodexPlanForTest(t, "shadow-logical", "shadow-conversation", CodexTransportHTTP, time.Unix(1_800_000_000, 0))
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	req = req.WithContext(ContextWithCodexRequestPlan(req.Context(), plan))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	before := SnapshotCodexShadowMetrics()
	legacy, err := svc.finalizeCodexOAuthIdentity(account, c, req.Header, "")
	require.NoError(t, err)
	require.NotNil(t, legacy)
	require.Zero(t, upstream.calls.Load(), "shadow comparison must be local only")
	_, hasAttempt := CodexAttemptStateFromContext(c.Request.Context())
	require.False(t, hasAttempt, "shadow candidate must not become the authoritative attempt")
	comparison, ok := CodexShadowComparisonFromContext(c)
	require.True(t, ok)
	require.True(t, comparison.Compared)
	require.Equal(t, before.Compared+1, SnapshotCodexShadowMetrics().Compared)

	_, err = svc.doOpenAIUpstream(req, "", account)
	require.Error(t, err)
	require.Equal(t, int32(1), upstream.calls.Load(), "only the authoritative request may reach upstream")
}

type codexRegistryGatewayCacheStub struct {
	stubGatewayCache
	mu    sync.Mutex
	state *CodexConversationState
}

func (s *codexRegistryGatewayCacheStub) ResolveOrCreateCodexConversation(_ context.Context, _ string, candidate CodexConversationState, _ time.Duration) (CodexConversationState, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != nil {
		return *s.state, false, nil
	}
	candidate.Revision = 1
	s.state = &candidate
	return candidate, true, nil
}

func (s *codexRegistryGatewayCacheStub) GetCodexConversation(_ context.Context, _ string) (CodexConversationState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == nil {
		return CodexConversationState{}, ErrCodexConversationNotFound
	}
	return *s.state, nil
}

func (s *codexRegistryGatewayCacheStub) CompareAndSwapCodexConversation(_ context.Context, _ string, expectedRevision int64, expectedAccountID int64, next CodexConversationState, _ time.Duration) (CodexConversationState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == nil {
		return CodexConversationState{}, ErrCodexConversationNotFound
	}
	if s.state.Revision != expectedRevision || s.state.AccountID != expectedAccountID {
		return *s.state, ErrCodexConversationCASConflict
	}
	next.Revision = expectedRevision + 1
	s.state = &next
	return next, nil
}

func (s *codexRegistryGatewayCacheStub) InvalidateCodexConversation(_ context.Context, _ string, expectedRevision int64, expectedAccountID int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == nil {
		return false, nil
	}
	if s.state.Revision != expectedRevision || s.state.AccountID != expectedAccountID {
		return false, ErrCodexConversationCASConflict
	}
	s.state = nil
	return true, nil
}
