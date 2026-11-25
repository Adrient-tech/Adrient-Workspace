import { describe, expect, it } from "vitest";
import type { Contract, ObItem } from "./api";
import { clockText, contractsCsv, csvEscape, groupObligations, relativeDays, renewalGroups } from "./logic";

const c = (o: Partial<Contract> & { renewal: Contract["renewal"] }): Contract => ({ id: Math.random().toString(), title: "T", type: "service", status: "active", party_name: "", party_label: "P", value: 0, currency: "INR", billing: "monthly", start_date: "", end_date: "", auto_renew: false, term_months: 12, notice_days: 0, owner_id: "", access: "org", shared_with: [], remind: true, tags: [], notes: "", annual_value: 0, can_edit: true, updated_at: "", ...o });

describe("renewal groups", () => {
  it("groups by urgency and skips evergreen and inactive", () => {
    const list = [
      c({ renewal: { phase: "notice", date: "2026-04-01", days_left: 10, handled: false, attention: true } }),
      c({ renewal: { phase: "soon", date: "2026-04-20", days_left: 25, handled: false, attention: false } }),
      c({ renewal: { phase: "ok", date: "2026-06-01", days_left: 80, handled: false, attention: false } }),
      c({ renewal: { phase: "ok", date: "2027-06-01", days_left: 400, handled: false, attention: false } }),
      c({ renewal: { phase: "notice", date: "2026-04-05", days_left: 14, handled: true, attention: false } }),
      c({ renewal: { phase: "evergreen", handled: false, attention: false } }),
      c({ status: "terminated", renewal: { phase: "none", handled: false, attention: false } }),
    ];
    const g = renewalGroups(list);
    expect(g.map((x) => [x.key, x.items.length])).toEqual([["attention", 1], ["30", 1], ["90", 1], ["later", 1], ["handled", 1]]);
  });
});

describe("text helpers", () => {
  it("words days", () => {
    expect(relativeDays(0)).toBe("today");
    expect(relativeDays(3)).toBe("in 3 days");
    expect(relativeDays(-2)).toBe("2 days ago");
  });
  it("describes the clock", () => {
    expect(clockText({ phase: "evergreen", handled: false, attention: false })).toBe("No end date");
    expect(clockText({ phase: "notice", date: "2026-05-01", days_left: 20, notice_date: "2026-04-25", notice_days_left: 4, handled: false, attention: true })).toBe("Notice by 2026-04-25 (in 4 days)");
    expect(clockText({ phase: "expired", date: "2026-01-01", days_left: -5, handled: false, attention: true })).toBe("Ended 5 days ago");
  });
  it("groups obligations", () => {
    const o = (urgency: ObItem["urgency"]) => ({ urgency } as ObItem);
    expect(groupObligations([o("later"), o("overdue"), o("overdue")]).map((g) => [g.key, g.items.length])).toEqual([["overdue", 2], ["later", 1]]);
  });
});

describe("csv", () => {
  it("quotes and neutralises formulas", () => {
    expect(csvEscape("a,b")).toBe('"a,b"');
    expect(csvEscape("=SUM(A1)")).toBe("'=SUM(A1)");
    expect(csvEscape("-5")).toBe("-5");
    expect(contractsCsv([c({ title: '=cmd', renewal: { phase: "ok", handled: false, attention: false } })])).toContain("'=cmd");
  });
});
