package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type codexResponseRegistryTestCache struct {
	codexRegistryGatewayCacheStub
	mu     sync.Mutex
	states map[string]CodexConversationState
}

func (r *codexResponseRegistryTestCache) GetCodexConversation(_ context.Context, digest string) (CodexConversationState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, found := r.states[digest]
	if !found {
		return CodexConversationState{}, ErrCodexConversationNotFound
	}
	return state, nil
}

func (r *codexResponseRegistryTestCache) ResolveOrCreateCodexConversation(_ context.Context, digest string, candidate CodexConversationState, _ time.Duration) (CodexConversationState, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if current, found := r.states[digest]; found {
		return current, false, nil
	}
	candidate.Revision = 1
	r.states[digest] = candidate
	return candidate, true, nil
}

func (r *codexResponseRegistryTestCache) CompareAndSwapCodexConversation(_ context.Context, digest string, revision, accountID int64, next CodexConversationState, _ time.Duration) (CodexConversationState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, found := r.states[digest]
	if !found {
		return CodexConversationState{}, ErrCodexConversationNotFound
	}
	if current.Revision != revision || current.AccountID != accountID {
		return current, ErrCodexConversationCASConflict
	}
	next.Revision = current.Revision + 1
	r.states[digest] = next
	return next, nil
}

func codexResponseTestContext(t *testing.T, userID, keyID int64, previous string, transport CodexEgressTransport) *gin.Context {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	groupID := int64(9)
	c.Set("api_key", &APIKey{ID: keyID, UserID: userID, GroupID: &groupID})
	operation := CodexOperationResponses
	if previous != "" {
		operation = CodexOperationResume
	}
	plan, err := NewCodexRequestPlan(CodexRequestPlanInput{
		LogicalRequestID: "turn-" + previous, SessionHash: "initial-session", PreviousResponseID: previous,
		Operation: operation, Transport: transport, Body: []byte(`{"input":"test"}`), DerivationSecret: testCodexRelaySecret,
	})
	require.NoError(t, err)
	c.Request = c.Request.WithContext(ContextWithCodexRequestPlan(c.Request.Context(), plan))
	return c
}

func TestCodexResponseChainPinsAcrossInstancesAndTransports(t *testing.T) {
	firstService, _ := migrationTestService()
	registry := &codexResponseRegistryTestCache{states: make(map[string]CodexConversationState)}
	firstService.cache = registry
	secondService, _ := migrationTestService()
	secondService.cache = registry
	account := migrationTestAccount()
	account.Extra[CodexInstallationPolicyExtraKey] = CodexInstallationStableV1
	account.Extra[codexFingerprintModeExtraKey] = "window40"
	first := codexResponseTestContext(t, 1, 7, "", CodexTransportHTTP)
	identity, err := firstService.finalizeCodexOAuthIdentity(account, first, first.Request.Header, "")
	require.NoError(t, err)
	initialAttempt, _ := codexAttemptStateFromGin(first)
	require.NoError(t, firstService.CommitCodexConversationResponse(first, "resp_first"))

	account.Extra[CodexClientProfileExtraKey] = CodexProfileExec
	account.Credentials["access_token"] = "test-refreshed"
	second := codexResponseTestContext(t, 1, 8, "resp_first", CodexTransportHTTP)
	restored, err := secondService.finalizeCodexOAuthIdentity(account, second, second.Request.Header, "")
	require.NoError(t, err)
	require.Equal(t, identity.InstallationID(), restored.InstallationID())
	require.Equal(t, identity.SessionID(), restored.SessionID())
	require.Equal(t, identity.ThreadID(), restored.ThreadID())
	require.Equal(t, identity.WindowID(), restored.WindowID())
	secondAttempt, _ := codexAttemptStateFromGin(second)
	require.Equal(t, initialAttempt.Profile(), secondAttempt.Profile())
	require.Equal(t, initialAttempt.PoolSlot(), secondAttempt.PoolSlot())
	require.NoError(t, secondService.CommitCodexConversationResponse(second, "resp_second"))

	// A rollback affects new conversations, not an already-bound stable chain.
	account.Extra[CodexInstallationPolicyExtraKey] = CodexInstallationLegacyV2
	third := codexResponseTestContext(t, 1, 8, "resp_second", CodexTransportWS)
	thirdIdentity, err := firstService.finalizeCodexOAuthIdentity(account, third, third.Request.Header, "")
	require.NoError(t, err)
	require.Equal(t, identity.SessionID(), thirdIdentity.SessionID())
	require.NoError(t, firstService.CommitCodexConversationResponse(third, "resp_third"))
	thirdAttempt, _ := codexAttemptStateFromGin(third)
	require.Equal(t, CodexInstallationStableV1, thirdAttempt.installationPolicy)
}

func TestCodexStableResponseSnapshotMissingFailsClosed(t *testing.T) {
	svc, _ := migrationTestService()
	svc.cache = &codexResponseRegistryTestCache{states: make(map[string]CodexConversationState)}
	account := migrationTestAccount()
	account.Extra[CodexInstallationPolicyExtraKey] = CodexInstallationStableV1
	c := codexResponseTestContext(t, 1, 7, "resp_before_migration", CodexTransportHTTP)
	_, err := svc.finalizeCodexOAuthIdentity(account, c, c.Request.Header, "")
	var recovery *UpstreamFailoverError
	require.ErrorAs(t, err, &recovery)
	require.Equal(t, OpenAIConversationRecoveryRequiredReason, recovery.Reason)
	require.False(t, recovery.ShouldRetryNextAccount())
}

func TestCodexResponseSnapshotIsTenantScopedAndCannotBeRebound(t *testing.T) {
	svc, _ := migrationTestService()
	svc.cache = &codexResponseRegistryTestCache{states: make(map[string]CodexConversationState)}
	account := migrationTestAccount()
	account.Extra[CodexInstallationPolicyExtraKey] = CodexInstallationStableV1
	c := codexResponseTestContext(t, 1, 7, "", CodexTransportHTTP)
	_, err := svc.finalizeCodexOAuthIdentity(account, c, c.Request.Header, "")
	require.NoError(t, err)
	require.NoError(t, svc.CommitCodexConversationResponse(c, "resp_owned"))
	other := codexResponseTestContext(t, 2, 9, "resp_owned", CodexTransportHTTP)
	_, err = svc.finalizeCodexOAuthIdentity(account, other, other.Request.Header, "")
	require.Error(t, err)

	attempt, _ := codexAttemptStateFromGin(c)
	digest, err := codexResponseConversationDigest(c, "resp_owned", attempt.deriver)
	require.NoError(t, err)
	registry := svc.cache.(*codexResponseRegistryTestCache)
	registry.mu.Lock()
	replaced := registry.states[digest]
	replaced.SessionID = "a-different-winner"
	registry.states[digest] = replaced
	registry.mu.Unlock()
	require.ErrorIs(t, svc.CommitCodexConversationResponse(c, "resp_owned"), ErrCodexConversationCASConflict)
	state, err := registry.GetCodexConversation(context.Background(), digest)
	require.NoError(t, err)
	require.Equal(t, "a-different-winner", state.SessionID)
}
