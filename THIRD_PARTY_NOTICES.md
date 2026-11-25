# Third-party data shipped with Adrient

Adrient's code is MIT (see `LICENSE`). These data files are bundled under
their own licences.

## Spell-check dictionaries (`web/app/public/dict/`)

- English (US) and English (UK) Hunspell dictionaries from SCOWL
  (http://wordlist.aspell.net), Copyright 2000-2018 Kevin Atkinson and
  others, via the `dictionary-en` / `dictionary-en-gb` npm packages.
  Permissive SCOWL / Ispell licence; full text in
  `web/app/public/dict/LICENSE-en_US.txt` and `LICENSE-en_GB.txt`.

## Fonts (`web/app/public/fonts/`)

See `web/app/public/fonts/README.md` for the list, sources and licence
files.

| Family | Licence | Copyright |
|---|---|---|
| Noto Sans, Noto Serif, Noto Sans Mono | SIL OFL 1.1 | The Noto Project Authors |
| Carlito | SIL OFL 1.1 | The Carlito Project Authors |
| Caladea | SIL OFL 1.1 | The Caladea Project Authors |
| Arimo, Tinos, Cousine | SIL OFL 1.1 | The Arimo / Tinos / Cousine Project Authors |
| Plus Jakarta Sans (`fonts/plus-jakarta-sans/`, Adrient display and heading face) | SIL OFL 1.1 | Copyright 2020 The Plus Jakarta Sans Project Authors (https://github.com/tokotype/PlusJakartaSans) |
| Inter (`fonts/inter/`, Adrient body face) | SIL OFL 1.1 | Copyright (c) 2016 The Inter Project Authors (https://github.com/rsms/inter) |
| Dancing Script (`pdf/frontend/src/assets/fonts/signature-script-*.woff2`, the PDF app's typed-signature face, registered as "Signature Script") | SIL OFL 1.1 | Copyright 2016 The Dancing Script Project Authors (https://github.com/googlefonts/DancingScript) |

Plus Jakarta Sans and Inter are bundled unmodified (Fontsource woff2 subsets of
the upstream releases) with their full licence text in each directory's
`LICENSE.txt`; the UI loads them from the app's own origin, never from a CDN.
The Adrient icon set (`web/app/src/components/icons/`) is original artwork
drawn for this project and carries no third-party licence.

## Games (`web/app/public/games/`)

| Component | Licence | Notes |
|---|---|---|
| Bolo (`games/bolo/`) and WinBolo (`games/winbolo/`) WebAssembly ports | GNU GPL v2 (`LICENSE` in each directory) | Not covered by the MIT licence. The GPL requires the corresponding source to be available to everyone who receives the binaries (`Bolo.wasm`, `Bolo.data`). |
| Maelstrom (`games/maelstrom/`) | Code: Zlib. Artwork and animations: CC BY 3.0 (`COPYING`) | Attribution: Sam Lantinga / Ambrosia Software. |
| Mighty Mike (`games/mightymike/`) | **Not stated in this repository** | No licence file ships with this port; its licence must be confirmed before redistribution. |

## Libraries

The Go modules in `go.mod` and the npm packages in `web/app/package-lock.json`
and `pdf/frontend/package-lock.json` are compiled into the server binary and
the web bundle under their own (mostly MIT, BSD and Apache-2.0) licences.
The `tibui` package used by `pdf/frontend` is downloaded from
`code.pick.haus` as pinned in `pdf/frontend/package.json`.

## Code derived from other projects

This is the only file in the repository that names these projects. The
source files listed below carry no origin comments.

### Base platform

The platform code this project started from is MIT licensed.

Copyright (c) 2026 Lucas Pick

Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the "Software"), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

### HeyPuter/everything

Logic in the files below was ported from the MIT-licensed apps of
https://github.com/HeyPuter/everything (accounting, expense, invoicing,
inventory, POS, CRM, hiring, help desk, survey, tasks, projects, calendar,
team chat, meetings, documents, records, wiki, contracts, signatures, legal,
cap table, shifts, time tracking, field service and fleet). No code was taken
from its AGPL apps.

Copyright (c) 2026 Puter Technologies Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the "Software"), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

Files:

- `internal/accounting/bankmatch.go`
- `internal/accounting/ledger.go`
- `internal/accounting/reports3.go`
- `internal/captable/math.go`
- `internal/chat/teams_store.go`
- `internal/contracts/dates.go`
- `internal/contracts/obligations.go`
- `internal/crm/repository.go`
- `internal/fieldservice/logic.go`
- `internal/fleet/logic.go`
- `internal/inventory/repository.go`
- `internal/inventory/shrinkage.go`
- `internal/invoicing/calc.go`
- `internal/invoicing/schedule.go`
- `internal/legal/logic.go`
- `internal/pos/profit.go`
- `internal/projects/wave6.go`
- `internal/records/csv.go`
- `internal/records/engine.go`
- `internal/records/formula.go`
- `internal/records/query.go`
- `internal/records/templates.go`
- `internal/records/types.go`
- `internal/shifts/logic.go`
- `internal/shifts/plan.go`
- `internal/signatures/sig.go`
- `internal/storage/migrations/0112_invoicing.sql`
- `internal/storage/migrations/0113_crm.sql`
- `internal/storage/migrations/0114_accounting.sql`
- `internal/storage/migrations/0290_tickets_desk.sql`
- `internal/storage/migrations/0291_talent_guides.sql`
- `internal/talent/guides.go`
- `internal/tasks/feed.go`
- `internal/tasks/links.go`
- `internal/tasks/rrule.go`
- `internal/tickets/desk_logic.go`
- `internal/tickets/desk_repo.go`
- `internal/timetracking/logic.go`
- `internal/wiki/content.go`
- `internal/wiki/wiki.go`
- `web/app/src/pages/calendar/holidays.ts`
- `web/app/src/pages/captable/logic.ts`
- `web/app/src/pages/chat/index.tsx`
- `web/app/src/pages/chat/slash.ts`
- `web/app/src/pages/chat/typing.ts`
- `web/app/src/pages/contracts/logic.ts`
- `web/app/src/pages/crm/ics.ts`
- `web/app/src/pages/crm/logic.ts`
- `web/app/src/pages/docs/docx/fidelity.ts`
- `web/app/src/pages/docs/fontEffects.ts`
- `web/app/src/pages/docs/readAloud.ts`
- `web/app/src/pages/docs/sortParagraphs.ts`
- `web/app/src/pages/docs/templates.ts`
- `web/app/src/pages/docs/textStats.ts`
- `web/app/src/pages/fieldservice/logic.ts`
- `web/app/src/pages/fleet/logic.ts`
- `web/app/src/pages/forms/insights.ts`
- `web/app/src/pages/forms/surveyTemplates.ts`
- `web/app/src/pages/forms/textImport.ts`
- `web/app/src/pages/inventory/Shrinkage.tsx`
- `web/app/src/pages/inventory/logic.ts`
- `web/app/src/pages/invoicing/logic.ts`
- `web/app/src/pages/legal/logic.ts`
- `web/app/src/pages/meet/background-compositor.ts`
- `web/app/src/pages/meet/background-effect.ts`
- `web/app/src/pages/meet/lib.ts`
- `web/app/src/pages/meet/media.ts`
- `web/app/src/pages/projects/ExchangeTab.tsx`
- `web/app/src/pages/projects/exchange.ts`
- `web/app/src/pages/projects/quickadd.ts`
- `web/app/src/pages/records/logic.ts`
- `web/app/src/pages/sheets/dataCleanup.ts`
- `web/app/src/pages/signatures/logic.ts`
- `web/app/src/pages/tasks/quickparse.ts`
- `web/app/src/pages/tickets/Desk.tsx`
- `web/app/src/pages/tickets/deskApi.ts`
- `web/app/src/pages/tickets/deskLogic.ts`
- `web/app/src/pages/timetracking/logic.ts`

### ONLYOFFICE (sdkjs test suites and core test documents)

- Project: ONLYOFFICE Docs, https://github.com/ONLYOFFICE/sdkjs and https://github.com/ONLYOFFICE/core
- Licence: **GNU AGPL-3.0**. No ONLYOFFICE source code is included in this repository.
- What was derived: test *behaviour* only. The cases in the folders below restate what the ONLYOFFICE sdkjs QUnit suites (tests v9.3.1: word, cell, slide, pdf, visio, common) and core converters check: inputs, steps and expected values, recorded as facts. Every test, harness, fixture and runner was re-written for Adrient from that observed behaviour; nothing was copied. Test names that matched suite titles were kept as plain descriptions, and the `oo:` scoreboard tags, the `onlyoffice-parity` scoreboard scripts and the CI scoreboard step were removed. Recorded differences are tagged `known-diff/...` (formerly `oo-diff/...`); "the baseline" in those notes means the recorded ONLYOFFICE result.
- Optional local corpora (`ADRIENT_CONVERSION_CORPUS`, `ADRIENT_LIBREOFFICE_CORPUS`, `VSDX_CORPUS_DIR`, default `research/corpus/`) were meant to point at ONLYOFFICE's AGPL sample documents; they are gitignored and never committed.
- Folders: `web/app/src/pages/docs/__tests__/compat/` (formerly `__tests__/oo/`), `web/app/src/pages/sheets/__compat__/` (formerly `__parity__/`), `internal/sheets/testdata/formulas/` (formerly `testdata/parity/`), `internal/sheets/testdata/{dynarray,numfmt,goalseek,structure,pivot}/`, `internal/sheets/*_test.go`, `internal/docs/{convert,corpus}_test.go`, `internal/forms/validate_test.go`, `web/app/src/pages/forms/validate.test.ts`, `web/app/src/lib/{colorMods,urlType,chartEx}.test.ts`, `web/app/src/codeStyle.test.ts`, `web/app/src/pages/{slides,whiteboard}/` tests, `web/e2e/` (`docs/forms-compat.spec.ts`, `docs/shortcuts-compat.spec.ts`, `sheets-*.spec.ts`, `integration/*-roundtrip.spec.ts`), `pdf/frontend/e2e/*forms-calc*.spec.ts`. Some editor modules (Docs content controls, forms, styles, numbering, Slides element ops) follow ONLYOFFICE API method names (SetText, GetMainForm, ConvertFormFixedType, ...) where a scripting-API equivalent exists.

### Spacelight

- Project: Spacelight (self-hosted family hub). URL and licence not recorded in this repository; confirm before redistribution.
- What was derived: the radio recorder design (ICY StreamTitle parsing, 30 s minimum song length, the German "Title von Artist" form) and the built-in radio station list (from its `apps/api/seed/seed-radio.ts`).
- Folders: `internal/music/radio/recorder.go`, `internal/music/radio_seed.go`. The app tile id `spacelight` (shown as "Lightsky") and the `VITE_SPACELIGHT_URL` setting are kept as runtime keys.

### Supertonic

- Project: Supertonic, https://github.com/supertone-inc/supertonic. Licence: see that repository (code MIT; model weights under their own licence).
- What was derived: `web/app/src/pages/translate/supertonic.ts` is a TypeScript port of its web example (`web/helper.js`).

### Design references (no code)

- The admin console layout (`web/app/src/pages/admin/`, `proto/adrient/v1/admin.proto`) was modelled on the Google Admin console and Google Workspace Admin's Security menu; the Projects issue tracker (`proto/adrient/v1/projects.proto`) on the Linear UI. Layout ideas only; no code or assets.
- `install.sh`, `deploy/helm/adrient/templates/zitadel-provision.yaml` and `deploy/manifests/adrient.yaml` follow the repository's own `deploy/zitadel/create-oidc-app.sh` and a private homelab Zitadel client job.
- Field service and fleet docs (`docs/services/fieldservice.md`, `docs/services/fleet.md`) describe logic ported from the HeyPuter apps listed above (`totals.js`, `reminders.js`).
