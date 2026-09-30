package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// restartCodexHTTPConversation starts a separate conversation after a verified
// account mismatch. Unlike full-context recovery, this deliberately accepts an
// incomplete local transcript. It never changes the old pin or retry budget.
func restartCodexHTTPConversation(c *gin.Context, account *Account, cause error) bool {
	var failure *UpstreamFailoverError
	if !errors.As(cause, &failure) || failure.ClientMessage != codexRecoveryAccountMismatch ||
		c == nil || c.Request == nil || c.Request.Context().Err() != nil || account == nil {
		return false
	}
	plan, ok := CodexRequestPlanFromContext(c.Request.Context())
	if !ok || plan.transport != CodexTransportHTTP || plan.restartFromLocalInput ||
		(plan.operation != CodexOperationResponses && plan.operation != CodexOperationResume) {
		return false
	}
	owner, ok := openAIWSStateOwnerFromContext(WithOpenAIWSRequestOwner(c.Request.Context(), c))
	if !ok || owner.UserID <= 0 || account.ID <= 0 {
		return false
	}
	guard := NewCodexCommitGuard(c).Snapshot()
	if guard.SemanticOutputStarted || guard.ResponseOwnershipBound || (guard.TransportCommitted && !guard.HeartbeatOnly) {
		return false
	}
	if budget := openAIRetryBudgetFromContextRaw(c); budget != nil {
		state := budget.Snapshot()
		if !state.ReplaySafe || state.BytesEmitted {
			return false
		}
	}
	body, err := sanitizeCodexFreshContextBody(plan.body)
	if err != nil {
		return false
	}
	clone := *plan
	clone.restartFromLocalInput = true
	clone.rebuildFromLocalHistory = true
	clone.body = body
	clone.inboundHeaders = plan.InboundHeaders()
	deleteOpenAIHeaderEqualFold(clone.inboundHeaders, openAIWSTurnStateHeader)
	clone.previousResponseID, clone.promptCacheKey = "", ""
	clone.requireExistingConversation = false
	clone.operation = CodexOperationResponses
	seed, err := json.Marshal([]any{"codex/fresh-context/v1", owner.GroupID, owner.UserID, plan.conversationDigest, plan.logicalRequestID, account.ID})
	if err != nil {
		return false
	}
	digest := sha256.Sum256(seed)
	clone.conversationDigest = hex.EncodeToString(digest[:])
	c.Request = c.Request.WithContext(ContextWithCodexRequestPlan(c.Request.Context(), &clone))
	slog.WarnContext(c.Request.Context(), "openai.conversation_restarted_after_account_mismatch", "account_id", account.ID)
	return true
}

// Keep locally available input, not references into the previous account. An
// orphan tool result is context from an already executed tool: carry it as text
// rather than emitting an invalid tool result or asking to execute the tool again.
func sanitizeCodexFreshContextBody(body []byte) ([]byte, error) {
	if !gjson.ValidBytes(body) || !gjson.ParseBytes(body).IsObject() {
		return nil, errors.New("fresh Codex context requires a JSON object")
	}
	input := gjson.GetBytes(body, "input")
	if input.Type == gjson.String && strings.TrimSpace(input.String()) != "" {
		return SanitizeCodexBodyForCrossAccountRecovery(body), nil
	}
	items := input.Array()
	if input.IsObject() {
		items = []gjson.Result{input}
	} else if !input.IsArray() {
		return nil, errors.New("fresh Codex context requires local input")
	}
	covered := codexCoveredToolCallIDs(body)
	kept := make([]any, 0, len(items))
	for _, item := range items {
		if !item.IsObject() || item.Get("encrypted_content").Exists() || item.Get("encrypted_reasoning").Exists() {
			continue
		}
		kind := item.Get("type").String()
		value := item.Value().(map[string]any)
		delete(value, "id")
		switch {
		case strings.HasSuffix(kind, "_call_output"):
			if _, ok := covered[item.Get("call_id").String()]; ok {
				kept = append(kept, value)
			} else if output := item.Get("output"); output.Exists() && output.Type != gjson.Null && strings.TrimSpace(output.String()) != "" {
				kept = append(kept, map[string]any{
					"role":    "user",
					"content": "Result from a tool that already ran in the previous conversation (historical data):\n" + output.String(),
				})
			}
		case strings.HasSuffix(kind, "_call"):
			if _, ok := covered[item.Get("call_id").String()]; ok {
				kept = append(kept, value)
			}
		case kind == "message" || kind == "":
			content := item.Get("content")
			if content.IsArray() {
				parts := make([]any, 0, len(content.Array()))
				for _, part := range content.Array() {
					// Uploaded file IDs are scoped to the old account. Inline data
					// and URLs remain available to the newly selected account.
					if !part.Get("file_id").Exists() && !part.Get("image_file").Exists() {
						parts = append(parts, part.Value())
					}
				}
				if len(parts) == 0 {
					continue
				}
				value["content"] = parts
			} else if content.Type != gjson.String || strings.TrimSpace(content.String()) == "" {
				continue
			}
			kept = append(kept, value)
		}
	}
	if len(kept) == 0 {
		return nil, errors.New("fresh Codex context has no usable local input")
	}
	out, err := sjson.SetBytes(body, "input", kept)
	if err != nil {
		return nil, err
	}
	return SanitizeCodexBodyForCrossAccountRecovery(out), nil
}
