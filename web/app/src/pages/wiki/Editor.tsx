import { useEffect, useRef } from "react";
import { Box, Button, IconButton } from "@mui/joy";
import { EditorContent, useEditor, type Editor } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import Link from "@tiptap/extension-link";
import Underline from "@tiptap/extension-underline";
import Highlight from "@tiptap/extension-highlight";
import TaskList from "@tiptap/extension-task-list";
import TaskItem from "@tiptap/extension-task-item";
import { Table } from "@tiptap/extension-table";
import TableRow from "@tiptap/extension-table-row";
import TableCell from "@tiptap/extension-table-cell";
import TableHeader from "@tiptap/extension-table-header";

export interface WikiEditorProps {
  content: unknown;
  editable: boolean;
  onChange: (doc: unknown) => void;
  onReady?: (e: Editor) => void;
  onPickPage?: () => void;
  onAttach?: () => void;
}

const SAFE_LINK = /^(https?:|mailto:|\/)/i;

/** Block editor on the TipTap stack Docs already uses: headings, lists, to-dos, quotes, code, tables, links. */
export function WikiEditor({ content, editable, onChange, onReady, onPickPage, onAttach }: WikiEditorProps) {
  const first = useRef(true);
  const editor = useEditor({
    extensions: [
      StarterKit.configure({ heading: { levels: [1, 2, 3] } }),
      Underline,
      Highlight,
      Link.configure({ openOnClick: false, autolink: true, validate: (href) => SAFE_LINK.test(href) }),
      TaskList,
      TaskItem.configure({ nested: true }),
      Table.configure({ resizable: false }),
      TableRow,
      TableHeader,
      TableCell,
    ],
    content: content as never,
    editable,
    onUpdate: ({ editor: e }) => {
      if (first.current) return;
      onChange(e.getJSON());
    },
    editorProps: { attributes: { class: "wiki-prose", "aria-label": "Page content", role: "textbox", "aria-multiline": "true" } },
  });
  useEffect(() => {
    first.current = false;
    if (editor) onReady?.(editor);
  }, [editor, onReady]);
  useEffect(() => { editor?.setEditable(editable); }, [editor, editable]);
  if (!editor) return null;
  return (
    <Box sx={{
      "& .wiki-prose": { outline: "none", minHeight: 320, fontSize: 16, lineHeight: 1.65, pb: 6 },
      "& .wiki-prose h1": { fontSize: "1.9em", mt: 2, mb: 0.5 }, "& .wiki-prose h2": { fontSize: "1.45em", mt: 1.6, mb: 0.4 }, "& .wiki-prose h3": { fontSize: "1.2em", mt: 1.2, mb: 0.3 },
      "& .wiki-prose p": { my: 0.5 }, "& .wiki-prose ul, & .wiki-prose ol": { pl: 3 },
      "& .wiki-prose ul[data-type=taskList]": { listStyle: "none", pl: 0.5 }, "& .wiki-prose ul[data-type=taskList] li": { display: "flex", gap: 1, alignItems: "flex-start" },
      "& .wiki-prose ul[data-type=taskList] li > label": { mt: "5px" }, "& .wiki-prose ul[data-type=taskList] li > div": { flex: 1 },
      "& .wiki-prose blockquote": { borderLeft: "3px solid", borderColor: "divider", pl: 2, ml: 0, color: "text.secondary" },
      "& .wiki-prose pre": { bgcolor: "background.level1", p: 1.5, borderRadius: "sm", overflow: "auto", fontSize: 13 },
      "& .wiki-prose code": { bgcolor: "background.level2", px: 0.5, borderRadius: "xs", fontSize: "0.9em" },
      "& .wiki-prose table": { borderCollapse: "collapse", width: "100%", my: 1 },
      "& .wiki-prose td, & .wiki-prose th": { border: "1px solid", borderColor: "divider", p: "6px 8px", verticalAlign: "top", minWidth: 60 },
      "& .wiki-prose th": { bgcolor: "background.level1", textAlign: "left" },
      "& .wiki-prose a": { color: "primary.600", textDecoration: "underline" }, "& .wiki-prose hr": { border: 0, borderTop: "1px solid", borderColor: "divider", my: 2 },
      "& .wiki-prose mark": { bgcolor: "#fff3a3" },
    }}>
      {editable && <Toolbar editor={editor} onPickPage={onPickPage} onAttach={onAttach} />}
      <EditorContent editor={editor} />
    </Box>
  );
}

function Toolbar({ editor, onPickPage, onAttach }: { editor: Editor; onPickPage?: () => void; onAttach?: () => void }) {
  const B = ({ label, on, active, children, disabled }: { label: string; on: () => void; active?: boolean; children: React.ReactNode; disabled?: boolean }) => (
    <IconButton size="sm" variant={active ? "soft" : "plain"} color={active ? "primary" : "neutral"} aria-label={label} title={label} aria-pressed={!!active} disabled={disabled} onMouseDown={(e) => e.preventDefault()} onClick={on} sx={{ minWidth: 30 }}>{children}</IconButton>
  );
  const link = () => {
    const prev = editor.getAttributes("link").href as string | undefined;
    const url = window.prompt("Link address (https://…)", prev ?? "https://");
    if (url === null) return;
    if (url === "") { editor.chain().focus().unsetLink().run(); return; }
    if (!SAFE_LINK.test(url)) { window.alert("Only http, https, mailto and in-app links are allowed."); return; }
    editor.chain().focus().extendMarkRange("link").setLink({ href: url }).run();
  };
  const c = () => editor.chain().focus();
  return (
    <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.3, py: 0.5, mb: 1, borderBottom: "1px solid", borderColor: "divider", position: "sticky", top: 0, bgcolor: "background.surface", zIndex: 5 }} role="toolbar" aria-label="Formatting" data-testid="wiki-toolbar">
      <B label="Heading 1" on={() => c().toggleHeading({ level: 1 }).run()} active={editor.isActive("heading", { level: 1 })}>H1</B>
      <B label="Heading 2" on={() => c().toggleHeading({ level: 2 }).run()} active={editor.isActive("heading", { level: 2 })}>H2</B>
      <B label="Heading 3" on={() => c().toggleHeading({ level: 3 }).run()} active={editor.isActive("heading", { level: 3 })}>H3</B>
      <B label="Bold" on={() => c().toggleBold().run()} active={editor.isActive("bold")}><b>B</b></B>
      <B label="Italic" on={() => c().toggleItalic().run()} active={editor.isActive("italic")}><i>I</i></B>
      <B label="Underline" on={() => c().toggleUnderline().run()} active={editor.isActive("underline")}><u>U</u></B>
      <B label="Strikethrough" on={() => c().toggleStrike().run()} active={editor.isActive("strike")}><s>S</s></B>
      <B label="Highlight" on={() => c().toggleHighlight().run()} active={editor.isActive("highlight")}>▮</B>
      <B label="Inline code" on={() => c().toggleCode().run()} active={editor.isActive("code")}>{"</>"}</B>
      <B label="Link" on={link} active={editor.isActive("link")}>🔗</B>
      <B label="Bulleted list" on={() => c().toggleBulletList().run()} active={editor.isActive("bulletList")}>•≡</B>
      <B label="Numbered list" on={() => c().toggleOrderedList().run()} active={editor.isActive("orderedList")}>1.</B>
      <B label="To-do list" on={() => c().toggleTaskList().run()} active={editor.isActive("taskList")}>☑</B>
      <B label="Quote" on={() => c().toggleBlockquote().run()} active={editor.isActive("blockquote")}>❝</B>
      <B label="Code block" on={() => c().toggleCodeBlock().run()} active={editor.isActive("codeBlock")}>{"{}"}</B>
      <B label="Divider" on={() => c().setHorizontalRule().run()}>―</B>
      <B label="Insert table" on={() => c().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run()}>▦</B>
      {editor.isActive("table") && (
        <>
          <B label="Add row below" on={() => c().addRowAfter().run()}>＋row</B>
          <B label="Add column after" on={() => c().addColumnAfter().run()}>＋col</B>
          <B label="Delete row" on={() => c().deleteRow().run()}>−row</B>
          <B label="Delete column" on={() => c().deleteColumn().run()}>−col</B>
          <B label="Delete table" on={() => c().deleteTable().run()}>🗑</B>
        </>
      )}
      {onPickPage && <Button size="sm" variant="plain" color="neutral" onMouseDown={(e) => e.preventDefault()} onClick={onPickPage}>@ Page</Button>}
      {onAttach && <Button size="sm" variant="plain" color="neutral" onMouseDown={(e) => e.preventDefault()} onClick={onAttach}>📎 Drive file</Button>}
      <B label="Undo" on={() => c().undo().run()} disabled={!editor.can().undo()}>↶</B>
      <B label="Redo" on={() => c().redo().run()} disabled={!editor.can().redo()}>↷</B>
    </Box>
  );
}
