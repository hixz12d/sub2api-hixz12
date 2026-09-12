package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func codexTurnStateRecoveryFixture(t *testing.T, previous, sameAccount bool) (*OpenAIGatewayService, *Account, *gin.Context, *codexResponseRegistryTestCache) {
	t.Helper()
	svc, _ := migrationTestService()
	registry := &codexResponseRegistryTestCache{states: make(map[string]CodexConversationState)}
	svc.cache = registry
	account := migrationTestAccount()
	account.Status, account.Schedulable = StatusActive, true
	account.Extra[CodexInstallationPolicyExtraKey] = CodexInstallationStableV1
	first := codexResponseTestContext(t, 1, 11, "", CodexTransportHTTP)
	_, err := svc.finalizeCodexOAuthIdentity(account, first, first.Request.Header, "")
	require.NoError(t, err)
	require.NoError(t, svc.CommitCodexConversationResponse(first, "resp_old"))
	if !sameAccount {
		account.ID++
	}
	responseID := ""
	body := RemovePreviousResponseIDFromBody([]byte(rebuildTranscript))
	if previous {
		responseID = "resp_old"
		body = []byte(rebuildTranscript)
	}
	c := codexResponseTestContext(t, 1, 11, responseID, CodexTransportHTTP)
	c.Request.Header.Set(openAIWSTurnStateHeader, "old-account-turn-state")
	plan, _ := CodexRequestPlanFromContext(c.Request.Context())
	plan.body = body
	plan.inboundHeaders = c.Request.Header.Clone()
	EnsureOpenAIRetryBudget(c, account, body)
	return svc, account, c, registry
}

func TestCodexHTTPTurnStateRecoveryReachesWireAndPreservesOriginal(t *testing.T) {
	for _, previous := range []bool{false, true} {
		for _, passthrough := range []bool{false, true} {
			for _, sameAccount := range []bool{false, true} {
				name := "session"
				if previous {
					name = "response"
				}
				if passthrough {
					name += "/passthrough"
				} else {
					name += "/normal"
				}
				if sameAccount {
					name += "/same_account"
				} else {
					name += "/replacement_account"
				}
				t.Run(name, func(t *testing.T) {
					svc, account, c, registry := codexTurnStateRecoveryFixture(t, previous, sameAccount)
					original, _ := CodexRequestPlanFromContext(c.Request.Context())
					before := make(map[string]CodexConversationState)
					for key, state := range registry.states {
						before[key] = state
					}
					budget := openAIRetryBudgetFromContextRaw(c)
					budgetBefore := budget.Snapshot()
					var req *http.Request
					var err error
					if passthrough {
						req, err = svc.buildUpstreamRequestOpenAIPassthrough(c.Request.Context(), c, account, original.body, "test-token")
					} else {
						identity, identityErr := svc.finalizeCodexOAuthIdentity(account, c, c.Request.Header, "")
						require.NoError(t, identityErr)
						stageCodexFingerprintIDs(c, identity)
						req, err = svc.buildUpstreamRequest(c.Request.Context(), c, account, original.body, "test-token", true, "", true)
					}
					require.NoError(t, err)
					body, err := io.ReadAll(req.Body)
					require.NoError(t, err)
					require.NoError(t, req.Body.Close())
					require.Len(t, gjson.GetBytes(body, "input").Array(), 3)
					require.JSONEq(t, gjson.GetBytes(original.body, "input").Raw, gjson.GetBytes(body, "input").Raw)
					require.Equal(t, int64(len(body)), req.ContentLength)
					current, _ := CodexRequestPlanFromContext(c.Request.Context())
					if sameAccount {
						require.Equal(t, "old-account-turn-state", req.Header.Get(openAIWSTurnStateHeader))
						require.False(t, current.rebuildFromLocalHistory)
					} else {
						require.Empty(t, req.Header.Get(openAIWSTurnStateHeader))
						require.Empty(t, current.InboundHeaders().Get(openAIWSTurnStateHeader))
						require.True(t, current.rebuildFromLocalHistory)
						require.NotEqual(t, original.ConversationDigest(), current.ConversationDigest())
						require.False(t, gjson.GetBytes(body, "previous_response_id").Exists())
						for key, state := range before {
							require.Equal(t, state, registry.states[key], "recovery must not rewrite the original pin")
						}
					}
					require.Equal(t, "old-account-turn-state", c.Request.Header.Get(openAIWSTurnStateHeader))
					require.Equal(t, "old-account-turn-state", original.InboundHeaders().Get(openAIWSTurnStateHeader))
					require.Same(t, budget, openAIRetryBudgetFromContextRaw(c))
					require.Equal(t, budgetBefore, budget.Snapshot(), "rebuilding must not replenish or widen retry admission")
					require.NoError(t, ReserveOpenAIUpstreamAttempt(c, account.ID))
					require.NoError(t, svc.CommitCodexConversationResponse(c, "resp_recovered"))
				})
			}
		}
	}
}

func TestCodexHTTPTurnStateRecoveryRejectsUnsafeRequests(t *testing.T) {
	for _, kind := range []string{"latest-message", "uncovered-tool-output", "unresolved-reference", "semantic-output", "owned-response", "unknown-acceptance", "websocket", "missing-owner"} {
		t.Run(kind, func(t *testing.T) {
			svc, account, c, registry := codexTurnStateRecoveryFixture(t, false, false)
			plan, _ := CodexRequestPlanFromContext(c.Request.Context())
			switch kind {
			case "latest-message":
				plan.body = []byte(`{"input":[{"role":"user","content":"continue"}]}`)
			case "uncovered-tool-output":
				plan.body = []byte(`{"input":[{"role":"user","content":"test"},{"role":"assistant","content":"running"},{"type":"function_call_output","call_id":"missing","output":"ok"}]}`)
			case "unresolved-reference":
				plan.body = []byte(`{"input":[{"role":"user","content":"test"},{"role":"assistant","content":"running"},{"type":"item_reference","id":"missing"}]}`)
			case "semantic-output":
				MarkOpenAISemanticOutputStarted(c)
			case "owned-response":
				TrackOpenAIResponseID(c, "resp_started")
			case "unknown-acceptance":
				openAIRetryBudgetFromContextRaw(c).replaySafe = false
			case "websocket":
				plan.transport = CodexTransportWS
			case "missing-owner":
				c.Set("api_key", (*APIKey)(nil))
			}
			before, err := json.Marshal(registry.states)
			require.NoError(t, err)
			_, err = svc.finalizeCodexOAuthIdentity(account, c, c.Request.Header, "")
			var failure *UpstreamFailoverError
			require.ErrorAs(t, err, &failure)
			require.Equal(t, OpenAIConversationRecoveryRequiredReason, failure.Reason)
			after, err := json.Marshal(registry.states)
			require.NoError(t, err)
			require.Equal(t, before, after)
			current, _ := CodexRequestPlanFromContext(c.Request.Context())
			require.Same(t, plan, current)
		})
	}
}

func TestCodexHTTPTurnStateRecoveryRestoresAttemptPlan(t *testing.T) {
	for _, previous := range []bool{false, true} {
		_, account, c, _ := codexTurnStateRecoveryFixture(t, previous, false)
		original, _ := CodexRequestPlanFromContext(c.Request.Context())
		originalBody := append([]byte(nil), original.body...)
		restore, err := PrepareCodexFullContextRecovery(c, account.ID, original.body)
		require.NoError(t, err)
		current, _ := CodexRequestPlanFromContext(c.Request.Context())
		require.NotSame(t, original, current)
		require.Empty(t, current.InboundHeaders().Get(openAIWSTurnStateHeader))
		require.Equal(t, originalBody, original.body)
		require.Equal(t, "old-account-turn-state", original.InboundHeaders().Get(openAIWSTurnStateHeader))
		restore()
		current, _ = CodexRequestPlanFromContext(c.Request.Context())
		require.Same(t, original, current)
	}
}

func TestCodexLegacyHTTPTurnStateRecoveryClearsWire(t *testing.T) {
	svc, _ := migrationTestService()
	svc.cache = &codexResponseRegistryTestCache{states: make(map[string]CodexConversationState)}
	svc.openaiWSStateStore = NewOpenAIWSStateStore(nil)
	account := migrationTestAccount()
	account.Status, account.Schedulable = StatusActive, true
	account.Extra[CodexInstallationPolicyExtraKey] = CodexInstallationStableV1
	c := rebuildTestContext(t, 1)
	c.Request.Header.Set(openAIWSTurnStateHeader, "legacy-account-turn-state")
	original, _ := CodexRequestPlanFromContext(c.Request.Context())
	original.inboundHeaders = c.Request.Header.Clone()
	// Seed historical ownership outside this attempt's response-commit ledger.
	ownerCtx := WithOpenAIWSRequestOwner(context.Background(), c)
	require.NoError(t, bindOpenAIWSResponseAccount(ownerCtx, svc.getOpenAIWSStateStore(), getOpenAIGroupIDFromContext(c), "resp_old", account.ID+1, svc.openAIAffinityResponseTTL()))
	req, err := svc.buildUpstreamRequestOpenAIPassthrough(c.Request.Context(), c, account, original.body, "test-token")
	require.NoError(t, err)
	body, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	require.NoError(t, req.Body.Close())
	require.Empty(t, req.Header.Get(openAIWSTurnStateHeader))
	require.False(t, gjson.GetBytes(body, "previous_response_id").Exists())
	require.Len(t, gjson.GetBytes(body, "input").Array(), 3)
	owner, err := getOpenAIWSResponseAccount(ownerCtx, svc.getOpenAIWSStateStore(), getOpenAIGroupIDFromContext(c), "resp_old")
	require.NoError(t, err)
	require.Equal(t, account.ID+1, owner)
}
