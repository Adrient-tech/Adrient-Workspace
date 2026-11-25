/**
 * Assistant tools for Keep. Note text can arrive from shared notes, so it is
 * data only. Creating or changing a note asks the user to confirm and can be
 * undone (a note created by the assistant is trashed on undo). Notes are
 * never deleted by the assistant.
 */
import { chainUndo, clip, defineTool, guard, str, strList, type AssistantTool } from "@/lib/ai";

export interface KNote {
  id: string;
  title: string;
  body: string;
  color: string;
  pinned: boolean;
  archived: boolean;
  labels: string[];
  checklist: { text: string; checked: boolean }[];
}
export type KInput = Omit<KNote, "id">;

export interface KeepAdapter {
  list(): Promise<KNote[]>;
  create(i: Partial<KInput>): Promise<KNote>;
  update(id: string, i: KInput): Promise<KNote>;
  trash(id: string): Promise<void>;
}

const toInput = (n: KNote): KInput => ({ title: n.title, body: n.body, color: n.color, pinned: n.pinned, archived: n.archived, labels: n.labels, checklist: n.checklist });
const row = (n: KNote, full = false) => ({
  id: n.id, title: clip(n.title, 100), body: clip(n.body, full ? 3000 : 200), pinned: n.pinned, labels: n.labels,
  checklist: (n.checklist ?? []).slice(0, full ? 60 : 8).map((c) => ({ text: clip(c.text, 100), done: c.checked })),
});
const items = (v: unknown): { text: string; checked: boolean }[] =>
  (Array.isArray(v) ? v : []).slice(0, 80).map((x) => (typeof x === "string" ? { text: x, checked: false } : { text: str((x as { text?: unknown }).text), checked: (x as { checked?: unknown }).checked === true })).filter((c) => c.text).map((c) => ({ ...c, text: clip(c.text, 200) }));

export function createKeepTools(ad: KeepAdapter): AssistantTool[] {
  return [
    defineTool({
      name: "keep_find_notes",
      description: "List or search the user's notes (title, text, labels). Up to 25 results; pass full=true for complete text (use before summarising or reorganising one note).",
      risk: "read",
      parameters: { type: "object", properties: { query: { type: "string" }, note_id: { type: "string" }, full: { type: "boolean" } } },
      handler: (a) => guard(async () => {
        const q = str(a.query).toLowerCase();
        const id = str(a.note_id);
        const ns = (await ad.list()).filter((n) => (id ? n.id === id : !q || [n.title, n.body, ...n.labels, ...n.checklist.map((c) => c.text)].join(" ").toLowerCase().includes(q)));
        return { result: { count: ns.length, notes: ns.slice(0, 25).map((n) => row(n, a.full === true || !!id)) } };
      }),
    }),
    defineTool({
      name: "keep_create_note",
      description: "Create a note. Give a title and either a body or a checklist (list of items). The user confirms first.",
      risk: "destructive",
      parameters: {
        type: "object",
        properties: { title: { type: "string" }, body: { type: "string" }, checklist: { type: "array", items: { type: "string" } }, labels: { type: "array", items: { type: "string" } } },
        required: ["title"],
      },
      label: (a) => `Create note "${clip(str(a.title), 60)}"${Array.isArray(a.checklist) ? ` with ${a.checklist.length} checklist items` : ""}`,
      handler: (a) => guard(async () => {
        const title = str(a.title);
        if (!title) return { error: "title is required" };
        const n = await ad.create({ title: clip(title, 200), body: clip(typeof a.body === "string" ? a.body : "", 10000), checklist: items(a.checklist), labels: strList(a.labels, 10) });
        return { result: { id: n.id, title: n.title }, summary: `Created note "${clip(n.title, 60)}"`, undo: () => ad.trash(n.id) };
      }),
    }),
    defineTool({
      name: "keep_update_note",
      description: "Rewrite one note: new title, body and/or checklist (replaces it: send the complete list, keep checked state of items that stay). Use it to summarise into a note, turn text into a checklist, or reorganise. The user confirms first.",
      risk: "destructive",
      parameters: {
        type: "object",
        properties: {
          note_id: { type: "string" }, title: { type: "string" }, body: { type: "string" },
          checklist: { type: "array", items: { type: "object", properties: { text: { type: "string" }, checked: { type: "boolean" } }, required: ["text"] } },
        },
        required: ["note_id"],
      },
      label: (a) => `Rewrite note ${str(a.note_id).slice(0, 8)}${a.title ? ` "${clip(str(a.title), 50)}"` : ""}${a.checklist ? " (checklist)" : ""}${a.body !== undefined ? " (text)" : ""}`,
      handler: (a) => guard(async () => {
        const cur = (await ad.list()).find((n) => n.id === str(a.note_id));
        if (!cur) return { error: "note not found" };
        const next = toInput(cur);
        if (str(a.title)) next.title = clip(str(a.title), 200);
        if (a.body !== undefined) next.body = clip(typeof a.body === "string" ? a.body : "", 10000);
        if (a.checklist !== undefined) next.checklist = items(a.checklist);
        const before = toInput(cur);
        await ad.update(cur.id, next);
        return { result: { id: cur.id, updated: true }, summary: `Updated note "${clip(next.title || cur.title, 60)}"`, undo: chainUndo([async () => void (await ad.update(cur.id, before))]) };
      }),
    }),
  ];
}

export function keepContext(count: number, filter: string): { label: string; text: string } {
  return { label: `${count} notes · ${filter}`, text: "" };
}

export const KEEP_SUGGESTIONS = ["Summarise my notes about the project", "Turn my newest note into a checklist", "Make a packing checklist for a 3-day trip", "Which notes look like duplicates?"];
