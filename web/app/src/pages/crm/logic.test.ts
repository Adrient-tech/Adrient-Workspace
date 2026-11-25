import { describe, expect, it } from "vitest";
import { forecast, funnel, groupFollowups, isCold, mrr, parseCSV, parseDealsCSV, stageWeight, toCSV, weekDays, winLoss, wonThisMonth } from "./logic";
import type { Deal, Followup, Settings } from "./types";

const S: Settings = {
  stages: [
    { id: "new", name: "New" },
    { id: "qual", name: "Qualified" },
    { id: "prop", name: "Proposal" },
  ],
  currency: "INR",
  monthly_goal: 0,
  stale_days: 14,
  cold_days: 14,
  weekly_target: 0,
};
const deal = (o: Partial<Deal>): Deal => ({
  id: "d",
  title: "t",
  company_id: null,
  contact_id: null,
  stage: "new",
  value: 100,
  currency: "INR",
  billing: "one_time",
  expected_close: null,
  source: "",
  notes: "",
  close_reason: "",
  closed_at: null,
  stage_entered_at: "2026-01-01T00:00:00Z",
  created_at: "",
  updated_at: "",
  committee: [],
  ...o,
});

describe("mrr", () => {
  it("normalises billing intervals", () => {
    expect(mrr({ value: 1200, billing: "yearly" })).toBe(100);
    expect(mrr({ value: 300, billing: "quarterly" })).toBe(100);
    expect(mrr({ value: 50, billing: "monthly" })).toBe(50);
    expect(mrr({ value: 999, billing: "one_time" })).toBe(0);
  });
});

describe("forecast", () => {
  it("weights later stages higher and groups by close month", () => {
    expect(stageWeight(S, "new")).toBe(0.25);
    expect(stageWeight(S, "prop")).toBe(0.75);
    const f = forecast(
      [deal({ id: "a", value: 400, stage: "prop", expected_close: "2026-03-10" }), deal({ id: "b", value: 100, expected_close: "2026-03-20" }), deal({ id: "c", stage: "won", value: 9, billing: "monthly" }), deal({ id: "d", value: 50 })],
      S,
    );
    expect(f.byMonth).toEqual([{ month: "2026-03", value: 500, weighted: 325 }]);
    expect(f.noDate).toBe(50);
    expect(f.openCount).toBe(3);
    expect(f.wonMrr).toBe(9);
  });
  it("sums revenue won this month", () => {
    const n = new Date("2026-05-15T10:00:00");
    expect(wonThisMonth([deal({ stage: "won", closed_at: "2026-05-02T00:00:00Z", value: 70 }), deal({ stage: "won", closed_at: "2026-04-30T00:00:00Z", value: 5 })], n)).toBe(70);
  });
});

describe("funnel", () => {
  it("counts deals that ever reached each stage", () => {
    const deals = [deal({ id: "1", stage: "won" }), deal({ id: "2", stage: "lost" }), deal({ id: "3", stage: "new" })];
    const hist = [
      { id: "h1", deal_id: "2", stage: "new", entered_at: "" },
      { id: "h2", deal_id: "2", stage: "qual", entered_at: "" },
      { id: "h3", deal_id: "2", stage: "lost", entered_at: "" },
      { id: "h4", deal_id: "3", stage: "new", entered_at: "" },
    ];
    const f = funnel(deals, hist, S);
    expect(f.map((r) => r.reached)).toEqual([3, 2, 1]);
    expect(f[0].toNext).toBe(67);
    expect(f[2].toNext).toBeNull();
  });
});

describe("winLoss", () => {
  it("computes win rate and reasons", () => {
    const w = winLoss([deal({ stage: "won", value: 100 }), deal({ stage: "lost", close_reason: "price" }), deal({ stage: "lost", close_reason: "price" }), deal({ stage: "new" })]);
    expect(w.winRate).toBe(33);
    expect(w.lostReasons).toEqual([["price", 2]]);
  });
});

describe("agenda", () => {
  const f = (id: string, due: string, done = false): Followup => ({ id, title: id, due_date: due, due_time: "", deal_id: null, contact_id: null, done, calendar_event_id: "", created_at: "" });
  it("groups into overdue/today/upcoming", () => {
    const g = groupFollowups([f("a", "2026-01-01"), f("b", "2026-01-05"), f("c", "2026-01-09"), f("d", "2026-01-01", true)], "2026-01-05");
    expect(g.overdue.map((x) => x.id)).toEqual(["a"]);
    expect(g.today.map((x) => x.id)).toEqual(["b"]);
    expect(g.upcoming.map((x) => x.id)).toEqual(["c"]);
    expect(g.done).toHaveLength(1);
  });
  it("builds a Monday-first week", () => {
    expect(weekDays("2026-11-18")[0]).toBe("2026-11-16");
    expect(weekDays("2026-11-22")[6]).toBe("2026-11-22");
  });
  it("flags cold contacts", () => {
    const now = Date.now();
    expect(isCold(0, 14, now)).toBe(true);
    expect(isCold(now - 2 * 86400000, 14, now)).toBe(false);
    expect(isCold(now - 20 * 86400000, 14, now)).toBe(true);
  });
});

describe("csv", () => {
  it("round-trips quotes and commas", () => {
    const rows = [["a,b", 'q"x', "n"], ["1", "", "3"]];
    expect(parseCSV(toCSV(rows))).toEqual([["a,b", 'q"x', "n"], ["1", "", "3"]]);
  });
  it("parses deals", () => {
    const r = parseDealsCSV("Title,Company,Value,Stage,expected_close\nBig,Acme,\"1,500\",qual,2026-02-01\n,x,1,,\n");
    expect(r).toEqual([{ title: "Big", company: "Acme", value: 1500, stage: "qual", expected_close: "2026-02-01", source: "" }]);
  });
});
