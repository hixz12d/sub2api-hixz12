package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type spilloverTestGatewayCache struct {
	*schedulerTestGatewayCache
	mu sync.Mutex

	lease *StickySpilloverState
	guard *StickySpilloverState

	claimCalls       int
	forceClaim       bool
	forcedClaimState *StickySpilloverState
	forcedOutcome    StickySpilloverClaimOutcome
	forcedClaimErr   error
}

func cloneSpilloverState(state *StickySpilloverState) *StickySpilloverState {
	if state == nil {
		return nil
	}
	copy := *state
	return &copy
}

func (c *spilloverTestGatewayCache) GetStickySpillover(context.Context, int64, string) (StickySpilloverSnapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return StickySpilloverSnapshot{Lease: cloneSpilloverState(c.lease), Guard: cloneSpilloverState(c.guard)}, nil
}

func (c *spilloverTestGatewayCache) ClaimStickySpillover(
	_ context.Context,
	_ int64,
	_ string,
	state StickySpilloverState,
	_, _ time.Duration,
) (*StickySpilloverState, StickySpilloverClaimOutcome, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.claimCalls++
	if c.forceClaim {
		if c.forcedClaimErr == nil && c.forcedOutcome == StickySpilloverClaimExisting && c.forcedClaimState != nil {
			c.lease = cloneSpilloverState(c.forcedClaimState)
			c.guard = cloneSpilloverState(c.forcedClaimState)
		}
		return cloneSpilloverState(c.forcedClaimState), c.forcedOutcome, c.forcedClaimErr
	}
	if c.lease != nil {
		return cloneSpilloverState(c.lease), StickySpilloverClaimExisting, nil
	}
	if c.guard != nil {
		return cloneSpilloverState(c.guard), StickySpilloverClaimBudget, nil
	}
	c.lease = cloneSpilloverState(&state)
	c.guard = cloneSpilloverState(&state)
	return cloneSpilloverState(&state), StickySpilloverClaimCreated, nil
}

func (c *spilloverTestGatewayCache) RestoreStickySpillover(
	_ context.Context,
	_ int64,
	_ string,
	expected StickySpilloverState,
	nowMS int64,
	_, _ time.Duration,
) (*StickySpilloverState, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lease != nil {
		return cloneSpilloverState(c.lease), false, nil
	}
	if c.guard == nil || c.guard.PrimaryAccountID != expected.PrimaryAccountID || c.guard.FallbackAccountID != expected.FallbackAccountID {
		return cloneSpilloverState(c.guard), false, nil
	}
	restored := *c.guard
	restored.LastUsedAtMS = nowMS
	c.lease = cloneSpilloverState(&restored)
	c.guard = cloneSpilloverState(&restored)
	return cloneSpilloverState(&restored), true, nil
}

func (c *spilloverTestGatewayCache) RefreshStickySpillover(
	_ context.Context,
	_ int64,
	_ string,
	expected StickySpilloverState,
	nowMS int64,
	_, _ time.Duration,
) (*StickySpilloverState, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lease == nil || c.lease.PrimaryAccountID != expected.PrimaryAccountID || c.lease.FallbackAccountID != expected.FallbackAccountID {
		return cloneSpilloverState(c.lease), false, nil
	}
	c.lease.LastUsedAtMS = nowMS
	c.guard = cloneSpilloverState(c.lease)
	return cloneSpilloverState(c.lease), true, nil
}

func (c *spilloverTestGatewayCache) InvalidateStickySpillover(
	_ context.Context,
	_ int64,
	_ string,
	expected StickySpilloverState,
) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lease == nil || c.lease.PrimaryAccountID != expected.PrimaryAccountID || c.lease.FallbackAccountID != expected.FallbackAccountID {
		return false, nil
	}
	c.lease = nil
	return true, nil
}

func (c *spilloverTestGatewayCache) ClearStickySpilloverGuard(
	_ context.Context,
	_ int64,
	_ string,
	expected StickySpilloverState,
) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lease != nil || c.guard == nil || c.guard.PrimaryAccountID != expected.PrimaryAccountID || c.guard.FallbackAccountID != expected.FallbackAccountID {
		return false, nil
	}
	c.guard = nil
	return true, nil
}

type spilloverSequenceConcurrencyCache struct {
	schedulerTestConcurrencyCache
	mu        sync.Mutex
	sequences map[int64][]*AccountLoadInfo
}

func (c *spilloverSequenceConcurrencyCache) GetAccountsLoadBatch(_ context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loadBatchErr != nil {
		return nil, c.loadBatchErr
	}
	loads := make(map[int64]*AccountLoadInfo, len(accounts))
	for _, account := range accounts {
		sequence := c.sequences[account.ID]
		if len(sequence) > 0 {
			loads[account.ID] = sequence[0]
			if len(sequence) > 1 {
				c.sequences[account.ID] = sequence[1:]
			}
			continue
		}
		if load := c.loadMap[account.ID]; load != nil {
			loads[account.ID] = load
			continue
		}
		loads[account.ID] = &AccountLoadInfo{AccountID: account.ID}
	}
	return loads, nil
}

type spilloverTestFixture struct {
	groupID     int64
	sessionHash string
	primary     Account
	fallback    Account
	third       Account
	cache       *spilloverTestGatewayCache
	concurrency *spilloverSequenceConcurrencyCache
	service     *OpenAIGatewayService
	acquiredIDs []int64
	releasedIDs []int64
}

func newSpilloverTestFixture(t *testing.T) *spilloverTestFixture {
	t.Helper()
	resetOpenAIAdvancedSchedulerSettingCacheForTest()

	fixture := &spilloverTestFixture{groupID: 104001, sessionHash: "phase-one-spillover"}
	fixture.primary = Account{
		ID: 44001, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive,
		Schedulable: true, Concurrency: 5, Priority: 0, GroupIDs: []int64{fixture.groupID},
		Extra: map[string]any{"openai_oauth_responses_websockets_v2_enabled": true},
	}
	fixture.fallback = Account{
		ID: 44002, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive,
		Schedulable: true, Concurrency: 5, Priority: 10, GroupIDs: []int64{fixture.groupID},
		Extra: map[string]any{"openai_oauth_responses_websockets_v2_enabled": true},
	}
	fixture.third = Account{
		ID: 44003, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive,
		Schedulable: true, Concurrency: 5, Priority: 100, GroupIDs: []int64{fixture.groupID},
		Extra: map[string]any{"openai_oauth_responses_websockets_v2_enabled": true},
	}
	baseCache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:" + fixture.sessionHash: fixture.primary.ID}}
	fixture.cache = &spilloverTestGatewayCache{schedulerTestGatewayCache: baseCache}
	fixture.concurrency = &spilloverSequenceConcurrencyCache{
		schedulerTestConcurrencyCache: schedulerTestConcurrencyCache{
			loadMap: map[int64]*AccountLoadInfo{
				fixture.primary.ID:  {AccountID: fixture.primary.ID, CurrentConcurrency: 4, LoadRate: 80},
				fixture.fallback.ID: {AccountID: fixture.fallback.ID, CurrentConcurrency: 0, LoadRate: 0},
				fixture.third.ID:    {AccountID: fixture.third.ID, CurrentConcurrency: 0, LoadRate: 0},
			},
			acquiredIDs: &fixture.acquiredIDs,
			releasedIDs: &fixture.releasedIDs,
		},
		sequences: make(map[int64][]*AccountLoadInfo),
	}
	cfg := newSchedulerTestOpenAIWSV2Config()
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Priority = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Queue = 1
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	cfg.Gateway.Scheduling.SoftSpilloverThresholdPercent = 80
	cfg.Gateway.Scheduling.SoftSpilloverGraceMS = 1
	cfg.Gateway.Scheduling.SoftSpilloverLeaseTTLSeconds = 600
	cfg.Gateway.Scheduling.SoftSpilloverReturnThresholdPercent = 50
	cfg.Gateway.Scheduling.SoftSpilloverMaxAccountsPerSession = 2
	cfg.Gateway.Scheduling.SoftSpilloverMaxSwitchesPer10M = 1
	cfg.Gateway.Scheduling.StickySessionMaxWaiting = 3
	cfg.Gateway.Scheduling.StickySessionWaitTimeout = time.Second
	cfg.Gateway.Scheduling.FallbackWaitTimeout = time.Second
	cfg.Gateway.Scheduling.FallbackMaxWaiting = 3
	fixture.service = &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: []Account{fixture.primary, fixture.fallback, fixture.third}},
		cache:              fixture.cache,
		cfg:                cfg,
		rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true"),
		concurrencyService: NewConcurrencyService(fixture.concurrency),
	}
	return fixture
}

func (f *spilloverTestFixture) selectAccount(ctx context.Context, previousResponseID string, excluded map[int64]struct{}) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	return f.service.SelectAccountWithScheduler(
		ctx, &f.groupID, previousResponseID, f.sessionHash, "gpt-5.1", excluded, OpenAIUpstreamTransportAny, false,
	)
}

func releaseFixtureSelection(selection *AccountSelectionResult) {
	if selection != nil && selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIStickySpilloverGraceRecoveryUsesPrimary(t *testing.T) {
	fixture := newSpilloverTestFixture(t)
	fixture.concurrency.sequences[fixture.primary.ID] = []*AccountLoadInfo{
		{AccountID: fixture.primary.ID, CurrentConcurrency: 4, LoadRate: 80},
		{AccountID: fixture.primary.ID, CurrentConcurrency: 2, LoadRate: 40},
	}

	selection, decision, err := fixture.selectAccount(context.Background(), "", nil)
	require.NoError(t, err)
	require.Equal(t, fixture.primary.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerSessionSticky, decision.Layer)
	require.Equal(t, 0, fixture.cache.claimCalls)
	require.Nil(t, fixture.cache.lease)
	releaseFixtureSelection(selection)
}

func TestOpenAIStickySpilloverSustainedFourOfFiveCreatesLeaseAndPreservesPrimary(t *testing.T) {
	fixture := newSpilloverTestFixture(t)
	fixture.concurrency.sequences[fixture.primary.ID] = []*AccountLoadInfo{
		{AccountID: fixture.primary.ID, CurrentConcurrency: 4, LoadRate: 80},
		{AccountID: fixture.primary.ID, CurrentConcurrency: 4, LoadRate: 80},
		{AccountID: fixture.primary.ID, CurrentConcurrency: 4, LoadRate: 80},
	}

	selection, decision, err := fixture.selectAccount(context.Background(), "", nil)
	require.NoError(t, err)
	require.Equal(t, fixture.fallback.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerSpillover, decision.Layer)
	require.True(t, selection.PreserveStickyBinding)
	require.Equal(t, fixture.primary.ID, fixture.cache.sessionBindings["openai:"+fixture.sessionHash])
	require.NotNil(t, fixture.cache.lease)
	require.Equal(t, fixture.primary.ID, fixture.cache.lease.PrimaryAccountID)
	require.Equal(t, fixture.fallback.ID, fixture.cache.lease.FallbackAccountID)
	require.Equal(t, 1, fixture.cache.lease.SwitchCount)
	releaseFixtureSelection(selection)
}

func TestOpenAIStickySpilloverLeasePinsFallback(t *testing.T) {
	fixture := newSpilloverTestFixture(t)
	now := time.Now().Add(-time.Minute).UnixMilli()
	state := &StickySpilloverState{
		PrimaryAccountID: fixture.primary.ID, FallbackAccountID: fixture.fallback.ID,
		CreatedAtMS: now, LastUsedAtMS: now, SwitchCount: 1,
	}
	fixture.cache.lease = cloneSpilloverState(state)
	fixture.cache.guard = cloneSpilloverState(state)
	fixture.concurrency.loadMap[fixture.primary.ID] = &AccountLoadInfo{AccountID: fixture.primary.ID, CurrentConcurrency: 0}

	for range 2 {
		selection, decision, err := fixture.selectAccount(context.Background(), "", nil)
		require.NoError(t, err)
		require.Equal(t, fixture.fallback.ID, selection.Account.ID)
		require.Equal(t, openAIAccountScheduleLayerSpillover, decision.Layer)
		require.True(t, selection.PreserveStickyBinding)
		releaseFixtureSelection(selection)
	}
	require.Equal(t, fixture.primary.ID, fixture.cache.sessionBindings["openai:"+fixture.sessionHash])
	require.Equal(t, []int64{fixture.fallback.ID, fixture.fallback.ID}, fixture.acquiredIDs)
}

func TestOpenAIStickySpilloverExpiredLeaseReturnsPrimaryBelowLowWater(t *testing.T) {
	fixture := newSpilloverTestFixture(t)
	state := &StickySpilloverState{
		PrimaryAccountID: fixture.primary.ID, FallbackAccountID: fixture.fallback.ID,
		CreatedAtMS:  time.Now().Add(-11 * time.Minute).UnixMilli(),
		LastUsedAtMS: time.Now().Add(-11 * time.Minute).UnixMilli(), SwitchCount: 1,
	}
	fixture.cache.guard = cloneSpilloverState(state)
	fixture.concurrency.sequences[fixture.primary.ID] = []*AccountLoadInfo{{
		AccountID: fixture.primary.ID, CurrentConcurrency: 2, LoadRate: 40,
	}}

	selection, decision, err := fixture.selectAccount(context.Background(), "", nil)
	require.NoError(t, err)
	require.Equal(t, fixture.primary.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerSessionSticky, decision.Layer)
	require.Nil(t, fixture.cache.guard)
	releaseFixtureSelection(selection)
}

func TestOpenAIStickySpilloverSwitchBudgetBlocksThirdAccount(t *testing.T) {
	fixture := newSpilloverTestFixture(t)
	state := &StickySpilloverState{
		PrimaryAccountID: fixture.primary.ID, FallbackAccountID: fixture.fallback.ID,
		CreatedAtMS: time.Now().UnixMilli(), LastUsedAtMS: time.Now().UnixMilli(), SwitchCount: 1,
	}
	fixture.cache.lease = cloneSpilloverState(state)
	fixture.cache.guard = cloneSpilloverState(state)

	selection, _, err := fixture.selectAccount(context.Background(), "", map[int64]struct{}{fixture.fallback.ID: {}})
	require.NoError(t, err)
	require.Equal(t, fixture.primary.ID, selection.Account.ID)
	require.NotContains(t, fixture.acquiredIDs, fixture.third.ID)
	require.Nil(t, fixture.cache.lease)
	require.NotNil(t, fixture.cache.guard, "invalidating the lease must retain the switch guard")
	releaseFixtureSelection(selection)
}

func TestOpenAIStickySpilloverPreviousResponseHardBindingWins(t *testing.T) {
	fixture := newSpilloverTestFixture(t)
	state := &StickySpilloverState{
		PrimaryAccountID: fixture.primary.ID, FallbackAccountID: fixture.fallback.ID,
		CreatedAtMS: time.Now().UnixMilli(), LastUsedAtMS: time.Now().UnixMilli(), SwitchCount: 1,
	}
	fixture.cache.lease = cloneSpilloverState(state)
	fixture.cache.guard = cloneSpilloverState(state)
	store := fixture.service.getOpenAIWSStateStore()
	require.NoError(t, store.BindResponseAccount(context.Background(), fixture.groupID, "resp_phase_one", fixture.primary.ID, time.Hour))

	selection, decision, err := fixture.selectAccount(context.Background(), "resp_phase_one", nil)
	require.NoError(t, err)
	require.Equal(t, fixture.primary.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerPreviousResponse, decision.Layer)
	require.True(t, decision.StickyPreviousHit)
	releaseFixtureSelection(selection)
}

func TestOpenAIStickySpilloverCASLoserReleasesCandidateSlot(t *testing.T) {
	fixture := newSpilloverTestFixture(t)
	fixture.concurrency.sequences[fixture.primary.ID] = []*AccountLoadInfo{
		{AccountID: fixture.primary.ID, CurrentConcurrency: 4, LoadRate: 80},
		{AccountID: fixture.primary.ID, CurrentConcurrency: 4, LoadRate: 80},
		{AccountID: fixture.primary.ID, CurrentConcurrency: 4, LoadRate: 80},
	}
	winner := &StickySpilloverState{
		PrimaryAccountID: fixture.primary.ID, FallbackAccountID: fixture.third.ID,
		CreatedAtMS: time.Now().UnixMilli(), LastUsedAtMS: time.Now().UnixMilli(), SwitchCount: 1,
	}
	fixture.cache.forceClaim = true
	fixture.cache.forcedClaimState = winner
	fixture.cache.forcedOutcome = StickySpilloverClaimExisting

	selection, decision, err := fixture.selectAccount(context.Background(), "", nil)
	require.NoError(t, err)
	require.Equal(t, fixture.third.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerSpillover, decision.Layer)
	require.Contains(t, fixture.releasedIDs, fixture.fallback.ID)
	require.Equal(t, []int64{fixture.fallback.ID, fixture.third.ID}, fixture.acquiredIDs)
	releaseFixtureSelection(selection)
}

func TestOpenAIStickySpilloverRedisClaimFailureDegradesWithoutRequestFailure(t *testing.T) {
	fixture := newSpilloverTestFixture(t)
	fixture.concurrency.sequences[fixture.primary.ID] = []*AccountLoadInfo{
		{AccountID: fixture.primary.ID, CurrentConcurrency: 4, LoadRate: 80},
		{AccountID: fixture.primary.ID, CurrentConcurrency: 4, LoadRate: 80},
		{AccountID: fixture.primary.ID, CurrentConcurrency: 4, LoadRate: 80},
	}
	fixture.cache.forceClaim = true
	fixture.cache.forcedClaimErr = errors.New("redis unavailable")

	selection, _, err := fixture.selectAccount(context.Background(), "", nil)
	require.NoError(t, err)
	require.Equal(t, fixture.fallback.ID, selection.Account.ID)
	require.True(t, selection.PreserveStickyBinding)
	releaseFixtureSelection(selection)
}

var _ StickySpilloverStore = (*spilloverTestGatewayCache)(nil)
var _ ConcurrencyCache = (*spilloverSequenceConcurrencyCache)(nil)
