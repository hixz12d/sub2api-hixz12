ALTER TABLE channel_monitor_v2_config
    ADD COLUMN IF NOT EXISTS status_card_settings JSONB NOT NULL DEFAULT
    '{"ttft_p90_warning_ms":null,"ttft_p90_critical_ms":null}'::jsonb;

-- Extending retention cannot restore rows pruned before this upgrade.
ALTER TABLE channel_monitor_v2_config
    ADD COLUMN IF NOT EXISTS status_cards_coverage_start TIMESTAMPTZ NOT NULL
    DEFAULT CURRENT_TIMESTAMP - INTERVAL '7 days';
