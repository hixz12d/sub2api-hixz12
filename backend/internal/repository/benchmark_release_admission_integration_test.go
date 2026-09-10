//go:build integration

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func monitorTestApprovedManifest(t *testing.T, db *sql.DB, actor int64) []byte {
	t.Helper()
	ctx := context.Background()
	registry := NewBenchmarkRegistryRepository(db)
	input := BenchmarkPackageInput{ID: uuid.NewString(), Version: "1", Mode: "gpt", Payload: []byte(`{"fixture":true}`), ContentSHA256: strings.Repeat("a", 64), EngineLock: json.RawMessage(`{"engine_commit":"fixture","engine_version":"1","scoring_version":"1","sample_policy_version":"1"}`)}
	input.SHA256 = benchmarkDigest(input.Payload)
	id, err := registry.Stage(ctx, input, actor)
	require.NoError(t, err)
	engineHash := benchmarkDigest(input.EngineLock)
	receipt, err := json.Marshal(map[string]any{"benchmark_sha256": input.SHA256, "engine_lock_sha256": engineHash, "adapter_sha256": strings.Repeat("b", 64), "calibration_valid": true})
	require.NoError(t, err)
	require.NoError(t, registry.ApproveValidated(ctx, id, input.SHA256, engineHash, receipt, actor))
	raw, err := json.Marshal(service.DetectorBenchmarkManifest{ReleaseID: id, EngineLockSHA256: engineHash, ID: input.ID, Version: input.Version, SHA256: input.SHA256, EngineCommit: "fixture", EngineVersion: "1", ScoringVersion: "1", SamplePolicyVersion: "1", RequestContractHash: strings.Repeat("c", 64)})
	require.NoError(t, err)
	return raw
}

func TestBenchmarkPlanAdmissionPostgres(t *testing.T) {
	for _, mode := range []string{"withdrawn", "unbound", "hash_mismatch"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			db := monitorBudgetTestDB(t)
			plan := monitorJobTestPlan(t, db, 1)
			var raw []byte
			require.NoError(t, db.QueryRow(`SELECT benchmark_manifest FROM llm_detector_plans WHERE id=$1`, plan).Scan(&raw))
			var manifest service.DetectorBenchmarkManifest
			require.NoError(t, json.Unmarshal(raw, &manifest))
			switch mode {
			case "withdrawn":
				require.NoError(t, NewBenchmarkRegistryRepository(db).Withdraw(ctx, manifest.ReleaseID, "test withdrawal", 1))
			case "unbound":
				manifest.ReleaseID = ""
			case "hash_mismatch":
				manifest.SHA256 = strings.Repeat("d", 64)
			}
			raw, err := json.Marshal(manifest)
			require.NoError(t, err)
			_, err = db.Exec(`UPDATE llm_detector_plans SET benchmark_manifest=$2 WHERE id=$1`, plan, raw)
			require.NoError(t, err)
			_, err = NewMonitorJobRepository(db).CreateFromPlan(ctx, CreateMonitorJobInput{PlanID: plan, OwnerID: 1, IdempotencyKey: "test", ConfigurationHash: strings.Repeat("a", 64), Limits: monitorBudgetTestLimits(1, 100, 100)})
			require.ErrorIs(t, err, ErrBenchmarkRelease)
			var count int
			require.NoError(t, db.QueryRow(`SELECT count(*) FROM monitor_jobs`).Scan(&count))
			require.Zero(t, count)
			require.NoError(t, db.QueryRow(`SELECT count(*) FROM monitor_budget_buckets`).Scan(&count))
			require.Zero(t, count)
		})
	}
}
