package service

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
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


// isOpenAIPermanentOAuthUnauthorized reports provider evidence that the OAuth
// credential is dead. Refresh cannot heal these codes; mark the account and
// switch immediately instead of burning the only refresh slot.
func isOpenAIPermanentOAuthUnauthorized(statusCode int, body []byte) bool {
	if statusCode != http.StatusUnauthorized {
		return false
	}
	code := strings.ToLower(strings.TrimSpace(extractUpstreamErrorCode(body)))
	switch code {
	case "token_revoked", "token_invalidated", "invalid_api_key", "account_deactivated", "access_terminated":
		return true
	}
	if gjson.GetBytes(body, "detail").String() == "Unauthorized" {
		return true
	}
	msg := strings.ToLower(strings.TrimSpace(extractUpstreamErrorMessage(body)))
	for _, marker := range []string{
		"invalidated oauth token",
		"token has been revoked",
		"token_revoked",
		"token_invalidated",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
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

func SanitizeOpenAIConversationRecoveryMessage(message string) string {
	return conversationRecoveryClientMessage(message)
}

func conversationRecoveryClientMessage(message string) string {
	switch strings.TrimSpace(message) {
	case OpenAIConversationRecoveryClientMessage,
		codexRecoveryOwnerMissing,
		codexRecoverySnapshotMissing,
		codexRecoveryAccountUnavailable,
		codexRecoveryAccountMismatch,
		codexRecoveryRouteChanged,
		codexRecoveryRefreshFailed:
		return strings.TrimSpace(message)
	default:
		return OpenAIConversationRecoveryClientMessage
	}
}

// A rejected refresh ends same-account recovery. Grant only the unused portion
// of the existing request budget to a different account, never a new budget.
func openAIRequestBodyHasRecoverableFullContext(c *gin.Context) bool {
	if c == nil {
		return false
	}
	if c.Request != nil {
		if plan, ok := CodexRequestPlanFromContext(c.Request.Context()); ok {
			return codexPlanHasRecoverableFullContext(plan)
		}
	}
	return false
}

func (b *OpenAIRetryBudget) allowExtraAccountForCredentialDeath() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	// A permanently dead OAuth credential must not exhaust the single-account
	// sticky budget: grant one additional distinct account for this request.
	if b.maxDistinctAccounts < 2 {
		b.maxDistinctAccounts = 2
	}
	if b.maxAttempts < b.maxDistinctAccounts {
		b.maxAttempts = b.maxDistinctAccounts
	}
}

func (s *OpenAIGatewayService) handleOpenAIRefreshFailure(ctx context.Context, c *gin.Context, account *Account, refreshErr error, passthrough bool) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(refreshErr, context.Canceled) {
		return refreshErr
	}
	guard := NewCodexCommitGuard(c).Snapshot()
	// previous_response_id alone is stateful, but Pi/OpenCode often resend the full
	// conversation input. Those turns can move to another account after OAuth death.
	fullContextRecoverable := openAIRequestBodyHasRecoverableFullContext(c)
	permanent := openAIPermanentRefreshRejection(refreshErr)
	// Permanent credential death still needs a rebuildable body (or a non-sticky
	// turn). A pure previous_response_id chain cannot safely move accounts.
	canSwitch := !guard.SemanticOutputStarted && !guard.ResponseOwnershipBound &&
		(fullContextRecoverable || (guard.ReplaySafe && !guard.Stateful))
	sharedFailure := isSharedProviderRefreshError(refreshErr)
	canSwitch = canSwitch && !sharedFailure
	if budget := OpenAIRetryBudgetFromContext(c); budget != nil {
		if permanent && canSwitch {
			budget.allowExtraAccountForCredentialDeath()
		}
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
	} else if canSwitch {
		failure.NextAccountAction = NextAccountRetry
	} else if guard.Stateful || fullContextRecoverable {
		// Sticky/stateful turn without a rebuildable body must stay on the original pin.
		failure = codexRecoveryFailure(codexRecoveryRefreshFailed)
	}
	setOpsUpstreamError(c, http.StatusUnauthorized, failure.ClientMessage, "")
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform: account.Platform, AccountID: account.ID, AccountName: account.Name,
		UpstreamStatusCode: http.StatusUnauthorized, Passthrough: passthrough,
		Kind: "credential_error", Message: failure.ClientMessage,
	})
	return failure
}


func (s *OpenAIGatewayService) newOpenAIPermanentOAuthUnauthorizedFailover(
	account *Account,
	resp *http.Response,
	body []byte,
	upstreamMsg string,
	shouldDisable bool,
) *UpstreamFailoverError {
	headers := http.Header{}
	if resp != nil {
		headers = resp.Header
	}
	failure := s.newOpenAIAccountFailoverError(account, http.StatusUnauthorized, headers, body, upstreamMsg, shouldDisable, false)
	if failure == nil {
		failure = &UpstreamFailoverError{StatusCode: http.StatusUnauthorized}
	}
	failure.Stage = GatewayFailureStageAccountAuth
	failure.Scope = GatewayFailureScopeAccount
	failure.Reason = OpenAIOAuthRefreshFailedReason
	failure.NextAccountAction = NextAccountRetry
	failure.RetryableOnSameAccount = false
	failure.ClientStatusCode = http.StatusServiceUnavailable
	failure.ClientMessage = OpenAIOAuthUnavailableClientMessage
	return failure
}
