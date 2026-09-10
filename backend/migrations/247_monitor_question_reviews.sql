-- Human assessments are append-only evidence, never account scheduling state.
CREATE TABLE IF NOT EXISTS monitor_question_records (
 id UUID PRIMARY KEY,
 account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
 created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 request_model VARCHAR(200) NOT NULL CHECK (length(request_model)>0),
 prompt TEXT NOT NULL CHECK (octet_length(prompt) BETWEEN 1 AND 4096),
 answer TEXT NOT NULL CHECK (octet_length(answer)<=32768),
 transport_state TEXT NOT NULL CHECK (transport_state IN ('completed','failed','incomplete')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS monitor_question_records_account_idx ON monitor_question_records(account_id,created_at DESC,id);
CREATE TABLE IF NOT EXISTS monitor_question_reviews (
 id UUID PRIMARY KEY,
 record_id UUID NOT NULL REFERENCES monitor_question_records(id) ON DELETE RESTRICT,
 reviewed_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 verdict TEXT NOT NULL CHECK (verdict IN ('normal','degraded','unlabeled')),
 reason TEXT NOT NULL CHECK (octet_length(reason) BETWEEN 1 AND 2000),
 revision BIGINT NOT NULL CHECK (revision>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
 UNIQUE(record_id,revision)
);
