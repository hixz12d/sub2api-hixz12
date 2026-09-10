package repository

import (
	"context"
	"database/sql"
	"errors"
	"sort"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Query-time filtering is not a send permit; the transactional lock below orders
// consumption with withdrawal. Malformed legacy snapshots fail closed.
const monitorJobBenchmarkArray = `CASE WHEN jsonb_typeof(j.config_snapshot->'benchmarks')='array'
 THEN j.config_snapshot->'benchmarks' ELSE '[]'::jsonb END`

const monitorJobBenchmarksValid = `(j.kind<>'capability' OR (
 jsonb_array_length(` + monitorJobBenchmarkArray + `) BETWEEN 1 AND 20
 AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(` + monitorJobBenchmarkArray + `) b
 WHERE NOT EXISTS(SELECT 1 FROM monitor_benchmark_releases r
 JOIN monitor_benchmark_packages p ON p.id=r.package_id
 WHERE r.id::text=b->>'release_id' AND r.state='approved'
 AND p.benchmark_id=b->>'id' AND p.benchmark_version=b->>'version'
 AND p.sha256=b->>'sha256' AND r.engine_lock_sha256=b->>'engine_lock_sha256'
 AND b->>'request_contract_hash' ~ '^[0-9a-f]{64}$'
 AND convert_from(r.engine_lock,'UTF8')::jsonb->>'engine_commit'=b->>'engine_commit'
 AND convert_from(r.engine_lock,'UTF8')::jsonb->>'engine_version'=b->>'engine_version'
 AND convert_from(r.engine_lock,'UTF8')::jsonb->>'scoring_version'=b->>'scoring_version'
 AND convert_from(r.engine_lock,'UTF8')::jsonb->>'sample_policy_version'=b->>'sample_policy_version'))))`

// The caller holds the job row lock. Lock releases in stable order before budget
// buckets so withdrawal is ordered with each permit, including cross-day renewal.
func lockMonitorJobBenchmarksTx(ctx context.Context, tx *sql.Tx, jobID string) error {
	var raw []byte
	if err := tx.QueryRowContext(ctx, `SELECT config_snapshot FROM monitor_jobs WHERE id=$1`, jobID).Scan(&raw); err != nil {
		return err
	}
	var snapshot service.MonitorJobSnapshot
	if len(raw) > 262144 || service.DecodeMonitorConfig(raw, &snapshot) != nil || len(snapshot.Benchmarks) == 0 || len(snapshot.Benchmarks) > 20 {
		return ErrBenchmarkRelease
	}
	sort.Slice(snapshot.Benchmarks, func(i, j int) bool { return snapshot.Benchmarks[i].ReleaseID < snapshot.Benchmarks[j].ReleaseID })
	for _, benchmark := range snapshot.Benchmarks {
		if err := lockApprovedBenchmarkTx(ctx, tx, benchmark); err != nil {
			return err
		}
	}
	return nil
}

// Keep release approval stable until the caller's transaction commits. This
// establishes ordering with withdrawal, not an exactly-once network guarantee.
func lockApprovedBenchmarkTx(ctx context.Context, tx *sql.Tx, b service.DetectorBenchmarkManifest) error {
	if !validDetectorResourceID(b.ReleaseID) || !validMonitorConfigurationHash(b.EngineLockSHA256) || !validMonitorConfigurationHash(b.SHA256) || !validMonitorConfigurationHash(b.RequestContractHash) {
		return ErrBenchmarkRelease
	}
	var id string
	err := tx.QueryRowContext(ctx, `SELECT r.id FROM monitor_benchmark_releases r
 JOIN monitor_benchmark_packages p ON p.id=r.package_id
 WHERE r.id=$1 AND r.state='approved' AND p.benchmark_id=$2 AND p.benchmark_version=$3
 AND p.sha256=$4 AND r.engine_lock_sha256=$5
 AND convert_from(r.engine_lock,'UTF8')::jsonb->>'engine_commit'=$6
 AND convert_from(r.engine_lock,'UTF8')::jsonb->>'engine_version'=$7
 AND convert_from(r.engine_lock,'UTF8')::jsonb->>'scoring_version'=$8
 AND convert_from(r.engine_lock,'UTF8')::jsonb->>'sample_policy_version'=$9
 FOR SHARE OF r`, b.ReleaseID, b.ID, b.Version, b.SHA256, b.EngineLockSHA256, b.EngineCommit, b.EngineVersion, b.ScoringVersion, b.SamplePolicyVersion).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrBenchmarkRelease
	}
	return err
}
