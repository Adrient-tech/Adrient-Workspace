import { describe, expect, it } from "vitest";
import { blankSchedule, computeTotals, describeSchedule } from "./logic";
import type { Item } from "./api";

const mk = (o: Partial<Item>): Item => ({ name: "x", qty: 1, unit_price: 0, unit_cost: 0, taxable: true, ...o });

describe("computeTotals", () => {
  it("matches the server maths: percent discount shared over the taxable part", () => {
    const t = computeTotals([mk({ name: "Mow", qty: 2, unit_price: 50, unit_cost: 20 }), mk({ name: "Fert", unit_price: 100, unit_cost: 60, taxable: false }), mk({ name: "Edge", unit_price: 40, optional: true })], { kind: "pct", value: 10 }, 18);
    expect(t).toEqual({ subtotal: 200, discount: 20, tax: 16.2, total: 196.2, cost: 100, profit: 80 });
  });
  it("counts an optional add-on only when selected and caps the discount", () => {
    const items = [mk({ unit_price: 40, optional: true, selected: true })];
    expect(computeTotals(items, null, 0).subtotal).toBe(40);
    expect(computeTotals(items, { kind: "amount", value: 999 }, 18).total).toBe(0);
  });
});

describe("describeSchedule", () => {
  it("reads naturally", () => {
    expect(describeSchedule(blankSchedule("2026-11-02"))).toBe("One-off");
    expect(describeSchedule({ ...blankSchedule("2026-11-02"), repeat: "weekly", interval: 2, weekdays: [1, 4], end_count: 8 })).toBe("Every 2 weeks on Monday, Thursday, 8 visits");
    expect(describeSchedule({ ...blankSchedule("2026-11-10"), repeat: "monthly", monthly_by: "weekday", end_count: 1 })).toBe("Every month on the second Tuesday, 1 visit");
  });
});
