import { describe, expect, it } from "vitest";
import { ACTIONS, ago, can, paramsFor, slaText, sortAlerts, validateAction } from "./logic";
import type { Alert } from "./api";

const mk = (o: Partial<Alert>): Alert => ({ id: "x", user_id: "", email: "", ip: "", rule: "r", severity: "low", status: "open", title: "t", evidence: {}, recommended: "", assignee_id: "", assignee_name: "", sla_due_at: null, sla_breached: false, resolved_at: null, created_at: "2026-11-20T10:00:00Z", ...o });

describe("slaText", () => {
  const now = new Date("2026-11-20T12:00:00Z");
  it("counts down and flags overdue", () => {
    expect(slaText("2026-11-20T13:30:00Z", "open", now)).toEqual({ text: "1 h 30 m left", late: false });
    expect(slaText("2026-11-20T11:15:00Z", "investigating", now)).toEqual({ text: "45 m overdue", late: true });
  });
  it("ignores closed alerts", () => {
    expect(slaText("2026-11-01T00:00:00Z", "false_positive", now).late).toBe(false);
  });
});

describe("validateAction", () => {
  const bypass = ACTIONS.find((a) => a.action === "mfa.bypass")!;
  it("requires a reason and required fields", () => {
    expect(validateAction(bypass, "", {})).toMatch(/reason/);
    expect(validateAction(bypass, "lost phone", {})).toMatch(/Hours/);
    expect(validateAction(bypass, "lost phone", { hours: "4" })).toBe("");
  });
  it("types numeric params", () => {
    expect(paramsFor(bypass, { hours: "4" })).toEqual({ hours: 4 });
    expect(paramsFor(bypass, { hours: " " })).toEqual({});
  });
});

describe("helpers", () => {
  it("every action names a capability", () => {
    for (const a of ACTIONS) expect(a.cap).not.toBe("");
  });
  it("can checks the list", () => {
    expect(can(["a", "b"], "b")).toBe(true);
    expect(can(undefined, "b")).toBe(false);
  });
  it("sorts active, severe, new first", () => {
    const l = sortAlerts([mk({ id: "a", severity: "low" }), mk({ id: "b", severity: "high", status: "policy" }), mk({ id: "c", severity: "high" })]);
    expect(l.map((x) => x.id)).toEqual(["c", "a", "b"]);
  });
  it("ago", () => {
    expect(ago("2026-11-20T11:00:00Z", new Date("2026-11-20T12:00:00Z"))).toBe("1 h ago");
    expect(ago(null)).toBe("never");
  });
});
