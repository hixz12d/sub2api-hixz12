-- Keep the legacy listing column for old binaries and existing API clients.
-- Request admission is opt-in and must not inherit a display-only configuration.
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS model_allowlist JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN groups.model_allowlist IS
    'Opt-in group model allowlist: constrains model listing and request admission independently of models_list_config';
