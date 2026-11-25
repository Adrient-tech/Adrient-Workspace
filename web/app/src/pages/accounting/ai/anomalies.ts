import type { Journal, Account } from "../types";

export interface Finding { kind: string; severity: "high" | "medium" | "low"; detail: string; journal_ids: string[] }

const r2 = (n: number) => Math.round((n + Number.EPSILON) * 100) / 100;

/** Rule-based checks over posted journals. Transparent and deterministic: the model explains, it does not detect. */
export function findAnomalies(journals: Journal[], accounts: Account[]): Finding[] {
  const out: Finding[] = [];
  const live = journals.filter((j) => !j.voided);
  const acct = new Map(accounts.map((a) => [a.id, a]));

  // 1. Unbalanced entries.
  for (const j of live) {
    const d = j.lines.reduce((s, l) => s + l.debit, 0), c = j.lines.reduce((s, l) => s + l.credit, 0);
    if (Math.abs(d - c) > 0.005) out.push({ kind: "unbalanced", severity: "high", detail: `Entry #${j.number} on ${j.date}: debits ${r2(d)} vs credits ${r2(c)}`, journal_ids: [j.id] });
  }
  // 2. Possible duplicates: same date, contact and total.
  const seen = new Map<string, Journal[]>();
  for (const j of live) {
    if (j.total <= 0) continue;
    const k = `${j.date}|${j.contact_id || j.contact_name}|${r2(j.total)}`;
    seen.set(k, [...(seen.get(k) ?? []), j]);
  }
  for (const [k, js] of seen) if (js.length > 1) out.push({ kind: "possible_duplicate", severity: "medium", detail: `${js.length} entries on ${k.split("|")[0]} for ${js[0].contact_name || "no contact"} with the same total ${r2(js[0].total)} (#${js.map((x) => x.number).join(", #")})`, journal_ids: js.map((x) => x.id) });
  // 3. Outliers per expense/income account: > mean + 3 sd with at least 8 samples.
  const byAcct = new Map<string, { j: Journal; v: number }[]>();
  for (const j of live) for (const l of j.lines) {
    const a = acct.get(l.account_id);
    if (!a || (a.type !== "expense" && a.type !== "income")) continue;
    const v = Math.max(l.debit, l.credit);
    byAcct.set(a.id, [...(byAcct.get(a.id) ?? []), { j, v }]);
  }
  for (const [id, rows] of byAcct) {
    if (rows.length < 8) continue;
    const mean = rows.reduce((s, r) => s + r.v, 0) / rows.length;
    const sd = Math.sqrt(rows.reduce((s, r) => s + (r.v - mean) ** 2, 0) / rows.length);
    if (sd === 0) continue;
    for (const r of rows) if (r.v > mean + 3 * sd) out.push({ kind: "outlier", severity: "medium", detail: `${r2(r.v)} in "${acct.get(id)!.name}" on ${r.j.date} (#${r.j.number}) is far above this account's usual ${r2(mean)}`, journal_ids: [r.j.id] });
  }
  // 4. Large round amounts on expenses.
  for (const j of live) if (j.total >= 50000 && j.total % 10000 === 0 && j.lines.some((l) => acct.get(l.account_id)?.type === "expense")) out.push({ kind: "round_amount", severity: "low", detail: `Round expense total ${r2(j.total)} on ${j.date} (#${j.number}, ${j.memo || "no memo"})`, journal_ids: [j.id] });
  // 5. Missing memo on big entries.
  for (const j of live) if (j.total >= 25000 && !j.memo.trim() && !j.ref.trim()) out.push({ kind: "no_description", severity: "low", detail: `Entry #${j.number} on ${j.date} for ${r2(j.total)} has no memo or reference`, journal_ids: [j.id] });
  const rank = { high: 0, medium: 1, low: 2 };
  return out.sort((a, b) => rank[a.severity] - rank[b.severity]);
}

/** A proposed journal is valid when it has 2+ lines, known accounts, one-sided positive amounts and balances. */
export function validateJournal(lines: { account_id: string; debit: number; credit: number }[], accounts: Account[]): string {
  if (lines.length < 2) return "A journal needs at least two lines.";
  const ids = new Set(accounts.map((a) => a.id));
  for (const l of lines) {
    if (!ids.has(l.account_id)) return `Unknown account id ${l.account_id}.`;
    if (!(l.debit >= 0) || !(l.credit >= 0) || (l.debit > 0) === (l.credit > 0)) return "Each line needs either a debit or a credit greater than zero, not both.";
  }
  const d = lines.reduce((s, l) => s + l.debit, 0), c = lines.reduce((s, l) => s + l.credit, 0);
  if (Math.abs(d - c) > 0.005) return `The journal does not balance: debits ${r2(d)}, credits ${r2(c)}.`;
  return "";
}
