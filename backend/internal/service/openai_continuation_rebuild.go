package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// A latest user message is not a transcript. Require visible prior input and
// assistant context, with every tool output covered by its local call.
func CanRebuildOpenAIContinuation(body []byte, headers http.Header) bool {
	if strings.TrimSpace(headers.Get(openAIWSTurnStateHeader)) != "" || !gjson.ValidBytes(body) {
		return false
	}
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return false
	}
	items := input.Array()
	localIDs := map[string]bool{}
	for _, item := range items {
		if item.Get("type").String() != "item_reference" {
			if id := item.Get("id").String(); id != "" {
				localIDs[id] = true
			}
		}
	}
	covered := codexCoveredToolCallIDs(body)
	userSeen, assistantSeen := false, false
	for _, item := range items {
		if !item.IsObject() {
			return false
		}
		kind := item.Get("type").String()
		switch kind {
		case "item_reference":
			if !localIDs[item.Get("id").String()] {
				return false
			}
			continue
		case "reasoning":
			continue // Account-bound reasoning is omitted by the existing recovery sanitizer.
		case "tool_search_output", "mcp_approval_response":
			return false
		}
		if item.Get("encrypted_content").Exists() || item.Get("encrypted_reasoning").Exists() {
			return false
		}
		if strings.HasSuffix(kind, "_call_output") {
			if _, ok := covered[item.Get("call_id").String()]; !ok {
				return false
			}
			continue
		}
		if strings.HasSuffix(kind, "_call") {
			if !userSeen || item.Get("call_id").String() == "" {
				return false
			}
			if _, ok := covered[item.Get("call_id").String()]; !ok {
				return false
			}
			assistantSeen = true
			continue
		}
		if kind != "" && kind != "message" {
			return false
		}
		content := item.Get("content")
		if !visibleCodexMessageContent(content) {
			return false
		}
		switch item.Get("role").String() {
		case "user":
			userSeen = true
		case "assistant":
			if userSeen {
				assistantSeen = true
			}
		case "system", "developer":
		default:
			return false
		}
	}
	return userSeen && assistantSeen
}

func visibleCodexMessageContent(content gjson.Result) bool {
	if content.Type == gjson.String {
		return strings.TrimSpace(content.String()) != ""
	}
	if !content.IsArray() || len(content.Array()) == 0 {
		return false
	}
	for _, part := range content.Array() {
		switch part.Get("type").String() {
		case "input_text", "output_text", "text":
			if strings.TrimSpace(part.Get("text").String()) == "" {
				return false
			}
		case "input_image":
			url := part.Get("image_url").String()
			if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "data:image/") {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// Rebase only the existing plan for this pre-send attempt. Preserve the request
// ID, clock, retry budget and auth context; never mutate the old committed pin.
func PrepareCodexFullContextRecovery(c *gin.Context, accountID int64, body []byte) (func(), error) {
	noop := func() {}
	if c == nil || c.Request == nil || accountID <= 0 {
		return noop, codexRecoveryFailure(codexRecoveryOwnerMissing)
	}
	if err := c.Request.Context().Err(); err != nil {
		return noop, err
	}
	owner, owned := openAIWSStateOwnerFromContext(WithOpenAIWSRequestOwner(c.Request.Context(), c))
	if !owned || owner.UserID <= 0 {
		return noop, codexRecoveryFailure(codexRecoveryOwnerMissing)
	}
	plan, ok := CodexRequestPlanFromContext(c.Request.Context())
	if !ok || plan.previousResponseID == "" {
		return noop, nil
	}
	if plan.transport != CodexTransportHTTP {
		return noop, codexRecoveryFailure(codexRecoverySnapshotMissing)
	}
	if !CanRebuildOpenAIContinuation(plan.body, plan.inboundHeaders) || !CanRebuildOpenAIContinuation(body, plan.inboundHeaders) {
		return noop, codexRecoveryFailure(codexRecoveryAccountMismatch)
	}
	guard := NewCodexCommitGuard(c).Snapshot()
	if guard.SemanticOutputStarted || guard.ResponseOwnershipBound {
		return noop, codexRecoveryFailure(codexRecoveryAccountMismatch)
	}
	if budget := openAIRetryBudgetFromContextRaw(c); budget != nil {
		snapshot := budget.Snapshot()
		if !snapshot.ReplaySafe || snapshot.BytesEmitted {
			return noop, codexRecoveryFailure(codexRecoveryAccountMismatch)
		}
	}
	clone := *plan
	clone.rebuildFromLocalHistory = true
	clone.body = SanitizeCodexBodyForCrossAccountRecovery(body)
	clone.previousResponseID, clone.promptCacheKey = "", ""
	clone.requireExistingConversation = false
	clone.operation = CodexOperationResponses
	seed, err := json.Marshal([]any{"codex/full-context-rebuild/v1", owner.GroupID, owner.UserID, plan.conversationDigest, plan.logicalRequestID, accountID})
	if err != nil {
		return noop, err
	}
	digest := sha256.Sum256(seed)
	clone.conversationDigest = hex.EncodeToString(digest[:])
	c.Request = c.Request.WithContext(ContextWithCodexRequestPlan(c.Request.Context(), &clone))
	return func() {
		if c.Request != nil {
			c.Request = c.Request.WithContext(ContextWithCodexRequestPlan(c.Request.Context(), plan))
		}
	}, nil
}
