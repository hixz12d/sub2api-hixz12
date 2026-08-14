package service

import (
	"context"
	"errors"
	"fmt"
)

// finalizeOpenAIAffinityColdWinner performs the database-backed cold-binding
// arbitration after a candidate has passed admission but before it can execute
// upstream. A losing candidate releases its slot and acquires the durable winner.
func (s *defaultOpenAIAccountScheduler) finalizeOpenAIAffinityColdWinner(
	ctx context.Context,
	req OpenAIAccountScheduleRequest,
	selection *AccountSelectionResult,
) (*AccountSelectionResult, error) {
	if s == nil || s.service == nil || selection == nil || selection.Account == nil {
		return selection, nil
	}
	value, enabled := openAIAffinityFromContext(ctx)
	if !selection.Account.IsOpenAIOAuth() || !enabled || !value.Writable || value.Identity.PrimaryHash == "" {
		return selection, nil
	}
	binding, _, err := s.service.createOrGetPersistentOpenAISession(ctx, selection.Account.ID)
	if errors.Is(err, ErrOpenAIAffinityNotFound) {
		return selection, nil
	}
	if err != nil {
		if selection.ReleaseFunc != nil {
			selection.ReleaseFunc()
		}
		return nil, err
	}
	if binding == nil || binding.AccountID == selection.Account.ID {
		return selection, nil
	}
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
	if req.ExcludedIDs != nil {
		if _, excluded := req.ExcludedIDs[binding.AccountID]; excluded {
			return nil, fmt.Errorf("%w: durable winner account %d is excluded", ErrOpenAIAffinityStateUnbound, binding.AccountID)
		}
	}
	winner, err := s.service.accountRepo.GetByID(ctx, binding.AccountID)
	if err != nil || winner == nil {
		return nil, fmt.Errorf("%w: load durable winner account %d", ErrOpenAIAffinityStateUnbound, binding.AccountID)
	}
	if winner.Status != StatusActive || !winner.IsOpenAICompatible() || winner.Platform != normalizeOpenAICompatiblePlatform(req.Platform) {
		return nil, fmt.Errorf("%w: durable winner account %d cannot continue", ErrOpenAIAffinityStateUnbound, binding.AccountID)
	}
	if !s.isAccountRequestCompatible(ctx, winner, req) || !s.isAccountTransportCompatible(winner, req.RequiredTransport) {
		return nil, fmt.Errorf("%w: durable winner account %d is incompatible", ErrOpenAIAffinityStateUnbound, binding.AccountID)
	}
	acquired, acquireErr := s.service.tryAcquireAccountSlot(ctx, winner.ID, winner.Concurrency)
	if acquireErr != nil {
		return nil, acquireErr
	}
	if acquired == nil || !acquired.Acquired {
		return attachSelectionProfitGate(ctx, &AccountSelectionResult{
			Account: winner,
			WaitPlan: &AccountWaitPlan{
				AccountID:      winner.ID,
				MaxConcurrency: winner.Concurrency,
				Timeout:        s.service.schedulingConfig().StickySessionWaitTimeout,
				MaxWaiting:     s.service.schedulingConfig().StickySessionMaxWaiting,
			},
		}), nil
	}
	return attachSelectionProfitGate(ctx, &AccountSelectionResult{
		Account:     winner,
		Acquired:    true,
		ReleaseFunc: acquired.ReleaseFunc,
	}), nil
}
