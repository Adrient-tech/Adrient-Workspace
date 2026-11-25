import { describe, expect, it } from "vitest";
import { deepChoice, deepParam } from "./deepLink";

describe("deepLink", () => {
  it("reads a parameter", () => expect(deepParam("employee", "?view=people&employee=abc")).toBe("abc"));
  it("returns empty when absent", () => expect(deepParam("x", "?a=1")).toBe(""));
  it("accepts only known choices", () => {
    expect(deepChoice("view", ["home", "people"] as const, "home", "?view=people")).toBe("people");
    expect(deepChoice("view", ["home", "people"] as const, "home", "?view=evil")).toBe("home");
  });
});
