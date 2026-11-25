// Mail search operators: from:, to:, has:attachment, before:, after:, label:,
// is:unread|read|starred|flagged, subject:. Free text is sent to the server;
// the operators are applied to the loaded threads on the client.
import type { MailThread } from "./types";

export interface ParsedSearch {
  text: string;
  from: string[];
  to: string[];
  subject: string[];
  label: string[];
  hasAttachment: boolean;
  isUnread?: boolean;
  isStarred?: boolean;
  before?: Date;
  after?: Date;
}

function parseDate(v: string): Date | undefined {
  const m = /^(\d{4})[-/](\d{1,2})[-/](\d{1,2})$/.exec(v);
  if (!m) return undefined;
  const d = new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]));
  return Number.isNaN(d.getTime()) ? undefined : d;
}

/** Split on spaces, keeping "quoted phrases" and op:"quoted value" together. */
function tokenize(q: string): string[] {
  const out: string[] = [];
  const re = /([a-z]+:)?"([^"]*)"|\S+/gi;
  let m: RegExpExecArray | null;
  while ((m = re.exec(q))) out.push(m[2] !== undefined ? `${m[1] ?? ""}${m[2]}` : m[0]);
  return out;
}

export function parseSearch(q: string): ParsedSearch {
  const p: ParsedSearch = { text: "", from: [], to: [], subject: [], label: [], hasAttachment: false };
  const free: string[] = [];
  for (const tok of tokenize(q)) {
    const i = tok.indexOf(":");
    const key = i > 0 ? tok.slice(0, i).toLowerCase() : "";
    const val = i > 0 ? tok.slice(i + 1) : "";
    if (!val) {
      free.push(tok);
      continue;
    }
    switch (key) {
      case "from": p.from.push(val.toLowerCase()); break;
      case "to": p.to.push(val.toLowerCase()); break;
      case "subject": p.subject.push(val.toLowerCase()); break;
      case "label": p.label.push(val.toLowerCase()); break;
      case "has":
        if (val.toLowerCase() === "attachment" || val.toLowerCase() === "attachments") p.hasAttachment = true;
        else free.push(tok);
        break;
      case "is": {
        const v = val.toLowerCase();
        if (v === "unread") p.isUnread = true;
        else if (v === "read") p.isUnread = false;
        else if (v === "starred" || v === "flagged") p.isStarred = true;
        else free.push(tok);
        break;
      }
      case "before": {
        const d = parseDate(val);
        if (d) p.before = d; else free.push(tok);
        break;
      }
      case "after": {
        const d = parseDate(val);
        if (d) p.after = d; else free.push(tok);
        break;
      }
      default: free.push(tok);
    }
  }
  p.text = free.join(" ");
  return p;
}

export function hasOperators(p: ParsedSearch): boolean {
  return (
    p.from.length + p.to.length + p.subject.length + p.label.length > 0 ||
    p.hasAttachment || p.isUnread !== undefined || p.isStarred !== undefined || !!p.before || !!p.after
  );
}

export function matchesThread(t: MailThread, p: ParsedSearch): boolean {
  const m = t.latest;
  const sent = new Date(m.sent_at).getTime();
  const from = `${m.from_name ?? ""} ${m.from_addr}`.toLowerCase();
  const to = [...(m.to_addrs ?? []), ...(m.cc_addrs ?? [])].join(" ").toLowerCase();
  const labels = [...(t.labels ?? []), ...(m.labels ?? [])].map((l) => l.toLowerCase());
  if (!p.from.every((f) => from.includes(f))) return false;
  if (!p.to.every((f) => to.includes(f))) return false;
  if (!p.subject.every((s) => (m.subject ?? "").toLowerCase().includes(s))) return false;
  if (!p.label.every((l) => labels.includes(l))) return false;
  if (p.hasAttachment && !(m.attachments ?? []).length) return false;
  if (p.isUnread !== undefined && t.any_unread !== p.isUnread) return false;
  if (p.isStarred && !t.starred) return false;
  if (p.before && !(sent < p.before.getTime())) return false;
  if (p.after && !(sent >= p.after.getTime() + 0)) return false;
  return true;
}

export function filterThreads(threads: MailThread[], p: ParsedSearch): MailThread[] {
  return hasOperators(p) ? threads.filter((t) => matchesThread(t, p)) : threads;
}

export type QuickFilter = "unread" | "tome" | "flagged" | "attachments" | "mentions";

export function applyQuickFilters(
  threads: MailThread[],
  active: Set<QuickFilter>,
  myEmail: string,
  myName = "",
): MailThread[] {
  if (active.size === 0) return threads;
  const me = myEmail.toLowerCase();
  const handle = (myName || me.split("@")[0]).toLowerCase();
  return threads.filter((t) => {
    const m = t.latest;
    if (active.has("unread") && !t.any_unread) return false;
    if (active.has("flagged") && !t.starred) return false;
    if (active.has("attachments") && !(m.attachments ?? []).length) return false;
    if (active.has("tome") && !(m.to_addrs ?? []).some((a) => a.toLowerCase() === me)) return false;
    if (active.has("mentions") && !`${m.body ?? ""} ${m.snippet ?? ""}`.toLowerCase().includes("@" + handle)) return false;
    return true;
  });
}

export type SortKey = "date-desc" | "date-asc" | "from" | "subject" | "unread-first";

export function sortThreads(threads: MailThread[], key: SortKey, pinned: (t: MailThread) => boolean): MailThread[] {
  const out = [...threads];
  const time = (t: MailThread) => new Date(t.latest.sent_at).getTime();
  out.sort((a, b) => {
    const pa = pinned(a) ? 0 : 1;
    const pb = pinned(b) ? 0 : 1;
    if (pa !== pb) return pa - pb;
    switch (key) {
      case "date-asc": return time(a) - time(b);
      case "from": return (a.latest.from_name || a.latest.from_addr).localeCompare(b.latest.from_name || b.latest.from_addr);
      case "subject": return (a.latest.subject ?? "").localeCompare(b.latest.subject ?? "");
      case "unread-first": return Number(b.any_unread) - Number(a.any_unread) || time(b) - time(a);
      default: return time(b) - time(a);
    }
  });
  return out;
}
