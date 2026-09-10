-- Additive observation counters; zeros mean no measured samples, not 0% cache.
ALTER TABLE channel_monitor_v2_metrics_1m
    ADD COLUMN IF NOT EXISTS monitor_metric_version INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS monitor_candidate_requests BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS monitor_cache_measured_requests BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS monitor_input_tokens_total BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS monitor_cache_read_tokens BIGINT NOT NULL DEFAULT 0;
ALTER TABLE channel_monitor_v2_metrics_rollup
    ADD COLUMN IF NOT EXISTS monitor_metric_version INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS monitor_candidate_requests BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS monitor_cache_measured_requests BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS monitor_input_tokens_total BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS monitor_cache_read_tokens BIGINT NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS channel_monitor_v2_tps_histograms_1m (
    bucket_start TIMESTAMPTZ NOT NULL,
    platform TEXT NOT NULL,
    group_id BIGINT NOT NULL,
    model TEXT NOT NULL,
    metric_version INTEGER NOT NULL CHECK (metric_version > 0),
    bucket_index INTEGER NOT NULL CHECK (bucket_index >= 0),
    sample_count BIGINT NOT NULL CHECK (sample_count > 0),
    PRIMARY KEY (bucket_start,platform,group_id,model,metric_version,bucket_index)
);
CREATE TABLE IF NOT EXISTS channel_monitor_v2_tps_histograms_rollup (
    bucket_start TIMESTAMPTZ NOT NULL,
    bucket_seconds INTEGER NOT NULL CHECK (bucket_seconds IN (300,3600,43200,86400)),
    platform TEXT NOT NULL,
    group_id BIGINT NOT NULL,
    model TEXT NOT NULL,
    metric_version INTEGER NOT NULL CHECK (metric_version > 0),
    bucket_index INTEGER NOT NULL CHECK (bucket_index >= 0),
    sample_count BIGINT NOT NULL CHECK (sample_count > 0),
    PRIMARY KEY (bucket_start,bucket_seconds,platform,group_id,model,metric_version,bucket_index)
);
CREATE INDEX IF NOT EXISTS channel_monitor_v2_tps_group_time_idx
    ON channel_monitor_v2_tps_histograms_rollup(group_id,bucket_seconds,bucket_start);
