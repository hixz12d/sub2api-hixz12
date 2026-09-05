package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCodexStableInstallationIgnoresMutableAccountFields(t *testing.T) {
	plan := mustCodexPlanForTest(t, "turn", "conversation", CodexTransportHTTP, time.Now())
	input := CodexAttemptInput{AccountID: 44, AccountVersion: "old", CredentialVersion: "token-old", ProfileID: CodexProfileCLI, FingerprintMode: "device", InstallationPolicy: CodexInstallationStableV1}
	first, err := FinalizeCodexAttempt(plan, input, testCodexRelaySecret)
	require.NoError(t, err)
	input.AccountVersion = "edited"
	input.CredentialVersion = "token-refreshed"
	input.ProxyIdentity = "proxy-new"
	profile, err := ResolveCodexClientProfile(CodexProfileCLI)
	require.NoError(t, err)
	profile.Revision++
	input.ProfileSnapshot = &profile
	second, err := FinalizeCodexAttempt(plan, input, testCodexRelaySecret)
	require.NoError(t, err)
	require.Equal(t, first.Identity().InstallationID(), second.Identity().InstallationID())
	require.NotEqual(t, first.TransportKey(), second.TransportKey())
	input.ProfileSnapshot = nil
	input.ProfileID = CodexProfileExec
	third, err := FinalizeCodexAttempt(plan, input, testCodexRelaySecret)
	require.NoError(t, err)
	require.Equal(t, first.Identity().InstallationID(), third.Identity().InstallationID(), "Codex CLI and exec share an installation family")
	input.ProfileID = CodexProfilePi
	otherFamily, err := FinalizeCodexAttempt(plan, input, testCodexRelaySecret)
	require.NoError(t, err)
	require.NotEqual(t, first.Identity().InstallationID(), otherFamily.Identity().InstallationID())
	input.AccountID++
	otherAccount, err := FinalizeCodexAttempt(plan, input, testCodexRelaySecret)
	require.NoError(t, err)
	require.NotEqual(t, otherFamily.Identity().InstallationID(), otherAccount.Identity().InstallationID())
}

func TestCodexLegacyDeviceDerivationRemainsUnchanged(t *testing.T) {
	plan := mustCodexPlanForTest(t, "turn", "conversation", CodexTransportHTTP, time.Now())
	input := CodexAttemptInput{AccountID: 44, AccountVersion: "old", CredentialVersion: "token-old", ProfileID: CodexProfileCLI, FingerprintMode: "device"}
	attempt, err := FinalizeCodexAttempt(plan, input, testCodexRelaySecret)
	require.NoError(t, err)
	deriver, err := NewCodexIdentityDeriver(testCodexRelaySecret)
	require.NoError(t, err)
	require.Equal(t, deriver.UUIDv4(codexNamespaceDevice, "44:old", "token-old", CodexProfileCLI, strconv.Itoa(attempt.Profile().Revision)), attempt.Identity().InstallationID())
}

func migrationTestContext(t *testing.T, logical string, transport CodexEgressTransport) *gin.Context {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	plan := mustCodexPlanForTest(t, logical, "pinned-conversation", transport, time.Now())
	c.Request = c.Request.WithContext(ContextWithCodexRequestPlan(c.Request.Context(), plan))
	return c
}

func migrationTestService() (*OpenAIGatewayService, *codexRegistryGatewayCacheStub) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIAffinity.Secret = testCodexRelaySecret
	registry := &codexRegistryGatewayCacheStub{}
	return &OpenAIGatewayService{cfg: cfg, cache: registry}, registry
}

func migrationTestAccount() *Account {
	account := newTestOAuthAccount(44, map[string]any{
		CodexRelayModeExtraKey: "relay_kernel", CodexIdentityPolicyVersionExtraKey: "v2",
		CodexClientProfileExtraKey: CodexProfileCLI, codexFingerprintModeExtraKey: "device",
	})
	account.Credentials = map[string]any{"access_token": "test-token-old"}
	return account
}

func TestCodexConversationPinsProfileBeforeCapabilityChecksAndFinalWire(t *testing.T) {
	svc, registry := migrationTestService()
	account := migrationTestAccount()
	firstContext := migrationTestContext(t, "first", CodexTransportWS)
	first, err := svc.finalizeCodexOAuthIdentity(account, firstContext, firstContext.Request.Header, "")
	require.NoError(t, err)
	require.NoError(t, svc.CommitCodexConversation(firstContext.Request.Context()))
	originalProfile := *registry.state.ProfileSnapshot
	account.Extra[CodexClientProfileExtraKey] = CodexProfilePi // Desired new profile cannot handle WS.
	account.Extra[CodexInstallationPolicyExtraKey] = CodexInstallationStableV1
	account.Extra[codexFingerprintModeExtraKey] = "full"
	account.UpdatedAt = time.Now().Add(time.Hour)
	account.Credentials["access_token"] = "test-token-refreshed"
	nextContext := migrationTestContext(t, "next", CodexTransportWS)
	next, err := svc.finalizeCodexOAuthIdentity(account, nextContext, nextContext.Request.Header, "")
	require.NoError(t, err)
	require.Equal(t, first.InstallationID(), next.InstallationID())
	require.Equal(t, first.SessionID(), next.SessionID())
	require.Equal(t, first.mode, next.mode)
	require.NotEqual(t, first.TurnID(), next.TurnID())
	attempt, ok := codexAttemptStateFromGin(nextContext)
	require.True(t, ok)
	require.Equal(t, originalProfile, attempt.Profile())
	headers := http.Header{"User-Agent": {"pi caller changed"}}
	svc.finalizeCodexAttemptWSWire(nextContext, headers, []byte(`{"type":"response.create"}`))
	require.Equal(t, originalProfile.App.UserAgent, headers.Get("User-Agent"))
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	svc.finalizeCodexAttemptHTTPWire(nextContext, req, []byte(`{"input":[]}`))
	require.Equal(t, originalProfile.App.UserAgent, req.Header.Get("User-Agent"))
	require.NoError(t, svc.CommitCodexConversation(nextContext.Request.Context()))
	require.Equal(t, CodexInstallationLegacyV2, registry.state.InstallationPolicy)
	require.Greater(t, registry.state.Revision, int64(2), "successful activity refreshes the registry rather than expiring an active pin")
}

func TestCodexLegacyRecordBackfillsWithoutRotatingIdentity(t *testing.T) {
	svc, registry := migrationTestService()
	account := migrationTestAccount()
	c := migrationTestContext(t, "old", CodexTransportHTTP)
	identity, err := svc.finalizeCodexOAuthIdentity(account, c, c.Request.Header, "")
	require.NoError(t, err)
	registry.state.ProfileSnapshot = nil
	registry.state.ProfileDigest = ""
	registry.state.InstallationPolicy = ""
	registry.state.FingerprintMode = ""
	registry.state.Committed = true
	account.Extra[CodexInstallationPolicyExtraKey] = CodexInstallationStableV1
	account.Extra[CodexClientProfileExtraKey] = CodexProfileExec
	next := migrationTestContext(t, "new", CodexTransportHTTP)
	restored, err := svc.finalizeCodexOAuthIdentity(account, next, next.Request.Header, "")
	require.NoError(t, err)
	require.Equal(t, identity.InstallationID(), restored.InstallationID())
	require.Equal(t, identity.SessionID(), restored.SessionID())
	require.NoError(t, svc.CommitCodexConversation(next.Request.Context()))
	require.NotNil(t, registry.state.ProfileSnapshot)
	require.NotEmpty(t, registry.state.ProfileDigest)
	require.Equal(t, CodexProfileCLI, registry.state.ProfileID)
	require.Equal(t, CodexInstallationLegacyV2, registry.state.InstallationPolicy)

	fresh, freshRegistry := migrationTestService()
	freshContext := migrationTestContext(t, "fresh", CodexTransportHTTP)
	migrated, err := fresh.finalizeCodexOAuthIdentity(account, freshContext, freshContext.Request.Header, "")
	require.NoError(t, err)
	require.NotEqual(t, identity.InstallationID(), migrated.InstallationID())
	require.Equal(t, CodexInstallationStableV1, freshRegistry.state.InstallationPolicy)
	// Rolling the desired policy back does not rewrite a stable conversation.
	account.Extra[CodexInstallationPolicyExtraKey] = CodexInstallationLegacyV2
	rollbackContext := migrationTestContext(t, "rollback", CodexTransportHTTP)
	preserved, err := fresh.finalizeCodexOAuthIdentity(account, rollbackContext, rollbackContext.Request.Header, "")
	require.NoError(t, err)
	require.Equal(t, migrated.InstallationID(), preserved.InstallationID())
	require.NoError(t, fresh.CommitCodexConversation(rollbackContext.Request.Context()))
	require.Equal(t, CodexInstallationStableV1, freshRegistry.state.InstallationPolicy)
}

func TestCodexCommitRejectsSameAccountStaleIdentityAndRoute(t *testing.T) {
	for _, committed := range []bool{false, true} {
		for _, change := range []string{"session", "route"} {
			t.Run(strconv.FormatBool(committed)+change, func(t *testing.T) {
				svc, registry := migrationTestService()
				account := migrationTestAccount()
				c := migrationTestContext(t, "first", CodexTransportHTTP)
				_, err := svc.finalizeCodexOAuthIdentity(account, c, c.Request.Header, "")
				require.NoError(t, err)
				registry.state.Committed = committed
				if change == "session" {
					registry.state.SessionID = "concurrent-winner"
					require.ErrorIs(t, svc.CommitCodexConversation(c.Request.Context()), ErrCodexConversationCASConflict)
				} else {
					registry.state.EgressRoute = "another-route"
					require.NoError(t, svc.CommitCodexConversation(c.Request.Context()))
				}
			})
		}
	}
}

func TestCodexSnapshotOwnsCopyAndRejectsCorruption(t *testing.T) {
	plan := mustCodexPlanForTest(t, "turn", "conversation", CodexTransportHTTP, time.Now())
	input := CodexAttemptInput{AccountID: 44, ProfileID: CodexProfileCLI, FingerprintMode: "device"}
	attempt, err := FinalizeCodexAttempt(plan, input, testCodexRelaySecret)
	require.NoError(t, err)
	state, err := codexConversationStateFromAttempt(plan, attempt, input)
	require.NoError(t, err)
	pinned, err := pinCodexInputToConversation(input, state)
	require.NoError(t, err)
	state.ProfileSnapshot.Transport.HeaderOrder[0] = "mutated"
	require.NotEqual(t, "mutated", pinned.ProfileSnapshot.Transport.HeaderOrder[0])
	require.Error(t, state.Validate())
	state.ProfileSnapshot = nil
	require.Error(t, state.Validate(), "partial snapshots cannot fall back to latest")
	state.ProfileDigest = ""
	require.NoError(t, state.Validate())
	state.InstallationPolicy = "future-policy"
	require.Error(t, state.Validate())
}

func TestCodexFrozenLegacyCatalogIsIndependent(t *testing.T) {
	for _, current := range CodexClientProfiles() {
		if current.BundleID != "" {
			_, err := resolveLegacyCodexConversationProfile(current.ID)
			require.Error(t, err, "new bundles must not fabricate legacy entries")
			continue
		}
		frozen, err := resolveLegacyCodexConversationProfile(current.ID)
		require.NoError(t, err)
		require.Equal(t, 1, frozen.Revision)
		encoded, err := json.Marshal(frozen)
		require.NoError(t, err)
		var decoded CodexClientProfile
		require.NoError(t, json.Unmarshal(encoded, &decoded))
		a, err := codexProfileSnapshotDigest(frozen)
		require.NoError(t, err)
		b, err := codexProfileSnapshotDigest(decoded)
		require.NoError(t, err)
		require.Equal(t, a, b)
	}
	_, err := resolveLegacyCodexConversationProfile("latest")
	require.Error(t, err)
}

func TestCodexInstallationPolicyAccountValidation(t *testing.T) {
	account := migrationTestAccount()
	for _, invalid := range []any{nil, 1, true, "latest"} {
		account.Extra[CodexInstallationPolicyExtraKey] = invalid
		require.True(t, hasCodexRelayAccountExtraUpdate(map[string]any{CodexInstallationPolicyExtraKey: invalid}))
		require.Error(t, ValidateCodexRelayAccountExtra(account.Platform, account.Type, account.Extra, testCodexRelaySecret))
	}
	account.Extra[CodexInstallationPolicyExtraKey] = CodexInstallationStableV1
	require.NoError(t, ValidateCodexRelayAccountExtra(account.Platform, account.Type, account.Extra, testCodexRelaySecret))
	account.Extra[CodexRelayModeExtraKey] = "legacy"
	require.ErrorContains(t, ValidateCodexRelayAccountExtra(account.Platform, account.Type, account.Extra, testCodexRelaySecret), "requires relay_kernel")
}

func TestCodexConcurrentCreatorAdoptsWinningProfile(t *testing.T) {
	svc, registry := migrationTestService()
	plan := mustCodexPlanForTest(t, "first", "conversation", CodexTransportHTTP, time.Now())
	firstInput := CodexAttemptInput{AccountID: 44, ProfileID: CodexProfileCLI, FingerprintMode: "device"}
	first, err := FinalizeCodexAttempt(plan, firstInput, testCodexRelaySecret)
	require.NoError(t, err)
	first, err = svc.resolveCodexConversationAttempt(context.Background(), plan, first, firstInput, true)
	require.NoError(t, err)
	secondInput := firstInput
	secondInput.ProfileID = CodexProfileExec
	secondInput.InstallationPolicy = CodexInstallationStableV1
	second, err := FinalizeCodexAttempt(plan, secondInput, testCodexRelaySecret)
	require.NoError(t, err)
	second, err = svc.resolveCodexConversationAttempt(context.Background(), plan, second, secondInput, true)
	require.NoError(t, err)
	require.Equal(t, first.Profile(), second.Profile())
	require.Equal(t, first.Identity(), second.Identity())
	require.Equal(t, first.TransportKey(), second.TransportKey())
	require.Equal(t, int64(1), registry.state.Revision, "loser must not replace an uncommitted winning profile")
}
