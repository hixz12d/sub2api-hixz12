package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type modelPlazaMetricsRepository struct {
	db *sql.DB
}

// NewModelPlazaMetricsRepository 创建模型广场专用只读质量聚合仓库。
// 真实流量仅查询 Channel Monitor V2 预聚合 rollup / histogram / watermark 表（严禁全表扫 usage_logs）；
// 探针流量仅轻量查询近 24h request_origin = 'availability_probe' 行用于服务池可用性与心跳。
func NewModelPlazaMetricsRepository(db *sql.DB) service.ModelPlazaMetricsRepository {
	return &modelPlazaMetricsRepository{db: db}
}

func (r *modelPlazaMetricsRepository) LoadPlazaListSnapshot(ctx context.Context, now time.Time) (*service.PlazaListMetricsRawSnapshot, error) {
	out := &service.PlazaListMetricsRawSnapshot{
		AggregationReady: false,
		Group30d:         make(map[int64]service.PlazaRawBucketCounts),
		Group7d:          make(map[int64]service.PlazaRawBucketCounts),
		Models:           make(map[service.PlazaModelKey]*service.PlazaListModelRaw),
		ProbeModels:      make(map[service.PlazaModelKey]*service.PlazaProbeModelRaw),
		ProbeGroups:      make(map[int64]*service.PlazaGroupProbeRaw),
	}
	if r == nil || r.db == nil {
		return out, nil
	}

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin plaza list metrics tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	asOf, updatedAt, ready, err := loadPlazaWatermarkTx(ctx, tx, now)
	if err != nil {
		return nil, err
	}
	if !ready {
		nowUTC := now.UTC()
		if nowUTC.IsZero() {
			nowUTC = time.Now().UTC()
		}
		hourEnd := alignPlazaHourEnd(nowUTC)
		hour24Start := hourEnd.Add(-24 * time.Hour)
		out.Window24hStart = hour24Start
		out.Window24hEnd = hourEnd
		if err = loadPlazaListProbeMetricsTx(ctx, tx, hour24Start, plazaProbeWindowEnd(hourEnd, nowUTC), out); err != nil {
			return nil, err
		}
		return out, tx.Commit()
	}

	hourEnd := alignPlazaHourEnd(asOf)
	hour24Start := hourEnd.Add(-24 * time.Hour)
	start7d := hourEnd.Add(-7 * 24 * time.Hour)
	todayStart := asOf.Truncate(24 * time.Hour)
	start30d := todayStart.Add(-29 * 24 * time.Hour)

	out.AggregationReady = true
	out.UpdatedAt = &updatedAt
	out.Window24hStart = hour24Start
	out.Window24hEnd = hourEnd

	// 1. 全站 30d 按分组汇总（历史完整天读 86400s 桶，当天未封桶部分读 3600s 桶，互斥不重叠）
	rows, err := tx.QueryContext(ctx, `
		SELECT group_id, SUM(success_requests), SUM(error_requests)
		FROM channel_monitor_v2_metrics_rollup
		WHERE group_id > 0
		  AND (
		    (bucket_seconds = 86400 AND bucket_start >= $1 AND bucket_start < $2)
		    OR (bucket_seconds = 3600 AND bucket_start >= $2 AND bucket_start < $3)
		  )
		GROUP BY group_id`, start30d, todayStart, hourEnd)
	if err != nil {
		return nil, fmt.Errorf("query plaza 30d rollup: %w", err)
	}
	for rows.Next() {
		var gid, succ, errs int64
		if scanErr := rows.Scan(&gid, &succ, &errs); scanErr != nil {
			_ = rows.Close()
			return nil, scanErr
		}
		out.Group30d[gid] = service.PlazaRawBucketCounts{
			SuccessRequests: succ,
			ErrorRequests:   errs,
		}
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()

	// 2. 全站 7d 按分组汇总（读 3600s 桶）
	rows, err = tx.QueryContext(ctx, `
		SELECT group_id, SUM(success_requests), SUM(error_requests)
		FROM channel_monitor_v2_metrics_rollup
		WHERE group_id > 0
		  AND bucket_seconds = 3600
		  AND bucket_start >= $1 AND bucket_start < $2
		GROUP BY group_id`, start7d, hourEnd)
	if err != nil {
		return nil, fmt.Errorf("query plaza 7d rollup: %w", err)
	}
	for rows.Next() {
		var gid, succ, errs int64
		if scanErr := rows.Scan(&gid, &succ, &errs); scanErr != nil {
			_ = rows.Close()
			return nil, scanErr
		}
		out.Group7d[gid] = service.PlazaRawBucketCounts{
			SuccessRequests: succ,
			ErrorRequests:   errs,
		}
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()

	// 3. 24h 按 (bucket_start, platform, group_id, model) 汇总（同时得出 24h 总计与 12 格 2h 心跳）
	rows, err = tx.QueryContext(ctx, `
		SELECT bucket_start, platform, group_id, model,
		       SUM(success_requests), SUM(error_requests),
		       SUM(input_tokens), SUM(output_tokens),
		       SUM(cache_creation_tokens), SUM(cache_read_tokens),
		       SUM(model_observed_requests), SUM(model_mismatch_requests)
		FROM channel_monitor_v2_metrics_rollup
		WHERE group_id > 0
		  AND bucket_seconds = 3600
		  AND bucket_start >= $1 AND bucket_start < $2
		GROUP BY bucket_start, platform, group_id, model`, hour24Start, hourEnd)
	if err != nil {
		return nil, fmt.Errorf("query plaza 24h metrics rollup: %w", err)
	}
	for rows.Next() {
		var bucketStart time.Time
		var platform, model string
		var gid int64
		var c service.PlazaRawBucketCounts
		if scanErr := rows.Scan(
			&bucketStart, &platform, &gid, &model,
			&c.SuccessRequests, &c.ErrorRequests,
			&c.InputTokens, &c.OutputTokens,
			&c.CacheCreationTokens, &c.CacheReadTokens,
			&c.ModelObservedRequests, &c.ModelMismatchRequests,
		); scanErr != nil {
			_ = rows.Close()
			return nil, scanErr
		}
		key := service.NewPlazaModelKey(platform, gid, model)
		if key.Model == "" {
			continue
		}
		entry := getOrCreatePlazaListModelRaw(out.Models, key)
		entry.Total24h.Add(c)
		slotIdx := int(bucketStart.UTC().Sub(hour24Start) / (2 * time.Hour))
		if slotIdx >= 0 && slotIdx < 12 {
			entry.Slots12[slotIdx].Add(c)
		}
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()

	// 4. 24h 全平台 TTFT 直方图
	rows, err = tx.QueryContext(ctx, `
		SELECT platform, group_id, model, upper_bound_ms, SUM(sample_count)
		FROM channel_monitor_v2_latency_histograms_rollup
		WHERE group_id > 0
		  AND bucket_seconds = 3600
		  AND bucket_start >= $1 AND bucket_start < $2
		  AND user_id = 0
		  AND metric = 'ttft'
		GROUP BY platform, group_id, model, upper_bound_ms`, hour24Start, hourEnd)
	if err != nil {
		return nil, fmt.Errorf("query plaza 24h ttft histogram rollup: %w", err)
	}
	for rows.Next() {
		var platform, model string
		var gid, upperBound, count int64
		if scanErr := rows.Scan(&platform, &gid, &model, &upperBound, &count); scanErr != nil {
			_ = rows.Close()
			return nil, scanErr
		}
		if count <= 0 {
			continue
		}
		key := service.NewPlazaModelKey(platform, gid, model)
		if key.Model == "" {
			continue
		}
		entry := getOrCreatePlazaListModelRaw(out.Models, key)
		entry.TTFTHist[upperBound] += count
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()

	// 5. 24h OpenAI 可见输出速度 TPS 直方图
	tpsBuckets := len(service.MonitorTPSBoundsV1()) + 1
	rows, err = tx.QueryContext(ctx, `
		SELECT group_id, model, bucket_index, SUM(sample_count)
		FROM channel_monitor_v2_tps_histograms_rollup
		WHERE group_id > 0
		  AND bucket_seconds = 3600
		  AND bucket_start >= $1 AND bucket_start < $2
		  AND platform = 'openai'
		  AND metric_version = 1
		GROUP BY group_id, model, bucket_index`, hour24Start, hourEnd)
	if err != nil {
		return nil, fmt.Errorf("query plaza 24h tps histogram rollup: %w", err)
	}
	for rows.Next() {
		var model string
		var gid, count int64
		var bucketIdx int
		if scanErr := rows.Scan(&gid, &model, &bucketIdx, &count); scanErr != nil {
			_ = rows.Close()
			return nil, scanErr
		}
		if count <= 0 || bucketIdx < 0 || bucketIdx >= tpsBuckets {
			continue
		}
		key := service.NewPlazaModelKey(service.PlatformOpenAI, gid, model)
		if key.Model == "" {
			continue
		}
		entry := getOrCreatePlazaListModelRaw(out.Models, key)
		if entry.TPSCounts == nil {
			entry.TPSCounts = make([]int64, tpsBuckets)
		}
		entry.TPSCounts[bucketIdx] += count
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()

	// 6. 近 24h 轻量 availability_probe 探针可用性查询（独立于 V2 真实流量预聚合，严禁污染缓存/TTFT/TPS）
	if err = loadPlazaListProbeMetricsTx(ctx, tx, hour24Start, plazaProbeWindowEnd(hourEnd, now), out); err != nil {
		return nil, err
	}

	return out, tx.Commit()
}

func (r *modelPlazaMetricsRepository) LoadPlazaModelDetail(
	ctx context.Context,
	platform string,
	groupID int64,
	model string,
	now time.Time,
) (*service.PlazaModelDetailRawSnapshot, error) {
	out := &service.PlazaModelDetailRawSnapshot{
		AggregationReady:    false,
		H24:                 service.PlazaDetailWindowRaw{TTFTHist: make(map[int64]int64)},
		D7:                  service.PlazaDetailWindowRaw{TTFTHist: make(map[int64]int64)},
		D30:                 service.PlazaDetailWindowRaw{TTFTHist: make(map[int64]int64)},
		GroupSlots72ByModel: make(map[service.PlazaModelKey]*[72]service.PlazaRawBucketCounts),
	}
	if r == nil || r.db == nil || groupID <= 0 || strings.TrimSpace(model) == "" {
		return out, nil
	}

	normPlatform := strings.ToLower(strings.TrimSpace(platform))
	normModel := strings.TrimSpace(model)

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin plaza model detail tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	asOf, updatedAt, ready, err := loadPlazaWatermarkTx(ctx, tx, now)
	if err != nil {
		return nil, err
	}
	if !ready {
		nowUTC := now.UTC()
		if nowUTC.IsZero() {
			nowUTC = time.Now().UTC()
		}
		asOf5m := alignPlaza5mEnd(nowUTC)
		hb72Start := asOf5m.Add(-24 * time.Hour)
		out.HeartbeatStart = hb72Start
		if err = loadPlazaDetailProbeMetricsTx(ctx, tx, normPlatform, groupID, normModel, hb72Start, plazaProbeWindowEnd(asOf5m, nowUTC), out); err != nil {
			return nil, err
		}
		return out, tx.Commit()
	}

	hourEnd := alignPlazaHourEnd(asOf)
	hour24Start := hourEnd.Add(-24 * time.Hour)
	start7d := hourEnd.Add(-7 * 24 * time.Hour)
	todayStart := asOf.Truncate(24 * time.Hour)
	start30d := todayStart.Add(-29 * 24 * time.Hour)

	asOf5m := alignPlaza5mEnd(asOf)
	hb72Start := asOf5m.Add(-24 * time.Hour)

	out.AggregationReady = true
	out.UpdatedAt = &updatedAt
	out.HeartbeatStart = hb72Start
	out.Trend24h = make([]service.PlazaDetailHourRaw, 24)
	for i := 0; i < 24; i++ {
		out.Trend24h[i] = service.PlazaDetailHourRaw{
			BucketStart: hour24Start.Add(time.Duration(i) * time.Hour),
			TTFTHist:    make(map[int64]int64),
		}
	}

	// 1. 30d 汇总指标（历史完整天 86400s + 当天 3600s）
	err = tx.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(success_requests), 0), COALESCE(SUM(error_requests), 0),
		       COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0),
		       COALESCE(SUM(cache_creation_tokens), 0), COALESCE(SUM(cache_read_tokens), 0),
		       COALESCE(SUM(model_observed_requests), 0), COALESCE(SUM(model_mismatch_requests), 0)
		FROM channel_monitor_v2_metrics_rollup
		WHERE platform = $1 AND group_id = $2 AND model = $3
		  AND (
		    (bucket_seconds = 86400 AND bucket_start >= $4 AND bucket_start < $5)
		    OR (bucket_seconds = 3600 AND bucket_start >= $5 AND bucket_start < $6)
		  )`, normPlatform, groupID, normModel, start30d, todayStart, hourEnd).Scan(
		&out.D30.Counts.SuccessRequests, &out.D30.Counts.ErrorRequests,
		&out.D30.Counts.InputTokens, &out.D30.Counts.OutputTokens,
		&out.D30.Counts.CacheCreationTokens, &out.D30.Counts.CacheReadTokens,
		&out.D30.Counts.ModelObservedRequests, &out.D30.Counts.ModelMismatchRequests,
	)
	if err != nil {
		return nil, fmt.Errorf("query plaza detail 30d metrics: %w", err)
	}

	// 2. 7d 小时级指标（同时汇总 7d、24h 与 24h 小时级趋势）
	rows, err := tx.QueryContext(ctx, `
		SELECT bucket_start,
		       SUM(success_requests), SUM(error_requests),
		       SUM(input_tokens), SUM(output_tokens),
		       SUM(cache_creation_tokens), SUM(cache_read_tokens),
		       SUM(model_observed_requests), SUM(model_mismatch_requests)
		FROM channel_monitor_v2_metrics_rollup
		WHERE platform = $1 AND group_id = $2 AND model = $3
		  AND bucket_seconds = 3600
		  AND bucket_start >= $4 AND bucket_start < $5
		GROUP BY bucket_start`, normPlatform, groupID, normModel, start7d, hourEnd)
	if err != nil {
		return nil, fmt.Errorf("query plaza detail 7d metrics: %w", err)
	}
	for rows.Next() {
		var bucketStart time.Time
		var c service.PlazaRawBucketCounts
		if scanErr := rows.Scan(
			&bucketStart,
			&c.SuccessRequests, &c.ErrorRequests,
			&c.InputTokens, &c.OutputTokens,
			&c.CacheCreationTokens, &c.CacheReadTokens,
			&c.ModelObservedRequests, &c.ModelMismatchRequests,
		); scanErr != nil {
			_ = rows.Close()
			return nil, scanErr
		}
		out.D7.Counts.Add(c)
		at := bucketStart.UTC()
		if !at.Before(hour24Start) && at.Before(hourEnd) {
			out.H24.Counts.Add(c)
			idx := int(at.Sub(hour24Start) / time.Hour)
			if idx >= 0 && idx < 24 {
				out.Trend24h[idx].Counts.Add(c)
			}
		}
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()

	// 3. 近 24h 的 72 格心跳（每格 20 分钟，源自 300s 桶合并）；
	// 按 group_id 拉取同分组各模型 300s 桶，供服务层按已上架白名单过滤未上架模型 4xx 噪点并支撑 group_pool 心跳回退。
	rows, err = tx.QueryContext(ctx, `
		SELECT bucket_start, platform, model, SUM(success_requests), SUM(error_requests)
		FROM channel_monitor_v2_metrics_rollup
		WHERE group_id = $1
		  AND bucket_seconds = 300
		  AND bucket_start >= $2 AND bucket_start < $3
		GROUP BY bucket_start, platform, model`, groupID, hb72Start, asOf5m)
	if err != nil {
		return nil, fmt.Errorf("query plaza detail 72-slot heartbeat: %w", err)
	}
	for rows.Next() {
		var bucketStart time.Time
		var rowPlatform, rowModel string
		var succ, errs int64
		if scanErr := rows.Scan(&bucketStart, &rowPlatform, &rowModel, &succ, &errs); scanErr != nil {
			_ = rows.Close()
			return nil, scanErr
		}
		idx := int(bucketStart.UTC().Sub(hb72Start) / (20 * time.Minute))
		if idx < 0 || idx >= 72 {
			continue
		}
		key := service.NewPlazaModelKey(rowPlatform, groupID, rowModel)
		if key.Model != "" {
			groupSlots, ok := out.GroupSlots72ByModel[key]
			if !ok {
				groupSlots = new([72]service.PlazaRawBucketCounts)
				out.GroupSlots72ByModel[key] = groupSlots
			}
			groupSlots[idx].SuccessRequests += succ
			groupSlots[idx].ErrorRequests += errs
		}
		if key.Platform == normPlatform && key.Model == normModel {
			out.Slots72[idx].SuccessRequests += succ
			out.Slots72[idx].ErrorRequests += errs
		}
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()

	// 4. 7d/24h 小时级 TTFT 直方图
	rows, err = tx.QueryContext(ctx, `
		SELECT bucket_start, upper_bound_ms, SUM(sample_count)
		FROM channel_monitor_v2_latency_histograms_rollup
		WHERE platform = $1 AND group_id = $2 AND model = $3
		  AND bucket_seconds = 3600
		  AND bucket_start >= $4 AND bucket_start < $5
		  AND user_id = 0 AND metric = 'ttft'
		GROUP BY bucket_start, upper_bound_ms`, normPlatform, groupID, normModel, start7d, hourEnd)
	if err != nil {
		return nil, fmt.Errorf("query plaza detail 7d ttft histogram: %w", err)
	}
	for rows.Next() {
		var bucketStart time.Time
		var upperBound, count int64
		if scanErr := rows.Scan(&bucketStart, &upperBound, &count); scanErr != nil {
			_ = rows.Close()
			return nil, scanErr
		}
		if count <= 0 {
			continue
		}
		out.D7.TTFTHist[upperBound] += count
		at := bucketStart.UTC()
		if !at.Before(hour24Start) && at.Before(hourEnd) {
			out.H24.TTFTHist[upperBound] += count
			idx := int(at.Sub(hour24Start) / time.Hour)
			if idx >= 0 && idx < 24 {
				out.Trend24h[idx].TTFTHist[upperBound] += count
			}
		}
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()

	// 5. 30d TTFT 直方图
	rows, err = tx.QueryContext(ctx, `
		SELECT upper_bound_ms, SUM(sample_count)
		FROM channel_monitor_v2_latency_histograms_rollup
		WHERE platform = $1 AND group_id = $2 AND model = $3
		  AND user_id = 0 AND metric = 'ttft'
		  AND (
		    (bucket_seconds = 86400 AND bucket_start >= $4 AND bucket_start < $5)
		    OR (bucket_seconds = 3600 AND bucket_start >= $5 AND bucket_start < $6)
		  )
		GROUP BY upper_bound_ms`, normPlatform, groupID, normModel, start30d, todayStart, hourEnd)
	if err != nil {
		return nil, fmt.Errorf("query plaza detail 30d ttft histogram: %w", err)
	}
	for rows.Next() {
		var upperBound, count int64
		if scanErr := rows.Scan(&upperBound, &count); scanErr != nil {
			_ = rows.Close()
			return nil, scanErr
		}
		if count > 0 {
			out.D30.TTFTHist[upperBound] += count
		}
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()

	// 6. 仅 OpenAI 平台读取可见输出速度 TPS 直方图（24h / 7d / 30d）
	if normPlatform == service.PlatformOpenAI {
		tpsBuckets := len(service.MonitorTPSBoundsV1()) + 1
		out.H24.TPSCounts = make([]int64, tpsBuckets)
		out.D7.TPSCounts = make([]int64, tpsBuckets)
		out.D30.TPSCounts = make([]int64, tpsBuckets)

		rows, err = tx.QueryContext(ctx, `
			SELECT bucket_start, bucket_index, SUM(sample_count)
			FROM channel_monitor_v2_tps_histograms_rollup
			WHERE platform = 'openai' AND group_id = $1 AND model = $2
			  AND bucket_seconds = 3600
			  AND bucket_start >= $3 AND bucket_start < $4
			  AND metric_version = 1
			GROUP BY bucket_start, bucket_index`, groupID, normModel, start7d, hourEnd)
		if err != nil {
			return nil, fmt.Errorf("query plaza detail 7d tps histogram: %w", err)
		}
		for rows.Next() {
			var bucketStart time.Time
			var bucketIdx int
			var count int64
			if scanErr := rows.Scan(&bucketStart, &bucketIdx, &count); scanErr != nil {
				_ = rows.Close()
				return nil, scanErr
			}
			if count <= 0 || bucketIdx < 0 || bucketIdx >= tpsBuckets {
				continue
			}
			out.D7.TPSCounts[bucketIdx] += count
			at := bucketStart.UTC()
			if !at.Before(hour24Start) && at.Before(hourEnd) {
				out.H24.TPSCounts[bucketIdx] += count
			}
		}
		if err = rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		_ = rows.Close()

		rows, err = tx.QueryContext(ctx, `
			SELECT bucket_index, SUM(sample_count)
			FROM channel_monitor_v2_tps_histograms_rollup
			WHERE platform = 'openai' AND group_id = $1 AND model = $2
			  AND metric_version = 1
			  AND (
			    (bucket_seconds = 86400 AND bucket_start >= $3 AND bucket_start < $4)
			    OR (bucket_seconds = 3600 AND bucket_start >= $4 AND bucket_start < $5)
			  )
			GROUP BY bucket_index`, groupID, normModel, start30d, todayStart, hourEnd)
		if err != nil {
			return nil, fmt.Errorf("query plaza detail 30d tps histogram: %w", err)
		}
		for rows.Next() {
			var bucketIdx int
			var count int64
			if scanErr := rows.Scan(&bucketIdx, &count); scanErr != nil {
				_ = rows.Close()
				return nil, scanErr
			}
			if count > 0 && bucketIdx >= 0 && bucketIdx < tpsBuckets {
				out.D30.TPSCounts[bucketIdx] += count
			}
		}
		if err = rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		_ = rows.Close()
	}

	// 7. 近 24h 同分组 availability_probe 轻量查询（用于模型/分组池 72 格心跳与回退 SLA）
	if err = loadPlazaDetailProbeMetricsTx(ctx, tx, normPlatform, groupID, normModel, hb72Start, plazaProbeWindowEnd(asOf5m, now), out); err != nil {
		return nil, err
	}

	return out, tx.Commit()
}

type plazaProbeReqKey struct {
	groupID   int64
	requestID string
}

const plazaListProbeSuccessSQL = `
SELECT DISTINCT ON (COALESCE(NULLIF(ul.request_id, ''), 'usage:' || ul.id::text))
       ul.created_at,
       COALESCE(NULLIF(lower(` + usageLogEffectivePlatformExpr + `), ''), 'unknown') AS platform,
       ul.group_id,
       ` + channelMonitorV2ModelSQL + ` AS model,
       COALESCE(NULLIF(ul.request_id, ''), '') AS request_id
FROM usage_logs ul
LEFT JOIN groups g ON g.id = ul.group_id
LEFT JOIN accounts a ON a.id = ul.account_id
WHERE ul.created_at >= $1 AND ul.created_at < $2
  AND ul.request_origin = 'availability_probe'
  AND ul.group_id > 0
  AND COALESCE(ul.request_type, 0) NOT IN (4, 6)
ORDER BY COALESCE(NULLIF(ul.request_id, ''), 'usage:' || ul.id::text), ul.created_at DESC, ul.id DESC`

const plazaListProbeErrorSQL = `
SELECT DISTINCT ON (COALESCE(NULLIF(e.request_id, ''), 'error:' || e.id::text))
       e.created_at,
       lower(CASE
         WHEN g.platform = 'composite' THEN COALESCE(NULLIF(TRIM(a.platform), ''), NULLIF(NULLIF(lower(TRIM(e.platform)), ''), 'composite'), 'unknown')
         ELSE COALESCE(NULLIF(TRIM(g.platform), ''), NULLIF(TRIM(e.platform), ''), NULLIF(TRIM(a.platform), ''), 'unknown')
       END) AS platform,
       e.group_id,
       COALESCE(NULLIF(TRIM(e.requested_model), ''), NULLIF(TRIM(e.model), ''), 'unknown') AS model,
       COALESCE(NULLIF(e.request_id, ''), '') AS request_id
FROM ops_error_logs e
LEFT JOIN groups g ON g.id = e.group_id
LEFT JOIN accounts a ON a.id = e.account_id
WHERE e.created_at >= $1 AND e.created_at < $2
  AND e.request_origin = 'availability_probe'
  AND e.group_id > 0
  AND NOT e.is_count_tokens
  AND (COALESCE(e.status_code, 0) >= 400 OR e.error_type = 'cyber_policy')
ORDER BY COALESCE(NULLIF(e.request_id, ''), 'error:' || e.id::text), e.created_at DESC, e.id DESC`

const plazaDetailProbeSuccessSQL = `
SELECT DISTINCT ON (COALESCE(NULLIF(ul.request_id, ''), 'usage:' || ul.id::text))
       ul.created_at,
       COALESCE(NULLIF(lower(` + usageLogEffectivePlatformExpr + `), ''), 'unknown') AS platform,
       ` + channelMonitorV2ModelSQL + ` AS model,
       COALESCE(NULLIF(ul.request_id, ''), '') AS request_id
FROM usage_logs ul
LEFT JOIN groups g ON g.id = ul.group_id
LEFT JOIN accounts a ON a.id = ul.account_id
WHERE ul.created_at >= $1 AND ul.created_at < $2
  AND ul.group_id = $3
  AND ul.request_origin = 'availability_probe'
  AND COALESCE(ul.request_type, 0) NOT IN (4, 6)
ORDER BY COALESCE(NULLIF(ul.request_id, ''), 'usage:' || ul.id::text), ul.created_at DESC, ul.id DESC`

const plazaDetailProbeErrorSQL = `
SELECT DISTINCT ON (COALESCE(NULLIF(e.request_id, ''), 'error:' || e.id::text))
       e.created_at,
       lower(CASE
         WHEN g.platform = 'composite' THEN COALESCE(NULLIF(TRIM(a.platform), ''), NULLIF(NULLIF(lower(TRIM(e.platform)), ''), 'composite'), 'unknown')
         ELSE COALESCE(NULLIF(TRIM(g.platform), ''), NULLIF(TRIM(e.platform), ''), NULLIF(TRIM(a.platform), ''), 'unknown')
       END) AS platform,
       COALESCE(NULLIF(TRIM(e.requested_model), ''), NULLIF(TRIM(e.model), ''), 'unknown') AS model,
       COALESCE(NULLIF(e.request_id, ''), '') AS request_id
FROM ops_error_logs e
LEFT JOIN groups g ON g.id = e.group_id
LEFT JOIN accounts a ON a.id = e.account_id
WHERE e.created_at >= $1 AND e.created_at < $2
  AND e.group_id = $3
  AND e.request_origin = 'availability_probe'
  AND NOT e.is_count_tokens
  AND (COALESCE(e.status_code, 0) >= 400 OR e.error_type = 'cyber_policy')
ORDER BY COALESCE(NULLIF(e.request_id, ''), 'error:' || e.id::text), e.created_at DESC, e.id DESC`

func loadPlazaListProbeMetricsTx(
	ctx context.Context,
	tx *sql.Tx,
	windowStart time.Time,
	probeEnd time.Time,
	out *service.PlazaListMetricsRawSnapshot,
) error {
	successReqs := make(map[plazaProbeReqKey]struct{})

	rows, err := tx.QueryContext(ctx, plazaListProbeSuccessSQL, windowStart, probeEnd)
	if err != nil {
		return fmt.Errorf("query plaza 24h probe successes: %w", err)
	}
	for rows.Next() {
		var createdAt time.Time
		var platform, model, reqID string
		var gid int64
		if scanErr := rows.Scan(&createdAt, &platform, &gid, &model, &reqID); scanErr != nil {
			_ = rows.Close()
			return scanErr
		}
		if reqID != "" {
			successReqs[plazaProbeReqKey{groupID: gid, requestID: reqID}] = struct{}{}
		}
		at := createdAt.UTC()
		slotIdx := plazaSlotIndex(at, windowStart, 2*time.Hour, 12)

		gp := getOrCreatePlazaGroupProbeRaw(out.ProbeGroups, gid)
		gp.Total24h.SuccessRequests++
		if slotIdx >= 0 {
			gp.Slots12[slotIdx].SuccessRequests++
		}
		updateLatestProbeTime(&gp.LastProbeAt, at)

		key := service.NewPlazaModelKey(platform, gid, model)
		if key.Model != "" {
			mp := getOrCreatePlazaProbeModelRaw(out.ProbeModels, key)
			mp.Total24h.SuccessRequests++
			if slotIdx >= 0 {
				mp.Slots12[slotIdx].SuccessRequests++
			}
			updateLatestProbeTime(&mp.LastProbeAt, at)
		}
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()

	rows, err = tx.QueryContext(ctx, plazaListProbeErrorSQL, windowStart, probeEnd)
	if err != nil {
		return fmt.Errorf("query plaza 24h probe errors: %w", err)
	}
	for rows.Next() {
		var createdAt time.Time
		var platform, model, reqID string
		var gid int64
		if scanErr := rows.Scan(&createdAt, &platform, &gid, &model, &reqID); scanErr != nil {
			_ = rows.Close()
			return scanErr
		}
		if reqID != "" {
			if _, recovered := successReqs[plazaProbeReqKey{groupID: gid, requestID: reqID}]; recovered {
				continue
			}
		}
		at := createdAt.UTC()
		slotIdx := plazaSlotIndex(at, windowStart, 2*time.Hour, 12)

		gp := getOrCreatePlazaGroupProbeRaw(out.ProbeGroups, gid)
		gp.Total24h.ErrorRequests++
		if slotIdx >= 0 {
			gp.Slots12[slotIdx].ErrorRequests++
		}
		updateLatestProbeTime(&gp.LastProbeAt, at)

		key := service.NewPlazaModelKey(platform, gid, model)
		if key.Model != "" {
			mp := getOrCreatePlazaProbeModelRaw(out.ProbeModels, key)
			mp.Total24h.ErrorRequests++
			if slotIdx >= 0 {
				mp.Slots12[slotIdx].ErrorRequests++
			}
			updateLatestProbeTime(&mp.LastProbeAt, at)
		}
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()
	return nil
}

func loadPlazaDetailProbeMetricsTx(
	ctx context.Context,
	tx *sql.Tx,
	normPlatform string,
	groupID int64,
	normModel string,
	hb72Start time.Time,
	probeEnd time.Time,
	out *service.PlazaModelDetailRawSnapshot,
) error {
	successReqs := make(map[string]struct{})

	rows, err := tx.QueryContext(ctx, plazaDetailProbeSuccessSQL, hb72Start, probeEnd, groupID)
	if err != nil {
		return fmt.Errorf("query plaza detail 24h probe successes: %w", err)
	}
	for rows.Next() {
		var createdAt time.Time
		var rowPlatform, rowModel, reqID string
		if scanErr := rows.Scan(&createdAt, &rowPlatform, &rowModel, &reqID); scanErr != nil {
			_ = rows.Close()
			return scanErr
		}
		if reqID != "" {
			successReqs[reqID] = struct{}{}
		}
		at := createdAt.UTC()
		slotIdx := plazaSlotIndex(at, hb72Start, 20*time.Minute, 72)

		out.GroupProbe24h.SuccessRequests++
		if slotIdx >= 0 {
			out.GroupProbeSlots72[slotIdx].SuccessRequests++
		}
		updateLatestProbeTime(&out.LastProbeAt, at)

		key := service.NewPlazaModelKey(rowPlatform, groupID, rowModel)
		if key.Platform == normPlatform && strings.EqualFold(key.Model, normModel) {
			out.ModelProbe24h.SuccessRequests++
			if slotIdx >= 0 {
				out.ModelProbeSlots72[slotIdx].SuccessRequests++
			}
		}
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()

	rows, err = tx.QueryContext(ctx, plazaDetailProbeErrorSQL, hb72Start, probeEnd, groupID)
	if err != nil {
		return fmt.Errorf("query plaza detail 24h probe errors: %w", err)
	}
	for rows.Next() {
		var createdAt time.Time
		var rowPlatform, rowModel, reqID string
		if scanErr := rows.Scan(&createdAt, &rowPlatform, &rowModel, &reqID); scanErr != nil {
			_ = rows.Close()
			return scanErr
		}
		if reqID != "" {
			if _, recovered := successReqs[reqID]; recovered {
				continue
			}
		}
		at := createdAt.UTC()
		slotIdx := plazaSlotIndex(at, hb72Start, 20*time.Minute, 72)

		out.GroupProbe24h.ErrorRequests++
		if slotIdx >= 0 {
			out.GroupProbeSlots72[slotIdx].ErrorRequests++
		}
		updateLatestProbeTime(&out.LastProbeAt, at)

		key := service.NewPlazaModelKey(rowPlatform, groupID, rowModel)
		if key.Platform == normPlatform && strings.EqualFold(key.Model, normModel) {
			out.ModelProbe24h.ErrorRequests++
			if slotIdx >= 0 {
				out.ModelProbeSlots72[slotIdx].ErrorRequests++
			}
		}
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()
	return nil
}

func alignPlaza5mEnd(asOf time.Time) time.Time {
	asOf5m := asOf.UTC().Truncate(5 * time.Minute)
	if asOf.UTC().After(asOf5m) {
		asOf5m = asOf5m.Add(5 * time.Minute)
	}
	return asOf5m
}

func plazaProbeWindowEnd(windowEnd, now time.Time) time.Time {
	end := windowEnd.UTC()
	nowUTC := now.UTC()
	if nowUTC.IsZero() {
		nowUTC = time.Now().UTC()
	}
	if !nowUTC.Before(end) {
		end = nowUTC.Add(time.Second)
	}
	return end
}

func plazaSlotIndex(at, windowStart time.Time, slotDuration time.Duration, slotCount int) int {
	if at.Before(windowStart) {
		return -1
	}
	idx := int(at.Sub(windowStart) / slotDuration)
	if idx >= slotCount {
		return slotCount - 1
	}
	return idx
}

func updateLatestProbeTime(dst **time.Time, candidate time.Time) {
	if candidate.IsZero() {
		return
	}
	if *dst == nil || candidate.After(**dst) {
		ts := candidate.UTC()
		*dst = &ts
	}
}

func getOrCreatePlazaProbeModelRaw(m map[service.PlazaModelKey]*service.PlazaProbeModelRaw, key service.PlazaModelKey) *service.PlazaProbeModelRaw {
	entry, ok := m[key]
	if !ok {
		entry = &service.PlazaProbeModelRaw{}
		m[key] = entry
	}
	return entry
}

func getOrCreatePlazaGroupProbeRaw(m map[int64]*service.PlazaGroupProbeRaw, gid int64) *service.PlazaGroupProbeRaw {
	entry, ok := m[gid]
	if !ok {
		entry = &service.PlazaGroupProbeRaw{}
		m[gid] = entry
	}
	return entry
}

func loadPlazaWatermarkTx(ctx context.Context, tx *sql.Tx, now time.Time) (asOf, updatedAt time.Time, ready bool, err error) {
	var dataThrough, lastSuccessful sql.NullTime
	err = tx.QueryRowContext(ctx, `
		SELECT data_through, last_successful_at
		FROM channel_monitor_v2_watermarks
		WHERE id = 1`).Scan(&dataThrough, &lastSuccessful)
	if err == sql.ErrNoRows {
		return time.Time{}, time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, time.Time{}, false, fmt.Errorf("query plaza watermark: %w", err)
	}
	if !dataThrough.Valid {
		return time.Time{}, time.Time{}, false, nil
	}
	asOf = dataThrough.Time.UTC()
	nowUTC := now.UTC()
	if !nowUTC.IsZero() && asOf.After(nowUTC) {
		asOf = nowUTC
	}
	if lastSuccessful.Valid {
		updatedAt = lastSuccessful.Time.UTC()
	} else {
		updatedAt = asOf
	}
	return asOf, updatedAt, true, nil
}

func alignPlazaHourEnd(asOf time.Time) time.Time {
	hourEnd := asOf.UTC().Truncate(time.Hour)
	if asOf.UTC().After(hourEnd) {
		hourEnd = hourEnd.Add(time.Hour)
	}
	return hourEnd
}

func getOrCreatePlazaListModelRaw(m map[service.PlazaModelKey]*service.PlazaListModelRaw, key service.PlazaModelKey) *service.PlazaListModelRaw {
	entry, ok := m[key]
	if !ok {
		entry = &service.PlazaListModelRaw{
			TTFTHist: make(map[int64]int64),
		}
		m[key] = entry
	}
	return entry
}
