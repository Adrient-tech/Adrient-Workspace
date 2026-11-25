// Pure logic behind the Ctrl-K command bar: which apps/actions match, how the
// server search groups and directory people are flattened, and the order.
import type { AppTile } from "../../catalog/apps";
import type { SearchGroup } from "../../api/types";
import type { Member } from "../../api/directory";

export type PaletteKind = "app" | "action" | "file" | "person" | "ask";

export interface PaletteItem {
  key: string;
  kind: PaletteKind;
  title: string;
  subtitle?: string;
  /** Adrient glyph key for the leading icon. */
  glyph: string;
  /** App id for apps/actions, result URL for files, email for people, question for ask. */
  target: string;
}

export interface PaletteAction {
  id: string;
  label: string;
  hint: string;
  glyph: string;
  /** The app this action needs; hidden when that app is off. */
  appId: string;
  keywords: string[];
}

/** matchRank: 0 exact word start / prefix, 1 substring, 2 loose keyword, -1 no match. */
export function matchRank(query: string, ...texts: (string | undefined)[]): number {
  const q = query.trim().toLowerCase();
  if (!q) return 0;
  let best = -1;
  for (const t of texts) {
    if (!t) continue;
    const s = t.toLowerCase();
    let r = -1;
    if (s === q || s.startsWith(q)) r = 0;
    else if (s.split(/[\s\-_/]+/).some((w) => w.startsWith(q))) r = 0;
    else if (s.includes(q)) r = 1;
    if (r >= 0 && (best < 0 || r < best)) best = r;
  }
  return best;
}

/** prefersAsk is true when the text reads like a question rather than a lookup. */
export function prefersAsk(query: string): boolean {
  const q = query.trim();
  if (!q) return false;
  if (q.endsWith("?")) return true;
  return q.split(/\s+/).length >= 5;
}

export interface BuildInput {
  query: string;
  apps: AppTile[];
  pinned: string[];
  actions: PaletteAction[];
  groups: SearchGroup[];
  people: Member[];
  /** Max rows per section. */
  limit?: number;
}

const RESULT_GLYPH: Record<string, string> = {
  SEARCH_RESULT_TYPE_DRIVE: "drive",
  SEARCH_RESULT_TYPE_DOCS: "docs",
  SEARCH_RESULT_TYPE_SHEETS: "sheets",
  SEARCH_RESULT_TYPE_SLIDES: "slides",
  SEARCH_RESULT_TYPE_CONTACTS: "contacts",
  SEARCH_RESULT_TYPE_KEEP: "keep",
  SEARCH_RESULT_TYPE_CALENDAR: "calendar",
  SEARCH_RESULT_TYPE_MAIL: "mail",
};

/** buildItems returns the flat, ordered list shown in the palette. */
export function buildItems(inp: BuildInput): PaletteItem[] {
  const limit = inp.limit ?? 6;
  const q = inp.query.trim();
  const visibleIds = new Set(inp.apps.map((a) => a.id));

  const appItems = inp.apps
    .map((a) => ({ a, r: matchRank(q, a.name, a.id, a.blurb) }))
    .filter((x) => (q ? x.r >= 0 : true))
    .sort((x, y) => {
      if (x.r !== y.r) return x.r - y.r;
      const px = inp.pinned.indexOf(x.a.id);
      const py = inp.pinned.indexOf(y.a.id);
      return (px < 0 ? 999 : px) - (py < 0 ? 999 : py);
    })
    .slice(0, q ? limit : 8)
    .map(
      ({ a }): PaletteItem => ({
        key: `app:${a.id}`,
        kind: "app",
        title: a.name,
        subtitle: a.blurb,
        glyph: a.iconName,
        target: a.id,
      }),
    );

  const actionItems = inp.actions
    .filter((a) => visibleIds.has(a.appId))
    .filter((a) => !q || matchRank(q, a.label, ...a.keywords) >= 0)
    .slice(0, q ? limit : 4)
    .map(
      (a): PaletteItem => ({
        key: `action:${a.id}`,
        kind: "action",
        title: a.label,
        subtitle: a.hint,
        glyph: a.glyph,
        target: a.id,
      }),
    );

  const fileItems: PaletteItem[] = q
    ? inp.groups
        .flatMap((g) => g.results)
        .slice(0, limit * 2)
        .map((r) => ({
          key: `file:${r.type}:${r.id}`,
          kind: "file" as const,
          title: r.title || "Untitled",
          subtitle: r.snippet || undefined,
          glyph: RESULT_GLYPH[r.type] ?? "drive",
          target: r.url,
        }))
    : [];

  const peopleItems: PaletteItem[] = q
    ? inp.people.slice(0, limit).map((m) => ({
        key: `person:${m.id}`,
        kind: "person" as const,
        title: m.name || m.email,
        subtitle: m.name ? m.email : undefined,
        glyph: "contacts",
        target: m.email,
      }))
    : [];

  const ask: PaletteItem[] = q
    ? [{ key: "ask", kind: "ask", title: `Ask Adrient: ${q}`, subtitle: "Answer from your files, mail and calendar", glyph: "ask", target: q }]
    : [];

  const rest = [...appItems, ...actionItems, ...fileItems, ...peopleItems];
  return prefersAsk(q) || rest.length === 0 ? [...ask, ...rest] : [...rest, ...ask];
}

/** greeting picks the time-of-day greeting for the home screen. */
export function greeting(date: Date, name: string): string {
  const h = date.getHours();
  const part = h < 5 ? "Working late" : h < 12 ? "Good morning" : h < 17 ? "Good afternoon" : "Good evening";
  const first = name.trim().split(/\s+/)[0];
  return first ? `${part}, ${first}` : part;
}
