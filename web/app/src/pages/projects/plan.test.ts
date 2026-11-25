import { describe, expect, it } from "vitest";
import { dateOfDay, dayNumber, depConflicts, emptyPlan, layoutChart, milestoneProgress, monthSpans, timeSummary, wouldCycle, type IssuePlan } from "./plan";

const P = (o: Partial<IssuePlan>): IssuePlan => ({ ...emptyPlan(), ...o });
const today = new Date(2030, 0, 10); // 10 Jan 2030

describe("projects plan logic", () => {
  it("converts days without drift", () => {
    expect(dateOfDay(dayNumber("2030-03-31"))).toBe("2030-03-31");
    expect(dayNumber("2030-03-31") - dayNumber("2030-03-01")).toBe(30);
    expect(dayNumber("x")).toBeNaN();
  });

  it("lays out bars, unscheduled issues, today and milestones", () => {
    const plans = {
      a: P({ start_date: "2030-01-05", due_date: "2030-01-08" }),
      b: P({ due_date: "2030-01-20" }),
      c: P({}),
      d: P({ start_date: "2030-01-12", due_date: "2030-01-11" }), // due before start -> one day at start
    };
    const c = layoutChart(["a", "b", "c", "d"], plans, [{ id: "m", project_id: "p", name: "Beta", due_date: "2030-01-25", done: false }], today, 2);
    expect(c.unscheduled).toEqual(["c"]);
    expect(c.bars.a.width).toBe(4);
    expect(c.bars.b.width).toBe(1);
    expect(c.bars.d.width).toBe(1);
    expect(c.bars.b.left - c.bars.a.left).toBe(15);
    // The axis covers the earliest bar, the milestone and today, with padding.
    expect(c.startDay).toBe(dayNumber("2030-01-05") - 2);
    expect(c.milestones[0].left).toBe(dayNumber("2030-01-25") - c.startDay);
    expect(c.todayLeft).toBe(dayNumber("2030-01-10") - c.startDay);
    expect(c.days).toBe(dayNumber("2030-01-25") - dayNumber("2030-01-05") + 1 + 4);
  });

  it("falls back to a two week window around today with nothing scheduled", () => {
    const c = layoutChart(["x"], {}, [], today, 1);
    expect(c.unscheduled).toEqual(["x"]);
    expect(c.todayLeft).toBe(1);
    expect(c.days).toBe(17);
  });

  it("finds dependency conflicts", () => {
    const plans = {
      a: P({ start_date: "2030-01-01", due_date: "2030-01-05" }),
      b: P({ start_date: "2030-01-05", depends_on: ["a"] }), // starts the day a is due
      c: P({ start_date: "2030-01-06", depends_on: ["a"] }), // fine
      d: P({ start_date: "2030-01-02", depends_on: ["zzz"] }), // unknown prerequisite ignored
    };
    expect(depConflicts(plans)).toEqual([{ issue: "b", prerequisite: "a" }]);
  });

  it("detects loops before saving", () => {
    const plans = { b: P({ depends_on: ["a"] }), c: P({ depends_on: ["b"] }) };
    expect(wouldCycle(plans, "a", ["c"])).toBe(true);
    expect(wouldCycle(plans, "a", ["a"])).toBe(true);
    expect(wouldCycle(plans, "d", ["c"])).toBe(false);
  });

  it("summarises time and milestone progress", () => {
    expect(timeSummary(undefined)).toEqual({ text: "", over: false });
    expect(timeSummary(P({ estimate_hours: 8, logged_hours: 9.25 }))).toEqual({ text: "9.3 / 8 h", over: true });
    expect(timeSummary(P({ logged_hours: 2 })).text).toBe("2 h logged");
    const plans = { a: P({ milestone_id: "m" }), b: P({ milestone_id: "m" }), c: P({ milestone_id: "x" }) };
    expect(milestoneProgress("m", [{ id: "a", status: "done" }, { id: "b", status: "todo" }, { id: "c", status: "done" }], plans)).toEqual({ done: 1, total: 2 });
  });

  it("groups the axis by month", () => {
    const s = monthSpans(dayNumber("2030-01-30"), 5);
    expect(s).toEqual([{ label: "Jan 2030", days: 2 }, { label: "Feb 2030", days: 3 }]);
  });
});
