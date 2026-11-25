import { describe, expect, it } from "vitest";
import { APP_SOURCES, fetchAppTable, tableWrites, uniqueSheetName } from "./appData";

describe("appData", () => {
  it("maps products and skips archived", () => {
    const src = APP_SOURCES.find((s) => s.id === "inventory")!;
    const t = src.map([
      { name: "Tea", sku: "T1", price: 10, cost: 4, total_stock: 7 },
      { name: "Old", archived: true },
    ]);
    expect(t.rows).toHaveLength(1);
    expect(t.rows[0].slice(0, 2)).toEqual(["Tea", "T1"]);
    expect(t.rows[0].length).toBe(t.header.length);
  });
  it("every source row matches its header width", () => {
    for (const s of APP_SOURCES) {
      const t = s.map([{ name: "x", title: "y", number: 1, total: 5 }]);
      expect(t.rows[0].length).toBe(t.header.length);
    }
  });
  it("fetches through the given fetcher and reports errors", async () => {
    const src = APP_SOURCES.find((s) => s.id === "crm")!;
    const ok = (async () => ({ ok: true, json: async () => ({ deals: [{ title: "Big", value: 9 }] }) })) as any;
    expect((await fetchAppTable(src, ok)).rows[0][0]).toBe("Big");
    const bad = (async () => ({ ok: false, status: 403 })) as any;
    await expect(fetchAppTable(src, bad)).rejects.toThrow(/access/);
  });
  it("names sheets uniquely and writes header + cells", () => {
    expect(uniqueSheetName("Deals", ["Deals", "Deals 2"])).toBe("Deals 3");
    const w = tableWrites({ title: "t", header: ["A", "B"], rows: [["x", 3]] }, "s1");
    expect(w).toHaveLength(4);
    expect(w[3].args[2].v).toBe(3);
  });
});
