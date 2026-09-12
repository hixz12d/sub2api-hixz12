-- Versioned authentication evidence is separate from editable account metadata.
CREATE TABLE oauth_auth_errors (
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('error', 'temporary')),
    credential_version BIGINT NOT NULL CHECK (credential_version > 0),
    token_hash TEXT NOT NULL,
    message TEXT NOT NULL,
    blocked_until TIMESTAMPTZ,
    observed_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (account_id, kind)
);
ALTER TABLE oauth_sync_operations ADD COLUMN auth_recovery TEXT NOT NULL DEFAULT 'skipped';
ALTER TABLE oauth_sync_operations ADD COLUMN validation_scope TEXT NOT NULL DEFAULT '';
ALTER TABLE oauth_sync_operations ADD COLUMN validated_at TIMESTAMPTZ;
