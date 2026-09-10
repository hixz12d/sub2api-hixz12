//go:build unit

package repository

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestMonitorBudgetLimits(t *testing.T) {
	valid := []MonitorBudgetLimit{{Scope: "user", ScopeID: 7, RequestLimit: 300}, {Scope: "global", RequestLimit: 10000}}
	normalized, err := normalizeMonitorBudgetLimits(valid)
	require.NoError(t, err)
	require.Equal(t, "global", normalized[0].Scope)
	require.Equal(t, "user", valid[0].Scope, "do not mutate caller configuration")
	require.NoError(t, validateMonitorBudgetScopes(monitorBudgetJob{owner: sql.NullInt64{Int64: 7, Valid: true}}, normalized))
	require.ErrorIs(t, validateMonitorBudgetScopes(monitorBudgetJob{owner: sql.NullInt64{Int64: 8, Valid: true}}, normalized), ErrMonitorBudgetInput)
	require.ErrorIs(t, validateMonitorBudgetScopes(monitorBudgetJob{group: sql.NullInt64{Int64: 7, Valid: true}}, normalized), ErrMonitorBudgetInput)
	for _, limits := range [][]MonitorBudgetLimit{
		nil,
		{{Scope: "global", RequestLimit: 100}},
		{{Scope: "user", ScopeID: 7, RequestLimit: 100}, {Scope: "group", ScopeID: 8, RequestLimit: 100}},
		{{Scope: "global", ScopeID: 1}, {Scope: "user", ScopeID: 7}},
		{{Scope: "global"}, {Scope: "user", ScopeID: 0}},
		{{Scope: "global"}, {Scope: "user", ScopeID: 7, RequestLimit: -1}},
		{{Scope: "global"}, {Scope: "user", ScopeID: 7, RequestLimit: 1000000001}},
		{{Scope: "global"}, {Scope: "invalid", ScopeID: 7}},
		{{Scope: "global"}, {Scope: "user", ScopeID: 7}, {Scope: "user", ScopeID: 8}},
	} {
		_, err := normalizeMonitorBudgetLimits(limits)
		require.ErrorIs(t, err, ErrMonitorBudgetInput)
	}
}

func TestMonitorBudgetRejectsInvalidBeforeDB(t *testing.T) {
	r := NewMonitorBudgetRepository(nil)
	ctx := context.Background()
	require.ErrorIs(t, r.Reserve(ctx, "bad", nil), ErrMonitorBudgetInput)
	require.ErrorIs(t, r.Reserve(ctx, uuid.NewString(), nil), ErrMonitorBudgetInput)
	require.ErrorIs(t, r.Consume(ctx, uuid.NewString(), "worker", 0, 0, nil), ErrMonitorBudgetInput)
	require.ErrorIs(t, r.Consume(ctx, uuid.NewString(), "worker", 1, -1, nil), ErrMonitorBudgetInput)
	require.ErrorIs(t, r.Settle(ctx, "bad"), ErrMonitorBudgetInput)
}
