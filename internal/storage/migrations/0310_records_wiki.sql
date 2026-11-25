-- 0310: Records (Airtable-style bases) and Wiki (spaces, pages, databases). Idempotent.

CREATE TABLE IF NOT EXISTS adrient.records_bases (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    color       TEXT NOT NULL DEFAULT '#126C5A',
    owner_id    UUID NOT NULL,
    org_access  TEXT NOT NULL DEFAULT 'none' CHECK (org_access IN ('none','view','edit')),
    source      TEXT NOT NULL DEFAULT '',     -- '' for normal bases, 'wiki' for databases inside wiki pages
    source_ref  TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    trashed_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS records_bases_org_idx ON adrient.records_bases (org_id, source) WHERE trashed_at IS NULL;

CREATE TABLE IF NOT EXISTS adrient.records_base_members (
    base_id UUID NOT NULL REFERENCES adrient.records_bases(id) ON DELETE CASCADE,
    org_id  UUID NOT NULL,
    user_id UUID NOT NULL,
    role    TEXT NOT NULL CHECK (role IN ('viewer','editor','admin')),
    PRIMARY KEY (base_id, user_id)
);
CREATE INDEX IF NOT EXISTS records_base_members_user_idx ON adrient.records_base_members (user_id);

CREATE TABLE IF NOT EXISTS adrient.records_tables (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL,
    base_id     UUID NOT NULL REFERENCES adrient.records_bases(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    position    INT NOT NULL DEFAULT 0,
    next_seq    BIGINT NOT NULL DEFAULT 1,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS records_tables_base_idx ON adrient.records_tables (base_id, position);

CREATE TABLE IF NOT EXISTS adrient.records_fields (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL,
    base_id     UUID NOT NULL REFERENCES adrient.records_bases(id) ON DELETE CASCADE,
    table_id    UUID NOT NULL REFERENCES adrient.records_tables(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    type        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    options     JSONB NOT NULL DEFAULT '{}',
    position    INT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS records_fields_name_idx ON adrient.records_fields (table_id, lower(name));
CREATE INDEX IF NOT EXISTS records_fields_base_idx ON adrient.records_fields (base_id);

CREATE TABLE IF NOT EXISTS adrient.records_records (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL,
    table_id   UUID NOT NULL REFERENCES adrient.records_tables(id) ON DELETE CASCADE,
    seq        BIGINT NOT NULL,
    data       JSONB NOT NULL DEFAULT '{}',
    created_by UUID,
    updated_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS records_records_table_idx ON adrient.records_records (table_id, seq);

CREATE TABLE IF NOT EXISTS adrient.records_links (
    org_id   UUID NOT NULL,
    field_id UUID NOT NULL REFERENCES adrient.records_fields(id) ON DELETE CASCADE,
    src      UUID NOT NULL REFERENCES adrient.records_records(id) ON DELETE CASCADE,
    dst      UUID NOT NULL REFERENCES adrient.records_records(id) ON DELETE CASCADE,
    pos      INT NOT NULL DEFAULT 0,
    PRIMARY KEY (field_id, src, dst)
);
CREATE INDEX IF NOT EXISTS records_links_dst_idx ON adrient.records_links (field_id, dst);
CREATE INDEX IF NOT EXISTS records_links_src_idx ON adrient.records_links (src);

CREATE TABLE IF NOT EXISTS adrient.records_views (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL,
    table_id   UUID NOT NULL REFERENCES adrient.records_tables(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    type       TEXT NOT NULL CHECK (type IN ('grid','kanban','gallery','calendar','list','form')),
    config     JSONB NOT NULL DEFAULT '{}',
    position   INT NOT NULL DEFAULT 0,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS records_views_table_idx ON adrient.records_views (table_id, position);

CREATE TABLE IF NOT EXISTS adrient.records_comments (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL,
    record_id  UUID NOT NULL REFERENCES adrient.records_records(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL,
    body       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS records_comments_rec_idx ON adrient.records_comments (record_id, created_at);

CREATE TABLE IF NOT EXISTS adrient.records_history (
    id        BIGSERIAL PRIMARY KEY,
    org_id    UUID NOT NULL,
    record_id UUID NOT NULL REFERENCES adrient.records_records(id) ON DELETE CASCADE,
    user_id   UUID,
    at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    changes   JSONB NOT NULL
);
CREATE INDEX IF NOT EXISTS records_history_rec_idx ON adrient.records_history (record_id, at DESC);

-- Natural ordering ("item2" before "item10") for text sorts; optional, Records falls back to the default collation.
DO $$ BEGIN
    CREATE COLLATION IF NOT EXISTS adrient.records_natural (provider = icu, locale = 'und-u-kn-true', deterministic = true);
EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'ICU collation not available: %', SQLERRM;
END $$;

-- Wiki ---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS adrient.wiki_spaces (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    icon       TEXT NOT NULL DEFAULT '',
    kind       TEXT NOT NULL DEFAULT 'team' CHECK (kind IN ('team','personal')),
    owner_id   UUID NOT NULL,
    org_access TEXT NOT NULL DEFAULT 'none' CHECK (org_access IN ('none','view','edit')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS wiki_spaces_org_idx ON adrient.wiki_spaces (org_id);

CREATE TABLE IF NOT EXISTS adrient.wiki_space_members (
    space_id UUID NOT NULL REFERENCES adrient.wiki_spaces(id) ON DELETE CASCADE,
    org_id   UUID NOT NULL,
    user_id  UUID NOT NULL,
    role     TEXT NOT NULL CHECK (role IN ('viewer','editor','admin')),
    PRIMARY KEY (space_id, user_id)
);

CREATE TABLE IF NOT EXISTS adrient.wiki_pages (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id           UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    space_id         UUID NOT NULL REFERENCES adrient.wiki_spaces(id) ON DELETE CASCADE,
    parent_id        UUID REFERENCES adrient.wiki_pages(id) ON DELETE CASCADE,
    title            TEXT NOT NULL DEFAULT '',
    icon             TEXT NOT NULL DEFAULT '',
    kind             TEXT NOT NULL DEFAULT 'page' CHECK (kind IN ('page','database')),
    content          JSONB NOT NULL DEFAULT '{"type":"doc","content":[{"type":"paragraph"}]}',
    text_content     TEXT NOT NULL DEFAULT '',
    records_base_id  UUID,
    records_table_id UUID,
    position         DOUBLE PRECISION NOT NULL DEFAULT 0,
    version          INT NOT NULL DEFAULT 1,
    created_by       UUID NOT NULL,
    updated_by       UUID NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    trashed_at       TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS wiki_pages_space_idx ON adrient.wiki_pages (space_id, parent_id, position) WHERE trashed_at IS NULL;
CREATE INDEX IF NOT EXISTS wiki_pages_search_idx ON adrient.wiki_pages USING GIN (to_tsvector('simple', title || ' ' || text_content));

CREATE TABLE IF NOT EXISTS adrient.wiki_page_versions (
    id       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id   UUID NOT NULL,
    page_id  UUID NOT NULL REFERENCES adrient.wiki_pages(id) ON DELETE CASCADE,
    title    TEXT NOT NULL,
    content  JSONB NOT NULL,
    saved_by UUID,
    saved_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS wiki_page_versions_idx ON adrient.wiki_page_versions (page_id, saved_at DESC);

CREATE TABLE IF NOT EXISTS adrient.wiki_favorites (
    user_id UUID NOT NULL,
    page_id UUID NOT NULL REFERENCES adrient.wiki_pages(id) ON DELETE CASCADE,
    org_id  UUID NOT NULL,
    PRIMARY KEY (user_id, page_id)
);

CREATE TABLE IF NOT EXISTS adrient.wiki_page_links (
    org_id         UUID NOT NULL,
    page_id        UUID NOT NULL REFERENCES adrient.wiki_pages(id) ON DELETE CASCADE,
    target_page_id UUID NOT NULL REFERENCES adrient.wiki_pages(id) ON DELETE CASCADE,
    PRIMARY KEY (page_id, target_page_id)
);
CREATE INDEX IF NOT EXISTS wiki_page_links_target_idx ON adrient.wiki_page_links (target_page_id);

CREATE TABLE IF NOT EXISTS adrient.wiki_comments (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL,
    page_id    UUID NOT NULL REFERENCES adrient.wiki_pages(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL,
    body       TEXT NOT NULL,
    resolved   BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS wiki_comments_page_idx ON adrient.wiki_comments (page_id, created_at);
