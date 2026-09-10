package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Version 1 writes first-visible latency only after validated, fully delivered
// text output. Billing cost is not evidence of completion. Other protocols
// remain unmeasured until they provide their own validated observation contract.
const channelMonitorV1CompletedTextSQL = `ul.request_origin='real_traffic'
 AND ul.monitor_observation_version=1 AND ul.monitor_first_visible_ms>=0
 AND COALESCE(ul.request_type,0) NOT IN (4,6)`

// This is part of the existing recompute transaction, never an independent scan
// or scheduler. Billing token fields are deliberately absent from this query.
func (r *channelMonitorV2Repository) recomputeMonitorObservations(ctx context.Context, tx *sql.Tx, start, end time.Time) error {
	const validCache = `ul.monitor_observation_version=1 AND ul.monitor_input_tokens_total>0 AND ul.monitor_cache_read_tokens BETWEEN 0 AND ul.monitor_input_tokens_total`
	query := `UPDATE channel_monitor_v2_metrics_1m m SET
 monitor_metric_version=1,monitor_candidate_requests=o.candidates,
 monitor_cache_measured_requests=o.measured,monitor_input_tokens_total=o.total,
 monitor_cache_read_tokens=o.cached
 FROM (SELECT date_trunc('minute',ul.created_at) AS bucket_start,` + channelMonitorV2PlatformSQL + ` AS platform,
 COALESCE(ul.group_id,0) AS group_id,` + channelMonitorV2ModelSQL + ` AS model,
 COUNT(*) AS candidates,COUNT(*) FILTER(WHERE ` + validCache + `) AS measured,
 COALESCE(SUM(ul.monitor_input_tokens_total) FILTER(WHERE ` + validCache + `),0) AS total,
 COALESCE(SUM(ul.monitor_cache_read_tokens) FILTER(WHERE ` + validCache + `),0) AS cached
 FROM usage_logs ul LEFT JOIN groups g ON g.id=ul.group_id LEFT JOIN accounts a ON a.id=ul.account_id
 WHERE ul.created_at >= $1 AND ul.created_at < $2 AND ul.request_origin='real_traffic'
 AND ` + channelMonitorV1CompletedTextSQL + `
 GROUP BY 1,2,3,4) o
 WHERE m.bucket_start=o.bucket_start AND m.platform=o.platform AND m.group_id=o.group_id AND m.model=o.model`
	if _, err := tx.ExecContext(ctx, query, start, end); err != nil {
		return fmt.Errorf("aggregate monitor cache: %w", err)
	}
	bounds := service.MonitorTPSBoundsV1()
	var bucket strings.Builder
	bucket.WriteString("CASE")
	for i, upper := range bounds {
		fmt.Fprintf(&bucket, " WHEN ul.monitor_output_tps_milli <= %d THEN %d", upper, i)
	}
	bucket.WriteString(" ELSE " + strconv.Itoa(len(bounds)) + " END")
	query = `INSERT INTO channel_monitor_v2_tps_histograms_1m(bucket_start,platform,group_id,model,metric_version,bucket_index,sample_count)
 SELECT date_trunc('minute',ul.created_at),` + channelMonitorV2PlatformSQL + `,COALESCE(ul.group_id,0),` + channelMonitorV2ModelSQL + `,1,` + bucket.String() + `,COUNT(*)
 FROM usage_logs ul LEFT JOIN groups g ON g.id=ul.group_id LEFT JOIN accounts a ON a.id=ul.account_id
 WHERE ul.created_at >= $1 AND ul.created_at < $2 AND ul.request_origin='real_traffic'
 AND ul.monitor_observation_version=1 AND ul.monitor_tps_method='visible_stream_v1'
 AND ul.monitor_output_tps_milli>=0 AND ul.monitor_visible_output_tokens>=16 AND ul.monitor_generation_ms>=200
 AND ` + channelMonitorV1CompletedTextSQL + ` GROUP BY 1,2,3,4,5,6`
	if _, err := tx.ExecContext(ctx, query, start, end); err != nil {
		return fmt.Errorf("aggregate monitor TPS: %w", err)
	}
	return nil
}

const channelMonitorV2TPSRollupSQL = `
INSERT INTO channel_monitor_v2_tps_histograms_rollup(bucket_start,bucket_seconds,platform,group_id,model,metric_version,bucket_index,sample_count)
` + channelMonitorV2FixedRollupBoundsSQL + `
SELECT date_bin($1::interval,h.bucket_start,TIMESTAMPTZ '1970-01-01'),$2::integer,
 platform,group_id,model,metric_version,bucket_index,SUM(sample_count)
FROM channel_monitor_v2_tps_histograms_1m h,bounds
WHERE h.bucket_start>=bounds.start_at AND h.bucket_start<bounds.end_at
GROUP BY 1,2,3,4,5,6,7`
