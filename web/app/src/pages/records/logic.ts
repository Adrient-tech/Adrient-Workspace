// Filter operators per value kind, summary functions and kanban/calendar grouping for the Records API.
import type { Field, FieldType, LinkVal, Node, Query, Rec, Table, View } from "./api";

export const COMPUTED: FieldType[] = ["lookup", "rollup", "count", "formula", "autonumber", "created_time", "modified_time", "created_by", "modified_by", "button"];
export const isComputed = (t: FieldType) => COMPUTED.includes(t);
export const isEditable = (f: Field) => !isComputed(f.type) && f.type !== "attachment";

export const TYPE_LABEL: Record<FieldType, string> = {
  text: "Single line text", long_text: "Long text", attachment: "Attachment", checkbox: "Checkbox", single_select: "Single select",
  multi_select: "Multiple select", date: "Date", phone: "Phone number", email: "Email", url: "URL", number: "Number", currency: "Currency",
  percent: "Percent", duration: "Duration", rating: "Rating", barcode: "Barcode", link: "Link to another record", lookup: "Lookup",
  rollup: "Rollup", count: "Count", formula: "Formula", autonumber: "Autonumber", created_time: "Created time", modified_time: "Last modified time",
  created_by: "Created by", modified_by: "Last modified by", button: "Button",
};
export const TYPE_ORDER: FieldType[] = ["text", "long_text", "attachment", "checkbox", "single_select", "multi_select", "date", "phone", "email", "url", "number", "currency", "percent", "duration", "rating", "barcode", "link", "lookup", "rollup", "count", "formula", "autonumber", "created_time", "modified_time", "created_by", "modified_by", "button"];

export type Kind = "text" | "number" | "select" | "multi" | "checkbox" | "date" | "link" | "attachment" | "list";
export function kindOf(f: Field): Kind {
  switch (f.type) {
    case "number": case "currency": case "percent": case "duration": case "rating": case "autonumber": case "count": return "number";
    case "single_select": return "select";
    case "multi_select": return "multi";
    case "checkbox": return "checkbox";
    case "date": case "created_time": case "modified_time": return "date";
    case "link": return "link";
    case "attachment": return "attachment";
    case "lookup": return "list";
    default: return "text";
  }
}

export const OPERATORS: Record<Kind, [string, string][]> = {
  text: [["contains", "contains"], ["doesNotContain", "does not contain"], ["is", "is"], ["isNot", "is not"], ["isEmpty", "is empty"], ["isNotEmpty", "is not empty"]],
  number: [["=", "="], ["!=", "≠"], ["<", "<"], [">", ">"], ["<=", "≤"], [">=", "≥"], ["isEmpty", "is empty"], ["isNotEmpty", "is not empty"]],
  select: [["is", "is"], ["isNot", "is not"], ["isAnyOf", "is any of"], ["isNoneOf", "is none of"], ["isEmpty", "is empty"], ["isNotEmpty", "is not empty"]],
  multi: [["hasAnyOf", "has any of"], ["hasAllOf", "has all of"], ["isExactly", "is exactly"], ["hasNoneOf", "has none of"], ["isEmpty", "is empty"], ["isNotEmpty", "is not empty"]],
  checkbox: [["is", "is"]],
  date: [["is", "is"], ["isBefore", "is before"], ["isAfter", "is after"], ["isOnOrBefore", "is on or before"], ["isOnOrAfter", "is on or after"], ["isWithin", "is within"], ["isEmpty", "is empty"], ["isNotEmpty", "is not empty"]],
  link: [["contains", "contains"], ["doesNotContain", "does not contain"], ["isEmpty", "is empty"], ["isNotEmpty", "is not empty"]],
  attachment: [["filenameContains", "filename contains"], ["hasFileType", "has file type"], ["isEmpty", "is empty"], ["isNotEmpty", "is not empty"]],
  list: [["contains", "contains"], ["doesNotContain", "does not contain"], ["isEmpty", "is empty"], ["isNotEmpty", "is not empty"]],
};
export const DATE_MODES: [string, string][] = [["today", "today"], ["tomorrow", "tomorrow"], ["yesterday", "yesterday"], ["oneWeekAgo", "one week ago"], ["oneWeekFromNow", "one week from now"], ["oneMonthAgo", "one month ago"], ["oneMonthFromNow", "one month from now"], ["daysAgo", "number of days ago"], ["daysFromNow", "number of days from now"], ["exactDate", "exact date"]];
export const WITHIN_MODES: [string, string][] = [["pastWeek", "the past week"], ["pastMonth", "the past month"], ["pastYear", "the past year"], ["nextWeek", "the next week"], ["nextMonth", "the next month"], ["nextYear", "the next year"], ["pastDays", "the past number of days"], ["nextDays", "the next number of days"]];
export const NO_VALUE = new Set(["isEmpty", "isNotEmpty"]);

export function defaultOp(f: Field): string {
  return OPERATORS[kindOf(f)][0][0];
}
export function newCondition(f: Field): Node {
  const op = defaultOp(f);
  return { field: f.id, op, value: kindOf(f) === "checkbox" ? true : undefined };
}

/** Drops conditions that are not complete yet, as the server does. */
export function cleanFilter(n: Node | undefined): Node | undefined {
  if (!n) return undefined;
  if (n.field) {
    if (!n.op) return undefined;
    if (NO_VALUE.has(n.op)) return n;
    const v = n.value;
    if (typeof v === "boolean") return n;
    if (v === undefined || v === null || v === "") return undefined;
    if (Array.isArray(v) && v.length === 0) return undefined;
    return n;
  }
  const conds = (n.conds ?? []).map(cleanFilter).filter(Boolean) as Node[];
  return conds.length ? { logic: n.logic ?? "and", conds } : undefined;
}

export function filterCount(n: Node | undefined): number {
  const c = cleanFilter(n);
  if (!c) return 0;
  if (c.field) return 1;
  return (c.conds ?? []).reduce((a, x) => a + filterCount(x), 0);
}

/** The query a view sends to the server. */
export function viewQuery(v: View | undefined, search: string, limit = 200, offset = 0): Query {
  const c = v?.config ?? {};
  return { filter: cleanFilter(c.filter), sort: c.sort ?? [], group_by: c.group_by ?? [], search: search.trim() || undefined, limit, offset };
}

/** Fields of a table in the view's order, without hidden ones. The first field is always shown first. */
export function visibleFields(t: Table, v: View | undefined, showHidden = false): Field[] {
  const c = v?.config ?? {};
  const order = c.field_order ?? [];
  const byId = new Map(t.fields.map((f) => [f.id, f]));
  const first = t.fields[0];
  const rest = t.fields.slice(1);
  const idx = (id: string) => { const i = order.indexOf(id); return i < 0 ? 1e6 + (rest.findIndex((f) => f.id === id)) : i; };
  const sorted = [...rest].sort((a, b) => idx(a.id) - idx(b.id));
  const hidden = new Set(c.hidden ?? []);
  void byId;
  return [first, ...sorted.filter((f) => showHidden || !hidden.has(f.id))];
}

export function choiceName(f: Field, id: unknown): string {
  return f.options.choices?.find((c) => c.id === id)?.name ?? String(id ?? "");
}
export function choiceColor(f: Field, id: unknown): string {
  return f.options.choices?.find((c) => c.id === id)?.color ?? "#8a94a6";
}

export function fmtNumber(n: number, p: number): string {
  return n.toLocaleString("en-IN", { minimumFractionDigits: p, maximumFractionDigits: p });
}

export function fmtDuration(sec: number): string {
  const neg = sec < 0 ? "-" : "";
  const s = Math.round(Math.abs(sec));
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${neg}${Math.floor(s / 3600)}:${pad(Math.floor((s % 3600) / 60))}:${pad(s % 60)}`;
}

/** The text a cell shows. */
export function cellText(f: Field, v: unknown): string {
  if (v === null || v === undefined || v === "") return "";
  if (typeof v === "object" && v !== null && !Array.isArray(v) && "error" in (v as object)) return "#ERROR";
  const p = f.options.precision ?? (f.type === "currency" ? 2 : 0);
  switch (f.type) {
    case "checkbox": return v ? "✓" : "";
    case "single_select": return choiceName(f, v);
    case "multi_select": return (v as string[]).map((id) => choiceName(f, id)).join(", ");
    case "number": case "count": case "autonumber": return typeof v === "number" ? (f.type === "number" && f.options.precision === undefined ? String(v) : fmtNumber(v, f.type === "autonumber" || f.type === "count" ? 0 : p)) : String(v);
    case "currency": return typeof v === "number" ? `${f.options.symbol ?? "₹"}${fmtNumber(v, p)}` : String(v);
    case "percent": return typeof v === "number" ? `${fmtNumber(v * 100, p)}%` : String(v);
    case "duration": return typeof v === "number" ? fmtDuration(v) : String(v);
    case "rating": return "★".repeat(Number(v));
    case "link": return (v as LinkVal[]).map((l) => l.name).join(", ");
    case "attachment": return (v as { name: string }[]).map((a) => a.name).join(", ");
    case "date": case "created_time": case "modified_time": {
      const s = String(v);
      const d = new Date(s.length === 10 ? s + "T00:00:00" : s);
      if (Number.isNaN(d.getTime())) return s;
      return f.options.include_time || s.length > 10 ? d.toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" }) : d.toLocaleDateString(undefined, { dateStyle: "medium" });
    }
  }
  if (Array.isArray(v)) return v.map((x) => (typeof x === "object" && x ? (x as { name?: string }).name ?? "" : String(x))).join(", ");
  if (typeof v === "number") return String(v);
  return String(v);
}

/** Text used for editing a cell (what a person types). */
export function editText(f: Field, v: unknown): string {
  if (v === null || v === undefined) return "";
  switch (f.type) {
    case "percent": return typeof v === "number" ? String(Math.round(v * 1e6) / 1e4) + "%" : "";
    case "duration": return typeof v === "number" ? fmtDuration(v) : "";
    case "date": return String(v).slice(0, f.options.include_time ? 16 : 10);
    case "multi_select": return (v as string[]).map((id) => choiceName(f, id)).join(", ");
    case "single_select": return choiceName(f, v);
  }
  return String(v);
}

/** Splits pasted text (TSV, CSV with quotes) into rows of cells. */
export function parsePasted(text: string): string[][] {
  const t = text.replace(/\r\n?/g, "\n").replace(/\n$/, "");
  if (!t) return [];
  const delim = t.includes("\t") ? "\t" : ",";
  if (!t.includes('"')) return t.split("\n").map((l) => l.split(delim));
  const rows: string[][] = [];
  let row: string[] = [], cell = "", q = false;
  for (let i = 0; i < t.length; i++) {
    const c = t[i];
    if (q) {
      if (c === '"' && t[i + 1] === '"') { cell += '"'; i++; }
      else if (c === '"') q = false;
      else cell += c;
    } else if (c === '"') q = true;
    else if (c === delim) { row.push(cell); cell = ""; }
    else if (c === "\n") { row.push(cell); rows.push(row); row = []; cell = ""; }
    else cell += c;
  }
  row.push(cell);
  rows.push(row);
  return rows;
}

// ---- kanban / calendar ------------------------------------------------------

export interface Column { id: string; name: string; color: string; recs: Rec[] }
/** Stacks for a kanban view: one per option of a select field, plus "No value". */
export function kanbanColumns(f: Field | undefined, recs: Rec[]): Column[] {
  if (!f) return [];
  if (f.type === "checkbox") {
    return [
      { id: "true", name: "Checked", color: "#2f9e44", recs: recs.filter((r) => !!r.fields[f.id]) },
      { id: "", name: "Unchecked", color: "#8a94a6", recs: recs.filter((r) => !r.fields[f.id]) },
    ];
  }
  const cols: Column[] = (f.options.choices ?? []).map((c) => ({ id: c.id, name: c.name, color: c.color ?? "#8a94a6", recs: [] }));
  const none: Column = { id: "", name: "No value", color: "#c4c9d4", recs: [] };
  const byId = new Map(cols.map((c) => [c.id, c]));
  for (const r of recs) {
    const v = r.fields[f.id];
    if (f.type === "link") {
      const first = (v as LinkVal[] | undefined)?.[0];
      let col = first ? byId.get(first.id) : undefined;
      if (first && !col) { col = { id: first.id, name: first.name, color: "#4263eb", recs: [] }; cols.push(col); byId.set(first.id, col); }
      (col ?? none).recs.push(r);
    } else {
      (byId.get(v as string) ?? none).recs.push(r);
    }
  }
  return [none, ...cols];
}

export function monthGrid(year: number, month: number): { date: string; inMonth: boolean }[] {
  const first = new Date(year, month, 1);
  const start = new Date(year, month, 1 - first.getDay());
  const out: { date: string; inMonth: boolean }[] = [];
  for (let i = 0; i < 42; i++) {
    const d = new Date(start.getFullYear(), start.getMonth(), start.getDate() + i);
    out.push({ date: isoDay(d), inMonth: d.getMonth() === month });
  }
  return out;
}
export function isoDay(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}
/** Records on each day; a record with an end date field spans every day between. */
export function recordsByDay(recs: Rec[], startField: string, endField?: string): Map<string, Rec[]> {
  const out = new Map<string, Rec[]>();
  const push = (d: string, r: Rec) => { const l = out.get(d) ?? []; l.push(r); out.set(d, l); };
  for (const r of recs) {
    const s = r.fields[startField] as string | undefined;
    if (!s) continue;
    const a = s.slice(0, 10);
    const e = endField ? (r.fields[endField] as string | undefined)?.slice(0, 10) : undefined;
    if (!e || e <= a) { push(a, r); continue; }
    const d = new Date(a + "T00:00:00");
    for (let i = 0; i < 62; i++) {
      const day = isoDay(new Date(d.getFullYear(), d.getMonth(), d.getDate() + i));
      if (day > e) break;
      push(day, r);
    }
  }
  return out;
}

// ---- summaries ---------------------------------------------------------------

export type Summary = "none" | "count" | "empty" | "filled" | "sum" | "avg" | "min" | "max";
export function summarize(f: Field, recs: Rec[], fn: Summary): string {
  if (fn === "none") return "";
  const vals = recs.map((r) => r.fields[f.id]);
  const filled = vals.filter((v) => v !== null && v !== undefined && v !== "" && !(Array.isArray(v) && v.length === 0));
  if (fn === "count") return String(vals.length);
  if (fn === "filled") return String(filled.length);
  if (fn === "empty") return String(vals.length - filled.length);
  const nums = filled.filter((v): v is number => typeof v === "number");
  if (!nums.length) return "";
  const sum = nums.reduce((a, b) => a + b, 0);
  const v = fn === "sum" ? sum : fn === "avg" ? sum / nums.length : fn === "min" ? Math.min(...nums) : Math.max(...nums);
  return cellText(f, f.type === "rating" ? Math.round(v) : v);
}
