# Contributing to Adrient Workspace

**Contributions and issues are very welcome** — thank you for helping make Adrient
better. Bug reports, feature ideas, new games, docs fixes, and pull requests are
all appreciated.

## Where things live

- **Project page:** https://adrient.com/open-source/adrient-workspace
- **Docs:** https://adrient.com/docs/adrient-workspace
- **Support & issues:** https://adrient.com/support

## Reporting issues

[Contact Adrient support](https://adrient.com/support) with:
- What you expected vs. what happened, steps to reproduce.
- Which app/service (Drive, Mail, a specific game, etc.) and platform (desktop/mobile).
- Screenshots or console errors if you have them.

## Pull requests

1. Fork the source repository.
2. Create a branch: `git checkout -b feat/your-thing`.
3. Make your change. Keep it focused; match the surrounding code style.
4. Verify it:
   - Backend: `go build ./... && go vet ./... && go test ./...`. Tests that
     need Postgres create their own scratch databases on the server in
     `ADRIENT_TEST_ADMIN_DSN`, e.g. with the local stack running:
     `ADRIENT_TEST_ADMIN_DSN=postgres://adrient@127.0.0.1:5533/postgres?sslmode=disable go test ./internal/storage/ ./internal/adminops/ ./internal/cloudimport/`
   - Frontend: `cd web/app && npx tsc --noEmit && npx vitest run`.
   - UI changes: check them in a browser (`deploy/local/stack.sh deploy`) and
     attach screenshots.
   CI (`.github/workflows/ci.yml`) runs the same checks on every pull request.
5. Open a pull request against `main` using the template. A maintainer reviews it.

## Database migrations

- Add a new file `internal/storage/migrations/NNNN_name.sql` with the next free
  number in your feature's range. Never renumber or edit a migration that has
  been released; fix forward with a new file.
- Make every statement idempotent (`CREATE TABLE IF NOT EXISTS`,
  `ADD COLUMN IF NOT EXISTS`, `CREATE INDEX IF NOT EXISTS`, guarded inserts).
  `go test ./internal/storage/` applies all migrations to an empty database,
  re-applies every file, and lints for missing `IF NOT EXISTS`, duplicate
  numbers and `DROP`/`TRUNCATE`.
- Scope every table to an organisation (`org_id`) unless it is truly global.

## License & third-party assets

By contributing, you agree your contributions are licensed under the project's
[MIT License](LICENSE). Note that a few **bundled third-party components keep
their own licenses** (e.g. the WinBolo port is GPL v2, Maelstrom is Zlib) — don't
relicense those, and don't add assets you don't have the rights to redistribute.
