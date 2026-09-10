//go:build integration

package repository

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMonitorPolicyDisablePropagation(t *testing.T) {
	db := monitorBudgetTestDB(t)
	ctx := context.Background()
	var policy int64
	require.NoError(t, db.QueryRow(`INSERT INTO channel_monitor_group_policies(group_id,primary_model,evaluation_revision,created_by,updated_by,enabled,probe_config) VALUES(1,'model',repeat('a',64),1,1,true,'{"enabled":true}') RETURNING id`).Scan(&policy))
	id := uuid.NewString()
	_, err := db.Exec(`INSERT INTO monitor_jobs(id,kind,source,policy_id,evaluation_revision,idempotency_scope,idempotency_key_hash,payload_hash,config_snapshot,deadline_at,queue_deadline_at,base_requests_planned,outbound_reserved) VALUES($1,'availability','platform_group',$2,repeat('a',64),'disable-policy',repeat('a',64),repeat('b',64),'{"policy_version":1}',clock_timestamp()+interval '1 hour',clock_timestamp()+interval '5 minutes',3,3)`, id, policy)
	require.NoError(t, err)
	limits := []MonitorBudgetLimit{{Scope: "global", RequestLimit: 10}, {Scope: "probe_group", ScopeID: 1, RequestLimit: 10}}
	budget := NewMonitorBudgetRepository(db)
	require.NoError(t, budget.Reserve(ctx, id, limits))
	jobs := NewMonitorJobRepository(db)
	lease, err := jobs.Claim(ctx, service.MonitorJobAvailability, "worker")
	require.NoError(t, err)
	control := NewChannelMonitorGroupRepository(db)
	require.NoError(t, control.DisablePolicy(ctx, policy, 1, 1))
	require.ErrorIs(t, control.DisablePolicy(ctx, policy, 1, 1), service.ErrMonitorPolicyConflict)
	require.Error(t, budget.Consume(ctx, id, "worker", lease.Generation, 0, limits))
	_, err = jobs.Renew(ctx, *lease)
	require.Error(t, err)
	recovered, err := jobs.RecoverNext(ctx)
	require.NoError(t, err)
	require.True(t, recovered)
	monitorBudgetTestBucket(t, db, "global", 0, 0, 0)
}
