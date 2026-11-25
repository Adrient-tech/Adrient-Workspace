#!/usr/bin/env bash
# Adrient Workspace: one-command server installer.
#
#   curl -fsSL <url>/install.sh | sudo bash
#   sudo ./install.sh [--domain example.com] [--email you@example.com] ...
#
# Installs Docker (if missing), generates all secrets, builds the app image,
# starts Postgres + Zitadel + RustFS + the app behind Caddy (automatic HTTPS),
# provisions single sign-on and the first admin, and prints the login.
# Re-running upgrades in place and keeps /opt/adrient-workspace/.env + data.
#
# Commands: install (default) | status | logs [service] | backup | restore FILE
#           | update | uninstall            Run with --help for all flags.
set -euo pipefail

# ---------------------------------------------------------------- defaults --
INSTALL_DIR="${ADRIENT_INSTALL_DIR:-/opt/adrient-workspace}"
SOURCE_URL="${ADRIENT_SOURCE_URL:-https://github.com/Adrient-tech/Adrient-Workspace.git}"
SOURCE_REF="${ADRIENT_SOURCE_REF:-}"
IMAGE="${ADRIENT_IMAGE:-}"
DOMAIN="${ADRIENT_DOMAIN:-}"
ACME_EMAIL="${ADRIENT_ACME_EMAIL:-}"
ADMIN_EMAIL="${ADRIENT_ADMIN_EMAIL:-}"
WITH_CRM="${ADRIENT_WITH_CRM:-}"
WITH_GIT="${ADRIENT_WITH_GIT:-}"
NONINTERACTIVE="${ADRIENT_NON_INTERACTIVE:-0}"
STAGING="${ADRIENT_ACME_STAGING:-0}"
SKIP_CHECKS="${ADRIENT_SKIP_CHECKS:-0}"
ASSUME_YES=0
PURGE=0
DB_ONLY=0
CMD=""
CMD_ARGS=()

SCRIPT_DIR=""
if [ -n "${BASH_SOURCE[0]:-}" ] && [ -f "${BASH_SOURCE[0]}" ]; then
  SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
fi

if [ -t 2 ]; then C_G=$'\033[1;32m'; C_Y=$'\033[1;33m'; C_R=$'\033[1;31m'; C_0=$'\033[0m'; else C_G=""; C_Y=""; C_R=""; C_0=""; fi
log()  { printf '%s[adrient]%s %s\n' "$C_G" "$C_0" "$*" >&2; }
warn() { printf '%s[adrient] warning:%s %s\n' "$C_Y" "$C_0" "$*" >&2; }
die()  { printf '%s[adrient] error:%s %s\n' "$C_R" "$C_0" "$*" >&2; exit 1; }

usage() {
  cat <<'EOF'
Adrient Workspace installer

Usage: install.sh [command] [flags]

Commands:
  install            Install or upgrade (default)
  status             Show service state and URLs
  logs [service]     Follow logs (e.g. logs adrient)
  backup [--db-only] Write a backup archive to <dir>/backups
  restore FILE       Restore a backup archive made by "backup"
  update             Pull the latest source, rebuild, restart (backs up first)
  uninstall          Stop everything; asks before deleting data (--purge to skip)

Flags (each also has an environment variable):
  --domain D           Base domain; hosts become workspace.D, auth.D, crm.D   ADRIENT_DOMAIN
  --email E            Contact email for Let's Encrypt                         ADRIENT_ACME_EMAIL
  --admin-email E      First admin login (default: --email)                    ADRIENT_ADMIN_EMAIL
  --with-crm           Install Twenty CRM at crm.D                             ADRIENT_WITH_CRM=1
  --with-git           Install Forgejo git hosting at workspace.D/git          ADRIENT_WITH_GIT=1
  --non-interactive,-y Never prompt; missing required values are errors        ADRIENT_NON_INTERACTIVE=1
  --dir PATH           Install directory (default /opt/adrient-workspace)      ADRIENT_INSTALL_DIR
  --source-url URL     Git URL of the source                                   ADRIENT_SOURCE_URL
  --ref REF            Branch or tag to build                                  ADRIENT_SOURCE_REF
  --image IMAGE        Pull this prebuilt image instead of building            ADRIENT_IMAGE
  --staging            Use the Let's Encrypt staging CA (test certificates)    ADRIENT_ACME_STAGING=1
  --skip-checks        Continue past failed OS/RAM/disk/port/DNS checks        ADRIENT_SKIP_CHECKS=1
  --purge              uninstall: delete data without asking
  --db-only            backup: skip file storage (Drive/Photos objects)
  -h, --help           This help
EOF
}

# ---------------------------------------------------------------- arguments --
parse_args() {
  while [ $# -gt 0 ]; do
    case "$1" in
      install|status|logs|backup|restore|update|uninstall)
        if [ -z "$CMD" ]; then CMD="$1"; else CMD_ARGS+=("$1"); fi ;;
      --domain)       DOMAIN="${2:?--domain needs a value}"; shift ;;
      --domain=*)     DOMAIN="${1#*=}" ;;
      --email)        ACME_EMAIL="${2:?--email needs a value}"; shift ;;
      --email=*)      ACME_EMAIL="${1#*=}" ;;
      --admin-email)  ADMIN_EMAIL="${2:?--admin-email needs a value}"; shift ;;
      --admin-email=*) ADMIN_EMAIL="${1#*=}" ;;
      --with-crm)     WITH_CRM=1 ;;
      --with-git)     WITH_GIT=1 ;;
      --non-interactive|-y|--yes) NONINTERACTIVE=1; ASSUME_YES=1 ;;
      --dir)          INSTALL_DIR="${2:?--dir needs a value}"; shift ;;
      --source-url)   SOURCE_URL="${2:?--source-url needs a value}"; shift ;;
      --ref)          SOURCE_REF="${2:?--ref needs a value}"; shift ;;
      --image)        IMAGE="${2:?--image needs a value}"; shift ;;
      --staging)      STAGING=1 ;;
      --skip-checks)  SKIP_CHECKS=1 ;;
      --purge)        PURGE=1 ;;
      --db-only)      DB_ONLY=1 ;;
      -h|--help)      usage; exit 0 ;;
      -*)             die "unknown flag: $1 (see --help)" ;;
      *)              CMD_ARGS+=("$1") ;;
    esac
    shift
  done
  CMD="${CMD:-install}"
}

# ------------------------------------------------------------------ helpers --
is_true() { case "${1:-}" in 1|true|yes|y|Y|on) return 0 ;; *) return 1 ;; esac; }
have() { command -v "$1" >/dev/null 2>&1; }
need_root() { [ "$(id -u)" -eq 0 ] || die "run as root: sudo ./install.sh (or pipe to 'sudo bash')"; }

# Read a line from the terminal even when the script itself arrives on stdin.
tty_read() { # var prompt default
  local __var="$1" __prompt="$2" __def="${3:-}" __ans=""
  if [ -r /dev/tty ] && [ -w /dev/tty ]; then
    printf '%s' "$__prompt" >/dev/tty
    IFS= read -r __ans </dev/tty || __ans=""
  else
    return 1
  fi
  printf -v "$__var" '%s' "${__ans:-$__def}"
}

ask() { # var "Prompt" default required(1|0)
  local __var="$1" __prompt="$2" __def="${3:-}" __req="${4:-1}" __cur="${!1:-}"
  [ -n "$__cur" ] && return 0
  if is_true "$NONINTERACTIVE"; then
    [ -n "$__def" ] && { printf -v "$__var" '%s' "$__def"; return 0; }
    [ "$__req" = 1 ] && die "missing required value: $__var (pass the flag or env var, see --help)"
    return 0
  fi
  local __suffix=""; [ -n "$__def" ] && __suffix=" [$__def]"
  tty_read "$__var" "$__prompt$__suffix: " "$__def" ||
    die "no terminal for prompts; re-run with --non-interactive and flags (see --help)"
  [ -z "${!__var}" ] && [ "$__req" = 1 ] && die "$__var is required"
  return 0
}

confirm() { # "Question" default(y|n)
  local q="$1" def="${2:-n}" ans=""
  is_true "$ASSUME_YES" && return 0
  is_true "$NONINTERACTIVE" && { [ "$def" = y ]; return; }
  tty_read ans "$q [$([ "$def" = y ] && echo Y/n || echo y/N)] " "$def" || return 1
  case "$ans" in y|Y|yes|YES) return 0 ;; *) return 1 ;; esac
}

gen_secret() { openssl rand -hex "${1:-24}"; }

ENV_FILE=""
get_env() { # KEY -> value ('' when absent)
  [ -f "$ENV_FILE" ] || return 0
  grep -m1 "^$1=" "$ENV_FILE" | cut -d= -f2- || true
}
put_env() { # KEY VALUE (always overwrite)
  local tmp; tmp="$(mktemp "$INSTALL_DIR/.env.XXXXXX")"
  { [ -f "$ENV_FILE" ] && { grep -v "^$1=" "$ENV_FILE" || true; }; printf '%s=%s\n' "$1" "$2"; } >"$tmp"
  chmod 600 "$tmp"; mv "$tmp" "$ENV_FILE"
}
default_env() { # KEY VALUE (only when absent or empty)
  [ -n "$(get_env "$1")" ] || put_env "$1" "$2"
}

dc() { docker compose --project-directory "$INSTALL_DIR" -f "$INSTALL_DIR/compose.yaml" --env-file "$ENV_FILE" "$@"; }

load_state() {
  ENV_FILE="$INSTALL_DIR/.env"
  [ -f "$ENV_FILE" ] || die "no installation found at $INSTALL_DIR (run install first)"
  DOMAIN="$(get_env ADRIENT_DOMAIN)"
  WORKSPACE_HOST="$(get_env WORKSPACE_HOST)"
  AUTH_HOST="$(get_env AUTH_HOST)"
  CRM_HOST="$(get_env CRM_HOST)"
}

# ---------------------------------------------------------------- preflight --
preflight() {
  log "checking this server"
  local fail=0
  [ "$(uname -s)" = Linux ] || die "Linux only"
  case "$(uname -m)" in x86_64|amd64|aarch64|arm64) ;; *) warn "untested CPU architecture: $(uname -m)" ;; esac

  OS_ID=""; OS_VER=""; OS_LIKE=""
  if [ -r /etc/os-release ]; then
    # shellcheck disable=SC1091
    OS_ID="$(. /etc/os-release; echo "${ID:-}")"
    OS_VER="$(. /etc/os-release; echo "${VERSION_ID:-}")"
    OS_LIKE="$(. /etc/os-release; echo "${ID_LIKE:-}")"
  fi
  case "$OS_ID:$OS_VER" in
    ubuntu:22.04|ubuntu:24.04|debian:12) log "OS: $OS_ID $OS_VER (supported)" ;;
    *)
      case "$OS_ID $OS_LIKE" in
        *rhel*|*fedora*|*centos*|*rocky*|*almalinux*) warn "OS: $OS_ID $OS_VER is RHEL-family (best effort, not fully tested)" ;;
        *debian*|*ubuntu*) warn "OS: $OS_ID $OS_VER is Debian-family but not a tested release" ;;
        *) warn "OS: ${OS_ID:-unknown} $OS_VER is not supported (Ubuntu 22.04/24.04, Debian 12)"; fail=1 ;;
      esac ;;
  esac

  local cpus mem_mb disk_gb
  cpus="$(nproc 2>/dev/null || echo 1)"
  [ "$cpus" -ge 2 ] || warn "only $cpus CPU core; 2+ recommended (the image build is slow)"
  mem_mb="$(awk '/MemTotal/ {print int($2/1024)}' /proc/meminfo)"
  if [ "$mem_mb" -lt 3500 ]; then
    warn "RAM ${mem_mb} MB is below the 4 GB minimum"; fail=1
  elif [ "$mem_mb" -lt 7500 ]; then
    warn "RAM ${mem_mb} MB works, but 8 GB+ is recommended (more with the CRM)"
  else
    log "RAM: ${mem_mb} MB"
  fi
  mkdir -p "$(dirname "$INSTALL_DIR")"
  disk_gb="$(df -Pk "$(dirname "$INSTALL_DIR")" | awk 'NR==2 {print int($4/1048576)}')"
  if [ "$disk_gb" -lt 15 ]; then warn "only ${disk_gb} GB free disk; need 15 GB+ (40 GB+ recommended)"; fail=1
  elif [ "$disk_gb" -lt 40 ]; then warn "${disk_gb} GB free disk; 40 GB+ recommended"
  else log "free disk: ${disk_gb} GB"; fi

  # Ports 80/443 must be free unless our own Caddy already holds them (re-run).
  local p busy=""
  for p in 80 443; do
    if have ss && ss -H -ltn "sport = :$p" 2>/dev/null | grep -q .; then busy="$busy $p"; fi
  done
  if [ -n "$busy" ]; then
    if have docker && [ -f "$INSTALL_DIR/compose.yaml" ] &&
       docker ps --format '{{.Names}}' 2>/dev/null | grep -q '^adrient-workspace-caddy'; then
      log "ports$busy held by the existing Adrient Caddy (upgrade)"
    else
      warn "port(s)$busy already in use by another service (stop nginx/apache first)"; fail=1
    fi
  fi

  if [ "$fail" = 1 ]; then
    is_true "$SKIP_CHECKS" || die "checks failed (fix the items above, or pass --skip-checks to continue anyway)"
    warn "continuing because --skip-checks was given"
  fi
}

install_packages() {
  local missing=() t
  for t in curl openssl jq git; do have "$t" || missing+=("$t"); done
  have ss || missing+=(iproute)
  [ ${#missing[@]} -eq 0 ] && return 0
  log "installing packages: ${missing[*]}"
  if have apt-get; then
    local pk=("${missing[@]/iproute/iproute2}")
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -qq && apt-get install -y -qq ca-certificates "${pk[@]}"
  elif have dnf; then
    dnf install -y -q "${missing[@]/iproute/iproute}" ca-certificates
  elif have yum; then
    yum install -y -q "${missing[@]}" ca-certificates
  else
    die "install these first: ${missing[*]}"
  fi
}

install_docker() {
  if have docker && docker compose version >/dev/null 2>&1; then
    log "Docker: $(docker --version | cut -d, -f1), $(docker compose version | head -1)"
    systemctl enable --now docker >/dev/null 2>&1 || true
    return 0
  fi
  log "installing Docker Engine + compose plugin from Docker's official repository"
  case "$OS_ID $OS_LIKE" in
    *ubuntu*|*debian*)
      local d="debian" codename
      case "$OS_ID" in ubuntu) d=ubuntu ;; debian) d=debian ;; *) case "$OS_LIKE" in *ubuntu*) d=ubuntu ;; esac ;; esac
      codename="$(. /etc/os-release; echo "${UBUNTU_CODENAME:-${VERSION_CODENAME:-}}")"
      [ -n "$codename" ] || die "cannot determine the distro codename for the Docker repository"
      export DEBIAN_FRONTEND=noninteractive
      apt-get update -qq && apt-get install -y -qq ca-certificates curl gnupg
      install -m 0755 -d /etc/apt/keyrings
      curl -fsSL "https://download.docker.com/linux/$d/gpg" -o /etc/apt/keyrings/docker.asc
      chmod a+r /etc/apt/keyrings/docker.asc
      printf 'deb [arch=%s signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/%s %s stable\n' \
        "$(dpkg --print-architecture)" "$d" "$codename" >/etc/apt/sources.list.d/docker.list
      apt-get update -qq
      apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
      ;;
    *rhel*|*fedora*|*centos*|*rocky*|*almalinux*)
      local repo=centos
      case "$OS_ID" in fedora) repo=fedora ;; rhel) repo=rhel ;; esac
      if have dnf; then
        dnf install -y -q dnf-plugins-core
        dnf config-manager --add-repo "https://download.docker.com/linux/$repo/docker-ce.repo" ||
          dnf config-manager addrepo --from-repofile="https://download.docker.com/linux/$repo/docker-ce.repo"
        dnf install -y -q docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
      else
        die "install Docker manually: https://docs.docker.com/engine/install/"
      fi
      ;;
    *) die "cannot install Docker automatically on ${OS_ID:-this OS}; see https://docs.docker.com/engine/install/" ;;
  esac
  systemctl enable --now docker
  docker compose version >/dev/null 2>&1 || die "Docker installed but the compose plugin is missing"
}

# ------------------------------------------------------------------- source --
SRC_DIR=""
prepare_source() {
  if [ -n "$SCRIPT_DIR" ] && [ -f "$SCRIPT_DIR/Dockerfile" ] && [ -f "$SCRIPT_DIR/deploy/production/compose.yaml" ]; then
    SRC_DIR="$SCRIPT_DIR"
    log "using the source checkout at $SRC_DIR"
    return 0
  fi
  SRC_DIR="$INSTALL_DIR/src"
  local stored; stored="$(get_env ADRIENT_SRC_DIR 2>/dev/null || true)"
  if [ -n "$stored" ] && [ "$stored" != "$SRC_DIR" ] && [ -f "$stored/Dockerfile" ]; then
    SRC_DIR="$stored"; log "using the source checkout recorded earlier: $SRC_DIR"; return 0
  fi
  if [ -d "$SRC_DIR/.git" ]; then
    log "updating source in $SRC_DIR"
    git -C "$SRC_DIR" fetch --quiet --tags origin
    if [ -n "$SOURCE_REF" ]; then git -C "$SRC_DIR" checkout --quiet "$SOURCE_REF"; fi
    git -C "$SRC_DIR" pull --quiet --ff-only 2>/dev/null || warn "source has local changes or a detached ref; building as is"
  else
    log "cloning $SOURCE_URL"
    git clone --quiet ${SOURCE_REF:+--branch "$SOURCE_REF"} "$SOURCE_URL" "$SRC_DIR" ||
      die "could not clone $SOURCE_URL (set ADRIENT_SOURCE_URL, or run install.sh from a checkout)"
  fi
  [ -f "$SRC_DIR/Dockerfile" ] || die "$SRC_DIR is not an Adrient Workspace source tree"
}

sync_files() {
  local p="$SRC_DIR/deploy/production"
  install -m 0644 "$p/compose.yaml" "$INSTALL_DIR/compose.yaml"
  install -m 0755 "$SRC_DIR/install.sh" "$INSTALL_DIR/install.sh" 2>/dev/null || true
  install -m 0644 "$p/Caddyfile" "$INSTALL_DIR/Caddyfile"
  mkdir -p "$INSTALL_DIR/initdb" "$INSTALL_DIR/sites"
  install -m 0644 "$p/initdb/01-databases.sql" "$INSTALL_DIR/initdb/01-databases.sql"
  if is_true "$(get_env WITH_CRM_ENABLED)"; then
    install -m 0644 "$p/sites/crm.caddy.example" "$INSTALL_DIR/sites/crm.caddy"
  else
    rm -f "$INSTALL_DIR/sites/crm.caddy"
  fi
  # Meet SFU (LiveKit): signalling is proxied at rtc.<domain>; media goes
  # straight to the server (7881/tcp, 50000-50999/udp, 3478/udp).
  install -m 0644 "$p/sites/rtc.caddy.example" "$INSTALL_DIR/sites/rtc.caddy"
  mkdir -p "$INSTALL_DIR/livekit"
  sed -e "s|@@KEY@@|$(get_env LIVEKIT_API_KEY)|" -e "s|@@SECRET@@|$(get_env LIVEKIT_API_SECRET)|" \
    "$SRC_DIR/deploy/livekit/livekit.prod.yaml.tmpl" >"$INSTALL_DIR/livekit/livekit.yaml"
  chmod 600 "$INSTALL_DIR/livekit/livekit.yaml"
  # Meet recording (LiveKit Egress) needs no rendered file: compose passes its
  # config (same API key as the SFU) via EGRESS_CONFIG_BODY, so the secret never
  # sits in a world-readable file (the egress container runs as a non-root user).
}

# --------------------------------------------------------------------- .env --
write_env() {
  mkdir -p "$INSTALL_DIR"; chmod 755 "$INSTALL_DIR"
  [ -f "$ENV_FILE" ] || (umask 077; : >"$ENV_FILE")
  chmod 600 "$ENV_FILE"

  # Values given now win; otherwise keep what an earlier run stored.
  [ -n "$DOMAIN" ] || DOMAIN="$(get_env ADRIENT_DOMAIN)"
  [ -n "$ACME_EMAIL" ] || ACME_EMAIL="$(get_env ACME_EMAIL)"
  [ -n "$ADMIN_EMAIL" ] || ADMIN_EMAIL="$(get_env ADRIENT_ADMIN_EMAIL)"
  [ -n "$WITH_CRM" ] || WITH_CRM="$(get_env WITH_CRM_ENABLED)"
  [ -n "$WITH_GIT" ] || WITH_GIT="$(get_env WITH_GIT_ENABLED)"

  put_env ADRIENT_DOMAIN "$DOMAIN"
  put_env WORKSPACE_HOST "workspace.$DOMAIN"
  put_env AUTH_HOST "auth.$DOMAIN"
  put_env CRM_HOST "crm.$DOMAIN"
  put_env RTC_HOST "rtc.$DOMAIN"
  put_env ACME_EMAIL "$ACME_EMAIL"
  put_env ADRIENT_ADMIN_EMAIL "$ADMIN_EMAIL"
  put_env ADRIENT_DATA_DIR "$INSTALL_DIR/data"
  put_env ADRIENT_SRC_DIR "$SRC_DIR"
  if is_true "$STAGING"; then
    put_env CADDY_ACME_CA "https://acme-staging-v02.api.letsencrypt.org/directory"
  else
    put_env CADDY_ACME_CA "https://acme-v02.api.letsencrypt.org/directory"
  fi
  if [ -n "$IMAGE" ]; then put_env ADRIENT_IMAGE "$IMAGE"; fi

  # Secrets: generated once, never rotated by a re-run.
  default_env POSTGRES_PASSWORD "$(gen_secret 24)"
  default_env ZITADEL_MASTERKEY "$(gen_secret 16)"                 # exactly 32 chars
  default_env ZITADEL_ADMIN_PASSWORD "Adr-$(gen_secret 10)-A1!"
  default_env RUSTFS_ACCESS_KEY "adrient$(gen_secret 6)"
  default_env RUSTFS_SECRET_KEY "$(gen_secret 24)"
  default_env PDF_KEK "$(openssl rand -base64 32)"
  default_env LIVEKIT_API_KEY "API$(gen_secret 6)"
  default_env LIVEKIT_API_SECRET "$(gen_secret 32)"                # LiveKit wants >= 32 chars
  default_env ADRIENT_FORGEJO_WEBHOOK_SECRET "$(gen_secret 24)"
  default_env ZITADEL_LOCAL_PORT 18081

  local profiles=""
  if is_true "$WITH_CRM"; then
    put_env WITH_CRM_ENABLED 1
    default_env TWENTY_DB_PASSWORD "$(gen_secret 24)"
    default_env TWENTY_ENCRYPTION_KEY "$(openssl rand -base64 32)"
    default_env TWENTY_APP_SECRET "$(openssl rand -base64 32)"
    put_env ADRIENT_CRM_URL "http://twenty-server:3000"
    put_env ADRIENT_CRM_HOST "crm.$DOMAIN"
    put_env ADRIENT_CRM_PUBLIC_URL "https://crm.$DOMAIN"
    profiles="crm"
  else
    put_env WITH_CRM_ENABLED 0
    put_env ADRIENT_CRM_URL ""; put_env ADRIENT_CRM_HOST ""; put_env ADRIENT_CRM_PUBLIC_URL ""
  fi
  if is_true "$WITH_GIT"; then
    put_env WITH_GIT_ENABLED 1
    default_env FORGEJO_SECRET_KEY "$(gen_secret 32)"
    put_env ADRIENT_FORGEJO_URL "http://forgejo:3000"
    put_env ADRIENT_GIT_PUBLIC_URL "/git/"
    profiles="${profiles:+$profiles,}git"
  else
    put_env WITH_GIT_ENABLED 0
    put_env ADRIENT_FORGEJO_URL ""; put_env ADRIENT_FORGEJO_ADMIN_TOKEN ""
    put_env ADRIENT_GIT_PUBLIC_URL ""
  fi
  put_env COMPOSE_PROFILES "$profiles"
  chmod 600 "$ENV_FILE"

  mkdir -p "$INSTALL_DIR/data"/{postgres,rustfs,zitadel,caddy/data,caddy/config,backups}
  mkdir -p "$INSTALL_DIR/backups"
  # Zitadel runs as uid 1000 and writes its bootstrap file here.
  chown 1000:1000 "$INSTALL_DIR/data/zitadel"; chmod 700 "$INSTALL_DIR/data/zitadel"
  is_true "$WITH_CRM" && mkdir -p "$INSTALL_DIR/data"/{twenty-db,twenty-storage}
  if is_true "$WITH_GIT"; then mkdir -p "$INSTALL_DIR/data/forgejo"; chown 1000:1000 "$INSTALL_DIR/data/forgejo"; fi
  if is_true "$WITH_CRM"; then chown 1000:1000 "$INSTALL_DIR/data/twenty-storage"; fi
  WORKSPACE_HOST="workspace.$DOMAIN"; AUTH_HOST="auth.$DOMAIN"; CRM_HOST="crm.$DOMAIN"
}

# ---------------------------------------------------------------------- DNS --
public_ip() { curl -4fsS --max-time 8 https://api.ipify.org 2>/dev/null || curl -4fsS --max-time 8 https://ifconfig.me 2>/dev/null || true; }

check_dns() {
  local ip h got hosts
  ip="$(public_ip)"
  hosts="$WORKSPACE_HOST $AUTH_HOST rtc.$DOMAIN"
  is_true "$WITH_CRM" && hosts="$hosts $CRM_HOST"
  local bad=0
  for h in $hosts; do
    got="$(getent ahostsv4 "$h" 2>/dev/null | awk '{print $1; exit}')"
    if [ -z "$got" ]; then warn "DNS: $h does not resolve yet"; bad=1
    elif [ -n "$ip" ] && [ "$got" != "$ip" ]; then warn "DNS: $h -> $got, but this server is $ip (fine behind Cloudflare proxy)"; bad=1
    else log "DNS ok: $h -> $got"; fi
  done
  if [ "$bad" = 1 ]; then
    cat >&2 <<EOF
  Create A (and AAAA) records pointing at ${ip:-the public IP of this server}:
    $WORKSPACE_HOST, $AUTH_HOST, rtc.$DOMAIN$(is_true "$WITH_CRM" && echo ", $CRM_HOST")   (or one wildcard *.$DOMAIN)
  rtc.$DOMAIN must be DNS-only (not proxied by Cloudflare): media needs the real IP.
  Firewall: 80/tcp 443/tcp 443/udp 7881/tcp 3478/udp 50000-50999/udp (Meet video calls).
  HTTPS certificates are issued automatically once the names resolve.
EOF
    if ! is_true "$SKIP_CHECKS" && ! is_true "$NONINTERACTIVE"; then
      confirm "Continue anyway? (the install works, but sign-in fails until DNS is set)" y || die "aborted; fix DNS and re-run"
    fi
  fi
}

# ----------------------------------------------------------- Zitadel (OIDC) --
ZURL=""; PAT=""
zapi() { # METHOD PATH [json]  (talks to loopback with the public Host header)
  local m="$1" path="$2" body="${3:-}"
  if [ -n "$body" ]; then
    curl -fsS -X "$m" "$ZURL$path" -H "Host: $AUTH_HOST" -H "Authorization: Bearer $PAT" \
      -H 'Content-Type: application/json' -d "$body"
  else
    curl -fsS -X "$m" "$ZURL$path" -H "Host: $AUTH_HOST" -H "Authorization: Bearer $PAT"
  fi
}

wait_zitadel() {
  ZURL="http://127.0.0.1:$(get_env ZITADEL_LOCAL_PORT)"
  log "waiting for Zitadel (first start runs database setup, 1-3 minutes)"
  local i
  for i in $(seq 1 120); do
    if curl -fsS -o /dev/null -H "Host: $AUTH_HOST" "$ZURL/debug/ready" 2>/dev/null &&
       [ -s "$INSTALL_DIR/data/zitadel/bootstrap-pat.txt" ]; then
      PAT="$(tr -d '[:space:]' <"$INSTALL_DIR/data/zitadel/bootstrap-pat.txt")"
      # /debug/ready flips before first-instance setup completes; confirm the API.
      if zapi GET /management/v1/orgs/me >/dev/null 2>&1; then return 0; fi
    fi
    sleep 3
  done
  dc logs --tail 40 zitadel >&2 || true
  die "Zitadel did not become ready (see logs above: install.sh logs zitadel)"
}

# create_oidc_app NAME PROJECT REDIRECT POSTLOGOUT -> prints "clientId clientSecret"
# Same flow as deploy/zitadel/create-oidc-app.sh, for HTTPS production hosts.
create_oidc_app() {
  local app="$1" project="$2" redirect="$3" logout="$4" pid aid resp cid sec
  pid="$(zapi POST /management/v1/projects/_search \
    "$(jq -n --arg n "$project" '{queries:[{nameQuery:{name:$n,method:"TEXT_QUERY_METHOD_EQUALS"}}]}')" |
    jq -r '.result[0].id // empty')"
  if [ -z "$pid" ]; then
    pid="$(zapi POST /management/v1/projects "$(jq -n --arg n "$project" '{name:$n}')" | jq -r '.id')"
  fi
  [ -n "$pid" ] && [ "$pid" != null ] || die "could not create Zitadel project $project"
  aid="$(zapi POST "/management/v1/projects/$pid/apps/_search" '{}' |
    jq -r --arg n "$app" '.result[]? | select(.name==$n) | .id // empty' | head -1)"
  # The client secret is only returned at creation, so an app we lost the secret for is recreated.
  [ -n "$aid" ] && zapi DELETE "/management/v1/projects/$pid/apps/$aid" >/dev/null
  resp="$(zapi POST "/management/v1/projects/$pid/apps/oidc" "$(jq -n \
    --arg name "$app" --arg redirect "$redirect" --arg logout "$logout" '{
      name:$name, redirectUris:[$redirect],
      responseTypes:["OIDC_RESPONSE_TYPE_CODE"],
      grantTypes:["OIDC_GRANT_TYPE_AUTHORIZATION_CODE","OIDC_GRANT_TYPE_REFRESH_TOKEN"],
      appType:"OIDC_APP_TYPE_WEB", authMethodType:"OIDC_AUTH_METHOD_TYPE_BASIC",
      postLogoutRedirectUris:[$logout], devMode:false,
      accessTokenType:"OIDC_TOKEN_TYPE_BEARER", accessTokenRoleAssertion:true,
      idTokenRoleAssertion:true, idTokenUserinfoAssertion:true, clockSkew:"1s", additionalOrigins:[]}')")"
  cid="$(jq -r '.clientId // empty' <<<"$resp")"; sec="$(jq -r '.clientSecret // empty' <<<"$resp")"
  [ -n "$cid" ] || die "Zitadel did not return a client id for $app: $resp"
  printf '%s %s' "$cid" "$sec"
}

provision_oidc() {
  if [ -n "$(get_env ADRIENT_OIDC_CLIENT_ID)" ] && [ -n "$(get_env ADRIENT_OIDC_CLIENT_SECRET)" ]; then
    log "OIDC app already provisioned (keeping existing credentials)"
    return 0
  fi
  log "provisioning the single sign-on app in Zitadel"
  local out cid sec
  out="$(create_oidc_app adrient-workspace-backend adrient-workspace \
    "https://$WORKSPACE_HOST/api/v1/auth/callback" "https://$WORKSPACE_HOST/")"
  cid="${out%% *}"; sec="${out#* }"
  put_env ADRIENT_OIDC_CLIENT_ID "$cid"
  put_env ADRIENT_OIDC_CLIENT_SECRET "$sec"
  # Service-account PAT enables the in-app account/security panel and password form.
  put_env ADRIENT_ZITADEL_SERVICE_TOKEN "$PAT"
}

provision_twenty() {
  is_true "$(get_env WITH_CRM_ENABLED)" || return 0
  local f="$INSTALL_DIR/twenty-oidc.env"
  [ -s "$f" ] && return 0
  log "creating the CRM single sign-on app in Zitadel"
  local out
  out="$(create_oidc_app twenty-crm twenty \
    "https://$CRM_HOST/auth/oidc/callback" "https://$CRM_HOST/")"
  (umask 077; printf 'TWENTY_OIDC_ISSUER=https://%s\nTWENTY_OIDC_CLIENT_ID=%s\nTWENTY_OIDC_CLIENT_SECRET=%s\n' \
    "$AUTH_HOST" "${out%% *}" "${out#* }" >"$f")
}

provision_forgejo() {
  is_true "$(get_env WITH_GIT_ENABLED)" || return 0
  [ -n "$(get_env ADRIENT_FORGEJO_ADMIN_TOKEN)" ] && return 0
  log "creating the Forgejo admin account for org provisioning"
  local i
  for i in $(seq 1 40); do
    dc exec -T -u git forgejo forgejo admin user list >/dev/null 2>&1 && break
    sleep 3
  done
  dc exec -T -u git forgejo forgejo admin user create --admin --username adrient-admin \
    --email "adrient-admin@$DOMAIN" --random-password --must-change-password=false >/dev/null 2>&1 || true
  local tok
  tok="$(dc exec -T -u git forgejo forgejo admin user generate-access-token \
    --username adrient-admin --token-name adrient-provisioner --scopes all --raw 2>/dev/null | tr -d '[:space:]' || true)"
  if [ -n "$tok" ]; then put_env ADRIENT_FORGEJO_ADMIN_TOKEN "$tok"
  else warn "could not mint a Forgejo admin key; /git works, only org auto-provisioning is off"; fi
}

# ------------------------------------------------------------------- health --
wait_healthy() { # service timeout_s
  local s="$1" t="${2:-300}" i=0 st cid
  while [ "$i" -lt "$t" ]; do
    cid="$(dc ps -q "$s" 2>/dev/null | head -1)"
    if [ -n "$cid" ]; then
      st="$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$cid" 2>/dev/null || true)"
      case "$st" in healthy|running) return 0 ;; esac
    fi
    sleep 3; i=$((i + 3))
  done
  return 1
}

# ------------------------------------------------------------------ install --
cmd_install() {
  need_root
  ENV_FILE="$INSTALL_DIR/.env"
  local upgrading=0; [ -f "$ENV_FILE" ] && upgrading=1
  [ "$upgrading" = 1 ] && log "existing install found at $INSTALL_DIR: upgrading (keeping .env and data)"

  preflight
  install_packages
  install_docker

  if [ "$upgrading" = 0 ]; then
    cat >&2 <<'EOF'

  Adrient Workspace needs a domain you control. Hosts used:
    workspace.<domain>  the app        auth.<domain>  sign-in (Zitadel)
    crm.<domain>        optional CRM (if enabled)
EOF
  fi
  [ -n "$DOMAIN" ] || [ -z "$(get_env ADRIENT_DOMAIN 2>/dev/null)" ] || DOMAIN="$(get_env ADRIENT_DOMAIN)"
  ask DOMAIN "Base domain (e.g. example.com)" "" 1
  DOMAIN="$(printf '%s' "$DOMAIN" | tr 'A-Z' 'a-z' | sed 's#^https\?://##; s#/.*$##; s#^workspace\.##')"
  printf '%s' "$DOMAIN" | grep -Eq '^([a-z0-9]([a-z0-9-]*[a-z0-9])?\.)+[a-z]{2,}$' || die "invalid domain: $DOMAIN"
  [ -n "$ACME_EMAIL" ] || ACME_EMAIL="$(get_env ACME_EMAIL 2>/dev/null || true)"
  ask ACME_EMAIL "Email for Let's Encrypt certificate notices" "" 1
  [ -n "$ADMIN_EMAIL" ] || ADMIN_EMAIL="$(get_env ADRIENT_ADMIN_EMAIL 2>/dev/null || true)"
  ask ADMIN_EMAIL "Admin login email" "$ACME_EMAIL" 1
  printf '%s' "$ACME_EMAIL" | grep -Eq '^[^@ ]+@[^@ ]+\.[^@ ]+$' || die "invalid email: $ACME_EMAIL"
  printf '%s' "$ADMIN_EMAIL" | grep -Eq '^[^@ ]+@[^@ ]+\.[^@ ]+$' || die "invalid email: $ADMIN_EMAIL"
  if [ -z "$WITH_CRM" ] && ! is_true "$NONINTERACTIVE"; then
    confirm "Install the CRM (Twenty) at crm.$DOMAIN? Adds ~1.5 GB RAM" n && WITH_CRM=1 || WITH_CRM=0
  fi
  if [ -z "$WITH_GIT" ] && ! is_true "$NONINTERACTIVE"; then
    confirm "Install git hosting (Forgejo) at workspace.$DOMAIN/git?" n && WITH_GIT=1 || WITH_GIT=0
  fi

  mkdir -p "$INSTALL_DIR"
  prepare_source
  write_env
  sync_files
  check_dns

  if [ -n "$(get_env ADRIENT_IMAGE)" ]; then
    log "pulling $(get_env ADRIENT_IMAGE)"
    dc pull adrient
  else
    log "building the app image (first build takes 5-15 minutes)"
    local ver commit
    ver="$(git -C "$SRC_DIR" describe --tags --always 2>/dev/null || echo dev)"
    commit="$(git -C "$SRC_DIR" rev-parse --short HEAD 2>/dev/null || echo unknown)"
    dc build --build-arg "VERSION=$ver" --build-arg "COMMIT=$commit" adrient
  fi

  log "starting database, storage, sign-in and the HTTPS front"
  dc up -d postgres rustfs bucket-init zitadel caddy
  wait_zitadel
  provision_oidc
  if is_true "$(get_env WITH_GIT_ENABLED)"; then dc up -d forgejo; provision_forgejo; fi
  provision_twenty

  log "starting the application"
  dc up -d
  wait_healthy adrient 300 || { dc logs --tail 60 adrient >&2 || true; die "the app did not become healthy (install.sh logs adrient)"; }

  print_summary "$upgrading"
}

print_summary() {
  local upgrading="$1"
  local ok="not reachable yet"
  curl -fsS -o /dev/null --max-time 10 "https://$WORKSPACE_HOST/healthz" 2>/dev/null && ok="reachable"
  cat <<EOF

============================================================
 Adrient Workspace is $( [ "$upgrading" = 1 ] && echo upgraded || echo installed )
============================================================
 App:      https://$WORKSPACE_HOST   ($ok from here)
 Sign-in:  https://$AUTH_HOST
$(is_true "$(get_env WITH_CRM_ENABLED)" && echo " CRM:      https://$CRM_HOST   (finish SSO: see $INSTALL_DIR/twenty-oidc.env and docs/install-server.md)")
$(is_true "$(get_env WITH_GIT_ENABLED)" && echo " Git:      https://$WORKSPACE_HOST/git/")

 Admin login:    $(get_env ADRIENT_ADMIN_EMAIL)
 Admin password: $(get_env ZITADEL_ADMIN_PASSWORD)
 (change it after first sign-in; secrets live in $INSTALL_DIR/.env, mode 600)

 If the page does not load yet, DNS or certificates are still settling:
   workspace.$DOMAIN, auth.$DOMAIN, rtc.$DOMAIN$(is_true "$(get_env WITH_CRM_ENABLED)" && echo ", crm.$DOMAIN") must point to this server's IP,
   with ports 80, 443, 7881/tcp, 3478/udp and 50000-50999/udp open (Meet calls). Check: sudo $0 status
 Manage:  install.sh status | logs | backup | update | uninstall
============================================================
EOF
}

# -------------------------------------------------------------- operations --
cmd_status() {
  load_state
  dc ps
  echo
  echo "App:     https://$WORKSPACE_HOST"
  echo "Sign-in: https://$AUTH_HOST"
  [ -n "$CRM_HOST" ] && is_true "$(get_env WITH_CRM_ENABLED)" && echo "CRM:     https://$CRM_HOST"
  if curl -fsS -o /dev/null --max-time 10 "https://$WORKSPACE_HOST/healthz" 2>/dev/null; then echo "health:  ok"; else echo "health:  not reachable over HTTPS (check DNS, ports 80/443, 'logs caddy')"; fi
}

cmd_logs() { load_state; dc logs -f --tail 200 "${CMD_ARGS[@]}"; }

dbs_to_dump() { echo adrient zitadel pdf; }

cmd_backup() {
  need_root; load_state
  local ts out stage db
  ts="$(date +%Y%m%d-%H%M%S)"
  stage="$(mktemp -d "$INSTALL_DIR/backups/.stage.XXXXXX")"
  out="$INSTALL_DIR/backups/adrient-$ts.tar.gz"
  log "dumping databases"
  for db in $(dbs_to_dump); do
    dc exec -T postgres pg_dump -U adrient -Fc "$db" >"$stage/$db.dump"
  done
  if is_true "$(get_env WITH_CRM_ENABLED)"; then
    dc exec -T twenty-db pg_dump -U twenty -Fc default >"$stage/twenty.dump" || warn "CRM dump failed"
  fi
  cp "$ENV_FILE" "$stage/env"
  if [ "$DB_ONLY" = 1 ]; then
    tar -C "$stage" -czf "$out" .
  else
    log "archiving file storage (this can be large)"
    local paths=(data/rustfs data/zitadel)
    is_true "$(get_env WITH_GIT_ENABLED)" && paths+=(data/forgejo)
    is_true "$(get_env WITH_CRM_ENABLED)" && paths+=(data/twenty-storage)
    tar -czf "$out" -C "$stage" . -C "$INSTALL_DIR" "${paths[@]}"
  fi
  rm -rf "$stage"; chmod 600 "$out"
  log "backup written: $out ($(du -h "$out" | cut -f1)); contains secrets, store it off-server"
}

cmd_restore() {
  need_root; load_state
  local f="${CMD_ARGS[0]:-}"; [ -f "$f" ] || die "usage: install.sh restore /path/to/adrient-backup.tar.gz"
  confirm "Restore $f? This overwrites the current databases and files." n || die "aborted"
  local stage db; stage="$(mktemp -d "$INSTALL_DIR/backups/.restore.XXXXXX")"
  tar -xzf "$f" -C "$stage"
  dc stop adrient zitadel >/dev/null
  for db in $(dbs_to_dump); do
    [ -f "$stage/$db.dump" ] || continue
    log "restoring database $db"
    dc exec -T postgres pg_restore -U adrient -d "$db" --clean --if-exists --no-owner <"$stage/$db.dump" || warn "pg_restore reported issues for $db"
  done
  if [ -f "$stage/twenty.dump" ]; then
    dc exec -T twenty-db pg_restore -U twenty -d default --clean --if-exists --no-owner <"$stage/twenty.dump" || true
  fi
  if [ -d "$stage/data" ]; then
    dc stop rustfs >/dev/null
    cp -a "$stage/data/." "$INSTALL_DIR/data/"
  fi
  rm -rf "$stage"
  dc up -d
  log "restore finished; the .env inside the archive was not applied (kept the current one)"
}

cmd_update() {
  need_root; load_state
  log "backing up before the update (databases only)"
  DB_ONLY=1 cmd_backup
  # Re-run the installer non-interactively with the stored settings.
  NONINTERACTIVE=1; ASSUME_YES=1
  DOMAIN="$(get_env ADRIENT_DOMAIN)"; ACME_EMAIL="$(get_env ACME_EMAIL)"; ADMIN_EMAIL="$(get_env ADRIENT_ADMIN_EMAIL)"
  WITH_CRM="$(get_env WITH_CRM_ENABLED)"; WITH_GIT="$(get_env WITH_GIT_ENABLED)"
  SKIP_CHECKS=1
  cmd_install
}

cmd_uninstall() {
  need_root; load_state
  log "this stops and removes all Adrient Workspace containers"
  confirm "Stop and remove the containers in $INSTALL_DIR?" n || die "aborted"
  dc --profile crm --profile git down --remove-orphans || true
  docker image rm adrient-workspace:local >/dev/null 2>&1 || true
  local del=0
  if [ "$PURGE" = 1 ]; then del=1
  elif ! is_true "$NONINTERACTIVE"; then
    printf '\nDelete ALL data (databases, uploaded files, certificates, .env) in %s?\n' "$INSTALL_DIR" >&2
    local ans=""; tty_read ans "Type DELETE to erase everything, anything else keeps it: " "" || true
    [ "$ans" = DELETE ] && del=1
  fi
  if [ "$del" = 1 ]; then
    case "$INSTALL_DIR" in ""|/|/opt|/usr|/home|/root|/var) die "refusing to delete $INSTALL_DIR" ;; esac
    [ -f "$INSTALL_DIR/compose.yaml" ] || die "$INSTALL_DIR does not look like an Adrient install; not deleting"
    rm -rf "$INSTALL_DIR"
    log "all data deleted. Docker itself was left installed."
  else
    log "containers removed; data kept in $INSTALL_DIR (re-run install.sh to bring it back)"
  fi
}

# --------------------------------------------------------------------- main --
main() {
  parse_args "$@"
  case "$CMD" in
    install)   cmd_install ;;
    status)    cmd_status ;;
    logs)      cmd_logs ;;
    backup)    cmd_backup ;;
    restore)   cmd_restore ;;
    update)    cmd_update ;;
    uninstall) cmd_uninstall ;;
    *)         usage; exit 1 ;;
  esac
}
main "$@"
