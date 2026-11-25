-- Initial schema for adrient-workspace.
--
-- We isolate all our tables under a `adrient` schema (not `public`) so a shared
-- Postgres instance can host multiple apps without clashing.

CREATE SCHEMA IF NOT EXISTS adrient;

CREATE TABLE IF NOT EXISTS adrient.schema_migrations (
    version     INTEGER PRIMARY KEY,
    applied_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
