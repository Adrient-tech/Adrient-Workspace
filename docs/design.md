# Adrient design system

Adrient Workspace should look like Adrient, not like the suite it grew from.
This page is the source of truth for the visual identity: principles, tokens,
the icon set and the shell. The code lives in:

| What | Where |
|---|---|
| Tokens and the MUI Joy theme | `web/app/src/theme.ts`, `web/app/src/styles/adrient.css` |
| Icon set | `web/app/src/components/icons/` (`glyphs.tsx`, `palette.ts`), `components/AppIcon.tsx` |
| App groups and pinning | `web/app/src/catalog/groups.ts`, `catalog/pins.ts` |
| Shell (dock rail, top bar, breadcrumbs, Ctrl-K) | `components/Shell.tsx`, `components/Header.tsx`, `components/breadcrumbs.ts`, `pages/home/CommandPalette.tsx` |
| Today home and All apps | `pages/home/`, `pages/Dashboard.tsx` |
| Page header and empty states | `components/ui/PageHeader.tsx`, `components/ui/EmptyState.tsx` |

## Principles

1. **Calm and warm.** Green-tinted neutrals, soft radii, hairline borders and
   shadows that only appear on hover or on floating layers. Never pure grey,
   never glossy gradients or drop-shadow stacks.
2. **One accent, one spark.** Adrient green `#126C5A` carries actions and
   selection. Saffron `#F2A21B` is the spark: highlights, the active-rail bar,
   "next" markers, warnings. Use it in small doses.
3. **Today first.** The home screen answers "what needs me now" with real data
   (meetings, mail, tasks, approvals, recent files) before it shows an app grid.
4. **Modules inherit, they do not opt in.** Colour, type, radii, buttons, tabs,
   chips, tables, menus and dialogs come from the theme overrides, and the rail,
   top bar and command bar come from the shell. A module changes nothing to
   look right.
5. **Honest empty and error states.** Every card says what it is showing, shows
   a small illustration when it is empty, and offers a retry when it failed. An
   app that is switched off is not shown, never shown broken.
6. **Light and dark are separate designs.** Dark is deep green-ink with lifted
   greens, not an inversion.

## Tokens

### Colour

| Token | Light | Dark | Use |
|---|---|---|---|
| Green (primary 500) | `#126C5A` | `#1A8F76` (solid `#178A71`) | actions, selection, links |
| Green deep | `#0C4A3E` | | pressed state, ink on tint |
| Saffron | `#F2A21B` (scale 50..900) | same | secondary accent, `warning` |
| Ink | `#0E2420` | | text on light, dark surfaces |
| Body | `#F6F8F7` | `#0A1311` | page background |
| Surface | `#FFFFFF` | `#111D1A` | cards, bars, dialogs |
| Level 1 / 2 / 3 | `#F0F4F2` / `#E5ECE9` / `#D9E3DF` | `#162520` / `#1D2F29` / `#263B34` | wells, hover, segmented tracks |
| Divider | `rgba(14,36,32,.09)` | `rgba(231,240,236,.09)` | all hairlines |
| Text primary / secondary / tertiary | `#0E2420` / `#3F5650` / `#6B807A` | `#E8F1ED` / `#AFC2BB` / `#7E948D` | |

The same values are exported from `theme.ts` as `tokens` for code that styles
through `sx` or inline SVG, and exposed as CSS variables in `adrient.css`
(`--ad-green`, `--ad-saffron`, `--ad-ink`, `--ad-s1..12`, `--ad-r-*`,
`--ad-rail-w`, `--ad-topbar-h`).

### Type

Two faces, both bundled in `web/app/public/fonts` (SIL OFL 1.1, notices in
`THIRD_PARTY_NOTICES.md`), no CDN:

* **Plus Jakarta Sans** 400-800: display, headings, titles (`h1`..`h4`,
  `title-*`). Tight tracking (-0.01 to -0.03em) on large sizes.
* **Inter** 400-600: body text and dense data (tables, forms).

Documents keep their own fonts (the Calibri/Arial/Noto bundle); the pairing
applies to the application chrome only.

### Shape, space, elevation

* Radius: 8 (chips, small), 12 (inputs, buttons), 16 (cards), 22 (dialogs),
  999 (pills, search, tabs of the home launcher).
* Spacing: 4px base: 4 / 8 / 12 / 16 / 20 / 24 / 32 / 40 / 48. Cards pad 16,
  page gutters 16 (phone) to 24 (desktop), page max width 1180.
* Elevation: `xs` hairline, `sm` raised control, `md` hovered card and menus,
  `lg` popovers, `xl` dialogs. Shadows are tinted with the ink colour.

### Component rules (set in `theme.ts`, nothing to do in a module)

* **Buttons** 600 weight, 11px radius, a subtle inner highlight on solid
  primary, 0.5px press.
* **Tabs** are a segmented control: a soft track with a raised selected pill.
* **Chips** are pills, 600 weight.
* **Tables** have a tinted header row with small caps labels, hairline row
  separators and a soft hover row.
* **Menus and dialogs** use 14px / 22px radii and the `lg` / `xl` shadows.
* **Focus** is a 2px green ring (`focusVisible`), never removed.

## Icons

The app icon set is original: simple geometric glyphs drawn on a 24px grid and
rendered as inline SVG (no icon font, no raster, no third-party artwork).

* **Construction:** 1.7px round-capped, round-joined stroke; live area
  3.5..20.5 (17px) so every glyph has the same optical size; corner radii from
  the set {1.2, 1.5, 2, 2.5, 3}. Each glyph has exactly one accent shape,
  either filled (`class="a"`), a translucent fill (`class="s"`) or an accent
  stroke (`class="as"`). Glyphs never use more than two colours.
* **Tile:** a soft rounded square (corner radius 30% of the size) with the
  group gradient. The glyph is the group's deep colour, the accent shape the
  group's second colour.
* **Groups, one gradient each:**

  | Group | Apps | Colours |
  |---|---|---|
  | Office | Drive, Docs, Sheets, Slides, PDF, Forms, Keep, Whiteboard, Wiki, Tasks | blue to teal |
  | Communicate | Mail, Chat, Meet, Calendar, Contacts, Telephony, Groups, Tickets, Live | violet to pink |
  | Business | CRM, Invoicing, Accounting, POS, Inventory, HR, Talent, Shifts, Time tracking, Projects, Contracts, Signatures, Records, Field service, Fleet, Sites | Adrient green with amber accents |
  | Admin | Admin, Access, VPN, Git, Cloud import, Share with friends, Home Assistant | slate |
  | More | Photos, Music, Video, Books, Games, Podcasts, Translate, 3D, Archaeology, Maps, Learn | coral to amber |

* **Variants:** `soft` light (pale tinted tile, deep stroke), `soft` dark (deep
  tinted tile, light stroke), and `solid` (saturated gradient, white glyph) for
  favicons, splash and share cards. All three come from the same SVG through CSS
  variables, selected by the colour scheme.
* **Adding an icon:** draw the glyph in `glyphs.tsx` (key = app id), add the id
  to a group in `catalog/groups.ts`, set `iconName` to the key in
  `catalog/apps.ts`. `groups.test.ts` fails if a catalog app has no glyph. An
  app with its own artwork (`iconUrl`, e.g. a game) keeps it; an unknown name
  falls back to a letter tile.
* **Mark and favicon:** the owner's own Adrient mark (`public/adrient-mark.png`
  and the owner-supplied favicons). Never redraw or recolour it; the PWA,
  Apple-touch and maskable icons are the same PNG scaled and padded (white tile
  where the platform needs an opaque icon). Light UI uses `adrient-logo.png`,
  dark UI uses `adrient-logo-white.png`.

## Shell

* **Dock rail** (md and up, 64px): logo, search, Home, the user's pinned apps
  (saffron bar marks the open app) and All apps. Hidden on phones, where the
  header menu is used, and in focus or public routes (document editors, share
  pages, POS display; see `railVisible`).
* **Top bar** (56px, sticky): app icon and title, breadcrumbs for the section
  below the app, a search pill (Ctrl/Cmd-K), notifications, the apps menu, an
  organisation switcher and the avatar menu.
* **Command bar** (Ctrl/Cmd-K, also the home search): apps, quick actions, files
  and results from global search, people from the directory, and "Ask Adrient".
  A lookup puts matches first; a question puts Ask first. Ctrl-K is left alone
  inside rich-text editors, where it inserts a link.
* **Pinning:** the star on an app tile pins it. Pins are stored per user in the
  preferences `extra` bag (`pinned_apps`), with a local copy for the first paint.

## Today home

`/` shows a greeting, the command bar, quick actions (only for apps that are on)
and cards: today's meetings (Calendar), mail (unread count and top threads),
my tasks, approvals waiting (HR leave and submitted expense reports) and recent
files, then the launcher (Pinned and the five groups, with a filter). A card
whose app is off for the user is not rendered; a card whose request fails shows
a retry; a card with no data shows an illustration. The full grid is at `/apps`.

Not included: POS discount approvals. Discounts above the limit are approved on
the till with a manager PIN (there is no queue to read), so they are not listed.

## Checking a change

Take screenshots at 1440x900, 1024 and 390 wide in light and dark and look at
them. Typical regressions: a dark surface that still uses a hard-coded white, a
page that sets its own `100vh` header and hides under the sticky bar, text that
is not in Inter or Plus Jakarta Sans.

## Pickers and tables at scale

An organisation can have 10,000+ people and millions of audit rows, so no screen loads a whole list and none renders a huge dropdown.

**Pickers** (`web/app/src/components/AsyncPicker.tsx`, `PeoplePicker.tsx`)
- `AsyncPicker` is the shared base: debounced type-ahead, server-side filtering, a windowed list (only the visible rows are in the DOM), the next page loads when the list scrolls near its end, arrow/Enter/Escape/Home keys, chips with a `+N` overflow, "Me" and recently used shortcuts, empty and error (with Retry) states, `combobox`/`listbox` roles with `aria-activedescendant`.
- `PeoplePicker`, `GroupPicker`, `UnitPicker` query `GET /api/v1/directory/search?type=user|group|unit&q=&limit=&cursor=` (at most 50 rows per page, keyset cursor over name then id, org scoped, guests only see people visible to them, blocked people hidden unless asked). `RolePicker` and `AppPicker` filter their short fixed lists in the browser through the same base.
- A person row shows avatar, name, email and department, title and employee ID, so people with the same name can be told apart. The search matches name, email, employee ID, UPN and department.
- Forms that store only ids rehydrate chips with `usePeopleByIds` (one request per 50 ids).

**Tables** (`components/DataTable.tsx`)
- Modes: `cursor` (previous/next over keyset pages, for time-ordered feeds), `page` (page jump when the server counts cheaply) and `feed` (infinite scroll).
- Page size 25/50/100, sortable columns, filter bar, free-text search, skeleton rows, empty and error states, row windowing above 60 rows.
- State lives in the URL (`q`, `sort`, `order`, `size`, `page`, `cur`, `f_<filter>`), so reload and shared links restore the same view.
- The range reads "1-50 of 12,340". Counts stop at 10,000 and read "10,000+", so a count never costs a full scan.
- "Export CSV" streams from the server with the same filters (formula-looking cells are neutralised).

**Server contract** (`internal/pagekit`)
- Parameters: `limit` (capped, default 50, max 200), `cursor` or `page`, `sort`, `order`, `q`, plus filters.
- Response: `{items, next_cursor, total_estimate, total_capped}`. In `page` mode a non-empty `next_cursor` means another page exists.
- Keyset cursors are opaque and carry `(sort value, id)`, so rows with equal timestamps or names never repeat or vanish between pages.
- Every filtered or sorted column has an index (migrations 0365, 0366). Check new queries with `EXPLAIN` on the scale data (`scripts/scale/seed_scale.sql`).
