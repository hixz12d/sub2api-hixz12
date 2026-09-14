package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStickySpilloverUnavailableAccountsReselectGroup(t *testing.T) {
	for _, schedulerMode := range []string{"false", "true"} {
		t.Run("advanced_scheduler_"+schedulerMode, func(t *testing.T) {
			fixture := newSpilloverTestFixture(t)
			fixture.service.rateLimitService = newOpenAIAdvancedSchedulerRateLimitService(schedulerMode)
			t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
			fixture.primary.Status, fixture.primary.Schedulable = StatusError, false
			fixture.fallback.Status, fixture.fallback.Schedulable = StatusError, false
			fixture.service.accountRepo = schedulerTestOpenAIAccountRepo{accounts: []Account{fixture.primary, fixture.fallback, fixture.third}}
			now := time.Now().UnixMilli()
			state := StickySpilloverState{PrimaryAccountID: fixture.primary.ID, FallbackAccountID: fixture.fallback.ID, CreatedAtMS: now, LastUsedAtMS: now, SwitchCount: 1}
			fixture.cache.lease, fixture.cache.guard = cloneSpilloverState(&state), cloneSpilloverState(&state)
			selection, _, err := fixture.selectAccount(context.Background(), "", nil)
			require.NoError(t, err)
			require.Equal(t, fixture.third.ID, selection.Account.ID)
			releaseFixtureSelection(selection)

			// Recreate the stale lease and prove fallback still respects failures
			// already excluded by this logical request.
			fixture.cache.sessionBindings["openai:"+fixture.sessionHash] = fixture.primary.ID
			fixture.cache.lease, fixture.cache.guard = cloneSpilloverState(&state), cloneSpilloverState(&state)
			selection, _, err = fixture.selectAccount(context.Background(), "", map[int64]struct{}{fixture.third.ID: {}})
			require.Error(t, err)
			require.Nil(t, selection)
		})
	}
}
