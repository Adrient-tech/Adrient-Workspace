import { describe, expect, it } from "vitest";
import { byDueDay, columnOf, dayFromDue, dueFromDay, emptyExtras, isOverdue, monthCells, quickDue, subtaskProgress, toColumns } from "./logic";
import type { Task } from "./types";

const mk = (id: string, o: Partial<Task> = {}): Task => ({
  id, org_id: "", list_id: "l", owner_user_id: "", title: id, notes: "", due_at: "", completed: false,
  completed_at: "", parent_task_id: "", position: 0, created_at: "", updated_at: "", ...o,
});

describe("tasks logic", () => {
  it("round-trips a due day through RFC 3339 without shifting the date", () => {
    const iso = dueFromDay("2030-03-31");
    expect(iso).toMatch(/Z$/);
    expect(dayFromDue(iso)).toBe("2030-03-31");
    expect(dueFromDay("")).toBe("");
    expect(dueFromDay("garbage")).toBe("");
  });

  it("computes quick reschedule dates at 09:00 local", () => {
    const now = new Date(2030, 0, 9, 15, 0); // Wednesday
    const loc = (w: Parameters<typeof quickDue>[0]) => new Date(quickDue(w, now));
    expect(loc("today").getDate()).toBe(9);
    expect(loc("today").getHours()).toBe(9);
    expect(loc("tomorrow").getDate()).toBe(10);
    expect(loc("nextweek").getDate()).toBe(16);
    const mon = loc("nextmonday");
    expect([mon.getDay(), mon.getDate()]).toEqual([1, 14]);
    // On a Monday "next Monday" is a week away, not today.
    expect(new Date(quickDue("nextmonday", new Date(2030, 0, 7, 8, 0))).getDate()).toBe(14);
  });

  it("detects overdue tasks only when open", () => {
    const now = new Date(2030, 0, 9, 12, 0);
    expect(isOverdue({ due_at: new Date(2030, 0, 8).toISOString(), completed: false }, now)).toBe(true);
    expect(isOverdue({ due_at: new Date(2030, 0, 8).toISOString(), completed: true }, now)).toBe(false);
    expect(isOverdue({ due_at: "", completed: false }, now)).toBe(false);
  });

  it("builds the board columns", () => {
    const tasks = [mk("a"), mk("b"), mk("c", { completed: true }), mk("sub", { parent_task_id: "a" })];
    const extras = { b: { ...emptyExtras("b"), board_status: "doing" } };
    const cols = toColumns(tasks, extras);
    expect(cols.todo.map((t) => t.id)).toEqual(["a"]);
    expect(cols.doing.map((t) => t.id)).toEqual(["b"]);
    expect(cols.done.map((t) => t.id)).toEqual(["c"]);
    expect(columnOf(mk("x", { completed: true }), { ...emptyExtras(), board_status: "doing" })).toBe("done");
  });

  it("lays out a month and groups tasks by due day", () => {
    const cells = monthCells(2030, 0); // January 2030 starts on Tuesday
    expect(cells.length % 7).toBe(0);
    expect(cells.findIndex((c) => c === "2030-01-01")).toBe(1);
    expect(cells.filter(Boolean)).toHaveLength(31);
    const g = byDueDay([mk("a", { due_at: dueFromDay("2030-01-15") }), mk("b", { due_at: dueFromDay("2030-01-15") }), mk("c"), mk("s", { due_at: dueFromDay("2030-01-16"), parent_task_id: "a" })]);
    expect(Object.keys(g)).toEqual(["2030-01-15"]);
    expect(g["2030-01-15"]).toHaveLength(2);
  });

  it("summarises subtasks", () => {
    expect(subtaskProgress([])).toBe("");
    expect(subtaskProgress([mk("a", { completed: true }), mk("b")])).toBe("1/2");
  });
});
