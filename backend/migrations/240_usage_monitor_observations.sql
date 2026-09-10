-- No defaults/backfill: historical and unadapted observations remain unknown.
ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS request_origin VARCHAR(32),
    ADD COLUMN IF NOT EXISTS monitor_observation_version INTEGER,
    ADD COLUMN IF NOT EXISTS monitor_input_tokens_total BIGINT,
    ADD COLUMN IF NOT EXISTS monitor_cache_read_tokens BIGINT,
    ADD COLUMN IF NOT EXISTS monitor_visible_output_tokens BIGINT,
    ADD COLUMN IF NOT EXISTS monitor_generation_ms BIGINT,
    ADD COLUMN IF NOT EXISTS monitor_output_tps_milli BIGINT,
    ADD COLUMN IF NOT EXISTS monitor_tps_method VARCHAR(32),
    ADD COLUMN IF NOT EXISTS monitor_first_visible_ms BIGINT;

-- Constraints are added without scanning the historical usage table. They apply
-- to new writes; validation of old rows can be scheduled independently.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='usage_logs'::regclass AND conname='usage_logs_monitor_observation_check') THEN
        ALTER TABLE usage_logs ADD CONSTRAINT usage_logs_monitor_observation_check CHECK (
            (request_origin IS NULL OR request_origin IN ('business','availability_probe','capability_detector'))
            AND (monitor_observation_version IS NULL OR monitor_observation_version > 0)
            AND (monitor_input_tokens_total IS NULL OR monitor_input_tokens_total >= 0)
            AND (monitor_cache_read_tokens IS NULL OR (monitor_input_tokens_total IS NOT NULL AND monitor_cache_read_tokens BETWEEN 0 AND monitor_input_tokens_total))
            AND (monitor_visible_output_tokens IS NULL OR monitor_visible_output_tokens >= 0)
            AND (monitor_generation_ms IS NULL OR monitor_generation_ms >= 0)
            AND (monitor_first_visible_ms IS NULL OR monitor_first_visible_ms >= 0)
            AND (monitor_output_tps_milli IS NULL OR (
                request_origin IS NOT NULL AND request_origin='business'
                AND monitor_observation_version IS NOT NULL AND monitor_observation_version=1
                AND monitor_tps_method IS NOT NULL AND monitor_tps_method='visible_stream_v1'
                AND monitor_visible_output_tokens IS NOT NULL AND monitor_visible_output_tokens >= 16
                AND monitor_generation_ms IS NOT NULL AND monitor_generation_ms >= 200
                AND monitor_output_tps_milli >= 0
            ))
        ) NOT VALID;
    END IF;
END $$;
