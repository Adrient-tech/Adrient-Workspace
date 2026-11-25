#!/usr/bin/env bash
# Local dev stack without Nix: Postgres + Zitadel + rustfs in Docker, the Go
# backend and the vite build native. Serves http://workspace.localtest.me:8080
# (login admin / DevPassword!1), the same target web/e2e expects.
#
#   deploy/local/stack.sh up [TREE]      start deps, build TREE, run backend
#   deploy/local/stack.sh deploy [TREE]  rebuild web + backend from TREE, restart
#   deploy/local/stack.sh backend [TREE] restart backend only (skip web build)
#   deploy/local/stack.sh status | logs | down | nuke
#
# TREE defaults to this checkout, so any git worktree can be deployed onto the
# one shared stack. ADRIENT_PORT=8090 runs an extra backend (own pid/log/build)
# beside the main :8080 one against the same databases, e.g. to verify a merge
# while someone else has a branch deployed. Log in via :8080 first: sessions
# live in Postgres and the cookie is shared across ports. State lives in $ADRIENT_LOCAL_DATA (default ~/.adrient-local)
# so it survives across worktrees; `nuke` deletes it.
set -euo pipefail

# Build with the Go version pinned in go.mod: newer Go (1.27+) breaks
# golang.org/x/net/http2 used by gRPC. Go downloads this toolchain itself.
export GOTOOLCHAIN="${GOTOOLCHAIN:-go1.25.7}"
export PATH="$HOME/go/bin:$PATH"

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
export ADRIENT_LOCAL_DATA="${ADRIENT_LOCAL_DATA:-$HOME/.adrient-local}"
DATA="$ADRIENT_LOCAL_DATA"
COMPOSE=(docker compose -f "$HERE/compose.yaml")
PORT="${ADRIENT_PORT:-8080}"
GRPC_PORT=$((PORT + 920))                  # 8080 -> 9000, matching process-compose
SUF=""; [ "$PORT" = 8080 ] || SUF="-$PORT" # per-port pid/log/binary
URL="http://workspace.localtest.me:8080"   # OIDC callback host (always :8080)

log() { printf '\033[1;32m[stack]\033[0m %s\n' "$*" >&2; }
die() { printf '\033[1;31m[stack]\033[0m %s\n' "$*" >&2; exit 1; }

tree_root() {
  local t="${1:-$HERE/../..}"
  t="$(cd "$t" && git rev-parse --show-toplevel)" || die "not a git tree: ${1:-}"
  if [ ! -e "$t/gen/go" ]; then
    command -v buf >/dev/null || die "buf not found: go install github.com/bufbuild/buf/cmd/buf@v1.64.0"
    log "generating protobuf code (gen/)"
    (cd "$t" && buf dep update && buf generate) >&2 || die "buf generate failed"
  fi
  [ -d "$t/web/app/node_modules" ] || { log "installing web dependencies"; (cd "$t/web/app" && npm ci --no-fund --no-audit) >&2 || die "npm ci failed"; }
  echo "$t"
}

wait_http() { # url name tries
  for _ in $(seq 1 "${3:-90}"); do
    curl -sf -o /dev/null "$1" && return 0
    sleep 2
  done
  die "$2 not ready at $1"
}

# LiveKit (Meet SFU): API key/secret are generated once per local install and
# kept in $DATA (never in git); the config is rendered from the template.
livekit_prepare() {
  local t="$1" kf="$DATA/livekit-keys.env"
  mkdir -p "$DATA/livekit"
  if [ ! -s "$kf" ]; then
    printf 'LIVEKIT_API_KEY=API%s\nLIVEKIT_API_SECRET=%s\n' "$(openssl rand -hex 6)" "$(openssl rand -hex 24)" >"$kf.tmp" &&
      chmod 600 "$kf.tmp" && mv "$kf.tmp" "$kf"
  fi
  # shellcheck disable=SC1090
  . "$kf"
  # Media address: the docker bridge (host-only, non-loopback) or loopback.
  local nip; nip="$(ip -4 -o addr show docker0 2>/dev/null | awk '{sub(/\/.*/,"",$4); print $4; exit}')"
  [ -n "$nip" ] || nip=127.0.0.1
  sed -e "s|@@KEY@@|$LIVEKIT_API_KEY|" -e "s|@@SECRET@@|$LIVEKIT_API_SECRET|" -e "s|@@NODE_IP@@|$nip|g" \
    "$t/deploy/livekit/livekit.local.yaml.tmpl" >"$DATA/livekit/livekit.yaml"
  chmod 600 "$DATA/livekit/livekit.yaml"
  sed -e "s|@@KEY@@|$LIVEKIT_API_KEY|" -e "s|@@SECRET@@|$LIVEKIT_API_SECRET|" \
    "$t/deploy/livekit/egress.local.yaml.tmpl" >"$DATA/livekit/egress.yaml"
  chmod 644 "$DATA/livekit/egress.yaml"  # local dev keys; the egress container is non-root
}

deps_up() {
  livekit_prepare "$1"
  mkdir -p "$DATA"/{postgres,rustfs,bin} "$DATA/root/deploy/zitadel/data"
  mkdir -p "$DATA"/{forgejo,twenty-db,twenty-storage}
  log "starting postgres, zitadel, rustfs, livekit, forgejo, twenty"
  "${COMPOSE[@]}" up -d
  wait_http http://127.0.0.1:7880 livekit 30
  wait_http http://127.0.0.1:8081/debug/ready zitadel
  wait_http http://127.0.0.1:9100/health rustfs 60
  local t="$1"
  forgejo_bootstrap
  if [ ! -s "$DATA/root/deploy/zitadel/data/oidc-client.env" ]; then
    log "provisioning OIDC app + admin user"
    # /debug/ready flips before first-boot setup finishes, so the first call
    # can fail on a fresh instance; the script is idempotent, so retry.
    for i in 1 2 3 4 5; do
      PROJECT_ROOT="$DATA/root" ZITADEL_URL=http://localhost:8081 \
        ADRIENT_OIDC_REDIRECT_URL="$URL/api/v1/auth/callback" \
        bash "$t/deploy/zitadel/create-oidc-app.sh" && break
      [ "$i" = 5 ] && die "OIDC provisioning failed"
      log "provisioning attempt $i failed; retrying"; sleep 5
    done
  fi
  RUSTFS_ENDPOINT=http://127.0.0.1:9100 RUSTFS_BUCKET=adrient-default \
    bash "$t/deploy/rustfs/init-bucket.sh"
  # Zitadel client for Twenty (used only once Twenty SSO is licensed; see
  # deploy/twenty/README.md). Idempotent; never fatal.
  PROJECT_ROOT="$DATA/root" ZITADEL_URL=http://localhost:8081 \
    bash "$t/deploy/zitadel/create-twenty-oidc-app.sh" >/dev/null 2>&1 ||
    log "WARN: Twenty OIDC client not created"
  RUSTFS_ENDPOINT=http://127.0.0.1:9100 RUSTFS_BUCKET=pdf-docs \
    bash "$t/deploy/rustfs/init-bucket.sh"
  RUSTFS_ENDPOINT=http://127.0.0.1:9100 RUSTFS_BUCKET=meet-recordings \
    bash "$t/deploy/rustfs/init-bucket.sh"
}

# Forgejo: wait until ready, create the service admin once and keep its API
# credential in $DATA (never in git) for the backend's org provisioner.
forgejo_bootstrap() {
  wait_http http://127.0.0.1:3300/api/healthz forgejo 60
  local f="$DATA/forgejo-admin.txt"
  [ -s "$f" ] && return 0
  log "creating Forgejo service admin"
  "${COMPOSE[@]}" exec -T -u git forgejo forgejo admin user create --admin \
    --username adrient-bot --email adrient-bot@adrient.localtest.me \
    --random-password --must-change-password=false >/dev/null 2>&1 || true
  "${COMPOSE[@]}" exec -T -u git forgejo forgejo admin user generate-access-token \
    --username adrient-bot --token-name "adrient-$(date +%s)" --scopes all --raw >"$f.tmp" &&
    mv "$f.tmp" "$f" || { rm -f "$f.tmp"; log "WARN: Forgejo admin credential not created"; }
}

build_web() {
  log "building web from $1"
  (cd "$1/web/app" && VITE_GIT_URL="${VITE_GIT_URL:-/git/}" \
    VITE_CRM_URL="${VITE_CRM_URL:-http://crm.workspace.localtest.me:8080/}" npm run -s build >"$DATA/web-build.log" 2>&1) ||
    { tail -30 "$DATA/web-build.log"; die "web build failed"; }
  # PDF editor & signing app, served by the backend under /pdf/.
  log "building PDF app"
  (
    cd "$1/pdf/frontend"
    [ -d node_modules ] || npm ci --no-fund --no-audit
    ADRIENT_PDF_BASE=/pdf/ VITE_ADRIENT_INTEGRATED=true VITE_API_BASE=/pdf-api/api \
      VITE_ADRIENT_LOGIN_URL=/api/v1/auth/login npm run -s build
  ) >"$DATA/pdf-build.log" 2>&1 ||
    { tail -30 "$DATA/pdf-build.log"; die "PDF app build failed"; }
}

stop_backend() {
  if [ -s "$DATA/backend$SUF.pid" ] && kill -0 "$(cat "$DATA/backend$SUF.pid")" 2>/dev/null; then
    kill "$(cat "$DATA/backend$SUF.pid")"
    for _ in $(seq 1 20); do kill -0 "$(cat "$DATA/backend$SUF.pid")" 2>/dev/null || break; sleep 0.5; done
  fi
  rm -f "$DATA/backend$SUF.pid"
}

start_backend() {
  local t="$1"
  log "building backend from $t"
  (cd "$t" && go build -o "$DATA/bin/adrient$SUF" ./cmd/server) || die "go build failed"
  stop_backend
  local creds="$DATA/root/deploy/zitadel/data/oidc-client.env"
  (
    [ -s "$creds" ] && { set -a; . "$creds"; set +a; }
    export ADRIENT_POSTGRES_DSN="${ADRIENT_POSTGRES_DSN_OVERRIDE:-postgres://adrient@127.0.0.1:5533/adrient?sslmode=disable}"
    export ADRIENT_OIDC_ISSUER=http://localhost:8081
    export ADRIENT_OIDC_REDIRECT_URL="$URL/api/v1/auth/callback"
    export ADRIENT_SESSION_COOKIE_NAME=adrient_session ADRIENT_SESSION_COOKIE_SECURE=false
    export ADRIENT_SESSION_COOKIE_DOMAIN=workspace.localtest.me ADRIENT_SESSION_LIFETIME=168h
    export ADRIENT_DEFAULT_ORG_SLUG=default
    export ADRIENT_RUSTFS_ENDPOINT=http://127.0.0.1:9100 ADRIENT_RUSTFS_ACCESS_KEY=adrient
    export ADRIENT_RUSTFS_SECRET_KEY='DevPassword!1' ADRIENT_RUSTFS_BUCKET=adrient-default
    export ADRIENT_ZITADEL_API_URL=http://localhost:8081
    # Local dev: the bootstrap service account's PAT enables the in-app
    # email/password form (otherwise only "Sign in with SSO" works).
    pat="$DATA/root/deploy/zitadel/data/bootstrap-pat.txt"
    [ -s "$pat" ] && export ADRIENT_ZITADEL_SERVICE_TOKEN="$(cat "$pat")"
    # Integrated Twenty CRM (Host proxy) and Forgejo (/git proxy + SSO).
    export ADRIENT_CRM_URL=http://127.0.0.1:3010 ADRIENT_CRM_HOST=crm.workspace.localtest.me:8080
    export ADRIENT_FORGEJO_URL=http://127.0.0.1:3300
    [ -s "$DATA/forgejo-admin.txt" ] && export ADRIENT_FORGEJO_ADMIN_TOKEN="$(cat "$DATA/forgejo-admin.txt")"
    export ADRIENT_FORGEJO_WEBHOOK_SECRET=local-dev-forgejo-webhook
    # Meet SFU (LiveKit). Browsers reach it on localhost; the backend calls the admin API.
    if [ -s "$DATA/livekit-keys.env" ]; then
      set -a; . "$DATA/livekit-keys.env"; set +a
      export ADRIENT_LIVEKIT_URL=http://127.0.0.1:7880 ADRIENT_LIVEKIT_CLIENT_URL=ws://localhost:7880
      export ADRIENT_LIVEKIT_API_KEY="$LIVEKIT_API_KEY" ADRIENT_LIVEKIT_API_SECRET="$LIVEKIT_API_SECRET"
      # Recording (Egress) -> RustFS bucket meet-recordings -> copied into Drive.
      export ADRIENT_LIVEKIT_EGRESS=1 ADRIENT_LIVEKIT_EGRESS_S3_ENDPOINT=http://127.0.0.1:9100
      export ADRIENT_LIVEKIT_EGRESS_S3_BUCKET=meet-recordings
      export ADRIENT_LIVEKIT_EGRESS_S3_ACCESS_KEY=adrient ADRIENT_LIVEKIT_EGRESS_S3_SECRET_KEY='DevPassword!1'
    fi
    export ADRIENT_ADMIN_EMAILS="${ADRIENT_ADMIN_EMAILS:-admin@adrient.localtest.me}"
    # Dev only: lets domain verification read TXT answers from a local file.
    export ADRIENT_DEV_MODE=1 ADRIENT_DEV_TXT_OVERRIDE="$DATA/dev-txt.json"
    export ADRIENT_DEV_DNS_OVERRIDE="$DATA/dev-dns.json" ADRIENT_SECRETS_KEY="${ADRIENT_SECRETS_KEY:-dev-local-secrets-key}"
    # Built-in PDF editor & signing. The key-encryption key is made once per
    # local install and kept in $DATA (never in git).
    kek="$DATA/pdf-kek.txt"
    [ -s "$kek" ] || head -c 32 /dev/urandom | base64 >"$kek"
    export ADRIENT_PDF_BUILTIN=true ADRIENT_PDF_STATIC_DIR="$t/pdf/frontend/dist"
    export PDF_DATABASE_URL="${PDF_DATABASE_URL_OVERRIDE:-postgres://adrient@127.0.0.1:5533/pdf?sslmode=disable}"
    export PDF_STORAGE_ENDPOINT=http://127.0.0.1:9100 PDF_STORAGE_BUCKET=pdf-docs PDF_STORAGE_REGION=us-east-1
    export PDF_STORAGE_ACCESS_KEY=adrient PDF_STORAGE_SECRET_KEY='DevPassword!1'
    export PDF_CRYPTO_KEY_ENCRYPTION_KEY="$(cat "$kek")"
    # Optional features read the caller's environment, which this subshell
    # inherits: e.g. ADRIENT_LIBREOFFICE=1 (plus ADRIENT_SOFFICE_PATH,
    # ADRIENT_LIBREOFFICE_ODF, …) enables LibreOffice import (see README.md).
    [ -n "${ADRIENT_LIBREOFFICE:-}" ] && log "ADRIENT_LIBREOFFICE=$ADRIENT_LIBREOFFICE (soffice: ${ADRIENT_SOFFICE_PATH:-auto-detect})"
    cd "$t"
    nohup "$DATA/bin/adrient$SUF" --http-addr=":$PORT" --grpc-addr=":$GRPC_PORT" \
      --static-dir="$t/web/app/dist" >"$DATA/backend$SUF.log" 2>&1 &
    echo $! >"$DATA/backend$SUF.pid"
  )
  wait_http "http://127.0.0.1:$PORT/healthz" backend 30 ||
    { tail -30 "$DATA/backend$SUF.log"; exit 1; }
  # /healthz answering isn't proof it's *our* process: a stray backend already
  # bound to $PORT (another worktree/agent) answers while ours dies on bind.
  # Require the process we started to be alive and to own the listener.
  local pid; pid="$(cat "$DATA/backend$SUF.pid")"
  sleep 1
  if ! kill -0 "$pid" 2>/dev/null; then
    tail -30 "$DATA/backend$SUF.log"
    die "backend exited after start; is :$PORT or :$GRPC_PORT held by another process? ($(lsof -nP -iTCP:"$PORT" -sTCP:LISTEN -t 2>/dev/null | head -1))"
  fi
  if command -v lsof >/dev/null; then
    local owner; owner="$(lsof -nP -iTCP:"$PORT" -sTCP:LISTEN -t 2>/dev/null | head -1)"
    [ -z "$owner" ] || [ "$owner" = "$pid" ] ||
      die ":$PORT is served by pid $owner, not the backend just started ($pid)"
  fi
  echo "$t" >"$DATA/deployed-tree$SUF"
  log "backend up: http://workspace.localtest.me:$PORT  (tree: $t @ $(git -C "$t" rev-parse --short HEAD))"
}

cmd="${1:-status}"; shift || true
case "$cmd" in
  up)      t="$(tree_root "${1:-}")"; deps_up "$t"; build_web "$t"; start_backend "$t" ;;
  deploy)  t="$(tree_root "${1:-}")"; build_web "$t"; start_backend "$t" ;;
  backend) t="$(tree_root "${1:-}")"; start_backend "$t" ;;
  status)
    "${COMPOSE[@]}" ps --format '{{.Service}}: {{.State}}'
    if curl -sf -o /dev/null "http://127.0.0.1:$PORT/healthz"; then
      echo "backend: up ($(cat "$DATA/deployed-tree$SUF" 2>/dev/null))"
    else echo "backend: down"; fi ;;
  logs)    tail -n "${1:-100}" -f "$DATA/backend$SUF.log" ;;
  down)    # On an extra port, stop only that backend; the shared deps stay up.
           stop_backend; [ "$PORT" = 8080 ] && "${COMPOSE[@]}" down ;;
  nuke)    stop_backend; "${COMPOSE[@]}" down -v; rm -rf "$DATA" ;;
  *)       die "unknown command: $cmd (up|deploy|backend|status|logs|down|nuke)" ;;
esac
