# Per-Org Admin Roles (RBAC)

adrient-workspace authorizes admin actions per organization. This replaces the
earlier email-allowlist model whose empty-allowlist fallback treated **every**
authenticated member as an admin — a security hole (any logged-in user could
reset others' Zitadel passwords, view the audit log, etc.).

## Authorization model

A caller is an **admin of their org** iff **either**:

- **(a)** their email is in `ADRIENT_ADMIN_EMAILS` (comma-separated) — these are
  _bootstrap super-admins_, configured out-of-band on the server; **or**
- **(b)** a row exists in `adrient.org_admins` for `(caller.org_id,
caller.user_id)` — an in-app-assignable, per-org admin grant.

There is **no open fallback**. An empty `ADRIENT_ADMIN_EMAILS` no longer grants
anyone admin; admin access then comes solely from `org_admins` grants (which are
seeded by the first-admin auto-bootstrap below).

### Where the rule is enforced

The rule lives in three decoupled handlers/services. Each is kept free of the
generated protobuf (`gen/`) by receiving an injected `AdminChecker` closure that
`internal/server/server.go` builds from the `org_admins` repo + the auth context
(mirroring the existing `EmailResolver` / audit-recorder wiring):

| Surface                                                  | File                                                      | Check                       |
| -------------------------------------------------------- | --------------------------------------------------------- | --------------------------- |
| Admin user-management API (`/api/v1/admin/users*`)       | `internal/adminusers/handler.go` (`authorize`, `IsAdmin`) | allowlist OR `AdminChecker` |
| Audit-log viewer (`/api/v1/admin/audit`)                 | `internal/audit/handler.go` (`ServeHTTP`)                 | allowlist OR `AdminChecker` |
| Service-settings RPC (`AdminService.SetServiceSettings`) | `internal/admin/service.go` (`requireAdmin`)              | allowlist OR `AdminChecker` |

The `org_admins` data layer is `internal/orgadmin/repository.go`
(`IsAdmin`, `ListAdmins`, `GrantAdmin`, `RevokeAdmin`, `CountAdmins`,
`EnsureFirstAdmin`, `AdminUserIDsForZitadel`). Schema: migration
`internal/storage/migrations/0041_org_admins.sql`.

## First-admin auto-bootstrap

When a user becomes the **first member of an org that has no admins yet**, they
are automatically granted org-admin. This guarantees a freshly-provisioned org is
never left with nobody who can administer it, even when `ADRIENT_ADMIN_EMAILS` is
empty.

**Hook:** `internal/auth/service.go`, `Service.Callback` — immediately after
`users.UpsertByOIDC` (the point where a user is first seen for an org), the
service calls `FirstAdminBootstrapper.EnsureFirstAdmin(orgID, userID)`. That
method (`internal/orgadmin/repository.go`) atomically checks `CountAdmins==0`
under a row lock and inserts the grant only if so — safe against a concurrent
first sign-in. It is **best-effort**: a failure does not block sign-in.

`server.go` injects the bootstrapper via `authSvc.WithFirstAdminBootstrapper(cfg.OrgAdminRepo)`.

> Migration note: on an existing deployment, the first existing user to sign in
> after migration 0041 — when their org still has zero `org_admins` rows — will
> be bootstrapped as that org's admin. Seed `ADRIENT_ADMIN_EMAILS` if you need a
> specific super-admin regardless of sign-in order.

## Bootstrap via `ADRIENT_ADMIN_EMAILS`

`ADRIENT_ADMIN_EMAILS` is the break-glass / super-admin path. Any caller whose
email matches is an admin of **any** org they're in, independent of
`org_admins`. Use it to grant the initial human admin before anyone has signed
in, or to retain platform-operator access. It is checked first (cheap, no DB).

## Grant / revoke API

Admin-gated, under the admin-users handler (`internal/adminusers/handler.go`):

- `GET    /api/v1/admin/users` — each user now carries an `isAdmin` bool.
- `POST   /api/v1/admin/users/{id}/admin` — grant the org-admin role.
- `DELETE /api/v1/admin/users/{id}/admin` — revoke it; returns **409** if it
  would remove the org's **last** admin (last-admin protection).

`{id}` is the **Zitadel user id**. See the id-mapping note below.

Frontend: the Admin console "Users" page (`web/app/src/pages/admin/index.tsx`,
client `usersApi.ts`) shows an **Admin** badge + a grant/revoke toggle per user,
and surfaces the 409 as "You can't remove the last admin." The Users section is
its own route at **`/admin/users`** (each admin section maps to `/admin/:section`).

## Org creation gating

Only an admin may create a new org, and the creator becomes that org's first
admin. The primitive is `orgs.Repository.Create(slug, displayName,
creatorUserID)` (`internal/orgs/repository.go`), which inserts the org and the
creator's `org_admins` row in one transaction.

> **Gap:** there is currently **no `CreateOrg` RPC** in the gRPC surface — orgs
> are seeded by migration `0003_default_org.sql` for single-org mode. `Create`
> exists so the multi-org path is gated correctly the moment such an RPC is
> added; that handler MUST reject non-admin callers (via the same `AdminChecker`)
> before calling `Create`.

## Zitadel-user → adrient-user id mapping (the `isAdmin` join)

The admin-users API lists users straight from **Zitadel**, returning Zitadel user
ids. `org_admins`, however, references **adrient** user ids. They are joined via
`adrient.users.oidc_subject` = Zitadel user id, scoped by `(org_id, oidc_issuer)`
— the same identity adrient stamps when provisioning a user
(`internal/directory` does this lazily; `auth.Callback` on sign-in).

- **Listing** (`AdminUserIDsForZitadel`): joins Zitadel ids to adrient users to
  `org_admins` in one query.
- **Granting** (`orgAdminRoster.AdrientUserIDForZitadel` in `server.go`): resolves
  the Zitadel id to a adrient user id, **lazily provisioning** a minimal adrient row
  (empty email/display_name, enriched on the user's next sign-in) if the user has
  never signed into adrient — so an admin can grant the role pre-emptively.

> **Known limitation:** a Zitadel user who has never signed into adrient has no
> adrient row until granted or until the directory search provisions one, so they
> report `isAdmin=false` in the list until then. The grant flow handles this by
> provisioning on demand.

---
