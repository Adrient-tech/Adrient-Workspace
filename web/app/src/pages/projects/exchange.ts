// Pure helpers to move a project out of Adrient (Markdown, CSV, iCalendar) and
// in from elsewhere (CSV from Asana or any spreadsheet, Trello board JSON).
import type { Issue, Label, Project } from "./types";
import type { IssuePlan, Milestone } from "./plan";

export const STATUS_LABEL: Record<string, string> = {
  backlog: "Backlog",
  todo: "Todo",
  in_progress: "In Progress",
  done: "Done",
  canceled: "Canceled",
};
const STATUS_ORDER = ["backlog", "todo", "in_progress", "done", "canceled"];
export const PRIORITY_LABEL = ["", "Urgent", "High", "Medium", "Low"];

export interface ExportInput {
  project: Pick<Project, "id" | "name">;
  issues: Issue[];
  plans: Record<string, IssuePlan>;
  milestones: Milestone[];
  labels: Label[];
  brief?: string;
  now?: Date;
}

const nameOfMilestone = (ms: Milestone[], id: string) => ms.find((m) => m.id === id)?.name ?? "";
const labelNames = (labels: Label[], ids: string[]) => ids.map((id) => labels.find((l) => l.id === id)?.name).filter(Boolean) as string[];

function indent(text: string, pad = "  "): string {
  return text.replace(/\r\n?/g, "\n").split("\n").map((l) => (l ? pad + l : l)).join("\n");
}

/** One project as a Markdown document: brief, milestones, issues by status. */
export function projectMarkdown(x: ExportInput): string {
  const now = x.now ?? new Date();
  const done = x.issues.filter((i) => i.status === "done").length;
  const lines = [`# ${x.project.name}`, ""];
  if (x.brief?.trim()) lines.push(x.brief.trim(), "");
  lines.push(`*${x.issues.length} issue${x.issues.length === 1 ? "" : "s"}, ${done} done. Exported ${now.toISOString().slice(0, 10)}.*`, "");
  if (x.milestones.length) {
    lines.push("## Milestones", "");
    for (const m of [...x.milestones].sort((a, b) => (a.due_date || "9").localeCompare(b.due_date || "9"))) {
      const mine = x.issues.filter((i) => x.plans[i.id]?.milestone_id === m.id);
      const d = mine.filter((i) => i.status === "done").length;
      lines.push(`- **${m.name}**${m.due_date ? ` — ${m.due_date}` : ""} · ${d}/${mine.length} done${m.done ? " · reached" : ""}`);
    }
    lines.push("");
  }
  for (const status of STATUS_ORDER) {
    const inCol = x.issues.filter((i) => i.status === status).sort((a, b) => a.number - b.number);
    if (!inCol.length) continue;
    lines.push(`## ${STATUS_LABEL[status]} (${inCol.length})`, "");
    for (const i of inCol) {
      const p = x.plans[i.id];
      const meta = [
        p?.start_date && `starts ${p.start_date}`,
        p?.due_date && `due ${p.due_date}`,
        PRIORITY_LABEL[i.priority],
        i.assignee_name && `@${i.assignee_name}`,
        ...labelNames(x.labels, i.label_ids ?? []).map((n) => `#${n}`),
        p?.milestone_id && nameOfMilestone(x.milestones, p.milestone_id) && `milestone: ${nameOfMilestone(x.milestones, p.milestone_id)}`,
      ].filter(Boolean);
      lines.push(`- [${status === "done" ? "x" : " "}] **${i.identifier} ${i.title || "Untitled"}**${meta.length ? ` — ${meta.join(" · ")}` : ""}`);
      if (i.description?.trim()) lines.push("", indent(i.description.trim()), "");
    }
    lines.push("");
  }
  return `${lines.join("\n").replace(/\n{3,}/g, "\n\n").trim()}\n`;
}

export const CSV_COLUMNS = ["Project", "ID", "Task", "Status", "Done", "Priority", "Assignee", "Labels", "Milestone", "Start", "Due", "Estimate hours", "Logged hours", "Description"];

/** A CSV cell, quoted only when it has to be. */
export function csvCell(v: unknown): string {
  const t = v === null || v === undefined ? "" : String(v);
  return /[",\n\r]/.test(t) ? `"${t.replace(/"/g, '""')}"` : t;
}

/** One row per issue, flat, for a spreadsheet. */
export function projectCsv(x: ExportInput): string {
  const rows = [CSV_COLUMNS.join(",")];
  for (const i of [...x.issues].sort((a, b) => a.number - b.number)) {
    const p = x.plans[i.id];
    rows.push(
      [
        x.project.name, i.identifier, i.title, STATUS_LABEL[i.status] ?? i.status, i.status === "done" ? "yes" : "no",
        PRIORITY_LABEL[i.priority] ?? "", i.assignee_name, labelNames(x.labels, i.label_ids ?? []).join("; "),
        p?.milestone_id ? nameOfMilestone(x.milestones, p.milestone_id) : "", p?.start_date ?? "", p?.due_date ?? "",
        p?.estimate_hours ?? "", p?.logged_hours || "", i.description,
      ].map(csvCell).join(","),
    );
  }
  return `${rows.join("\n")}\n`;
}

function icsText(s: string): string {
  return s.replace(/\\/g, "\\\\").replace(/;/g, "\\;").replace(/,/g, "\\,").replace(/\r?\n/g, "\\n");
}
function foldLine(line: string): string {
  const enc = new TextEncoder();
  if (enc.encode(line).length <= 73) return line;
  let out = "", cur = 0;
  for (const ch of line) {
    const n = enc.encode(ch).length;
    if (cur + n > 73) { out += "\r\n "; cur = 1; }
    out += ch;
    cur += n;
  }
  return out;
}
const compact = (d: string) => d.replace(/-/g, "");
function nextDay(d: string): string {
  const [y, m, dd] = d.split("-").map(Number);
  const t = new Date(Date.UTC(y, m - 1, dd + 1));
  return `${t.getUTCFullYear()}${String(t.getUTCMonth() + 1).padStart(2, "0")}${String(t.getUTCDate()).padStart(2, "0")}`;
}

/** All-day events for every dated, unfinished issue and every milestone. Stable UIDs, so re-importing updates. */
export function projectIcs(x: ExportInput): string {
  const stamp = (x.now ?? new Date()).toISOString().replace(/[-:]/g, "").replace(/\.\d+/, "");
  const out: string[] = ["BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//Adrient Workspace//Projects//EN", "CALSCALE:GREGORIAN", `X-WR-CALNAME:${icsText(x.project.name)}`];
  const ev = (uid: string, date: string, summary: string, desc: string) => {
    out.push("BEGIN:VEVENT", `UID:${uid}@adrient-projects`, `DTSTAMP:${stamp}`, `DTSTART;VALUE=DATE:${compact(date)}`, `DTEND;VALUE=DATE:${nextDay(date)}`, `SUMMARY:${icsText(summary)}`);
    if (desc) out.push(`DESCRIPTION:${icsText(desc)}`);
    out.push("END:VEVENT");
  };
  for (const i of x.issues) {
    const due = x.plans[i.id]?.due_date;
    if (!due || i.status === "done" || i.status === "canceled") continue;
    ev(`issue-${i.id}`, due, `${i.identifier} ${i.title}`, i.description.slice(0, 500));
  }
  for (const m of x.milestones) if (m.due_date) ev(`milestone-${m.id}`, m.due_date, `Milestone: ${m.name}`, m.done ? "Reached" : "");
  out.push("END:VCALENDAR");
  return out.map(foldLine).join("\r\n") + "\r\n";
}

// ---------------------------------------------------------------- import

/** RFC 4180 CSV into rows. Handles BOM, CRLF, quoted newlines and doubled quotes. */
export function parseCsv(text: string): string[][] {
  const src = String(text ?? "").replace(/^\uFEFF/, "");
  const rows: string[][] = [];
  let row: string[] = [], cell = "", quoted = false;
  for (let i = 0; i < src.length; i++) {
    const c = src[i];
    if (quoted) {
      if (c === '"') {
        if (src[i + 1] === '"') { cell += '"'; i++; } else quoted = false;
      } else cell += c;
      continue;
    }
    if (c === '"' && cell === "") quoted = true;
    else if (c === ",") { row.push(cell); cell = ""; }
    else if (c === "\r") continue;
    else if (c === "\n") { row.push(cell); rows.push(row); row = []; cell = ""; }
    else cell += c;
  }
  if (cell !== "" || row.length) { row.push(cell); rows.push(row); }
  return rows.filter((r) => r.some((c) => c.trim() !== ""));
}

export type CsvField = "title" | "description" | "status" | "due" | "start" | "priority" | "assignee" | "labels" | "milestone" | "done";
export const CSV_FIELDS: { key: CsvField; label: string }[] = [
  { key: "title", label: "Task title (required)" },
  { key: "description", label: "Description" },
  { key: "status", label: "Status, column or section" },
  { key: "due", label: "Due date" },
  { key: "start", label: "Start date" },
  { key: "priority", label: "Priority" },
  { key: "assignee", label: "Assignee" },
  { key: "labels", label: "Labels or tags" },
  { key: "milestone", label: "Milestone" },
  { key: "done", label: "Done or completed" },
];
const HINTS: Record<CsvField, string[]> = {
  title: ["task name", "task title", "name", "title", "task", "summary", "subject"],
  description: ["description", "notes", "note", "details", "body"],
  status: ["section/column", "section", "column", "list", "status", "stage", "bucket"],
  due: ["due date", "due at", "due on", "due", "deadline", "end date"],
  start: ["start date", "start at", "start on", "start", "begin"],
  priority: ["priority", "importance"],
  assignee: ["assignee", "assigned to", "owner", "responsible"],
  labels: ["labels", "label", "tags", "tag", "categories"],
  milestone: ["milestone", "epic", "phase", "release"],
  done: ["completed at", "completed", "complete", "done", "is done", "finished"],
};
const norm = (s: unknown) => String(s ?? "").trim().toLowerCase();

/** Guess which field each header is. The first matching column takes a field. */
export function detectColumns(headers: string[]): (CsvField | null)[] {
  const taken = new Set<CsvField>();
  const mapping: (CsvField | null)[] = headers.map(() => null);
  for (const field of ["title", "status", "due", "start", "description", "assignee", "labels", "priority", "milestone", "done"] as CsvField[]) {
    let best = -1, bestScore = 0;
    headers.forEach((h, idx) => {
      if (mapping[idx]) return;
      const n = norm(h);
      const score = HINTS[field].includes(n) ? 2 : HINTS[field].some((hint) => n.includes(hint)) ? 1 : 0;
      if (score > bestScore) { bestScore = score; best = idx; }
    });
    if (best >= 0 && !taken.has(field)) { mapping[best] = field; taken.add(field); }
  }
  return mapping;
}

const FALSY = new Set(["", "no", "false", "0", "n", "incomplete", "not done"]);
export const meansDone = (v: unknown) => !FALSY.has(norm(v));

const PRIORITY_ALIASES: Record<string, number> = {
  urgent: 1, critical: 1, highest: 1, p0: 1, high: 2, p1: 2, medium: 3, normal: 3, med: 3, p2: 3, low: 4, lowest: 4, minor: 4, p3: 4,
};
/** 1..4, 0 for empty, null for something unreadable. */
export function readPriority(v: unknown): number | null {
  const t = norm(v);
  if (!t) return 0;
  return PRIORITY_ALIASES[t] ?? null;
}
/** A YYYY-MM-DD date, "" for empty, null for text that cannot be read unambiguously. */
export function readDate(v: unknown): string | null {
  const t = String(v ?? "").trim();
  if (!t) return "";
  const iso = t.slice(0, 10).replace(/\//g, "-");
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(iso);
  if (!m) return null;
  const d = new Date(Date.UTC(+m[1], +m[2] - 1, +m[3]));
  return d.getUTCFullYear() === +m[1] && d.getUTCMonth() === +m[2] - 1 && d.getUTCDate() === +m[3] ? iso : null;
}
/** Map a free-text status or column name to one of our statuses. */
export function readStatus(v: unknown): string {
  const t = norm(v);
  if (!t) return "todo";
  if (/(done|complete|closed|finish|shipped|resolved)/.test(t)) return "done";
  if (/(cancel|won'?t|abandon|reject)/.test(t)) return "canceled";
  if (/(progress|doing|active|review|wip|started|testing)/.test(t)) return "in_progress";
  if (/(backlog|icebox|ideas?|later|someday)/.test(t)) return "backlog";
  return "todo";
}
const splitList = (v: unknown) => String(v ?? "").split(/[,;|]/).map((x) => x.trim()).filter(Boolean);

export interface ImportDraft {
  title: string;
  description: string;
  status: string;
  priority: number;
  assignee: string;
  labels: string[];
  milestone: string;
  start: string;
  due: string;
  checklist?: string[];
}
export interface ImportReport {
  tasks: number;
  skippedRows: number;
  unreadableDates: number;
  unreadablePriorities: number;
  notes: string[];
}

/** rows[0] is the header; mapping[i] is the field column i becomes, or null to skip it. */
export function csvToDrafts(rows: string[][], mapping: (CsvField | null)[]): { drafts: ImportDraft[]; report: ImportReport } {
  const report: ImportReport = { tasks: 0, skippedRows: 0, unreadableDates: 0, unreadablePriorities: 0, notes: [] };
  const get = (row: string[], k: CsvField) => { const i = mapping.indexOf(k); return i >= 0 ? row[i] : undefined; };
  const drafts: ImportDraft[] = [];
  for (const row of rows.slice(1)) {
    const title = String(get(row, "title") ?? "").trim();
    if (!title) { report.skippedRows++; continue; }
    const due = readDate(get(row, "due")), start = readDate(get(row, "start"));
    if (due === null || start === null) report.unreadableDates++;
    const pr = mapping.includes("priority") ? readPriority(get(row, "priority")) : 0;
    if (pr === null) report.unreadablePriorities++;
    const done = mapping.includes("done") && meansDone(get(row, "done"));
    drafts.push({
      title: title.slice(0, 200),
      description: String(get(row, "description") ?? "").trim(),
      status: done ? "done" : readStatus(get(row, "status")),
      priority: pr ?? 0,
      assignee: String(get(row, "assignee") ?? "").trim().split(/[;|]/)[0].trim(),
      labels: splitList(get(row, "labels")),
      milestone: String(get(row, "milestone") ?? "").trim(),
      start: start ?? "",
      due: due ?? "",
    });
    report.tasks++;
  }
  if (report.unreadableDates) report.notes.push(`${report.unreadableDates} date(s) could not be read (only YYYY-MM-DD is unambiguous) and were left empty`);
  if (report.unreadablePriorities) report.notes.push(`${report.unreadablePriorities} priority value(s) did not match Urgent, High, Medium or Low and were left unset`);
  const skipped = mapping.filter((m) => m === null).length;
  if (skipped) report.notes.push(`${skipped} column(s) in the file were not imported`);
  return { drafts, report };
}

interface TrelloBoard {
  name?: string;
  lists?: { id: string; name: string; closed?: boolean }[];
  cards?: { id: string; name: string; desc?: string; idList: string; closed?: boolean; due?: string | null; start?: string | null; dueComplete?: boolean; idLabels?: string[]; idChecklists?: string[]; pos?: number }[];
  labels?: { id: string; name: string; color?: string }[];
  checklists?: { id: string; idCard: string; checkItems?: { name: string; state?: string; pos?: number }[] }[];
}

/** A Trello board export (JSON) as import drafts. Archived cards and lists are skipped, checklists are appended to the description. */
export function trelloToDrafts(json: unknown): { name: string; drafts: ImportDraft[]; report: ImportReport } {
  const b = json as TrelloBoard;
  if (!b || typeof b !== "object" || !Array.isArray(b.cards) || !Array.isArray(b.lists)) throw new Error("This does not look like a Trello board export");
  const report: ImportReport = { tasks: 0, skippedRows: 0, unreadableDates: 0, unreadablePriorities: 0, notes: [] };
  const lists = new Map(b.lists.map((l) => [l.id, l]));
  const labels = new Map((b.labels ?? []).map((l) => [l.id, l.name || l.color || ""]));
  const checks = new Map<string, NonNullable<TrelloBoard["checklists"]>>();
  for (const c of b.checklists ?? []) checks.set(c.idCard, [...(checks.get(c.idCard) ?? []), c]);
  const drafts: ImportDraft[] = [];
  const cards = [...b.cards].sort((a, c) => (a.pos ?? 0) - (c.pos ?? 0));
  for (const c of cards) {
    const list = lists.get(c.idList);
    if (c.closed || !list || list.closed) { report.skippedRows++; continue; }
    const items = (checks.get(c.id) ?? []).flatMap((cl) => [...(cl.checkItems ?? [])].sort((x, y) => (x.pos ?? 0) - (y.pos ?? 0)));
    const checklist = items.map((i) => `- [${i.state === "complete" ? "x" : " "}] ${i.name}`);
    const desc = [c.desc?.trim(), checklist.length ? `Checklist:\n${checklist.join("\n")}` : ""].filter(Boolean).join("\n\n");
    drafts.push({
      title: (c.name || "Untitled").slice(0, 200),
      description: desc,
      status: c.dueComplete ? "done" : readStatus(list.name),
      priority: 0,
      assignee: "",
      labels: (c.idLabels ?? []).map((id) => labels.get(id) ?? "").filter(Boolean),
      milestone: "",
      start: c.start ? c.start.slice(0, 10) : "",
      due: c.due ? c.due.slice(0, 10) : "",
    });
    report.tasks++;
  }
  if (report.skippedRows) report.notes.push(`${report.skippedRows} archived card(s) were skipped`);
  return { name: b.name || "Imported board", drafts, report };
}
