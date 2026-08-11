package service

import (
	"context"
	"sort"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

func (s *OpenAIGatewayService) resolveOpenAIAccountPriorityMode(ctx context.Context, groupID *int64) (string, error) {
	if groupID == nil || *groupID <= 0 {
		return OpenAIAccountPriorityModeGlobal, nil
	}
	if group, ok := ctx.Value(ctxkey.Group).(*Group); ok && IsGroupContextValid(group) && group.ID == *groupID {
		return NormalizeOpenAIAccountPriorityMode(group.Platform, group.OpenAIAccountPriorityMode), nil
	}
	if s != nil && s.schedulerSnapshot != nil {
		group, err := s.schedulerSnapshot.GetGroupByIDLite(ctx, *groupID)
		if err == nil && group != nil && group.Hydrated && group.ID == *groupID {
			return NormalizeOpenAIAccountPriorityMode(group.Platform, group.OpenAIAccountPriorityMode), nil
		}
	}
	// Unit tests and simple-mode deployments may not have a group snapshot.
	// Preserve the historical global behavior unless a trusted context group opts in.
	return OpenAIAccountPriorityModeGlobal, nil
}

func openAIAccountSchedulingPriority(account *Account, groupID *int64, mode string) (int, bool) {
	if account == nil {
		return 0, false
	}
	if mode != OpenAIAccountPriorityModeBinding {
		return account.Priority, true
	}
	if groupID == nil || *groupID <= 0 {
		return 0, false
	}
	for _, binding := range account.AccountGroups {
		if binding.GroupID == *groupID {
			return binding.Priority, true
		}
	}
	return 0, false
}

func openAIAccountBindingPriority(account *Account, groupID *int64) (int, bool) {
	return openAIAccountSchedulingPriority(account, groupID, OpenAIAccountPriorityModeBinding)
}

func sortOpenAIAccountsBySchedulingPriorityAndLastUsed(accounts []*Account, groupID *int64, mode string) {
	sort.SliceStable(accounts, func(i, j int) bool {
		leftPriority, leftOK := openAIAccountSchedulingPriority(accounts[i], groupID, mode)
		rightPriority, rightOK := openAIAccountSchedulingPriority(accounts[j], groupID, mode)
		if leftOK != rightOK {
			return leftOK
		}
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		left, right := accounts[i], accounts[j]
		switch {
		case left.LastUsedAt == nil && right.LastUsedAt != nil:
			return true
		case left.LastUsedAt != nil && right.LastUsedAt == nil:
			return false
		case left.LastUsedAt == nil && right.LastUsedAt == nil:
			return left.ID < right.ID
		default:
			if left.LastUsedAt.Equal(*right.LastUsedAt) {
				return left.ID < right.ID
			}
			return left.LastUsedAt.Before(*right.LastUsedAt)
		}
	})
}

// shuffleOpenAIWithinSortGroups preserves the binding-priority tier barrier.
// The generic shuffle groups by global account priority and may otherwise mix
// adjacent binding tiers that happen to share the same global sort values.
func shuffleOpenAIWithinSortGroups(accounts []accountWithLoad, groupID *int64, mode string) {
	if mode != OpenAIAccountPriorityModeBinding {
		shuffleWithinSortGroups(accounts)
		return
	}
	for start := 0; start < len(accounts); {
		priority, ok := openAIAccountBindingPriority(accounts[start].account, groupID)
		if !ok {
			start++
			continue
		}
		end := start + 1
		for end < len(accounts) {
			candidatePriority, candidateOK := openAIAccountBindingPriority(accounts[end].account, groupID)
			if !candidateOK || candidatePriority != priority {
				break
			}
			end++
		}
		shuffleWithinSortGroups(accounts[start:end])
		start = end
	}
}

func prioritizeOpenAICompactAccountsForScheduling(accounts []*Account, groupID *int64, mode string) []*Account {
	if mode != OpenAIAccountPriorityModeBinding {
		return prioritizeOpenAICompactAccounts(accounts)
	}
	ordered := append([]*Account(nil), accounts...)
	sort.SliceStable(ordered, func(i, j int) bool {
		leftPriority, leftOK := openAIAccountBindingPriority(ordered[i], groupID)
		rightPriority, rightOK := openAIAccountBindingPriority(ordered[j], groupID)
		if leftOK != rightOK {
			return leftOK
		}
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		return openAICompactSupportTier(ordered[i]) > openAICompactSupportTier(ordered[j])
	})
	return ordered
}

func (s *OpenAIGatewayService) isOpenAIStickyBindingPriorityCurrent(
	ctx context.Context,
	groupID *int64,
	platform string,
	sticky *Account,
	accounts []Account,
	requestedModel string,
	excludedIDs map[int64]struct{},
	requireCompact bool,
	requiredCapability OpenAIEndpointCapability,
) bool {
	mode, err := s.resolveOpenAIAccountPriorityMode(ctx, groupID)
	if err != nil {
		return false
	}
	if mode != OpenAIAccountPriorityModeBinding {
		return true
	}
	stickyPriority, ok := openAIAccountBindingPriority(sticky, groupID)
	if !ok {
		return false
	}
	if accounts == nil {
		accounts, err = s.listSchedulableAccounts(ctx, groupID, platform)
		if err != nil {
			return false
		}
	}
	best := 0
	found := false
	platform = normalizeOpenAICompatiblePlatform(platform)
	for i := range accounts {
		account := &accounts[i]
		if excludedIDs != nil {
			if _, excluded := excludedIDs[account.ID]; excluded {
				continue
			}
		}
		if !isOpenAICompatibleAccountEligibleForRequest(ctx, account, platform, requestedModel, requireCompact, requiredCapability) ||
			s.isOpenAIAccountRequestRuntimeBlocked(account, requestedModel) {
			continue
		}
		priority, ok := openAIAccountBindingPriority(account, groupID)
		if !ok {
			continue
		}
		if !found || priority < best {
			best = priority
			found = true
		}
	}
	return !found || stickyPriority <= best
}
