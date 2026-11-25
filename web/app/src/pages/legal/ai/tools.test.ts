import { afterEach, describe, expect, it, vi } from "vitest";
import { mockApi, run } from "@/lib/ai/testing";
import { createLegalTools } from "./tools";

afterEach(() => vi.unstubAllGlobals());

const matter = { id: "m1", number: "2026-00001", title: "Dispute", client_name: "Kaveri", status: "open", practice_area: "Litigation", responsible: "Asha", unbilled: 123450, unbilled_minutes: 72, billed: 0, sol_date: "" };

describe("Legal assistant tools", () => {
  it("only logging time, deadlines and notes write, and all of them need confirmation", () => {
    const w = createLegalTools().filter((t) => t.risk !== "read");
    expect(w.map((t) => t.name).sort()).toEqual(["legal_add_deadline", "legal_add_note", "legal_log_time"]);
    expect(w.every((t) => t.risk === "destructive")).toBe(true);
  });
  it("formats money and hours from server figures", async () => {
    mockApi({ "GET /api/v1/legal/matters": { matters: [matter] } });
    const r = await run(createLegalTools(), "legal_unbilled");
    expect(JSON.stringify(r.result)).toContain("₹1,234.50");
    expect(JSON.stringify(r.result)).toContain("1h 12m");
  });
  it("logs time through the API and can undo it", async () => {
    const { calls } = mockApi({ "POST /api/v1/legal/matters/m1/entries": { id: "e1", minutes: 12, amount: 100000 }, "DELETE /api/v1/legal/entries/e1": {} });
    const r = await run(createLegalTools(), "legal_log_time", { matter_id: "m1", duration: "7m" });
    expect(calls[0].body).toMatchObject({ kind: "time", duration: "7m" });
    expect(JSON.stringify(r.result)).toContain("₹1,000.00");
    expect((await run(createLegalTools(), "legal_log_time", { matter_id: "m1", duration: "soon" })).error).toMatch(/cannot read that duration/);
  });
  it("hides restricted matters in conflict results", async () => {
    mockApi({ "GET /api/v1/legal/conflicts": { clear: false, query: "z", hits: [{ name: "Zenith", source: "party", role: "Counterparty", score: 70, matters: [{ role: "Counterparty", restricted: true }] }] } });
    const r = await run(createLegalTools(), "legal_conflict_check", { query: "Zenith" });
    expect(JSON.stringify(r.result)).toContain("a restricted matter as Counterparty");
  });
});
