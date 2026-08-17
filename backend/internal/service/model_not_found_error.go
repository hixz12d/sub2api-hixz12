package service

import (
	"encoding/json"
	"net/http"
	"strings"
)

var upstreamModelNotFoundKeywords = []string{"model not found", "unknown model", "unknown provider for model", "not found"}

func isUpstreamModelNotFoundError(statusCode int, body []byte) bool {
	normalized := normalizeModelNotFoundBody(body)
	if normalized == "" {
		return false
	}
	if statusCode == http.StatusNotFound {
		if !strings.Contains(normalized, "model") {
			return false
		}
		return containsModelNotFoundKeyword(normalized)
	}
	if statusCode != http.StatusBadRequest {
		return false
	}
	// Compatible aggregators use HTTP 400 for an account/model capability miss.
	// Require a provider-specific phrase or an explicit structured error code so
	// ordinary client-side 400 messages do not trigger account failover.
	return strings.Contains(normalized, "unknown provider for model") || hasModelNotFoundCode(body)
}

func hasModelNotFoundCode(body []byte) bool {
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	return containsModelNotFoundCode(payload)
}

func containsModelNotFoundCode(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if strings.EqualFold(key, "code") {
				if code, ok := nested.(string); ok && strings.EqualFold(code, "model_not_found") {
					return true
				}
			}
			if containsModelNotFoundCode(nested) {
				return true
			}
		}
	case []any:
		for _, nested := range typed {
			if containsModelNotFoundCode(nested) {
				return true
			}
		}
	}
	return false
}

func isModelNotFoundError(statusCode int, body []byte) bool {
	return isUpstreamModelNotFoundError(statusCode, body) || statusCode == http.StatusNotFound
}

// openAICodexPlanGatedModelPhrase matches the deterministic Codex 400 returned
// when a ChatGPT OAuth account's plan cannot serve the requested model, e.g.
// {"detail":"The 'gpt-5.6-sol' model is not supported when using Codex with a ChatGPT account."}
// The phrase is compared against the normalized body (lowercased, "_"/"-"
// folded to spaces), so it also matches the same message embedded in
// error.message-style payloads.
const openAICodexPlanGatedModelPhrase = "model is not supported when using codex"

// isOpenAICodexPlanGatedModelError reports whether the upstream response is the
// deterministic Codex rejection of a plan-gated model on a ChatGPT account.
// Unlike transient failures, retrying the same account cannot succeed until the
// account's plan changes, so callers should treat it like model-not-found and
// cool the (account, model) pair down instead of re-selecting the account.
func isOpenAICodexPlanGatedModelError(statusCode int, body []byte) bool {
	if statusCode != http.StatusBadRequest {
		return false
	}
	normalized := normalizeModelNotFoundBody(body)
	if normalized == "" {
		return false
	}
	return strings.Contains(normalized, openAICodexPlanGatedModelPhrase)
}

func containsModelNotFoundKeyword(normalizedBody string) bool {
	if normalizedBody == "" {
		return false
	}
	for _, keyword := range upstreamModelNotFoundKeywords {
		if strings.Contains(normalizedBody, keyword) {
			return true
		}
	}
	return false
}

func normalizeModelNotFoundBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	normalized := strings.ToLower(string(body))
	normalized = strings.NewReplacer("_", " ", "-", " ", "\n", " ", "\r", " ", "\t", " ").Replace(normalized)
	return strings.Join(strings.Fields(normalized), " ")
}
