-- P1 persistence only. No schedules are seeded and no existing monitor is enabled.
-- These tables follow the SQL-owned V2 repository pattern, not Ent-generated CRUD.
INSERT INTO settings (key, value) VALUES
    ('channel_monitor_group_view_enabled', 'false'),
    ('channel_monitor_group_probe_enabled', 'false'),
    ('channel_monitor_show_output_tps', 'false'),
    ('llm_detector_enabled', 'false'),
    ('llm_detector_user_testing_enabled', 'false'),
    ('llm_detector_scheduled_enabled', 'false')
ON CONFLICT (key) DO NOTHING;

CREATE TABLE IF NOT EXISTS channel_monitor_group_policies (
    id BIGSERIAL PRIMARY KEY,
    group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE RESTRICT,
    display_name VARCHAR(200) NOT NULL DEFAULT '',
    primary_model VARCHAR(200) NOT NULL CHECK (length(btrim(primary_model)) > 0),
    extra_models JSONB NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(extra_models) = 'array' AND jsonb_array_length(extra_models) <= 19),
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    probe_config JSONB NOT NULL DEFAULT '{"enabled":false,"interval_seconds":60,"jitter_seconds":15,"timeout_seconds":45,"selection_mode":"random","fixed_account_ids":[],"sample_size":1,"include_extra_models":false,"daily_request_limit":2000}'
        CHECK (jsonb_typeof(probe_config) = 'object'),
    capability_config JSONB NOT NULL DEFAULT '{"enabled":false,"interval_seconds":43200,"jitter_seconds":6480,"selection_mode":"random","fixed_account_ids":[],"sample_size":1,"tier":"low","execution_timeout_seconds":1800,"result_ttl_seconds":86400,"daily_request_limit":1000,"retest_on_mismatch":true,"retest_delay_seconds":1800,"retest_limit_per_execution":1,"targets":[]}'
        CHECK (jsonb_typeof(capability_config) = 'object'),
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    evaluation_revision TEXT NOT NULL CHECK (evaluation_revision ~ '^[0-9a-f]{64}$'),
    next_probe_at TIMESTAMPTZ,
    next_capability_at TIMESTAMPTZ,
    -- Active links are scheduler-owned; the active-job partial index is authoritative.
    active_probe_job_id UUID,
    active_capability_job_id UUID,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    updated_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS channel_monitor_group_policies_live_group_uq
    ON channel_monitor_group_policies (group_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS channel_monitor_group_policies_probe_due_idx
    ON channel_monitor_group_policies (next_probe_at) WHERE enabled AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS channel_monitor_group_policies_capability_due_idx
    ON channel_monitor_group_policies (next_capability_at) WHERE enabled AND deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS llm_detector_plans (
    id UUID PRIMARY KEY,
    owner_user_id BIGINT REFERENCES users(id) ON DELETE RESTRICT,
    source TEXT NOT NULL CHECK (source IN ('platform_group', 'external_api', 'site_api_key')),
    policy_id BIGINT REFERENCES channel_monitor_group_policies(id) ON DELETE RESTRICT,
    evaluation_revision TEXT CHECK (evaluation_revision ~ '^[0-9a-f]{64}$'),
    normalized_target_spec JSONB NOT NULL CHECK (jsonb_typeof(normalized_target_spec) = 'object' AND octet_length(normalized_target_spec::text) <= 65536),
    benchmark_manifest JSONB NOT NULL CHECK (jsonb_typeof(benchmark_manifest) = 'object' AND octet_length(benchmark_manifest::text) <= 65536),
    configuration_hash TEXT NOT NULL CHECK (configuration_hash ~ '^[0-9a-f]{64}$'),
    planned_base_requests BIGINT NOT NULL CHECK (planned_base_requests > 0 AND planned_base_requests <= 5000),
    retry_budget_requests BIGINT NOT NULL CHECK (retry_budget_requests >= 0),
    maximum_outbound_requests BIGINT NOT NULL CHECK (maximum_outbound_requests = planned_base_requests + retry_budget_requests AND maximum_outbound_requests <= 5000),
    estimated_tokens BIGINT CHECK (estimated_tokens >= 0),
    estimated_cost NUMERIC(24, 10) CHECK (estimated_cost >= 0),
    estimate_currency VARCHAR(16),
    estimate_status TEXT NOT NULL DEFAULT 'unknown' CHECK (estimate_status IN ('unknown', 'estimated')),
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (expires_at > created_at),
    CHECK ((source = 'platform_group' AND policy_id IS NOT NULL AND evaluation_revision IS NOT NULL)
        OR (source IN ('external_api', 'site_api_key') AND owner_user_id IS NOT NULL AND policy_id IS NULL AND evaluation_revision IS NULL)),
    UNIQUE (id, owner_user_id, source)
);
CREATE INDEX IF NOT EXISTS llm_detector_plans_owner_idx ON llm_detector_plans(owner_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS llm_detector_plans_expiry_idx ON llm_detector_plans(expires_at);

CREATE TABLE IF NOT EXISTS monitor_jobs (
    id UUID PRIMARY KEY,
    kind TEXT NOT NULL CHECK (kind IN ('availability', 'capability')),
    source TEXT NOT NULL CHECK (source IN ('platform_group', 'external_api', 'site_api_key')),
    policy_id BIGINT REFERENCES channel_monitor_group_policies(id) ON DELETE RESTRICT,
    evaluation_revision TEXT CHECK (evaluation_revision ~ '^[0-9a-f]{64}$'),
    -- This unique link is the single source of truth for plan consumption.
    plan_id UUID UNIQUE REFERENCES llm_detector_plans(id) ON DELETE RESTRICT,
    owner_user_id BIGINT REFERENCES users(id) ON DELETE RESTRICT,
    idempotency_scope VARCHAR(200) NOT NULL CHECK (length(idempotency_scope) > 0),
    idempotency_key_hash TEXT NOT NULL CHECK (idempotency_key_hash ~ '^[0-9a-f]{64}$'),
    payload_hash TEXT NOT NULL CHECK (payload_hash ~ '^[0-9a-f]{64}$'),
    schedule_occurrence_id VARCHAR(200),
    state TEXT NOT NULL DEFAULT 'queued' CHECK (state IN ('queued','running','cancelling','completed','failed','cancelled','interrupted','skipped')),
    config_snapshot JSONB NOT NULL CHECK (jsonb_typeof(config_snapshot) = 'object' AND octet_length(config_snapshot::text) <= 262144),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    deadline_at TIMESTAMPTZ NOT NULL,
    queue_deadline_at TIMESTAMPTZ NOT NULL,
    lease_owner VARCHAR(200),
    lease_expires_at TIMESTAMPTZ,
    lease_generation BIGINT NOT NULL DEFAULT 0 CHECK (lease_generation >= 0),
    secret_owner_instance_id VARCHAR(200),
    cancel_requested_at TIMESTAMPTZ,
    failure_code VARCHAR(100),
    base_requests_planned BIGINT NOT NULL CHECK (base_requests_planned > 0 AND base_requests_planned <= 5000),
    outbound_reserved BIGINT NOT NULL CHECK (outbound_reserved >= base_requests_planned AND outbound_reserved <= 5000),
    outbound_dispatched BIGINT NOT NULL DEFAULT 0 CHECK (outbound_dispatched >= 0 AND outbound_dispatched <= outbound_reserved),
    outbound_completed BIGINT NOT NULL DEFAULT 0 CHECK (outbound_completed >= 0 AND outbound_completed <= outbound_dispatched),
    CHECK (deadline_at > created_at AND deadline_at <= created_at + INTERVAL '2 hours'),
    CHECK (queue_deadline_at > created_at AND queue_deadline_at <= deadline_at),
    CHECK ((source = 'platform_group' AND policy_id IS NOT NULL AND evaluation_revision IS NOT NULL)
        OR (source IN ('external_api','site_api_key') AND owner_user_id IS NOT NULL AND policy_id IS NULL AND plan_id IS NOT NULL AND evaluation_revision IS NULL)),
    CHECK (kind = 'capability' OR source = 'platform_group'),
    CHECK (source <> 'external_api' OR NULLIF(secret_owner_instance_id, '') IS NOT NULL),
    CHECK ((lease_owner IS NULL) = (lease_expires_at IS NULL)),
    CHECK (state NOT IN ('running','cancelling') OR (lease_owner IS NOT NULL AND lease_generation > 0)),
    CHECK ((state IN ('completed','failed','cancelled','interrupted','skipped')) = (finished_at IS NOT NULL)),
    UNIQUE (idempotency_scope, idempotency_key_hash),
    UNIQUE (id, source),
    FOREIGN KEY (plan_id, owner_user_id, source) REFERENCES llm_detector_plans(id, owner_user_id, source) ON DELETE RESTRICT
);
CREATE UNIQUE INDEX IF NOT EXISTS monitor_jobs_occurrence_uq
    ON monitor_jobs(policy_id, kind, schedule_occurrence_id) WHERE schedule_occurrence_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS monitor_jobs_active_policy_uq
    ON monitor_jobs(policy_id, kind) WHERE policy_id IS NOT NULL AND state IN ('queued','running','cancelling');
CREATE INDEX IF NOT EXISTS monitor_jobs_queue_idx ON monitor_jobs(state, queue_deadline_at);
CREATE INDEX IF NOT EXISTS monitor_jobs_lease_idx ON monitor_jobs(lease_expires_at) WHERE state IN ('running','cancelling');
CREATE INDEX IF NOT EXISTS monitor_jobs_owner_idx ON monitor_jobs(owner_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS monitor_jobs_policy_idx ON monitor_jobs(policy_id, kind, created_at DESC);

CREATE TABLE IF NOT EXISTS llm_detector_executions (
    id UUID PRIMARY KEY,
    job_id UUID NOT NULL REFERENCES monitor_jobs(id) ON DELETE RESTRICT,
    source TEXT NOT NULL,
    target_index INTEGER NOT NULL CHECK (target_index >= 0 AND target_index < 24),
    group_id BIGINT REFERENCES groups(id) ON DELETE RESTRICT,
    effective_group_id BIGINT REFERENCES groups(id) ON DELETE RESTRICT,
    account_id BIGINT REFERENCES accounts(id) ON DELETE RESTRICT,
    credential_revision VARCHAR(200),
    site_api_key_id BIGINT REFERENCES api_keys(id) ON DELETE RESTRICT,
    request_model VARCHAR(200) NOT NULL CHECK (length(request_model) > 0),
    resolved_upstream_model VARCHAR(200),
    claimed_model VARCHAR(200) NOT NULL CHECK (length(claimed_model) > 0 AND claimed_model <> 'reference-only:other'),
    benchmark_id VARCHAR(200) NOT NULL,
    benchmark_version VARCHAR(100) NOT NULL,
    benchmark_sha256 TEXT NOT NULL CHECK (benchmark_sha256 ~ '^[0-9a-f]{64}$'),
    engine_commit VARCHAR(64) NOT NULL,
    engine_version VARCHAR(100) NOT NULL,
    scoring_version VARCHAR(100) NOT NULL,
    sample_policy_version VARCHAR(100) NOT NULL,
    request_contract_hash TEXT NOT NULL CHECK (request_contract_hash ~ '^[0-9a-f]{64}$'),
    effective_contract_hash TEXT CHECK (effective_contract_hash ~ '^[0-9a-f]{64}$'),
    contract_status TEXT NOT NULL DEFAULT 'unknown' CHECK (contract_status IN ('exact','mutated','unsupported','unknown')),
    tier TEXT NOT NULL CHECK (tier IN ('low','medium','high')),
    base_request_count BIGINT NOT NULL CHECK (base_request_count > 0 AND base_request_count <= 300),
    retry_budget BIGINT NOT NULL CHECK (retry_budget >= 0 AND retry_budget <= 5000),
    selection_mode TEXT CHECK (selection_mode IN ('random','fixed')),
    selection_snapshot JSONB NOT NULL CHECK (jsonb_typeof(selection_snapshot) = 'object'),
    state TEXT NOT NULL DEFAULT 'queued' CHECK (state IN ('queued','running','cancelling','completed','failed','cancelled','interrupted','skipped')),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    verdict TEXT NOT NULL DEFAULT 'not_evaluated' CHECK (verdict IN ('match','mismatch','insufficient','not_evaluated')),
    failure_code VARCHAR(100),
    valid_samples BIGINT NOT NULL DEFAULT 0 CHECK (valid_samples >= 0 AND valid_samples <= planned_samples),
    planned_samples BIGINT NOT NULL CHECK (planned_samples = base_request_count),
    FOREIGN KEY (job_id, source) REFERENCES monitor_jobs(id, source) ON DELETE RESTRICT,
    CHECK ((source = 'platform_group' AND group_id IS NOT NULL AND account_id IS NOT NULL AND credential_revision IS NOT NULL AND selection_mode IS NOT NULL AND site_api_key_id IS NULL)
        OR (source = 'external_api' AND group_id IS NULL AND account_id IS NULL AND site_api_key_id IS NULL AND selection_mode IS NULL)
        OR (source = 'site_api_key' AND account_id IS NULL AND site_api_key_id IS NOT NULL AND selection_mode IS NULL)),
    CHECK ((state IN ('completed','failed','cancelled','interrupted','skipped')) = (finished_at IS NOT NULL)),
    CHECK (verdict NOT IN ('match','mismatch') OR state = 'completed')
);
CREATE UNIQUE INDEX IF NOT EXISTS llm_detector_executions_platform_target_uq
    ON llm_detector_executions(job_id, target_index, account_id) WHERE source = 'platform_group';
CREATE UNIQUE INDEX IF NOT EXISTS llm_detector_executions_platform_model_uq
    ON llm_detector_executions(job_id, account_id, request_model) WHERE source = 'platform_group';
CREATE UNIQUE INDEX IF NOT EXISTS llm_detector_executions_private_target_uq
    ON llm_detector_executions(job_id, target_index) WHERE source IN ('external_api','site_api_key');
CREATE INDEX IF NOT EXISTS llm_detector_executions_group_model_idx ON llm_detector_executions(group_id, request_model, finished_at DESC);

CREATE TABLE IF NOT EXISTS llm_detector_attempts (
    id UUID PRIMARY KEY,
    execution_id UUID NOT NULL REFERENCES llm_detector_executions(id) ON DELETE RESTRICT,
    cell_id VARCHAR(200) NOT NULL,
    sample_index INTEGER NOT NULL CHECK (sample_index >= 0),
    attempt_index INTEGER NOT NULL CHECK (attempt_index >= 0),
    outbound_request_id UUID NOT NULL UNIQUE,
    lease_generation BIGINT NOT NULL CHECK (lease_generation > 0),
    dispatch_state TEXT NOT NULL DEFAULT 'reserved' CHECK (dispatch_state IN ('reserved','dispatching','completed','uncertain','cancelled_before_dispatch')),
    http_status INTEGER CHECK (http_status >= 100 AND http_status <= 599),
    error_category VARCHAR(100),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    normalized_answer TEXT CHECK (octet_length(normalized_answer) <= 8192),
    answer_category VARCHAR(100),
    usage_summary JSONB CHECK (jsonb_typeof(usage_summary) = 'object'),
    elapsed_ms BIGINT CHECK (elapsed_ms >= 0),
    effective_account_id BIGINT REFERENCES accounts(id) ON DELETE RESTRICT,
    contract_status TEXT NOT NULL DEFAULT 'unknown' CHECK (contract_status IN ('exact','mutated','unsupported','unknown')),
    UNIQUE (execution_id, cell_id, sample_index, attempt_index)
);

CREATE TABLE IF NOT EXISTS llm_detector_reports (
    id UUID PRIMARY KEY,
    execution_id UUID NOT NULL REFERENCES llm_detector_executions(id) ON DELETE RESTRICT,
    version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
    evidence JSONB NOT NULL CHECK (jsonb_typeof(evidence) = 'object' AND octet_length(evidence::text) <= 1048576),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,
    UNIQUE (execution_id, version)
);

CREATE TABLE IF NOT EXISTS channel_monitor_probe_results (
    id UUID PRIMARY KEY,
    job_id UUID NOT NULL REFERENCES monitor_jobs(id) ON DELETE RESTRICT,
    policy_id BIGINT NOT NULL REFERENCES channel_monitor_group_policies(id) ON DELETE RESTRICT,
    group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE RESTRICT,
    request_model VARCHAR(200) NOT NULL,
    account_id BIGINT REFERENCES accounts(id) ON DELETE RESTRICT,
    checked_at TIMESTAMPTZ NOT NULL,
    transport_state TEXT NOT NULL CHECK (transport_state IN ('passed','failed','incomplete','unsupported')),
    challenge_state TEXT NOT NULL CHECK (challenge_state IN ('passed','failed','not_evaluated')),
    ttft_ms BIGINT CHECK (ttft_ms >= 0),
    error_category VARCHAR(100),
    source TEXT NOT NULL DEFAULT 'availability_probe' CHECK (source = 'availability_probe')
);
CREATE INDEX IF NOT EXISTS channel_monitor_probe_results_group_idx ON channel_monitor_probe_results(group_id, request_model, checked_at DESC);
CREATE INDEX IF NOT EXISTS channel_monitor_probe_results_job_idx ON channel_monitor_probe_results(job_id);

CREATE TABLE IF NOT EXISTS monitor_budget_buckets (
    scope TEXT NOT NULL CHECK (scope IN ('global','user','group','credential')),
    scope_id BIGINT NOT NULL CHECK ((scope = 'global' AND scope_id = 0) OR (scope <> 'global' AND scope_id > 0)),
    utc_day DATE NOT NULL,
    request_limit BIGINT NOT NULL CHECK (request_limit >= 0 AND request_limit <= 1000000000),
    reserved BIGINT NOT NULL DEFAULT 0 CHECK (reserved >= 0),
    consumed BIGINT NOT NULL DEFAULT 0 CHECK (consumed >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (scope, scope_id, utc_day),
    CHECK (reserved <= request_limit - consumed)
);
CREATE TABLE IF NOT EXISTS monitor_budget_reservations (
    id UUID PRIMARY KEY,
    job_id UUID NOT NULL REFERENCES monitor_jobs(id) ON DELETE RESTRICT,
    scope TEXT NOT NULL,
    scope_id BIGINT NOT NULL,
    utc_day DATE NOT NULL,
    reserved BIGINT NOT NULL CHECK (reserved >= 0 AND reserved <= 5000),
    consumed BIGINT NOT NULL DEFAULT 0 CHECK (consumed >= 0),
    released BIGINT NOT NULL DEFAULT 0 CHECK (released >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    settled_at TIMESTAMPTZ,
    FOREIGN KEY (scope, scope_id, utc_day) REFERENCES monitor_budget_buckets(scope, scope_id, utc_day) ON DELETE RESTRICT,
    UNIQUE (job_id, scope, scope_id, utc_day),
    CHECK (consumed <= reserved AND released <= reserved - consumed),
    CHECK (settled_at IS NULL OR consumed + released = reserved)
);
