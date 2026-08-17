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

	// Keep compatibility continuation and prompt-cache anchors here. Their keys
	// include the previous account ID and API key ID, so retaining them cannot
	// attach state to the next account; it lets a later request recover the
	// previous account's cache immediately if that account becomes eligible again.

	// Remove only opaque turn/continuation tokens minted by the failed account.
	// Client session/conversation headers are stable routing and cache seeds; on
	// a WebSocket request they must survive this reset for all later turns.
	if c.Request != nil {
		for _, header := range []string{
			legacyCodexThreadHeader,
			codexThreadHeader,
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
