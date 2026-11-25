// Pure helpers for the Today home cards (no network, easy to test).
import type { CalendarEvent } from "../calendar/types";
import type { MailThread } from "../mail/types";
import type { Task } from "../tasks/types";
import type { DriveFile } from "../drive/types";

/** localDayRange returns RFC3339 bounds of the local calendar day of `now`. */
export function localDayRange(now: Date): { from: string; to: string } {
  const a = new Date(now.getFullYear(), now.getMonth(), now.getDate(), 0, 0, 0, 0);
  const b = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1, 0, 0, 0, 0);
  return { from: a.toISOString(), to: b.toISOString() };
}

export interface TodayEvent {
  event: CalendarEvent;
  /** Happening right now. */
  live: boolean;
  /** The first upcoming event of the day. */
  next: boolean;
  label: string;
}

function fmtTime(d: Date): string {
  return d.toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
}

/** eventLabel is "All day" or "9:30 AM – 10:00 AM". */
export function eventLabel(e: CalendarEvent): string {
  if (e.all_day) return "All day";
  const s = new Date(e.start_at);
  const f = new Date(e.end_at);
  if (isNaN(s.getTime())) return "";
  return isNaN(f.getTime()) ? fmtTime(s) : `${fmtTime(s)} – ${fmtTime(f)}`;
}

/** todayEvents keeps today's real events (not tasks), drops the ones already
 *  over, sorts by start and flags the live and the next one. */
export function todayEvents(events: CalendarEvent[], now: Date, limit = 5): TodayEvent[] {
  const t = now.getTime();
  const kept = events
    .filter((e) => !e.item_type || e.item_type === "event" || e.item_type === "out_of_office" || e.item_type === "focus_time")
    .filter((e) => {
      const end = new Date(e.end_at).getTime();
      return e.all_day || isNaN(end) || end > t;
    })
    .sort((a, b) => {
      if (a.all_day !== b.all_day) return a.all_day ? -1 : 1;
      return new Date(a.start_at).getTime() - new Date(b.start_at).getTime();
    });
  let nextSet = false;
  return kept.slice(0, limit).map((event) => {
    const start = new Date(event.start_at).getTime();
    const end = new Date(event.end_at).getTime();
    const live = !event.all_day && start <= t && end > t;
    const next = !event.all_day && !live && !nextSet && start > t;
    if (next) nextSet = true;
    return { event, live, next, label: eventLabel(event) };
  });
}

/** topThreads lists unread inbox threads first, then the newest others. */
export function topThreads(threads: MailThread[], limit = 4): MailThread[] {
  const when = (t: MailThread) => new Date(t.latest?.sent_at ?? 0).getTime() || 0;
  return [...threads]
    .sort((a, b) => Number(b.any_unread) - Number(a.any_unread) || when(b) - when(a))
    .slice(0, limit);
}

/** relTime renders "5m", "3h", "2d" or a short date for a past timestamp. */
export function relTime(iso: string | number, now: Date): string {
  const t = typeof iso === "number" ? iso : new Date(iso).getTime();
  if (!t || isNaN(t)) return "";
  const m = Math.max(0, Math.round((now.getTime() - t) / 60000));
  if (m < 1) return "now";
  if (m < 60) return `${m}m`;
  if (m < 60 * 24) return `${Math.round(m / 60)}h`;
  if (m < 60 * 24 * 7) return `${Math.round(m / 1440)}d`;
  return new Date(t).toLocaleDateString([], { month: "short", day: "numeric" });
}

export interface TaskRow {
  task: Task;
  due: "overdue" | "today" | "later" | "none";
  label: string;
}

/** myTasks picks open top-level tasks: overdue first, then due today, then the
 *  nearest later ones, then undated. */
export function myTasks(tasks: Task[], now: Date, limit = 5): TaskRow[] {
  const startToday = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime();
  const endToday = startToday + 86400000;
  const rows: TaskRow[] = tasks
    .filter((t) => !t.completed && !t.parent_task_id)
    .map((task) => {
      const d = task.due_at ? new Date(task.due_at).getTime() : NaN;
      if (isNaN(d)) return { task, due: "none" as const, label: "" };
      if (d < startToday) return { task, due: "overdue" as const, label: "Overdue" };
      if (d < endToday) return { task, due: "today" as const, label: "Today" };
      return {
        task,
        due: "later" as const,
        label: new Date(d).toLocaleDateString([], { month: "short", day: "numeric" }),
      };
    });
  const rank = { overdue: 0, today: 1, later: 2, none: 3 } as const;
  rows.sort((a, b) => {
    if (rank[a.due] !== rank[b.due]) return rank[a.due] - rank[b.due];
    return (new Date(a.task.due_at).getTime() || 0) - (new Date(b.task.due_at).getTime() || 0) || a.task.position - b.task.position;
  });
  return rows.slice(0, limit);
}

/** glyphForFile maps a Drive file to an Adrient glyph by mime type. */
export function glyphForFile(f: Pick<DriveFile, "mime_type" | "name">): string {
  const m = (f.mime_type || "").toLowerCase();
  const n = (f.name || "").toLowerCase();
  if (m === "application/vnd.adrient.folder") return "drive";
  if (m.includes("pdf") || n.endsWith(".pdf")) return "pdf";
  if (m.includes("spreadsheet") || m.includes("excel") || m.includes("csv") || /\.(xlsx?|csv|ods)$/.test(n)) return "sheets";
  if (m.includes("presentation") || m.includes("powerpoint") || /\.(pptx?|odp)$/.test(n)) return "slides";
  if (m.includes("word") || m.includes("document") || m.startsWith("text/") || /\.(docx?|odt|txt|md)$/.test(n)) return "docs";
  if (m.startsWith("image/")) return "photos";
  if (m.startsWith("video/")) return "video";
  if (m.startsWith("audio/")) return "music";
  return "drive";
}

/** fileUpdatedAt reads Drive's unix-seconds string as epoch millis. */
export function fileUpdatedAt(f: Pick<DriveFile, "updated_at">): number {
  const s = Number(f.updated_at);
  return s > 0 ? s * 1000 : 0;
}
