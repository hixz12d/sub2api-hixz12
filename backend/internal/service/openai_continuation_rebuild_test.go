package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const rebuildTranscript = `{"previous_response_id":"resp_old","input":[{"role":"user","content":"Remember my project"},{"role":"assistant","content":"The project is a calculator"},{"role":"user","content":"Continue"}]}`

func TestCanRebuildOpenAIContinuation(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       bool
	}{
		{"history", rebuildTranscript, true},
		{"pointer only", `{"previous_response_id":"resp_old"}`, false},
		{"latest string", `{"previous_response_id":"resp_old","input":"continue"}`, false},
		{"latest message", `{"input":[{"role":"user","content":"continue"}]}`, false},
		{"empty role", `{"input":[{"role":"user"},{"role":"assistant"}]}`, false},
		{"opaque reference", `{"input":[{"type":"item_reference","id":"msg_old"},{"role":"user","content":"continue"}]}`, false},
		{"uncovered tool output", `{"input":[{"role":"user","content":"run tests"},{"role":"assistant","content":"running"},{"type":"function_call_output","call_id":"missing","output":"ok"}]}`, false},
		{"covered tool output", `{"input":[{"role":"user","content":"run tests"},{"type":"function_call","call_id":"call_local","name":"test","arguments":"{}"},{"type":"function_call_output","call_id":"call_local","output":"ok"}]}`, true},
		{"account bound file", `{"input":[{"role":"user","content":[{"type":"input_file","file_id":"file_old"}]},{"role":"assistant","content":"read it"}]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) { require.Equal(t, tc.want, CanRebuildOpenAIContinuation([]byte(tc.body), nil)) })
	}
	require.False(t, CanRebuildOpenAIContinuation([]byte(rebuildTranscript), http.Header{http.CanonicalHeaderKey(openAIWSTurnStateHeader): []string{"opaque"}}))
	require.False(t, codexBodyHasLocalRebuildableContext([]byte(`{"previous_response_id":"resp_old","input":"continue"}`)))
}

func rebuildTestContext(t *testing.T, userID int64) *gin.Context {
	t.Helper()
	c := codexResponseTestContext(t, userID, userID+10, "resp_old", CodexTransportHTTP)
	plan, _ := CodexRequestPlanFromContext(c.Request.Context())
	plan.body = []byte(rebuildTranscript)
	plan.requireExistingConversation = true
	PrepareOpenAIRetryBudget(c, plan.body)
	return c
}

func TestCodexFullContextRebuildClearsStalePlanWithoutMutatingOriginal(t *testing.T) {
	c := rebuildTestContext(t, 1)
	original, _ := CodexRequestPlanFromContext(c.Request.Context())
	state := OpenAIAttemptStateSnapshot(c)
	budget := openAIRetryBudgetFromContextRaw(c)
	cleaned := SanitizeCodexBodyForCrossAccountRecovery(original.body)
	restore, err := PrepareCodexFullContextRecovery(c, 12, cleaned)
	require.NoError(t, err)
	current, _ := CodexRequestPlanFromContext(c.Request.Context())
	require.NotSame(t, original, current)
	require.Empty(t, current.previousResponseID)
	require.False(t, current.requireExistingConversation)
	require.Equal(t, CodexOperationResponses, current.Operation())
	require.NotEqual(t, original.ConversationDigest(), current.ConversationDigest())
	require.Equal(t, original.LogicalRequestID(), current.LogicalRequestID())
	require.Equal(t, original.createdAt, current.createdAt)
	require.Equal(t, original.clientRequestID, current.clientRequestID)
	require.Equal(t, state.CanonicalBodySHA256, OpenAIAttemptStateSnapshot(c).CanonicalBodySHA256)
	require.Same(t, budget, openAIRetryBudgetFromContextRaw(c))
	require.Equal(t, "resp_old", original.previousResponseID)
	require.True(t, original.requireExistingConversation)
	require.True(t, gjson.GetBytes(original.body, "previous_response_id").Exists())
	require.False(t, gjson.GetBytes(current.body, "previous_response_id").Exists())
	require.Len(t, gjson.GetBytes(current.body, "input").Array(), 3)
	restore()
	restored, _ := CodexRequestPlanFromContext(c.Request.Context())
	require.Same(t, original, restored)
}

func TestCodexFullContextRebuildNamespacesTenantAndAccount(t *testing.T) {
	digests := map[string]bool{}
	for _, pair := range [][2]int64{{1, 11}, {1, 12}, {2, 11}} {
		c := rebuildTestContext(t, pair[0])
		restore, err := PrepareCodexFullContextRecovery(c, pair[1], []byte(rebuildTranscript))
		require.NoError(t, err)
		plan, _ := CodexRequestPlanFromContext(c.Request.Context())
		require.False(t, digests[plan.ConversationDigest()])
		digests[plan.ConversationDigest()] = true
		restore()
	}
}

func TestCodexFullContextRebuildRejectsCommittedOrIncompleteRequests(t *testing.T) {
	for _, kind := range []string{"semantic", "owned-response", "unknown-acceptance", "latest-message", "missing-owner"} {
		t.Run(kind, func(t *testing.T) {
			c := rebuildTestContext(t, 1)
			original, _ := CodexRequestPlanFromContext(c.Request.Context())
			switch kind {
			case "semantic":
				MarkOpenAISemanticOutputStarted(c)
			case "owned-response":
				TrackOpenAIResponseID(c, "resp_started")
			case "unknown-acceptance":
				openAIRetryBudgetFromContextRaw(c).replaySafe = false
			case "latest-message":
				original.body = []byte(`{"previous_response_id":"resp_old","input":"continue"}`)
			case "missing-owner":
				c.Set("api_key", (*APIKey)(nil))
			}
			_, err := PrepareCodexFullContextRecovery(c, 12, original.body)
			require.Error(t, err)
			current, _ := CodexRequestPlanFromContext(c.Request.Context())
			require.Same(t, original, current)
		})
	}
}

func TestCodexRebuiltLegacyWindowCanFinalizeOnSelectedAccount(t *testing.T) {
	svc, _ := migrationTestService()
	registry := &codexResponseRegistryTestCache{states: make(map[string]CodexConversationState)}
	svc.cache = registry
	account := migrationTestAccount()
	account.Status, account.Schedulable = StatusActive, true
	account.Extra[CodexInstallationPolicyExtraKey] = CodexInstallationStableV1
	c := rebuildTestContext(t, 1)
	original, _ := CodexRequestPlanFromContext(c.Request.Context())
	// Preserve an old committed pin for a different account/window.
	seed := *original
	seed.previousResponseID, seed.requireExistingConversation = "", false
	attempt, err := FinalizeCodexAttempt(&seed, CodexAttemptInput{AccountID: account.ID + 1, ProfileID: CodexProfileCLI, FingerprintMode: "device"}, testCodexRelaySecret)
	require.NoError(t, err)
	old, err := codexConversationStateFromAttempt(&seed, attempt, CodexAttemptInput{})
	require.NoError(t, err)
	old.Committed = true
	registry.states[original.ConversationDigest()] = old
	before, err := json.Marshal(old)
	require.NoError(t, err)
	restore, err := PrepareCodexFullContextRecovery(c, account.ID, []byte(rebuildTranscript))
	require.NoError(t, err)
	defer restore()
	_, err = svc.finalizeCodexOAuthIdentity(account, c, c.Request.Header, "")
	require.NoError(t, err)
	current, _ := codexAttemptStateFromGin(c)
	require.False(t, gjson.GetBytes(current.FinalHTTPBody(), "previous_response_id").Exists())
	require.NoError(t, svc.CommitCodexConversationResponse(c, "resp_rebuilt"))
	after, err := json.Marshal(registry.states[original.ConversationDigest()])
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestCodexLegacyContinuationWithLocalHistoryRecoversMissingSnapshot(t *testing.T) {
	for _, differentAccount := range []bool{false, true} {
		svc, _ := migrationTestService()
		svc.cache = &codexResponseRegistryTestCache{states: make(map[string]CodexConversationState)}
		svc.openaiWSStateStore = NewOpenAIWSStateStore(nil)
		account := migrationTestAccount()
		account.Status, account.Schedulable = StatusActive, true
		account.Extra[CodexInstallationPolicyExtraKey] = CodexInstallationStableV1
		c := rebuildTestContext(t, 1)
		ownerAccount := account.ID
		if differentAccount {
			ownerAccount++
		}
		ctx := WithOpenAIWSRequestOwner(context.Background(), c)
		require.NoError(t, bindOpenAIWSResponseAccount(ctx, svc.getOpenAIWSStateStore(), getOpenAIGroupIDFromContext(c), "resp_old", ownerAccount, svc.openAIAffinityResponseTTL()))
		original, _ := CodexRequestPlanFromContext(c.Request.Context())
		_, err := svc.finalizeCodexOAuthIdentity(account, c, c.Request.Header, "")
		require.NoError(t, err)
		rebuilt, _ := CodexRequestPlanFromContext(c.Request.Context())
		require.Empty(t, rebuilt.previousResponseID)
		require.NotEqual(t, original.ConversationDigest(), rebuilt.ConversationDigest())
		attempt, _ := codexAttemptStateFromGin(c)
		require.False(t, gjson.GetBytes(attempt.FinalHTTPBody(), "previous_response_id").Exists())
		require.NoError(t, svc.CommitCodexConversationResponse(c, "resp_recovered"))
	}
}
