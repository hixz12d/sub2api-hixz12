-- Keep legacy TTFT untouched; visible text arrival is a separate metric.
ALTER TABLE channel_monitor_v2_latency_histograms_1m
    DROP CONSTRAINT IF EXISTS channel_monitor_v2_latency_histograms_1m_metric_check;
ALTER TABLE channel_monitor_v2_latency_histograms_1m
    ADD CONSTRAINT channel_monitor_v2_latency_histograms_1m_metric_check
    CHECK (metric IN ('ttft','duration','visible_ttft_v1')) NOT VALID;
ALTER TABLE channel_monitor_v2_latency_histograms_rollup
    DROP CONSTRAINT IF EXISTS channel_monitor_v2_latency_histograms_rollup_metric_check;
ALTER TABLE channel_monitor_v2_latency_histograms_rollup
    ADD CONSTRAINT channel_monitor_v2_latency_histograms_rollup_metric_check
    CHECK (metric IN ('ttft','duration','visible_ttft_v1')) NOT VALID;

-- A conservative collection boundary, never moved backwards by legacy backfill.
-- NULL means the new version has not recomputed a window successfully yet.
ALTER TABLE channel_monitor_v2_watermarks
    ADD COLUMN IF NOT EXISTS observation_v1_collection_start TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS observation_v1_data_through TIMESTAMPTZ;
UPDATE channel_monitor_v2_watermarks
SET observation_v1_collection_start=NOW()
WHERE observation_v1_collection_start IS NULL;
