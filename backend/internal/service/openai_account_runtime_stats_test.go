package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenAIAccountRuntimeStats_StaleSamplesBecomeNeutral(t *testing.T) {
	stats := newOpenAIAccountRuntimeStats()
	now := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)
	ttft := 240

	stats.reportAt(1001, false, &ttft, now)
	errorRate, observedTTFT, hasTTFT := stats.snapshotAt(1001, now.Add(openAIAccountRuntimeStatsStaleAfter+time.Second))

	require.Zero(t, errorRate)
	require.Zero(t, observedTTFT)
	require.False(t, hasTTFT)

	stats.reportAt(1001, true, nil, now.Add(openAIAccountRuntimeStatsStaleAfter+2*time.Second))
	errorRate, _, hasTTFT = stats.snapshotAt(1001, now.Add(openAIAccountRuntimeStatsStaleAfter+2*time.Second))
	require.InDelta(t, 0.0, errorRate, 1e-9)
	require.False(t, hasTTFT)
}

func TestOpenAIAccountRuntimeStats_IsBoundedAndEvictsOldest(t *testing.T) {
	stats := newOpenAIAccountRuntimeStats(2)
	now := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)

	stats.reportAt(1, true, nil, now)
	stats.reportAt(2, true, nil, now.Add(time.Second))
	stats.reportAt(3, true, nil, now.Add(2*time.Second))

	require.Equal(t, 2, stats.size())
	_, accountOneStillTracked := stats.accounts.Load(int64(1))
	require.False(t, accountOneStillTracked, "the least recently touched account should be evicted")
	_, accountTwoStillTracked := stats.accounts.Load(int64(2))
	_, accountThreeStillTracked := stats.accounts.Load(int64(3))
	require.True(t, accountTwoStillTracked)
	require.True(t, accountThreeStillTracked)
}
