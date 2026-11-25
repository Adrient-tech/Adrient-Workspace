import { describe, expect, it } from "vitest";
import { createTaskTools, repeatFromText, type TaskRow, type TasksAdapter } from "./tools";

describe("repeatFromText", () => {
  it("understands plain English", () => {
    expect(repeatFromText("daily")).toBe("daily");
    expect(repeatFromText("every weekday")).toBe("weekdays");
    expect(repeatFromText("every 2 weeks on mon,wed")).toBe("every:2:weekly:1,3");
    expect(repeatFromText("month")).toBe("monthly");
    expect(repeatFromText("none")).toBe("");
    expect(repeatFromText("whenever I feel like it")).toBeNull();
  });
});

describe("tasks_set_repeat_tags", () => {
  const row: TaskRow = { id: "a", list_id: "L", title: "A", notes: "", due_at: "", completed: false, parent_task_id: "" };
  function make() {
    const state: Record<string, { recurrence: string; tags: string[] }> = {};
    const ad = {
      lists: async () => [{ id: "L", name: "L" }],
      currentListId: () => "L",
      tasks: async () => [row],
      priorities: async () => ({}),
      repeatTags: async () => JSON.parse(JSON.stringify(state)),
      setRepeatTags: async (id: string, v: { recurrence?: string; tags?: string[] }) => {
        state[id] = { recurrence: v.recurrence ?? state[id]?.recurrence ?? "", tags: v.tags ?? state[id]?.tags ?? [] };
      },
    } as unknown as TasksAdapter;
    return { ad, state };
  }
  const tool = (ad: TasksAdapter) => createTaskTools(ad).find((x) => x.name === "tasks_set_repeat_tags")!;

  it("is only offered when the adapter supports it", () => {
    expect(createTaskTools({ lists: async () => [], currentListId: () => null } as unknown as TasksAdapter).some((x) => x.name === "tasks_set_repeat_tags")).toBe(false);
  });
  it("sets repeat and tags, rejects unknown phrases, and undoes", async () => {
    const { ad, state } = make();
    const t = tool(ad);
    const bad = (await t.handler({ items: [{ task_id: "a", repeat: "whenever I feel like it" }] })) as { error?: string };
    expect(bad.error).toContain("could not understand");
    const r = (await t.handler({ items: [{ task_id: "a", repeat: "every 2 weeks on mon", tags: ["#Home", "bills"] }, { task_id: "zzz", repeat: "daily" }] })) as { result: { changed: number }; undo: () => Promise<void> };
    expect(r.result.changed).toBe(1);
    expect(state.a).toEqual({ recurrence: "every:2:weekly:1", tags: ["home", "bills"] });
    await r.undo();
    expect(state.a).toEqual({ recurrence: "", tags: [] });
  });
});
