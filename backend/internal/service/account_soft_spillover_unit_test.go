//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGatewayStickySoftSpilloverPreservesBinding(t *testing.T) {
	ctx := context.Background()
	const sessionHash = "anthropic-sticky-soft-spillover"
	sticky := Account{ID: 43001, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5}
	backup := Account{ID: 43002, Platform: PlatformAnthropic, Priority: 50, Status: StatusActive, Schedulable: true, Concurrency: 5}
	repo := &mockAccountRepoForPlatform{
		accounts:     []Account{sticky, backup},
		accountsByID: map[int64]*Account{sticky.ID: &sticky, backup.ID: &backup},
	}
	cache := &mockGatewayCacheForPlatform{sessionBindings: map[string]int64{sessionHash: sticky.ID}}
	cfg := testConfig()
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	cfg.Gateway.Scheduling.SoftSpilloverThresholdPercent = 80
	concurrencyCache := &mockConcurrencyCache{
		loadMap: map[int64]*AccountLoadInfo{
			sticky.ID: {AccountID: sticky.ID, CurrentConcurrency: 4, LoadRate: 80},
			backup.ID: {AccountID: backup.ID, CurrentConcurrency: 0, LoadRate: 0},
		},
	}
	svc := &GatewayService{
		accountRepo:        repo,
		cache:              cache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(concurrencyCache),
	}

	selection, err := svc.SelectAccountWithLoadAwareness(ctx, nil, sessionHash, "claude-3-5-sonnet-20241022", nil, "", 0)

	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, backup.ID, selection.Account.ID)
	require.True(t, selection.Acquired)
	require.True(t, selection.PreserveStickyBinding)
	require.Equal(t, sticky.ID, cache.sessionBindings[sessionHash])
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestGatewayStickySoftSpilloverFallsBackToStickyWhenNoAlternative(t *testing.T) {
	ctx := context.Background()
	const sessionHash = "anthropic-sticky-soft-spillover-only-account"
	sticky := Account{ID: 43101, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5}
	repo := &mockAccountRepoForPlatform{
		accounts:     []Account{sticky},
		accountsByID: map[int64]*Account{sticky.ID: &sticky},
	}
	cache := &mockGatewayCacheForPlatform{sessionBindings: map[string]int64{sessionHash: sticky.ID}}
	cfg := testConfig()
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	cfg.Gateway.Scheduling.SoftSpilloverThresholdPercent = 80
	concurrencyCache := &mockConcurrencyCache{
		loadMap: map[int64]*AccountLoadInfo{
			sticky.ID: {AccountID: sticky.ID, CurrentConcurrency: 4, LoadRate: 80},
		},
	}
	svc := &GatewayService{
		accountRepo:        repo,
		cache:              cache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(concurrencyCache),
	}

	selection, err := svc.SelectAccountWithLoadAwareness(ctx, nil, sessionHash, "claude-3-5-sonnet-20241022", nil, "", 0)

	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, sticky.ID, selection.Account.ID)
	require.True(t, selection.Acquired)
	require.False(t, selection.PreserveStickyBinding)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}
