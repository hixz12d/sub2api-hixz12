-- Separate short availability checks from capability evaluation at group scope.
-- Global/credential ceilings remain shared; existing capability reservations are unchanged.
ALTER TABLE monitor_budget_buckets DROP CONSTRAINT IF EXISTS monitor_budget_buckets_scope_check;
ALTER TABLE monitor_budget_buckets ADD CONSTRAINT monitor_budget_buckets_scope_check
 CHECK (scope IN ('global','user','group','credential','probe_group'));

-- One availability sample permits one physical request. Retries need a new job
-- and reservation; an ambiguous dispatch must never be transparently replayed.
CREATE TABLE IF NOT EXISTS monitor_probe_dispatches (
 id UUID PRIMARY KEY,
 job_id UUID NOT NULL REFERENCES monitor_jobs(id) ON DELETE RESTRICT,
 target_index INTEGER NOT NULL CHECK (target_index >= 0 AND target_index < 20),
 sample_index INTEGER NOT NULL CHECK (sample_index >= 0 AND sample_index < 3),
 account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
 request_model VARCHAR(200) NOT NULL CHECK (length(request_model)>0),
 request_sha256 TEXT NOT NULL CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
 lease_generation BIGINT NOT NULL CHECK (lease_generation > 0),
 dispatch_ordinal BIGINT NOT NULL CHECK (dispatch_ordinal >= 0 AND dispatch_ordinal < 5000),
 state TEXT NOT NULL DEFAULT 'dispatching' CHECK (state IN ('dispatching','completed','uncertain')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 completed_at TIMESTAMPTZ,
 UNIQUE(job_id,target_index,sample_index),
 UNIQUE(job_id,dispatch_ordinal),
 CHECK ((state <> 'dispatching') = (completed_at IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS monitor_probe_dispatches_account_idx ON monitor_probe_dispatches(account_id,created_at DESC);
