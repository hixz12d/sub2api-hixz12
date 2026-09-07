package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

var _ service.ChannelMonitorV2CardsRepository = (*channelMonitorV2Repository)(nil)

func (r *channelMonitorV2Repository) GetCards(ctx context.Context, q service.ChannelMonitorV2CardsQuery, cfg service.ChannelMonitorV2Config) (*service.ChannelMonitorV2Cards, error) {
	out := &service.ChannelMonitorV2Cards{Items: []service.ChannelMonitorV2StatusCard{}, Page: q.Page, PageSize: q.PageSize, ServerNow: q.ServerNow}
	if channelMonitorV2RestrictedGroupScopeEmpty(q.Filter, cfg) {
		return out, nil
	}
	// Watermark, dimensions and samples belong to one read-only snapshot.
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var usage, errors, through, computed, upgradeStart sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT w.usage_coverage_start,w.error_coverage_start,w.data_through,w.last_successful_at,c.status_cards_coverage_start FROM channel_monitor_v2_watermarks w JOIN channel_monitor_v2_config c ON c.id=w.id WHERE w.id=1`).Scan(&usage, &errors, &through, &computed, &upgradeStart)
	if err == sql.ErrNoRows || (err == nil && (!usage.Valid || !errors.Valid || !through.Valid || !computed.Valid)) {
		out.Stale = true
		return out, tx.Commit()
	}
	if err != nil {
		return nil, err
	}
	asOf := through.Time.UTC()
	if asOf.After(q.ServerNow) {
		asOf = q.ServerNow
	}
	asOf = asOf.Truncate(5 * time.Minute)
	if q.AsOf != nil {
		if q.AsOf.After(asOf) {
			return nil, fmt.Errorf("%w: as_of exceeds aggregation watermark", service.ErrChannelMonitorV2InvalidRange)
		}
		asOf = q.AsOf.UTC()
	}
	covered := usage.Time.UTC()
	if errors.Time.After(covered) {
		covered = errors.Time.UTC()
	}
	if upgradeStart.Valid && upgradeStart.Time.After(covered) {
		covered = upgradeStart.Time.UTC()
	}
	retained := q.ServerNow.Add(-channelMonitorV2RetentionRollup5m)
	if retained.After(covered) {
		covered = retained
	}
	// Partial five-minute buckets cannot establish complete coverage.
	if !covered.Equal(covered.Truncate(5 * time.Minute)) {
		covered = covered.Truncate(5 * time.Minute).Add(5 * time.Minute)
	}
	lag := int64(q.ServerNow.Sub(asOf).Seconds())
	freshness := int64(3 * cfg.RefreshIntervalSeconds)
	if freshness < 900 {
		freshness = 900
	}
	computedAt := computed.Time.UTC()
	out.AsOf, out.ComputedAt, out.AggregationLagSeconds = &asOf, &computedAt, &lag
	out.CoverageStart = &covered
	out.CoverageComplete = !covered.After(asOf.Add(-7 * 24 * time.Hour))
	out.Stale = lag > freshness
	if !covered.Before(asOf) {
		return out, tx.Commit()
	}

	filter := q.Filter
	filter.Start, filter.End, filter.Bucket = asOf.Add(-7*24*time.Hour), asOf, 5*time.Minute
	if covered.After(filter.Start) {
		filter.Start = covered
	}
	where, args, _ := channelMonitorV2WhereWithRollup(filter, cfg, "m")
	modelExpr := channelMonitorV2CardModelSQL(cfg, "m", &args)
	if len(filter.Models) > 0 {
		args = append(args, pq.Array(filter.Models))
		where += fmt.Sprintf(" AND (%s) = ANY($%d)", modelExpr, len(args))
	}
	where += " AND m.group_id > 0"
	args = append(args, q.PageSize+1, (q.Page-1)*q.PageSize)
	query := fmt.Sprintf(`SELECT m.platform,m.group_id,g.name,%s AS display_model
		FROM channel_monitor_v2_metrics_rollup m JOIN groups g ON g.id=m.group_id AND g.deleted_at IS NULL AND g.status='active'
		%s GROUP BY m.platform,m.group_id,g.name,display_model
		HAVING SUM(m.success_requests+m.error_requests)>0
		ORDER BY m.platform,m.group_id,display_model LIMIT $%d OFFSET $%d`, modelExpr, where, len(args)-1, len(args))
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	identities := []service.ChannelMonitorV2CardIdentity{}
	names := map[service.ChannelMonitorV2CardIdentity]string{}
	for rows.Next() {
		var id service.ChannelMonitorV2CardIdentity
		var name string
		if err := rows.Scan(&id.Platform, &id.GroupID, &name, &id.Model); err != nil {
			_ = rows.Close()
			return nil, err
		}
		identities = append(identities, id)
		names[id] = name
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	if len(identities) > q.PageSize {
		out.HasMore = true
		identities = identities[:q.PageSize]
	}
	if len(identities) == 0 {
		return out, tx.Commit()
	}
	accs := make(map[service.ChannelMonitorV2CardIdentity]*channelMonitorV2CardAccumulator, len(identities))
	for _, id := range identities {
		accs[id] = newChannelMonitorV2CardAccumulator()
	}

	// Both sample queries are constrained to this page's exact display identities.
	where, args, _ = channelMonitorV2WhereWithRollup(filter, cfg, "m")
	modelExpr = channelMonitorV2CardModelSQL(cfg, "m", &args)
	pageJSON, err := json.Marshal(identities)
	if err != nil {
		return nil, err
	}
	args = append(args, string(pageJSON))
	pageJoin := fmt.Sprintf(` JOIN jsonb_to_recordset($%d::jsonb) AS selected(platform text,group_id bigint,model text)
		ON selected.platform=m.platform AND selected.group_id=m.group_id AND selected.model=(%s) `, len(args), modelExpr)
	rows, err = tx.QueryContext(ctx, `SELECT m.bucket_start,m.platform,m.group_id,selected.model,
		SUM(m.success_requests),SUM(m.error_requests),SUM(m.input_tokens),SUM(m.output_tokens),SUM(m.cache_creation_tokens),SUM(m.cache_read_tokens),
		SUM(m.ttft_sum_ms),SUM(m.ttft_count),SUM(m.duration_sum_ms),SUM(m.duration_count)
		FROM channel_monitor_v2_metrics_rollup m`+pageJoin+where+` GROUP BY m.bucket_start,m.platform,m.group_id,selected.model`, args...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var at time.Time
		var id service.ChannelMonitorV2CardIdentity
		var f channelMonitorV2Fact
		if err := rows.Scan(&at, &id.Platform, &id.GroupID, &id.Model, &f.Success, &f.Errors, &f.Input, &f.Output, &f.CacheCreation, &f.CacheRead, &f.TTFTSum, &f.TTFTCount, &f.DurationSum, &f.DurationCount); err != nil {
			_ = rows.Close()
			return nil, err
		}
		for _, acc := range accs[id].at(at, asOf) {
			acc.addFact(f)
		}
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT m.bucket_start,m.platform,m.group_id,selected.model,m.metric,m.upper_bound_ms,SUM(m.sample_count)
		FROM channel_monitor_v2_latency_histograms_rollup m`+pageJoin+where+` AND m.user_id=0 GROUP BY m.bucket_start,m.platform,m.group_id,selected.model,m.metric,m.upper_bound_ms`, args...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var at time.Time
		var id service.ChannelMonitorV2CardIdentity
		var h channelMonitorV2Histogram
		if err := rows.Scan(&at, &id.Platform, &id.GroupID, &id.Model, &h.Metric, &h.UpperBound, &h.Count); err != nil {
			_ = rows.Close()
			return nil, err
		}
		for _, acc := range accs[id].at(at, asOf) {
			acc.addHistogram(h)
		}
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	for _, id := range identities {
		a := accs[id]
		card := a.card(id, names[id], asOf, covered, cfg)
		out.Items = append(out.Items, card)
		if q.IncludeAdmin {
			metrics := service.ChannelMonitorV2AdminCardMetrics{H24: a.h24.metric(1440, false), D7: a.d7.metric(7*1440, false), Current: a.current.metric(90, false)}
			for _, m := range []*service.ChannelMonitorV2Metric{&metrics.H24, &metrics.D7, &metrics.Current} {
				m.Measurement = service.ChannelMonitorV2MetricEvidence(*m, cfg.HealthThresholds.MinimumSample)
			}
			out.AppendAdminCard(card, metrics)
		}
	}
	return out, tx.Commit()
}

func channelMonitorV2CardModelSQL(cfg service.ChannelMonitorV2Config, alias string, args *[]any) string {
	raw := "COALESCE(NULLIF(TRIM(" + alias + ".model),''),'__other__')"
	parts := []string{"CASE"}
	for _, p := range cfg.Platforms {
		if len(p.Models) == 0 {
			continue
		}
		*args = append(*args, p.Platform, pq.Array(p.Models))
		parts = append(parts, fmt.Sprintf("WHEN %s.platform=$%d AND NOT (%s=ANY($%d)) THEN '__other__'", alias, len(*args)-1, raw, len(*args)))
	}
	if len(parts) == 1 {
		return raw
	}
	return strings.Join(append(parts, "ELSE "+raw+" END"), " ")
}

type channelMonitorV2CardAccumulator struct {
	d7, h24, current *metricAccumulator
	slots            [72]*metricAccumulator
}

func newChannelMonitorV2CardAccumulator() *channelMonitorV2CardAccumulator {
	a := &channelMonitorV2CardAccumulator{d7: newMetricAccumulator(), h24: newMetricAccumulator(), current: newMetricAccumulator()}
	for i := range a.slots {
		a.slots[i] = newMetricAccumulator()
	}
	return a
}

func (a *channelMonitorV2CardAccumulator) at(at, asOf time.Time) []*metricAccumulator {
	if at.Before(asOf.Add(-7*24*time.Hour)) || !at.Before(asOf) {
		return nil
	}
	out := []*metricAccumulator{a.d7}
	start := asOf.Add(-24 * time.Hour)
	if !at.Before(start) {
		i := int(at.Sub(start) / service.ChannelMonitorV2CardBucket)
		out = append(out, a.h24, a.slots[i])
	}
	if !at.Before(asOf.Add(-90 * time.Minute)) {
		out = append(out, a.current)
	}
	return out
}

func channelMonitorV2CardWindow(acc *metricAccumulator, start, end, covered time.Time, minimum int64) service.ChannelMonitorV2CardWindow {
	m := acc.metric(end.Sub(start).Minutes(), false)
	e := service.ChannelMonitorV2MetricEvidence(m, minimum)
	w := service.ChannelMonitorV2CardWindow{Evidence: e, CoverageComplete: !covered.After(start)}
	if covered.After(start) {
		start = covered
	}
	if start.Before(end) {
		w.CoveredStart, w.CoveredEnd = &start, &end
	}
	if e.HasRequests {
		w.SuccessRate = &m.SuccessRate
	}
	if e.HasTTFT {
		w.TTFTP50Ms, w.TTFTP90Ms = m.TTFT.P50Ms, m.TTFT.P90Ms
	}
	if e.HasDuration {
		w.DurationP50Ms = m.Duration.P50Ms
	}
	if e.HasCacheMeasurement {
		w.CacheReadRatio = &m.CacheRate
	}
	return w
}

func (a *channelMonitorV2CardAccumulator) card(id service.ChannelMonitorV2CardIdentity, name string, asOf, covered time.Time, cfg service.ChannelMonitorV2Config) service.ChannelMonitorV2StatusCard {
	start := asOf.Add(-24 * time.Hour)
	m := a.current.metric(90, false)
	card := service.ChannelMonitorV2StatusCard{
		Identity: id, Display: service.ChannelMonitorV2CardDisplay{PlatformLabel: id.Platform, GroupLabel: name, ModelLabel: channelMonitorV2ModelLabel(id.Model)}, Source: "real_traffic",
		Current:  service.ChannelMonitorV2CardCurrent{RequestState: service.ChannelMonitorV2CardRequestState(m, cfg.HealthThresholds), PerformanceState: service.ChannelMonitorV2CardPerformanceState(m, cfg.StatusCardSettings, cfg.HealthThresholds.MinimumSample), WindowSeconds: 5400},
		Windows:  service.ChannelMonitorV2CardWindows{H24: channelMonitorV2CardWindow(a.h24, start, asOf, covered, cfg.HealthThresholds.MinimumSample), D7: channelMonitorV2CardWindow(a.d7, asOf.Add(-7*24*time.Hour), asOf, covered, cfg.HealthThresholds.MinimumSample)},
		Timeline: make([]service.ChannelMonitorV2CardSlot, 72),
	}
	for i, acc := range a.slots {
		s := start.Add(time.Duration(i) * service.ChannelMonitorV2CardBucket)
		end := s.Add(service.ChannelMonitorV2CardBucket)
		w := channelMonitorV2CardWindow(acc, s, end, covered, cfg.HealthThresholds.MinimumSample)
		card.Timeline[i] = service.ChannelMonitorV2CardSlot{Start: s, End: end, SuccessRate: w.SuccessRate, TTFTP90Ms: w.TTFTP90Ms, State: service.ChannelMonitorV2CardRequestState(acc.metric(20, false), cfg.HealthThresholds)}
	}
	if service.IsImageGenerationIntentForPlatform("", id.Model, nil, id.Platform) {
		card.Current.PerformanceState = "unknown"
		card.Windows.H24.TTFTP50Ms, card.Windows.H24.TTFTP90Ms = nil, nil
		card.Windows.D7.TTFTP50Ms, card.Windows.D7.TTFTP90Ms = nil, nil
		card.Windows.H24.Evidence.HasTTFT, card.Windows.D7.Evidence.HasTTFT = false, false
		for i := range card.Timeline {
			card.Timeline[i].TTFTP90Ms = nil
		}
	}
	return card
}
