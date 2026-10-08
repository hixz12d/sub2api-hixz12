package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
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
	body, stats, err := sanitizeCodexCrossAccountBody(plan.body)
	if err == nil && !codexSanitizedInputAvailable(body) {
		err = errors.New("fresh Codex context has no usable local input")
	}
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
	var mismatch *codexAccountMismatchError
	var previousAccountID int64
	if errors.As(cause, &mismatch) {
		previousAccountID = mismatch.previousAccountID
	}
	slog.WarnContext(c.Request.Context(), "openai.conversation_restarted_after_account_mismatch",
		"request_id", plan.logicalRequestID,
		"user_id", owner.UserID,
		"previous_account_id", previousAccountID,
		"account_id", account.ID,
		"dropped", stats.dropped,
		"stripped", stats.stripped,
	)
	return true
}

// Keep all locally available tool declarations and history. Account mismatch
// alone is not evidence that a tool output is invalid; never rewrite it as text.
func sanitizeCodexFreshContextBody(body []byte) ([]byte, error) {
	out, _, err := sanitizeCodexCrossAccountBody(body)
	if err != nil {
		return nil, err
	}
	if !codexSanitizedInputAvailable(out) {
		return nil, errors.New("fresh Codex context has no usable local input")
	}
	return out, nil
}

func codexSanitizedInputAvailable(body []byte) bool {
	input := gjson.GetBytes(body, "input")
	return input.Type == gjson.String && strings.TrimSpace(input.String()) != "" ||
		input.IsArray() && len(input.Array()) > 0
}

type codexContextSanitizeStats struct {
	dropped  map[string]int // Entire input items/content parts removed, by type.
	stripped map[string]int // Account-bound fields removed, including from retained items.
}

// Use raw JSON for retained items: decoding through interface{} would round
// large numbers in tool arguments/results and rewrite otherwise portable data.
func sanitizeCodexCrossAccountBody(body []byte) ([]byte, codexContextSanitizeStats, error) {
	stats := codexContextSanitizeStats{dropped: map[string]int{}, stripped: map[string]int{}}
	if !gjson.ValidBytes(body) || !gjson.ParseBytes(body).IsObject() {
		return nil, stats, errors.New("Codex context requires a JSON object")
	}
	out := body
	for _, key := range []string{"previous_response_id", "conversation", "conversation_id", "prompt_cache_key"} {
		if gjson.GetBytes(out, key).Exists() {
			next, err := sjson.DeleteBytes(out, key)
			if err != nil {
				return nil, stats, err
			}
			out = next
			stats.stripped[key]++
		}
	}
	input := gjson.GetBytes(out, "input")
	if !input.IsArray() && !input.IsObject() {
		return out, stats, nil
	}
	items := input.Array()
	if input.IsObject() {
		items = []gjson.Result{input}
	}
	kept := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		clean, err := sanitizeCodexCrossAccountItem(item, &stats)
		if err != nil {
			return nil, stats, err
		}
		if len(clean) > 0 {
			kept = append(kept, clean)
		}
	}
	out, err := sjson.SetBytes(out, "input", kept)
	return out, stats, err
}

func sanitizeCodexCrossAccountItem(item gjson.Result, stats *codexContextSanitizeStats) (json.RawMessage, error) {
	if !item.IsObject() {
		if item.Type == gjson.String && strings.TrimSpace(item.String()) != "" {
			return json.RawMessage(item.Raw), nil
		}
		stats.dropped["invalid_input"]++
		return nil, nil
	}
	kind := item.Get("type").String()
	if kind == "item_reference" {
		stats.dropped[kind]++
		return nil, nil
	}
	clean := []byte(item.Raw)
	encrypted := false
	for _, key := range []string{"encrypted_content", "encrypted_reasoning"} {
		if item.Get(key).Exists() {
			next, err := sjson.DeleteBytes(clean, key)
			if err != nil {
				return nil, err
			}
			clean = next
			encrypted = true
			stats.stripped[key]++
		}
	}
	if encrypted {
		visible := false
		for _, key := range []string{"summary", "content", "text", "output"} {
			if codexContextValueHasContent(gjson.GetBytes(clean, key)) {
				visible = true
				break
			}
		}
		// Compaction is an opaque account-bound record, not a portable message.
		if kind == "compaction" || !visible {
			if kind == "reasoning" {
				stats.dropped["encrypted_reasoning"]++
			} else if kind != "" {
				stats.dropped[kind]++
			} else {
				stats.dropped["encrypted_item"]++
			}
			return nil, nil
		}
	}
	// Only inspect protocol message/media fields, never tool schemas, arguments
	// or result payloads (which may legitimately contain a key named file_id).
	if kind == "input_file" || kind == "input_image" || kind == "image_file" {
		if codexContextHasForeignFile(item) {
			stats.dropped[kind]++
			return nil, nil
		}
	}
	if kind == "message" || kind == "" {
		content := gjson.GetBytes(clean, "content")
		if content.IsArray() {
			parts := content.Array()
			kept := make([]json.RawMessage, 0, len(parts))
			for _, part := range parts {
				if codexContextHasForeignFile(part) {
					stats.dropped["file_reference"]++
					continue
				}
				kept = append(kept, json.RawMessage(part.Raw))
			}
			if len(kept) != len(parts) {
				if len(kept) == 0 {
					stats.dropped["message"]++
					return nil, nil
				}
				next, err := sjson.SetBytes(clean, "content", kept)
				if err != nil {
					return nil, err
				}
				clean = next
			}
		} else if content.IsObject() && codexContextHasForeignFile(content) {
			stats.dropped["file_reference"]++
			stats.dropped["message"]++
			return nil, nil
		}
	}
	return json.RawMessage(clean), nil
}

func codexContextHasForeignFile(item gjson.Result) bool {
	return item.Get("file_id").Exists() || item.Get("image_file").Exists() || item.Get("type").String() == "image_file"
}

func codexContextValueHasContent(value gjson.Result) bool {
	if value.Type == gjson.String {
		return strings.TrimSpace(value.String()) != ""
	}
	if value.IsArray() {
		for _, item := range value.Array() {
			if codexContextValueHasContent(item) {
				return true
			}
		}
	}
	if value.IsObject() {
		for _, key := range []string{"text", "content", "summary", "output"} {
			if codexContextValueHasContent(value.Get(key)) {
				return true
			}
		}
	}
	return false
}

// A missing call is the only upstream error that permits degrading a tool
// result to historical text. Callers allow at most one retry and still reserve
// an attempt from the shared budget; no retry is allowed after semantic output.
func prepareCodexRejectedToolOutputRetry(c *gin.Context, account *Account, body []byte, status int, response []byte) ([]byte, bool) {
	if status != http.StatusBadRequest || c == nil || c.Request == nil ||
		c.Request.Context().Err() != nil || account == nil || !usesCodexRelayKernel(account) {
		return body, false
	}
	plan, ok := CodexRequestPlanFromContext(c.Request.Context())
	if !ok || plan.transport != CodexTransportHTTP || !plan.rebuildFromLocalHistory {
		return body, false
	}
	message := strings.ToLower(strings.TrimSpace(extractUpstreamErrorMessage(response)))
	if !strings.HasPrefix(message, "no tool call found for ") || !strings.Contains(message, "call output") {
		return body, false
	}
	guard := NewCodexCommitGuard(c).Snapshot()
	if guard.SemanticOutputStarted || guard.ResponseOwnershipBound || (guard.TransportCommitted && !guard.HeartbeatOnly) {
		return body, false
	}
	if budget := openAIRetryBudgetFromContextRaw(c); budget != nil {
		state := budget.Snapshot()
		if !state.ReplaySafe || state.BytesEmitted {
			return body, false
		}
	}
	cleaned, _, err := sanitizeCodexCrossAccountBody(body)
	if err != nil {
		return body, false
	}
	input := gjson.GetBytes(cleaned, "input")
	if !input.IsArray() {
		return body, false
	}
	covered := codexCoveredToolCallIDs(cleaned)
	items := input.Array()
	kept := make([]json.RawMessage, 0, len(items))
	converted := map[string]int{}
	for _, item := range items {
		kind := item.Get("type").String()
		_, paired := covered[strings.TrimSpace(item.Get("call_id").String())]
		// tool_search_output has its own protocol and must retain its tool
		// definitions. Only degrade the call-output types named by this error.
		if (kind == "function_call_output" || kind == "custom_tool_call_output") && !paired &&
			strings.TrimSpace(item.Get("call_id").String()) != "" && item.Get("output").Exists() {
			value, marshalErr := json.Marshal(map[string]string{
				"role":    "user",
				"content": "Result from a tool that already ran in the previous conversation (historical data):\n" + item.Get("output").String(),
			})
			if marshalErr != nil {
				return body, false
			}
			kept = append(kept, value)
			converted[kind]++
		} else {
			kept = append(kept, json.RawMessage(item.Raw))
		}
	}
	if len(converted) == 0 {
		return body, false
	}
	out, err := sjson.SetBytes(cleaned, "input", kept)
	if err != nil {
		return body, false
	}
	owner, _ := openAIWSStateOwnerFromContext(WithOpenAIWSRequestOwner(c.Request.Context(), c))
	slog.WarnContext(c.Request.Context(), "openai.cross_account_tool_output_text_fallback",
		"request_id", plan.logicalRequestID,
		"user_id", owner.UserID,
		"account_id", account.ID,
		"converted", converted,
	)
	return out, true
}
