package service

import (
	"context"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	coderws "github.com/coder/websocket"
)

const openAIModelRoutePauseReason = "upstream_model_route_astra_to_luna"

// pauseOpenAIAccountOnModelRoute runs synchronously before forwarding returns
// (or before a WS turn's usage callback). Compare the model actually sent with
// the raw upstream response, never the client-facing model after rewriting.
// Only this explicit downgrade is an account-wide pause; other mismatches are
// still observations. The selected terminal declaration wins even if an earlier
// response.created frame echoed a different model.
func (s *OpenAIGatewayService) pauseOpenAIAccountOnModelRoute(ctx context.Context, account *Account, result *OpenAIForwardResult) bool {
	if s == nil || account == nil || account.ID <= 0 || account.Platform != PlatformOpenAI || result == nil || !result.SucceededForScheduling() {
		return false
	}
	sentModel := upstreamSentModel(result.Model, result.UpstreamModel)
	responseModel := strings.TrimSpace(result.UpstreamResponseModel)
	if !strings.EqualFold(sentModel, "gpt-6-astra") || !strings.EqualFold(responseModel, "gpt-5.6-luna") {
		return false
	}
	// HTTP bridging can report the same result again through the WS turn hook.
	if result.modelRoutePauseHandled {
		return true
	}
	result.modelRoutePauseHandled = true

	// Bridge the persistence/cache propagation window before touching the DB.
	// Do not mutate account: scheduler snapshots may be shared by other requests.
	s.BlockAccountScheduling(account, time.Time{}, openAIModelRoutePauseReason)
	stateCtx, cancel := openAIAccountStateContext(ctx)
	defer cancel()
	if s.accountRepo == nil {
		slog.Error("openai_model_route_pause_failed", "account_id", account.ID, "reason", openAIModelRoutePauseReason, "error", "account repository unavailable")
		return true
	}
	// This repository method also refreshes the shared scheduler snapshot and
	// enqueues the account-change outbox event. No TTL or OAuth refresh resumes
	// schedulable=false; administrators can use the existing resume switch.
	if err := s.accountRepo.SetSchedulable(stateCtx, account.ID, false); err != nil {
		slog.Error("openai_model_route_pause_failed", "account_id", account.ID, "reason", openAIModelRoutePauseReason, "error", err)
		return true
	}
	slog.Warn("openai_model_route_account_paused", "account_id", account.ID,
		"request_id", result.RequestID, "sent_model", sentModel, "response_model", responseModel,
		"response_model_conflict", result.UpstreamResponseModelConflict, "reason", openAIModelRoutePauseReason)
	return true
}

// Wrap a copy so callers retain their callbacks and every ingress mode (pool,
// passthrough and HTTP bridge) stops sending subsequent turns to a paused account.
func (s *OpenAIGatewayService) withOpenAIModelRoutePauseHooks(ctx context.Context, account *Account, hooks *OpenAIWSIngressHooks) *OpenAIWSIngressHooks {
	wrapped := OpenAIWSIngressHooks{}
	if hooks != nil {
		wrapped = *hooks
	}
	beforeTurn, afterTurn := wrapped.BeforeTurn, wrapped.AfterTurn
	var paused atomic.Bool
	wrapped.BeforeTurn = func(turn int) error {
		if paused.Load() {
			return NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "account scheduling paused after upstream model downgrade; reconnect to select another account", nil)
		}
		if beforeTurn != nil {
			return beforeTurn(turn)
		}
		return nil
	}
	wrapped.AfterTurn = func(turn int, result *OpenAIForwardResult, turnErr error) {
		if turnErr == nil && s.pauseOpenAIAccountOnModelRoute(ctx, account, result) {
			paused.Store(true)
		}
		if afterTurn != nil {
			afterTurn(turn, result, turnErr)
		}
	}
	return &wrapped
}
