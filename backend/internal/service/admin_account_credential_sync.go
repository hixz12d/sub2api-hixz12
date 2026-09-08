package service

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// SyncOAuthCredentialsRequest is the v1 narrow credential sync contract.
// It never enables schedulable and never clears rate-limit / temp blockers.
type SyncOAuthCredentialsRequest struct {
	ContractVersion    int            `json:"contract_version"`
	OperationID        string         `json:"operation_id"`
	ExpectedUpdatedAt  string         `json:"expected_updated_at"`
	ExpectedIdentity   map[string]any `json:"expected_identity"`
	Credentials        map[string]any `json:"credentials"`
	RecoveryMode       string         `json:"recovery_mode"` // credentials_only | auth_only
}

// SyncOAuthCredentialsResult separates write success from recovery success.
type SyncOAuthCredentialsResult struct {
	ContractVersion         int      `json:"contract_version"`
	OperationID             string   `json:"operation_id"`
	RemoteAccountID         int64    `json:"remote_account_id"`
	CredentialWrite         string   `json:"credential_write"`
	TokenCacheInvalidation  string   `json:"token_cache_invalidation"`
	AuthRecovery            string   `json:"auth_recovery"`
	Schedulable             bool     `json:"schedulable"`
	SchedulingAssessment    string   `json:"scheduling_assessment"`
	RemainingBlockers       []string `json:"remaining_blockers"`
	Partial                 bool     `json:"partial"`
	Status                  string   `json:"status"`
	ErrorMessage            string   `json:"error_message,omitempty"`
}

func isRecognizedAuthError(status, message string) bool {
	st := strings.ToLower(strings.TrimSpace(status))
	msg := strings.ToLower(strings.TrimSpace(message))
	if st != strings.ToLower(StatusError) && st != "unauthorized" && st != "auth_error" {
		return false
	}
	if msg == "" {
		return false
	}
	markers := []string{
		"401",
		"unauthorized",
		"token_expired",
		"token is expired",
		"invalid_token",
		"authentication",
		"oauth",
		"access token",
		"refresh token",
	}
	for _, marker := range markers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

func filterOAuthCredentialPatch(incoming map[string]any) map[string]any {
	if incoming == nil {
		return map[string]any{}
	}
	allowed := []string{"access_token", "refresh_token", "id_token", "expires_at", "expired", "email"}
	out := map[string]any{}
	for _, key := range allowed {
		if value, ok := incoming[key]; ok {
			// Explicit empty string is not a default wipe for refresh_token.
			if key == "refresh_token" {
				text, isString := value.(string)
				if isString && strings.TrimSpace(text) == "" {
					continue
				}
			}
			out[key] = value
		}
	}
	return out
}

func identityMatches(account *Account, expected map[string]any) error {
	if expected == nil {
		return nil
	}
	if emailRaw, ok := expected["email"]; ok {
		want := strings.ToLower(strings.TrimSpace(fmt.Sprint(emailRaw)))
		if want != "" {
			got := ""
			if account.Credentials != nil {
				if v, ok := account.Credentials["email"]; ok {
					got = strings.ToLower(strings.TrimSpace(fmt.Sprint(v)))
				}
			}
			if got != "" && got != want {
				return fmt.Errorf("expected identity email mismatch")
			}
		}
	}
	if wsRaw, ok := expected["workspace_id"]; ok {
		want := strings.TrimSpace(fmt.Sprint(wsRaw))
		if want != "" {
			got := ""
			if account.Credentials != nil {
				for _, key := range []string{"workspace_id", "organization_id", "organization_uuid"} {
					if v, ok := account.Credentials[key]; ok && strings.TrimSpace(fmt.Sprint(v)) != "" {
						got = strings.TrimSpace(fmt.Sprint(v))
						break
					}
				}
			}
			if got != "" && got != want {
				return fmt.Errorf("expected identity workspace mismatch")
			}
		}
	}
	return nil
}

// SyncOpenAIOAuthCredentials writes OAuth tokens with optional auth-only recovery.
// It must not call ClearAccountError or flip schedulable.
func (s *adminServiceImpl) SyncOpenAIOAuthCredentials(ctx context.Context, id int64, req *SyncOAuthCredentialsRequest) (*SyncOAuthCredentialsResult, *Account, error) {
	if req == nil {
		return nil, nil, fmt.Errorf("request required")
	}
	if req.ContractVersion != 0 && req.ContractVersion != 1 {
		return nil, nil, fmt.Errorf("unsupported contract_version")
	}
	mode := strings.TrimSpace(req.RecoveryMode)
	if mode == "" {
		mode = "credentials_only"
	}
	if mode != "credentials_only" && mode != "auth_only" {
		return nil, nil, fmt.Errorf("unsupported recovery_mode")
	}

	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if account == nil {
		return nil, nil, fmt.Errorf("account not found")
	}
	if account.IsShadow() {
		return nil, nil, fmt.Errorf("shadow accounts are not supported")
	}
	if !account.IsOpenAI() || !account.IsOAuth() {
		return nil, nil, fmt.Errorf("only OpenAI OAuth accounts are supported")
	}
	if err := identityMatches(account, req.ExpectedIdentity); err != nil {
		return nil, nil, err
	}
	if req.ExpectedUpdatedAt != "" {
		// Best-effort concurrency token: compare RFC3339 / raw string forms when present.
		got := account.UpdatedAt.UTC().Format(time.RFC3339Nano)
		alt := account.UpdatedAt.UTC().Format(time.RFC3339)
		want := strings.TrimSpace(req.ExpectedUpdatedAt)
		if want != got && want != alt && want != account.UpdatedAt.String() {
			// Soft check only when the client supplied a comparable stamp.
			// Unknown formats do not hard-fail credential writes in v1.
		}
	}

	beforeStatus := account.Status
	beforeError := account.ErrorMessage
	beforeSchedulable := account.Schedulable

	patch := filterOAuthCredentialPatch(req.Credentials)
	if len(patch) == 0 {
		return nil, nil, fmt.Errorf("no credential fields to write")
	}
	merged := MergePreservingSensitiveCreds(account.Credentials, patch)
	updater, ok := s.accountRepo.(interface {
		UpdateCredentials(context.Context, int64, map[string]any) error
	})
	if !ok {
		return nil, nil, fmt.Errorf("credential updater unavailable")
	}
	if err := updater.UpdateCredentials(ctx, id, merged); err != nil {
		return nil, nil, err
	}

	result := &SyncOAuthCredentialsResult{
		ContractVersion:        1,
		OperationID:            req.OperationID,
		RemoteAccountID:        id,
		CredentialWrite:        "succeeded",
		TokenCacheInvalidation: "skipped",
		AuthRecovery:           "skipped",
		Schedulable:            beforeSchedulable,
		SchedulingAssessment:   "unknown",
		RemainingBlockers:      []string{},
		Partial:                false,
		Status:                 beforeStatus,
		ErrorMessage:           beforeError,
	}

	// Reload after credential write. Token cache invalidation is done by the handler
	// so this service never owns broad recovery side effects.
	if _, err := s.accountRepo.GetByID(ctx, id); err != nil {
		result.Partial = true
		result.CredentialWrite = "succeeded"
		result.TokenCacheInvalidation = "pending"
		return result, account, nil
	}
	result.TokenCacheInvalidation = "pending"


	finalAccount, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		result.Partial = true
		return result, account, nil
	}
	result.Schedulable = finalAccount.Schedulable
	result.Status = finalAccount.Status
	result.ErrorMessage = finalAccount.ErrorMessage
	if !finalAccount.Schedulable {
		result.RemainingBlockers = append(result.RemainingBlockers, "schedulable_off")
		result.SchedulingAssessment = "paused"
	} else if strings.EqualFold(finalAccount.Status, StatusError) {
		result.RemainingBlockers = append(result.RemainingBlockers, "status_error")
		result.SchedulingAssessment = "blocked"
	} else if finalAccount.TempUnschedulableUntil != nil && finalAccount.TempUnschedulableUntil.After(time.Now()) {
		result.RemainingBlockers = append(result.RemainingBlockers, "temp_unschedulable")
		result.SchedulingAssessment = "blocked"
	} else if result.AuthRecovery == "cleared" || result.AuthRecovery == "not_applicable" || result.AuthRecovery == "skipped" {
		result.SchedulingAssessment = "configuration_allows_scheduling"
	}
	return result, finalAccount, nil
}


// RecoverAuthErrorOnly clears a previously observed auth error after credentials
// were written and token cache invalidation succeeded. It never clears rate limits.
func (s *adminServiceImpl) RecoverAuthErrorOnly(ctx context.Context, id int64, expectedStatus, expectedErrorMessage string) (string, error) {
	if !isRecognizedAuthError(expectedStatus, expectedErrorMessage) {
		return "not_applicable", nil
	}
	clearer, ok := s.accountRepo.(interface {
		ClearAuthErrorOnly(context.Context, int64, string, string) (bool, error)
	})
	if !ok {
		return "not_supported_by_repo", nil
	}
	cleared, err := clearer.ClearAuthErrorOnly(ctx, id, expectedStatus, expectedErrorMessage)
	if err != nil {
		return "failed", err
	}
	if !cleared {
		return "conflict", nil
	}
	return "cleared", nil
}
