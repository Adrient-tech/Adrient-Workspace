import { describeRule } from "./quickparse";
import type { Task } from "./types";

export interface TaskExtras {
  task_id: string;
  assignee_id: string;
  assignee_email?: string;
  assignee_name?: string;
  priority: number;
  recurrence: string;
  remind_minutes: number | null;
  board_status: string;
  link_url: string;
  tags?: string[];
}

export const emptyExtras = (id = ""): TaskExtras => ({
  task_id: id,
  assignee_id: "",
  priority: 0,
  recurrence: "",
  remind_minutes: null,
  board_status: "todo",
  link_url: "",
  tags: [],
});

export const RECURRENCES: { value: string; label: string }[] = [
  { value: "", label: "Does not repeat" },
  { value: "daily", label: "Every day" },
  { value: "weekdays", label: "Every weekday" },
  { value: "weekly", label: "Every week" },
  { value: "monthly", label: "Every month" },
  { value: "yearly", label: "Every year" },
];

export const REMINDERS: { value: number | null; label: string }[] = [
  { value: null, label: "No reminder" },
  { value: 0, label: "At due time" },
  { value: 30, label: "30 minutes before" },
  { value: 60, label: "1 hour before" },
  { value: 1440, label: "1 day before" },
];

export const PRIORITIES = ["None", "Low", "Medium", "High"];

/** Due date as sent to the server: the picked day at 09:00 local time, or "" to clear. */
export function dueFromDay(day: string, time?: string | null): string {
  if (!day) return "";
  const [y, m, d] = day.split("-").map(Number);
  if (!y || !m || !d) return "";
  const [hh, mm] = time ? time.split(":").map(Number) : [9, 0];
  return new Date(y, m - 1, d, hh || 0, mm || 0, 0, 0).toISOString();
}

/** YYYY-MM-DD of a stored due date in local time, for <input type="date">. */
export function dayFromDue(iso: string): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (isNaN(d.getTime())) return "";
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

export type QuickWhen = "today" | "tomorrow" | "nextweek" | "nextmonday";

/** Quick reschedule targets, always 09:00 local, relative to `now`. */
export function quickDue(when: QuickWhen, now = new Date()): string {
  const d = new Date(now.getFullYear(), now.getMonth(), now.getDate(), 9, 0, 0, 0);
  switch (when) {
    case "tomorrow":
      d.setDate(d.getDate() + 1);
      break;
    case "nextweek":
      d.setDate(d.getDate() + 7);
      break;
    case "nextmonday": {
      const add = ((8 - d.getDay()) % 7) || 7;
      d.setDate(d.getDate() + add);
      break;
    }
  }
  return d.toISOString();
}

export function isOverdue(t: Pick<Task, "due_at" | "completed">, now = new Date()): boolean {
  return !t.completed && !!t.due_at && new Date(t.due_at).getTime() < now.getTime();
}

export type Column = "todo" | "doing" | "done";

/** Board column of a task: done wins, then the saved board status. */
export function columnOf(t: Task, x?: TaskExtras): Column {
  if (t.completed) return "done";
  return x?.board_status === "doing" ? "doing" : "todo";
}

/** Splits top-level tasks into the three board columns, keeping list order. */
export function toColumns(tasks: Task[], extras: Record<string, TaskExtras>): Record<Column, Task[]> {
  const out: Record<Column, Task[]> = { todo: [], doing: [], done: [] };
  for (const t of tasks) {
    if (t.parent_task_id) continue;
    out[columnOf(t, extras[t.id])].push(t);
  }
  return out;
}

/** Local YYYY-MM-DD key of a due date. */
export const dayKey = dayFromDue;

/** Month grid (Monday first) of day keys; leading and trailing cells are null. */
export function monthCells(year: number, month: number): (string | null)[] {
  const first = new Date(year, month, 1);
  const lead = (first.getDay() + 6) % 7;
  const n = new Date(year, month + 1, 0).getDate();
  const cells: (string | null)[] = Array(lead).fill(null);
  for (let d = 1; d <= n; d++) cells.push(`${year}-${String(month + 1).padStart(2, "0")}-${String(d).padStart(2, "0")}`);
  while (cells.length % 7) cells.push(null);
  return cells;
}

/** Groups tasks that have a due date by local day. */
export function byDueDay(tasks: Task[]): Record<string, Task[]> {
  const out: Record<string, Task[]> = {};
  for (const t of tasks) {
    if (!t.due_at || t.parent_task_id) continue;
    (out[dayKey(t.due_at)] ??= []).push(t);
  }
  return out;
}

/** Progress of the subtasks of a task, e.g. "2/3". */
export function subtaskProgress(subs: Task[]): string {
  if (!subs.length) return "";
  return `${subs.filter((s) => s.completed).length}/${subs.length}`;
}

/** Short label for the repeat rule. */
export function recurrenceLabel(r: string): string {
  const known = RECURRENCES.find((x) => x.value === r)?.label;
  if (known) return known.replace("Every ", "Repeats ").replace("Repeats day", "Repeats daily");
  const d = describeRule(r);
  return d ? d.replace("Every ", "Repeats every ") : "";
}
