import { afterEach, describe, expect, it, vi } from "vitest";
import { mockApi, run } from "@/lib/ai/testing";
import { createFieldTools } from "./tools";

const visit = { id: "v1", job_id: "j1", date: "2099-03-02", start: "09:00", end: "10:00", status: "scheduled", assignees: ["u1"], job_title: "Lawn care", client_name: "Mrs Rao", address: "12 Lake Road" };
const routes = {
  "GET /api/v1/fieldservice/visits": { visits: [visit] },
  "GET /api/v1/fieldservice/customers": { customers: [{ id: "c1", name: "Mrs Rao" }, { id: "c2", name: "Mr Rao" }] },
};
afterEach(() => vi.unstubAllGlobals());

describe("Field service assistant tools", () => {
  it("writes are confirmed, and nothing can send a quote or an invoice", () => {
    const tools = createFieldTools();
    const w = tools.filter((t) => t.risk !== "read").map((t) => t.name).sort();
    expect(w).toEqual(["fs_complete_visit", "fs_create_request", "fs_reschedule_visit"]);
    expect(tools.filter((t) => t.risk !== "read").every((t) => t.risk === "destructive")).toBe(true);
    expect(tools.map((t) => t.name).join(" ")).not.toMatch(/send|invoice_/);
  });
  it("lists visits with customer and address", async () => {
    mockApi(routes);
    const r = await run(createFieldTools(), "fs_visits", { from: "2099-03-01" });
    expect(r.result.rows[0]).toMatchObject({ id: "v1", customer: "Mrs Rao", address: "12 Lake Road" });
  });
  it("create request needs exactly one matching customer", async () => {
    mockApi({ ...routes, "POST /api/v1/fieldservice/requests": { id: "r1", status: "new", title: "Gutters" }, "POST /api/v1/fieldservice/requests/r1/status": {} });
    const tools = createFieldTools();
    expect((await run(tools, "fs_create_request", { customer: "rao", title: "Gutters" })).error).toMatch(/Several customers/);
    expect((await run(tools, "fs_create_request", { customer: "nobody", title: "x" })).error).toBeTruthy();
  });
  it("reschedule moves the visit and undo moves it back", async () => {
    const { calls } = mockApi({ ...routes, "PUT /api/v1/fieldservice/visits/v1": { id: "v1", date: "2099-03-05", start: "09:00", end: "10:00" } });
    const r = await run(createFieldTools(), "fs_reschedule_visit", { visit_id: "v1", date: "2099-03-05" });
    expect(calls.find((c) => c.method === "PUT")?.body).toMatchObject({ date: "2099-03-05" });
    await r.undo!();
    expect(calls.filter((c) => c.method === "PUT")[1].body).toMatchObject({ date: "2099-03-02", start: "09:00" });
  });
});
