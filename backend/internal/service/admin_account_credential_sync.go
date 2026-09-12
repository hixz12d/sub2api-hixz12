package service

import (
	"context"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var (
	ErrOAuthSyncConflict            = infraerrors.Conflict("CREDENTIAL_VERSION_CONFLICT", "account changed; read current state before submitting a new operation")
	ErrOAuthSyncInvalid             = infraerrors.BadRequest("OAUTH_SYNC_INVALID", "invalid credential sync request")
	ErrOAuthSyncRecoveryUnsupported = infraerrors.BadRequest("AUTH_RECOVERY_UNSUPPORTED", "auth-only recovery requires candidate validation and versioned auth errors; no credentials were written")
)

// This contract writes credentials only. It does not validate provider capabilities
// or remove authentication, manual, quota, or rotation blockers.
type SyncOAuthCredentialsRequest struct {
	ContractVersion    int            `json:"contract_version"`
	OperationID        string         `json:"operation_id"`
	ExpectedUpdatedAt  string         `json:"expected_updated_at"`
	ExpectedInstanceID string         `json:"expected_instance_id,omitempty"`
	ExpectedIdentity   map[string]any `json:"expected_identity"`
	Credentials        map[string]any `json:"credentials"`
	RecoveryMode       string         `json:"recovery_mode"`
}

type SyncOAuthCredentialsResult struct {
	ContractVersion        int      `json:"contract_version"`
	OperationID            string   `json:"operation_id"`
	RemoteAccountID        int64    `json:"remote_account_id"`
	CredentialWrite        string   `json:"credential_write"`
	TokenCacheInvalidation string   `json:"token_cache_invalidation"`
	AuthRecovery           string   `json:"auth_recovery"`
	Schedulable            bool     `json:"schedulable"`
	SchedulingAssessment   string   `json:"scheduling_assessment"`
	RemainingBlockers      []string `json:"remaining_blockers"`
	Partial                bool     `json:"partial"`
	Status                 string   `json:"status"`
	ErrorMessage           string   `json:"error_message,omitempty"`
}

type OAuthCredentialSyncRepository interface {
	UpdateOAuthCredentialsIfUnchanged(context.Context, int64, time.Time, map[string]any, map[string]any) (bool, error)
}

func isRecognizedAuthError(status, message string) bool {
	if !strings.EqualFold(strings.TrimSpace(status), StatusError) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(message)) {
	case "token is expired", "token_expired", "invalid_token", "401 unauthorized", "oauth 401 unauthorized":
		return true
	default:
		return false
	}
}

func filterOAuthCredentialPatch(incoming map[string]any) map[string]any {
	out := map[string]any{}
	for _, key := range []string{"access_token", "refresh_token", "id_token", "expires_at", "expired", "client_id"} {
		if value, ok := incoming[key]; ok {
			if text, isString := value.(string); isString && strings.TrimSpace(text) == "" {
				continue
			}
			out[key] = value
		}
	}
	return out
}

func syncIdentityString(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}

func identityMatches(account *Account, expected map[string]any) error {
	if account == nil || expected == nil {
		return infraerrors.BadRequest("OAUTH_SYNC_INVALID", "expected identity is required")
	}
	want := syncIdentityString(expected, "email")
	got := syncIdentityString(account.Credentials, "email")
	if got == "" {
		got = syncIdentityString(account.Extra, "email")
	}
	if want == "" || got == "" || !strings.EqualFold(want, got) {
		return infraerrors.BadRequest("OAUTH_SYNC_INVALID", "remote email is missing or does not match")
	}
	workspace, explicit := expected["workspace_id"]
	if !explicit {
		return infraerrors.BadRequest("OAUTH_SYNC_INVALID", "expected workspace context is required")
	}
	if workspace != nil {
		if _, ok := workspace.(string); !ok {
			return infraerrors.BadRequest("OAUTH_SYNC_INVALID", "expected workspace context must be a string or null")
		}
	}
	wantWorkspace := syncIdentityString(expected, "workspace_id")
	gotWorkspace := ""
	for _, key := range []string{"workspace_id", "organization_uuid", "organization_id"} {
		if value := syncIdentityString(account.Credentials, key); value != "" {
			if gotWorkspace != "" && !strings.EqualFold(gotWorkspace, value) {
				return infraerrors.BadRequest("OAUTH_SYNC_INVALID", "remote workspace metadata is ambiguous")
			}
			gotWorkspace = value
		}
	}
	if gotWorkspace == "" {
		gotWorkspace = syncIdentityString(account.Extra, "workspace_id")
	}
	if !strings.EqualFold(wantWorkspace, gotWorkspace) {
		return infraerrors.BadRequest("OAUTH_SYNC_INVALID", "remote workspace does not match")
	}
	return nil
}

func validateOAuthSyncPatch(account *Account, incoming map[string]any) (map[string]any, error) {
	patch := filterOAuthCredentialPatch(incoming)
	for key, value := range incoming {
		if _, allowed := patch[key]; !allowed {
			return nil, infraerrors.BadRequest("OAUTH_SYNC_INVALID", "unsupported or empty credential field")
		}
		if key == "expired" {
			if _, ok := value.(bool); !ok {
				return nil, infraerrors.BadRequest("OAUTH_SYNC_INVALID", "expired must be a boolean")
			}
			continue
		}
		if text, ok := value.(string); !ok || strings.TrimSpace(text) == "" {
			return nil, infraerrors.BadRequest("OAUTH_SYNC_INVALID", "credential fields must be nonempty strings")
		}
	}
	if syncIdentityString(patch, "access_token") == "" {
		return nil, infraerrors.BadRequest("OAUTH_SYNC_INVALID", "access_token is required")
	}
	oldClient := syncIdentityString(account.Credentials, "client_id")
	newClient := syncIdentityString(patch, "client_id")
	if newClient != "" && oldClient != "" && newClient != oldClient {
		return nil, infraerrors.BadRequest("OAUTH_SYNC_INVALID", "OAuth client changed; credential grant handoff is required")
	}
	if syncIdentityString(patch, "refresh_token") != "" && newClient == "" {
		return nil, infraerrors.BadRequest("OAUTH_SYNC_INVALID", "client_id is required with refresh_token")
	}
	if syncIdentityString(account.Credentials, "refresh_token") != "" && syncIdentityString(patch, "refresh_token") == "" && syncIdentityString(patch, "access_token") != syncIdentityString(account.Credentials, "access_token") {
		return nil, infraerrors.BadRequest("OAUTH_SYNC_INVALID", "replacement access_token must not inherit an unverified refresh_token")
	}
	return patch, nil
}

func (s *adminServiceImpl) SyncOpenAIOAuthCredentials(ctx context.Context, id int64, req *SyncOAuthCredentialsRequest) (*SyncOAuthCredentialsResult, *Account, error) {
	if req == nil || id <= 0 || (req.ContractVersion != 0 && req.ContractVersion != 1) {
		return nil, nil, ErrOAuthSyncInvalid
	}
	operationID, err := NormalizeIdempotencyKey(req.OperationID)
	if err != nil || operationID == "" {
		return nil, nil, infraerrors.BadRequest("OAUTH_SYNC_INVALID", "operation_id is required and must be a valid idempotency key")
	}
	mode := strings.TrimSpace(req.RecoveryMode)
	if mode == "auth_only" {
		return nil, nil, ErrOAuthSyncRecoveryUnsupported
	}
	if mode != "" && mode != "credentials_only" {
		return nil, nil, infraerrors.BadRequest("OAUTH_SYNC_INVALID", "unsupported recovery_mode")
	}
	expectedAt, err := time.Parse(time.RFC3339Nano, req.ExpectedUpdatedAt)
	if err != nil {
		return nil, nil, infraerrors.BadRequest("OAUTH_SYNC_INVALID", "expected_updated_at must be the timestamp returned by the account API")
	}
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if account == nil {
		return nil, nil, ErrAccountNotFound
	}
	if account.IsShadow() || !account.IsOpenAI() || !account.IsOAuth() {
		return nil, nil, infraerrors.BadRequest("OAUTH_SYNC_INVALID", "only non-shadow OpenAI OAuth accounts are supported")
	}
	if !account.UpdatedAt.Equal(expectedAt) {
		return nil, nil, ErrOAuthSyncConflict
	}
	if err := identityMatches(account, req.ExpectedIdentity); err != nil {
		return nil, nil, err
	}
	patch, err := validateOAuthSyncPatch(account, req.Credentials)
	if err != nil {
		return nil, nil, err
	}
	merged := MergePreservingSensitiveCreds(account.Credentials, patch)
	version := time.Now().UnixMilli()
	if old := account.GetCredentialAsInt64("_token_version"); old >= version {
		version = old + 1
	}
	merged["_token_version"] = version
	updater, ok := s.accountRepo.(OAuthCredentialSyncRepository)
	if !ok {
		return nil, nil, infraerrors.ServiceUnavailable("OAUTH_SYNC_CAS_UNAVAILABLE", "conditional credential storage is unavailable")
	}
	applied, err := updater.UpdateOAuthCredentialsIfUnchanged(ctx, id, expectedAt, account.Credentials, merged)
	if err != nil {
		return nil, nil, err
	}
	if !applied {
		return nil, nil, ErrOAuthSyncConflict
	}
	return &SyncOAuthCredentialsResult{
		ContractVersion: 1, OperationID: operationID, RemoteAccountID: id,
		CredentialWrite: "succeeded", TokenCacheInvalidation: "pending", AuthRecovery: "skipped",
		Schedulable: account.Schedulable, SchedulingAssessment: "not_assessed",
		RemainingBlockers: []string{}, Status: account.Status,
	}, account, nil
}

// Legacy service callers must not bypass candidate validation via this helper.
func (s *adminServiceImpl) RecoverAuthErrorOnly(context.Context, int64, string, string) (string, error) {
	return "unsupported", ErrOAuthSyncRecoveryUnsupported
}

// Read only the existing, redacted receipt. Expired or ambiguous operations are
// never treated as evidence that a credential write did not happen.
func LookupOAuthSyncOperation(ctx context.Context, scope, operationID string) (map[string]any, error) {
	key, err := NormalizeIdempotencyKey(operationID)
	if err != nil || key == "" {
		return nil, ErrOAuthSyncInvalid
	}
	coordinator := DefaultIdempotencyCoordinator()
	if coordinator == nil || coordinator.repo == nil {
		return nil, ErrIdempotencyStoreUnavail
	}
	record, err := coordinator.repo.GetByScopeAndKeyHash(ctx, scope, HashIdempotencyKey(key))
	if err != nil {
		return nil, ErrIdempotencyStoreUnavail
	}
	result := map[string]any{"operation_id": key, "state": "unknown"}
	if record == nil || !record.ExpiresAt.After(time.Now()) {
		return result, nil
	}
	if record.Status != IdempotencyStatusSucceeded {
		return result, nil
	}
	receipt, err := coordinator.decodeStoredResponse(record.ResponseBody)
	if err != nil {
		return nil, ErrIdempotencyStoreUnavail
	}
	result["state"] = "recorded"
	result["receipt"] = receipt
	return result, nil
}
