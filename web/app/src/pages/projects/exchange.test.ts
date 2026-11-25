import { describe, expect, it } from "vitest";
import { csvToDrafts, detectColumns, parseCsv, projectCsv, projectIcs, projectMarkdown, readDate, readPriority, readStatus, trelloToDrafts, type ExportInput } from "./exchange";
import type { Issue } from "./types";

const issue = (o: Partial<Issue>): Issue => ({
  id: "i1", org_id: "o", team_id: "t", identifier: "ENG-1", number: 1, title: "Ship, the \"beta\"", description: "Line 1\nLine 2", status: "todo", priority: 2,
  assignee_id: "u", assignee_name: "Maya", label_ids: ["l1"], project_id: "p", estimate: 0, sort_order: 0, creator_id: "u", created_at: "", updated_at: "", ...o,
});
const input = (): ExportInput => ({
  project: { id: "p", name: "Launch" },
  issues: [issue({}), issue({ id: "i2", identifier: "ENG-2", number: 2, title: "Done thing", status: "done", description: "" })],
  plans: { i1: { start_date: "2026-11-20", due_date: "2026-11-25", milestone_id: "m1", estimate_hours: 3, logged_hours: 1, depends_on: [] } },
  milestones: [{ id: "m1", project_id: "p", name: "Beta", due_date: "2026-12-01", done: false }],
  labels: [{ id: "l1", org_id: "o", name: "design", color: "", created_at: "" }],
  brief: "Goal: ship.",
  now: new Date("2026-11-19T10:00:00Z"),
});

describe("exports", () => {
  it("markdown lists milestones and issues by status", () => {
    const md = projectMarkdown(input());
    expect(md).toContain("# Launch");
    expect(md).toContain("- **Beta** — 2026-12-01 · 0/1 done");
    expect(md).toContain("- [ ] **ENG-1 Ship, the \"beta\"** — starts 2026-11-20 · due 2026-11-25 · High · @Maya · #design · milestone: Beta");
    expect(md).toContain("- [x] **ENG-2 Done thing**");
  });
  it("csv quotes cells that need it", () => {
    const csv = projectCsv(input()).split("\n");
    expect(csv[0]).toBe("Project,ID,Task,Status,Done,Priority,Assignee,Labels,Milestone,Start,Due,Estimate hours,Logged hours,Description");
    expect(csv[1]).toContain('"Ship, the ""beta"""');
    expect(parseCsv(projectCsv(input()))[1][2]).toBe('Ship, the "beta"');
  });
  it("ics has all-day events for open dated issues and milestones only", () => {
    const ics = projectIcs(input());
    expect(ics).toContain("UID:issue-i1@adrient-projects");
    expect(ics).toContain("DTSTART;VALUE=DATE:20261125");
    expect(ics).toContain("DTEND;VALUE=DATE:20261126");
    expect(ics).toContain("SUMMARY:Milestone: Beta");
    expect(ics).not.toContain("issue-i2");
  });
});

describe("csv import", () => {
  it("parses quoted newlines, BOM and CRLF", () => {
    const rows = parseCsv('\uFEFFName,Notes\r\n"a, b","x\ny"\r\n\r\nc,d\r\n');
    expect(rows).toEqual([["Name", "Notes"], ["a, b", "x\ny"], ["c", "d"]]);
  });
  it("detects Asana-style headers", () => {
    const m = detectColumns(["Task ID", "Name", "Section/Column", "Due Date", "Assignee", "Tags", "Completed At", "Notes"]);
    expect(m).toEqual([null, "title", "status", "due", "assignee", "labels", "done", "description"]);
  });
  it("turns rows into drafts and reports problems", () => {
    const rows = parseCsv("Name,Status,Due,Priority\nA,Doing,2026-11-30,high\nB,Done,03/04/26,whatever\n,x,,\n");
    const { drafts, report } = csvToDrafts(rows, detectColumns(rows[0]));
    expect(drafts.map((d) => [d.title, d.status, d.due, d.priority])).toEqual([["A", "in_progress", "2026-11-30", 2], ["B", "done", "", 0]]);
    expect(report.skippedRows).toBe(1);
    expect(report.unreadableDates).toBe(1);
    expect(report.unreadablePriorities).toBe(1);
  });
  it("reads values conservatively", () => {
    expect(readDate("2026-02-30")).toBeNull();
    expect(readDate("2026-02-28T10:00:00Z")).toBe("2026-02-28");
    expect(readPriority("P0")).toBe(1);
    expect(readStatus("In review")).toBe("in_progress");
  });
});

describe("trello import", () => {
  it("skips archived cards and appends checklists", () => {
    const { name, drafts, report } = trelloToDrafts({
      name: "Board",
      lists: [{ id: "l1", name: "To Do" }, { id: "l2", name: "Done" }, { id: "l3", name: "Old", closed: true }],
      labels: [{ id: "b1", name: "bug" }],
      cards: [
        { id: "c1", name: "One", desc: "d", idList: "l1", idLabels: ["b1"], due: "2026-12-01T00:00:00.000Z", pos: 1, idChecklists: ["k1"] },
        { id: "c2", name: "Two", idList: "l2", pos: 2 },
        { id: "c3", name: "Gone", idList: "l1", closed: true },
        { id: "c4", name: "In closed list", idList: "l3" },
      ],
      checklists: [{ id: "k1", idCard: "c1", checkItems: [{ name: "step", state: "complete", pos: 1 }] }],
    });
    expect(name).toBe("Board");
    expect(drafts).toHaveLength(2);
    expect(drafts[0]).toMatchObject({ title: "One", status: "todo", due: "2026-12-01", labels: ["bug"] });
    expect(drafts[0].description).toContain("- [x] step");
    expect(drafts[1].status).toBe("done");
    expect(report.skippedRows).toBe(2);
  });
  it("rejects other JSON", () => {
    expect(() => trelloToDrafts({ foo: 1 })).toThrow();
  });
});
