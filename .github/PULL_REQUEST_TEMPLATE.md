## What and why

<!-- What does this change do, and what problem does it solve? Link the issue. -->

## How it was tested

<!-- Commands you ran, and what you looked at in the browser. -->
- [ ] `go build ./... && go test ./...` (with `ADRIENT_TEST_ADMIN_DSN` for Postgres tests)
- [ ] `cd web/app && npx tsc --noEmit && npx vitest run`
- [ ] Checked the change in a browser (screenshots below for UI changes)

## Checklist

- [ ] New migrations are idempotent (`IF NOT EXISTS`), forward-only, and never renumber an existing file
- [ ] Every query is org-scoped and parameterised; no secrets in code or logs
- [ ] Docs updated (`docs/services/<module>.md`, CHANGELOG "Unreleased")
- [ ] Nothing in the UI is a button that does nothing; unbuilt features are hidden or say "not available"
