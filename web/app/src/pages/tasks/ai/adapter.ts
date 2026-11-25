import { listEvents } from "../../calendar/api";
import { createTask, deleteTask, listTaskLists, listTasks, toggleTask, updateTask } from "../api";
import { listExtras, saveExtras } from "../extrasApi";
import { emptyExtras } from "../logic";
import type { TasksAdapter } from "./tools";


/** The real adapter: the user's own session against /api/v1/tasks. */
export function tasksAdapter(h: { currentListId: () => string | null; changed: () => void }): TasksAdapter {
  const after = async <T,>(p: Promise<T>) => {
    const r = await p;
    h.changed();
    return r;
  };
  const listOfTask = new Map<string, string>(); // task id -> list id, filled as tasks are read
  return {
    lists: async () => (await listTaskLists()).map((l) => ({ id: l.id, name: l.name })),
    currentListId: h.currentListId,
    tasks: async (lid) => {
      const ts = await listTasks(lid);
      ts.forEach((t) => listOfTask.set(t.id, lid));
      return ts;
    },
    priorities: async (lid) => {
      const ex = await listExtras(lid);
      return Object.fromEntries(Object.entries(ex).map(([k, v]) => [k, v.priority ?? 0]));
    },
    create: (lid, t) => after(createTask(lid, { title: t.title, notes: t.notes ?? "", due_at: t.due_at ?? "", parent_task_id: t.parent_task_id ?? "" })),
    update: (lid, id, p) => after(updateTask(lid, id, p)),
    setCompleted: async (lid, id, completed) => {
      const cur = (await listTasks(lid)).find((t) => t.id === id);
      if (cur && cur.completed !== completed) await after(toggleTask(lid, id));
    },
    setPriority: async (taskId, p) => {
      const lid = listOfTask.get(taskId) ?? h.currentListId();
      const ex = lid ? ((await listExtras(lid))[taskId] ?? emptyExtras(taskId)) : emptyExtras(taskId);
      await after(saveExtras(taskId, { ...ex, priority: p }));
    },
    repeatTags: async (lid) => {
      const ex = await listExtras(lid);
      return Object.fromEntries(Object.entries(ex).map(([id, x]) => [id, { recurrence: x.recurrence, tags: x.tags ?? [] }]));
    },
    setRepeatTags: async (taskId, v) => {
      const lid = listOfTask.get(taskId) ?? h.currentListId();
      const ex = lid ? ((await listExtras(lid))[taskId] ?? emptyExtras(taskId)) : emptyExtras(taskId);
      await after(saveExtras(taskId, { ...ex, ...(v.recurrence !== undefined ? { recurrence: v.recurrence } : {}), ...(v.tags !== undefined ? { tags: v.tags } : {}) }));
    },
    remove: (lid, id) => after(deleteTask(lid, id)),
    todaysEvents: async () => {
      const s = new Date();
      s.setHours(0, 0, 0, 0);
      const e = new Date(s.getTime() + 86400000);
      return listEvents(s.toISOString(), e.toISOString());
    },
    now: () => new Date(),
  };
}
