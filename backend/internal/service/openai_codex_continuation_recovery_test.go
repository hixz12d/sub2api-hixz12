package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCodexLegacyContinuationSurvivesStableOptIn(t *testing.T) {
	svc, _ := migrationTestService()
	registry := &codexResponseRegistryTestCache{states: make(map[string]CodexConversationState)}
	svc.cache = registry
	svc.openaiWSStateStore = NewOpenAIWSStateStore(nil)
	account := migrationTestAccount()
	account.Status, account.Schedulable = StatusActive, true
	c := codexResponseTestContext(t, 1, 7, "resp_legacy", CodexTransportHTTP)
	old, err := svc.finalizeCodexOAuthIdentity(account, c, c.Request.Header, "")
	require.NoError(t, err)
	require.NoError(t, svc.CommitCodexConversation(c.Request.Context()))
	ctx := WithOpenAIWSRequestOwner(c.Request.Context(), c)
	require.NoError(t, bindOpenAIWSResponseAccount(ctx, svc.getOpenAIWSStateStore(), getOpenAIGroupIDFromContext(c), "resp_legacy", account.ID, time.Hour))
	plan, _ := CodexRequestPlanFromContext(c.Request.Context())
	registry.mu.Lock()
	state := registry.states[plan.ConversationDigest()]
	state.ProfileSnapshot, state.ProfileDigest = nil, ""
	state.InstallationPolicy, state.FingerprintMode = "", ""
	registry.states[plan.ConversationDigest()] = state
	registry.mu.Unlock()
	account.Extra[CodexInstallationPolicyExtraKey] = CodexInstallationStableV1
	account.Extra[CodexClientProfileExtraKey] = CodexProfilePi
	account.Credentials["access_token"] = "refreshed-test-token"
	next := codexResponseTestContext(t, 1, 7, "resp_legacy", CodexTransportHTTP)
	restored, err := svc.finalizeCodexOAuthIdentity(account, next, next.Request.Header, "")
	require.NoError(t, err)
	require.Equal(t, old.InstallationID(), restored.InstallationID())
	require.Equal(t, old.SessionID(), restored.SessionID())
	attempt, _ := codexAttemptStateFromGin(next)
	require.Equal(t, CodexProfileCLI, attempt.Profile().ID)
	require.Equal(t, CodexInstallationLegacyV2, attempt.installationPolicy)
	require.NoError(t, svc.CommitCodexConversationResponse(next, "resp_warmed"))

	other := codexResponseTestContext(t, 2, 9, "resp_legacy", CodexTransportHTTP)
	_, err = svc.finalizeCodexOAuthIdentity(account, other, other.Request.Header, "")
	var failure *UpstreamFailoverError
	require.ErrorAs(t, err, &failure)
	require.Equal(t, codexRecoveryOwnerMissing, failure.ClientMessage)

	registry.mu.Lock()
	delete(registry.states, plan.ConversationDigest())
	registry.mu.Unlock()
	missing := codexResponseTestContext(t, 1, 7, "resp_legacy", CodexTransportHTTP)
	_, err = svc.finalizeCodexOAuthIdentity(account, missing, missing.Request.Header, "")
	require.ErrorAs(t, err, &failure)
	require.Equal(t, codexRecoverySnapshotMissing, failure.ClientMessage)
	require.False(t, failure.ShouldRetryNextAccount())
}

func TestCodexConnectionOnlyRefreshPreservesIdentityAndLateCommit(t *testing.T) {
	svc, _ := migrationTestService()
	registry := &codexResponseRegistryTestCache{states: make(map[string]CodexConversationState)}
	svc.cache = registry
	plan := mustCodexPlanForTest(t, "turn", "conversation", CodexTransportHTTP, time.Now())
	plan.previousResponseID = "resp_existing"
	input := CodexAttemptInput{AccountID: 44, ProfileID: CodexProfileCLI, FingerprintMode: "device", ProxyIdentity: "direct", EgressRoute: "official", TransportConfigVersion: "tls:0"}
	original, err := FinalizeCodexAttempt(plan, input, testCodexRelaySecret)
	require.NoError(t, err)
	original, err = svc.resolveCodexConversationAttempt(context.Background(), plan, original, input, false)
	require.NoError(t, err)
	input.TransportConfigVersion = "tls:1"
	input.CredentialVersion = "refreshed"
	newer, err := FinalizeCodexAttempt(plan, input, testCodexRelaySecret)
	require.NoError(t, err)
	newer, err = svc.resolveCodexConversationAttempt(context.Background(), plan, newer, input, false)
	require.NoError(t, err)
	require.Equal(t, original.Identity().InstallationID(), newer.Identity().InstallationID())
	require.Equal(t, original.Identity().SessionID(), newer.Identity().SessionID())
	require.NotEqual(t, original.TransportKey(), newer.TransportKey())
	late := ContextWithCodexAttemptState(ContextWithCodexRequestPlan(context.Background(), plan), original)
	require.NoError(t, svc.CommitCodexConversation(late))
	current, err := registry.GetCodexConversation(context.Background(), plan.ConversationDigest())
	require.NoError(t, err)
	require.Equal(t, "tls:1", current.TransportConfigVersion)

	input.ProxyIdentity = "proxy:other"
	changed, err := FinalizeCodexAttempt(plan, input, testCodexRelaySecret)
	require.NoError(t, err)
	changed, err = svc.resolveCodexConversationAttempt(context.Background(), plan, changed, input, false)
	require.NoError(t, err)
	require.Equal(t, original.Identity().InstallationID(), changed.Identity().InstallationID())
	require.Equal(t, original.Identity().SessionID(), changed.Identity().SessionID())
	require.NoError(t, svc.CommitCodexConversation(ContextWithCodexAttemptState(ContextWithCodexRequestPlan(context.Background(), plan), changed)))
	current, err = registry.GetCodexConversation(context.Background(), plan.ConversationDigest())
	require.NoError(t, err)
	require.Equal(t, "proxy:other", current.ProxyIdentity)
}

func TestCodexResponsePinTTLDoesNotUndercutOwnership(t *testing.T) {
	svc, _ := migrationTestService()
	plan := mustCodexPlanForTest(t, "turn", "conversation", CodexTransportHTTP, time.Now())
	require.Equal(t, 24*time.Hour, svc.codexConversationTTL(plan))
	plan.previousResponseID = "resp_existing"
	require.Equal(t, 72*time.Hour, svc.codexConversationTTL(plan))
	svc.cfg.Gateway.OpenAIAffinity.StrongTTLHours = 168
	require.Equal(t, 168*time.Hour, svc.codexConversationTTL(plan))
}

func TestCodexRecoveryMessagesKeepTerminalClassification(t *testing.T) {
	for _, message := range []string{codexRecoveryOwnerMissing, codexRecoverySnapshotMissing, codexRecoveryAccountMismatch, codexRecoveryAccountUnavailable, codexRecoveryRouteChanged, codexRecoveryRefreshFailed} {
		failure := codexRecoveryFailure(message)
		require.Equal(t, OpenAIConversationRecoveryRequiredReason, failure.Reason)
		require.Equal(t, 409, failure.ClientStatusCode)
		require.Equal(t, message, failure.ClientMessage)
		require.False(t, failure.ShouldRetryNextAccount())
	}
}

func TestCodexPinnedContinuationDoesNotRecreateExpiredRecord(t *testing.T) {
	svc, _ := migrationTestService()
	registry := &codexResponseRegistryTestCache{states: make(map[string]CodexConversationState)}
	svc.cache = registry
	plan := mustCodexPlanForTest(t, "turn", "conversation", CodexTransportHTTP, time.Now())
	plan.requireExistingConversation = true
	input := CodexAttemptInput{AccountID: 44, ProfileID: CodexProfileCLI, FingerprintMode: "device"}
	attempt, err := FinalizeCodexAttempt(plan, input, testCodexRelaySecret)
	require.NoError(t, err)
	_, err = svc.resolveCodexConversationAttempt(context.Background(), plan, attempt, input, false)
	var failure *UpstreamFailoverError
	require.ErrorAs(t, err, &failure)
	require.Equal(t, codexRecoverySnapshotMissing, failure.ClientMessage)
	require.Empty(t, registry.states)
}
