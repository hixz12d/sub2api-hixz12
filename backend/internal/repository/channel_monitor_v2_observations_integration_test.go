//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestMonitorV2ObservationAggregationPostgres(t *testing.T) {
	ctx := context.Background()
	integrationDB, client := testMonitorDatabase(t)
	user := mustCreateUser(t, client, &service.User{Email: "rollup-" + uuid.NewString() + "@example.com"})
	key := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "fixture-" + uuid.NewString(), Name: "rollup"})
	account := mustCreateAccount(t, client, &service.Account{Name: "rollup-" + uuid.NewString()})
	var group int64
	require.NoError(t, integrationDB.QueryRow(`INSERT INTO groups(name,platform,status) VALUES('observation-fixture','openai','active') RETURNING id`).Scan(&group))
	start := time.Now().UTC().Truncate(time.Hour).Add(-time.Hour)
	for _, sample := range []struct {
		origin              any
		model               string
		total, cached, rate any
		version             any
	}{
		{"real_traffic", "a", 100, 100, 20000, 1},
		{"real_traffic", "b", 900, 0, 40000, 1},
		{"real_traffic", "a", 500, nil, nil, 1},
		{nil, "a", nil, nil, nil, nil},
		{"real_traffic", "a", 1000, 1000, nil, 2},
		{"availability_probe", "a", 1000, 1000, nil, 1},
		{"capability_probe", "a", 1000, 1000, nil, 1},
		{"user_detector", "a", 1000, 1000, nil, 1},
	} {
		_, err := integrationDB.Exec(`INSERT INTO usage_logs(user_id,api_key_id,account_id,group_id,request_id,model,created_at,actual_cost,input_tokens,output_tokens,request_origin,monitor_observation_version,monitor_input_tokens_total,monitor_cache_read_tokens,monitor_output_tps_milli,monitor_visible_output_tokens,monitor_generation_ms,monitor_tps_method,monitor_first_visible_ms) VALUES($1,$2,$3,$4,$5,$6,$7,1,7777,8888,$8,$9,$10,$11,$12,80,2000,'visible_stream_v1',1000)`, user.ID, key.ID, account.ID, group, uuid.NewString(), sample.model, start.Add(time.Minute), sample.origin, sample.version, sample.total, sample.cached, sample.rate)
		require.NoError(t, err)
	}
	for _, origin := range []any{nil, "real_traffic", "availability_probe", "capability_probe", "user_detector"} {
		_, err := integrationDB.Exec(`INSERT INTO ops_error_logs(request_id,group_id,platform,model,status_code,error_phase,error_type,created_at,request_origin) VALUES($1,$2,'openai','a',503,'upstream','upstream_error',$3,$4)`, uuid.NewString(), group, start.Add(2*time.Minute), origin)
		require.NoError(t, err)
	}
	// Earlier upstream failures with a final successful usage are diagnostic
	// attempts, not an additional failed user request.
	_, err := integrationDB.Exec(`INSERT INTO ops_error_logs(request_id,group_id,api_key_id,platform,model,status_code,error_phase,error_type,error_owner,upstream_errors,created_at,request_origin)
 SELECT request_id,group_id,api_key_id,'openai',model,503,'upstream','upstream_error','provider','[{"status":503},{"status":503}]'::jsonb,$2,'real_traffic'
 FROM usage_logs WHERE group_id=$1 AND monitor_output_tps_milli=20000`, group, start.Add(time.Minute))
	require.NoError(t, err)
	repo := &channelMonitorV2Repository{db: integrationDB}
	for i := 0; i < 2; i++ {
		require.NoError(t, repo.RecomputeRange(ctx, start, start.Add(time.Hour)))
		var success, fail, candidates, measured, total, cached int64
		require.NoError(t, integrationDB.QueryRow(`SELECT SUM(success_requests),SUM(error_requests),SUM(monitor_candidate_requests),SUM(monitor_cache_measured_requests),SUM(monitor_input_tokens_total),SUM(monitor_cache_read_tokens) FROM channel_monitor_v2_metrics_rollup WHERE group_id=$1 AND bucket_seconds=300`, group).Scan(&success, &fail, &candidates, &measured, &total, &cached))
		require.EqualValues(t, 5, success)
		require.EqualValues(t, 2, fail)
		var attempts int64
		require.NoError(t, integrationDB.QueryRow(`SELECT SUM(upstream_attempt_count) FROM channel_monitor_v2_metrics_rollup WHERE group_id=$1 AND bucket_seconds=300`, group).Scan(&attempts))
		require.EqualValues(t, 2, attempts)
		require.EqualValues(t, 3, candidates)
		require.EqualValues(t, 2, measured)
		require.EqualValues(t, 1000, total)
		require.EqualValues(t, 100, cached)
		require.InDelta(t, .1, float64(cached)/float64(total), 1e-9)
		var samples int64
		require.NoError(t, integrationDB.QueryRow(`SELECT SUM(sample_count) FROM channel_monitor_v2_tps_histograms_rollup WHERE group_id=$1 AND bucket_seconds=300 AND metric_version=1`, group).Scan(&samples))
		require.EqualValues(t, 2, samples)
		var visibleSamples int64
		require.NoError(t, integrationDB.QueryRow(`SELECT SUM(sample_count) FROM channel_monitor_v2_latency_histograms_rollup WHERE group_id=$1 AND bucket_seconds=300 AND metric='visible_ttft_v1' AND user_id=0`, group).Scan(&visibleSamples))
		require.EqualValues(t, 3, visibleSamples)
		var through time.Time
		require.NoError(t, integrationDB.QueryRow(`SELECT observation_v1_data_through FROM channel_monitor_v2_watermarks WHERE id=1`).Scan(&through))
		require.False(t, through.Before(start.Add(time.Hour)))
		rows, err := integrationDB.Query(`SELECT bucket_index,SUM(sample_count) FROM channel_monitor_v2_tps_histograms_rollup WHERE group_id=$1 AND bucket_seconds=300 AND metric_version=1 GROUP BY bucket_index`, group)
		require.NoError(t, err)
		counts := make([]int64, len(service.MonitorTPSBoundsV1())+1)
		for rows.Next() {
			var bucket int
			var n int64
			require.NoError(t, rows.Scan(&bucket, &n))
			counts[bucket] += n
		}
		require.NoError(t, rows.Err())
		require.NoError(t, rows.Close())
		p50, reason, err := service.MonitorTPSPercentileV1(counts, 50)
		require.NoError(t, err)
		require.Empty(t, reason)
		require.EqualValues(t, 20995, *p50)
	}
	// A coarse bucket skipped by a short refresh must survive intact.
	coarse := start.Truncate(24 * time.Hour)
	_, err = integrationDB.Exec(`INSERT INTO channel_monitor_v2_tps_histograms_rollup(bucket_start,bucket_seconds,platform,group_id,model,metric_version,bucket_index,sample_count) VALUES($1,86400,'openai',$2,'preserved',1,0,7)`, coarse, group)
	require.NoError(t, err)
	require.NoError(t, repo.RecomputeRange(ctx, coarse, coarse.Add(time.Minute)))
	var preserved int64
	require.NoError(t, integrationDB.QueryRow(`SELECT sample_count FROM channel_monitor_v2_tps_histograms_rollup WHERE group_id=$1 AND bucket_seconds=86400 AND model='preserved'`, group).Scan(&preserved))
	require.EqualValues(t, 7, preserved)
	// A corrected/late source value replaces prior counts instead of adding to them.
	_, err = integrationDB.Exec(`UPDATE usage_logs SET monitor_cache_read_tokens=90 WHERE group_id=$1 AND model='b' AND request_origin='real_traffic'`, group)
	require.NoError(t, err)
	require.NoError(t, repo.RecomputeRange(ctx, start, start.Add(time.Hour)))
	var lateCached int64
	require.NoError(t, integrationDB.QueryRow(`SELECT SUM(monitor_cache_read_tokens) FROM channel_monitor_v2_metrics_rollup WHERE group_id=$1 AND bucket_seconds=300`, group).Scan(&lateCached))
	require.EqualValues(t, 190, lateCached)

	// Completion, not amount charged, determines new observation eligibility.
	for _, sample := range []struct {
		model       string
		cost        float64
		first       any
		requestType int
		eligible    int64
	}{
		{"free-completed", 0, 1000, 2, 1},
		{"paid-incomplete", 1, nil, 2, 0},
		{"cyber-completed", 1, 1000, 4, 0},
	} {
		_, err = integrationDB.Exec(`INSERT INTO usage_logs(user_id,api_key_id,account_id,group_id,request_id,model,created_at,actual_cost,request_type,request_origin,monitor_observation_version,monitor_input_tokens_total,monitor_cache_read_tokens,monitor_output_tps_milli,monitor_visible_output_tokens,monitor_generation_ms,monitor_tps_method,monitor_first_visible_ms,first_token_ms,duration_ms) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'real_traffic',1,100,50,40000,80,2000,'visible_stream_v1',$10,900,3000)`, user.ID, key.ID, account.ID, group, uuid.NewString(), sample.model, start.Add(time.Minute), sample.cost, sample.requestType, sample.first)
		require.NoError(t, err)
		require.NoError(t, repo.RecomputeRange(ctx, start, start.Add(time.Hour)))
		var measured, cached, tps, visible, legacy int64
		require.NoError(t, integrationDB.QueryRow(`SELECT SUM(monitor_cache_measured_requests),SUM(monitor_cache_read_tokens) FROM channel_monitor_v2_metrics_rollup WHERE group_id=$1 AND model=$2 AND bucket_seconds=300`, group, sample.model).Scan(&measured, &cached))
		require.Equal(t, sample.eligible, measured)
		require.Equal(t, sample.eligible*50, cached)
		require.NoError(t, integrationDB.QueryRow(`SELECT COALESCE(SUM(sample_count),0) FROM channel_monitor_v2_tps_histograms_rollup WHERE group_id=$1 AND model=$2 AND bucket_seconds=300`, group, sample.model).Scan(&tps))
		require.Equal(t, sample.eligible, tps)
		require.NoError(t, integrationDB.QueryRow(`SELECT COALESCE(SUM(sample_count),0) FROM channel_monitor_v2_latency_histograms_rollup WHERE group_id=$1 AND model=$2 AND bucket_seconds=300 AND metric='visible_ttft_v1' AND user_id=0`, group, sample.model).Scan(&visible))
		require.Equal(t, sample.eligible, visible)
		require.NoError(t, integrationDB.QueryRow(`SELECT COALESCE(SUM(sample_count),0) FROM channel_monitor_v2_latency_histograms_rollup WHERE group_id=$1 AND model=$2 AND bucket_seconds=300 AND metric='ttft' AND user_id=0`, group, sample.model).Scan(&legacy))
		if sample.cost == 0 {
			require.Zero(t, legacy)
		} else {
			require.EqualValues(t, 1, legacy)
		}
	}
}
