// Renewal urgency grouping and wording. The date maths runs on the server (internal/contracts/dates.go).
import type { Contract, ObItem, Renewal } from "./api";

export const TYPE_LABEL: Record<string, string> = { service: "Service agreement", vendor: "Vendor / supplier", nda: "NDA", employment: "Employment", lease: "Lease", license: "License / SaaS", other: "Other" };
export const PHASE_LABEL: Record<Renewal["phase"], string> = { none: "", evergreen: "No end date", expired: "Past its end date", notice: "Notice window open", soon: "Ends soon", ok: "On track" };
export const PHASE_COLOR: Record<Renewal["phase"], "neutral" | "danger" | "warning" | "success" | "primary"> = { none: "neutral", evergreen: "neutral", expired: "danger", notice: "warning", soon: "primary", ok: "success" };

export function relativeDays(d: number): string {
  if (d === 0) return "today";
  if (d === 1) return "tomorrow";
  if (d === -1) return "yesterday";
  return d > 0 ? `in ${d} days` : `${-d} days ago`;
}

export type Group = { key: string; label: string; items: Contract[] };
/** Renewals grouped by urgency, most urgent first. Contracts with no end date are left out. */
export function renewalGroups(list: Contract[]): Group[] {
  const g: Group[] = [
    { key: "attention", label: "Needs a decision", items: [] },
    { key: "30", label: "Next 30 days", items: [] },
    { key: "90", label: "Next 90 days", items: [] },
    { key: "later", label: "Later", items: [] },
    { key: "handled", label: "Decided", items: [] },
  ];
  for (const c of list) {
    if (c.status !== "active" || !c.renewal.date) continue;
    const d = c.renewal.days_left ?? 0;
    if (c.renewal.attention) g[0].items.push(c);
    else if (c.renewal.handled && d <= 90) g[4].items.push(c);
    else if (d <= 30) g[1].items.push(c);
    else if (d <= 90) g[2].items.push(c);
    else g[3].items.push(c);
  }
  const by = (a: Contract, b: Contract) => (a.renewal.date ?? "").localeCompare(b.renewal.date ?? "");
  g.forEach((x) => x.items.sort(by));
  return g.filter((x) => x.items.length);
}

export function groupObligations(items: ObItem[]): { key: string; label: string; items: ObItem[] }[] {
  const defs: [string, string][] = [["overdue", "Overdue"], ["soon", "Due this week"], ["later", "Later"], ["undated", "No date"]];
  return defs.map(([key, label]) => ({ key, label, items: items.filter((i) => i.urgency === key) })).filter((g) => g.items.length);
}

export function repeatLabel(every?: string): string {
  return ({ monthly: "every month", quarterly: "every 3 months", annually: "every year" } as Record<string, string>)[every ?? ""] ?? "";
}

/** One-line summary of the renewal clock for a list row. */
export function clockText(r: Renewal): string {
  if (r.phase === "none") return "";
  if (r.phase === "evergreen") return "No end date";
  const when = r.days_left !== undefined ? relativeDays(r.days_left) : "";
  if (r.phase === "expired") return `Ended ${when}`;
  if (r.notice_date && r.notice_days_left !== undefined && r.notice_days_left >= 0 && !r.handled) return `Notice by ${r.notice_date} (${relativeDays(r.notice_days_left)})`;
  return `Ends ${when}`;
}

export function csvEscape(v: string): string {
  const s = /^[=+\-@\t\r]/.test(v) && !/^-?\d+([.,]\d+)?$/.test(v) ? "'" + v : v;
  return /[",\n\r]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
}
export function contractsCsv(list: Contract[]): string {
  const head = ["Title", "Type", "Status", "Counterparty", "Value", "Currency", "Billing", "Start", "End", "Auto-renew", "Notice days", "Next date", "Tags"];
  const rows = list.map((c) => [c.title, TYPE_LABEL[c.type] ?? c.type, c.status, c.party_label, c.value.toFixed(2), c.currency, c.billing, c.start_date, c.end_date, c.auto_renew ? "yes" : "no", String(c.notice_days), c.renewal.date ?? "", c.tags.join("; ")]);
  return [head, ...rows].map((r) => r.map(csvEscape).join(",")).join("\r\n") + "\r\n";
}
