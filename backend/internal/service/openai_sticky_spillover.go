package service

import (
	"context"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"
)

const (
	defaultSoftSpilloverGrace           = time.Second
	defaultSoftSpilloverLeaseTTL        = 10 * time.Minute
	defaultSoftSpilloverReturnPercent   = 50
	softSpilloverSwitchBudgetWindow     = 10 * time.Minute
	openAIAccountScheduleLayerSpillover = "spillover_lease"
	stickySpilloverRedisWarningInterval = 30 * time.Second
)

type StickySpilloverState struct {
	PrimaryAccountID  int64 `json:"primary_account_id,string"`
	FallbackAccountID int64 `json:"fallback_account_id,string"`
	CreatedAtMS       int64 `json:"created_at,string"`
	LastUsedAtMS      int64 `json:"last_used_at,string"`
	SwitchCount       int   `json:"switch_count"`
}

type StickySpilloverSnapshot struct {
	Lease *StickySpilloverState
	Guard *StickySpilloverState
}

type StickySpilloverClaimOutcome string

const (
	StickySpilloverClaimCreated  StickySpilloverClaimOutcome = "created"
	StickySpilloverClaimExisting StickySpilloverClaimOutcome = "existing"
	StickySpilloverClaimBudget   StickySpilloverClaimOutcome = "budget_exhausted"
)

// StickySpilloverStore is optional so existing GatewayCache test doubles and
// non-OpenAI scheduling paths do not inherit the spillover behavior surface.
type StickySpilloverStore interface {
	GetStickySpillover(ctx context.Context, groupID int64, sessionHash string) (StickySpilloverSnapshot, error)
	ClaimStickySpillover(ctx context.Context, groupID int64, sessionHash string, state StickySpilloverState, leaseTTL, guardTTL time.Duration) (*StickySpilloverState, StickySpilloverClaimOutcome, error)
	RestoreStickySpillover(ctx context.Context, groupID int64, sessionHash string, expected StickySpilloverState, nowMS int64, leaseTTL, guardTTL time.Duration) (*StickySpilloverState, bool, error)
	RefreshStickySpillover(ctx context.Context, groupID int64, sessionHash string, expected StickySpilloverState, nowMS int64, leaseTTL, guardTTL time.Duration) (*StickySpilloverState, bool, error)
	InvalidateStickySpillover(ctx context.Context, groupID int64, sessionHash string, expected StickySpilloverState) (bool, error)
	ClearStickySpilloverGuard(ctx context.Context, groupID int64, sessionHash string, expected StickySpilloverState) (bool, error)
}

type openAIStickySpilloverRequest struct {
	groupID                 *int64
	previousResponseID      string
	sessionHash             string
	requestedModel          string
	excludedIDs             map[int64]struct{}
	requiredTransport       OpenAIUpstreamTransport
	requiredCapability      OpenAIEndpointCapability
	requiredImageCapability OpenAIImagesCapability
	requireCompact          bool
	platform                string
	previousResponseCanMove bool
}

type openAIStickySpilloverNext func() (*AccountSelectionResult, OpenAIAccountScheduleDecision, error)

var stickySpilloverLastRedisWarningMS atomic.Int64

func (s *OpenAIGatewayService) spilloverGrace() time.Duration {
	value := s.schedulingConfig().SoftSpilloverGraceMS
	if value <= 0 {
		return defaultSoftSpilloverGrace
	}
	return time.Duration(value) * time.Millisecond
}

func (s *OpenAIGatewayService) spilloverLeaseTTL() time.Duration {
	value := s.schedulingConfig().SoftSpilloverLeaseTTLSeconds
	if value <= 0 {
		return defaultSoftSpilloverLeaseTTL
	}
	return time.Duration(value) * time.Second
}

func (s *OpenAIGatewayService) spilloverReturnPercent() int {
	value := s.schedulingConfig().SoftSpilloverReturnThresholdPercent
	if value <= 0 {
		return defaultSoftSpilloverReturnPercent
	}
	return value
}

func (s *OpenAIGatewayService) selectAccountWithStickySpillover(
	ctx context.Context,
	req openAIStickySpilloverRequest,
	next openAIStickySpilloverNext,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	if s == nil || next == nil || normalizeOpenAICompatiblePlatform(req.platform) != PlatformOpenAI ||
		strings.TrimSpace(req.sessionHash) == "" || s.cache == nil || s.concurrencyService == nil {
		return next()
	}
	if owner, ok := openAIWSStateOwnerFromContext(ctx); ok && owner.AccountID > 0 {
		return next()
	}
	if strings.TrimSpace(req.previousResponseID) != "" &&
		(!s.isOpenAIAdvancedSchedulerStickyWeightedEnabled(ctx) || !req.previousResponseCanMove) {
		return next()
	}

	store, ok := s.cache.(StickySpilloverStore)
	if !ok {
		return next()
	}
	primaryID, err := s.getStickySessionAccountID(ctx, req.groupID, req.sessionHash)
	if err != nil || primaryID <= 0 {
		if err != nil && err != ErrStickySessionNotFound {
			logStickySpilloverRedisFailure("read_primary", err)
		}
		return next()
	}
	primary, err := s.getSchedulableAccount(ctx, primaryID)
	if err != nil || primary == nil || primary.Platform != PlatformOpenAI || primary.Type != AccountTypeOAuth {
		return next()
	}

	groupID := derefGroupID(req.groupID)
	leaseTTL := s.spilloverLeaseTTL()
	guardTTL := leaseTTL + softSpilloverSwitchBudgetWindow
	snapshot, err := store.GetStickySpillover(ctx, groupID, req.sessionHash)
	if err != nil {
		logStickySpilloverRedisFailure("read_lease", err)
		return next()
	}
	if state := snapshot.Lease; state != nil {
		if state.PrimaryAccountID != primaryID {
			_, _ = store.InvalidateStickySpillover(ctx, groupID, req.sessionHash, *state)
			return next()
		}
		return s.useStickySpilloverState(ctx, req, store, *state, leaseTTL, guardTTL, primary)
	}
	if state := snapshot.Guard; state != nil {
		if state.PrimaryAccountID != primaryID {
			_, _ = store.ClearStickySpilloverGuard(ctx, groupID, req.sessionHash, *state)
			return next()
		}
		return s.handleExpiredStickySpillover(ctx, req, store, *state, leaseTTL, guardTTL, primary, next)
	}

	cfg := s.schedulingConfig()
	reached, load := fetchAccountSoftSpilloverState(
		ctx, s.concurrencyService, primaryID, primary.Concurrency, cfg.SoftSpilloverThresholdPercent,
	)
	if !reached {
		return next()
	}
	slog.Debug("sticky_spillover_grace_started",
		"primary_account_id", primaryID,
		"current_concurrency", load.CurrentConcurrency,
		"waiting_count", load.WaitingCount,
		"max_concurrency", primary.Concurrency,
	)
	if !waitStickySpilloverGrace(ctx, s.spilloverGrace()) {
		return nil, OpenAIAccountScheduleDecision{}, ctx.Err()
	}
	reached, load, freshLoadErr := fetchAccountSoftSpilloverStateFresh(
		ctx, s.concurrencyService, primaryID, primary.Concurrency, cfg.SoftSpilloverThresholdPercent,
	)
	if freshLoadErr != nil {
		logStickySpilloverRedisFailure("fresh_load", freshLoadErr)
		return next()
	}
	if !reached {
		slog.Debug("sticky_spillover_grace_recovered",
			"primary_account_id", primaryID,
			"current_concurrency", spilloverLoadConcurrency(load),
			"waiting_count", spilloverLoadWaiting(load),
			"max_concurrency", primary.Concurrency,
		)
		selection, valid, routeErr := s.selectSpecificOpenAIOAuthAccount(ctx, req, primary, false)
		if routeErr != nil {
			return nil, OpenAIAccountScheduleDecision{}, routeErr
		}
		if valid {
			return selection, spilloverDecision(openAIAccountScheduleLayerSessionSticky, selection, true), nil
		}
		return next()
	}

	selection, decision, err := next()
	if err != nil || selection == nil || selection.Account == nil || selection.Account.ID == primaryID {
		return selection, decision, err
	}
	fallback := selection.Account
	if fallback.Platform != PlatformOpenAI || fallback.Type != AccountTypeOAuth {
		return selection, decision, err
	}
	selection.PreserveStickyBinding = true
	nowMS := time.Now().UnixMilli()
	candidateState := StickySpilloverState{
		PrimaryAccountID:  primaryID,
		FallbackAccountID: fallback.ID,
		CreatedAtMS:       nowMS,
		LastUsedAtMS:      nowMS,
		SwitchCount:       1,
	}
	state, outcome, claimErr := store.ClaimStickySpillover(
		ctx, groupID, req.sessionHash, candidateState, leaseTTL, guardTTL,
	)
	if claimErr != nil {
		logStickySpilloverRedisFailure("claim", claimErr)
		return selection, decision, nil
	}
	switch outcome {
	case StickySpilloverClaimCreated:
		slog.Info("sticky_spillover_lease_created",
			"primary_account_id", primaryID,
			"fallback_account_id", fallback.ID,
			"current_concurrency", spilloverLoadConcurrency(load),
			"waiting_count", spilloverLoadWaiting(load),
			"max_concurrency", primary.Concurrency,
		)
		decision.Layer = openAIAccountScheduleLayerSpillover
		return selection, decision, nil
	case StickySpilloverClaimExisting:
		if state != nil && state.PrimaryAccountID == primaryID && state.FallbackAccountID == fallback.ID {
			decision.Layer = openAIAccountScheduleLayerSpillover
			return selection, decision, nil
		}
		releaseSpilloverSelection(selection)
		if state != nil && state.PrimaryAccountID == primaryID {
			return s.useStickySpilloverState(ctx, req, store, *state, leaseTTL, guardTTL, primary)
		}
		return s.selectPrimaryAfterSpilloverBlocked(ctx, req, primary)
	default:
		releaseSpilloverSelection(selection)
		slog.Info("sticky_spillover_switch_budget_exhausted",
			"primary_account_id", primaryID,
			"fallback_account_id", fallback.ID,
			"reason", string(outcome),
		)
		return s.selectPrimaryAfterSpilloverBlocked(ctx, req, primary)
	}
}

func (s *OpenAIGatewayService) handleExpiredStickySpillover(
	ctx context.Context,
	req openAIStickySpilloverRequest,
	store StickySpilloverStore,
	state StickySpilloverState,
	leaseTTL, guardTTL time.Duration,
	primary *Account,
	next openAIStickySpilloverNext,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	belowReturn, loadErr := fetchAccountBelowSoftSpilloverThresholdFresh(
		ctx, s.concurrencyService, primary.ID, primary.Concurrency, s.spilloverReturnPercent(),
	)
	if loadErr == nil && belowReturn {
		cleared, err := store.ClearStickySpilloverGuard(ctx, derefGroupID(req.groupID), req.sessionHash, state)
		if err != nil {
			logStickySpilloverRedisFailure("clear_expired_guard", err)
			return next()
		}
		if cleared {
			slog.Info("sticky_spillover_lease_expired",
				"primary_account_id", state.PrimaryAccountID,
				"fallback_account_id", state.FallbackAccountID,
				"reason", "primary_below_return_threshold",
			)
			selection, valid, routeErr := s.selectSpecificOpenAIOAuthAccount(ctx, req, primary, false)
			if routeErr != nil {
				return nil, OpenAIAccountScheduleDecision{}, routeErr
			}
			if valid {
				return selection, spilloverDecision(openAIAccountScheduleLayerSessionSticky, selection, true), nil
			}
			return next()
		}
	}

	current, restored, err := store.RestoreStickySpillover(
		ctx, derefGroupID(req.groupID), req.sessionHash, state, time.Now().UnixMilli(), leaseTTL, guardTTL,
	)
	if err != nil {
		logStickySpilloverRedisFailure("restore", err)
		return next()
	}
	if current == nil || current.PrimaryAccountID != primary.ID {
		return s.selectPrimaryAfterSpilloverBlocked(ctx, req, primary)
	}
	if restored {
		slog.Info("sticky_spillover_lease_created",
			"primary_account_id", current.PrimaryAccountID,
			"fallback_account_id", current.FallbackAccountID,
			"reason", "primary_above_return_threshold",
		)
	}
	return s.useStickySpilloverState(ctx, req, store, *current, leaseTTL, guardTTL, primary)
}

func (s *OpenAIGatewayService) useStickySpilloverState(
	ctx context.Context,
	req openAIStickySpilloverRequest,
	store StickySpilloverStore,
	state StickySpilloverState,
	leaseTTL, guardTTL time.Duration,
	primary *Account,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	fallback, err := s.getSchedulableAccount(ctx, state.FallbackAccountID)
	if err != nil || fallback == nil {
		s.invalidateStickySpillover(ctx, req, store, state, "fallback_missing")
		return s.selectPrimaryAfterSpilloverBlocked(ctx, req, primary)
	}
	selection, valid, routeErr := s.selectSpecificOpenAIOAuthAccount(ctx, req, fallback, true)
	if routeErr != nil {
		return nil, OpenAIAccountScheduleDecision{}, routeErr
	}
	if !valid {
		s.invalidateStickySpillover(ctx, req, store, state, "fallback_ineligible")
		return s.selectPrimaryAfterSpilloverBlocked(ctx, req, primary)
	}

	current, refreshed, refreshErr := store.RefreshStickySpillover(
		ctx, derefGroupID(req.groupID), req.sessionHash, state, time.Now().UnixMilli(), leaseTTL, guardTTL,
	)
	if refreshErr != nil {
		logStickySpilloverRedisFailure("refresh", refreshErr)
		return selection, spilloverDecision(openAIAccountScheduleLayerSpillover, selection, false), nil
	}
	if !refreshed {
		releaseSpilloverSelection(selection)
		if current != nil && current.PrimaryAccountID == state.PrimaryAccountID && current.FallbackAccountID != state.FallbackAccountID {
			winner, getErr := s.getSchedulableAccount(ctx, current.FallbackAccountID)
			if getErr == nil && winner != nil {
				winnerSelection, winnerValid, winnerErr := s.selectSpecificOpenAIOAuthAccount(ctx, req, winner, true)
				if winnerErr != nil {
					return nil, OpenAIAccountScheduleDecision{}, winnerErr
				}
				if winnerValid {
					_, _, _ = store.RefreshStickySpillover(ctx, derefGroupID(req.groupID), req.sessionHash, *current, time.Now().UnixMilli(), leaseTTL, guardTTL)
					return winnerSelection, spilloverDecision(openAIAccountScheduleLayerSpillover, winnerSelection, false), nil
				}
			}
		}
		return s.selectPrimaryAfterSpilloverBlocked(ctx, req, primary)
	}
	slog.Debug("sticky_spillover_lease_hit",
		"primary_account_id", state.PrimaryAccountID,
		"fallback_account_id", state.FallbackAccountID,
		"lease_age_ms", time.Now().UnixMilli()-state.CreatedAtMS,
	)
	return selection, spilloverDecision(openAIAccountScheduleLayerSpillover, selection, false), nil
}

func (s *OpenAIGatewayService) invalidateStickySpillover(
	ctx context.Context,
	req openAIStickySpilloverRequest,
	store StickySpilloverStore,
	state StickySpilloverState,
	reason string,
) {
	invalidated, err := store.InvalidateStickySpillover(ctx, derefGroupID(req.groupID), req.sessionHash, state)
	if err != nil {
		logStickySpilloverRedisFailure("invalidate", err)
		return
	}
	if invalidated {
		slog.Info("sticky_spillover_lease_invalidated",
			"primary_account_id", state.PrimaryAccountID,
			"fallback_account_id", state.FallbackAccountID,
			"reason", reason,
		)
	}
}

func (s *OpenAIGatewayService) selectPrimaryAfterSpilloverBlocked(
	ctx context.Context,
	req openAIStickySpilloverRequest,
	primary *Account,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	selection, valid, err := s.selectSpecificOpenAIOAuthAccount(ctx, req, primary, false)
	if err != nil {
		return nil, OpenAIAccountScheduleDecision{}, err
	}
	if !valid {
		return nil, OpenAIAccountScheduleDecision{}, ErrNoAvailableAccounts
	}
	return selection, spilloverDecision(openAIAccountScheduleLayerSessionSticky, selection, true), nil
}

func (s *OpenAIGatewayService) selectSpecificOpenAIOAuthAccount(
	ctx context.Context,
	req openAIStickySpilloverRequest,
	account *Account,
	preserveSticky bool,
) (*AccountSelectionResult, bool, error) {
	if account == nil || account.ID <= 0 || account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth || !account.IsSchedulableForModel(req.requestedModel) {
		return nil, false, nil
	}
	if req.excludedIDs != nil {
		if _, excluded := req.excludedIDs[account.ID]; excluded {
			return nil, false, nil
		}
	}
	checkReq := OpenAIAccountScheduleRequest{
		GroupID:                 req.groupID,
		Platform:                req.platform,
		RequestedModel:          req.requestedModel,
		RequiredTransport:       req.requiredTransport,
		RequiredCapability:      req.requiredCapability,
		RequiredImageCapability: req.requiredImageCapability,
		RequireCompact:          req.requireCompact,
	}
	checker := &defaultOpenAIAccountScheduler{service: s}
	if !checker.isAccountRequestCompatible(ctx, account, checkReq) || !checker.isAccountTransportCompatible(account, req.requiredTransport) {
		return nil, false, nil
	}
	fresh := s.resolveFreshSchedulableOpenAIAccount(ctx, account, PlatformOpenAI, req.requestedModel, false, req.requiredCapability)
	if fresh == nil || fresh.Type != AccountTypeOAuth || !checker.isAccountRequestCompatible(ctx, fresh, checkReq) || !checker.isAccountTransportCompatible(fresh, req.requiredTransport) {
		return nil, false, nil
	}
	fresh = s.recheckSelectedOpenAIAccountFromDB(ctx, fresh, req.groupID, PlatformOpenAI, req.requestedModel, req.requireCompact, req.requiredCapability)
	if fresh == nil || fresh.Type != AccountTypeOAuth || !s.openAIAccountMatchesSchedulingGroup(fresh, req.groupID) ||
		!checker.isAccountRequestCompatible(ctx, fresh, checkReq) || !checker.isAccountTransportCompatible(fresh, req.requiredTransport) {
		return nil, false, nil
	}
	result, acquireErr := s.tryAcquireAccountSlot(ctx, fresh.ID, fresh.Concurrency)
	if acquireErr == nil && result != nil && result.Acquired {
		selection, err := s.newAcquiredSelectionResult(ctx, fresh, result.ReleaseFunc)
		if selection != nil {
			selection.PreserveStickyBinding = preserveSticky
		}
		return selection, true, err
	}
	cfg := s.schedulingConfig()
	selection, err := s.newSelectionResult(ctx, fresh, false, nil, &AccountWaitPlan{
		AccountID:      fresh.ID,
		MaxConcurrency: fresh.Concurrency,
		Timeout:        cfg.StickySessionWaitTimeout,
		MaxWaiting:     cfg.StickySessionMaxWaiting,
	})
	if selection != nil {
		selection.PreserveStickyBinding = preserveSticky
	}
	return selection, true, err
}

func spilloverDecision(layer string, selection *AccountSelectionResult, sticky bool) OpenAIAccountScheduleDecision {
	decision := OpenAIAccountScheduleDecision{Layer: layer, StickySessionHit: sticky}
	if selection != nil && selection.Account != nil {
		decision.SelectedAccountID = selection.Account.ID
		decision.SelectedAccountType = selection.Account.Type
	}
	return decision
}

func waitStickySpilloverGrace(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return true
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func releaseSpilloverSelection(selection *AccountSelectionResult) {
	if selection != nil && selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
		selection.ReleaseFunc = nil
		selection.Acquired = false
	}
}

func spilloverLoadConcurrency(load *AccountLoadInfo) int {
	if load == nil {
		return 0
	}
	return load.CurrentConcurrency
}

func spilloverLoadWaiting(load *AccountLoadInfo) int {
	if load == nil {
		return 0
	}
	return load.WaitingCount
}

func logStickySpilloverRedisFailure(operation string, err error) {
	if err == nil {
		return
	}
	now := time.Now().UnixMilli()
	last := stickySpilloverLastRedisWarningMS.Load()
	if now-last >= stickySpilloverRedisWarningInterval.Milliseconds() && stickySpilloverLastRedisWarningMS.CompareAndSwap(last, now) {
		slog.Warn("sticky_spillover_redis_failed", "operation", operation, "error", err)
		return
	}
	slog.Debug("sticky_spillover_redis_failed", "operation", operation, "error", err)
}
