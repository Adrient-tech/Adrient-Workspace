-- 0140: External collaboration policy (plan C): guest invite policy, partner
-- domain allow/block lists, guest directory visibility.

CREATE TABLE IF NOT EXISTS adrient.external_policy (
    org_id                          UUID PRIMARY KEY REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    guest_invites                   TEXT NOT NULL DEFAULT 'admins_and_sponsors'
        CHECK (guest_invites IN ('none','admins','admins_and_sponsors','members','everyone')),
    domain_mode                     TEXT NOT NULL DEFAULT 'allow_all'
        CHECK (domain_mode IN ('allow_all','allowlist','blocklist')),
    guest_visibility                TEXT NOT NULL DEFAULT 'limited' CHECK (guest_visibility IN ('limited','directory')),
    guest_default_expiry_days       INT NOT NULL DEFAULT 30,
    external_member_default_expiry_days INT NOT NULL DEFAULT 90,
    require_mfa_for_guests          BOOLEAN NOT NULL DEFAULT true,
    updated_at                      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS adrient.partner_domains (
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    domain     TEXT NOT NULL,
    effect     TEXT NOT NULL CHECK (effect IN ('allow','block')),
    note       TEXT NOT NULL DEFAULT '',
    overrides  JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by UUID REFERENCES adrient.users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, domain)
);
