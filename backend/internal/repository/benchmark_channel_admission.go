package repository

import (
	"context"
	"database/sql"
	"errors"
	"sort"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Only queued work follows the moving channel. Running work keeps its immutable
// release unless that release itself is withdrawn.
const monitorQueuedBenchmarkChannelsCurrent = `(j.kind<>'capability' OR j.state<>'queued' OR NOT EXISTS (
 SELECT 1 FROM jsonb_array_elements(` + monitorJobBenchmarkArray + `) b
 WHERE (COALESCE(b->>'channel','')<>'' OR COALESCE(b->>'channel_revision','0')<>'0')
 AND NOT EXISTS (SELECT 1 FROM monitor_benchmark_channels c
 WHERE c.name=b->>'channel' AND c.release_id::text=b->>'release_id'
 AND c.revision::text=b->>'channel_revision')))`

func lockCurrentBenchmarkChannelTx(ctx context.Context, tx *sql.Tx, b service.DetectorBenchmarkManifest) error {
	if b.Channel == "" && b.ChannelRevision == 0 {
		return nil
	}
	if !benchmarkLabel(b.Channel, 200) || b.ChannelRevision < 1 {
		return ErrBenchmarkRelease
	}
	var name string
	err := tx.QueryRowContext(ctx, `SELECT name FROM monitor_benchmark_channels WHERE name=$1 AND release_id=$2 AND revision=$3 FOR SHARE`, b.Channel, b.ReleaseID, b.ChannelRevision).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrBenchmarkRelease
	}
	return err
}

func lockMonitorJobChannelsTx(ctx context.Context, tx *sql.Tx, jobID string) error {
	var raw []byte
	if err := tx.QueryRowContext(ctx, `SELECT config_snapshot FROM monitor_jobs WHERE id=$1`, jobID).Scan(&raw); err != nil {
		return err
	}
	var snapshot service.MonitorJobSnapshot
	if len(raw) > 262144 || service.DecodeMonitorConfig(raw, &snapshot) != nil || len(snapshot.Benchmarks) == 0 || len(snapshot.Benchmarks) > 20 {
		return ErrBenchmarkRelease
	}
	sort.Slice(snapshot.Benchmarks, func(i, j int) bool { return snapshot.Benchmarks[i].Channel < snapshot.Benchmarks[j].Channel })
	for _, b := range snapshot.Benchmarks {
		if err := lockCurrentBenchmarkChannelTx(ctx, tx, b); err != nil {
			return err
		}
	}
	return nil
}
