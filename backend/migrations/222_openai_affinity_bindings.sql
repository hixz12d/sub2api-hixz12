-- Phase 3: durable OpenAI affinity ownership.
-- Additive only. Sensitive client/session/response identifiers are stored as
-- domain-separated HMAC-SHA256 hex digests, never as raw values.

CREATE TABLE IF NOT EXISTS gateway_session_bindings (
    id BIGSERIAL PRIMARY KEY,
    owner_scope_hash VARCHAR(64) NOT NULL,
    provider VARCHAR(32) NOT NULL,
    namespace_hash VARCHAR(64) NOT NULL,
    primary_hash VARCHAR(64) NOT NULL,
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    strength SMALLINT NOT NULL CHECK (strength BETWEEN 0 AND 2),
    source VARCHAR(64) NOT NULL,
    stateful BOOLEAN NOT NULL DEFAULT FALSE,
    capability VARCHAR(64) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_hit_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    version BIGINT NOT NULL DEFAULT 1,
    CONSTRAINT gateway_session_bindings_identity_unique
        UNIQUE (owner_scope_hash, provider, namespace_hash, primary_hash)
);

CREATE TABLE IF NOT EXISTS gateway_session_binding_aliases (
    id BIGSERIAL PRIMARY KEY,
    binding_id BIGINT NOT NULL REFERENCES gateway_session_bindings(id) ON DELETE CASCADE,
    owner_scope_hash VARCHAR(64) NOT NULL,
    provider VARCHAR(32) NOT NULL,
    namespace_hash VARCHAR(64) NOT NULL,
    alias_hash VARCHAR(64) NOT NULL,
    source VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT gateway_session_binding_alias_pair_unique UNIQUE (binding_id, alias_hash),
    CONSTRAINT gateway_session_binding_alias_identity_unique
        UNIQUE (owner_scope_hash, provider, namespace_hash, alias_hash)
);

CREATE TABLE IF NOT EXISTS gateway_response_bindings (
    id BIGSERIAL PRIMARY KEY,
    owner_scope_hash VARCHAR(64) NOT NULL,
    provider VARCHAR(32) NOT NULL,
    response_key_hash VARCHAR(64) NOT NULL,
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    session_binding_id BIGINT NULL REFERENCES gateway_session_bindings(id) ON DELETE SET NULL,
    capability VARCHAR(64) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_hit_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    version BIGINT NOT NULL DEFAULT 1,
    CONSTRAINT gateway_response_bindings_identity_unique
        UNIQUE (owner_scope_hash, provider, response_key_hash)
);

CREATE INDEX IF NOT EXISTS idx_gateway_session_bindings_account
    ON gateway_session_bindings (account_id);
CREATE INDEX IF NOT EXISTS idx_gateway_session_bindings_expiry
    ON gateway_session_bindings (expires_at);
CREATE INDEX IF NOT EXISTS idx_gateway_session_aliases_binding
    ON gateway_session_binding_aliases (binding_id);
CREATE INDEX IF NOT EXISTS idx_gateway_response_bindings_account
    ON gateway_response_bindings (account_id);
CREATE INDEX IF NOT EXISTS idx_gateway_response_bindings_expiry
    ON gateway_response_bindings (expires_at);
CREATE INDEX IF NOT EXISTS idx_gateway_response_bindings_session
    ON gateway_response_bindings (session_binding_id) WHERE session_binding_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS gateway_affinity_migration_audit (
    id BIGSERIAL PRIMARY KEY,
    binding_id BIGINT NOT NULL REFERENCES gateway_session_bindings(id) ON DELETE RESTRICT,
    from_account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    to_account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    expected_version BIGINT NOT NULL,
    resulting_version BIGINT NOT NULL,
    reason VARCHAR(256) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_gateway_affinity_migration_audit_binding
    ON gateway_affinity_migration_audit (binding_id, created_at DESC);
