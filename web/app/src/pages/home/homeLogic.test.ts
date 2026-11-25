import { describe, it, expect } from "vitest";
import { eventLabel, glyphForFile, localDayRange, myTasks, relTime, todayEvents, topThreads } from "./homeLogic";
import type { CalendarEvent } from "../calendar/types";
import type { MailThread } from "../mail/types";
import type { Task } from "../tasks/types";

const now = new Date(2026, 5, 10, 10, 0, 0);
const iso = (h: number, m = 0) => new Date(2026, 5, 10, h, m).toISOString();
const ev = (id: string, sh: number, eh: number, extra: Partial<CalendarEvent> = {}): CalendarEvent =>
  ({ id, title: id, start_at: iso(sh), end_at: iso(eh), all_day: false, ...extra }) as CalendarEvent;

describe("homeLogic", () => {
  it("local day range spans exactly one local day", () => {
    const r = localDayRange(now);
    expect(new Date(r.to).getTime() - new Date(r.from).getTime()).toBe(86400000);
  });

  it("today's events: drops finished ones and tasks, flags live and next", () => {
    const rows = todayEvents(
      [ev("done", 8, 9), ev("live", 9, 11), ev("next", 13, 14), ev("later", 15, 16), ev("t", 12, 12, { item_type: "task" }), ev("allday", 0, 23, { all_day: true })],
      now,
    );
    expect(rows.map((r) => r.event.id)).toEqual(["allday", "live", "next", "later"]);
    expect(rows.find((r) => r.event.id === "live")!.live).toBe(true);
    expect(rows.find((r) => r.event.id === "next")!.next).toBe(true);
    expect(rows.find((r) => r.event.id === "later")!.next).toBe(false);
    expect(eventLabel(ev("x", 9, 10, { all_day: true }))).toBe("All day");
    expect(eventLabel(ev("x", 9, 10))).toContain("–");
  });

  it("mail: unread first, then newest", () => {
    const th = (id: string, unread: boolean, at: string): MailThread =>
      ({ thread_id: id, any_unread: unread, latest: { sent_at: at } }) as unknown as MailThread;
    const out = topThreads([th("old", false, "2026-06-01T00:00:00Z"), th("new", false, "2026-06-09T00:00:00Z"), th("u", true, "2026-05-01T00:00:00Z")], 2);
    expect(out.map((t) => t.thread_id)).toEqual(["u", "new"]);
  });

  it("tasks: overdue, then today, then later, then undated; skips done and subtasks", () => {
    const t = (id: string, due: string, extra: Partial<Task> = {}): Task =>
      ({ id, title: id, due_at: due, completed: false, parent_task_id: "", position: 0, ...extra }) as Task;
    const rows = myTasks(
      [
        t("none", ""),
        t("later", new Date(2026, 5, 20).toISOString()),
        t("today", new Date(2026, 5, 10, 17).toISOString()),
        t("over", new Date(2026, 5, 1).toISOString()),
        t("done", new Date(2026, 5, 1).toISOString(), { completed: true }),
        t("sub", new Date(2026, 5, 1).toISOString(), { parent_task_id: "x" }),
      ],
      now,
    );
    expect(rows.map((r) => r.task.id)).toEqual(["over", "today", "later", "none"]);
    expect(rows[0].due).toBe("overdue");
  });

  it("relative times and file glyphs", () => {
    expect(relTime(now.getTime() - 5 * 60000, now)).toBe("5m");
    expect(relTime(now.getTime() - 3 * 3600000, now)).toBe("3h");
    expect(relTime("", now)).toBe("");
    expect(glyphForFile({ mime_type: "application/pdf", name: "a" })).toBe("pdf");
    expect(glyphForFile({ mime_type: "application/octet-stream", name: "budget.xlsx" })).toBe("sheets");
    expect(glyphForFile({ mime_type: "image/png", name: "a.png" })).toBe("photos");
    expect(glyphForFile({ mime_type: "application/vnd.adrient.folder", name: "f" })).toBe("drive");
  });
});
