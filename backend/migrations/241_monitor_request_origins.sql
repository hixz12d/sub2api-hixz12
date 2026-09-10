-- Align previously collected attribution with the versioned five-source contract.
-- NULL historical attribution stays NULL; readers treat it as legacy_unknown.
ALTER TABLE usage_logs DROP CONSTRAINT IF EXISTS usage_logs_monitor_observation_check;
UPDATE usage_logs SET request_origin = CASE request_origin
    WHEN 'business' THEN 'real_traffic'
    WHEN 'capability_detector' THEN 'capability_probe'
    END WHERE request_origin IN ('business', 'capability_detector');

ALTER TABLE usage_logs ADD CONSTRAINT usage_logs_monitor_observation_check CHECK (
    (request_origin IS NULL OR request_origin IN ('real_traffic','availability_probe','capability_probe','user_detector','legacy_unknown'))
    AND (monitor_observation_version IS NULL OR monitor_observation_version > 0)
    AND (monitor_input_tokens_total IS NULL OR monitor_input_tokens_total >= 0)
    AND (monitor_cache_read_tokens IS NULL OR (monitor_input_tokens_total IS NOT NULL AND monitor_cache_read_tokens BETWEEN 0 AND monitor_input_tokens_total))
    AND (monitor_visible_output_tokens IS NULL OR monitor_visible_output_tokens >= 0)
    AND (monitor_generation_ms IS NULL OR monitor_generation_ms >= 0)
    AND (monitor_first_visible_ms IS NULL OR monitor_first_visible_ms >= 0)
    AND (monitor_output_tps_milli IS NULL OR (
        request_origin IS NOT NULL AND request_origin='real_traffic'
        AND monitor_observation_version IS NOT NULL AND monitor_observation_version=1
        AND monitor_tps_method IS NOT NULL AND monitor_tps_method='visible_stream_v1'
        AND monitor_visible_output_tokens IS NOT NULL AND monitor_visible_output_tokens >= 16
        AND monitor_generation_ms IS NOT NULL AND monitor_generation_ms >= 200
        AND monitor_output_tps_milli >= 0
    ))
) NOT VALID;

ALTER TABLE ops_error_logs ADD COLUMN IF NOT EXISTS request_origin VARCHAR(32);
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='ops_error_logs'::regclass AND conname='ops_error_logs_request_origin_check') THEN
        ALTER TABLE ops_error_logs ADD CONSTRAINT ops_error_logs_request_origin_check CHECK (
            request_origin IS NULL OR request_origin IN ('real_traffic','availability_probe','capability_probe','user_detector','legacy_unknown')
        ) NOT VALID;
    END IF;
END $$;
