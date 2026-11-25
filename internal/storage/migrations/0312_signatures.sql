-- 0312: Signatures (e-signature requests with a tamper-evident audit trail). Idempotent.

CREATE TABLE IF NOT EXISTS adrient.sig_requests (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    title         TEXT NOT NULL,
    message       TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','sent','completed','declined','voided','expired')),
    sender_id     UUID NOT NULL,
    file_id       UUID NOT NULL,
    file_name     TEXT NOT NULL DEFAULT '',
    doc_hash      TEXT NOT NULL DEFAULT '',
    signed_file_id UUID,
    signed_hash   TEXT NOT NULL DEFAULT '',
    sequential    BOOLEAN NOT NULL DEFAULT false,
    expires_at    TIMESTAMPTZ,
    contract_id   UUID,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at       TIMESTAMPTZ,
    completed_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS sig_requests_org_idx ON adrient.sig_requests (org_id, status, created_at DESC);

CREATE TABLE IF NOT EXISTS adrient.sig_signers (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID NOT NULL,
    request_id     UUID NOT NULL REFERENCES adrient.sig_requests(id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    email          TEXT NOT NULL,
    user_id        UUID,
    contact_id     UUID,
    role           TEXT NOT NULL DEFAULT 'signer' CHECK (role IN ('signer','cc')),
    order_idx      INT NOT NULL DEFAULT 0,
    token_hash     TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','viewed','signed','declined')),
    viewed_at      TIMESTAMPTZ,
    signed_at      TIMESTAMPTZ,
    declined_at    TIMESTAMPTZ,
    decline_reason TEXT NOT NULL DEFAULT '',
    signature_kind TEXT NOT NULL DEFAULT '',
    signature_data TEXT NOT NULL DEFAULT '',
    sign_ip        TEXT NOT NULL DEFAULT '',
    sign_ua        TEXT NOT NULL DEFAULT '',
    consent_at     TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS sig_signers_req_idx ON adrient.sig_signers (request_id, order_idx);
CREATE UNIQUE INDEX IF NOT EXISTS sig_signers_token_idx ON adrient.sig_signers (token_hash) WHERE token_hash <> '';

CREATE TABLE IF NOT EXISTS adrient.sig_fields (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL,
    request_id UUID NOT NULL REFERENCES adrient.sig_requests(id) ON DELETE CASCADE,
    signer_id  UUID NOT NULL REFERENCES adrient.sig_signers(id) ON DELETE CASCADE,
    type       TEXT NOT NULL CHECK (type IN ('signature','initials','date','text','checkbox')),
    label      TEXT NOT NULL DEFAULT '',
    page       INT NOT NULL DEFAULT 1,
    x          DOUBLE PRECISION NOT NULL,
    y          DOUBLE PRECISION NOT NULL,
    w          DOUBLE PRECISION NOT NULL,
    h          DOUBLE PRECISION NOT NULL,
    required   BOOLEAN NOT NULL DEFAULT true,
    value      TEXT NOT NULL DEFAULT '',
    filled_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS sig_fields_req_idx ON adrient.sig_fields (request_id);

-- Append-only audit trail; each entry carries the hash of the previous one, so edits are detectable.
CREATE TABLE IF NOT EXISTS adrient.sig_events (
    id         BIGSERIAL PRIMARY KEY,
    org_id     UUID NOT NULL,
    request_id UUID NOT NULL REFERENCES adrient.sig_requests(id) ON DELETE CASCADE,
    signer_id  UUID,
    event      TEXT NOT NULL,
    actor      TEXT NOT NULL DEFAULT '',
    ip         TEXT NOT NULL DEFAULT '',
    ua         TEXT NOT NULL DEFAULT '',
    detail     TEXT NOT NULL DEFAULT '',
    at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    prev_hash  TEXT NOT NULL DEFAULT '',
    hash       TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS sig_events_req_idx ON adrient.sig_events (request_id, id);

-- The exact PDF that was sent. Signers and the final stamping read this copy, so a later edit of the Drive file cannot change what they sign.
CREATE TABLE IF NOT EXISTS adrient.sig_documents (
    request_id UUID PRIMARY KEY REFERENCES adrient.sig_requests(id) ON DELETE CASCADE,
    org_id     UUID NOT NULL,
    data       BYTEA NOT NULL
);
