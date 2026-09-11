-- Repair a missing admission column without renaming, dropping, or importing
-- the legacy display-only column. Keep an explicitly configured allowlist.
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS model_allowlist JSONB NOT NULL DEFAULT '{}'::jsonb;

UPDATE groups SET model_allowlist = '{}'::jsonb WHERE model_allowlist IS NULL;

ALTER TABLE groups ALTER COLUMN model_allowlist SET DEFAULT '{}'::jsonb;
ALTER TABLE groups ALTER COLUMN model_allowlist SET NOT NULL;

COMMENT ON COLUMN groups.model_allowlist IS
    'Opt-in group model allowlist: constrains model listing and request admission independently of models_list_config';
