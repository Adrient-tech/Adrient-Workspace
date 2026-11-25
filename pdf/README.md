# PDF app

The workspace's PDF editor and document-signing service. Feature list and API:
[FEATURES.md](./FEATURES.md). Certificate signing with tokens and the browser
extension: [docs/X509_SIGNING.md](./docs/X509_SIGNING.md).

## Where the code lives

| Path | What |
|---|---|
| `pdf/frontend/` | The browser app, built with base `/pdf/` |
| `internal/pdf/` | The Go server: API handlers, signing, content removal, storage, migrations (`internal/pdf/database/migrations/`) |
| `internal/pdf/api/proto/` | API definitions; generated code in `internal/pdf/proto/` |
| `cmd/pdfserver/` | Stand-alone server entry point |
| `pdf/extension/`, `pdf/native-signer/` | Browser extension and native host for signing with a hardware token |

## Run locally

The workspace stack builds and serves the app:

```bash
deploy/local/stack.sh up                       # shared stack on :8080
ADRIENT_PORT=8160 deploy/local/stack.sh deploy # a worktree on its own port
```

The app opens at `/pdf/` (editor at `/pdf/editor`). For a scratch PDF
database pass `PDF_DATABASE_URL_OVERRIDE=postgres://...`.

## Tests

```bash
go test ./internal/pdf/...
cd pdf/frontend && npx tsc -b && npx eslint .
# Browser run against a running stack (open, edit text in place, image, sign,
# highlight, form, reorder, download, re-open checks; VP=phone for 390x844):
node pdf/frontend/e2e/make-sample.mjs "$OUT"
ADRIENT_URL=... STATE=... CHROMIUM_PATH=... PLAYWRIGHT_CORE=... OUT="$OUT" node pdf/frontend/e2e/editor-flow.mjs
```

## License

Proprietary - Adrient
