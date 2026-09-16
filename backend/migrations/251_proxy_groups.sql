-- Proxy groups describe egress pools, independently of account routing groups.
CREATE TABLE proxy_groups (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE CHECK (length(btrim(name)) > 0),
    max_accounts_per_proxy INTEGER NOT NULL DEFAULT 2 CHECK (max_accounts_per_proxy BETWEEN 1 AND 10000)
);
CREATE TABLE proxy_group_members (
    proxy_id BIGINT PRIMARY KEY REFERENCES proxies(id) ON DELETE CASCADE,
    group_id BIGINT NOT NULL REFERENCES proxy_groups(id) ON DELETE CASCADE
);
CREATE INDEX proxy_group_members_group_id_idx ON proxy_group_members(group_id);

-- Serialize allocation and membership edits across application instances.
-- Shadows share their parent's credential/egress and do not consume a slot.
CREATE FUNCTION enforce_proxy_group_capacity() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE capacity INTEGER;
BEGIN
    IF NEW.proxy_id IS NULL OR NEW.deleted_at IS NOT NULL OR NEW.parent_account_id IS NOT NULL THEN
        RETURN NEW;
    END IF;
    IF TG_OP = 'UPDATE' THEN
        IF NEW.proxy_id IS NOT DISTINCT FROM OLD.proxy_id
            AND OLD.deleted_at IS NULL AND OLD.parent_account_id IS NULL THEN
            RETURN NEW;
        END IF;
    END IF;
    PERFORM pg_advisory_xact_lock(731204, 1);
    SELECT g.max_accounts_per_proxy INTO capacity
    FROM proxy_groups g JOIN proxy_group_members m ON m.group_id = g.id
    WHERE m.proxy_id = NEW.proxy_id;
    IF capacity IS NOT NULL AND (
        SELECT count(*) FROM accounts a
        WHERE a.proxy_id = NEW.proxy_id AND a.deleted_at IS NULL
            AND a.parent_account_id IS NULL AND a.id <> NEW.id
    ) >= capacity THEN
        RAISE EXCEPTION 'proxy group capacity reached' USING ERRCODE = '23514', CONSTRAINT = 'proxy_group_capacity';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER accounts_proxy_group_capacity
    BEFORE INSERT OR UPDATE OF proxy_id, deleted_at, parent_account_id ON accounts
    FOR EACH ROW EXECUTE FUNCTION enforce_proxy_group_capacity();
