//go:build integration

package repository

import (
	"context"
	"database/sql"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestMonitorJobLifecyclePostgres(t *testing.T) {
	ctx := context.Background()
	t.Run("competing workers and independent pools", func(t *testing.T) {
		db := monitorBudgetTestDB(t)
		budgets := NewMonitorBudgetRepository(db)
		jobs := NewMonitorJobRepository(db)
		id := monitorBudgetTestJob(t, db, 1, 3)
		_, err := jobs.Claim(ctx, service.MonitorJobCapability, "worker")
		require.ErrorIs(t, err, sql.ErrNoRows, "unreserved jobs cannot run")
		require.NoError(t, budgets.Reserve(ctx, id, monitorBudgetTestLimits(1, 10, 10)))
		_, err = jobs.Claim(ctx, service.MonitorJobAvailability, "worker")
		require.ErrorIs(t, err, sql.ErrNoRows)
		type result struct {
			lease *MonitorJobLease
			err   error
		}
		results := make(chan result, 16)
		var wg sync.WaitGroup
		for i := 0; i < 16; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				lease, err := jobs.Claim(ctx, service.MonitorJobCapability, uuid.NewString())
				results <- result{lease, err}
			}()
		}
		wg.Wait()
		close(results)
		var claimed *MonitorJobLease
		for result := range results {
			if result.err != nil {
				require.ErrorIs(t, result.err, sql.ErrNoRows)
			} else {
				require.Nil(t, claimed)
				claimed = result.lease
			}
		}
		require.NotNil(t, claimed)
		require.Equal(t, id, claimed.JobID)
		require.EqualValues(t, 1, claimed.Generation)
		expires, err := jobs.Renew(ctx, *claimed)
		require.NoError(t, err)
		require.False(t, expires.Before(claimed.ExpiresAt))
		stale := *claimed
		stale.Generation++
		_, err = jobs.Renew(ctx, stale)
		require.ErrorIs(t, err, ErrMonitorJobLease)
		require.ErrorIs(t, jobs.Finish(ctx, *claimed, service.MonitorJobCompleted), ErrMonitorJobLease, "no execution evidence")
		require.NoError(t, jobs.Finish(ctx, *claimed, service.MonitorJobFailed))
		require.ErrorIs(t, jobs.Finish(ctx, *claimed, service.MonitorJobFailed), ErrMonitorJobLease)
		monitorBudgetTestBucket(t, db, "global", 0, 0, 0)
	})
	t.Run("owner scoped queued and running cancellation", func(t *testing.T) {
		db := monitorBudgetTestDB(t)
		budgets := NewMonitorBudgetRepository(db)
		jobs := NewMonitorJobRepository(db)
		id := monitorBudgetTestJob(t, db, 1, 3)
		limits := monitorBudgetTestLimits(1, 10, 10)
		require.NoError(t, budgets.Reserve(ctx, id, limits))
		require.ErrorIs(t, jobs.CancelForOwner(ctx, id, 2), sql.ErrNoRows)
		require.ErrorIs(t, jobs.CancelForOwner(ctx, "malformed", 1), sql.ErrNoRows)
		require.NoError(t, jobs.CancelForOwner(ctx, id, 1))
		require.NoError(t, jobs.CancelForOwner(ctx, id, 1))
		monitorBudgetTestBucket(t, db, "global", 0, 0, 0)
		id = monitorBudgetTestJob(t, db, 1, 3)
		require.NoError(t, budgets.Reserve(ctx, id, limits))
		lease, err := jobs.Claim(ctx, service.MonitorJobCapability, "worker")
		require.NoError(t, err)
		require.NoError(t, budgets.Consume(ctx, id, "worker", lease.Generation, 0, limits))
		require.NoError(t, jobs.CancelForOwner(ctx, id, 1))
		_, err = jobs.Renew(ctx, *lease)
		require.ErrorIs(t, err, ErrMonitorJobLease)
		require.ErrorIs(t, budgets.Consume(ctx, id, "worker", lease.Generation, 1, limits), ErrMonitorBudgetLease)
		require.ErrorIs(t, jobs.Finish(ctx, *lease, service.MonitorJobCompleted), ErrMonitorJobLease)
		require.NoError(t, jobs.Finish(ctx, *lease, service.MonitorJobCancelled))
		monitorBudgetTestBucket(t, db, "global", 0, 0, 1)
	})
	t.Run("expired lease interrupts attempts and releases exactly once", func(t *testing.T) {
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
		_, err = db.Exec(`INSERT INTO llm_detector_attempts(id,execution_id,cell_id,sample_index,attempt_index,outbound_request_id,lease_generation,dispatch_state)
 VALUES($1,$2,'fixture',0,0,$3,1,'dispatching'),($4,$2,'fixture',1,0,$5,1,'reserved')`, uuid.NewString(), execution, uuid.NewString(), uuid.NewString(), uuid.NewString())
		require.NoError(t, err)
		_, err = db.Exec(`UPDATE monitor_jobs SET lease_expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, id)
		require.NoError(t, err)
		_, err = jobs.Renew(ctx, *lease)
		require.ErrorIs(t, err, ErrMonitorJobLease)
		type result struct {
			recovered bool
			err       error
		}
		results := make(chan result, 12)
		var wg sync.WaitGroup
		for i := 0; i < 12; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); recovered, err := jobs.RecoverNext(ctx); results <- result{recovered, err} }()
		}
		wg.Wait()
		count := 0
		for i := 0; i < 12; i++ {
			r := <-results
			require.NoError(t, r.err)
			if r.recovered {
				count++
			}
		}
		require.Equal(t, 1, count)
		var state string
		var generation int64
		require.NoError(t, db.QueryRow(`SELECT state,lease_generation FROM monitor_jobs WHERE id=$1`, id).Scan(&state, &generation))
		require.Equal(t, "interrupted", state)
		require.Greater(t, generation, lease.Generation)
		require.NoError(t, db.QueryRow(`SELECT state FROM llm_detector_executions WHERE id=$1`, execution).Scan(&state))
		require.Equal(t, "interrupted", state)
		require.NoError(t, db.QueryRow(`SELECT dispatch_state FROM llm_detector_attempts WHERE execution_id=$1 AND sample_index=0`, execution).Scan(&state))
		require.Equal(t, "uncertain", state)
		require.NoError(t, db.QueryRow(`SELECT dispatch_state FROM llm_detector_attempts WHERE execution_id=$1 AND sample_index=1`, execution).Scan(&state))
		require.Equal(t, "cancelled_before_dispatch", state)
		monitorBudgetTestBucket(t, db, "global", 0, 0, 1)
		require.ErrorIs(t, jobs.Finish(ctx, *lease, service.MonitorJobFailed), ErrMonitorJobLease)
		_, err = jobs.Claim(ctx, service.MonitorJobCapability, "new-worker")
		require.ErrorIs(t, err, sql.ErrNoRows)
	})
	t.Run("external secret affinity and queue expiry", func(t *testing.T) {
		db := monitorBudgetTestDB(t)
		budgets := NewMonitorBudgetRepository(db)
		jobs := NewMonitorJobRepository(db)
		id, plan := uuid.NewString(), uuid.NewString()
		manifest := monitorTestApprovedManifest(t, db, 1)
		limits := monitorBudgetTestLimits(1, 10, 10)
		_, err := db.Exec(`INSERT INTO llm_detector_plans(id,owner_user_id,source,normalized_target_spec,benchmark_manifest,configuration_hash,
 planned_base_requests,retry_budget_requests,maximum_outbound_requests,expires_at)
 VALUES($1,1,'external_api','{}','{}',repeat('a',64),3,0,3,clock_timestamp()+interval '5 minutes')`, plan)
		require.NoError(t, err)
		_, err = db.Exec(`INSERT INTO monitor_jobs(id,kind,source,plan_id,owner_user_id,idempotency_scope,idempotency_key_hash,payload_hash,
 config_snapshot,deadline_at,queue_deadline_at,base_requests_planned,outbound_reserved,secret_owner_instance_id)
 VALUES($1,'capability','external_api',$2,1,'external-fixture',repeat('a',64),repeat('b',64),jsonb_build_object('benchmarks',jsonb_build_array($3::jsonb)),clock_timestamp()+interval '1 hour',clock_timestamp()+interval '5 minutes',3,3,'secret-owner')`, id, plan, manifest)
		require.NoError(t, err)
		require.NoError(t, budgets.Reserve(ctx, id, limits))
		_, err = jobs.Claim(ctx, service.MonitorJobCapability, "wrong-instance")
		require.ErrorIs(t, err, sql.ErrNoRows)
		lease, err := jobs.Claim(ctx, service.MonitorJobCapability, "secret-owner")
		require.NoError(t, err)
		require.Equal(t, service.MonitorSourceExternalAPI, lease.Source)
		require.NoError(t, jobs.Finish(ctx, *lease, service.MonitorJobFailed))
		id = monitorBudgetTestJob(t, db, 1, 3)
		require.NoError(t, budgets.Reserve(ctx, id, limits))
		_, err = db.Exec(`UPDATE monitor_jobs SET created_at=clock_timestamp()-interval '10 minutes',queue_deadline_at=clock_timestamp()-interval '1 second' WHERE id=$1`, id)
		require.NoError(t, err)
		recovered, err := jobs.RecoverNext(ctx)
		require.NoError(t, err)
		require.True(t, recovered)
		monitorBudgetTestBucket(t, db, "global", 0, 0, 0)
	})
	t.Run("recovery failure rolls back job and budget", func(t *testing.T) {
		db := monitorBudgetTestDB(t)
		budgets := NewMonitorBudgetRepository(db)
		jobs := NewMonitorJobRepository(db)
		id := monitorBudgetTestJob(t, db, 1, 3)
		limits := monitorBudgetTestLimits(1, 10, 10)
		require.NoError(t, budgets.Reserve(ctx, id, limits))
		_, err := jobs.Claim(ctx, service.MonitorJobCapability, "worker")
		require.NoError(t, err)
		_, err = db.Exec(`UPDATE monitor_jobs SET lease_expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, id)
		require.NoError(t, err)
		_, err = db.Exec(`CREATE FUNCTION reject_settlement_test() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.scope='user' THEN RAISE EXCEPTION 'settlement fixture'; END IF; RETURN NEW; END $$;
 CREATE TRIGGER reject_settlement_test BEFORE UPDATE ON monitor_budget_buckets FOR EACH ROW EXECUTE FUNCTION reject_settlement_test();`)
		require.NoError(t, err)
		recovered, err := jobs.RecoverNext(ctx)
		require.Error(t, err)
		require.False(t, recovered)
		var state string
		require.NoError(t, db.QueryRow(`SELECT state FROM monitor_jobs WHERE id=$1`, id).Scan(&state))
		require.Equal(t, "running", state)
		monitorBudgetTestBucket(t, db, "global", 0, 3, 0)
		_, err = db.Exec(`DROP TRIGGER reject_settlement_test ON monitor_budget_buckets`)
		require.NoError(t, err)
		recovered, err = jobs.RecoverNext(ctx)
		require.NoError(t, err)
		require.True(t, recovered)
		monitorBudgetTestBucket(t, db, "global", 0, 0, 0)
	})
}

func monitorJobTestExecution(t *testing.T, db *sql.DB, jobID string) string {
	t.Helper()
	id := uuid.NewString()
	_, err := db.Exec(`INSERT INTO api_keys(id) VALUES(1) ON CONFLICT DO NOTHING`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO llm_detector_executions(id,job_id,source,target_index,site_api_key_id,request_model,claimed_model,
 benchmark_id,benchmark_version,benchmark_sha256,engine_commit,engine_version,scoring_version,sample_policy_version,
 request_contract_hash,tier,base_request_count,retry_budget,selection_snapshot,planned_samples)
 VALUES($1,$2,'site_api_key',0,1,'fixture-model','fixture-model','fixture','1',repeat('a',64),'fixture','fixture','1','1',repeat('b',64),'low',3,0,'{}',3)`, id, jobID)
	require.NoError(t, err)
	return id
}
