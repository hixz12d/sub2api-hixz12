//go:build integration

package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestChannelMonitorV2CardsPostgres(t *testing.T) {
	ctx := context.Background()
	r := &channelMonitorV2Repository{db: integrationDB}
	asOf := time.Now().UTC().Truncate(5 * time.Minute)
	start := asOf.Add(-7 * 24 * time.Hour)
	var groupID, privateID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO groups(name,platform,status) VALUES ('cards-public','openai','active') RETURNING id`).Scan(&groupID))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO groups(name,platform,status) VALUES ('cards-private','openai','active') RETURNING id`).Scan(&privateID))
	t.Cleanup(func() {
		for _, table := range []string{"channel_monitor_v2_metrics_rollup", "channel_monitor_v2_latency_histograms_rollup", "channel_monitor_v2_tps_histograms_rollup"} {
			_, _ = integrationDB.ExecContext(ctx, "DELETE FROM "+table+" WHERE group_id=ANY($1)", pq.Array([]int64{groupID, privateID}))
		}
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM groups WHERE id=ANY($1)", pq.Array([]int64{groupID, privateID}))
	})
	var oldObservationStart, oldObservationThrough sql.NullTime
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT observation_v1_collection_start,observation_v1_data_through FROM channel_monitor_v2_watermarks WHERE id=1`).Scan(&oldObservationStart, &oldObservationThrough))
	t.Cleanup(func() {
		_, err := integrationDB.ExecContext(ctx, `UPDATE channel_monitor_v2_watermarks SET observation_v1_collection_start=$1,observation_v1_data_through=$2 WHERE id=1`, oldObservationStart, oldObservationThrough)
		require.NoError(t, err)
	})
	if _, err := integrationDB.ExecContext(ctx, `UPDATE channel_monitor_v2_watermarks SET observation_v1_collection_start=$1,observation_v1_data_through=$2 WHERE id=1`, start, asOf); err != nil {
		t.Fatal(err)
	}
	var oldUsage, oldErrors, oldThrough, oldComputed sql.NullTime
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT usage_coverage_start,error_coverage_start,data_through,last_successful_at FROM channel_monitor_v2_watermarks WHERE id=1`).Scan(&oldUsage, &oldErrors, &oldThrough, &oldComputed))
	t.Cleanup(func() {
		_, err := integrationDB.ExecContext(ctx, `UPDATE channel_monitor_v2_watermarks SET usage_coverage_start=$1,error_coverage_start=$2,data_through=$3,last_successful_at=$4 WHERE id=1`, oldUsage, oldErrors, oldThrough, oldComputed)
		require.NoError(t, err)
	})
	_, err := integrationDB.ExecContext(ctx, `UPDATE channel_monitor_v2_watermarks SET usage_coverage_start=$1,error_coverage_start=$1,data_through=$2,last_successful_at=$2 WHERE id=1`, start, asOf)
	require.NoError(t, err)
	// Restore shared singleton state after this non-parallel integration case.
	cfg, err := r.GetConfig(ctx)
	require.NoError(t, err)
	original := *cfg
	defer func() {
		current, err := r.GetConfig(ctx)
		if err == nil {
			_, _ = r.UpdateConfig(ctx, original, current.Version)
		}
	}()
	warning, critical := int64(1000), int64(5000)
	cfg.Enabled = true
	cfg.Platforms = []service.ChannelMonitorV2PlatformConfig{{Platform: "openai", Enabled: true, Models: []string{"visible"}}}
	cfg.GroupIDs = []int64{groupID, privateID}
	cfg.StatusCardSettings = service.ChannelMonitorV2StatusCardSettings{TTFTP90WarningMs: &warning, TTFTP90CriticalMs: &critical}
	updated, err := r.UpdateConfig(ctx, *cfg, cfg.Version)
	require.NoError(t, err)
	require.Equal(t, warning, *updated.StatusCardSettings.TTFTP90WarningMs)
	_, err = r.UpdateConfig(ctx, *cfg, cfg.Version)
	require.ErrorIs(t, err, service.ErrChannelMonitorV2ConfigConflict)
	for _, gid := range []int64{groupID, privateID} {
		for _, model := range []string{"visible", "private-model-a", "private-model-b"} {
			_, err = integrationDB.ExecContext(ctx, `INSERT INTO channel_monitor_v2_metrics_rollup(bucket_start,bucket_seconds,platform,group_id,model,success_requests,error_requests,ttft_sum_ms,ttft_count) VALUES ($1,300,'openai',$2,$3,90,10,180000,90)`, asOf.Add(-5*time.Minute), gid, model)
			require.NoError(t, err)
			_, err = integrationDB.ExecContext(ctx, `INSERT INTO channel_monitor_v2_latency_histograms_rollup(bucket_start,bucket_seconds,platform,group_id,model,user_id,metric,upper_bound_ms,sample_count) VALUES ($1,300,'openai',$2,$3,0,'ttft',2000,90)`, asOf.Add(-5*time.Minute), gid, model)
			require.NoError(t, err)
		}
	}
	for _, sample := range []struct {
		group  int64
		model  string
		bucket int
		count  int64
	}{
		{groupID, "private-model-a", 0, 1}, {groupID, "private-model-b", 1, 3},
		{privateID, "private-model-a", 235, 1000},
	} {
		_, err = integrationDB.ExecContext(ctx, `INSERT INTO channel_monitor_v2_tps_histograms_rollup(bucket_start,bucket_seconds,platform,group_id,model,metric_version,bucket_index,sample_count) VALUES($1,300,'openai',$2,$3,1,$4,$5)`, asOf.Add(-5*time.Minute), sample.group, sample.model, sample.bucket, sample.count)
		require.NoError(t, err)
	}
	for _, sample := range []struct {
		group         int64
		model         string
		total, cached int64
	}{
		{groupID, "private-model-a", 100, 100}, {groupID, "private-model-b", 900, 0}, {privateID, "private-model-a", 10000, 10000},
	} {
		_, err = integrationDB.ExecContext(ctx, `UPDATE channel_monitor_v2_metrics_rollup SET monitor_input_tokens_total=$1,monitor_cache_read_tokens=$2,monitor_cache_measured_requests=1 WHERE group_id=$3 AND model=$4`, sample.total, sample.cached, sample.group, sample.model)
		require.NoError(t, err)
	}
	q := service.ChannelMonitorV2CardsQuery{Page: 1, PageSize: 1, ServerNow: asOf, Filter: service.ChannelMonitorV2Filter{RestrictGroups: true, AllowedGroupIDs: []int64{groupID}}}
	out, err := r.GetCards(ctx, q, *updated)
	require.NoError(t, err)
	require.Len(t, out.Items, 1)
	require.True(t, out.HasMore)
	require.Equal(t, "__other__", out.Items[0].Identity.Model)
	require.Equal(t, groupID, out.Items[0].Identity.GroupID)
	require.Equal(t, "cards-public", out.Items[0].Display.GroupLabel)
	require.NotNil(t, out.Items[0].Windows.H24.OutputTpsP50Milli)
	require.EqualValues(t, 105, *out.Items[0].Windows.H24.OutputTpsP50Milli)
	require.EqualValues(t, 105, *out.Items[0].Windows.D7.OutputTpsP50Milli)
	require.Empty(t, out.Items[0].Windows.H24.OutputTpsReason)
	require.Equal(t, "low_sample", out.Items[0].Windows.H24.ObservationEvidence.TPS)
	require.Equal(t, "low_sample", out.Items[0].Windows.H24.ObservationEvidence.Cache)
	require.Equal(t, "no_data", out.Items[0].Windows.H24.ObservationEvidence.VisibleTTFT)
	require.InDelta(t, .1, *out.Items[0].Windows.H24.ObservedCacheReadRatio, 1e-9)
	require.InDelta(t, .1, *out.Items[0].Windows.D7.ObservedCacheReadRatio, 1e-9)
	require.Nil(t, out.Items[0].Windows.H24.CacheReadRatio)
	require.InDelta(t, .9, *out.Items[0].Windows.H24.SuccessRate, 1e-9)
	require.Equal(t, int64(2000), *out.Items[0].Windows.H24.TTFTP90Ms)
	require.Equal(t, "slow", out.Items[0].Current.PerformanceState)
	_, err = integrationDB.ExecContext(ctx, `INSERT INTO channel_monitor_v2_latency_histograms_rollup(bucket_start,bucket_seconds,platform,group_id,model,user_id,metric,upper_bound_ms,sample_count) VALUES($1,300,'openai',$2,'private-model-a',0,'visible_ttft_v1',1000,$3)`, asOf.Add(-5*time.Minute), groupID, updated.HealthThresholds.MinimumSample)
	require.NoError(t, err)
	withVisible, err := r.GetCards(ctx, q, *updated)
	require.NoError(t, err)
	require.Equal(t, "valid", withVisible.Items[0].Windows.H24.ObservationEvidence.VisibleTTFT)
	require.Equal(t, "low_sample", withVisible.Items[0].Windows.H24.ObservationEvidence.TPS)
	require.Equal(t, "low_sample", withVisible.Items[0].Windows.H24.ObservationEvidence.Cache)
	_, err = integrationDB.ExecContext(ctx, `UPDATE channel_monitor_v2_watermarks SET observation_v1_collection_start=$1 WHERE id=1`, asOf.Add(-time.Hour))
	require.NoError(t, err)
	partial, err := r.GetCards(ctx, q, *updated)
	require.NoError(t, err)
	require.Nil(t, partial.Items[0].Windows.H24.OutputTpsP50Milli)
	require.Equal(t, "partial_coverage", partial.Items[0].Windows.H24.ObservationEvidence.Cache)
	require.Equal(t, "partial_coverage", partial.Items[0].Windows.H24.ObservationEvidence.TPS)
	require.Nil(t, partial.Items[0].Windows.H24.ObservedCacheReadRatio)
	require.Equal(t, "partial_coverage", partial.Items[0].Windows.H24.ObservedCacheReason)
	require.Equal(t, "partial_coverage", partial.Items[0].Windows.H24.OutputTpsReason)
	_, err = integrationDB.ExecContext(ctx, `UPDATE channel_monitor_v2_watermarks SET observation_v1_collection_start=$1 WHERE id=1`, start)
	require.NoError(t, err)
	q.IncludeAdmin = true
	admin, err := r.GetCards(ctx, q, *updated)
	require.NoError(t, err)
	require.Equal(t, int64(200), admin.AdminResponse().Items[0].Metrics.H24.RequestCount)
	q.IncludeAdmin = false
	q.Page, q.AsOf = 2, out.AsOf
	out, err = r.GetCards(ctx, q, *updated)
	require.NoError(t, err)
	require.Len(t, out.Items, 1)
	require.False(t, out.HasMore)
	require.Equal(t, "visible", out.Items[0].Identity.Model)
	require.Nil(t, out.Items[0].Windows.H24.OutputTpsP50Milli)
	require.Equal(t, "no_data", out.Items[0].Windows.H24.OutputTpsReason)
	require.Nil(t, out.Items[0].Windows.H24.ObservedCacheReadRatio)
	require.Equal(t, "no_data", out.Items[0].Windows.H24.ObservedCacheReason)
	q.Page = 1
	q.Filter.Models = []string{"private-model-a"}
	out, err = r.GetCards(ctx, q, *updated)
	require.NoError(t, err)
	require.Empty(t, out.Items)
	q.Filter.Models = nil
	q.Filter.GroupIDs = []int64{privateID}
	out, err = r.GetCards(ctx, q, *updated)
	require.NoError(t, err)
	require.Empty(t, out.Items)
	verifyMonitorCardsHTTPPostgres(t, groupID, privateID)
}
