package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestSoftSpilloverConcurrencyThreshold(t *testing.T) {
	t.Parallel()

	require.Equal(t, 4, softSpilloverConcurrencyThreshold(5, 80))
	require.Equal(t, 3, softSpilloverConcurrencyThreshold(3, 80))
	require.Equal(t, 5, softSpilloverConcurrencyThreshold(5, 100))
	require.Equal(t, 1, softSpilloverConcurrencyThreshold(1, 80))
	require.Equal(t, 0, softSpilloverConcurrencyThreshold(0, 80))
}

func TestAccountLoadReachedSoftSpilloverIncludesWaiting(t *testing.T) {
	t.Parallel()

	load := &AccountLoadInfo{CurrentConcurrency: 3, WaitingCount: 1}
	require.True(t, accountLoadReachedSoftSpillover(load, 5, 80))
	require.False(t, accountLoadReachedSoftSpillover(load, 5, 100))
	require.False(t, accountLoadReachedSoftSpillover(nil, 5, 80))
}

func TestOpenAILegacyStickySoftSpilloverPreservesBinding(t *testing.T) {
	groupID := int64(102001)
	const sessionHash = "legacy-sticky-soft-spillover"
	sticky := Account{
		ID: 42001, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true, Concurrency: 5,
		AccountGroups: []AccountGroup{{AccountID: 42001, GroupID: groupID, Priority: 1}}, GroupIDs: []int64{groupID},
	}
	backup := Account{
		ID: 42002, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true, Concurrency: 5,
		AccountGroups: []AccountGroup{{AccountID: 42002, GroupID: groupID, Priority: 50}}, GroupIDs: []int64{groupID},
	}
	acquiredIDs := []int64{}
	cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:" + sessionHash: sticky.ID}}
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	cfg.Gateway.Scheduling.SoftSpilloverThresholdPercent = 80
	svc := &OpenAIGatewayService{
		accountRepo: schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: []Account{sticky, backup}}},
		cache:       cache,
		cfg:         cfg,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{
			loadMap: map[int64]*AccountLoadInfo{
				sticky.ID: {AccountID: sticky.ID, CurrentConcurrency: 4, LoadRate: 80},
				backup.ID: {AccountID: backup.ID, CurrentConcurrency: 0, LoadRate: 0},
			},
			acquiredIDs: &acquiredIDs,
		}),
	}

	selection, err := svc.SelectAccountWithLoadAwareness(
		bindingPriorityTestContext(groupID), &groupID, sessionHash, "gpt-5.1", nil,
	)

	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, backup.ID, selection.Account.ID)
	require.True(t, selection.Acquired)
	require.True(t, selection.PreserveStickyBinding)
	require.Equal(t, sticky.ID, cache.sessionBindings["openai:"+sessionHash])
	require.Equal(t, []int64{backup.ID}, acquiredIDs)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIAdvancedStickySoftSpilloverPreservesBinding(t *testing.T) {
	ctx := context.Background()
	groupID := int64(102002)
	const sessionHash = "advanced-sticky-soft-spillover"
	sticky := Account{
		ID: 42101, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true, Concurrency: 5, Priority: 1,
		GroupIDs: []int64{groupID},
	}
	backup := Account{
		ID: 42102, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true, Concurrency: 5, Priority: 50,
		GroupIDs: []int64{groupID},
	}
	acquiredIDs := []int64{}
	cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:" + sessionHash: sticky.ID}}
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	cfg.Gateway.Scheduling.SoftSpilloverThresholdPercent = 80
	svc := &OpenAIGatewayService{
		accountRepo:      schedulerTestOpenAIAccountRepo{accounts: []Account{sticky, backup}},
		cache:            cache,
		cfg:              cfg,
		rateLimitService: newOpenAIAdvancedSchedulerRateLimitService("true"),
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{
			loadMap: map[int64]*AccountLoadInfo{
				sticky.ID: {AccountID: sticky.ID, CurrentConcurrency: 4, LoadRate: 80},
				backup.ID: {AccountID: backup.ID, CurrentConcurrency: 0, LoadRate: 0},
			},
			acquiredIDs: &acquiredIDs,
		}),
	}

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx, &groupID, "", sessionHash, "gpt-5.1", nil, OpenAIUpstreamTransportAny, false,
	)

	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, backup.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
	require.False(t, decision.StickySessionHit)
	require.True(t, selection.PreserveStickyBinding)
	require.Equal(t, sticky.ID, cache.sessionBindings["openai:"+sessionHash])
	require.Equal(t, []int64{backup.ID}, acquiredIDs)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}
