//go:build integration

package repository

// Migration 194 regression: every persisted group update invalidates API-key
// auth snapshots. This deliberately favors complete authorization/routing cache
// convergence over a fragile column allowlist as the group schema evolves.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAuthCacheInvalidationTrigger_ProfitControlColumns(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	group := mustCreateGroup(t, integrationEntClient, &service.Group{
		Name: fmt.Sprintf("profit-trigger-group-%d", suffix), Platform: service.PlatformOpenAI, RateMultiplier: 1,
	})
	user := mustCreateUser(t, integrationEntClient, &service.User{
		Email: fmt.Sprintf("profit-trigger-%d@example.com", suffix), Concurrency: 5,
	})
	groupID := group.ID
	keyValue := fmt.Sprintf("sk-profit-trigger-%d", suffix)
	apiKeyRepo := NewAPIKeyRepository(integrationEntClient, integrationDB)
	key := &service.APIKey{UserID: user.ID, GroupID: &groupID, Key: keyValue, Name: "profit-trigger", Status: service.StatusActive}
	require.NoError(t, apiKeyRepo.Create(ctx, key))

	sum := sha256.Sum256([]byte(keyValue))
	cacheKey := hex.EncodeToString(sum[:])
	clear := func() {
		_, err := integrationDB.ExecContext(ctx, "DELETE FROM auth_cache_invalidation_outbox WHERE cache_key = $1", cacheKey)
		require.NoError(t, err)
	}
	count := func() int {
		var value int
		require.NoError(t, integrationDB.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM auth_cache_invalidation_outbox WHERE cache_key = $1", cacheKey).Scan(&value))
		return value
	}
	clear()
	t.Cleanup(clear)
	t.Cleanup(func() {
		_, err := integrationDB.ExecContext(ctx, "DELETE FROM api_keys WHERE id = $1", key.ID)
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(ctx, "DELETE FROM users WHERE id = $1", user.ID)
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(ctx, "DELETE FROM groups WHERE id = $1", group.ID)
		require.NoError(t, err)
	})

	_, err := integrationDB.ExecContext(ctx, "UPDATE groups SET name = name || '-cosmetic' WHERE id = $1", group.ID)
	require.NoError(t, err)
	require.Equal(t, 1, count(), "every group update must invalidate auth snapshots")
	clear()

	_, err = integrationDB.ExecContext(ctx, "UPDATE groups SET profit_control_enabled = NOT profit_control_enabled WHERE id = $1", group.ID)
	require.NoError(t, err)
	require.Equal(t, 1, count(), "profit_control_enabled 变更必须入队")
	clear()

	_, err = integrationDB.ExecContext(ctx, "UPDATE groups SET profit_min_margin = 0.3 WHERE id = $1", group.ID)
	require.NoError(t, err)
	require.Equal(t, 1, count(), "profit_min_margin 变更必须入队")
	clear()

	_, err = integrationDB.ExecContext(ctx, "UPDATE groups SET profit_safety_buffer = 0.02 WHERE id = $1", group.ID)
	require.NoError(t, err)
	require.Equal(t, 1, count(), "profit_safety_buffer 变更必须入队")
	clear()

	_, err = integrationDB.ExecContext(ctx, "UPDATE groups SET profit_min_margin = profit_min_margin WHERE id = $1", group.ID)
	require.NoError(t, err)
	require.Equal(t, 1, count(), "even no-op SQL updates must invalidate auth snapshots")

	for name, update := range map[string]string{
		"platform":             "platform = 'anthropic'",
		"subscription_type":    "subscription_type = 'subscription'",
		"rate_multiplier":      "rate_multiplier = 0.9",
		"peak_rate_enabled":    "peak_rate_enabled = true",
		"peak_start":           "peak_start = '08:00'",
		"peak_end":             "peak_end = '09:00'",
		"peak_rate_multiplier": "peak_rate_multiplier = 1.2",
		"allow_live":           "allow_live = NOT allow_live",
	} {
		t.Run(name, func(t *testing.T) {
			clear()
			_, err := integrationDB.ExecContext(ctx, "UPDATE groups SET "+update+" WHERE id = $1", group.ID)
			require.NoError(t, err)
			require.Equal(t, 1, count(), name+" 变更必须入队")
		})
	}
}
