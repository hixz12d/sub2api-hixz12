//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestMonitorJobGuardsPostgres(t *testing.T) {
	ctx := context.Background()
	t.Run("platform revision invalidates dispatch renewal and recovery", func(t *testing.T) {
		db := monitorBudgetTestDB(t)
		budgets := NewMonitorBudgetRepository(db)
		jobs := NewMonitorJobRepository(db)
		var policy int64
		require.NoError(t, db.QueryRow(`INSERT INTO channel_monitor_group_policies(group_id,primary_model,evaluation_revision,created_by,updated_by,enabled,probe_config)
 VALUES(1,'fixture-model',repeat('a',64),1,1,true,'{"enabled":true}') RETURNING id`).Scan(&policy))
		id := uuid.NewString()
		_, err := db.Exec(`INSERT INTO monitor_jobs(id,kind,source,policy_id,evaluation_revision,idempotency_scope,idempotency_key_hash,payload_hash,
 config_snapshot,deadline_at,queue_deadline_at,base_requests_planned,outbound_reserved)
 VALUES($1,'availability','platform_group',$2,repeat('a',64),'platform-fixture',repeat('a',64),repeat('b',64),'{"policy_version":1}',clock_timestamp()+interval '1 hour',clock_timestamp()+interval '5 minutes',3,3)`, id, policy)
		require.NoError(t, err)
		limits := []MonitorBudgetLimit{{Scope: "global", RequestLimit: 10}, {Scope: "probe_group", ScopeID: 1, RequestLimit: 10}}
		require.NoError(t, budgets.Reserve(ctx, id, limits))
		lease, err := jobs.Claim(ctx, service.MonitorJobAvailability, "worker")
		require.NoError(t, err)
		_, err = db.Exec(`UPDATE channel_monitor_group_policies SET evaluation_revision=repeat('b',64) WHERE id=$1`, policy)
		require.NoError(t, err)
		require.ErrorIs(t, budgets.Consume(ctx, id, "worker", lease.Generation, 0, limits), ErrMonitorBudgetLease)
		_, err = jobs.Renew(ctx, *lease)
		require.ErrorIs(t, err, ErrMonitorJobLease)
		recovered, err := jobs.RecoverNext(ctx)
		require.NoError(t, err)
		require.True(t, recovered)
		monitorBudgetTestBucket(t, db, "global", 0, 0, 0)
		monitorBudgetTestBucket(t, db, "probe_group", 0, 0, 0)
		var code string
		require.NoError(t, db.QueryRow(`SELECT failure_code FROM monitor_jobs WHERE id=$1`, id).Scan(&code))
		require.Equal(t, "configuration_changed", code)
	})
	t.Run("completion requires a persisted report and keeps insufficient verdict", func(t *testing.T) {
		db := monitorBudgetTestDB(t)
		budgets := NewMonitorBudgetRepository(db)
		jobs := NewMonitorJobRepository(db)
		id := monitorBudgetTestJob(t, db, 1, 3)
		limits := monitorBudgetTestLimits(1, 10, 10)
		require.NoError(t, budgets.Reserve(ctx, id, limits))
		lease, err := jobs.Claim(ctx, service.MonitorJobCapability, "worker")
		require.NoError(t, err)
		require.NoError(t, budgets.Consume(ctx, id, "worker", lease.Generation, 0, limits))
		execution := monitorJobTestExecution(t, db, id)
		_, err = db.Exec(`UPDATE monitor_jobs SET outbound_completed=1 WHERE id=$1`, id)
		require.NoError(t, err)
		_, err = db.Exec(`UPDATE llm_detector_executions SET state='completed',finished_at=clock_timestamp(),verdict='insufficient' WHERE id=$1`, execution)
		require.NoError(t, err)
		require.ErrorIs(t, jobs.Finish(ctx, *lease, service.MonitorJobCompleted), ErrMonitorJobLease)
		_, err = db.Exec(`INSERT INTO llm_detector_reports(id,execution_id,evidence) VALUES($1,$2,'{"verdict":"insufficient"}')`, uuid.NewString(), execution)
		require.NoError(t, err)
		require.NoError(t, jobs.Finish(ctx, *lease, service.MonitorJobCompleted))
		var verdict string
		require.NoError(t, db.QueryRow(`SELECT verdict FROM llm_detector_executions WHERE id=$1`, execution).Scan(&verdict))
		require.Equal(t, "insufficient", verdict)
		monitorBudgetTestBucket(t, db, "global", 0, 0, 1)
	})
	t.Run("renew checks database time after acquiring job lock", func(t *testing.T) {
		db := monitorBudgetTestDB(t)
		budgets := NewMonitorBudgetRepository(db)
		jobs := NewMonitorJobRepository(db)
		id := monitorBudgetTestJob(t, db, 1, 3)
		limits := monitorBudgetTestLimits(1, 10, 10)
		require.NoError(t, budgets.Reserve(ctx, id, limits))
		lease, err := jobs.Claim(ctx, service.MonitorJobCapability, "worker")
		require.NoError(t, err)
		_, err = db.Exec(`UPDATE monitor_jobs SET lease_expires_at=clock_timestamp()+interval '150 milliseconds' WHERE id=$1`, id)
		require.NoError(t, err)
		blocker, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer blocker.Rollback()
		_, err = blocker.Exec(`SELECT 1 FROM monitor_jobs WHERE id=$1 FOR UPDATE`, id)
		require.NoError(t, err)
		done := make(chan error, 1)
		go func() { _, err := jobs.Renew(ctx, *lease); done <- err }()
		time.Sleep(250 * time.Millisecond)
		require.NoError(t, blocker.Rollback())
		require.ErrorIs(t, <-done, ErrMonitorJobLease)
	})
}
