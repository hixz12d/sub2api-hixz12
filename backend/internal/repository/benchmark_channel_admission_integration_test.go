//go:build integration

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func benchmarkChannelTestPlan(t *testing.T, db *sql.DB) (string, service.DetectorBenchmarkManifest) {
	t.Helper()
	plan := monitorJobTestPlan(t, db, 1)
	var raw []byte
	require.NoError(t, db.QueryRow(`SELECT benchmark_manifest FROM llm_detector_plans WHERE id=$1`, plan).Scan(&raw))
	var manifest service.DetectorBenchmarkManifest
	require.NoError(t, json.Unmarshal(raw, &manifest))
	manifest.Channel = "test-channel"
	revision, err := NewBenchmarkRegistryRepository(db).Activate(context.Background(), manifest.Channel, manifest.ReleaseID, 0, 1)
	require.NoError(t, err)
	manifest.ChannelRevision = revision
	raw, err = json.Marshal(manifest)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE llm_detector_plans SET benchmark_manifest=$2 WHERE id=$1`, plan, raw)
	require.NoError(t, err)
	return plan, manifest
}

func TestBenchmarkChannelPlanAndQueuePostgres(t *testing.T) {
	for _, mode := range []string{"plan", "queued", "running", "switch_back"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			db := monitorBudgetTestDB(t)
			plan, old := benchmarkChannelTestPlan(t, db)
			registry := NewBenchmarkRegistryRepository(db)
			jobs := NewMonitorJobRepository(db)
			limits := monitorBudgetTestLimits(1, 100, 100)
			input := CreateMonitorJobInput{PlanID: plan, OwnerID: 1, IdempotencyKey: "bound-plan", ConfigurationHash: strings.Repeat("a", 64), Limits: limits}
			var job string
			var lease *MonitorJobLease
			if mode != "plan" {
				var err error
				job, err = jobs.CreateFromPlan(ctx, input)
				require.NoError(t, err)
				if mode == "running" {
					lease, err = jobs.Claim(ctx, service.MonitorJobCapability, "worker")
					require.NoError(t, err)
				}
			}
			var next service.DetectorBenchmarkManifest
			require.NoError(t, json.Unmarshal(monitorTestApprovedManifest(t, db, 1), &next))
			revision, err := registry.Activate(ctx, old.Channel, next.ReleaseID, 1, 1)
			require.NoError(t, err)
			require.EqualValues(t, 2, revision)
			if mode == "switch_back" {
				_, err = registry.Activate(ctx, old.Channel, old.ReleaseID, 2, 1)
				require.NoError(t, err)
			}
			if mode == "plan" {
				_, err = jobs.CreateFromPlan(ctx, input)
				require.ErrorIs(t, err, ErrBenchmarkRelease)
				var count int
				require.NoError(t, db.QueryRow(`SELECT count(*) FROM monitor_jobs`).Scan(&count))
				require.Zero(t, count)
				return
			}
			if mode == "running" {
				_, err = jobs.Renew(ctx, *lease)
				require.NoError(t, err)
				require.NoError(t, NewMonitorBudgetRepository(db).Consume(ctx, job, "worker", lease.Generation, 0, limits))
				recovered, err := jobs.RecoverNext(ctx)
				require.NoError(t, err)
				require.False(t, recovered)
				require.NoError(t, registry.Withdraw(ctx, old.ReleaseID, "withdraw running version", 1))
				_, err = jobs.Renew(ctx, *lease)
				require.ErrorIs(t, err, ErrMonitorJobLease)
				return
			}
			_, err = jobs.Claim(ctx, service.MonitorJobCapability, "worker")
			require.ErrorIs(t, err, sql.ErrNoRows)
			recovered, err := jobs.RecoverNext(ctx)
			require.NoError(t, err)
			require.True(t, recovered)
			var state, code string
			require.NoError(t, db.QueryRow(`SELECT state,failure_code FROM monitor_jobs WHERE id=$1`, job).Scan(&state, &code))
			require.Equal(t, "skipped", state)
			require.Equal(t, "benchmark_channel_changed", code)
			monitorBudgetTestBucket(t, db, "global", 0, 0, 0)
		})
	}
}
