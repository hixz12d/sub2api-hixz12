package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// OAuthSyncOperation never contains credentials or provider response bodies.
type OAuthSyncOperation struct {
	ID                                  int64
	AuthRecovery, ValidationScope       string
	ValidatedAt                         *time.Time
	Scope, OperationID, RequestHash     string
	AccountID, CredentialVersion        int64
	CacheDone, SchedulerDone            bool
	State                               string
	Attempts                            int
	LeaseID, LastError                  string
	CreatedAt, UpdatedAt, NextAttemptAt time.Time
}

type OAuthSyncRepository interface {
	InstanceID(context.Context) (string, error)
	Get(context.Context, string, string) (*OAuthSyncOperation, error)
	Latest(context.Context, string, int64) (*OAuthSyncOperation, error)
	Commit(context.Context, *OAuthSyncOperation, *Account, map[string]any) (*OAuthSyncOperation, bool, error)
	Claim(context.Context) (*OAuthSyncOperation, error)
	Progress(context.Context, *OAuthSyncOperation, bool, bool, string, bool, bool) error
	Retry(context.Context, string, string) error
}

type oauthSyncInvalidator interface {
	InvalidateTokenStrict(context.Context, *Account) error
}
type oauthSyncProjector interface {
	RefreshOAuthSyncProjection(context.Context, int64) error
}

type OAuthSyncService struct {
	repo                OAuthSyncRepository
	accounts            AccountRepository
	invalidator         oauthSyncInvalidator
	projector           oauthSyncProjector
	wake                chan struct{}
	validator           oauthCandidateValidator
	cancel              context.CancelFunc
	startOnce, stopOnce sync.Once
	wg                  sync.WaitGroup
}

func NewOAuthSyncService(repo OAuthSyncRepository, accounts AccountRepository, invalidator *CompositeTokenCacheInvalidator, projector *SchedulerSnapshotService) *OAuthSyncService {
	return &OAuthSyncService{repo: repo, accounts: accounts, invalidator: invalidator, projector: projector, wake: make(chan struct{}, 1)}
}

func (s *OAuthSyncService) Start() {
	s.startOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		s.cancel = cancel
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				// Bounded batches ensure shutdown and pending-account fairness.
				for i := 0; i < 20 && ctx.Err() == nil; i++ {
					jobCtx, done := context.WithTimeout(ctx, 30*time.Second)
					worked, err := s.ProcessNext(jobCtx)
					done()
					if err != nil {
						slog.Warn("oauth_sync_followup_failed")
						break
					}
					if !worked {
						break
					}
				}
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				case <-s.wake:
				}
			}
		}()
	})
}
func (s *OAuthSyncService) Stop() {
	s.stopOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		s.wg.Wait()
	})
}
func (s *OAuthSyncService) notify() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *OAuthSyncService) InstanceID(ctx context.Context) (string, error) {
	if s == nil || s.repo == nil {
		return "", ErrIdempotencyStoreUnavail
	}
	return s.repo.InstanceID(ctx)
}

func (s *OAuthSyncService) Submit(ctx context.Context, scope string, id int64, req *SyncOAuthCredentialsRequest) (*OAuthSyncOperation, bool, error) {
	if req == nil || id <= 0 || req.ContractVersion != 1 {
		return nil, false, ErrOAuthSyncInvalid
	}
	key, err := NormalizeIdempotencyKey(req.OperationID)
	if err != nil || key == "" {
		return nil, false, ErrIdempotencyKeyRequired
	}
	req.OperationID = key
	if req.RecoveryMode == "" {
		req.RecoveryMode = "credentials_only"
	}
	if req.RecoveryMode != "credentials_only" && req.RecoveryMode != "auth_only" {
		return nil, false, ErrOAuthSyncRecoveryUnsupported
	}
	instance, err := s.InstanceID(ctx)
	if err != nil {
		return nil, false, ErrIdempotencyStoreUnavail
	}
	if req.ExpectedInstanceID == "" || req.ExpectedInstanceID != instance {
		return nil, false, infraerrors.Conflict("SYNC_INSTANCE_MISMATCH", "integration instance changed; verify the binding")
	}
	encoded, err := json.Marshal(req)
	if err != nil {
		return nil, false, ErrOAuthSyncInvalid
	}
	sum := sha256.Sum256(encoded)
	fingerprint := hex.EncodeToString(sum[:])
	existing, err := s.repo.Get(ctx, scope, key)
	if err != nil {
		return nil, false, ErrIdempotencyStoreUnavail
	}
	if existing != nil {
		if existing.RequestHash != fingerprint || existing.AccountID != id {
			return nil, false, ErrIdempotencyKeyConflict
		}
		return existing, true, nil
	}
	expectedAt, err := time.Parse(time.RFC3339Nano, req.ExpectedUpdatedAt)
	if err != nil {
		return nil, false, ErrOAuthSyncInvalid
	}
	account, err := s.accounts.GetByID(ctx, id)
	if err != nil {
		return nil, false, err
	}
	if account == nil {
		return nil, false, ErrAccountNotFound
	}
	if account.IsShadow() || !account.IsOpenAI() || !account.IsOAuth() {
		return nil, false, ErrOAuthSyncInvalid
	}
	if !expectedAt.Equal(account.UpdatedAt) {
		// A simultaneous identical submission may have committed between the
		// initial receipt lookup and this account read.
		if saved, lookupErr := s.repo.Get(ctx, scope, key); lookupErr != nil {
			return nil, false, ErrIdempotencyStoreUnavail
		} else if saved != nil {
			if saved.RequestHash != fingerprint || saved.AccountID != id {
				return nil, false, ErrIdempotencyKeyConflict
			}
			return saved, true, nil
		}
		return nil, false, ErrOAuthSyncConflict
	}
	if err := identityMatches(account, req.ExpectedIdentity); err != nil {
		return nil, false, err
	}
	patch, err := validateOAuthSyncPatch(account, req.Credentials)
	if err != nil {
		return nil, false, err
	}
	merged := MergePreservingSensitiveCreds(account.Credentials, patch)
	version := time.Now().UnixMilli()
	if old := account.GetCredentialAsInt64("_token_version"); old >= version {
		version = old + 1
	}
	if version <= 0 {
		return nil, false, ErrOAuthSyncConflict
	}
	merged["_token_version"] = version
	pending := &OAuthSyncOperation{Scope: scope, OperationID: key, RequestHash: fingerprint, AccountID: id, CredentialVersion: version, AuthRecovery: "skipped"}
	if req.RecoveryMode == "auth_only" {
		if s.validator == nil {
			return nil, false, ErrOAuthSyncRecoveryUnsupported
		}
		if err := s.validator.Validate(ctx, account, merged); err != nil {
			return nil, false, ErrOAuthValidationFailed
		}
		now := time.Now().UTC()
		pending.ValidatedAt, pending.ValidationScope, pending.AuthRecovery = &now, OAuthValidationScope, "cleared"
	}
	op, replay, err := s.repo.Commit(ctx, pending, account, merged)
	if err == nil {
		s.notify()
	}
	return op, replay, err
}

// ProcessNext resumes only propagation, using CURRENT account data. No OAuth
// exchange or credential update can occur in this path, including after restart.
func (s *OAuthSyncService) ProcessNext(ctx context.Context) (bool, error) {
	op, err := s.repo.Claim(ctx)
	if err != nil || op == nil {
		return false, err
	}
	account, err := s.accounts.GetByID(ctx, op.AccountID)
	if errors.Is(err, ErrAccountNotFound) || (err == nil && account == nil) {
		return true, s.repo.Progress(ctx, op, op.CacheDone, op.SchedulerDone, "account_missing", true, true)
	}
	if err != nil {
		return true, s.repo.Progress(ctx, op, op.CacheDone, op.SchedulerDone, "account_read_failed", true, false)
	}
	if !account.IsOpenAI() || !account.IsOAuth() || account.IsShadow() {
		return true, s.repo.Progress(ctx, op, op.CacheDone, op.SchedulerDone, "account_type_changed", true, true)
	}
	if !op.CacheDone {
		if s.invalidator == nil {
			err = errors.New("unavailable")
		} else {
			err = s.invalidator.InvalidateTokenStrict(ctx, account)
		}
		if err != nil {
			return true, s.repo.Progress(ctx, op, false, op.SchedulerDone, "token_cache_failed", true, false)
		}
		if err := s.repo.Progress(ctx, op, true, op.SchedulerDone, "", false, false); err != nil {
			return true, err
		}
		op.CacheDone = true
	}
	if !op.SchedulerDone {
		if s.projector == nil {
			err = errors.New("unavailable")
		} else {
			err = s.projector.RefreshOAuthSyncProjection(ctx, op.AccountID)
		}
		if err != nil {
			return true, s.repo.Progress(ctx, op, true, false, "scheduler_refresh_failed", true, false)
		}
		op.SchedulerDone = true
	}
	return true, s.repo.Progress(ctx, op, true, true, "", true, false)
}

// Shared Redis projection acknowledgement; this is not an upstream availability
// probe and does not certify that already-running requests have drained.
func (s *SchedulerSnapshotService) RefreshOAuthSyncProjection(ctx context.Context, id int64) error {
	if s == nil || s.cache == nil || s.accountRepo == nil {
		return ErrSchedulerCacheNotReady
	}
	return s.handleAccountEvent(ctx, &id, nil, nil)
}

func (s *OAuthSyncService) Operation(ctx context.Context, scope string, id int64, key string) (*OAuthSyncOperation, error) {
	op, err := s.repo.Get(ctx, scope, key)
	if err != nil {
		return nil, ErrIdempotencyStoreUnavail
	}
	if op != nil && op.AccountID != id {
		return nil, ErrAccountNotFound
	}
	return op, nil
}
func (s *OAuthSyncService) Retry(ctx context.Context, scope string, id int64, key, instance string) (*OAuthSyncOperation, error) {
	current, err := s.InstanceID(ctx)
	if err != nil {
		return nil, ErrIdempotencyStoreUnavail
	}
	if current != instance {
		return nil, infraerrors.Conflict("SYNC_INSTANCE_MISMATCH", "integration instance changed")
	}
	op, err := s.Operation(ctx, scope, id, key)
	if err != nil {
		return nil, err
	}
	if op == nil {
		return nil, ErrAccountNotFound
	}
	if op.State == "pending" {
		if err := s.repo.Retry(ctx, scope, key); err != nil {
			return nil, ErrIdempotencyStoreUnavail
		}
		s.notify()
	}
	return op, nil
}

func oauthSyncReceipt(op *OAuthSyncOperation) map[string]any {
	cache, scheduler := "pending", "pending"
	if op.CacheDone {
		cache = "succeeded"
	}
	if op.SchedulerDone {
		scheduler = "succeeded"
	}
	return map[string]any{
		"contract_version": 1, "operation_id": op.OperationID, "remote_account_id": op.AccountID,
		"credential_version": op.CredentialVersion, "credential_write": "succeeded",
		"auth_recovery": defaultOAuthAuthRecovery(op.AuthRecovery), "validation_scope": op.ValidationScope, "validated_at": op.ValidatedAt,
		"token_cache_invalidation": cache, "scheduler_refresh": scheduler,
		"state": op.State, "partial": op.State != "completed", "ok": op.State == "completed",
		"attempts": op.Attempts, "last_error": op.LastError, "created_at": op.CreatedAt,
		"updated_at": op.UpdatedAt, "next_attempt_at": op.NextAttemptAt,
		"scheduling_assessment": "not_assessed", "remaining_blockers": []string{}, "schedulable": false,
	}
}
func (s *OAuthSyncService) Receipt(ctx context.Context, op *OAuthSyncOperation) map[string]any {
	result := oauthSyncReceipt(op)
	instance, _ := s.InstanceID(ctx)
	result["instance_id"] = instance
	account, err := s.accounts.GetByID(ctx, op.AccountID)
	if err != nil || account == nil {
		result["remaining_blockers"] = []string{"final_state_unknown"}
		return result
	}
	result["schedulable"] = account.Schedulable
	result["remaining_blockers"] = oauthSyncBlockers(account, time.Now())
	result["operation_is_current"] = account.GetCredentialAsInt64("_token_version") == op.CredentialVersion
	return result
}
func oauthSyncBlockers(a *Account, now time.Time) []string {
	blockers := []string{}
	if !a.Schedulable {
		blockers = append(blockers, "schedulable_off")
	}
	if a.Status != StatusActive || strings.TrimSpace(a.ErrorMessage) != "" {
		blockers = append(blockers, "account_error")
	}
	if a.RateLimitResetAt != nil && a.RateLimitResetAt.After(now) {
		blockers = append(blockers, "rate_limit")
	}
	if a.TempUnschedulableUntil != nil && a.TempUnschedulableUntil.After(now) {
		blockers = append(blockers, "temporary_pause")
	}
	if a.OverloadUntil != nil && a.OverloadUntil.After(now) {
		blockers = append(blockers, "overload")
	}
	if a.ExpiresAt != nil && a.AutoPauseOnExpired && !a.ExpiresAt.After(now) {
		blockers = append(blockers, "account_expired")
	}
	return blockers
}
func (s *OAuthSyncService) Snapshot(ctx context.Context, scope string, id int64) (map[string]any, error) {
	instance, err := s.InstanceID(ctx)
	if err != nil {
		return nil, ErrIdempotencyStoreUnavail
	}
	account, err := s.accounts.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, ErrAccountNotFound
	}
	if !account.IsOpenAI() || !account.IsOAuth() || account.IsShadow() {
		return nil, ErrOAuthSyncInvalid
	}
	op, err := s.repo.Latest(ctx, scope, id)
	if err != nil {
		return nil, ErrIdempotencyStoreUnavail
	}
	email := syncIdentityString(account.Credentials, "email")
	if email == "" {
		email = syncIdentityString(account.Extra, "email")
	}
	workspace := ""
	for _, key := range []string{"workspace_id", "organization_uuid", "organization_id"} {
		value := syncIdentityString(account.Credentials, key)
		if workspace != "" && value != "" && workspace != value {
			return nil, ErrOAuthSyncInvalid
		}
		if value != "" {
			workspace = value
		}
	}
	if workspace == "" {
		workspace = syncIdentityString(account.Extra, "workspace_id")
	}
	result := map[string]any{
		"schema_version": 1, "instance_id": instance, "remote_account_id": id,
		"access_token_readback": true,
		"platform":              account.Platform, "type": account.Type,
		"identity":           map[string]any{"email": email, "workspace_id": workspace, "official_account_id": syncIdentityString(account.Credentials, "chatgpt_account_id")},
		"credential_version": account.GetCredentialAsInt64("_token_version"), "account_updated_at": account.UpdatedAt,
		"schedulable": account.Schedulable, "status": account.Status,
		"remaining_blockers": oauthSyncBlockers(account, time.Now()), "availability": "not_verified",
		"observed_at": time.Now().UTC(), "latest_operation": nil,
	}
	if op != nil {
		result["latest_operation"] = oauthSyncReceipt(op)
		result["operation_is_current"] = op.CredentialVersion == account.GetCredentialAsInt64("_token_version")
	}
	return result, nil
}

func defaultOAuthAuthRecovery(value string) string {
	if value == "" {
		return "skipped"
	}
	return value
}
func (s *OAuthSyncService) AuthRecoverySupported() bool { return s != nil && s.validator != nil }

func ProvideOAuthSyncService(repo OAuthSyncRepository, accounts AccountRepository, invalidator *CompositeTokenCacheInvalidator, projector *SchedulerSnapshotService, factory PrivacyClientFactory, proxies ProxyRepository, cfg *config.Config) *OAuthSyncService {
	svc := NewOAuthSyncService(repo, accounts, invalidator, projector)
	svc.validator = &oauthHTTPValidator{factory: factory, egress: newOpenAIEgressResolverWithAccountRepo(cfg, proxies, accounts)}
	svc.Start()
	return svc
}
