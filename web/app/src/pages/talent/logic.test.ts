import { describe, expect, it } from "vitest";
import type { Application, Job } from "./api";
import { dueLabel, funnelPercent, groupByStage, jobActions, jobStatusLabel, progressOf, salaryRange, daysInStage } from "./logic";

const stages = [{ id: "applied", name: "Applied" }, { id: "interview", name: "Interview" }];
const app = (id: string, stage: string, status: Application["status"] = "active"): Application => ({ id, stage, status, stage_entered_at: "2026-01-01T00:00:00Z" } as Application);
const job = (p: Partial<Job>): Job => ({ status: "draft", approval_status: "none", ...p } as Job);

describe("groupByStage", () => {
  it("buckets active applications and skips closed ones", () => {
    const g = groupByStage([app("1", "applied"), app("2", "interview"), app("3", "interview", "rejected")], stages);
    expect(g.applied.map((a) => a.id)).toEqual(["1"]);
    expect(g.interview.map((a) => a.id)).toEqual(["2"]);
  });
  it("puts applications of a removed stage in the first column", () => {
    expect(groupByStage([app("1", "gone")], stages).applied).toHaveLength(1);
  });
});

describe("jobActions", () => {
  it("lets HR publish a draft at once", () => {
    expect(jobActions(job({}), true).publish).toBe(true);
  });
  it("makes a manager request approval first", () => {
    const a = jobActions(job({}), false);
    expect(a.publish).toBe(false);
    expect(a.requestApproval).toBe(true);
  });
  it("lets a manager publish once approved, and HR approve a pending one", () => {
    expect(jobActions(job({ approval_status: "approved" }), false).publish).toBe(true);
    expect(jobActions(job({ approval_status: "pending" }), true).approve).toBe(true);
    expect(jobActions(job({ approval_status: "pending" }), true).publish).toBe(false);
  });
  it("offers close for open and reopen for closed", () => {
    expect(jobActions(job({ status: "open" }), true).close).toBe(true);
    expect(jobActions(job({ status: "closed" }), true).reopen).toBe(true);
  });
});

describe("labels", () => {
  it("describes draft states", () => {
    expect(jobStatusLabel(job({ approval_status: "pending" }))).toBe("Awaiting approval");
    expect(jobStatusLabel(job({ status: "open" }))).toBe("Open");
  });
  it("formats salary ranges", () => {
    expect(salaryRange({ salary_min: 1500000, salary_max: 2500000, currency: "INR" })).toBe("INR 15,00,000 to 25,00,000");
    expect(salaryRange({ salary_min: null, salary_max: null, currency: "INR" })).toBe("");
  });
});

describe("numbers", () => {
  it("computes progress, clamped", () => {
    expect(progressOf(0, 100, 50)).toBe(50);
    expect(progressOf(0, 100, 250)).toBe(100);
    expect(progressOf(10, 20, 5)).toBe(0);
    expect(progressOf(5, 5, 5)).toBe(100);
  });
  it("computes funnel percent", () => {
    expect(funnelPercent(25, 100)).toBe(25);
    expect(funnelPercent(5, 0)).toBe(0);
  });
  it("counts days in stage", () => {
    expect(daysInStage({ stage_entered_at: "2026-01-01T00:00:00Z" }, new Date("2026-01-04T12:00:00Z"))).toBe(3);
  });
  it("labels due dates", () => {
    const now = new Date(2026, 5, 10);
    expect(dueLabel("2026-06-08", now)).toEqual({ text: "2 days overdue", overdue: true });
    expect(dueLabel("2026-06-10", now).text).toBe("Due today");
    expect(dueLabel("2026-06-11", now).text).toBe("Due in 1 day");
    expect(dueLabel(null, now).overdue).toBe(false);
  });
});
