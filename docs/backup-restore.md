# Backup and restore

Adrient keeps state in three places: **Postgres** (the `adrient` schema, plus
the `zitadel` and `pdf` databases), **object storage** (Drive, Photos, Mail
attachments, recordings; RustFS or any S3-compatible store) and the **config**
(environment file with secrets).

There are two tools. Use the one that matches how you installed.

| | `install.sh backup` | `adrient-admin backup` |
|---|---|---|
| For | single-server installs made with `install.sh` | any install (Helm, external Postgres/S3, install.sh) |
| Covers | all databases (`adrient`, `zitadel`, `pdf`, CRM), RustFS data directory, Zitadel data, `.env` | the `adrient` schema, every object in the Adrient bucket, optionally the env file |
| Format | `pg_dump -Fc` per database + tar of data dirs | portable tar.gz: `manifest.json`, one CSV per table, objects with SHA-256 checksums |
| Verifies | no | yes: restore compares every table's row count and every object's checksum |
| Sign-in (Zitadel) | included | **not** included; back Zitadel up separately (its own database) |

## install.sh (single server)

```sh
sudo ./install.sh backup              # databases + files -> /opt/adrient-workspace/backups/
sudo ./install.sh backup --db-only    # databases only
sudo ./install.sh restore /opt/adrient-workspace/backups/adrient-YYYYMMDD-HHMMSS.tar.gz
```

The archive contains your `.env` (secrets); store it off the server and
encrypted. `install.sh update` takes a database backup before upgrading.

## adrient-admin

`adrient-admin` ships in the image next to the server and reads the same
environment (`ADRIENT_POSTGRES_DSN`, `ADRIENT_RUSTFS_*`).

```sh
# inside the app container (install.sh / compose)
docker compose exec adrient /app/adrient-admin backup -o /tmp/adrient.tgz
docker compose cp adrient:/tmp/adrient.tgz ./adrient-$(date +%F).tgz

# Kubernetes
kubectl -n adrient exec deploy/adrient -- /app/adrient-admin backup -o /tmp/adrient.tgz
kubectl -n adrient cp adrient/<pod>:/tmp/adrient.tgz ./adrient.tgz
```

Options:

- `--config /path/to/.env` includes the env file (contains secrets).
- `--no-objects` skips object storage (database only).

The database part is one consistent snapshot (a single `REPEATABLE READ`
transaction), so it can run while Adrient is in use. Objects are read after
the snapshot; objects uploaded during the backup may or may not be included.

### Restore

```sh
adrient-admin restore adrient.tgz --replace
```

- Restore **replaces** all Adrient data in the target database; it never merges.
  `--replace` is required to confirm.
- The target can be an empty database: pending migrations are applied first.
- A backup made by a newer release (schema versions this binary does not know),
  or containing tables that do not exist after migrating, is refused instead
  of restored partially. Restore with the release that made the backup, then
  upgrade.
- Objects are checked against their SHA-256 before upload.
- At the end every table's row count is compared with the manifest; any
  difference is printed and the command exits non-zero.
- The config file inside the archive is **not** applied; extract it by hand if
  you need it.
- The database role must be allowed to set `session_replication_role`
  (the `adrient` superuser in the shipped compose files is), because rows are
  loaded in file order with constraints and triggers deferred.

Stop the Adrient server during a restore so nobody writes in between.

### Tested

`internal/adminops/roundtrip_test.go` backs up a seeded database plus objects,
restores into a brand-new database, compares every table's row count, the
object bytes and a value full of quotes and commas, and restores a second time
over existing data. CI runs it on every push. A backup of the 254-table shared
development database was also taken with the CLI (1,943 rows, 69 objects);
its restore was correctly refused because that database contained migrations
from other branches.

## Export everything for one organisation (data portability)

```sh
adrient-admin export-org <org-slug-or-id> -o acme-export.tgz
```

The archive holds `data/<table>.ndjson` (one JSON object per row for every
table that belongs to the organisation, found through `org_id` or foreign
keys), `files/` (Drive files with their folder structure, trash excluded),
`manifest.json` (row counts, what was left out) and a `README.txt`. Sessions,
API tokens and any column whose name looks like a password, secret, token hash
or private key are left out. Tables that cannot be tied to one organisation
are listed under `not_exported`.
