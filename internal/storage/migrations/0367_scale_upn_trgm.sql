-- 0367: substring search on UPN (directory search matches name, email, employee id, UPN).
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_trgm') THEN
        EXECUTE 'CREATE INDEX IF NOT EXISTS users_upn_trgm_idx ON adrient.users USING gin (upn gin_trgm_ops)';
    END IF;
END
$$;
