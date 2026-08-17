package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
)

// ResetForAccountSwitch is the single cold-start boundary for a logical OpenAI
// request. It removes state owned by the failed account before the next account
// is allowed to start. Stateful requests normally never reach this method
// because their retry policy is fail-closed, but keeping the boundary explicit
// protects future callers and WS turn retries.
func (s *OpenAIGatewayService) ResetForAccountSwitch(ctx context.Context, c *gin.Context, nextAccountID int64) error {
	if s == nil || c == nil {
		return nil
	}
	state := OpenAIAttemptStateFromContext(c)
	if state == nil || state.CurrentAccountID <= 0 || state.CurrentAccountID == nextAccountID {
		return nil
	}

	previousAccountID := state.CurrentAccountID
	groupID := getOpenAIGroupIDFromContext(c)
	apiKeyID := getAPIKeyIDFromContext(c)
	var resetErr error

	if sessionHash := strings.TrimSpace(state.SessionHash); sessionHash != "" {
		resetErr = errors.Join(resetErr, s.ClearOpenAIStickySession(ctx, &groupID, sessionHash))
	}

	store := s.getOpenAIWSStateStore()
	owner, hasOwner := openAIWSStateOwnerForRequest(ctx, c, previousAccountID, state.SessionHash)
	responseIDs := append([]string{state.PreviousResponseID}, state.ResponseIDs...)
	for _, responseID := range responseIDs {
		responseID = strings.TrimSpace(responseID)
		if responseID == "" || store == nil {
			continue
		}
		if hasOwner {
			resetErr = errors.Join(resetErr, store.DeleteResponseAccountOwned(ctx, groupID, responseID, owner))
			store.DeleteResponseConnOwned(responseID, owner)
		} else {
			// Unauthenticated callers have no tenant namespace to protect; this
			// path is retained for internal compatibility callers only.
			resetErr = errors.Join(resetErr, store.DeleteResponseAccount(ctx, groupID, responseID))
			store.DeleteResponseConn(responseID)
		}
	}
	for _, connID := range state.ResponseConnIDs {
		_ = connID // connection IDs are tracked for audit; response bindings own deletion.
	}
	if store != nil {
		store.DeleteSessionTurnState(groupID, state.SessionHash)
		store.DeleteSessionConn(groupID, state.SessionHash)
	}

	// The in-process Codex turn provenance is keyed by API key + client session,
	// not by account. Remove it only when it was minted by the account being left.
	if seed := openAICodexTurnStateSeed(c); seed != "" {
		if raw, ok := s.openaiCodexTurnStateOrigins.Load(seed); ok {
			if origin, ok := raw.(openAICodexTurnStateOrigin); ok && origin.accountID == previousAccountID {
				s.openaiCodexTurnStateOrigins.Delete(seed)
			}
		}
	}

	// Compatibility continuation and prompt-cache anchors are account-scoped in
	// memory. Clear the complete previous-account namespace for this API key so
	// no stale response or cache anchor can be attached to the next account.
	s.resetOpenAICompatState(previousAccountID, apiKeyID)

	// Do not let a downstream client header from the failed account leak into the
	// next attempt. The next account's identity finalizer will derive fresh values.
	if c.Request != nil {
		for _, header := range []string{
			legacyCodexSessionHeader,
			codexSessionHeader,
			legacyCodexThreadHeader,
			codexThreadHeader,
			"conversation_id",
			"session_id",
			"x-codex-turn-state",
			"x-codex-turn-metadata",
		} {
			c.Request.Header.Del(header)
		}
	}

	state.PreviousAccountID = previousAccountID
	state.CurrentAccountID = 0
	state.TurnState = ""
	state.RouteKey = ""
	state.ResponseIDs = nil
	state.ResponseConnIDs = nil
	state.AccountSwitches++
	state.LastResetReason = fmt.Sprintf("account_switch:%d->%d", previousAccountID, nextAccountID)
	state.attemptActive = false

	return resetErr
}

func (s *OpenAIGatewayService) resetOpenAICompatState(accountID, apiKeyID int64) {
	if s == nil || accountID <= 0 || apiKeyID <= 0 {
		return
	}
	prefix := fmt.Sprintf("%d\x00%d\x00", accountID, apiKeyID)
	s.openaiCompatSessionResponses.Range(func(key, _ any) bool {
		if value, ok := key.(string); ok && strings.HasPrefix(value, prefix) {
			s.openaiCompatSessionResponses.Delete(key)
		}
		return true
	})

	digestPrefix := fmt.Sprintf("%d|%d|", accountID, apiKeyID)
	s.openaiCompatAnthropicDigestSessions.Range(func(key, _ any) bool {
		if value, ok := key.(string); ok && strings.HasPrefix(value, digestPrefix) {
			s.openaiCompatAnthropicDigestSessions.Delete(key)
		}
		return true
	})
}
