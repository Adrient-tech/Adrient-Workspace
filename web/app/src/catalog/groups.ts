/**
 * App groups for the launcher, dock and icon colours.
 *
 * Every catalog app belongs to exactly one group. The group decides the icon
 * colour family (see components/icons/groups.ts) and the section it appears
 * in on the home screen and the full app grid. Unknown ids (a new catalog
 * entry nobody grouped yet) fall into "more".
 */
export type AppGroup = "office" | "communicate" | "business" | "admin" | "more";

export const GROUP_ORDER: AppGroup[] = ["office", "communicate", "business", "admin", "more"];

export const GROUP_LABEL: Record<AppGroup, string> = {
  office: "Office",
  communicate: "Communicate",
  business: "Business",
  admin: "Admin",
  more: "More",
};

const MEMBERS: Record<AppGroup, string[]> = {
  office: ["drive", "docs", "sheets", "slides", "pdf", "forms", "keep", "whiteboard", "wiki", "tasks"],
  communicate: ["mail", "chat", "meet", "calendar", "contacts", "telephony", "groups", "tickets", "live"],
  business: [
    "crm", "invoicing", "accounting", "pos", "inventory", "hr", "talent", "shifts", "timetracking",
    "projects", "contracts", "signatures", "legal", "captable", "records", "fieldservice", "fleet", "sites", "workspace", "ask",
  ],
  admin: ["admin", "access", "vpn", "git", "cloudimport", "orgsync", "homeassistant", "assemble", "spacelight"],
  more: [
    "photos", "music", "video", "books", "games", "podcasts", "translate", "3d", "archaeology", "maps", "learn",
  ],
};

const BY_ID: Record<string, AppGroup> = {};
for (const g of GROUP_ORDER) for (const id of MEMBERS[g]) BY_ID[id] = g;

/** groupOf returns the launcher group of an app id ("more" when ungrouped). */
export function groupOf(id: string): AppGroup {
  return BY_ID[id] ?? "more";
}

/** groupApps buckets apps by group in GROUP_ORDER, dropping empty groups.
 *  Inside a group the order of MEMBERS is kept (most used first), then any
 *  ungrouped extras in catalog order. */
export function groupApps<T extends { id: string }>(items: T[]): { group: AppGroup; items: T[] }[] {
  const out: { group: AppGroup; items: T[] }[] = [];
  for (const group of GROUP_ORDER) {
    const inGroup = items.filter((a) => groupOf(a.id) === group);
    const rank = (id: string) => {
      const i = MEMBERS[group].indexOf(id);
      return i < 0 ? 999 : i;
    };
    inGroup.sort((a, b) => rank(a.id) - rank(b.id));
    if (inGroup.length) out.push({ group, items: inGroup });
  }
  return out;
}
