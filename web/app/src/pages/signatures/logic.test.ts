import { describe, expect, it } from "vitest";
import type { CreateInput, Request } from "./api";
import { defaultSize, draftProblems, hasInk, placeField, removeSigner, waitingOn } from "./logic";

const ok: CreateInput = { title: "NDA", file_id: "f", signers: [{ name: "Asha", email: "a@x.co" }, { name: "Ravi", email: "r@x.co" }], fields: [
  { signer_index: 0, type: "signature", page: 1, x: 0.1, y: 0.1, w: 0.2, h: 0.05 }, { signer_index: 1, type: "signature", page: 1, x: 0.5, y: 0.1, w: 0.2, h: 0.05 }] };

describe("draftProblems", () => {
  it("accepts a complete draft", () => expect(draftProblems(ok)).toEqual([]));
  it("lists what is missing", () => {
    expect(draftProblems({})).toEqual(expect.arrayContaining(["Give the request a title.", "Choose a PDF from Drive.", "Add at least one person."]));
    const p = draftProblems({ ...ok, signers: [{ name: "A", email: "bad" }, { name: "B", email: "a@x.co" }, { name: "C", email: "A@x.co" }], fields: [] });
    expect(p.some((x) => x.includes("valid email"))).toBe(true);
    expect(p.some((x) => x.includes("appears twice"))).toBe(true);
    expect(p.some((x) => x.includes("needs a signature field"))).toBe(true);
  });
  it("needs a signer, not only copies", () => {
    expect(draftProblems({ ...ok, signers: [{ name: "A", email: "a@x.co", role: "cc" }], fields: [] })).toContain("At least one person must sign, not only receive a copy.");
  });
});

describe("fields", () => {
  it("keeps a placed field inside the page", () => {
    const p = placeField("signature", 0.99, 0.01);
    expect(p.x + p.w).toBeLessThanOrEqual(1);
    expect(p.y).toBe(0);
    expect(defaultSize("checkbox").w).toBeLessThan(defaultSize("signature").w);
  });
  it("renumbers after removing a person", () => {
    const r = removeSigner(ok.fields, 0);
    expect(r).toHaveLength(1);
    expect(r[0].signer_index).toBe(0);
  });
});

describe("misc", () => {
  it("names who is next", () => {
    const r = { status: "sent", sequential: true, signers: [{ name: "Ravi", role: "signer", status: "pending", order: 1 }, { name: "Asha", role: "signer", status: "pending", order: 0 }] } as unknown as Request;
    expect(waitingOn(r)).toBe("Waiting for Asha");
    expect(waitingOn({ ...r, sequential: false })).toBe("Waiting for Ravi, Asha");
    expect(waitingOn({ ...r, status: "completed" })).toBe("");
  });
  it("detects ink", () => {
    expect(hasInk(new Uint8ClampedArray(400))).toBe(false);
    const d = new Uint8ClampedArray(4 * 100).fill(255);
    expect(hasInk(d)).toBe(true);
  });
});
