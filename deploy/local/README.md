# Local stack without Nix

`deploy/process-compose` needs the Nix dev shell (postgres, zitadel,
process-compose). `deploy/local` gives you the same stack on any machine with
Docker, Go and Node. Postgres, Zitadel and rustfs run in containers; the Go
backend and the vite build run natively, so redeploying a change takes seconds.

```sh
deploy/local/stack.sh up                 # first boot: deps + OIDC app + build + backend
deploy/local/stack.sh deploy ../other-wt # rebuild web + backend from another worktree
deploy/local/stack.sh backend            # Go-only change: skip the web build
deploy/local/stack.sh status | logs | down
deploy/local/stack.sh nuke               # delete all local state
```

- App: http://workspace.localtest.me:8080, login `admin@adrient.localtest.me` / `DevPassword!1`.
  Zitadel is at http://localhost:8081.
- State lives in `$ADRIENT_LOCAL_DATA` (default `~/.adrient-local`), shared by
  every worktree, so one stack can serve whichever branch you deploy.
- `stack.sh` generates `gen/` with `buf` and runs `npm ci` when missing. Install
  buf once: `go install github.com/bufbuild/buf/cmd/buf@v1.64.0`. It builds with
  Go 1.25.7 (`GOTOOLCHAIN`), which Go downloads by itself.
- No AWS CLI needed: the bucket is created with the `amazon/aws-cli` image.
- The containers restart with Docker (`restart: unless-stopped`). If Docker
  Desktop restarts, or the disk fills up, run `stack.sh up` again.

### Git (Forgejo) and CRM (Twenty)

Both run as separate containers under their own licenses (Forgejo GPL-3.0,
Twenty AGPL-3.0); the backend only reverse-proxies them.

- **Git**: http://workspace.localtest.me:8080/git/ (container port 127.0.0.1:3300).
  Single sign-on through Forgejo reverse-proxy auth: the backend sets
  `X-WEBAUTH-USER` from the Adrient session, Forgejo auto-creates the user.
  `stack.sh up` creates a `adrient-bot` admin and keeps its API credential in
  `$ADRIENT_LOCAL_DATA/forgejo-admin.txt` for org provisioning. Branding
  (name "Adrient Git", logo, favicon, green theme, no version footer) comes
  from env in `compose.yaml` and `deploy/forgejo/custom/`.
- **CRM**: http://crm.workspace.localtest.me:8080/ (container port 127.0.0.1:3010),
  proxied by Host. See `deploy/twenty/README.md` for the one-time workspace
  setup and why SSO is not automatic.

The web build gets `VITE_GIT_URL=/git/` and
`VITE_CRM_URL=http://crm.workspace.localtest.me:8080/` from `stack.sh`
(override by exporting them).

### Optional LibreOffice import

Legacy `.doc`/`.xls`/`.ppt` import runs LibreOffice headless on the server
and is off by default. `stack.sh` passes your environment through to the
backend, so enable it per deploy:

```sh
ADRIENT_LIBREOFFICE=1 deploy/local/stack.sh backend
# optional: ADRIENT_SOFFICE_PATH=/path/to/soffice (default: PATH, then
# /Applications/LibreOffice.app), ADRIENT_LIBREOFFICE_ODF=1 to also route
# .odt/.ods/.odp/.rtf through LibreOffice
```

`web/e2e/docs-legacy-doc.spec.ts` needs a backend started this way (it skips
otherwise).

Then run the e2e suite (`cd web/e2e && npx playwright test`). For screenshots,
run the visual tour: `ADRIENT_TOUR=1 npx playwright test tour.spec.ts`, which
writes `test-results/tour/*.png`.

## Testing a frontend branch without redeploying

Several people or agents can share one stack. Serve your worktree's frontend
from vite on its own port; it proxies `/api` (including the collab
WebSockets) to the backend on :8080:

```sh
cd web/app && npx vite --port 5181 --strictPort --host workspace.localtest.me
cd web/e2e && npx playwright test auth.setup.ts            # log in via :8080
ADRIENT_HTTP_URL=http://workspace.localtest.me:5181 npx playwright test sheets.spec.ts --no-deps
```

The OIDC callback always returns to :8080, so log in there first. The
session cookie is shared across ports. `--no-deps` skips re-running the
login against the vite port. Known gap: Docs' Yjs collab can misbehave under
the vite dev build, so check Docs persistence on a real deploy
(`stack.sh deploy`).
