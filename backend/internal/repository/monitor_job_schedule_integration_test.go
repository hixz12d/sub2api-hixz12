//go:build integration

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func monitorJobTestDuePolicy(t *testing.T, db *sql.DB, probe service.GroupProbeConfig) int64 {
	t.Helper()
	raw, err := json.Marshal(probe)
	require.NoError(t, err)
	var id int64
	require.NoError(t, db.QueryRow(`INSERT INTO channel_monitor_group_policies(group_id,primary_model,extra_models,evaluation_revision,created_by,updated_by,enabled,probe_config,next_probe_at)
 VALUES(1,'fixture-model','["extra-one","extra-two"]',repeat('a',64),1,1,true,$1,clock_timestamp()-interval '12 hours') RETURNING id`, raw).Scan(&id))
	return id
}

func TestMonitorJobSchedulePostgres(t *testing.T) {
	ctx := context.Background()
	t.Run("competing schedulers create one frozen occurrence", func(t *testing.T) {
		db := monitorBudgetTestDB(t)
		jobs := NewMonitorJobRepository(db)
		cfg := service.DefaultGroupProbeConfig()
		cfg.Enabled = true
		cfg.SampleSize = 2
		cfg.IncludeExtraModels = true
		policy := monitorJobTestDuePolicy(t, db, cfg)
		ids, err := jobs.ListDueProbePolicyIDs(ctx, 100)
		require.NoError(t, err)
		require.Equal(t, []int64{policy}, ids)
		type result struct {
			id  string
			err error
		}
		results := make(chan result, 16)
		var wg sync.WaitGroup
		for i := 0; i < 16; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); id, err := jobs.EnqueueProbe(ctx, policy, 10000); results <- result{id, err} }()
		}
		wg.Wait()
		id := ""
		for i := 0; i < 16; i++ {
			r := <-results
			if r.err != nil {
				require.ErrorIs(t, r.err, sql.ErrNoRows)
			} else {
				require.Empty(t, id)
				id = r.id
			}
		}
		require.NotEmpty(t, id)
		var raw []byte
		var ceiling int64
		require.NoError(t, db.QueryRow(`SELECT config_snapshot,outbound_reserved FROM monitor_jobs WHERE id=$1`, id).Scan(&raw, &ceiling))
		var snapshot service.MonitorJobSnapshot
		require.NoError(t, json.Unmarshal(raw, &snapshot))
		require.EqualValues(t, 1, snapshot.PolicyVersion)
		require.Len(t, snapshot.Targets, 3)
		require.EqualValues(t, 6, ceiling)
		require.Equal(t, cfg, *snapshot.ProbeConfig)
		monitorBudgetTestBucket(t, db, "global", 0, 6, 0)
		monitorBudgetTestBucket(t, db, "probe_group", 0, 6, 0)
		ids, err = jobs.ListDueProbePolicyIDs(ctx, 100)
		require.NoError(t, err)
		require.Empty(t, ids)
		var active string
		var next sql.NullTime
		require.NoError(t, db.QueryRow(`SELECT active_probe_job_id,next_probe_at FROM channel_monitor_group_policies WHERE id=$1`, policy).Scan(&active, &next))
		require.Equal(t, id, active)
		require.False(t, next.Valid)
	})
	t.Run("next occurrence starts after terminal interval not historical backlog", func(t *testing.T) {
		db := monitorBudgetTestDB(t)
		jobs := NewMonitorJobRepository(db)
		cfg := service.DefaultGroupProbeConfig()
		cfg.Enabled = true
		policy := monitorJobTestDuePolicy(t, db, cfg)
		id, err := jobs.EnqueueProbe(ctx, policy, 10000)
		require.NoError(t, err)
		lease, err := jobs.Claim(ctx, service.MonitorJobAvailability, "worker")
		require.NoError(t, err)
		require.NoError(t, jobs.Finish(ctx, *lease, service.MonitorJobFailed))
		var next, finished time.Time
		var active sql.NullString
		require.NoError(t, db.QueryRow(`SELECT next_probe_at,active_probe_job_id FROM channel_monitor_group_policies WHERE id=$1`, policy).Scan(&next, &active))
		require.NoError(t, db.QueryRow(`SELECT finished_at FROM monitor_jobs WHERE id=$1`, id).Scan(&finished))
		require.False(t, active.Valid)
		require.GreaterOrEqual(t, next.Sub(finished), 45*time.Second)
		require.LessOrEqual(t, next.Sub(finished), 75*time.Second)
		_, err = jobs.EnqueueProbe(ctx, policy, 10000)
		require.ErrorIs(t, err, sql.ErrNoRows)
		_, err = db.Exec(`UPDATE channel_monitor_group_policies SET next_probe_at=clock_timestamp()-interval '1 second' WHERE id=$1`, policy)
		require.NoError(t, err)
		nextID, err := jobs.EnqueueProbe(ctx, policy, 10000)
		require.NoError(t, err)
		require.NotEqual(t, id, nextID)
	})
	t.Run("budget denial leaves no job or active pointer", func(t *testing.T) {
		db := monitorBudgetTestDB(t)
		jobs := NewMonitorJobRepository(db)
		cfg := service.DefaultGroupProbeConfig()
		cfg.Enabled = true
		cfg.SampleSize = 2
		policy := monitorJobTestDuePolicy(t, db, cfg)
		_, err := jobs.EnqueueProbe(ctx, policy, 1)
		require.ErrorIs(t, err, ErrMonitorBudgetExhausted)
		var count int
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM monitor_jobs`).Scan(&count))
		require.Zero(t, count)
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM monitor_budget_buckets`).Scan(&count))
		require.Zero(t, count)
		ids, err := jobs.ListDueProbePolicyIDs(ctx, 100)
		require.NoError(t, err)
		require.Equal(t, []int64{policy}, ids)
	})
	t.Run("probe version invalidates while capability revision is unchanged", func(t *testing.T) {
		db := monitorBudgetTestDB(t)
		jobs := NewMonitorJobRepository(db)
		budgets := NewMonitorBudgetRepository(db)
		cfg := service.DefaultGroupProbeConfig()
		cfg.Enabled = true
		policy := monitorJobTestDuePolicy(t, db, cfg)
		id, err := jobs.EnqueueProbe(ctx, policy, 10000)
		require.NoError(t, err)
		lease, err := jobs.Claim(ctx, service.MonitorJobAvailability, "worker")
		require.NoError(t, err)
		_, err = db.Exec(`UPDATE channel_monitor_group_policies SET version=version+1 WHERE id=$1`, policy)
		require.NoError(t, err)
		limits := []MonitorBudgetLimit{{Scope: "global", RequestLimit: 10000}, {Scope: "probe_group", ScopeID: 1, RequestLimit: int64(cfg.DailyRequestLimit)}}
		require.ErrorIs(t, budgets.Consume(ctx, id, "worker", lease.Generation, 0, limits), ErrMonitorBudgetLease)
		recovered, err := jobs.RecoverNext(ctx)
		require.NoError(t, err)
		require.True(t, recovered)
		monitorBudgetTestBucket(t, db, "probe_group", 0, 0, 0)
	})
	t.Run("disabled policy never reschedules and finish rollback is atomic", func(t *testing.T) {
		db := monitorBudgetTestDB(t)
		jobs := NewMonitorJobRepository(db)
		cfg := service.DefaultGroupProbeConfig()
		cfg.Enabled = true
		policy := monitorJobTestDuePolicy(t, db, cfg)
		id, err := jobs.EnqueueProbe(ctx, policy, 10000)
		require.NoError(t, err)
		lease, err := jobs.Claim(ctx, service.MonitorJobAvailability, "worker")
		require.NoError(t, err)
		_, err = db.Exec(`CREATE FUNCTION reject_probe_settle() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'probe settlement fixture'; END $$;
 CREATE TRIGGER reject_probe_settle BEFORE UPDATE ON monitor_budget_buckets FOR EACH ROW EXECUTE FUNCTION reject_probe_settle();`)
		require.NoError(t, err)
		require.Error(t, jobs.Finish(ctx, *lease, service.MonitorJobFailed))
		var active string
		var next sql.NullTime
		require.NoError(t, db.QueryRow(`SELECT active_probe_job_id,next_probe_at FROM channel_monitor_group_policies WHERE id=$1`, policy).Scan(&active, &next))
		require.Equal(t, id, active)
		require.False(t, next.Valid)
		_, err = db.Exec(`DROP TRIGGER reject_probe_settle ON monitor_budget_buckets;UPDATE channel_monitor_group_policies SET enabled=false`)
		require.NoError(t, err)
		require.NoError(t, jobs.Finish(ctx, *lease, service.MonitorJobFailed))
		var count int
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM channel_monitor_group_policies WHERE next_probe_at IS NOT NULL OR active_probe_job_id IS NOT NULL`).Scan(&count))
		require.Zero(t, count)
	})
}
