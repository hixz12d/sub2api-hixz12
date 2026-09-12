-- OAuth sync receipts contain metadata only. Tokens never enter this table.
CREATE TABLE integration_identity (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
    instance_id UUID NOT NULL UNIQUE DEFAULT gen_random_uuid()
);
INSERT INTO integration_identity (singleton) VALUES (TRUE);

CREATE TABLE oauth_sync_operations (
    id BIGSERIAL PRIMARY KEY,
    scope VARCHAR(160) NOT NULL,
    operation_id VARCHAR(128) NOT NULL,
    request_hash CHAR(64) NOT NULL,
    account_id BIGINT NOT NULL,
    credential_version BIGINT NOT NULL,
    cache_done BOOLEAN NOT NULL DEFAULT FALSE,
    scheduler_done BOOLEAN NOT NULL DEFAULT FALSE,
    state VARCHAR(24) NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'completed', 'needs_review')),
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    lease_id UUID,
    lease_until TIMESTAMPTZ,
    last_error VARCHAR(64) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    UNIQUE (scope, operation_id),
    CHECK (state <> 'completed' OR (cache_done AND scheduler_done))
);
-- Deliberately no cascading account FK: deletion must not erase write evidence.
CREATE INDEX oauth_sync_operations_pending ON oauth_sync_operations (next_attempt_at, id) WHERE state = 'pending';
CREATE INDEX oauth_sync_operations_account ON oauth_sync_operations (account_id, id DESC);
