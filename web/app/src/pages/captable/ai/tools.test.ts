import { afterEach, describe, expect, it, vi } from "vitest";
import { mockApi, run } from "@/lib/ai/testing";
import { createCapTableTools } from "./tools";

afterEach(() => vi.unstubAllGlobals());

describe("Cap table assistant tools", () => {
  it("has no write tools: equity records cannot be changed by the assistant", () => {
    expect(createCapTableTools().every((t) => t.risk === "read")).toBe(true);
  });
  it("shows server percentages as given", async () => {
    mockApi({ "GET /api/v1/captable/table": { as_of: "2026-11-21", outstanding: 10, fully_diluted: 12, pool_unallocated: 2, convertible_cents: 0, rows: [{ name: "Asha", kind: "founder", stock: 6, options: 0, warrants: 0, pct_outstanding: 600000, pct_fully_diluted: 500000 }] } });
    const r = await run(createCapTableTools(), "captable_summary");
    expect(JSON.stringify(r.result)).toContain("60.0000%");
    expect(JSON.stringify(r.result)).toContain("50.0000%");
  });
  it("sends exact paise to the round model", async () => {
    const { calls } = mockApi({ "POST /api/v1/captable/model/round": { price_per_share_cents: "100.000000", new_investor_shares: 5, pool_top_up: 0, post_fully_diluted: 9, post_money_cents: 900, converted: [], holders: [] } });
    await run(createCapTableTools(), "captable_model_round", { pre_money: "40 crore", investment: "5 crore", pool_target_percent: 10 });
    expect(calls[0].body).toEqual({ pre_money: 40_00_00_000 * 100, investment: 5_00_00_000 * 100, pool_target_bp: 1000 });
    expect((await run(createCapTableTools(), "captable_model_round", { pre_money: "lots", investment: "1" })).error).toMatch(/cannot read/);
  });
  it("needs exactly one holder match", async () => {
    mockApi({ "GET /api/v1/captable/stakeholders": { stakeholders: [{ id: "1", name: "Asha Rao" }, { id: "2", name: "Asha Iyer" }] } });
    expect((await run(createCapTableTools(), "captable_holder", { name: "asha" })).error).toMatch(/Several holders/);
  });
});
