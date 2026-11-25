<p align="center"><img src="web/app/public/adrient-logo.png" alt="Adrient" width="260"></p>

<h1 align="center">Adrient Workspace</h1>

<p align="center"><b>Your own workspace. Your data. Your infrastructure.</b></p>

<p align="center">
  <a href="LICENSE"><img alt="MIT License" src="https://img.shields.io/badge/license-MIT-126C5A?style=flat-square"></a>
  <a href="https://adrient.com"><img alt="adrient.com" src="https://img.shields.io/badge/web-adrient.com-126C5A?style=flat-square"></a>
  <a href="mailto:info@adrient.com"><img alt="Support" src="https://img.shields.io/badge/support-info%40adrient.com-126C5A?style=flat-square"></a>
  <a href="https://razorpay.me/@adrient"><img alt="Sponsor" src="https://img.shields.io/badge/sponsor-razorpay.me%2F%40adrient-126C5A?style=flat-square"></a>
</p>

Adrient Workspace is a **self-hosted, open-source workspace platform** from [Adrient Pvt Ltd](https://adrient.com).
It puts the tools a company uses every day on servers you control: Drive, Mail, Calendar, Docs, Sheets, Slides, Chat and
Meet, together with the business apps that run an organisation: CRM, HR, hiring, accounting, invoicing, inventory,
purchasing, point of sale, projects, helpdesk, shifts, time tracking, field service, fleet, records, a wiki, contracts and
e-signatures, legal matters and a cap table. Every app has an AI assistant, and the apps work with each other.

<p align="center"><img src="docs/screenshots/home.jpg" alt="Adrient Workspace home" width="900"></p>

> **MIT licensed, free forever.** Use it, run it, change it, host it for others.
> Website: [adrient.com](https://adrient.com) · Support: [info@adrient.com](mailto:info@adrient.com)

---

## Contents

- [What is inside](#what-is-inside)
- [Screenshots](#screenshots)
- [Office-style editors](#office-style-editors)
- [AI assistant in every app](#ai-assistant-in-every-app)
- [How the apps work together](#how-the-apps-work-together)
- [Administration and security](#administration-and-security)
- [Moving in from Google, Microsoft and Zoho](#moving-in-from-google-microsoft-and-zoho)
- [Install](#install)
- [Architecture](#architecture)
- [Repository layout](#repository-layout)
- [Support, bugs, feature requests and partners](#support-bugs-feature-requests-and-partners)
- [Contributing](#contributing)
- [Sponsor](#sponsor)
- [License](#license)

---

## What is inside

| Area | Apps |
|---|---|
| **Files and documents** | Drive, Docs, Sheets, Slides, PDF (view, fill, sign), Forms, Keep (notes), Whiteboard, Photos, Books |
| **Communication** | Mail, Chat (people, groups, channels), Meet (video calls), Calendar with booking pages, Contacts, Telephony |
| **People** | HR (employees, leave, attendance, payroll with payslips), Talent (hiring, onboarding, performance, learning), Shifts, Time tracking |
| **Money and operations** | Accounting, Invoicing, Inventory and purchasing, POS, Field service, Fleet |
| **Customers and work** | CRM, Projects, Tasks, Tickets (helpdesk), Contracts, Signatures, Legal (matters and billing), Cap table |
| **Knowledge** | Records (databases with many field types), Wiki, Sites (website builder) |
| **Administration** | Admin console, Groups, Access, Domains and mail DNS, Cloud Import, AI settings |
| **Media and extras** | Music, Video, Live, Podcasts, Maps, Translate, VPN, Git, a browser game arcade |

Highlights:

- **Docs, Sheets and Slides** have a ribbon, AutoSave, real-time editing, and import and export of xlsx, docx, pptx, PDF and csv.
- **Meet** runs on a self-hosted LiveKit server with TURN. It has host controls, a waiting room for guests, recordings saved to Drive and screen share.
- **Mail** is your own mail service with a ribbon, rules, saved searches, signatures, an Archive folder and a setup wizard for SPF, DKIM, DMARC and MTA-STS.
- **Calendar** has booking pages, find-a-time, public holidays, reminders and ICS import, export and subscribe.
- **Accounting** is double-entry bookkeeping with GST reports, bank matching, recurring entries, budgets, fixed assets and multi-currency.
- **HR and payroll** cover leave, attendance, PF, ESI and professional tax, and post one journal to Accounting.
- **CRM** has pipelines, automation, custom fields, web-to-lead forms and a Customer 360 view.
- **Inventory** has warehouses, lots and serial numbers, valuation, purchase orders, stock counts and write-offs.
- **POS** works offline, with manager PIN, gift cards, loyalty and an end-of-day report.

## Screenshots

Real screens from a running instance with fictional demo data (1440 x 900).

| | |
|---|---|
| ![Home](docs/screenshots/home.jpg)<br>**Home** | ![All apps](docs/screenshots/apps.jpg)<br>**All apps** |
| ![Mail](docs/screenshots/mail.jpg)<br>**Mail** | ![Drive](docs/screenshots/drive.jpg)<br>**Drive** |
| ![Chat](docs/screenshots/chat.jpg)<br>**Chat** | ![Calendar](docs/screenshots/calendar.jpg)<br>**Calendar** |
| ![Docs](docs/screenshots/docs-editor.jpg)<br>**Docs** | ![Sheets](docs/screenshots/sheets-editor.jpg)<br>**Sheets** |
| ![Slides](docs/screenshots/slides-editor.jpg)<br>**Slides** | ![Forms](docs/screenshots/forms.jpg)<br>**Forms** |
| ![CRM](docs/screenshots/crm-pipeline.jpg)<br>**CRM** | ![HR](docs/screenshots/hr-dashboard.jpg)<br>**HR** |
| ![Payroll](docs/screenshots/payroll.jpg)<br>**Payroll** | ![Talent](docs/screenshots/talent-pipeline.jpg)<br>**Talent** |
| ![Accounting](docs/screenshots/accounting-overview.jpg)<br>**Accounting** | ![Invoicing](docs/screenshots/invoicing.jpg)<br>**Invoicing** |
| ![Inventory](docs/screenshots/inventory.jpg)<br>**Inventory** | ![POS](docs/screenshots/pos.jpg)<br>**POS** |
| ![Shifts](docs/screenshots/shifts.jpg)<br>**Shifts** | ![Time tracking](docs/screenshots/timetracking.jpg)<br>**Time tracking** |
| ![Field service](docs/screenshots/fieldservice.jpg)<br>**Field service** | ![Fleet](docs/screenshots/fleet.jpg)<br>**Fleet** |
| ![Records](docs/screenshots/records-suppliers.jpg)<br>**Records** | ![Wiki](docs/screenshots/wiki.jpg)<br>**Wiki** |
| ![Contracts](docs/screenshots/contracts-all.jpg)<br>**Contracts** | ![Signatures](docs/screenshots/signatures.jpg)<br>**Signatures** |
| ![Legal](docs/screenshots/legal.jpg)<br>**Legal** | ![Cap table](docs/screenshots/captable.jpg)<br>**Cap table** |
| ![Tickets](docs/screenshots/tickets.jpg)<br>**Tickets** | ![Admin](docs/screenshots/admin-console.jpg)<br>**Admin console** |

## Office-style editors

Sheets, Docs and Slides use a ribbon with tabs (Home, Insert, Layout and more), a classic menu bar, AutoSave, sharing,
light and dark themes and a simple toolbar on phones.

- **Sheets:** tables, charts, pivots, conditional formatting, data validation, named ranges, goal seek, what-if tools,
  hundreds of functions with dynamic arrays, and xlsx import and export.
- **Docs:** styles, tables, footnotes, table of contents, comments, suggestions, headers and footers, citations,
  mail merge, version history, and docx and PDF export.
- **Slides:** masters and layouts, themes, shapes and diagrams, animations and transitions, presenter view, sections and templates.

## AI assistant in every app

Each app has an **Assistant** button that opens a panel on the right. It can answer questions about what you have open
and make changes for you. It always asks before it changes anything, and every change can be undone.

- **Office:** build a budget in Sheets, rewrite or summarise in Docs, draft slides in Slides.
- **Communication:** summarise a mail thread or a chat channel, draft replies (drafts are never sent for you), find a free time.
- **Business:** summarise a CRM deal, answer "how much did we spend on X" in Accounting, draft a purchase order, triage a ticket.
  HR answers only about the signed-in person.
- **Ask Adrient:** one search box that answers from your own files, mail, calendar and contacts, with its sources.
- **You choose the model.** Admins connect any OpenAI-compatible endpoint (Ollama, llama.cpp, vLLM, OpenRouter, OpenAI,
  Azure OpenAI), Anthropic or Gemini, or use local models only. Keys are stored encrypted and never reach the browser.
  There is a kill switch, per-app switches, quotas and an audit log.
- **Safe by design:** the assistant runs with your own permissions, and text from mails, tickets and files is treated as data, never as instructions.

## How the apps work together

- **Open with the right app.** Every file and link opens in the app that handles it, and a Back chip returns you.
- **Apps inside apps.** Meet opens inside Chat and Calendar. Mail attachments save to Drive, and Drive files attach to Mail or go to Chat.
- **One identity.** A single sign-in (OpenID Connect through Zitadel) for every app, with one directory of people, groups and domains.
- **Shared business data.** A won CRM deal becomes an estimate in Invoicing. A POS sale lowers stock and posts to the books.
  Payroll posts a journal to Accounting. A purchase order, a goods receipt and a supplier bill keep stock and ledger in step.
  A hired candidate becomes an employee with an onboarding checklist. Approved leave appears in Calendar.

## Administration and security

- Users, groups, org units and domains with DNS checks, and **user types**: internal, external, guest and service accounts.
- **Per-app controls** by user type, role presets (HR manager, accountant, sales rep, cashier and more) and an
  "effective access" viewer that shows what a person can actually do.
- **External collaboration rules:** partner domains (allow or block), guest invitations with an expiry and a sponsor.
- **Identity:** sign-in with other identity providers, SCIM provisioning, offboarding with data transfer, access reviews,
  session management and an audit log.
- **Multi-organisation:** every organisation's data is separated and every query is scoped to it.

## Moving in from Google, Microsoft and Zoho

The **Cloud Import** app migrates users, files, mail, calendars and contacts from Google Workspace, Microsoft 365, Zoho,
Nextcloud, ownCloud and any IMAP, CalDAV or WebDAV server. It supports dry runs, per-user mapping, progress, error
reports and safe re-runs. Guides: [Google](docs/migrating-from-google.md), [Microsoft](docs/migrating-from-microsoft.md),
[Zoho](docs/migrating-from-zoho.md). There is also an `adrient-admin` command for backups, restore, upgrades and a full
export of an organisation ([backup and restore](docs/backup-restore.md), [upgrade](docs/upgrade.md)).

## Install

### Try it on your computer

Needs Docker, Go 1.25+ and Node 20+ on Linux or macOS.

```sh
go install github.com/bufbuild/buf/cmd/buf@v1.64.0   # protobuf generator, once
deploy/local/stack.sh up                             # starts databases, sign-in, storage and the app
```

Open <http://workspace.localtest.me:8080> and sign in with `admin@adrient.localtest.me` / `DevPassword!1`
(development credentials, for your own computer only). Other commands: `deploy/local/stack.sh deploy`, `status`, `logs`, `down`.

To fill an organisation with fictional demo data for a tour:

```sh
adrient-admin seed-demo --org <org-slug>
adrient-admin seed-demo --org <org-slug> --remove   # take it out again
```

### Install on a server

One command installs everything (Docker, database, sign-in, storage, HTTPS) on a fresh Ubuntu 22.04/24.04 or Debian 12
server with 4 GB RAM or more:

```sh
sudo ./install.sh --domain example.com --email you@example.com
```

Point `workspace.` and `auth.` of your domain at the server first. Full guide: [docs/install-server.md](docs/install-server.md).

### Install on Kubernetes

```sh
helm install adrient deploy/helm/adrient -n adrient --create-namespace --set domain=adrient.example.com
```

Plain manifests are in `deploy/manifests/adrient.yaml`.

## Architecture

- **Backend:** Go, gRPC with a REST gateway, one binary.
- **Web app:** React and TypeScript (Vite, Joy UI), served by the backend.
- **Data:** PostgreSQL for records and S3-compatible object storage (RustFS) for files.
- **Sign-in:** Zitadel (OpenID Connect).
- **Video:** self-hosted LiveKit with TURN.
- **Mail:** a built-in mail service with DNS helpers.
- **Deploy:** Docker (single server), Helm and plain manifests (Kubernetes), and a local dev stack script.

## Repository layout

| Path | What is in it |
|---|---|
| `cmd/` | The server, the `adrient-admin` command and helper programs |
| `internal/<app>/` | The server code of each app (accounting, hr, crm, mail, drive, ...) |
| `internal/storage/migrations/` | Numbered database migrations |
| `web/app/` | The web app. Screens are in `src/pages/<app>/` |
| `deploy/` | Local stack, Docker, Helm and manifests |
| `docs/` | Install, migration, upgrade, design and per-app notes (`docs/services/`) |
| `proto/`, `gen/` | API definitions and generated code |

## Support, bugs, feature requests and partners

For **any question, upgrade help, bug report, feature request, partner request or enterprise development need**, write to:

### 📧 [info@adrient.com](mailto:info@adrient.com)

Visit **[adrient.com](https://adrient.com)** for products, services, support plans and partnerships.

| You want to | Do this |
|---|---|
| Ask a question or get help installing | Email info@adrient.com |
| Report a bug | Email info@adrient.com with the app name, what you did and what you saw (a screenshot helps) |
| Request a feature | Email info@adrient.com, or open an issue in this repository |
| Become a partner or reseller | Email info@adrient.com with your company name and website |
| Get enterprise development, custom apps, migration or hosting | Email info@adrient.com |

## Contributing

Contributions are welcome: fixes, features, translations and docs. Read [CONTRIBUTING.md](CONTRIBUTING.md) to set up and
send a pull request. A maintainer reviews every pull request.

## Sponsor

Adrient Workspace is free. If it helps you or your company, you can support its development with any amount:
**[razorpay.me/@adrient](https://razorpay.me/@adrient)**. See [SPONSORS.md](SPONSORS.md).

## License

[MIT](LICENSE) for the platform's own code. A few bundled components keep their own licences, noted in their folders
and in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

<p align="center">Made by <a href="https://adrient.com">Adrient Pvt Ltd</a> · <a href="mailto:info@adrient.com">info@adrient.com</a></p>
