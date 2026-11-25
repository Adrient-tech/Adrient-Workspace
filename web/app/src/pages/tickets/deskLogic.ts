import type { TicketEvent } from "./deskApi";

export const FILTER_HELP: [string, string][] = [
  ["status:open", "A status of the project, or unsolved / solved"],
  ["assignee:me", "Assigned to you (or none)"],
  ["priority:urgent", "low, normal, high or urgent"],
  ["tag:billing", "Has a tag"],
  ["requester:ana", "Requester name or e-mail"],
  ["type:incident", "question, incident, problem or task"],
  ["via:public", "Channel the ticket came in on (web, public)"],
  ["#123", "Ticket number"],
];

/** Adds a filter to the search text, replacing an existing filter with the same key. */
export function withFilter(q: string, filter: string): string {
  const key = filter.includes(":") ? filter.slice(0, filter.indexOf(":") + 1) : "";
  const parts = q.split(/\s+/).filter(Boolean).filter((p) => !(key && p.toLowerCase().startsWith(key)));
  if (!filter.includes(":") && /^#\d+$/.test(filter)) return [...parts.filter((p) => !/^#\d+$/.test(p)), filter].join(" ");
  return [...parts, filter].join(" ");
}

/** Tags typed as "a, b  c" become a clean list (same rules as the server). */
export function parseTags(text: string): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const raw of text.split(/[,\n]/)) {
    const t = raw.trim().toLowerCase().split(/\s+/).join("-").replace(/^[#,-]+|[,-]+$/g, "");
    if (t && !seen.has(t)) {
      seen.add(t);
      out.push(t.slice(0, 40));
    }
  }
  return out.slice(0, 20);
}

/** One human line per trail entry. */
export function describeEvent(e: Pick<TicketEvent, "kind" | "detail">): string {
  const d = e.detail ?? {};
  const s = (k: string) => String(d[k] ?? "");
  const list = (k: string) => (Array.isArray(d[k]) ? (d[k] as string[]).join(", ") : s(k));
  switch (e.kind) {
    case "status":
      return `changed status from ${s("from")} to ${s("to")}`;
    case "priority":
      return `changed priority from ${s("from")} to ${s("to")}`;
    case "assignee":
      return s("to") ? "changed the assignee" : "removed the assignee";
    case "tags":
      return `set tags to ${list("to") || "none"}`;
    case "type":
      return `changed type from ${s("from")} to ${s("to")}`;
    case "problem":
      return s("to") ? "linked the ticket to a problem" : "unlinked the problem";
    case "merged":
      return `merged ${s("ticket")} into this ticket`;
    case "merged_into":
      return `merged this ticket into ${s("ticket")}`;
    case "macro":
      return `applied the macro "${s("name")}"`;
    default:
      return e.kind;
  }
}

/** Header-checkbox state for a list selection. */
export function selectionState(ids: string[], selected: Set<string>): "none" | "some" | "all" {
  const n = ids.filter((i) => selected.has(i)).length;
  return n === 0 ? "none" : n === ids.length ? "all" : "some";
}

export function toggleAll(ids: string[], selected: Set<string>): Set<string> {
  return selectionState(ids, selected) === "all" ? new Set([...selected].filter((i) => !ids.includes(i))) : new Set([...selected, ...ids]);
}
