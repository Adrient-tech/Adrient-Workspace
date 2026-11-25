-- 0003: Bootstrap the "default" org for single-org-mode deployments.
--
-- In single-org mode, the backend reads ADRIENT_DEFAULT_ORG_SLUG (which
-- defaults to "default") and resolves all requests to this org.
-- Multi-org-mode deployments will add additional orgs via the admin API.

INSERT INTO adrient.orgs (slug, display_name)
VALUES ('default', 'Default')
ON CONFLICT (slug) DO NOTHING;
