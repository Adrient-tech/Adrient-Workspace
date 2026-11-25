# Identity and access operations (IT runbook)

Console: **Admin > Security operations** (`/admin/secops`). Each person also has **My security** (`/security`).
Code: `internal/iamops`, migrations 0370-0374, API under `/api/v1/admin/iam` and `/api/v1/iam/my`.

## Roles

| Role | Can do |
| --- | --- |
| Help desk | Reset password (link, temporary password, expire), unlock, reset or revoke second steps, sign people out, view sessions and sign-ins. Cannot touch administrators, policies or roles. |
| Security analyst | Read sign-ins and alerts, triage, contain an account, quarantine, sign out, read reports and the action log. No policy edits. |
| Identity admin | Everything, including settings, allow/block lists, delegation, MFA bypass, approvals. Organisation admins hold this role automatically. |
| Auditor | Read only: sign-ins, alerts, reports, action log. |

Older delegated "Help desk" and "Read-only admin" assignments count as Help desk and Auditor.
Only a super admin (owner or `ADRIENT_ADMIN_EMAILS`) can act on another administrator or make someone an identity admin.

## Safety rules the server enforces

- Every action needs a reason (3+ characters), takes an optional ticket id, is written to the action log (`iam_actions`) and the audit log, and notifies the person (in-app and e-mail). A security mailbox and the manager can be copied.
- Sensitive actions need a **step-up**: the admin re-enters their password (and an authenticator code if they have one). It is checked at the identity provider and valid for 10 minutes. Without it the API answers 428.
- Optional **two-person approval** per action (Detection and policy). The approver must be a different identity admin.
- Nobody can lock themselves out or remove the **last active administrator** (suspend, quarantine, contain, expiry are refused).
- At most 30 actions per admin in 10 minutes.
- A temporary password or bypass code is shown once in the dialog. It is never stored in clear and never logged.

## Runbooks

**Suspicious sign-in.** Alerts tab, open the alert, read the evidence and the 48-hour timeline. Take it (status Investigating). If the person confirms it was not them: **Contain account** (Preview first). It signs the person out everywhere, sets an unknown password and e-mails a reset link, removes second steps and notifies them. It is idempotent: run it twice and the password is not reset again until the person has signed in. Then resolve as Compromised. If the address is the problem, add it to **Allow and block**. If it is a false alarm, resolve as False positive.

**"This wasn't me" from a user.** The person's report opens a High alert and signs out their other sessions at once. Contact them, then contain.

**Lost phone / lost authenticator.** People tab, open the person, Second step: **Reset all second steps** (or Revoke the one factor). They enrol again at their next sign-in. For a short exception use **Time-limited bypass**: it removes the second steps for 1-24 hours and gives you a one-time code to read to the person to confirm identity. Enable approval for this action if you want two people.

**Password reset.** *Send reset link* is the default. Use *temporary password* when the person has no working e-mail: it is shown once, they must change it at sign-in. *Unlock* clears a lock after too many wrong attempts.

**Offboarding.** Use Admin > Offboarding. Suspend here only blocks sign-in and ends sessions; Offboarding also handles data, seats and holds.

**Break-glass.** If the last administrator is locked out: use an account listed in `ADRIENT_ADMIN_EMAILS` (a super admin, independent of org roles). If none exists, an operator with database access adds the person to `adrient.org_admins` and, for the identity provider, unlocks the user in the identity provider console. Record it in the ticket system. Keep one break-glass account with a strong password in a sealed store, and review its sign-ins (they raise alerts like any other).

## Detection rules

Impossible travel, new country, new device, failed bursts per account and per address, credential stuffing (many accounts from one address), off-hours, anonymising networks (list you maintain), second-step fatigue, administrator signing in from a new place, attempts on disabled accounts, honeypot hits, blocked-list hits. Thresholds, the off-hours window and the org clock offset are in Detection and policy. Alerts merge per rule and subject while open, carry an SLA by severity, and go to the notification centre, an optional security mailbox and an optional webhook.

## Limits you should know

- **Country and city** come from the `CF-IPCountry` header (set by a CDN or reverse proxy) on password sign-ins, and from events posted to `/events/ingest`. Sign-ins through the identity provider's hosted page (SSO callback) arrive without a country, so country-based rules do not fire for them. Address-based rules do. Impossible travel is country-to-country, not distance based.
- **Second-step events** (denied prompts) are not visible to this app for sign-ins done on the identity provider's own page. The fatigue rule runs on events with an `mfa_result` that you post to `/events/ingest` (for example from an identity provider action).
- **Second-step policy**: "Everyone" is enforced by the identity provider's login policy. "Administrators" and "Risky sign-ins" only alert and report.
- **Require re-enrolment** relies on the login policy forcing a second step; the app cannot make the hosted login page prompt one person only.
- Bypass codes are a help-desk verification aid. The bypass itself works by removing the factors; there is no automatic re-enforcement at the end of the window (the person is expected to enrol; the report of privileged accounts without a second step lists anyone who did not).
- Quarantine is enforced by the app server (writes refused for the account). It does not stop access through API tokens, so revoke them as well.
- Adding a security key or passkey for yourself is done at the identity provider's sign-in page; the My security page adds and removes authenticator apps and removes any factor.

## Ingest API (scripts and identity provider actions)

`POST /api/v1/admin/iam/events/ingest` with `{"events":[{"email","ip","user_agent","method","success","country","city","mfa_result","reason","at"}]}`. Needs an identity admin session. Returns the stored event ids and the findings.
