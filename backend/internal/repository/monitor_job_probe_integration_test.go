//go:build integration

package repository

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func monitorProbeTestFixture(t *testing.T) (*sql.DB, *MonitorJobRepository, *MonitorJobLease, service.MonitorProbeDispatch) {
	t.Helper()
	db := monitorBudgetTestDB(t)
	jobs := NewMonitorJobRepository(db)
	_, err := db.Exec(`ALTER TABLE accounts ADD COLUMN status TEXT NOT NULL DEFAULT 'active',ADD COLUMN schedulable BOOLEAN NOT NULL DEFAULT true,ADD COLUMN deleted_at TIMESTAMPTZ;
 ALTER TABLE groups ADD COLUMN status TEXT NOT NULL DEFAULT 'active',ADD COLUMN deleted_at TIMESTAMPTZ;
 INSERT INTO accounts(id) VALUES(1),(2);
 CREATE TABLE account_groups(account_id BIGINT REFERENCES accounts(id),group_id BIGINT REFERENCES groups(id),PRIMARY KEY(account_id,group_id));
 INSERT INTO account_groups VALUES(1,1);`)
	require.NoError(t, err)
	cfg := service.DefaultGroupProbeConfig()
	cfg.Enabled = true
	policy := monitorJobTestDuePolicy(t, db, cfg)
	_, err = jobs.EnqueueProbe(context.Background(), policy, 10000)
	require.NoError(t, err)
	lease, err := jobs.Claim(context.Background(), service.MonitorJobAvailability, "worker")
	require.NoError(t, err)
	spec := service.MonitorProbeDispatch{Lease: *lease, AccountID: 1, RequestModel: "fixture-model", RequestSHA256: strings.Repeat("a", 64)}
	return db, jobs, lease, spec
}

func TestMonitorJobProbeDispatchPostgres(t *testing.T) {
	ctx := context.Background()
	t.Run("one sample authorizes only one physical send and one result", func(t *testing.T) {
		db, jobs, lease, spec := monitorProbeTestFixture(t)
		type result struct {
			id  string
			err error
		}
		results := make(chan result, 12)
		var wg sync.WaitGroup
		for i := 0; i < 12; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				id, err := jobs.BeginProbeDispatch(ctx, spec, 10000)
				results <- result{id, err}
			}()
		}
		wg.Wait()
		id := ""
		for i := 0; i < 12; i++ {
			r := <-results
			if r.err != nil {
				require.ErrorIs(t, r.err, service.ErrMonitorOutboundDenied)
			} else {
				require.Empty(t, id)
				id = r.id
			}
		}
		require.NotEmpty(t, id)
		monitorBudgetTestBucket(t, db, "probe_group", 0, 0, 1)
		outcome := service.MonitorProbeOutcome{DispatchID: id, TransportState: "passed", ChallengeState: "passed"}
		require.NoError(t, jobs.CompleteProbeDispatch(ctx, *lease, outcome))
		require.NoError(t, jobs.CompleteProbeDispatch(ctx, *lease, outcome))
		changed := outcome
		changed.ChallengeState = "failed"
		require.ErrorIs(t, jobs.CompleteProbeDispatch(ctx, *lease, changed), ErrMonitorBudgetConflict)
		var completed int64
		require.NoError(t, db.QueryRow(`SELECT outbound_completed FROM monitor_jobs WHERE id=$1`, lease.JobID).Scan(&completed))
		require.EqualValues(t, 1, completed)
		require.NoError(t, jobs.Finish(ctx, *lease, service.MonitorJobCompleted))
		var source string
		var ttft sql.NullInt64
		require.NoError(t, db.QueryRow(`SELECT source,ttft_ms FROM channel_monitor_probe_results WHERE id=$1`, id).Scan(&source, &ttft))
		require.Equal(t, "availability_probe", source)
		require.False(t, ttft.Valid)
	})
	t.Run("membership model and lease rejection never consume", func(t *testing.T) {
		db, jobs, lease, spec := monitorProbeTestFixture(t)
		wrong := spec
		wrong.AccountID = 2
		_, err := jobs.BeginProbeDispatch(ctx, wrong, 10000)
		require.ErrorIs(t, err, service.ErrMonitorOutboundDenied)
		wrong = spec
		wrong.RequestModel = "different-model"
		_, err = jobs.BeginProbeDispatch(ctx, wrong, 10000)
		require.ErrorIs(t, err, ErrMonitorBudgetInput)
		wrong = spec
		wrong.Lease.Generation++
		_, err = jobs.BeginProbeDispatch(ctx, wrong, 10000)
		require.ErrorIs(t, err, sql.ErrNoRows)
		_, err = db.Exec(`UPDATE accounts SET schedulable=false WHERE id=1`)
		require.NoError(t, err)
		_, err = jobs.BeginProbeDispatch(ctx, spec, 10000)
		require.ErrorIs(t, err, service.ErrMonitorOutboundDenied)
		monitorBudgetTestBucket(t, db, "global", 0, 1, 0)
		var count int
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM monitor_probe_dispatches WHERE job_id=$1`, lease.JobID).Scan(&count))
		require.Zero(t, count)
	})
	t.Run("crashed dispatch becomes uncertain without refund", func(t *testing.T) {
		db, jobs, lease, spec := monitorProbeTestFixture(t)
		id, err := jobs.BeginProbeDispatch(ctx, spec, 10000)
		require.NoError(t, err)
		_, err = db.Exec(`UPDATE monitor_jobs SET lease_expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, lease.JobID)
		require.NoError(t, err)
		recovered, err := jobs.RecoverNext(ctx)
		require.NoError(t, err)
		require.True(t, recovered)
		var state string
		require.NoError(t, db.QueryRow(`SELECT state FROM monitor_probe_dispatches WHERE id=$1`, id).Scan(&state))
		require.Equal(t, "uncertain", state)
		monitorBudgetTestBucket(t, db, "global", 0, 0, 1)
		_, err = jobs.BeginProbeDispatch(ctx, spec, 10000)
		require.Error(t, err)
	})
	t.Run("result write failure rolls back completion", func(t *testing.T) {
		db, jobs, lease, spec := monitorProbeTestFixture(t)
		id, err := jobs.BeginProbeDispatch(ctx, spec, 10000)
		require.NoError(t, err)
		_, err = db.Exec(`CREATE FUNCTION reject_probe_result() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'probe result fixture'; END $$;
 CREATE TRIGGER reject_probe_result BEFORE INSERT ON channel_monitor_probe_results FOR EACH ROW EXECUTE FUNCTION reject_probe_result();`)
		require.NoError(t, err)
		outcome := service.MonitorProbeOutcome{DispatchID: id, TransportState: "failed", ChallengeState: "not_evaluated", ErrorCategory: "http_error"}
		require.Error(t, jobs.CompleteProbeDispatch(ctx, *lease, outcome))
		var state string
		require.NoError(t, db.QueryRow(`SELECT state FROM monitor_probe_dispatches WHERE id=$1`, id).Scan(&state))
		require.Equal(t, "dispatching", state)
		_, err = db.Exec(`DROP TRIGGER reject_probe_result ON channel_monitor_probe_results`)
		require.NoError(t, err)
		require.NoError(t, jobs.CompleteProbeDispatch(ctx, *lease, outcome))
	})
	t.Run("probe and capability group ceilings coexist across repeat migration", func(t *testing.T) {
		db, _, _, _ := monitorProbeTestFixture(t)
		_, err := db.Exec(`INSERT INTO monitor_budget_buckets(scope,scope_id,utc_day,request_limit,reserved)
 VALUES('group',1,(clock_timestamp() AT TIME ZONE 'UTC')::date,1000,500)`)
		require.NoError(t, err)
		raw, err := migrations.FS.ReadFile("244_monitor_probe_dispatches.sql")
		require.NoError(t, err)
		for i := 0; i < 2; i++ {
			_, err = db.Exec(string(raw))
			require.NoError(t, err)
		}
		monitorBudgetTestBucket(t, db, "probe_group", 0, 1, 0)
		monitorBudgetTestBucket(t, db, "group", 0, 500, 0)
	})
}
