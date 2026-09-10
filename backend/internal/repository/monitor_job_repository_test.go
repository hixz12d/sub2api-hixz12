//go:build unit

package repository

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestMonitorJobInputsRejectBeforeDB(t *testing.T) {
	r := NewMonitorJobRepository(nil)
	ctx := context.Background()
	_, err := r.Claim(ctx, service.MonitorJobKind("unknown"), "worker")
	require.ErrorIs(t, err, ErrMonitorBudgetInput)
	_, err = r.Claim(ctx, service.MonitorJobCapability, "")
	require.ErrorIs(t, err, ErrMonitorBudgetInput)
	_, err = r.Renew(ctx, MonitorJobLease{})
	require.ErrorIs(t, err, ErrMonitorBudgetInput)
	require.ErrorIs(t, r.Finish(ctx, MonitorJobLease{}, service.MonitorJobCompleted), ErrMonitorBudgetInput)
	require.ErrorIs(t, r.CancelForOwner(ctx, uuid.NewString(), 0), sql.ErrNoRows)
	valid := CreateMonitorJobInput{PlanID: uuid.NewString(), OwnerID: 1, IdempotencyKey: "key", ConfigurationHash: strings.Repeat("a", 64), Limits: []MonitorBudgetLimit{{Scope: "global", RequestLimit: 100}, {Scope: "user", ScopeID: 1, RequestLimit: 100}}}
	for _, mutate := range []func(*CreateMonitorJobInput){
		func(i *CreateMonitorJobInput) { i.OwnerID = 0 },
		func(i *CreateMonitorJobInput) { i.PlanID = "malformed" },
		func(i *CreateMonitorJobInput) { i.IdempotencyKey = "" },
		func(i *CreateMonitorJobInput) { i.IdempotencyKey = " padded " },
		func(i *CreateMonitorJobInput) { i.IdempotencyKey = strings.Repeat("x", 201) },
		func(i *CreateMonitorJobInput) { i.ConfigurationHash = strings.Repeat("A", 64) },
		func(i *CreateMonitorJobInput) { i.ConfigurationHash = strings.Repeat("g", 64) },
		func(i *CreateMonitorJobInput) { i.Limits = nil },
		func(i *CreateMonitorJobInput) { i.SecretOwnerInstanceID = strings.Repeat("x", 201) },
	} {
		input := valid
		mutate(&input)
		_, err := r.CreateFromPlan(ctx, input)
		require.ErrorIs(t, err, ErrMonitorBudgetInput)
	}
}
