import { describe, expect, it } from "vitest";
import { createTaskTools, groupForPlan, type TaskRow, type TasksAdapter } from "./tools";

const NOW = new Date(2026, 10, 18, 9, 0);
const t = (id: string, o: Partial<TaskRow> = {}): TaskRow => ({ id, list_id: "L", title: id, notes: "", due_at: "", completed: false, parent_task_id: "", ...o });

function setup(initial: TaskRow[]) {
  const store = [...initial];
  const prio: Record<string, number> = {};
  let n = 0;
  const ad: TasksAdapter = {
    lists: async () => [{ id: "L", name: "My tasks" }],
    currentListId: () => "L",
    tasks: async () => store.map((x) => ({ ...x })),
    priorities: async () => ({ ...prio }),
    create: async (_l, x) => { const r = t("n" + n++, { title: x.title, due_at: x.due_at ?? "", parent_task_id: x.parent_task_id ?? "" }); store.push(r); return r; },
    update: async (_l, id, p) => { const r = store.find((x) => x.id === id)!; Object.assign(r, p.title !== undefined ? { title: p.title } : {}, p.due_at !== undefined ? { due_at: p.due_at } : {}); return r; },
    setCompleted: async (_l, id, c) => { store.find((x) => x.id === id)!.completed = c; },
    setPriority: async (id, p) => { prio[id] = p; },
    remove: async (_l, id) => { store.splice(store.findIndex((x) => x.id === id), 1); },
    todaysEvents: async () => [{ title: "Standup", start_at: NOW.toISOString(), end_at: NOW.toISOString(), all_day: false }],
    now: () => NOW,
  };
  return { store, prio, tools: Object.fromEntries(createTaskTools(ad).map((x) => [x.name, x])) };
}

describe("groupForPlan", () => {
  it("splits overdue / today / upcoming / undated and sorts by priority", () => {
    const g = groupForPlan([t("a", { due_at: new Date(2026, 10, 10).toISOString() }), t("b", { due_at: new Date(2026, 10, 18, 15).toISOString() }), t("c", { due_at: new Date(2026, 10, 25).toISOString() }), t("d"), t("e", { completed: true })], { d: 3 }, NOW);
    expect(g.overdue.map((x) => x.id)).toEqual(["a"]);
    expect(g.today.map((x) => x.id)).toEqual(["b"]);
    expect(g.upcoming.map((x) => x.id)).toEqual(["c"]);
    expect(g.no_date[0].priority).toBe("high");
  });
});

describe("task tools", () => {
  it("create tasks with a subtask; confirmation risk; undo removes", async () => {
    const { tools, store } = setup([]);
    expect(tools.tasks_create.risk).toBe("destructive");
    const r = await tools.tasks_create.handler({ items: [{ title: "Launch", due: "2026-11-30", priority: "high" }, { title: "Write copy", parent_title: "Launch" }] });
    expect(store).toHaveLength(2);
    expect(store[1].parent_task_id).toBe(store[0].id);
    await r.undo!();
    expect(store).toHaveLength(0);
    expect((await tools.tasks_create.handler({ items: [{ title: "x", due: "banana" }] })).error).toContain("not a valid date");
  });
  it("update with undo, and bulk priorities", async () => {
    const { tools, store, prio } = setup([t("a", { title: "Old" })]);
    const r = await tools.tasks_update.handler({ task_id: "a", title: "New", completed: true, priority: "high" });
    expect(store[0]).toMatchObject({ title: "New", completed: true });
    expect(prio.a).toBe(3);
    await r.undo!();
    expect(store[0]).toMatchObject({ title: "Old", completed: false });
    expect(prio.a).toBe(0);
    const p = await tools.tasks_set_priorities.handler({ items: [{ task_id: "a", priority: "low" }, { task_id: "zzz", priority: "high" }] });
    expect(prio.a).toBe(1);
    await p.undo!();
    expect(prio.a).toBe(0);
  });
  it("plan context includes calendar", async () => {
    const { tools } = setup([t("a")]);
    const r = (await tools.tasks_plan_context.handler({})) as { result: { calendar_today: unknown[] } };
    expect(r.result.calendar_today).toHaveLength(1);
  });
});
