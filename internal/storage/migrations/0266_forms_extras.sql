-- Forms extras: per-form theme, owner notification on new responses, send log.
-- Idempotent.
CREATE TABLE IF NOT EXISTS adrient.form_extras (
    form_id      UUID PRIMARY KEY REFERENCES adrient.forms(id) ON DELETE CASCADE,
    org_id       UUID NOT NULL,
    theme_color  TEXT NOT NULL DEFAULT '',
    theme_bg     TEXT NOT NULL DEFAULT '',
    notify_owner BOOLEAN NOT NULL DEFAULT false,
    notified_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS adrient.form_sends (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    form_id    UUID NOT NULL REFERENCES adrient.forms(id) ON DELETE CASCADE,
    org_id     UUID NOT NULL,
    sent_by    UUID,
    recipients INT  NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
