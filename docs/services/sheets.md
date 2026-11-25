# Sheets

Collaborative spreadsheets with templates (monthly budget, to-do list, weekly schedule, expense tracker) and recent files.

## Keyboard shortcuts

Help ▸ Keyboard shortcuts (Ctrl+/) lists every shortcut and switches the
**shortcut style**; the same choice is in Settings ▸ Keyboard shortcuts. It is
saved per user and applies at once. On a Mac, Ctrl means ⌘ and Alt means
Option.

| Action | Microsoft Excel style (default) | Google Sheets style |
|---|---|---|
| Strikethrough | Ctrl+5 (and Alt+Shift+5) | Alt+Shift+5 |
| Format cells (number format) | Ctrl+1 | — |
| Paste special… | Ctrl+Alt+V | — |
| Insert rows (columns when whole columns are selected) | Ctrl+Shift+= | Ctrl+Alt+= |
| Delete rows (columns when whole columns are selected) | Ctrl+- | Ctrl+Alt+- |
| Hide rows / columns | Ctrl+9 / Ctrl+0 | Ctrl+Alt+9 / Ctrl+Alt+0 |
| Align left / centre / right | — | Ctrl+Shift+L / E / R |
| Filter on/off | Ctrl+Shift+L | — |
| Table total row | Ctrl+Shift+R | — |
| Insert table | Ctrl+L / Ctrl+T | Ctrl+Alt+T |

Shared by both: Ctrl+Shift+V paste values, Ctrl+D / Ctrl+R fill down / right,
Ctrl+Shift+9 / Ctrl+Shift+0 unhide rows / columns, Ctrl+Space / Shift+Space
select column / row, Ctrl+Alt+M and Shift+F2 comment (Adrient has no separate
notes), Ctrl+Shift+1…6 number formats, and the rest of the list.

Not bound, and why:

- Borders (Google's Alt+Shift+1…7, Excel's Ctrl+Shift+& / Ctrl+Shift+_): the
  grid (FortuneSheet) offers no way to set borders from outside its toolbar;
  use the toolbar's border menu.
- Excel's Ctrl+Shift+= / Ctrl+- open an Insert / Delete dialog for a partial
  selection; Adrient inserts or deletes whole rows (or whole columns when whole
  columns are selected), as Google Sheets does.
- Ctrl+T, Ctrl+W, Ctrl+N and (in some browsers) Ctrl+0…9 belong to the browser,
  which may act before the page sees them; Ctrl+Space / ⌘+Space may be taken
  by the operating system's input switcher or Spotlight.

## Desktop

![Sheets — desktop](screenshots/sheets-desktop.png)

## Mobile

<img src="screenshots/sheets-mobile.png" alt="Sheets — mobile" width="320">

---

_Live-captured at `http://workspace.localtest.me:8080/sheets` against the local full stack, authenticated as `admin@adrient.localtest.me` via Zitadel SSO._

## responsive ribbon, grouping, functions

- **Responsive ribbon** (`web/app/src/components/ribbon/ResponsiveRibbon.tsx`, shared with Docs and Slides).
  When the window narrows, groups shrink one at a time to small icon buttons, then collapse one at a time into
  a single drop-down button that opens the group in a popover. Groups with a lower priority shrink first, so
  Clipboard, Font and Editing keep their full size longest. The panel no longer scrolls sideways at 1024 px and up.
- **Home ▸ Cells** has Insert, Delete and Format (row height, column width, hide/unhide, protect), and
  **Home ▸ Editing** has AutoSum, Fill (down/right/up/left, series), Clear (all, formats, contents),
  Sort & filter and Find & select, laid out like Excel.
- **Row and column grouping** (`outlineOps.ts`): Data ▸ Outline ▸ Group and View ▸ Group. Groups nest up to
  8 levels, cannot partly overlap, collapse/expand by hiding rows or columns, and offer "Show level N" like
  Excel's outline buttons. Groups are stored on the sheet as `adrientOutline`. There are no +/- markers in the
  grid margin yet: groups are toggled from the menu.
- **Functions**: Insert ▸ Function now lists 517 functions (was 510; both counts include the parser forms LAMBDA,
  LET and ARRAYFORMULA). Added: TO_DATE, TO_DOLLARS, TO_PERCENT, TO_PURE_NUMBER,
  TO_TEXT, TRIMRANGE, PHONETIC were added (`internal/sheets/formula_wave7.go`). The Google Sheets operator
  functions (ADD, DIVIDE, EQ …) were left out on purpose: Excel has none and the parity fixtures expect #NAME?.
- Not built: slicers (the menu item stays disabled).
