-- Reset cards (consumption rebate cards): admin-issued batches, one card per user,
-- the time-limited credit created when a card is used, and a per (user, group)
-- watermark so the same usage is never rebated twice.
-- Only new tables and indexes; existing tables are untouched so older binaries keep working.

CREATE TABLE IF NOT EXISTS reset_card_batches (
    id                BIGSERIAL PRIMARY KEY,
    name              VARCHAR(100) NOT NULL,
    group_ids         BIGINT[] NOT NULL,
    percent           NUMERIC(5,2) NOT NULL CHECK (percent > 0 AND percent <= 100),
    max_amount        NUMERIC(20,8) NOT NULL CHECK (max_amount > 0),
    card_valid_days   INTEGER NOT NULL DEFAULT 14,
    credit_valid_days INTEGER NOT NULL DEFAULT 7,
    target_mode       VARCHAR(16) NOT NULL CHECK (target_mode IN ('all', 'users')),
    only_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_by        BIGINT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_reset_card_batches_created_at ON reset_card_batches(created_at DESC);

CREATE TABLE IF NOT EXISTS reset_cards (
    id            BIGSERIAL PRIMARY KEY,
    batch_id      BIGINT NOT NULL REFERENCES reset_card_batches(id),
    user_id       BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status        VARCHAR(16) NOT NULL DEFAULT 'unused' CHECK (status IN ('unused', 'used', 'revoked')),
    expires_at    TIMESTAMPTZ NOT NULL,
    used_at       TIMESTAMPTZ,
    revoked_at    TIMESTAMPTZ,
    popup_seen_at TIMESTAMPTZ,
    refund_amount NUMERIC(20,8),
    base_spend    NUMERIC(20,8),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_reset_cards_batch_user UNIQUE (batch_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_reset_cards_user_status ON reset_cards(user_id, status);

CREATE TABLE IF NOT EXISTS reset_card_credits (
    id              BIGSERIAL PRIMARY KEY,
    card_id         BIGINT NOT NULL UNIQUE REFERENCES reset_cards(id) ON DELETE CASCADE,
    user_id         BIGINT NOT NULL,
    group_ids       BIGINT[] NOT NULL,
    amount          NUMERIC(20,8) NOT NULL,
    starts_at       TIMESTAMPTZ NOT NULL,
    expires_at      TIMESTAMPTZ NOT NULL,
    status          VARCHAR(16) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'settled')),
    used_amount     NUMERIC(20,8),
    clawback_amount NUMERIC(20,8),
    settled_at      TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_reset_card_credits_status_expires ON reset_card_credits(status, expires_at);
CREATE INDEX IF NOT EXISTS idx_reset_card_credits_user_status ON reset_card_credits(user_id, status);

CREATE TABLE IF NOT EXISTS reset_card_watermarks (
    user_id       BIGINT NOT NULL,
    group_id      BIGINT NOT NULL,
    rebated_until TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (user_id, group_id)
);
