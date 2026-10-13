-- Nullable request timing breakdown: client upload, gateway processing, upstream wait before first token.
ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS request_receive_ms INTEGER,
    ADD COLUMN IF NOT EXISTS gateway_ms INTEGER,
    ADD COLUMN IF NOT EXISTS upstream_wait_ms INTEGER;
