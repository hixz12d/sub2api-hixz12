-- Add an opt-in OpenAI scheduler mode that resolves account priority from the
-- current account_groups binding. Existing groups remain on the global account
-- priority behavior until an administrator explicitly enables binding mode.

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS openai_account_priority_mode VARCHAR(20) NOT NULL DEFAULT 'global';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'groups_openai_account_priority_mode_check'
          AND conrelid = 'groups'::regclass
    ) THEN
        ALTER TABLE groups
            ADD CONSTRAINT groups_openai_account_priority_mode_check
            CHECK (openai_account_priority_mode IN ('global', 'binding'));
    END IF;
END
$$;

COMMENT ON COLUMN groups.openai_account_priority_mode IS
    'OpenAI scheduling priority source: global=accounts.priority, binding=account_groups.priority for the active group';

-- Every persisted group field can influence an API-key auth snapshot now or in a
-- later release. Invalidate on every UPDATE/DELETE so direct SQL and crash windows
-- cannot leave stale L1/L2 authorization or routing capabilities. This body
-- supersedes migration 193.
CREATE OR REPLACE FUNCTION enqueue_group_auth_cache_invalidation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    target_group_id BIGINT;
BEGIN
    target_group_id := OLD.id;

    INSERT INTO auth_cache_invalidation_outbox (cache_key)
    SELECT encode(sha256(convert_to(k.key, 'UTF8')), 'hex')
    FROM api_keys AS k
    WHERE k.group_id = target_group_id
      AND k.deleted_at IS NULL
      AND k.key <> '';
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;
