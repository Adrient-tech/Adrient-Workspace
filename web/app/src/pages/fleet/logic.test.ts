import { describe, expect, it } from "vitest";
import { cleanVin, compliancePct, describeInterval, dueText, vinCheckOk, vinShapeOk } from "./logic";

describe("VIN", () => {
  it("cleans, checks the shape and the North American check digit", () => {
    expect(cleanVin(" 1m8-gdm 9axkp042788 ")).toBe("1M8GDM9AXKP042788");
    expect(vinShapeOk("1M8GDM9AXKP042788")).toBe(true);
    expect(vinShapeOk("1M8GDM9AXKP04278I")).toBe(false);
    expect(vinShapeOk("ABC")).toBe(false);
    expect(vinCheckOk("1M8GDM9AXKP042788")).toBe(true);
    expect(vinCheckOk("1M8GDM9A1KP042788")).toBe(false);
  });
});

describe("reminder text", () => {
  it("describes intervals and what is left", () => {
    expect(describeInterval({ every: 6, unit: "months" }, 10000)).toBe("Every 6 months or 10,000 km");
    expect(describeInterval({ every: 1, unit: "years" }, 0)).toBe("Every 1 year");
    expect(describeInterval({ every: 0, unit: "" }, 0)).toBe("One time");
    expect(dueText(12, 300)).toBe("in 12 days · 300 km left");
    expect(dueText(-5, -40)).toBe("5 days overdue · 40 km over");
  });
  it("computes compliance", () => {
    expect(compliancePct([])).toBeNull();
    expect(compliancePct([{ on_time: true }, { on_time: false }, { on_time: true }, { on_time: true }])).toBe(75);
  });
});
