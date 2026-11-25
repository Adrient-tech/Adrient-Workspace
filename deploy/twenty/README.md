# Twenty CRM in Adrient Workspace

Twenty (twentyhq/twenty, AGPL-3.0) runs unmodified as its own containers
(`twenty-server`, `twenty-worker`, `twenty-db`, `twenty-redis` in
`deploy/local/compose.yaml`). The Adrient backend proxies every request whose
Host is `ADRIENT_CRM_HOST` (local: `crm.workspace.localtest.me:8080`) to
`ADRIENT_CRM_URL` (local: `http://127.0.0.1:3010`).

## Branding

Twenty has no env for the product name or logo. The workspace name and logo
are workspace data, set once at onboarding:

1. Open http://crm.workspace.localtest.me:8080/ and sign up / sign in.
2. On "Create your workspace" upload `web/app/public/adrient-mark.png` as the
   logo and enter **Adrient CRM** as the name.
   Later: Settings -> Workspace -> General.

Twenty's own UI strings (login page "Welcome to your workspace", tab
titles) stay as upstream ships them; changing them needs a fork, which we
do not do.

## Single sign-on

`deploy/zitadel/create-twenty-oidc-app.sh` creates the Zitadel OIDC client
(redirect `http://crm.workspace.localtest.me:8080/auth/oidc/callback`) and
writes `deploy/zitadel/data/twenty-oidc.env`. The `twenty-oidc-bridge`
sidecar makes the issuer `http://localhost:8081` reachable from inside the
Twenty container.

Twenty's OIDC/SAML SSO (Settings -> Workspace -> Security -> "Add SSO
Identity Provider") is an **Enterprise** feature: the button stays disabled
until Twenty validates a signed `ENTERPRISE_KEY` from Twenty's licensing
service. Without that key, CRM users sign in with Twenty's own
email/password. With a key: set `ENTERPRISE_KEY` on `twenty-server` and
`twenty-worker`, then add an OIDC provider with the values from
`twenty-oidc.env` (issuer `http://localhost:8081`).
