//go:build integration

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestBenchmarkDispatchAdmissionPostgres(t *testing.T) {
	for _, mode := range []string{"withdrawn", "unbound", "hash_mismatch", "engine_mismatch", "second_withdrawn", "empty", "null", "invalid_shape"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			db := monitorBudgetTestDB(t)
			job := monitorBudgetTestJob(t, db, 1, 3)
			budget := NewMonitorBudgetRepository(db)
			limits := monitorBudgetTestLimits(1, 10, 10)
			require.NoError(t, budget.Reserve(ctx, job, limits))
			monitorBudgetTestRunning(t, db, job)
			require.NoError(t, budget.Consume(ctx, job, "worker", 1, 0, limits))
			var raw []byte
			require.NoError(t, db.QueryRow(`SELECT config_snapshot FROM monitor_jobs WHERE id=$1`, job).Scan(&raw))
			var snapshot service.MonitorJobSnapshot
			require.NoError(t, json.Unmarshal(raw, &snapshot))
			switch mode {
			case "withdrawn":
				require.NoError(t, NewBenchmarkRegistryRepository(db).Withdraw(ctx, snapshot.Benchmarks[0].ReleaseID, "test withdrawal", 1))
			case "unbound":
				snapshot.Benchmarks[0].ReleaseID = ""
			case "hash_mismatch":
				snapshot.Benchmarks[0].SHA256 = strings.Repeat("d", 64)
			case "engine_mismatch":
				snapshot.Benchmarks[0].EngineVersion = "unknown"
			case "second_withdrawn":
				var second service.DetectorBenchmarkManifest
				require.NoError(t, json.Unmarshal(monitorTestApprovedManifest(t, db, 1), &second))
				snapshot.Benchmarks = append(snapshot.Benchmarks, second)
				require.NoError(t, NewBenchmarkRegistryRepository(db).Withdraw(ctx, second.ReleaseID, "test withdrawal", 1))
			case "empty":
				snapshot.Benchmarks = nil
			}
			raw, err := json.Marshal(snapshot)
			require.NoError(t, err)
			if mode == "null" {
				raw = []byte(`{"benchmarks":null}`)
			} else if mode == "invalid_shape" {
				raw = []byte(`{"benchmarks":{}}`)
			}
			_, err = db.Exec(`UPDATE monitor_jobs SET config_snapshot=$2 WHERE id=$1`, job, raw)
			require.NoError(t, err)
			require.ErrorIs(t, budget.Consume(ctx, job, "worker", 1, 1, limits), ErrBenchmarkRelease)
			monitorBudgetTestBucket(t, db, "global", 0, 2, 1)
			monitorBudgetTestBucket(t, db, "user", 0, 2, 1)
			var dispatched int
			require.NoError(t, db.QueryRow(`SELECT outbound_dispatched FROM monitor_jobs WHERE id=$1`, job).Scan(&dispatched))
			require.Equal(t, 1, dispatched)
			recovered, err := NewMonitorJobRepository(db).RecoverNext(ctx)
			require.NoError(t, err)
			require.True(t, recovered)
			monitorBudgetTestBucket(t, db, "global", 0, 0, 1)
		})
	}
}

func TestBenchmarkDispatchWithdrawalOrderingPostgres(t *testing.T) {
	ctx := context.Background()
	db := monitorBudgetTestDB(t)
	job := monitorBudgetTestJob(t, db, 1, 3)
	budget := NewMonitorBudgetRepository(db)
	limits, err := normalizeMonitorBudgetLimits(monitorBudgetTestLimits(1, 10, 10))
	require.NoError(t, err)
	require.NoError(t, budget.Reserve(ctx, job, limits))
	monitorBudgetTestRunning(t, db, job)
	var release string
	require.NoError(t, db.QueryRow(`SELECT config_snapshot->'benchmarks'->0->>'release_id' FROM monitor_jobs WHERE id=$1`, job).Scan(&release))

	permit, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer permit.Rollback()
	require.NoError(t, consumeMonitorBudgetTx(ctx, permit, job, "worker", 1, 0, limits))
	withdrawal, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer withdrawal.Rollback()
	_, err = withdrawal.Exec(`SET LOCAL lock_timeout='100ms'`)
	require.NoError(t, err)
	_, err = withdrawal.Exec(`UPDATE monitor_benchmark_releases SET state='withdrawn' WHERE id=$1`, release)
	var lockError *pq.Error
	require.ErrorAs(t, err, &lockError)
	require.Equal(t, pq.ErrorCode("55P03"), lockError.Code)
	require.NoError(t, withdrawal.Rollback())
	require.NoError(t, permit.Commit())

	require.NoError(t, NewBenchmarkRegistryRepository(db).Withdraw(ctx, release, "test withdrawal", 1))
	require.ErrorIs(t, budget.Consume(ctx, job, "worker", 1, 1, limits), ErrBenchmarkRelease)
	monitorBudgetTestBucket(t, db, "global", 0, 2, 1)
}

func TestBenchmarkDispatchWithdrawalCrossDayPostgres(t *testing.T) {
	ctx := context.Background()
	db := monitorBudgetTestDB(t)
	job := monitorBudgetTestJob(t, db, 1, 3)
	budget := NewMonitorBudgetRepository(db)
	limits := monitorBudgetTestLimits(1, 10, 10)
	require.NoError(t, budget.Reserve(ctx, job, limits))
	monitorBudgetTestRunning(t, db, job)
	require.NoError(t, budget.Consume(ctx, job, "worker", 1, 0, limits))
	monitorBudgetTestYesterday(t, db)
	var release string
	require.NoError(t, db.QueryRow(`SELECT config_snapshot->'benchmarks'->0->>'release_id' FROM monitor_jobs WHERE id=$1`, job).Scan(&release))
	require.NoError(t, NewBenchmarkRegistryRepository(db).Withdraw(ctx, release, "test withdrawal", 1))
	require.ErrorIs(t, budget.Consume(ctx, job, "worker", 1, 1, limits), ErrBenchmarkRelease)
	monitorBudgetTestBucket(t, db, "global", -1, 2, 1)
	var count int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM monitor_budget_buckets WHERE utc_day=(clock_timestamp() AT TIME ZONE 'UTC')::date`).Scan(&count))
	require.Zero(t, count)
}

func TestBenchmarkWithdrawalRecoveryPostgres(t *testing.T) {
	for _, running := range []bool{false, true} {
		name := "queued"
		if running {
			name = "running"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			db := monitorBudgetTestDB(t)
			job := monitorBudgetTestJob(t, db, 1, 3)
			jobs := NewMonitorJobRepository(db)
			budget := NewMonitorBudgetRepository(db)
			limits := monitorBudgetTestLimits(1, 10, 10)
			require.NoError(t, budget.Reserve(ctx, job, limits))
			var lease *MonitorJobLease
			var execution string
			if running {
				var err error
				lease, err = jobs.Claim(ctx, service.MonitorJobCapability, "worker")
				require.NoError(t, err)
				require.NoError(t, budget.Consume(ctx, job, "worker", lease.Generation, 0, limits))
				execution = monitorJobTestExecution(t, db, job)
			}
			var release string
			require.NoError(t, db.QueryRow(`SELECT config_snapshot->'benchmarks'->0->>'release_id' FROM monitor_jobs WHERE id=$1`, job).Scan(&release))
			require.NoError(t, NewBenchmarkRegistryRepository(db).Withdraw(ctx, release, "test withdrawal", 1))
			_, err := jobs.Claim(ctx, service.MonitorJobCapability, "new-worker")
			require.ErrorIs(t, err, sql.ErrNoRows)
			if running {
				_, err = jobs.Renew(ctx, *lease)
				require.ErrorIs(t, err, ErrMonitorJobLease)
			}
			recovered, err := jobs.RecoverNext(ctx)
			require.NoError(t, err)
			require.True(t, recovered)
			var state, code string
			require.NoError(t, db.QueryRow(`SELECT state,failure_code FROM monitor_jobs WHERE id=$1`, job).Scan(&state, &code))
			require.Equal(t, "benchmark_unavailable", code)
			consumed := int64(0)
			if running {
				consumed = 1
				require.Equal(t, "interrupted", state)
				require.NoError(t, db.QueryRow(`SELECT state FROM llm_detector_executions WHERE id=$1`, execution).Scan(&state))
				require.Equal(t, "interrupted", state)
				require.ErrorIs(t, jobs.Finish(ctx, *lease, service.MonitorJobCompleted), ErrMonitorJobLease)
				require.ErrorIs(t, budget.Consume(ctx, job, "worker", lease.Generation, 1, limits), ErrMonitorBudgetLease)
			} else {
				require.Equal(t, "skipped", state)
			}
			monitorBudgetTestBucket(t, db, "global", 0, 0, consumed)
			monitorBudgetTestBucket(t, db, "user", 0, 0, consumed)
			recovered, err = jobs.RecoverNext(ctx)
			require.NoError(t, err)
			require.False(t, recovered)
		})
	}
}
