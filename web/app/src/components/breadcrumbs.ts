import { apps } from "../catalog/apps";

export interface Crumb {
  label: string;
  to: string;
}

const LABELS: Record<string, string> = {
  d: "",
  file: "File",
  new: "New",
  settings: "Settings",
  apps: "All apps",
  "org-chart": "Org chart",
  trash: "Trash",
  recent: "Recent",
  starred: "Starred",
  shared: "Shared with me",
};

/** looksLikeId: uuids, long hex/number ids and base64-ish tokens are not shown. */
function looksLikeId(s: string): boolean {
  return /^[0-9a-f]{8}-[0-9a-f]{4}-/i.test(s) || /^[0-9a-f]{12,}$/i.test(s) || /^\d{5,}$/.test(s) || s.length > 24;
}

function titleCase(s: string): string {
  return decodeURIComponent(s).replace(/[-_]+/g, " ").replace(/^./, (c) => c.toUpperCase());
}

/**
 * breadcrumbsFor builds the trail shown after the title in the top bar. Inside
 * an app the title is the app itself, so the trail holds only the sections
 * below it ("Docs" then "Templates"); on a non-app page (Settings) it holds
 * that page. Ids (document, file, thread) are never shown. The home screen has
 * no trail.
 */
export function breadcrumbsFor(pathname: string, _appName?: string): Crumb[] {
  const segs = pathname.split("/").filter(Boolean);
  if (segs.length === 0) return [];
  const app = apps.find((a) => a.id === segs[0]);
  if (!app) {
    return [{ label: LABELS[segs[0]] ?? titleCase(segs[0]), to: `/${segs[0]}` }];
  }
  const out: Crumb[] = [];
  let acc = `/${segs[0]}`;
  for (const s of segs.slice(1, 3)) {
    acc += `/${s}`;
    if (looksLikeId(s)) break;
    const label = s in LABELS ? LABELS[s] : titleCase(s);
    if (!label) continue;
    out.push({ label, to: acc });
  }
  return out;
}
