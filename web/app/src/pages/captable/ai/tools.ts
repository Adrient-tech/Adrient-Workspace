/**
 * Cap table assistant tools. Read only: share counts, percentages and prices come from the server's exact integer maths and the
 * assistant never calculates or estimates ownership itself. Round and exit models are previews (nothing is saved).
 */
import type { AssistantTool } from "@/lib/ai/types";
import { bizTool, capList, int, obj, str } from "@/lib/ai/biz";
import { captable } from "../api";
import { fmtMoney, fmtPct, parseExit } from "../logic";

export const CAPTABLE_SUGGESTIONS = ["Who owns what today?", "What if we raise at a 40 crore pre-money?", "What does each holder get in a 150 crore sale?", "Whose options vest in the next 6 months?"];
export const CAPTABLE_HINT = "All share counts, percentages and prices come from tool results; never calculate or estimate ownership yourself. You cannot change equity records.";

export function createCapTableTools(): AssistantTool[] {
  const A = "captable";
  return [
    bizTool(A, {
      name: "captable_summary",
      description: "The cap table today (or on a date): shares outstanding, fully diluted count, unallocated pool and each holder's ownership.",
      sensitive: true,
      parameters: obj({ as_of: str("YYYY-MM-DD, default today") }),
      run: async (a) => {
        const t = await captable.table(a.as_of ? String(a.as_of) : "");
        return { result: { as_of: t.as_of, outstanding: t.outstanding, fully_diluted: t.fully_diluted, pool_unallocated: t.pool_unallocated, convertibles_open: fmtMoney(t.convertible_cents),
          holders: capList(t.rows.map((r) => ({ name: r.name, type: r.kind, shares: r.stock, options: r.options, warrants: r.warrants, pct_outstanding: fmtPct(r.pct_outstanding, 4), pct_fully_diluted: fmtPct(r.pct_fully_diluted, 4) })), 40) } };
      },
    }),
    bizTool(A, {
      name: "captable_holder",
      description: "One holder's equity: shares, options with vesting, and ownership.",
      sensitive: true,
      parameters: obj({ name: str("Part of the holder's name") }, ["name"]),
      run: async (a) => {
        const list = (await captable.stakeholders()).filter((s) => s.name.toLowerCase().includes(String(a.name).toLowerCase()));
        if (list.length !== 1) throw new Error(list.length ? `Several holders match: ${list.map((s) => s.name).join(", ")}` : "No holder with that name");
        const s = await captable.statement(list[0].id);
        return { result: { name: s.holder.name, shares: s.stock, options: s.options, vested_options: s.vested_options, pct_outstanding: fmtPct(s.pct_outstanding, 4), pct_fully_diluted: fmtPct(s.pct_fully_diluted, 4),
          securities: s.securities.map((x) => ({ label: x.label, kind: x.kind, class: x.class, quantity: x.quantity, outstanding: x.outstanding, vested: x.vested, exercisable: x.exercisable, price: x.price, issued: x.issue_date })) } };
      },
    }),
    bizTool(A, {
      name: "captable_upcoming",
      description: "Cliffs, full-vest dates and option expiries in the next 6 months.",
      sensitive: true,
      run: async () => ({ result: capList((await captable.dashboard()).upcoming, 30) }),
    }),
    bizTool(A, {
      name: "captable_model_round",
      description: "Preview a priced round: price per share, new shares, SAFE and note conversions, pool top-up and every holder's ownership before and after. Nothing is saved. Amounts are in rupees.",
      parameters: obj({ pre_money: str("Pre-money valuation, e.g. 40 crore"), investment: str("New money, e.g. 5 crore"), pool_target_percent: int("Unallocated pool after the round as a percent of fully diluted, 0 for none") }, ["pre_money", "investment"]),
      run: async (a) => {
        const pre = parseExit(String(a.pre_money)), inv = parseExit(String(a.investment));
        if (pre === null || inv === null) throw new Error("I cannot read those amounts; try 40 crore or 4,00,00,000");
        const m = await captable.modelRound({ pre_money: pre, investment: inv, pool_target_bp: Math.round(Number(a.pool_target_percent ?? 0) * 100) });
        return { result: { price_per_share: `${(Number(m.price_per_share_cents) / 100).toFixed(4)} rupees`, new_investor_shares: m.new_investor_shares, pool_top_up: m.pool_top_up, post_fully_diluted: m.post_fully_diluted, post_money: fmtMoney(m.post_money_cents),
          converted: (m.converted ?? []).map((c) => ({ name: c.name, shares: c.shares, price_cents: c.price_cents, cap_binds: c.cap_binds })),
          holders: m.holders.map((h) => ({ name: h.name, before: fmtPct(h.pct_before, 4), after: fmtPct(h.pct_after, 4), shares_after: h.after })) } };
      },
    }),
    bizTool(A, {
      name: "captable_model_exit",
      description: "Preview the payout to every holder if the company is sold for an amount. Notes are repaid first, then preferences, then the rest per share. Nothing is saved.",
      sensitive: true,
      parameters: obj({ exit_value: str("Sale price, e.g. 150 crore") }, ["exit_value"]),
      run: async (a) => {
        const v = parseExit(String(a.exit_value));
        if (v === null) throw new Error("I cannot read that amount; try 150 crore");
        const [o] = await captable.modelExit([v]);
        return { result: { exit: fmtMoney(o.exit), common_per_share_cents: o.common_per_share_cents, holders: o.holders.map((h) => ({ name: h.name, payout: fmtMoney(h.payout), pct_of_exit: `${h.pct}%` })), notes: o.notes } };
      },
    }),
  ];
}
