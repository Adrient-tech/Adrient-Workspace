// Share and percentage display. All numbers shown come from the server's exact integer maths; nothing is recomputed here
// except display formatting and chart geometry.
export { fmtMoney, parseMoney } from "../legal/logic";

/** 10000000 -> "1,00,00,000" (Indian grouping). */
export function fmtShares(n: number | null | undefined): string {
  if (n === null || n === undefined) return "";
  return new Intl.NumberFormat("en-IN", { maximumFractionDigits: 0 }).format(n);
}

/** Share counts are whole numbers: "1,00,000" or "100000" -> 100000; anything else -> null. */
export function parseShares(s: string): number | null {
  const t = s.replace(/[,\s]/g, "");
  return /^\d{1,13}$/.test(t) ? Number(t) : null;
}

/** The server sends percentages in units of 0.0001% (1,000,000 = 100%): 166667 -> "16.6667%". */
export function fmtPct(v: number, digits = 2): string {
  const whole = Math.trunc(v / 10000);
  const frac = String(v % 10000).padStart(4, "0").slice(0, digits);
  return digits > 0 ? `${whole}.${frac}%` : `${whole}%`;
}

/** Decimal price string "12.5" -> "₹12.50"; keeps up to 6 decimals when the price needs them. */
export function fmtPrice(p: string, currency = "INR"): string {
  if (p === "" || p === undefined) return "";
  const [w, f = ""] = p.split(".");
  const sig = f.replace(/0+$/, "").length;
  const frac = f.padEnd(2, "0").slice(0, sig > 2 ? Math.min(6, sig) : 2);
  let whole: string;
  try {
    whole = new Intl.NumberFormat("en-IN", { style: "currency", currency, maximumFractionDigits: 0 }).format(Number(w));
  } catch {
    whole = w;
  }
  return `${whole}.${frac}`;
}

export const KIND_LABEL: Record<string, string> = { founder: "Founder", employee: "Employee", investor: "Investor", advisor: "Advisor", entity: "Entity", pool: "Option pool" };
export const SEC_LABEL: Record<string, string> = { stock: "Shares", option: "Options", warrant: "Warrant", safe: "SAFE", note: "Convertible note" };

export const PALETTE = ["#126C5A", "#2F80ED", "#E2A03F", "#9B51E0", "#EB5757", "#27AE60", "#56CCF2", "#BB6BD9", "#F2994A", "#6FCF97", "#828282", "#C0392B"];

export interface Slice { label: string; value: number; color: string }

/** Builds donut slices from (label, value) pairs; values <= 0 are dropped. */
export function slices(items: { label: string; value: number }[]): Slice[] {
  return items.filter((i) => i.value > 0).map((i, k) => ({ ...i, color: PALETTE[k % PALETTE.length] }));
}

/** SVG path for one donut arc (angles in turns, 0 = top). */
export function arcPath(cx: number, cy: number, r0: number, r1: number, from: number, to: number): string {
  const full = to - from >= 0.99999;
  const t = full ? from + 0.99999 : to;
  const pt = (r: number, a: number) => [cx + r * Math.sin(a * 2 * Math.PI), cy - r * Math.cos(a * 2 * Math.PI)];
  const [x0, y0] = pt(r1, from), [x1, y1] = pt(r1, t), [x2, y2] = pt(r0, t), [x3, y3] = pt(r0, from);
  const large = t - from > 0.5 ? 1 : 0;
  return `M${x0.toFixed(2)},${y0.toFixed(2)} A${r1},${r1} 0 ${large} 1 ${x1.toFixed(2)},${y1.toFixed(2)} L${x2.toFixed(2)},${y2.toFixed(2)} A${r0},${r0} 0 ${large} 0 ${x3.toFixed(2)},${y3.toFixed(2)} Z`;
}

/** An exit value such as "150 cr", "2.5 crore", "40 lakh", "1,00,000" or "1b", as exact paise (null if unreadable). */
export function parseExit(s: string): number | null {
  const m = /^\s*₹?\s*([\d,]*\.?\d+)\s*(cr|crore|crores|l|lakh|lakhs|lac|k|m|mn|b|bn)?\s*$/i.exec(s);
  if (!m) return null;
  const mult: Record<string, number> = { cr: 1e7, crore: 1e7, crores: 1e7, l: 1e5, lakh: 1e5, lakhs: 1e5, lac: 1e5, k: 1e3, m: 1e6, mn: 1e6, b: 1e9, bn: 1e9 };
  const [w, f = ""] = m[1].replace(/,/g, "").split(".");
  const unit = BigInt(mult[(m[2] ?? "").toLowerCase()] ?? 1);
  const rupees = BigInt(w || "0") * unit + (f ? (BigInt(f) * unit) / BigInt(10) ** BigInt(f.length) : BigInt(0));
  const paise = rupees * BigInt(100);
  return paise <= BigInt(Number.MAX_SAFE_INTEGER) ? Number(paise) : null;
}
