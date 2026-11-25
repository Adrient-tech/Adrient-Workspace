# Migrating from Zoho Workplace (and Nextcloud, ownCloud, IMAP, CalDAV)

These sources use open protocols and an app password, so no OAuth app is
needed. Each person connects their own account under **Move to Adrient**.

## Zoho Workplace

| Zoho | How | Adrient |
|---|---|---|
| Zoho Mail | IMAP, `imap.zoho.<region>:993` | Mail, folders mapped like other IMAP sources |
| Zoho Calendar | CalDAV, `https://calendar.zoho.<region>/caldav/` | Calendar |
| Zoho Contacts | CardDAV, `https://contacts.zoho.<region>/carddav/` | Contacts |
| WorkDrive | not live; export a ZIP from WorkDrive and use **Upload an export** | Drive |
| Cliq chat | not available | |

1. Enable IMAP access in Zoho Mail (Settings > Mail Accounts > IMAP).
2. Create an **app-specific password** in Zoho Accounts (Security > App
   Passwords). With two-factor sign-in on, your normal password will not work.
3. In Adrient choose **Zoho Workplace**, enter your Zoho e-mail, the app
   password and your data centre (`com`, `eu`, `in`, `com.au`, `jp`, `ca`), and
   press **Test and connect**. Adrient signs in to IMAP before saving.

Zoho users are not listed automatically: create the Adrient accounts first, and
each person migrates their own mailbox. **The Zoho endpoints above were taken
from Zoho's documentation and have not been tested against a real Zoho
account**; if your account uses different CalDAV/CardDAV URLs, use the
"Any CalDAV / CardDAV / WebDAV server" option with the URLs Zoho shows you.

## Nextcloud

Enter the server address (`https://cloud.example.com`), your username and an
app password (Settings > Security > Devices & sessions > Create new app
password). Adrient reads:

- Files over WebDAV (`/remote.php/dav/files/<user>/`), with folders.
- Calendars over CalDAV (task lists are skipped).
- Address books over CardDAV.
- Mail, if you add your mail server's IMAP details (Nextcloud itself does not
  host mail).

Nextcloud Talk history and Nextcloud shares are not migrated.

## ownCloud 10

Same as Nextcloud (same DAV layout). Calendar and Contacts need the ownCloud
Calendar and Contacts apps. ownCloud Infinite Scale (oCIS) is not covered.

## Any IMAP server

Dovecot, Exchange (IMAP enabled), Fastmail, Yahoo, cPanel and similar: enter the
server, port (993 with TLS or 143 with STARTTLS) and credentials. Every
selectable folder is copied; Inbox, Sent, Drafts, Trash and Spam/Junk map to
the Adrient folders (by special-use flags where the server reports them, else by
name), other folders become labels. Mailboxes are opened read-only.

## Any CalDAV / CardDAV / WebDAV server

Radicale, Baïkal, SOGo, Fastmail, iCloud (with an app-specific password): give
any of the CalDAV, CardDAV and WebDAV URLs; empty ones are skipped. Adrient
follows `current-user-principal` and the calendar/address-book home sets, so the
server root usually works.

## Self-hosted servers on your own network

To stop the migration feature from being used to probe internal services,
Adrient refuses server addresses that resolve to private, loopback or
link-local IPs, and plain `http://` URLs. If your Nextcloud lives on the LAN,
an administrator can allow this with `ADRIENT_IMPORT_ALLOW_PRIVATE_HOSTS=1` in
the server environment.

## What was tested

Nextcloud/ownCloud and generic DAV were tested against a local fake server that
answers like sabre/dav (Nextcloud's DAV library), and IMAP against an in-memory
IMAP server (`internal/cloudimport/dav_test.go`). Try a single test account first, then run the full migration.
