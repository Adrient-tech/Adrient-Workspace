# Security policy

## Reporting a vulnerability

Please report security problems **privately** through
[adrient.com/support](https://adrient.com/support) with "Security" in the subject, not in
a public issue or pull request. Include the affected version, how to reproduce,
and the impact you see. We acknowledge reports within 3 working days and keep
you informed until a fix ships. We credit reporters in the CHANGELOG unless you
ask us not to.

Please do not run tests against instances you do not own (including
workspace.adrient.com) beyond what is needed to demonstrate the issue, and do
not access other people's data.

## Supported versions

Security fixes go into `main` and the next release. Self-hosters should stay on
the latest release; see [docs/upgrade.md](docs/upgrade.md).

## How Adrient protects data (summary)

- All data is scoped to an organisation (`org_id`) on every query; queries are
  parameterised.
- Credentials stored for migrations (OAuth tokens, app passwords, OAuth client
  secrets) and DKIM keys are encrypted at rest with AES-256-GCM using
  `ADRIENT_SECRETS_KEY`. Keep that key out of backups you share and rotate it
  only together with re-entering those credentials.
- Migration connections to private network addresses are refused unless an
  administrator sets `ADRIENT_IMPORT_ALLOW_PRIVATE_HOSTS=1`, to prevent
  server-side request forgery. IMAP without TLS is only allowed to loopback.
- Backups made with `adrient-admin backup --config` contain your secrets: store
  them encrypted.
