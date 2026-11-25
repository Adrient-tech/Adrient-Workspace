import { afterEach, describe, expect, it, vi } from "vitest";
import { mockApi, run } from "@/lib/ai/testing";
import { createShiftsTools } from "./tools";

const staff = [{ employee_id: "e1", name: "Asha", availability: { days: [], exceptions: [] } }, { employee_id: "e2", name: "Ben", availability: { days: [], exceptions: [] } }];
const shift = (id: string, emp: string) => ({ id, date: "2099-01-05", start: "09:00", end: "17:00", break_minutes: 30, role: "Cashier", location: "", notes: "", employee_id: emp, published: true });
const routes = {
  "GET /api/v1/shifts/me": { employee_id: "e1", scheduler: false },
  "GET /api/v1/shifts/staff": { staff },
  "GET /api/v1/shifts/shifts": { shifts: [shift("s1", "e1"), shift("s2", "e2"), shift("s3", "")] },
  "GET /api/v1/shifts/gaps": { gaps: [{ date: "2099-01-05", role: "Cook", required: 2, scheduled: 1, missing: 1 }] },
};
afterEach(() => vi.unstubAllGlobals());

describe("Shifts assistant tools", () => {
  it("only the swap, assign and create tools write, and each needs confirmation", () => {
    const w = createShiftsTools().filter((t) => t.risk !== "read").map((t) => t.name).sort();
    expect(w).toEqual(["shifts_assign_shift", "shifts_create_shift", "shifts_request_swap"]);
    expect(createShiftsTools().filter((t) => t.risk !== "read").every((t) => t.risk === "destructive")).toBe(true);
  });
  it("my schedule returns only the caller's own shifts", async () => {
    mockApi(routes);
    const r = await run(createShiftsTools(), "shifts_my_schedule", { from: "2099-01-01" });
    expect(r.result.rows.map((s: { id: string }) => s.id)).toEqual(["s1"]);
  });
  it("overview names assignees, counts open shifts and lists understaffed days", async () => {
    mockApi(routes);
    const r = await run(createShiftsTools(), "shifts_week_overview", { from: "2099-01-05" });
    expect(r.result.open_shifts).toBe(1);
    expect(r.result.shifts.rows.find((s: { id: string }) => s.id === "s2").assigned_to).toBe("Ben");
    expect(r.result.understaffed[0].role).toBe("Cook");
  });
  it("a 403 from the API reaches the model as a permission error", async () => {
    mockApi({ ...routes, "GET /api/v1/shifts/shifts/": { __status: 403 } });
    const r = await run(createShiftsTools(), "shifts_suggest_assignee", { shift_id: "s3" });
    expect(r.error).toBeTruthy();
  });
  it("create shift posts a draft and undo deletes it", async () => {
    const { calls } = mockApi({ ...routes, "POST /api/v1/shifts/shifts": { shift: shift("n1", ""), warnings: [] }, "DELETE /api/v1/shifts/shifts/n1": {} });
    const r = await run(createShiftsTools(), "shifts_create_shift", { date: "2099-01-06", start: "08:00", end: "12:00", role: "Cook" });
    expect(calls.find((c) => c.method === "POST")?.body).toMatchObject({ published: false, role: "Cook" });
    await r.undo!();
    expect(calls.some((c) => c.method === "DELETE" && c.url.endsWith("/n1"))).toBe(true);
  });
});
