package service

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestBlueprintV2LegacyAdmissionWindow(t *testing.T) {
	for _, cfg := range []*config.Config{
		nil,
		{},
		{Gateway: config.GatewayConfig{OpenAIPreoutputRecoveryMode: "legacy"}},
	} {
		require.Equal(t, 20*time.Second, openAIRetryBudgetMaxElapsed(cfg))
	}
	budget := NewOpenAIRetryBudget(false)
	require.NoError(t, budget.Reserve(11))
	budget.mu.Lock()
	budget.startedAt = time.Now().Add(-21 * time.Second)
	budget.mu.Unlock()
	require.ErrorIs(t, budget.Reserve(11), ErrOpenAIRetryBudgetExhausted)
	require.Equal(t, 1, budget.Snapshot().Attempts, "denied admission must not count as dispatch")
}

func TestBlueprintV2BoundedAdmissionKeepsExistingWindow(t *testing.T) {
	cfg := &config.Config{Gateway: config.GatewayConfig{OpenAIPreoutputRecoveryMode: "bounded_preoutput"}}
	budget := newOpenAIRetryBudget(false, false, openAIRetryBudgetMaxElapsed(cfg))
	require.NoError(t, budget.Reserve(11))
	budget.mu.Lock()
	budget.startedAt = time.Now().Add(-35 * time.Second)
	budget.mu.Unlock()
	require.NoError(t, budget.Reserve(11))
	require.Equal(t, 2, budget.Snapshot().Attempts)
	require.ErrorIs(t, budget.Reserve(11), ErrOpenAIRetryBudgetExhausted)
}
