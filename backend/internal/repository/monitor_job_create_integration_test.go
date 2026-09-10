//go:build integration

package repository

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func monitorJobTestPlan(t *testing.T, db *sql.DB, owner int64) string {
	t.Helper()
	id := uuid.NewString()
	manifest := monitorTestApprovedManifest(t, db, owner)
	_, err := db.Exec(`INSERT INTO llm_detector_plans(id,owner_user_id,source,normalized_target_spec,benchmark_manifest,configuration_hash,
 planned_base_requests,retry_budget_requests,maximum_outbound_requests,expires_at)
 VALUES($1,$2,'site_api_key','{"source":"site_api_key","site_api_key_id":1}',$3,repeat('a',64),3,1,4,clock_timestamp()+interval '5 minutes')`, id, owner, manifest)
	require.NoError(t, err)
	return id
}

func TestMonitorJobCreatePostgres(t *testing.T) {
	ctx := context.Background()
	t.Run("concurrent identical requests consume plan once", func(t *testing.T) {
		db := monitorBudgetTestDB(t)
		jobs := NewMonitorJobRepository(db)
		input := CreateMonitorJobInput{PlanID: monitorJobTestPlan(t, db, 1), OwnerID: 1, IdempotencyKey: "same-key", ConfigurationHash: strings.Repeat("a", 64), Limits: monitorBudgetTestLimits(1, 100, 100)}
		type result struct {
			id  string
			err error
		}
		results := make(chan result, 16)
		var wg sync.WaitGroup
		for i := 0; i < 16; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); id, err := jobs.CreateFromPlan(ctx, input); results <- result{id, err} }()
		}
		wg.Wait()
		id := ""
		for i := 0; i < 16; i++ {
			r := <-results
			require.NoError(t, r.err)
			if id == "" {
				id = r.id
			}
			require.Equal(t, id, r.id)
		}
		monitorBudgetTestBucket(t, db, "global", 0, 4, 0)
		var count int
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM monitor_jobs`).Scan(&count))
		require.Equal(t, 1, count)
		input.IdempotencyKey = "different-key"
		_, err := jobs.CreateFromPlan(ctx, input)
		require.ErrorIs(t, err, ErrMonitorBudgetConflict)
		input.IdempotencyKey = "same-key"
		input.ConfigurationHash = strings.Repeat("b", 64)
		_, err = jobs.CreateFromPlan(ctx, input)
		require.ErrorIs(t, err, ErrMonitorBudgetConflict)
		input.ConfigurationHash = strings.Repeat("a", 64)
		input.PlanID = monitorJobTestPlan(t, db, 1)
		_, err = jobs.CreateFromPlan(ctx, input)
		require.ErrorIs(t, err, ErrMonitorBudgetConflict)
	})
	t.Run("budget rejection leaves plan unconsumed", func(t *testing.T) {
		db := monitorBudgetTestDB(t)
		jobs := NewMonitorJobRepository(db)
		input := CreateMonitorJobInput{PlanID: monitorJobTestPlan(t, db, 1), OwnerID: 1, IdempotencyKey: "key", ConfigurationHash: strings.Repeat("a", 64), Limits: monitorBudgetTestLimits(1, 100, 3)}
		_, err := jobs.CreateFromPlan(ctx, input)
		require.ErrorIs(t, err, ErrMonitorBudgetExhausted)
		var count int
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM monitor_jobs`).Scan(&count))
		require.Zero(t, count)
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM monitor_budget_buckets`).Scan(&count))
		require.Zero(t, count)
		input.Limits = monitorBudgetTestLimits(1, 100, 100)
		id, err := jobs.CreateFromPlan(ctx, input)
		require.NoError(t, err)
		require.NotEmpty(t, id)
	})
	t.Run("expired and wrong owner plans never create jobs", func(t *testing.T) {
		db := monitorBudgetTestDB(t)
		jobs := NewMonitorJobRepository(db)
		input := CreateMonitorJobInput{PlanID: monitorJobTestPlan(t, db, 1), OwnerID: 2, IdempotencyKey: "key", ConfigurationHash: strings.Repeat("a", 64), Limits: monitorBudgetTestLimits(2, 100, 100)}
		_, err := jobs.CreateFromPlan(ctx, input)
		require.ErrorIs(t, err, sql.ErrNoRows)
		input.OwnerID = 1
		input.Limits = monitorBudgetTestLimits(1, 100, 100)
		_, err = db.Exec(`UPDATE llm_detector_plans SET created_at=clock_timestamp()-interval '10 minutes',expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, input.PlanID)
		require.NoError(t, err)
		_, err = jobs.CreateFromPlan(ctx, input)
		require.ErrorIs(t, err, ErrMonitorBudgetLease)
		var count int
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM monitor_jobs`).Scan(&count))
		require.Zero(t, count)
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM monitor_budget_buckets`).Scan(&count))
		require.Zero(t, count)
	})
	t.Run("different plans compete for the same idempotency key", func(t *testing.T) {
		db := monitorBudgetTestDB(t)
		jobs := NewMonitorJobRepository(db)
		input := CreateMonitorJobInput{OwnerID: 1, IdempotencyKey: "key", ConfigurationHash: strings.Repeat("a", 64), Limits: monitorBudgetTestLimits(1, 100, 100)}
		plans := []string{monitorJobTestPlan(t, db, 1), monitorJobTestPlan(t, db, 1)}
		results := make(chan error, 2)
		var wg sync.WaitGroup
		for _, plan := range plans {
			wg.Add(1)
			go func(id string) {
				defer wg.Done()
				copy := input
				copy.PlanID = id
				_, err := jobs.CreateFromPlan(ctx, copy)
				results <- err
			}(plan)
		}
		wg.Wait()
		successes := 0
		for i := 0; i < 2; i++ {
			err := <-results
			if err == nil {
				successes++
			} else {
				require.ErrorIs(t, err, ErrMonitorBudgetConflict)
			}
		}
		require.Equal(t, 1, successes)
		monitorBudgetTestBucket(t, db, "global", 0, 4, 0)
	})
}
