-- Lifetime campaign ceilings do not reset at UTC midnight or on process restart.
-- Each physical attempt charges a conservative upper bound before dispatch.
CREATE TABLE IF NOT EXISTS monitor_spend_campaigns (
 id UUID PRIMARY KEY,
 request_limit BIGINT NOT NULL CHECK (request_limit > 0 AND request_limit <= 1000000000),
 usd_limit_micros BIGINT NOT NULL CHECK (usd_limit_micros > 0 AND usd_limit_micros <= 1000000000000),
 requests_charged BIGINT NOT NULL DEFAULT 0 CHECK (requests_charged >= 0 AND requests_charged <= request_limit),
 usd_charged_micros BIGINT NOT NULL DEFAULT 0 CHECK (usd_charged_micros >= 0 AND usd_charged_micros <= usd_limit_micros),
 enabled BOOLEAN NOT NULL DEFAULT FALSE,
 created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS monitor_spend_charges (
 outbound_request_id UUID PRIMARY KEY,
 campaign_id UUID NOT NULL REFERENCES monitor_spend_campaigns(id) ON DELETE RESTRICT,
 job_id UUID NOT NULL REFERENCES monitor_jobs(id) ON DELETE RESTRICT,
 upper_bound_micros BIGINT NOT NULL CHECK (upper_bound_micros > 0 AND upper_bound_micros <= 1000000000000),
 created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS monitor_spend_charges_job_idx ON monitor_spend_charges(job_id);
