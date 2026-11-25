// Pure helpers of Field service. computeTotals mirrors the Go Compute.
import type { Discount, Item, Schedule, Totals } from "./api";

export const ACCENT = "#C2410C";
const r2 = (n: number) => Math.round(n * 100) / 100;

export const included = (it: Item) => !it.optional || !!it.selected;

export function computeTotals(items: Item[], discount: Discount | null, taxRate: number): Totals {
  let sub = 0, taxable = 0, cost = 0;
  for (const it of items) {
    if (!included(it)) continue;
    const line = r2(it.qty * it.unit_price);
    sub += line;
    if (it.taxable) taxable += line;
    cost += r2(it.qty * it.unit_cost);
  }
  let disc = 0;
  if (discount && discount.value > 0) {
    disc = discount.kind === "pct" ? r2((sub * Math.min(100, discount.value)) / 100) : r2(discount.value);
    disc = Math.max(0, Math.min(sub, disc));
  }
  const taxAfter = sub > 0 ? taxable - r2((disc * taxable) / sub) : 0;
  const tax = taxRate > 0 ? r2((taxAfter * taxRate) / 100) : 0;
  return { subtotal: r2(sub), discount: disc, tax, total: r2(sub - disc + tax), cost: r2(cost), profit: r2(sub - disc - cost) };
}

const ORD = ["first", "second", "third", "fourth", "fifth"];
const DAYS = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];
/** "Every 2 weeks on Monday, Thursday, 8 visits". */
export function describeSchedule(s: Schedule): string {
  if (!s.repeat || s.repeat === "none") return "One-off";
  const n = Math.max(1, s.interval || 1);
  const unit = ({ daily: "day", weekly: "week", monthly: "month", yearly: "year" } as Record<string, string>)[s.repeat] ?? s.repeat;
  let t = n === 1 ? `Every ${unit}` : `Every ${n} ${unit}s`;
  const d = s.start ? new Date(`${s.start}T00:00:00`) : null;
  if (s.repeat === "weekly") {
    const days = s.weekdays?.length ? [...s.weekdays].sort() : d ? [d.getDay()] : [];
    t += ` on ${days.map((x) => DAYS[x]).join(", ")}`;
  } else if (s.repeat === "monthly" && d) {
    t += s.monthly_by === "weekday" ? ` on the ${ORD[Math.floor((d.getDate() - 1) / 7)]} ${DAYS[d.getDay()]}` : ` on day ${d.getDate()}`;
  }
  if (s.end_type === "count") t += `, ${s.end_count} visit${s.end_count === 1 ? "" : "s"}`;
  if (s.end_type === "until" && s.end_date) t += `, until ${s.end_date}`;
  return t;
}

export const STATUS_LABEL: Record<string, string> = {
  new: "New", assessment: "Assessment", quoted: "Quoted", converted: "Job created", closed: "Closed",
  active: "Active", late: "Late", requires_invoicing: "Needs invoice", completed: "Completed", cancelled: "Cancelled", scheduled: "Scheduled",
};
export const STATUS_COLOR: Record<string, "primary" | "success" | "warning" | "danger" | "neutral"> = {
  new: "primary", assessment: "warning", quoted: "warning", converted: "success", closed: "neutral",
  active: "primary", late: "danger", requires_invoicing: "warning", completed: "success", cancelled: "neutral", scheduled: "primary",
};

export const blankItem = (): Item => ({ name: "", qty: 1, unit_price: 0, unit_cost: 0, taxable: true });
export const blankSchedule = (start: string): Schedule => ({ start, repeat: "none", interval: 1, weekdays: [], monthly_by: "date", end_type: "count", end_count: 12, end_date: "", start_time: "", end_time: "" });
