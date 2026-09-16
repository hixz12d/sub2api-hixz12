package service

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

const openAICredentialPoolUnavailableMessage = "no healthy openai oauth credential is currently available"
const openAICredentialPoolCooldown = 5 * time.Minute

// This is an upstream credential-pool health signal, not a request/model error.
// It also applies to API-key relay accounts and to accounts in pool mode.
func isOpenAICredentialPoolUnavailable(account *Account, status int, body []byte) bool {
	if account == nil || account.Platform != PlatformOpenAI || status < http.StatusInternalServerError || status > 599 {
		return false
	}
	message := extractUpstreamErrorMessage(body)
	if !gjson.ValidBytes(body) {
		message = string(body)
	}
	return strings.Contains(strings.ToLower(message), openAICredentialPoolUnavailableMessage)
}

func (s *RateLimitService) handleOpenAICredentialPoolUnavailable(ctx context.Context, account *Account, status int, body []byte) bool {
	if !isOpenAICredentialPoolUnavailable(account, status, body) {
		return false
	}
	// Do not pass the request's model: no credentials means all models are affected.
	// The normal temporary block path updates persistence, cache and runtime listeners.
	s.triggerTempUnschedulable(ctx, account, TempUnschedulableRule{DurationMinutes: int(openAICredentialPoolCooldown / time.Minute)}, -1, status, openAICredentialPoolUnavailableMessage, body)
	// Even if persistence is unavailable, stop retrying this account in this request.
	return true
}
