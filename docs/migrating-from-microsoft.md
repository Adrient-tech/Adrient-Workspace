# Migrating from Microsoft 365 or Outlook

Open **Move to Adrient** (`/cloudimport`) and choose **Microsoft 365 / Outlook**.
Data is read through Microsoft Graph.

## What is migrated

| Microsoft 365 | Adrient | Notes |
|---|---|---|
| Groups (Microsoft 365 and security groups) | Groups, members mapped by e-mail | |
| SharePoint document libraries | A folder `<site> (SharePoint)` | Imported for the person who connected (or with scope "me"). |
| Outlook mail | Mail | Every folder, including sub-folders. Inbox, Sent Items, Drafts, Deleted Items and Junk Email map to the same Adrient folders; other folders become labels (`Inbox/Projects`). Categories become labels. Read and flagged state are kept. Messages are fetched as full MIME. **Attachments are not imported yet.** |
| Calendar | Calendar | Daily, weekly, absolute-monthly and absolute-yearly recurrences become RRULEs. "Relative" patterns (e.g. "second Tuesday") are imported as a single occurrence. Attendees are not copied. |
| Contacts | Contacts | |
| Teams chat | not available | Exporting Teams messages needs Microsoft's protected-API approval and is metered by Microsoft, so Adrient does not offer it. |

## Set up the app registration (admin, once)

1. Microsoft Entra admin centre > **App registrations > New registration**.
   Supported account types: *this organisation only* is enough.
2. **Redirect URI** (platform *Web*): the value shown in Adrient under
   **Provider settings** (`https://<host>/api/v1/import/wizard/oauth/callback`).
3. **Certificates & secrets > New client secret**. Copy the value.
4. **API permissions** (Microsoft Graph):
   - For people migrating **their own account** (delegated): `User.Read`,
     `User.ReadBasic.All`, `Files.Read.All`, `Sites.Read.All`, `Mail.Read`,
     `Calendars.Read`, `Contacts.Read`, `offline_access`.
   - For an **organisation-wide** migration (application permissions, then
     **Grant admin consent**): `User.Read.All`, `Group.Read.All`,
     `GroupMember.Read.All`, `Files.Read.All`, `Sites.Read.All`, `Mail.Read`,
     `Calendars.Read`, `Contacts.Read`.
5. Paste the client ID, secret and your **tenant ID** into Adrient's Provider
   settings. Organisation-wide migration needs the tenant ID; it uses the
   client-credentials flow, so no one has to sign in.

All permissions are read-only. Adrient never changes data in Microsoft 365.

## Running

Same as for Google: dry run first, then the real run; re-runs skip what is
already imported, so a cancelled or interrupted run resumes; throttling
(429/503 with `Retry-After`) is retried automatically; download the CSV error
report at the end.

## What was tested

Tested against a local fake Graph server with responses written from the
Graph v1.0 reference (`internal/cloudimport/microsoft_test.go`), including
paging via `@odata.nextLink`, nested mail folders, permissions and recurrence.
Try a single test account first, then run the full migration.
