/**
 * Assistant tools for Docs (reference set, see docs/ai.md). Tools talk to the
 * open document only through a DocsAdapter; tiptapAdapter.ts implements it
 * for the TipTap editor. Rewrite / summarise / translate / fix grammar are
 * done by the model, which then calls doc_replace_selection or doc_insert_text.
 */
import type { AssistantTool, ToolOutcome } from "@/lib/ai";

export type Undo = () => void;
export type DocStyle = "paragraph" | "heading1" | "heading2" | "heading3" | "bold" | "italic" | "bulletList" | "numberedList" | "quote";
export const DOC_STYLES: DocStyle[] = ["paragraph", "heading1", "heading2", "heading3", "bold", "italic", "bulletList", "numberedList", "quote"];
export type InsertAt = "cursor" | "start" | "end" | "after_selection";

export interface DocsAdapter {
  /** Plain text of the selection ("" when nothing is selected). */
  selectionText(): string;
  /** The document as Markdown-ish text with headings. */
  documentText(): string;
  /** Headings in order (for the outline and section reads). */
  headings(): { level: number; text: string }[];
  /** Text of the section under a heading (until the next heading of the same or higher level). */
  sectionText(heading: string): string | null;
  insert(at: InsertAt, markdown: string): Undo;
  /** Replaces the selection; returns an error message when nothing is selected. */
  replaceSelection(markdown: string): Undo | string;
  applyStyle(style: DocStyle): Undo | string;
  insertTable(rows: string[][], header: boolean): Undo;
  addComment?(text: string): Promise<Undo | void> | Undo | void;
}

const MAX_TEXT = 20000;
const fail = (error: string): ToolOutcome => ({ error });
const clip = (s: string) => (s.length > 6000 ? s.slice(0, 6000) + "…[truncated, read a section]" : s);

export function createDocTools(ad: DocsAdapter): AssistantTool[] {
  const tools: AssistantTool[] = [
    {
      name: "doc_read",
      risk: "read",
      description: "Read the open document: the selection, the whole document, or one section by its heading. Also returns the outline.",
      parameters: { type: "object", properties: { scope: { type: "string", enum: ["selection", "document", "section"] }, heading: { type: "string" } }, required: ["scope"] },
      label: (a) => (a.scope === "section" ? `Read section "${String(a.heading ?? "")}"` : `Read the ${String(a.scope)}`),
      handler: (a) => {
        const outline = ad.headings().map((h) => `${"#".repeat(h.level)} ${h.text}`);
        if (a.scope === "selection") return { result: { selection: clip(ad.selectionText()), outline } };
        if (a.scope === "section") {
          const t = ad.sectionText(String(a.heading ?? ""));
          return t === null ? fail("no heading with that text; see outline") : { result: { section: clip(t), outline } };
        }
        return { result: { document: clip(ad.documentText()), outline } };
      },
    },
    {
      name: "doc_insert_text",
      risk: "write",
      description: "Insert new text (Markdown: # headings, - bullets, 1. lists, **bold**, *italic*) at the cursor, the start, the end, or after the selection.",
      parameters: { type: "object", properties: { text: { type: "string" }, at: { type: "string", enum: ["cursor", "start", "end", "after_selection"] } }, required: ["text"] },
      label: (a) => `Insert text at ${String(a.at ?? "cursor")}`,
      handler: (a) => {
        const text = String(a.text);
        if (!text.trim() || text.length > MAX_TEXT) return fail(`text must be 1 to ${MAX_TEXT} characters`);
        const undo = ad.insert((a.at as InsertAt) ?? "cursor", text);
        return { result: { inserted: text.length }, undo, summary: `Inserted ${words(text)} words` };
      },
    },
    {
      name: "doc_replace_selection",
      risk: "write",
      description: "Replace the selected text with new text (Markdown). Use for rewrite, shorten, translate, fix grammar and summarise-in-place.",
      parameters: { type: "object", properties: { text: { type: "string" } }, required: ["text"] },
      label: () => "Replace the selection",
      handler: (a) => {
        const text = String(a.text);
        if (text.length > MAX_TEXT) return fail("text is too long");
        const res = ad.replaceSelection(text);
        return typeof res === "string" ? fail(res) : { result: { replaced: true }, undo: res, summary: `Replaced the selection (${words(text)} words)` };
      },
    },
    {
      name: "doc_apply_style",
      risk: "write",
      description: "Apply a style to the selection or the current paragraph.",
      parameters: { type: "object", properties: { style: { type: "string", enum: DOC_STYLES } }, required: ["style"] },
      label: (a) => `Apply ${String(a.style)}`,
      handler: (a) => {
        const res = ad.applyStyle(a.style as DocStyle);
        return typeof res === "string" ? fail(res) : { result: { applied: a.style }, undo: res, summary: `Applied ${String(a.style)}` };
      },
    },
    {
      name: "doc_insert_table",
      risk: "write",
      description: "Insert a table at the cursor. rows is a list of rows of cell text.",
      parameters: { type: "object", properties: { rows: { type: "array", items: { type: "array", items: { type: "string" } } }, header: { type: "boolean" } }, required: ["rows"] },
      label: (a) => `Insert a ${(a.rows as unknown[]).length}-row table`,
      handler: (a) => {
        const rows = (a.rows as unknown[][]).map((r) => r.map((c) => String(c ?? "")));
        if (!rows.length || rows.length > 100 || rows.some((r) => r.length === 0 || r.length > 20)) return fail("1-100 rows of 1-20 cells");
        const width = Math.max(...rows.map((r) => r.length));
        const norm = rows.map((r) => [...r, ...Array(width - r.length).fill("")]);
        return { result: { rows: norm.length, cols: width }, undo: ad.insertTable(norm, a.header !== false), summary: `Inserted a ${norm.length}×${width} table` };
      },
    },
  ];
  const comment = ad.addComment?.bind(ad);
  if (comment) {
    tools.push({
      name: "doc_add_comment",
      risk: "write",
      description: "Add a comment on the selected text (for review suggestions the user can accept or ignore).",
      parameters: { type: "object", properties: { text: { type: "string" } }, required: ["text"] },
      label: () => "Add a comment",
      handler: async (a) => {
        const text = String(a.text).trim();
        if (!text || text.length > 2000) return fail("comment must be 1-2000 characters");
        if (!ad.selectionText()) return fail("select the text to comment on first");
        const undo = await comment(text);
        return { result: { commented: true }, undo: undo || undefined, summary: "Added a comment" };
      },
    });
  }
  return tools;
}

function words(s: string): number {
  return s.trim().split(/\s+/).filter(Boolean).length;
}

/** Context chip for the panel. */
export function docContext(ad: DocsAdapter, title?: string): { label: string; text?: string } {
  const sel = ad.selectionText();
  if (sel) return { label: `Selection (${words(sel)} words)`, text: sel.slice(0, 8000) };
  return { label: title || "Document", text: ad.documentText().slice(0, 8000) };
}
