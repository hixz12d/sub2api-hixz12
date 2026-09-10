-- Shared release authority. Additive only; no release is admitted by migration.
CREATE TABLE monitor_benchmark_packages (
    id BIGSERIAL PRIMARY KEY,
    benchmark_id VARCHAR(200) NOT NULL,
    benchmark_version VARCHAR(100) NOT NULL,
    sha256 TEXT NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    content_sha256 TEXT NOT NULL CHECK (content_sha256 ~ '^[0-9a-f]{64}$'),
    mode TEXT NOT NULL CHECK (mode IN ('gpt','claude','chat')),
    payload BYTEA NOT NULL CHECK (octet_length(payload) BETWEEN 1 AND 33554432),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    UNIQUE(benchmark_id,benchmark_version)
);
CREATE TABLE monitor_benchmark_releases (
    id UUID PRIMARY KEY,
    package_id BIGINT NOT NULL REFERENCES monitor_benchmark_packages(id),
    engine_lock_sha256 TEXT NOT NULL CHECK (engine_lock_sha256 ~ '^[0-9a-f]{64}$'),
    engine_lock BYTEA NOT NULL CHECK (octet_length(engine_lock) BETWEEN 2 AND 65536),
    state TEXT NOT NULL DEFAULT 'candidate' CHECK(state IN ('candidate','approved','withdrawn')),
    validation_receipt JSONB CHECK (jsonb_typeof(validation_receipt)='object' AND octet_length(validation_receipt::text)<=65536),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    UNIQUE(package_id,engine_lock_sha256),
    CHECK(state<>'approved' OR validation_receipt IS NOT NULL)
);
CREATE TABLE monitor_benchmark_channels (
    name VARCHAR(200) PRIMARY KEY,
    release_id UUID NOT NULL REFERENCES monitor_benchmark_releases(id),
    revision BIGINT NOT NULL CHECK(revision>0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE monitor_benchmark_audit (
    id BIGSERIAL PRIMARY KEY,
    actor_id BIGINT NOT NULL REFERENCES users(id),
    operation TEXT NOT NULL CHECK(operation IN ('stage','approve','activate','withdraw')),
    release_id UUID NOT NULL REFERENCES monitor_benchmark_releases(id),
    channel VARCHAR(200),
    revision BIGINT,
    reason VARCHAR(500),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX monitor_benchmark_audit_release_idx ON monitor_benchmark_audit(release_id,id);
