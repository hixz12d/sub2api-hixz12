package admin

import (
	"log/slog"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// SyncOAuthCredentials handles the narrow Team48 credential sync contract.
// POST /api/v1/admin/accounts/:id/sync-oauth-credentials
//
// Unlike apply-oauth-credentials, this path never calls ClearAccountError and
// never flips schedulable. Auth recovery is conditional and CAS-guarded.
func (h *AccountHandler) SyncOAuthCredentials(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}

	var req service.SyncOAuthCredentialsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if req.ContractVersion == 0 {
		req.ContractVersion = 1
	}

	ctx := c.Request.Context()
	result, account, err := h.adminService.SyncOpenAIOAuthCredentials(ctx, accountID, &req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if result == nil {
		response.BadRequest(c, "sync failed")
		return
	}

	beforeStatus := result.Status
	beforeError := result.ErrorMessage
	mode := req.RecoveryMode
	if mode == "" {
		mode = "credentials_only"
	}

	// Invalidate token cache after a successful credential write. Auth recovery
	// only runs after invalidation succeeds.
	if account != nil && h.tokenCacheInvalidator != nil && account.IsOAuth() && result.CredentialWrite == "succeeded" {
		if invErr := h.tokenCacheInvalidator.InvalidateToken(ctx, account); invErr != nil {
			slog.Warn("sync_oauth_credentials.invalidate_token_failed",
				"account_id", accountID,
				"err", invErr,
			)
			result.TokenCacheInvalidation = "failed"
			result.Partial = true
			result.AuthRecovery = "skipped"
		} else {
			result.TokenCacheInvalidation = "succeeded"
		}
	} else if result.TokenCacheInvalidation == "pending" {
		result.TokenCacheInvalidation = "not_applicable"
	}

	if mode == "auth_only" && result.CredentialWrite == "succeeded" && result.TokenCacheInvalidation == "succeeded" {
		recovery, recoverErr := h.adminService.RecoverAuthErrorOnly(ctx, accountID, beforeStatus, beforeError)
		if recoverErr != nil {
			result.AuthRecovery = "failed"
			result.Partial = true
		} else {
			result.AuthRecovery = recovery
			if recovery == "conflict" || recovery == "failed" {
				result.Partial = true
			}
		}
		// Refresh final status after optional recovery.
		if refreshed, getErr := h.adminService.GetAccount(ctx, accountID); getErr == nil && refreshed != nil {
			result.Status = refreshed.Status
			result.ErrorMessage = refreshed.ErrorMessage
			result.Schedulable = refreshed.Schedulable
			result.RemainingBlockers = []string{}
			if !refreshed.Schedulable {
				result.RemainingBlockers = append(result.RemainingBlockers, "schedulable_off")
				result.SchedulingAssessment = "paused"
			} else if refreshed.Status == service.StatusError {
				result.RemainingBlockers = append(result.RemainingBlockers, "status_error")
				result.SchedulingAssessment = "blocked"
			} else if result.AuthRecovery == "cleared" || result.AuthRecovery == "not_applicable" || result.AuthRecovery == "skipped" {
				result.SchedulingAssessment = "configuration_allows_scheduling"
			}
		}
	} else if mode == "auth_only" && result.TokenCacheInvalidation != "succeeded" {
		result.AuthRecovery = "skipped"
	}

	response.Success(c, result)
}
