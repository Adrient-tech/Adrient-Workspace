# Migrating from Google Workspace or Gmail

Open **Move to Adrient** (`/cloudimport`) and choose **Google Workspace / Gmail**.
There are two ways in, and they can be combined:

| Way | What it moves | Needs |
|---|---|---|
| **Connect a provider** (live, via Google APIs) | Drive (My Drive and shared drives, folder structure, per-user sharing), Gmail with labels, Calendar, Contacts, Google Chat (your own spaces), and for admins users and groups | An OAuth app you register in Google Cloud (below) |
| **Upload an export** (Google Takeout) | Drive files, Gmail `.mbox` with labels, Calendar `.ics`, Contacts `.vcf` | Nothing; works offline |

## What is migrated, and how

| Google | Adrient | Notes |
|---|---|---|
| Groups | Groups, members mapped by e-mail | Created once; re-runs do not duplicate. |
| My Drive | Drive, under `Imported from Google` | Google Docs, Sheets, Slides and Drawings are exported to `.docx`, `.xlsx`, `.pptx`, `.png`. Google Forms, Sites, My Maps and shortcuts have no export format and are listed in the error report. |
| Shared drives | A folder `<name> (shared drive)` | Content is imported for the person running the migration. |
| Sharing | Per-user grants (viewer, commenter, editor) | Only to people who have an Adrient account. Link sharing ("anyone with the link") is not copied; re-share in Adrient. |
| Calendar | Calendar | Calendars you own. Recurring events keep their RRULE. Calendars shared to you are left for their owners to migrate. Attendees and reminders are not copied. |
| Contacts | Contacts | Names, e-mails, phones, organisation, title, notes. |
| Google Chat | Chat, a channel `Imported: <space>` | Only your own spaces (Chat API limit). Messages keep the original author and time inside the text, because the import time becomes the message time. |

## Set up the OAuth app (admin, once)

1. In [Google Cloud console](https://console.cloud.google.com/), create a project.
2. Enable these APIs: Admin SDK API, Google Drive API, Google Calendar API,
   People API, Gmail API, Google Chat API.
3. **OAuth consent screen**: user type *Internal* (Workspace) or *External*
   (personal Gmail; you then need to add test users or verify the app).
4. **Credentials > Create credentials > OAuth client ID > Web application**.
   Authorised redirect URI: the value shown in Adrient under
   **Provider settings** (it is
   `https://<your workspace host>/api/v1/import/wizard/oauth/callback`).
5. Paste the client ID and secret into **Move to Adrient > Google > Provider
   settings**. Until this is saved, Google shows "Not configured" and cannot be
   connected.

The scopes Adrient asks for are read-only, except `https://mail.google.com/`,
which is the only scope Gmail's IMAP accepts. Adrient opens every mailbox
read-only and never changes or deletes anything in Google.

### Whole-organisation migration (optional)

Each person can migrate their own account with **My own account**. To move every
user at once as an admin:

1. Create a service account in the same project and download its JSON key.
2. In the Google Admin console, **Security > API controls > Domain-wide
   delegation**, add the service account's client ID with the same scopes as
   above.
3. Paste the JSON key into Provider settings ("Service account key").
4. Connect with **The whole organisation**, sign in as a Google admin, then
   **Find users**, check the mapping table, and run.

Without a service account an organisation connection can still map users and
import groups; per-user data then fails with "needs a service account with
domain-wide delegation" in the error report.

## Running a migration

1. Pick what to move, leave **Dry run** on, and start. The dry run reads
   everything and counts it without writing; problems show up in the report.
2. Turn dry run off and start again.
3. You can cancel at any time. **Running again is safe**: everything already
   imported is skipped ("Already there"), so a cancelled, failed or interrupted
   run resumes where it stopped. A server restart marks running jobs
   "interrupted"; start them again.
4. Download the error report (CSV) for anything that could not be moved.

Google rate limits (HTTP 429/503) are retried automatically with back-off,
honouring `Retry-After`.

## What was tested

The Google code path was tested against a local fake of the Google APIs and an
in-memory IMAP server that accepts XOAUTH2, using response shapes from Google's
public API reference (see `internal/cloudimport/google_test.go` and
`wizard_integration_test.go`). Try a single test account first, then run the full migration.
