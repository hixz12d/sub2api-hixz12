package service

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	OpenAIOAuthRefreshFailedReason           GatewayFailureReason = "openai_oauth_refresh_failed"
	OpenAIConversationRecoveryRequiredReason GatewayFailureReason = "openai_conversation_recovery_required"
	OpenAIOAuthUnavailableClientMessage                           = "No healthy OpenAI OAuth credential is currently available"
	OpenAIConversationRecoveryClientMessage                       = "The account bound to this conversation is unavailable or its connection settings changed. Start a new conversation with the full context."
)

// The mutation must match the exact credentials used by the refresh request,
// not a scheduler snapshot that may predate a concurrent reauthorization.
type OpenAIOAuthConditionalErrorRepository interface {
	SetOpenAIOAuthErrorIfCredentialsUnchanged(context.Context, int64, map[string]any, string) (bool, error)
}

func openAIPermanentRefreshRejection(err error) bool {
	if err == nil || isSharedProviderRefreshError(err) {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{
		"invalid_grant", "invalid_refresh_token", "token_expired",
		"refresh_token_reused", "refresh_token_invalidated", "app_session_terminated",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func (p *OpenAITokenProvider) quarantineRejectedRefresh(ctx context.Context, result *OAuthRefreshResult, refreshErr error) {
	if p == nil || result == nil || result.Account == nil || !result.Account.IsOpenAIOAuth() || !openAIPermanentRefreshRejection(refreshErr) {
		return
	}
	repo, ok := p.accountRepo.(OpenAIOAuthConditionalErrorRepository)
	if !ok {
		slog.Warn("openai_refresh_quarantine_unavailable", "account_id", result.Account.ID)
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	account := result.Account
	applied, err := repo.SetOpenAIOAuthErrorIfCredentialsUnchanged(cleanupCtx, account.ID, account.Credentials,
		"OpenAI OAuth refresh credential rejected; reauthorize the account")
	if err != nil {
		slog.Warn("openai_refresh_quarantine_failed", "account_id", account.ID, "error", err)
		return
	}
	if !applied {
		return
	}
	// Repository publication rereads durable state; do not install an unversioned
	// permanent in-memory block that could outlive concurrent reauthorization.
	if p.tokenCache != nil {
		if err := p.tokenCache.DeleteAccessToken(cleanupCtx, OpenAITokenCacheKey(account)); err != nil {
			slog.Warn("openai_refresh_cache_delete_failed", "account_id", account.ID, "error", err)
		}
	}
	slog.Warn("openai_refresh_account_quarantined", "account_id", account.ID)
}

func openAIConversationRecoveryError() *UpstreamFailoverError {
	return &UpstreamFailoverError{
		StatusCode:        http.StatusConflict,
		Stage:             GatewayFailureStageAccountAuth,
		Scope:             GatewayFailureScopeRequest,
		Reason:            OpenAIConversationRecoveryRequiredReason,
		NextAccountAction: NextAccountStop,
		ClientStatusCode:  http.StatusConflict,
		ClientMessage:     OpenAIConversationRecoveryClientMessage,
	}
}

// A rejected refresh ends same-account recovery. Grant only the unused portion
// of the existing request budget to a different account, never a new budget.
func (s *OpenAIGatewayService) handleOpenAIRefreshFailure(ctx context.Context, c *gin.Context, account *Account, refreshErr error, passthrough bool) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(refreshErr, context.Canceled) {
		return refreshErr
	}
	guard := NewCodexCommitGuard(c).Snapshot()
	canSwitch := !guard.Stateful && guard.ReplaySafe && !guard.SemanticOutputStarted && !guard.ResponseOwnershipBound
	sharedFailure := isSharedProviderRefreshError(refreshErr)
	canSwitch = canSwitch && !sharedFailure
	if budget := OpenAIRetryBudgetFromContext(c); budget != nil {
		budget.RecordFailure(OpenAIRetryDecision{
			Class:             OpenAIRetryFailureCredential,
			Scope:             OpenAIRetryScopeAccount,
			RetryOtherAccount: canSwitch,
		})
		snapshot := budget.Snapshot()
		canSwitch = canSwitch && snapshot.Attempts < snapshot.MaxAttempts && snapshot.DistinctAccounts < snapshot.MaxDistinctAccounts
	}
	failure := &UpstreamFailoverError{
		StatusCode:        http.StatusUnauthorized,
		Stage:             GatewayFailureStageAccountAuth,
		Scope:             GatewayFailureScopeAccount,
		Reason:            OpenAIOAuthRefreshFailedReason,
		NextAccountAction: NextAccountStop,
		ClientStatusCode:  http.StatusServiceUnavailable,
		ClientMessage:     OpenAIOAuthUnavailableClientMessage,
	}
	if sharedFailure {
		failure.Scope = GatewayFailureScopeProvider
	} else if guard.Stateful {
		failure = codexRecoveryFailure(codexRecoveryRefreshFailed)
	} else if canSwitch {
		failure.NextAccountAction = NextAccountRetry
	}
	setOpsUpstreamError(c, http.StatusUnauthorized, failure.ClientMessage, "")
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform: account.Platform, AccountID: account.ID, AccountName: account.Name,
		UpstreamStatusCode: http.StatusUnauthorized, Passthrough: passthrough,
		Kind: "credential_error", Message: failure.ClientMessage,
	})
	return failure
}
