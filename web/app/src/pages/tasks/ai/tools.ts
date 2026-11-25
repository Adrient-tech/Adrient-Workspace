/**
 * Assistant tools for Tasks. Task titles and notes may come from other
 * people (shared lists, tasks created from mail), so they are data only.
 * Creating or changing tasks asks the user to confirm and can be undone;
 * deleting tasks is not offered (undo of a create is the only delete).
 */
import { parseQuickAdd } from "../quickparse";
import { chainUndo, clip, defineTool, guard, str, type AssistantTool } from "@/lib/ai";

export interface TaskRow {
  id: string;
  list_id: string;
  title: string;
  notes: string;
  due_at: string;
  completed: boolean;
  parent_task_id: string;
}
export interface ListRow { id: string; name: string }
export interface PlanEvent { title: string; start_at: string; end_at: string; all_day: boolean }

export interface TasksAdapter {
  lists(): Promise<ListRow[]>;
  currentListId(): string | null;
  tasks(listId: string): Promise<TaskRow[]>;
  /** priority 0..3 per task id in the list */
  priorities(listId: string): Promise<Record<string, number>>;
  create(listId: string, t: { title: string; notes?: string; due_at?: string; parent_task_id?: string }): Promise<TaskRow>;
  update(listId: string, id: string, patch: { title?: string; notes?: string; due_at?: string }): Promise<TaskRow>;
  setCompleted(listId: string, id: string, completed: boolean): Promise<void>;
  setPriority(taskId: string, p: number): Promise<void>;
  remove(listId: string, id: string): Promise<void>;
  todaysEvents(): Promise<PlanEvent[]>;
  now(): Date;
  /** Repeat rule and tags per task id of a list (optional: older adapters lack it). */
  repeatTags?(listId: string): Promise<Record<string, { recurrence: string; tags: string[] }>>;
  setRepeatTags?(taskId: string, v: { recurrence?: string; tags?: string[] }): Promise<void>;
}

/** "every 2 weeks on mon,wed" or "daily" to the stored repeat rule; "none" clears it. null when not understood. */
export function repeatFromText(text: string): string | null {
  const t = text.trim().toLowerCase();
  if (!t || t === "none" || t === "never") return "";
  const p = parseQuickAdd(`x ${t.startsWith("every") || /^(daily|weekly|monthly|yearly|annually)$/.test(t) ? t : `every ${t}`}`, { today: "2026-01-05" });
  return p.repeat;
}

const PRI = ["none", "low", "medium", "high"];

export function dayStart(d: Date): Date {
  const x = new Date(d);
  x.setHours(0, 0, 0, 0);
  return x;
}

/** Group open tasks the way a daily plan wants them. */
export function groupForPlan(tasks: TaskRow[], prio: Record<string, number>, now: Date) {
  const t0 = dayStart(now).getTime();
  const t1 = t0 + 86400000;
  const open = tasks.filter((t) => !t.completed && !t.parent_task_id);
  const r = (t: TaskRow) => ({ id: t.id, list_id: t.list_id, title: clip(t.title, 120), due: t.due_at || null, priority: PRI[prio[t.id] ?? 0] ?? "none" });
  const due = (t: TaskRow) => (t.due_at ? new Date(t.due_at).getTime() : NaN);
  const byPri = (a: TaskRow, b: TaskRow) => (prio[b.id] ?? 0) - (prio[a.id] ?? 0) || (due(a) || 9e15) - (due(b) || 9e15);
  return {
    overdue: open.filter((t) => due(t) < t0).sort(byPri).map(r),
    today: open.filter((t) => due(t) >= t0 && due(t) < t1).sort(byPri).map(r),
    upcoming: open.filter((t) => due(t) >= t1).sort(byPri).slice(0, 15).map(r),
    no_date: open.filter((t) => isNaN(due(t))).sort(byPri).slice(0, 15).map(r),
  };
}

function isoOrEmpty(v: unknown): string | { error: string } {
  const s = str(v);
  if (!s) return "";
  const d = /^\d{4}-\d{2}-\d{2}$/.test(s) ? new Date(`${s}T00:00:00`) : new Date(s);
  return isNaN(d.getTime()) ? { error: `"${s}" is not a valid date (use YYYY-MM-DD or ISO 8601)` } : d.toISOString();
}

export function createTaskTools(ad: TasksAdapter): AssistantTool[] {
  const listOf = async (a: Record<string, unknown>): Promise<string | { error: string }> => {
    const id = str(a.list_id) || ad.currentListId();
    if (id) return id;
    const ls = await ad.lists();
    return ls[0]?.id ?? { error: "no task list exists; ask the user to create one" };
  };
  return [
    defineTool({
      name: "tasks_list",
      description: "List the user's task lists, or the tasks of one list (default: the list on screen). Returns up to 80 tasks with due date, completion and priority.",
      risk: "read",
      parameters: { type: "object", properties: { list_id: { type: "string" }, include_completed: { type: "boolean" } } },
      handler: (a) =>
        guard(async () => {
          const lists = await ad.lists();
          const lid = await listOf(a);
          if (typeof lid !== "string") return { error: lid.error };
          const [ts, pr] = await Promise.all([ad.tasks(lid), ad.priorities(lid).catch(() => ({}) as Record<string, number>)]);
          const rows = ts.filter((t) => a.include_completed === true || !t.completed).slice(0, 80);
          return { result: { lists, list_id: lid, tasks: rows.map((t) => ({ id: t.id, title: clip(t.title, 140), notes: clip(t.notes, 200), due: t.due_at || null, completed: t.completed, priority: PRI[pr[t.id] ?? 0], subtask_of: t.parent_task_id || undefined })) } };
        }),
    }),
    defineTool({
      name: "tasks_plan_context",
      description: "Everything needed to plan the user's day: open tasks grouped as overdue / due today / upcoming / no date (with priority) and today's calendar events. Use it for 'plan my day' and 'what should I do first'.",
      risk: "read",
      parameters: { type: "object", properties: { list_id: { type: "string" } } },
      handler: (a) =>
        guard(async () => {
          const lid = await listOf(a);
          if (typeof lid !== "string") return { error: lid.error };
          const [ts, pr, evs] = await Promise.all([ad.tasks(lid), ad.priorities(lid).catch(() => ({}) as Record<string, number>), ad.todaysEvents().catch(() => [] as PlanEvent[])]);
          return { result: { now: ad.now().toISOString(), ...groupForPlan(ts, pr, ad.now()), calendar_today: evs.slice(0, 20).map((e) => ({ title: clip(e.title, 80), start: e.start_at, end: e.end_at, all_day: e.all_day })) } };
        }),
    }),
    defineTool({
      name: "tasks_create",
      description: "Create tasks (turn pasted text, notes or a plan into tasks). Give each a short imperative title; add due (YYYY-MM-DD) only when the text states or clearly implies one. A subtask names its parent by parent_title (a task created earlier in the same call). The user confirms first. Max 25 per call.",
      risk: "destructive",
      parameters: {
        type: "object",
        properties: {
          list_id: { type: "string", description: "Default: the list on screen" },
          items: {
            type: "array",
            items: {
              type: "object",
              properties: {
                title: { type: "string" }, notes: { type: "string" }, due: { type: "string" },
                priority: { type: "string", enum: PRI }, parent_title: { type: "string" },
              },
              required: ["title"],
            },
          },
        },
        required: ["items"],
      },
      label: (a) => `Create ${Array.isArray(a.items) ? a.items.length : 0} task(s): ${(Array.isArray(a.items) ? a.items : []).slice(0, 4).map((i) => clip(str((i as { title?: unknown }).title), 36)).join("; ")}`,
      handler: (a) =>
        guard(async () => {
          const lid = await listOf(a);
          if (typeof lid !== "string") return { error: lid.error };
          const items = (Array.isArray(a.items) ? a.items : []).slice(0, 25) as Array<Record<string, unknown>>;
          if (items.length === 0) return { error: "items is empty" };
          const made: TaskRow[] = [];
          const byTitle = new Map<string, string>();
          try {
            for (const it of items) {
              const title = str(it.title);
              if (!title) continue;
              const due = isoOrEmpty(it.due);
              if (typeof due !== "string") return { error: due.error };
              const parent = byTitle.get(str(it.parent_title).toLowerCase()) ?? "";
              const t = await ad.create(lid, { title: clip(title, 200), notes: clip(str(it.notes), 1000), due_at: due || undefined, parent_task_id: parent || undefined });
              made.push(t);
              byTitle.set(title.toLowerCase(), t.id);
              const p = PRI.indexOf(str(it.priority));
              if (p > 0) await ad.setPriority(t.id, p).catch(() => {});
            }
          } catch (e) {
            for (const t of made.reverse()) await ad.remove(lid, t.id).catch(() => {});
            throw e;
          }
          return {
            result: { created: made.map((t) => ({ id: t.id, title: t.title })) },
            summary: `Created ${made.length} task(s)`,
            undo: async () => { for (const t of [...made].reverse()) await ad.remove(lid, t.id); },
          };
        }),
    }),
    defineTool({
      name: "tasks_update",
      description: "Change one task: title, notes, due date (YYYY-MM-DD, or empty to clear), completion or priority. The user confirms first.",
      risk: "destructive",
      parameters: {
        type: "object",
        properties: {
          list_id: { type: "string" }, task_id: { type: "string" },
          title: { type: "string" }, notes: { type: "string" }, due: { type: "string" },
          completed: { type: "boolean" }, priority: { type: "string", enum: PRI },
        },
        required: ["task_id"],
      },
      label: (a) => `Update task ${str(a.task_id).slice(0, 8)}${a.title ? ` "${clip(str(a.title), 40)}"` : ""}${a.due !== undefined ? ` due ${str(a.due) || "none"}` : ""}${a.completed !== undefined ? ` completed=${a.completed}` : ""}${a.priority ? ` priority ${str(a.priority)}` : ""}`,
      handler: (a) =>
        guard(async () => {
          const lid = await listOf(a);
          if (typeof lid !== "string") return { error: lid.error };
          const cur = (await ad.tasks(lid)).find((t) => t.id === str(a.task_id));
          if (!cur) return { error: "task not found in that list" };
          const undo: Array<() => Promise<void>> = [];
          const patch: { title?: string; notes?: string; due_at?: string } = {};
          if (a.title !== undefined && str(a.title)) patch.title = clip(str(a.title), 200);
          if (a.notes !== undefined) patch.notes = clip(typeof a.notes === "string" ? a.notes : "", 1000);
          if (a.due !== undefined) {
            const d = isoOrEmpty(a.due);
            if (typeof d !== "string") return { error: d.error };
            patch.due_at = d;
          }
          if (Object.keys(patch).length) {
            await ad.update(lid, cur.id, patch);
            undo.push(async () => void (await ad.update(lid, cur.id, { title: cur.title, notes: cur.notes, due_at: cur.due_at })));
          }
          if (typeof a.completed === "boolean" && a.completed !== cur.completed) {
            await ad.setCompleted(lid, cur.id, a.completed);
            undo.push(() => ad.setCompleted(lid, cur.id, cur.completed));
          }
          if (a.priority !== undefined) {
            const p = PRI.indexOf(str(a.priority));
            if (p < 0) return { error: `priority must be one of ${PRI.join(", ")}` };
            const before = (await ad.priorities(lid).catch(() => ({}) as Record<string, number>))[cur.id] ?? 0;
            await ad.setPriority(cur.id, p);
            undo.push(() => ad.setPriority(cur.id, before));
          }
          if (undo.length === 0) return { error: "nothing to change" };
          return { result: { id: cur.id, updated: true }, summary: `Updated "${clip(cur.title, 60)}"`, undo: chainUndo(undo) };
        }),
    }),
    defineTool({
      name: "tasks_set_priorities",
      description: "Set the priority (none, low, medium, high) of several tasks at once, for 'prioritise my tasks'. The user confirms the whole change first. Max 40 tasks.",
      risk: "destructive",
      parameters: {
        type: "object",
        properties: {
          list_id: { type: "string" },
          items: { type: "array", items: { type: "object", properties: { task_id: { type: "string" }, priority: { type: "string", enum: PRI } }, required: ["task_id", "priority"] } },
        },
        required: ["items"],
      },
      label: (a) => `Set priority on ${Array.isArray(a.items) ? a.items.length : 0} task(s)`,
      handler: (a) =>
        guard(async () => {
          const lid = await listOf(a);
          if (typeof lid !== "string") return { error: lid.error };
          const ids = new Set((await ad.tasks(lid)).map((t) => t.id));
          const before = await ad.priorities(lid).catch(() => ({}) as Record<string, number>);
          const done: Array<{ id: string; was: number }> = [];
          for (const it of ((Array.isArray(a.items) ? a.items : []) as Array<Record<string, unknown>>).slice(0, 40)) {
            const id = str(it.task_id);
            const p = PRI.indexOf(str(it.priority));
            if (!ids.has(id) || p < 0) continue;
            await ad.setPriority(id, p);
            done.push({ id, was: before[id] ?? 0 });
          }
          if (!done.length) return { error: "no valid task_id/priority pairs" };
          return { result: { changed: done.length }, summary: `Set priority on ${done.length} task(s)`, undo: async () => { for (const d of done) await ad.setPriority(d.id, d.was); } };
        }),
    }),
    ...(ad.setRepeatTags && ad.repeatTags
      ? [
          defineTool({
            name: "tasks_set_repeat_tags",
            description: "Set the repeat rule and/or tags of tasks. repeat is plain English: 'daily', 'every weekday', 'every 2 weeks on mon,wed', 'every month', or 'none' to stop repeating. tags are up to 5 short words. The user confirms first. Max 25 tasks.",
            risk: "destructive",
            parameters: {
              type: "object",
              properties: {
                list_id: { type: "string" },
                items: {
                  type: "array",
                  items: { type: "object", properties: { task_id: { type: "string" }, repeat: { type: "string" }, tags: { type: "array", items: { type: "string" } } }, required: ["task_id"] },
                },
              },
              required: ["items"],
            },
            label: (a) => `Set repeat/tags on ${Array.isArray(a.items) ? a.items.length : 0} task(s)`,
            handler: (a) =>
              guard(async () => {
                const lid = await listOf(a);
                if (typeof lid !== "string") return { error: lid.error };
                const ids = new Set((await ad.tasks(lid)).map((t) => t.id));
                const before = await ad.repeatTags!(lid).catch(() => ({}) as Record<string, { recurrence: string; tags: string[] }>);
                const done: Array<{ id: string; was: { recurrence: string; tags: string[] } }> = [];
                for (const it of ((Array.isArray(a.items) ? a.items : []) as Array<Record<string, unknown>>).slice(0, 25)) {
                  const id = str(it.task_id);
                  if (!ids.has(id)) continue;
                  const v: { recurrence?: string; tags?: string[] } = {};
                  if (it.repeat !== undefined) {
                    const r = repeatFromText(str(it.repeat));
                    if (r === null) return { error: `could not understand the repeat "${str(it.repeat)}"` };
                    v.recurrence = r;
                  }
                  if (Array.isArray(it.tags)) v.tags = it.tags.map((x) => str(x).toLowerCase().replace(/^#/, "").replace(/\s+/g, "-")).filter(Boolean).slice(0, 5);
                  if (v.recurrence === undefined && v.tags === undefined) continue;
                  await ad.setRepeatTags!(id, v);
                  done.push({ id, was: before[id] ?? { recurrence: "", tags: [] } });
                }
                if (!done.length) return { error: "no valid task_id with a repeat or tags" };
                return { result: { changed: done.length }, summary: `Set repeat/tags on ${done.length} task(s)`, undo: async () => { for (const d of done) await ad.setRepeatTags!(d.id, d.was); } };
              }),
          }),
        ]
      : []),
  ];
}

export function tasksContext(listName: string | null, tasks: TaskRow[]): { label: string; text: string } | null {
  if (!listName) return null;
  const text = tasks.filter((t) => !t.completed).slice(0, 60).map((t) => `- ${clip(t.title, 100)}${t.due_at ? ` (due ${t.due_at.slice(0, 10)})` : ""}`).join("\n");
  return { label: listName, text: text || "(no open tasks)" };
}

export const TASKS_SUGGESTIONS = [
  "Plan my day",
  "Prioritise my open tasks",
  "Turn this text into tasks: ",
  "What is overdue?",
];
