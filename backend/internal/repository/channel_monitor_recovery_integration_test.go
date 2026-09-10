//go:build integration

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestMonitorRetentionBoundariesPostgres(t *testing.T) {
	ctx := context.Background()
	db, _ := testMonitorDatabase(t)
	repo := &channelMonitorV2Repository{db: db}
	now := time.Now().UTC().Truncate(time.Minute)
	rules := append([]channelMonitorV2RetentionRule(nil), channelMonitorV2RetentionRules...)
	rules = append(rules, channelMonitorV2RetentionRule{table: "channel_monitor_v2_tps_histograms_1m", retention: channelMonitorV2RetentionHistogram1m})
	for _, seconds := range channelMonitorV2FixedRollupSeconds {
		for _, rule := range channelMonitorV2RetentionRules {
			if rule.table == "channel_monitor_v2_latency_histograms_rollup" && rule.bucketSeconds == seconds {
				rule.table = "channel_monitor_v2_tps_histograms_rollup"
				rules = append(rules, rule)
			}
		}
	}
	for _, rule := range rules {
		t.Run(fmt.Sprintf("%s/%d", rule.table, rule.bucketSeconds), func(t *testing.T) {
			tx, err := db.BeginTx(ctx, nil)
			require.NoError(t, err)
			defer tx.Rollback()
			columns := "bucket_start,platform,group_id,model"
			values := "$1,'openai',0,'retention-fixture'"
			if rule.bucketSeconds != 0 {
				columns += ",bucket_seconds"
				values += fmt.Sprintf(",%d", rule.bucketSeconds)
			}
			switch {
			case strings.Contains(rule.table, "user_metrics"):
				columns += ",user_id"
				values += ",1"
			case strings.Contains(rule.table, "error_metrics"):
				columns += ",error_category,taxonomy_version,error_requests"
				values += ",'other',1,1"
			case strings.Contains(rule.table, "latency_histograms"):
				columns += ",metric,upper_bound_ms,sample_count"
				values += ",'visible_ttft_v1',1000,1"
			case strings.Contains(rule.table, "tps_histograms"):
				columns += ",metric_version,bucket_index,sample_count"
				values += ",1,0,1"
			default:
				columns += ",monitor_candidate_requests,monitor_cache_measured_requests,monitor_input_tokens_total,monitor_cache_read_tokens"
				values += ",1,1,100,50"
			}
			cutoff := channelMonitorV2RetentionCutoff(now, rule.retention)
			for _, at := range []time.Time{cutoff.Add(-time.Minute), cutoff, cutoff.Add(time.Minute)} {
				_, err = tx.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", rule.table, columns, values), at)
				require.NoError(t, err)
			}
			assertRows := func(expected int) {
				t.Helper()
				var count int
				require.NoError(t, tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+rule.table).Scan(&count))
				require.Equal(t, expected, count)
			}
			// Backfill needs adjacent chunks' minute facts to rebuild coarse buckets.
			_, err = tx.ExecContext(ctx, "UPDATE channel_monitor_v2_watermarks SET backfill_cursor=$1 WHERE id=1", now.Add(-time.Hour))
			require.NoError(t, err)
			require.NoError(t, repo.pruneChannelMonitorV2Retention(ctx, tx, now))
			assertRows(3)
			_, err = tx.ExecContext(ctx, "UPDATE channel_monitor_v2_watermarks SET backfill_cursor=$1 WHERE id=1", channelMonitorV2RetentionCutoff(now, channelMonitorV2RetentionMax))
			require.NoError(t, err)
			require.NoError(t, repo.pruneChannelMonitorV2Retention(ctx, tx, now))
			assertRows(2)
			var earliest time.Time
			require.NoError(t, tx.QueryRowContext(ctx, "SELECT MIN(bucket_start) FROM "+rule.table).Scan(&earliest))
			require.True(t, cutoff.Equal(earliest))
			require.NoError(t, repo.pruneChannelMonitorV2Retention(ctx, tx, now))
			assertRows(2)
		})
	}
}

func TestMonitorRecomputeRollbackRecoveryPostgres(t *testing.T) {
	ctx := context.Background()
	db, _ := testMonitorDatabase(t)
	repo := &channelMonitorV2Repository{db: db}
	start := time.Now().UTC().Truncate(time.Hour).Add(-time.Hour)
	end := start.Add(time.Hour)
	require.NoError(t, repo.RecomputeRange(ctx, start, end))
	_, err := db.ExecContext(ctx, `INSERT INTO channel_monitor_v2_metrics_1m
 (bucket_start,platform,group_id,model,success_requests,monitor_candidate_requests,monitor_cache_measured_requests,monitor_input_tokens_total,monitor_cache_read_tokens)
 VALUES($1,'openai',0,'rollback-fixture',7,7,7,700,350)`, start)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO channel_monitor_v2_tps_histograms_1m
 (bucket_start,platform,group_id,model,metric_version,bucket_index,sample_count)
 VALUES($1,'openai',0,'rollback-fixture',1,0,7)`, start)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO channel_monitor_v2_latency_histograms_1m
 (bucket_start,platform,group_id,model,metric,upper_bound_ms,sample_count)
 VALUES($1,'openai',0,'rollback-fixture','visible_ttft_v1',1000,7)`, start)
	require.NoError(t, err)
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	require.NoError(t, repo.recomputeFixedRollups(ctx, tx, start, end))
	require.NoError(t, tx.Commit())

	// Fail at the final watermark update, after minute and rollup rewrites.
	_, err = db.ExecContext(ctx, `CREATE FUNCTION monitor_fail_watermark() RETURNS trigger LANGUAGE plpgsql AS $$
 BEGIN RAISE EXCEPTION 'injected monitor watermark failure' USING ERRCODE='P0001'; END $$;
 CREATE TRIGGER monitor_fail_watermark BEFORE UPDATE ON channel_monitor_v2_watermarks
 FOR EACH ROW EXECUTE FUNCTION monitor_fail_watermark()`)
	require.NoError(t, err)
	before := monitorRecoverySnapshot(t, db)
	err = repo.RecomputeRange(ctx, start, end.Add(time.Minute))
	var pgErr *pq.Error
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, pq.ErrorCode("P0001"), pgErr.Code)
	require.Contains(t, pgErr.Message, "injected monitor watermark failure")
	require.Equal(t, before, monitorRecoverySnapshot(t, db), "all facts, rollups and watermarks must roll back together")
	_, err = db.ExecContext(ctx, "DROP TRIGGER monitor_fail_watermark ON channel_monitor_v2_watermarks; DROP FUNCTION monitor_fail_watermark()")
	require.NoError(t, err)
	for i := 0; i < 2; i++ {
		require.NoError(t, repo.RecomputeRange(ctx, start, end.Add(time.Minute)))
		for _, table := range []string{"channel_monitor_v2_metrics_1m", "channel_monitor_v2_metrics_rollup", "channel_monitor_v2_tps_histograms_1m", "channel_monitor_v2_tps_histograms_rollup", "channel_monitor_v2_latency_histograms_1m", "channel_monitor_v2_latency_histograms_rollup"} {
			var count int
			require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE model='rollback-fixture'").Scan(&count))
			require.Zero(t, count, table+" should reflect the empty authoritative source after recovery")
		}
		var through time.Time
		require.NoError(t, db.QueryRowContext(ctx, "SELECT observation_v1_data_through FROM channel_monitor_v2_watermarks WHERE id=1").Scan(&through))
		require.True(t, end.Add(time.Minute).Equal(through))
	}
}

func monitorRecoverySnapshot(t *testing.T, db *sql.DB) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, table := range []string{"channel_monitor_v2_metrics_1m", "channel_monitor_v2_metrics_rollup", "channel_monitor_v2_tps_histograms_1m", "channel_monitor_v2_tps_histograms_rollup", "channel_monitor_v2_latency_histograms_1m", "channel_monitor_v2_latency_histograms_rollup", "channel_monitor_v2_watermarks"} {
		var snapshot string
		require.NoError(t, db.QueryRow("SELECT COALESCE(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)::text),'[]'::jsonb)::text FROM "+table+" r").Scan(&snapshot))
		out[table] = snapshot
	}
	return out
}
