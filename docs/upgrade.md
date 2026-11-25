# Upgrading Adrient

## How schema changes work

- Migrations live in `internal/storage/migrations/NNNN_name.sql` and are
  embedded in the binary. On start the server applies every migration that is
  not yet in `adrient.schema_migrations`, each in its own transaction (apply
  and record together, or neither).
- Migrations are **forward-only**. There are no down migrations; to go back,
  restore a backup taken before the upgrade.
- Every migration is written to be idempotent (`CREATE ... IF NOT EXISTS`,
  `ADD COLUMN IF NOT EXISTS`, guarded inserts). CI applies all migrations to an
  empty database, runs the runner again, and then re-executes every file a
  second time, checking that the schema does not change.
- A Postgres advisory lock makes concurrent starts (several replicas, or
  `adrient-admin migrate up` racing the server) apply migrations once.
- Version numbers have gaps on purpose (ranges reserved per feature). They
  never get renumbered once released.

## Before upgrading

```sh
adrient-admin doctor          # config, database, storage, sign-in reachable
adrient-admin migrate status  # applied / pending migrations
adrient-admin backup -o pre-upgrade.tgz
```

`migrate status` exits with code 3 and a warning if the database has
migrations this binary does not know: the database is newer than the binary
(someone ran a newer release against it). Do not start an older release
against it.

## Upgrade

**install.sh:** `sudo ./install.sh update` (backs up the databases, rebuilds,
restarts).

**Helm:** `helm upgrade adrient deploy/helm/adrient -n adrient --reuse-values`
with the new chart/image.

**Manual:** start the new image. To apply migrations as a separate step (for
example in a CI job before rolling out), run the new binary's
`adrient-admin migrate up`. It runs the same pre-flight check and refuses if
the database is ahead of the binary.

## If something goes wrong

1. Stop the new version.
2. Restore the pre-upgrade backup with the **old** release's tools
   (`install.sh restore ...` or `adrient-admin restore pre-upgrade.tgz --replace`).
3. Start the old version and open an issue with the failing migration's error.
