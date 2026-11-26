-- Additive model consistency counters; zeros mean no measured samples, not 0% consistency.
ALTER TABLE channel_monitor_v2_metrics_1m
    ADD COLUMN IF NOT EXISTS model_observed_requests BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS model_mismatch_requests BIGINT NOT NULL DEFAULT 0;

ALTER TABLE channel_monitor_v2_metrics_rollup
    ADD COLUMN IF NOT EXISTS model_observed_requests BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS model_mismatch_requests BIGINT NOT NULL DEFAULT 0;
