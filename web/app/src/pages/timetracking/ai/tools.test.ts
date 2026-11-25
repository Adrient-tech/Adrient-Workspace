import { afterEach, describe, expect, it, vi } from "vitest";
import { mockApi, run } from "@/lib/ai/testing";
import { createTimeTools } from "./tools";

const projects = [{ id: "p1", name: "Website", client_id: "c1", billable: true, archived: false }, { id: "p2", name: "Website v2", client_id: "c1", billable: true, archived: false }, { id: "p3", name: "Support", client_id: "", billable: false, archived: false }];
const entries = [{ id: "e1", date: "2099-01-05", project_id: "p1", seconds: 5400, task: "Design", invoice_id: "" }, { id: "e2", date: "2099-01-05", project_id: "p3", seconds: 1800, task: "", invoice_id: "" }];
const routes = { "GET /api/v1/timetracking/projects": { projects }, "GET /api/v1/timetracking/entries": { entries }, "GET /api/v1/timetracking/timer": { timer: null } };
afterEach(() => vi.unstubAllGlobals());

describe("Time tracking assistant tools", () => {
  it("members get no amount or billing tools", () => {
    const names = createTimeTools({ admin: false }).map((t) => t.name);
    expect(names).toEqual(["tt_my_hours", "tt_timer_status", "tt_log_time", "tt_start_timer"]);
    expect(createTimeTools({ admin: true }).map((t) => t.name)).toEqual(expect.arrayContaining(["tt_unbilled_by_client", "tt_report"]));
  });
  it("only logging and the timer write, and they need confirmation", () => {
    const w = createTimeTools({ admin: true }).filter((t) => t.risk !== "read");
    expect(w.map((t) => t.name).sort()).toEqual(["tt_log_time", "tt_start_timer"]);
    expect(w.every((t) => t.risk === "destructive")).toBe(true);
  });
  it("sums my hours by project", async () => {
    mockApi(routes);
    const r = await run(createTimeTools({ admin: false }), "tt_my_hours", { from: "2099-01-01", to: "2099-01-07" });
    expect(r.result.total_hours).toBe(2);
    expect(r.result.by_project).toEqual(expect.arrayContaining([{ project: "Website", hours: 1.5 }]));
  });
  it("log time parses the duration, refuses ambiguous project names and can be undone", async () => {
    const { calls } = mockApi({ ...routes, "POST /api/v1/timetracking/entries": { entry: { id: "n1" }, warnings: [] }, "DELETE /api/v1/timetracking/entries/n1": {} });
    const tools = createTimeTools({ admin: false });
    expect((await run(tools, "tt_log_time", { project: "web", duration: "1h" })).error).toMatch(/Several projects/);
    expect((await run(tools, "tt_log_time", { project: "Support", duration: "soon" })).error).toMatch(/not a duration/);
    const ok = await run(tools, "tt_log_time", { project: "Support", duration: "1:30", date: "2099-01-05" });
    expect(calls.find((c) => c.method === "POST")?.body).toMatchObject({ project_id: "p3", seconds: 5400, date: "2099-01-05" });
    await ok.undo!();
    expect(calls.some((c) => c.method === "DELETE")).toBe(true);
  });
});
