-- A lease never expires into permission to reuse a potentially consumed RT.
CREATE TABLE openai_refresh_grants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    epoch BIGINT NOT NULL DEFAULT 1,
    owner TEXT NOT NULL DEFAULT 'sub2api' CHECK (owner IN ('sub2api','draining','team')),
    attempt UUID,
    attempt_started_at TIMESTAMPTZ,
    uncertain BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE TABLE openai_refresh_tokens (
    token_hash TEXT PRIMARY KEY,
    grant_id UUID NOT NULL REFERENCES openai_refresh_grants(id),
    consumed BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE INDEX openai_refresh_tokens_grant_idx ON openai_refresh_tokens(grant_id);
CREATE TABLE openai_refresh_handoffs (
    operation_id TEXT PRIMARY KEY,
    scope TEXT NOT NULL,
    account_id BIGINT NOT NULL REFERENCES accounts(id),
    grant_id UUID NOT NULL REFERENCES openai_refresh_grants(id),
    expected_version BIGINT NOT NULL,
    expected_updated_at TIMESTAMPTZ NOT NULL,
    identity JSONB NOT NULL,
    released_version BIGINT,
    state TEXT NOT NULL DEFAULT 'draining' CHECK(state IN ('draining','ready','acknowledged')),
    epoch BIGINT NOT NULL,
    -- Same DB protection as accounts.credentials; never included in state/receipt queries.
    escrow JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE UNIQUE INDEX openai_refresh_handoffs_grant_idx ON openai_refresh_handoffs(grant_id);
CREATE TABLE openai_refresh_delegated_accounts (
    account_id BIGINT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    grant_id UUID NOT NULL REFERENCES openai_refresh_grants(id)
);
CREATE FUNCTION guard_delegated_openai_refresh_token() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF COALESCE(NEW.credentials->>'refresh_token','') <> '' AND EXISTS (
        SELECT 1 FROM openai_refresh_delegated_accounts WHERE account_id=NEW.id
    ) THEN
        RAISE EXCEPTION 'delegated account accepts access tokens only';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER guard_delegated_openai_refresh_token BEFORE UPDATE OF credentials ON accounts
FOR EACH ROW EXECUTE FUNCTION guard_delegated_openai_refresh_token();
