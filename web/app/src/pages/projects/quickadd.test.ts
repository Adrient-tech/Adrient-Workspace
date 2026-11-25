import { describe, expect, it } from "vitest";
import { parseIssueQuick } from "./quickadd";

const members = [{ id: "u1", name: "Maya Rao" }, { id: "u2", name: "Sam" }];
const labels = [{ id: "l1", name: "design" }, { id: "l2", name: "Needs review" }];
const today = "2026-11-19";

describe("parseIssueQuick", () => {
  it("parses priority, label, assignee and date", () => {
    const d = parseIssueQuick("Ship the beta !high #design @maya friday", { members, labels, today });
    expect(d.title).toBe("Ship the beta");
    expect(d.priority).toBe(2);
    expect(d.labelIds).toEqual(["l1"]);
    expect(d.assigneeId).toBe("u1");
    expect(d.due).toBe("2026-11-20");
    expect(d.tokens.map((t) => t.type).sort()).toEqual(["assignee", "due", "label", "priority"]);
  });
  it("maps priority words to the Projects scale", () => {
    expect(parseIssueQuick("a !urgent").priority).toBe(1);
    expect(parseIssueQuick("a !medium").priority).toBe(3);
    expect(parseIssueQuick("a !low").priority).toBe(4);
  });
  it("keeps unknown labels and unknown people in the title", () => {
    const d = parseIssueQuick("Fix login #backend @nobody", { members, labels, today });
    expect(d.labelIds).toEqual([]);
    expect(d.assigneeId).toBe("");
    expect(d.title).toBe("Fix login @nobody #backend");
  });
  it("a dismissed chip returns its text", () => {
    const d = parseIssueQuick("Plan tomorrow", { today, ignore: new Set(["due"]) });
    expect(d.due).toBe("");
    expect(d.title).toBe("Plan tomorrow");
    const p = parseIssueQuick("Bug !high", { ignore: new Set(["priority"]) });
    expect(p.priority).toBe(0);
    expect(p.title).toBe("Bug !high");
  });
  it("matches labels with spaces by hyphenated tag", () => {
    expect(parseIssueQuick("x #needs-review", { labels }).labelIds).toEqual(["l2"]);
  });
  it("does not treat times or repeats as chips", () => {
    const d = parseIssueQuick("Standup every day 9am", { today });
    expect(d.title).toContain("every day");
    expect(d.tokens).toHaveLength(0);
  });
});
