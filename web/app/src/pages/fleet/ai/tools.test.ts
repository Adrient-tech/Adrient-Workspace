import { afterEach, describe, expect, it, vi } from "vitest";
import { mockApi, run } from "@/lib/ai/testing";
import { createFleetTools } from "./tools";

const vehicles = [
  { id: "v1", name: "White van", plate: "KA01AB1234", status: "active", meter: 50000, assignee_name: "Ravi", overdue: 1, due_soon: 0, open_issues: 1 },
  { id: "v2", name: "White truck", plate: "KA02CD5678", status: "active", meter: 90000 },
];
const routes = {
  "GET /api/v1/fleet/vehicles": (c: { url: string }) => ({ vehicles: c.url.includes("q=van") ? [vehicles[0]] : vehicles }),
  "GET /api/v1/fleet/reminders": { reminders: [{ vehicle_id: "v1", title: "Oil change", status: "overdue", next_date: "2099-01-01", days_left: -3 }] },
  "GET /api/v1/fleet/renewals": { renewals: [{ vehicle_id: "v1", title: "Insurance", due_date: "2099-02-01", status: "due_soon", done: false }] },
};
afterEach(() => vi.unstubAllGlobals());

describe("Fleet assistant tools", () => {
  it("only reporting an issue and recording a meter reading write, both with confirmation", () => {
    const w = createFleetTools().filter((t) => t.risk !== "read");
    expect(w.map((t) => t.name).sort()).toEqual(["fleet_record_meter", "fleet_report_issue"]);
    expect(w.every((t) => t.risk === "destructive")).toBe(true);
  });
  it("lists reminders and renewals by vehicle name", async () => {
    mockApi(routes);
    const r = await run(createFleetTools(), "fleet_reminders", { vehicle: "van" });
    expect(r.result.reminders.rows[0]).toMatchObject({ vehicle: "White van", title: "Oil change", status: "overdue" });
    expect(r.result.renewals.rows[0].title).toBe("Insurance");
  });
  it("refuses an ambiguous vehicle name", async () => {
    mockApi(routes);
    const r = await run(createFleetTools(), "fleet_report_issue", { vehicle: "white", title: "Noise" });
    expect(r.error).toMatch(/Several vehicles match/);
  });
  it("reports an issue on the matching vehicle", async () => {
    const { calls } = mockApi({ ...routes, "POST /api/v1/fleet/issues": { id: "i1", status: "open" } });
    const r = await run(createFleetTools(), "fleet_report_issue", { vehicle: "van", title: "Brake noise", priority: "high" });
    expect(calls.find((c) => c.method === "POST")?.body).toMatchObject({ vehicle_id: "v1", title: "Brake noise", priority: "high" });
    expect(r.summary).toMatch(/White van/);
  });
});
