import { Editor } from "@tiptap/core";
import StarterKit from "@tiptap/starter-kit";
import Table from "@tiptap/extension-table";
import TableRow from "@tiptap/extension-table-row";
import TableCell from "@tiptap/extension-table-cell";
import TableHeader from "@tiptap/extension-table-header";
import { afterEach, describe, expect, it } from "vitest";
import { markdownToHtml } from "./markdown";
import { tiptapAdapter } from "./tiptapAdapter";
import { createDocTools, docContext } from "./tools";

let editor: Editor | null = null;
afterEach(() => {
  editor?.destroy();
  editor = null;
});

function make(html: string) {
  editor = new Editor({ extensions: [StarterKit, Table, TableRow, TableCell, TableHeader], content: html });
  const ad = tiptapAdapter(() => editor);
  const tools = createDocTools(ad);
  const run = async (name: string, args: Record<string, unknown>) => tools.find((t) => t.name === name)!.handler(args);
  return { ad, tools, run, ed: editor };
}

describe("markdownToHtml", () => {
  it("converts blocks and escapes HTML", () => {
    expect(markdownToHtml("# Title\n\n- a\n- **b**\n\n1. one\n\nText <script>x</script>")).toBe(
      "<h1>Title</h1><ul><li><p>a</p></li><li><p><strong>b</strong></p></li></ul><ol><li><p>one</p></li></ol><p>Text &lt;script&gt;x&lt;/script&gt;</p>",
    );
  });
});

describe("doc tools on TipTap", () => {
  it("reads the document, outline and a section", async () => {
    const { run } = make("<h1>Plan</h1><p>Intro</p><h2>Budget</h2><p>Money</p><h2>Team</h2><p>People</p>");
    const doc = await run("doc_read", { scope: "document" });
    expect(doc.result).toMatchObject({ outline: ["# Plan", "## Budget", "## Team"] });
    expect(String((doc.result as { document: string }).document)).toContain("## Budget");
    expect((await run("doc_read", { scope: "section", heading: "budget" })).result).toMatchObject({ section: "Budget\n\nMoney" });
    expect((await run("doc_read", { scope: "section", heading: "nope" })).error).toBeTruthy();
  });

  it("replaces the selection and undoes only its own change after other edits", async () => {
    const { run, ed } = make("<p>Hello wrold, nice day.</p><p>Second</p>");
    ed.commands.setTextSelection({ from: 1, to: 13 });
    expect(docContext(tiptapAdapter(() => ed)).label).toBe("Selection (2 words)");
    const out = await run("doc_replace_selection", { text: "Hello world," });
    expect(ed.getText()).toContain("Hello world, nice day.");
    // A later edit elsewhere (the user typing at the end).
    ed.commands.insertContentAt(ed.state.doc.content.size - 1, " more");
    out.undo!();
    expect(ed.getText()).toContain("Hello wrold, nice day.");
    expect(ed.getText()).toContain("Second more");
  });

  it("refuses to replace with nothing selected", async () => {
    const { run } = make("<p>x</p>");
    expect((await run("doc_replace_selection", { text: "y" })).error).toContain("Nothing is selected");
  });

  it("inserts markdown at the end, a table, and a style, each undoable", async () => {
    const { run, ed } = make("<p>Body</p>");
    const before = ed.getHTML();
    const ins = await run("doc_insert_text", { text: "## Summary\n- one\n- two", at: "end" });
    expect(ed.getHTML()).toContain("<h2>Summary</h2><ul><li><p>one</p></li>");
    const tbl = await run("doc_insert_table", { rows: [["Item", "Cost"], ["Rent", "100"]], header: true });
    expect(ed.getHTML()).toContain("<th");
    tbl.undo!();
    ins.undo!();
    expect(ed.getHTML()).toBe(before);
    ed.commands.setTextSelection({ from: 1, to: 5 });
    const st = await run("doc_apply_style", { style: "heading1" });
    expect(ed.getHTML()).toBe("<h1>Body</h1>");
    st.undo!();
    expect(ed.getHTML()).toBe("<p>Body</p>");
  });
});
