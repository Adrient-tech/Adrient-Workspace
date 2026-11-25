/**
 * DocsAdapter over a TipTap editor. Every change is one transaction; its
 * undo applies the inverted steps mapped through everything that happened
 * since (other edits, collaborators), so it reverts only the assistant's
 * change. If the text it changed was edited again, undo reports an error.
 */
import type { Editor } from "@tiptap/core";
import { Mapping, type Step } from "@tiptap/pm/transform";
import type { Transaction } from "@tiptap/pm/state";
import { isInlineText, markdownToHtml } from "./markdown";
import type { DocStyle, DocsAdapter, InsertAt, Undo } from "./tools";

/** Runs fn as one tracked change and returns its undo. */
function tracked(editor: Editor, fn: () => void): Undo {
  const steps: Step[] = [];
  const mapping = new Mapping();
  let recording = true;
  const onTx = ({ transaction }: { transaction: Transaction }) => {
    if (!transaction.docChanged) return;
    if (recording) {
      transaction.steps.forEach((s, i) => steps.push(s.invert(transaction.docs[i])));
    } else {
      mapping.appendMapping(transaction.mapping);
    }
  };
  editor.on("transaction", onTx);
  try {
    fn();
  } finally {
    recording = false;
  }
  let done = false;
  return () => {
    if (done) return;
    done = true;
    editor.off("transaction", onTx);
    const tr = editor.state.tr;
    // Inverses were recorded in order; undo newest first, mapped through later edits.
    for (let i = steps.length - 1; i >= 0; i--) {
      const mapped = steps[i].map(mapping);
      if (!mapped) throw new Error("That text was changed again, so the assistant's edit cannot be undone.");
      const res = tr.maybeStep(mapped);
      if (res.failed) throw new Error("The document changed there, so the assistant's edit cannot be undone.");
    }
    editor.view.dispatch(tr.setMeta("addToHistory", true));
  };
}

function textBetween(editor: Editor, from: number, to: number): string {
  return editor.state.doc.textBetween(from, to, "\n\n", " ");
}

export function tiptapAdapter(getEditor: () => Editor | null): DocsAdapter {
  const ed = () => {
    const e = getEditor();
    if (!e || e.isDestroyed) throw new Error("The document is not ready yet.");
    return e;
  };
  const insertMd = (e: Editor, pos: number | null, md: string) => {
    const content = isInlineText(md) ? md.trim().replace(/[<>&]/g, (c) => ({ "<": "&lt;", ">": "&gt;", "&": "&amp;" })[c]!) : markdownToHtml(md);
    const chain = e.chain();
    (pos === null ? chain.insertContent(content) : chain.insertContentAt(pos, content)).run();
  };
  return {
    selectionText: () => {
      const e = ed();
      const { from, to } = e.state.selection;
      return from === to ? "" : textBetween(e, from, to);
    },
    documentText: () => {
      const e = ed();
      const parts: string[] = [];
      e.state.doc.forEach((node) => {
        const t = node.textContent;
        if (node.type.name === "heading") parts.push(`${"#".repeat(Number(node.attrs.level) || 1)} ${t}`);
        else if (t) parts.push(node.type.name === "bulletList" || node.type.name === "orderedList" ? node.content.content.map((li) => `- ${li.textContent}`).join("\n") : t);
      });
      return parts.join("\n\n");
    },
    headings: () => {
      const out: { level: number; text: string }[] = [];
      ed().state.doc.forEach((n) => {
        if (n.type.name === "heading") out.push({ level: Number(n.attrs.level) || 1, text: n.textContent });
      });
      return out;
    },
    sectionText: (heading) => {
      const e = ed();
      const want = heading.trim().toLowerCase();
      let level = 0;
      let found = false;
      const parts: string[] = [];
      let stop = false;
      e.state.doc.forEach((n) => {
        if (stop) return;
        const isH = n.type.name === "heading";
        const lv = Number(n.attrs?.level) || 1;
        if (!found) {
          if (isH && n.textContent.trim().toLowerCase() === want) {
            found = true;
            level = lv;
            parts.push(n.textContent);
          }
          return;
        }
        if (isH && lv <= level) {
          stop = true;
          return;
        }
        parts.push(n.textContent);
      });
      return found ? parts.join("\n\n") : null;
    },
    insert: (at: InsertAt, md) => {
      const e = ed();
      return tracked(e, () => {
        const { to } = e.state.selection;
        const pos = at === "start" ? 0 : at === "end" ? e.state.doc.content.size : at === "after_selection" ? to : null;
        insertMd(e, pos, md);
      });
    },
    replaceSelection: (md) => {
      const e = ed();
      const { from, to } = e.state.selection;
      if (from === to) return "Nothing is selected. Ask the user to select the text first, or use doc_insert_text.";
      return tracked(e, () => {
        e.chain().deleteRange({ from, to }).run();
        insertMd(e, from, md);
      });
    },
    applyStyle: (style: DocStyle) => {
      const e = ed();
      const map: Record<DocStyle, () => boolean> = {
        paragraph: () => e.chain().focus().setParagraph().run(),
        heading1: () => e.chain().focus().setHeading({ level: 1 }).run(),
        heading2: () => e.chain().focus().setHeading({ level: 2 }).run(),
        heading3: () => e.chain().focus().setHeading({ level: 3 }).run(),
        bold: () => e.chain().focus().setBold().run(),
        italic: () => e.chain().focus().setItalic().run(),
        bulletList: () => e.chain().focus().toggleBulletList().run(),
        numberedList: () => e.chain().focus().toggleOrderedList().run(),
        quote: () => e.chain().focus().toggleBlockquote().run(),
      };
      let ok = false;
      const undo = tracked(e, () => {
        ok = map[style]();
      });
      return ok ? undo : `The style ${style} cannot be applied here.`;
    },
    insertTable: (rows, header) => {
      const e = ed();
      const cell = (t: string, h: boolean) => `<${h ? "th" : "td"}><p>${t.replace(/[<>&]/g, (c) => ({ "<": "&lt;", ">": "&gt;", "&": "&amp;" })[c]!)}</p></${h ? "th" : "td"}>`;
      const html = `<table><tbody>${rows.map((r, i) => `<tr>${r.map((c) => cell(c, header && i === 0)).join("")}</tr>`).join("")}</tbody></table><p></p>`;
      return tracked(e, () => {
        e.chain().focus().insertContent(html).run();
      });
    },
  };
}
