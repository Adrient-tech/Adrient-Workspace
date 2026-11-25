# Install Adrient Workspace on your own server

One script installs the whole suite on a fresh Linux server: Docker, Postgres,
Zitadel (sign-in), RustFS (file storage), the app, and Caddy for automatic HTTPS.
Optional: Twenty CRM and Forgejo git hosting.

## Requirements

| Item | Minimum | Recommended |
|---|---|---|
| OS | Ubuntu 22.04 / 24.04, Debian 12 (RHEL-family: best effort) | same |
| CPU | 2 cores | 4 cores |
| RAM | 4 GB | 8 GB (add ~1.5 GB for the CRM) |
| Disk | 15 GB free | 40 GB+ (files live on this disk) |
| Network | Public IPv4, ports 80 and 443 free | plus AAAA if you have IPv6 |
| Domain | One you control | |

The first image build needs 2 to 3 GB of RAM. On a 4 GB server add swap if the
build is killed.

## One-line install

```sh
curl -fsSL https://raw.githubusercontent.com/Adrient-tech/Adrient-Workspace/main/install.sh | sudo bash
```

Or from a checkout:

```sh
git clone https://github.com/Adrient-tech/Adrient-Workspace.git
cd Adrient-Workspace
sudo ./install.sh
```

Unattended:

```sh
sudo ./install.sh --non-interactive --domain example.com --email you@example.com --with-crm --with-git
```

When run through `curl | bash` the installer clones the source into
`/opt/adrient-workspace/src` (override the URL with `ADRIENT_SOURCE_URL`). When
run from a checkout it builds that checkout. Plan on 5 to 15 minutes for the
first build.

At the end it prints the URLs and the admin email and password. Secrets live in
`/opt/adrient-workspace/.env` (mode 600). Change the admin password after your
first sign-in.

## Prompts and flags

Every prompt has a flag and an environment variable. Flags win over the
environment; both skip the prompt.

| Prompt | Flag | Environment | Default |
|---|---|---|---|
| Base domain | `--domain example.com` | `ADRIENT_DOMAIN` | none (required) |
| Let's Encrypt email | `--email you@example.com` | `ADRIENT_ACME_EMAIL` | none (required) |
| Admin login email | `--admin-email a@example.com` | `ADRIENT_ADMIN_EMAIL` | the Let's Encrypt email |
| Install CRM | `--with-crm` | `ADRIENT_WITH_CRM=1` | no |
| Install git hosting | `--with-git` | `ADRIENT_WITH_GIT=1` | no |
| (no prompt) | `--non-interactive`, `-y` | `ADRIENT_NON_INTERACTIVE=1` | off |
| (no prompt) | `--dir PATH` | `ADRIENT_INSTALL_DIR` | `/opt/adrient-workspace` |
| (no prompt) | `--source-url URL` | `ADRIENT_SOURCE_URL` | `https://github.com/Adrient-tech/Adrient-Workspace.git` |
| (no prompt) | `--ref REF` | `ADRIENT_SOURCE_REF` | repository default branch |
| (no prompt) | `--image IMAGE` | `ADRIENT_IMAGE` | build locally instead of pulling |
| (no prompt) | `--staging` | `ADRIENT_ACME_STAGING=1` | real Let's Encrypt |
| (no prompt) | `--skip-checks` | `ADRIENT_SKIP_CHECKS=1` | checks enforced |

With `--non-interactive`, missing required values are an error.

## Domain mapping

Given the base domain `example.com`, the installer serves:

| DNS name | Type | Value | Serves | Needed |
|---|---|---|---|---|
| `workspace.example.com` | A (and AAAA) | server public IP | the apps; git at `/git` | always |
| `auth.example.com` | A (and AAAA) | server public IP | Zitadel sign-in | always |
| `crm.example.com` | A (and AAAA) | server public IP | Twenty CRM | with `--with-crm` |
| `rtc.example.com` | A (and AAAA) | server public IP | Meet media server (LiveKit) signalling | always |

A single wildcard record `*.example.com` -> server IP covers all three. Create
the records before or right after installing; Caddy retries certificate
issuance until the names resolve. Check with `dig +short workspace.example.com`.

The base domain itself (`example.com`) is not used, so your existing website
keeps working.

### Cloudflare

- First install: set the three records to **DNS only** (grey cloud). Let's
  Encrypt must reach port 80/443 on your server to issue certificates.
- After the certificates are issued you may switch to **Proxied** (orange cloud).
  Set SSL/TLS mode to **Full (strict)**. Never use "Flexible": it causes
  redirect loops.
- Cloudflare's free plan limits uploads to 100 MB per request through the
  proxy. For large Drive/Video uploads use DNS only for `workspace`.
- Cloudflare proxies only some ports; 80 and 443 are the ones used here.
- `rtc.example.com` must stay **DNS only**: Meet media (UDP) goes straight to
  the server IP and cannot pass through the Cloudflare proxy.

### Firewall

Allow SSH, HTTP/HTTPS and the Meet media ports. Docker publishes ports by
editing iptables directly, so `ufw` rules do not hide published ports; that is
fine here because only Caddy (80/443) publishes ports. Zitadel also listens on
`127.0.0.1:18081`, reachable only from the server itself.

Meet video calls run through a self-hosted [LiveKit](https://livekit.io) SFU
(Apache-2.0) with its built-in TURN server. It uses host networking (media
needs the real IP), so these ports are opened by `ufw` itself:

| Port | Protocol | Purpose |
|---|---|---|
| 7881 | TCP | ICE over TCP, for networks that block UDP |
| 50000-50999 | UDP | media (about one port per participant) |
| 3478 | UDP | built-in TURN, for strict NATs |
| 5349 | TCP | TURN over TLS (optional, see below) |
| 7880 | TCP | signalling; **do not open**, Caddy proxies it as `wss://rtc.<domain>` |

```sh
sudo ufw allow 22/tcp
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw allow 443/udp           # HTTP/3, optional
sudo ufw allow 7881/tcp          # Meet: ICE/TCP
sudo ufw allow 3478/udp          # Meet: TURN
sudo ufw allow 50000:50999/udp   # Meet: media
sudo ufw deny 7880/tcp           # Meet signalling stays behind Caddy
sudo ufw enable
```

Cloud firewalls (AWS security groups, Hetzner, DigitalOcean) need the same
rules. The LiveKit config with the generated API key/secret is written to
`/opt/adrient-workspace/livekit/livekit.yaml` (mode 600) and the key pair is
stored in `.env` as `LIVEKIT_API_KEY` / `LIVEKIT_API_SECRET`; neither is ever
committed. TURN over TLS (port 5349, for networks that allow only TLS) needs a
certificate for `turn.<domain>`: add `domain`, `tls_port: 5349`, `cert_file`
and `key_file` under `turn:` in that file, mount the certificate, and run
`install.sh update`.

Meet recording uses LiveKit Egress and is off by default (it needs an extra
~2 GB RAM container). To enable it run `livekit/egress` next to the stack and
set `ADRIENT_LIVEKIT_EGRESS=1` plus `ADRIENT_LIVEKIT_EGRESS_S3_ENDPOINT`,
`_BUCKET`, `_ACCESS_KEY`, `_SECRET_KEY` (RustFS). The host then gets a Record
button in the People panel; files land under `meet-recordings/<org>/<room>/`.

## What gets installed

| Service | Role | Exposed |
|---|---|---|
| `caddy` | HTTPS front, Let's Encrypt | 80, 443 |
| `livekit` | Meet SFU + TURN (host network) | 7881/tcp, 3478/udp, 50000-50999/udp |
| `adrient` | the app (built from this repo's `Dockerfile`) | internal |
| `postgres` | databases `adrient`, `zitadel`, `pdf` | internal |
| `zitadel` | OIDC sign-in | internal (+ 127.0.0.1:18081) |
| `rustfs` + `bucket-init` | S3 storage and the buckets `adrient-default`, `pdf-docs` | internal |
| `twenty-*` (profile `crm`) | CRM | internal, via `crm.` host |
| `forgejo` (profile `git`) | git hosting | internal, via `workspace/git` |

Files in `/opt/adrient-workspace`: `.env`, `compose.yaml`, `Caddyfile`,
`sites/`, `data/` (all persistent state), `backups/`, `src/` (when cloned),
and a copy of `install.sh`.

The installer provisions the single sign-on app in Zitadel, creates the first
admin (your admin email, password in the summary) and makes that email an
Adrient admin (`ADRIENT_ADMIN_EMAILS`).

### CRM sign-in (one manual step)

Twenty configures sign-in inside its own UI. The installer creates the Zitadel
app and writes the values to `/opt/adrient-workspace/twenty-oidc.env`. Open
`https://crm.example.com`, create the first workspace, then go to
**Settings -> Security -> SSO -> New (OIDC)** and paste the issuer, client id
and client secret.

### Email (SMTP) - optional

Sign-in works without email. To let Zitadel send verification and
password-reset mail, open `https://auth.example.com/ui/console` (sign in as the
admin) and set **Settings -> SMTP**. For invitation mail from the app set
`RESEND_API_KEY` and `ADRIENT_EMAIL_FROM` in `.env`, then
`sudo ./install.sh update`.

### Mail domains, DNS records and name servers

**Admin -> Domains -> (verified domain) -> Domain setup** lists every mail DNS
record for that domain, with copy buttons and a live check against public
resolvers (1.1.1.1, 8.8.8.8). The wizard reads DNS only; it never changes it.

**Records the customer creates at the registrar / DNS host** for the base
domain `example.com` (the installer's four hosts first):

| Name | Type | Value | Purpose |
|---|---|---|---|
| `workspace.example.com` | A / AAAA | server public IP | apps, autoconfig, MTA-STS policy, DAV |
| `auth.example.com` | A / AAAA | server public IP | Zitadel sign-in |
| `rtc.example.com` | A / AAAA | server public IP, **DNS only** | Meet media |
| `mail.example.com` | A / AAAA | IP of the mail server (mailcow) | MX target, IMAP/SMTP host |
| `example.com` | MX | `10 mail.example.com.` | inbound mail |
| `example.com` | TXT | `v=spf1 mx -all` (add `ip4:` of the MTA if it differs) | SPF |
| `<selector>._domainkey.example.com` | TXT | `v=DKIM1; k=rsa; p=...` | DKIM, key from the wizard |
| `_dmarc.example.com` | TXT | `v=DMARC1; p=none; rua=mailto:dmarc@example.com` | DMARC |
| `_mta-sts.example.com`, `_smtp._tls.example.com` | TXT | see wizard | MTA-STS, TLS reports |
| `mta-sts.`, `autoconfig.`, `autodiscover.` + domain | CNAME | `workspace.example.com.` | policy file and client auto-setup |
| `_imaps._tcp`, `_submission._tcp`, `_submissions._tcp`, `_caldavs._tcp`, `_carddavs._tcp` | SRV | see wizard | client discovery |

Also set reverse DNS (PTR) of the mail server IP to `mail.example.com`; that is
done at the hosting provider. DMARC starts at `p=none`; after 14 days in which
SPF, DKIM and DMARC all pass, the wizard suggests `p=quarantine`.

**Name servers.** By default the customer keeps their own DNS provider and
creates the records above; no name servers are needed from us. Running
authoritative name servers (PowerDNS Authoritative with the Postgres backend,
`ns1.`/`ns2.` hosts, glue records and DS records at the registrar) is an
optional later phase and is not installed today.

**Mail backend.** `ADRIENT_MAIL_BACKEND=local` (default) cannot deliver to the
internet. `imap` or `mailcow` uses the IMAP/SMTP bridge (`ADRIENT_MAIL_*`).
Adrient does not provision mailcow domains or mailboxes yet. For DKIM, create
the domain in mailcow yourself, then in the wizard generate the key and call
`GET /api/v1/admin/domains/<id>/dkim/export` (admin only, audited). It returns
the private key and the steps: mailcow **Configuration -> ARC/DKIM keys ->
Import** with that selector and key, publish the TXT record, wait for the
green check, click **Activate signing**. Private keys are stored AES-256-GCM
encrypted with a key derived from `ADRIENT_SECRETS_KEY` (falls back to
`ADRIENT_OIDC_CLIENT_SECRET`); losing that secret makes stored keys unreadable.

The `mta-sts.`, `autoconfig.` and `autodiscover.` hostnames must be served over
HTTPS by your reverse proxy with a valid certificate (add them to the Caddy
site list for the app host); the app answers them by Host header.

Optional env: `ADRIENT_DNS_WEB_HOST` (host that serves autoconfig/MTA-STS;
default is the request host), `ADRIENT_DNS_SPF_INCLUDE` (extra SPF include).

## Day-2 commands

```sh
sudo /opt/adrient-workspace/install.sh status
sudo /opt/adrient-workspace/install.sh logs [service]     # e.g. logs adrient
sudo /opt/adrient-workspace/install.sh backup [--db-only]
sudo /opt/adrient-workspace/install.sh restore /opt/adrient-workspace/backups/adrient-<time>.tar.gz
sudo /opt/adrient-workspace/install.sh update
sudo /opt/adrient-workspace/install.sh uninstall
```

### Backups and restore

`backup` writes `/opt/adrient-workspace/backups/adrient-<time>.tar.gz` with a
`pg_dump` of every database, the `.env`, and (unless `--db-only`) the file
storage, Zitadel bootstrap data, and CRM/git data. The archive contains secrets;
copy it off the server. Schedule it:

```sh
echo '17 3 * * * root /opt/adrient-workspace/install.sh backup' | sudo tee /etc/cron.d/adrient-backup
```

`restore` stops the app and Zitadel, restores the databases and files from the
archive, and starts everything again. It keeps the current `.env`. To move to a
new server: install there with the same domain, then `restore` the archive,
and copy the old `.env` over the new one before restoring (the Zitadel master
key and database passwords must match the data).

### Upgrade

`sudo /opt/adrient-workspace/install.sh update` takes a database backup,
pulls the latest source, rebuilds the image and restarts. Re-running
`sudo ./install.sh` does the same without the automatic backup. The `.env` and
`data/` are never touched. Database migrations run on app start. When you built
from your own checkout, `git pull` there first.

### Uninstall

`sudo /opt/adrient-workspace/install.sh uninstall` stops and removes the
containers, then asks you to type `DELETE` before it erases `/opt/adrient-workspace`
(databases, files, certificates, secrets). Answer anything else to keep the
data. Docker itself stays installed. Use `--purge` to skip the question.

## Troubleshooting

| Symptom | Cause and fix |
|---|---|
| Installer stops at checks | Fix the item shown (RAM, disk, port 80/443 taken by nginx/apache) or pass `--skip-checks`. |
| Browser shows a certificate error | DNS does not point at the server yet, port 80/443 blocked, or Let's Encrypt rate limit. `install.sh logs caddy`. Test the flow with `--staging` first. |
| Cloudflare 521/525/526 or redirect loop | Use DNS only until the certificate exists; then SSL mode Full (strict). |
| Sign-in loops or "invalid redirect" | `workspace.` and `auth.` must both resolve to the server; the redirect URL is `https://workspace.<domain>/api/v1/auth/callback`. |
| `adrient` container restarts | `install.sh logs adrient`. Usually `auth.<domain>` is not reachable yet; it recovers once DNS and the certificate work. |
| Image build killed | Out of memory. Add swap (`fallocate -l 4G /swapfile && chmod 600 /swapfile && mkswap /swapfile && swapon /swapfile`) and re-run. |
| Zitadel takes minutes on first start | Normal (database setup). `install.sh logs zitadel`. |
| Forgot the admin password | In `/opt/adrient-workspace/.env` as `ZITADEL_ADMIN_PASSWORD` (the initial value). If you changed it, reset it in the Zitadel console. |
| Reinstalling on the same domain after `--purge` | Safe: a purge removes all state, a new install starts clean. |
| Anything else | Send `install.sh status` and the last lines of `install.sh logs` to [Adrient support](https://adrient.com/support). |
