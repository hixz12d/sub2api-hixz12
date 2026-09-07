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
		for _, table := range []string{"channel_monitor_v2_metrics_rollup", "channel_monitor_v2_latency_histograms_rollup"} {
			_, _ = integrationDB.ExecContext(ctx, "DELETE FROM "+table+" WHERE group_id=ANY($1)", pq.Array([]int64{groupID, privateID}))
		}
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM groups WHERE id=ANY($1)", pq.Array([]int64{groupID, privateID}))
	})
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
	q := service.ChannelMonitorV2CardsQuery{Page: 1, PageSize: 1, ServerNow: asOf, Filter: service.ChannelMonitorV2Filter{RestrictGroups: true, AllowedGroupIDs: []int64{groupID}}}
	out, err := r.GetCards(ctx, q, *updated)
	require.NoError(t, err)
	require.Len(t, out.Items, 1)
	require.True(t, out.HasMore)
	require.Equal(t, "__other__", out.Items[0].Identity.Model)
	require.Equal(t, groupID, out.Items[0].Identity.GroupID)
	require.Equal(t, "cards-public", out.Items[0].Display.GroupLabel)
	require.InDelta(t, .9, *out.Items[0].Windows.H24.SuccessRate, 1e-9)
	require.Equal(t, int64(2000), *out.Items[0].Windows.H24.TTFTP90Ms)
	require.Equal(t, "slow", out.Items[0].Current.PerformanceState)
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
}
