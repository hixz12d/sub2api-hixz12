package service

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCodexFreshContextAccountMismatchReachesWire(t *testing.T) {
	for _, previous := range []bool{false, true} {
		for _, passthrough := range []bool{false, true} {
			for _, tc := range []struct {
				name, input string
				wantText    string
			}{
				{"latest-message", `[{"role":"user","content":"continue with the calculation"}]`, "continue with the calculation"},
				{"latest-string", `"continue with the calculation"`, "continue with the calculation"},
				{"tool-result", `[{"type":"function_call_output","call_id":"old_call","output":"tests passed; no need to run again"}]`, "tests passed; no need to run again"},
				{"compacted", `[{"type":"compaction","encrypted_content":"old-account-secret"},{"type":"item_reference","id":"old_item"},{"role":"user","content":"continue with the calculation"}]`, "continue with the calculation"},
			} {
				name := tc.name
				if previous {
					name += "/response"
				}
				if passthrough {
					name += "/passthrough"
				}
				t.Run(name, func(t *testing.T) {
					svc, account, c, registry := codexTurnStateRecoveryFixture(t, previous, false)
					original, _ := CodexRequestPlanFromContext(c.Request.Context())
					original.body = []byte(`{"model":"gpt-5","input":` + tc.input + `}`)
					if previous {
						original.body = []byte(`{"model":"gpt-5","previous_response_id":"resp_old","input":` + tc.input + `}`)
					}
					before, err := json.Marshal(registry.states)
					require.NoError(t, err)
					budget := openAIRetryBudgetFromContextRaw(c)
					budgetBefore := budget.Snapshot()
					var wire []byte
					if passthrough {
						req, err := svc.buildUpstreamRequestOpenAIPassthrough(c.Request.Context(), c, account, original.body, "test-token")
						require.NoError(t, err)
						require.Empty(t, req.Header.Get(openAIWSTurnStateHeader))
						wire, err = io.ReadAll(req.Body)
						require.NoError(t, err)
						require.NoError(t, req.Body.Close())
						require.Equal(t, int64(len(wire)), req.ContentLength)
					} else {
						identity, err := svc.finalizeCodexOAuthIdentity(account, c, c.Request.Header, "")
						require.NoError(t, err)
						stageCodexFingerprintIDs(c, identity)
						req, err := svc.buildUpstreamRequest(c.Request.Context(), c, account, original.body, "test-token", true, "", true)
						require.NoError(t, err)
						require.Empty(t, req.Header.Get(openAIWSTurnStateHeader))
						wire, err = io.ReadAll(req.Body)
						require.NoError(t, err)
						require.NoError(t, req.Body.Close())
						require.Equal(t, int64(len(wire)), req.ContentLength)
					}
					current, _ := CodexRequestPlanFromContext(c.Request.Context())
					require.True(t, current.restartFromLocalInput)
					require.Empty(t, current.previousResponseID)
					require.NotEqual(t, original.ConversationDigest(), current.ConversationDigest())
					require.Equal(t, original.LogicalRequestID(), current.LogicalRequestID())
					require.False(t, gjson.GetBytes(wire, "previous_response_id").Exists())
					require.Contains(t, string(wire), tc.wantText)
					require.NotContains(t, string(wire), "old-account-secret")
					require.NotContains(t, string(wire), "old_item")
					if tc.name == "tool-result" {
						require.Equal(t, "function_call_output", gjson.GetBytes(wire, "input.0.type").String())
						require.Equal(t, "old_call", gjson.GetBytes(wire, "input.0.call_id").String())
					}
					require.Same(t, budget, openAIRetryBudgetFromContextRaw(c))
					require.Equal(t, budgetBefore, budget.Snapshot())
					// The fresh pin is additional; all old committed pins remain intact.
					oldStates := make(map[string]CodexConversationState)
					require.NoError(t, json.Unmarshal(before, &oldStates))
					for key, old := range oldStates {
						actual, err := json.Marshal(registry.states[key])
						require.NoError(t, err)
						expected, err := json.Marshal(old)
						require.NoError(t, err)
						require.JSONEq(t, string(expected), string(actual))
					}
					require.NoError(t, ReserveOpenAIUpstreamAttempt(c, account.ID))
					require.NoError(t, svc.CommitCodexConversationResponse(c, "resp_fresh"))
				})
			}
		}
	}
}

func TestCodexFreshContextKeepsToolPairsAndDropsAccountReferences(t *testing.T) {
	body := []byte(`{"previous_response_id":"resp_old","conversation_id":"old","prompt_cache_key":"old","input":[{"role":"user","id":"msg_old","content":[{"type":"input_text","text":"check result"},{"type":"input_file","file_id":"file_old"}]},{"type":"function_call","id":"fc_old","call_id":"paired","name":"test","arguments":"{}"},{"type":"function_call_output","call_id":"paired","output":"passed"},{"type":"function_call","call_id":"pending","name":"test","arguments":"{}"}]}`)
	cleaned, err := sanitizeCodexFreshContextBody(body)
	require.NoError(t, err)
	require.Len(t, gjson.GetBytes(cleaned, "input").Array(), 4)
	require.Len(t, gjson.GetBytes(cleaned, "input.0.content").Array(), 1)
	require.Equal(t, "paired", gjson.GetBytes(cleaned, "input.1.call_id").String())
	require.Equal(t, "paired", gjson.GetBytes(cleaned, "input.2.call_id").String())
	require.Equal(t, "pending", gjson.GetBytes(cleaned, "input.3.call_id").String())
	for _, old := range []string{"resp_old", "file_old", "conversation_id", "prompt_cache_key"} {
		require.NotContains(t, string(cleaned), old)
	}
}

func TestCodexFreshContextDoesNotHideNonMismatchOrEmptyContext(t *testing.T) {
	for _, kind := range []string{"owner", "snapshot", "route", "empty", "cancelled", "already-restarted"} {
		t.Run(kind, func(t *testing.T) {
			_, account, c, _ := codexTurnStateRecoveryFixture(t, false, false)
			plan, _ := CodexRequestPlanFromContext(c.Request.Context())
			cause := codexRecoveryFailure(codexRecoveryAccountMismatch)
			switch kind {
			case "owner":
				cause = codexRecoveryFailure(codexRecoveryOwnerMissing)
			case "snapshot":
				cause = codexRecoveryFailure(codexRecoverySnapshotMissing)
			case "route":
				cause = codexRecoveryFailure(codexRecoveryRouteChanged)
			case "empty":
				plan.body = []byte(`{"previous_response_id":"resp_old","input":[{"type":"item_reference","id":"old"}]}`)
			case "cancelled":
				ctx, cancel := context.WithCancel(c.Request.Context())
				cancel()
				c.Request = c.Request.WithContext(ctx)
			case "already-restarted":
				plan.restartFromLocalInput = true
			}
			require.False(t, restartCodexHTTPConversation(c, account, cause))
			current, _ := CodexRequestPlanFromContext(c.Request.Context())
			require.Same(t, plan, current)
		})
	}
}
