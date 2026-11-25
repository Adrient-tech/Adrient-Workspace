import { forwardRef, useEffect, useImperativeHandle, useRef, useState } from "react";
import { Avatar, Box, IconButton, Sheet, Tooltip, Typography } from "@mui/joy";
import { EditorContent, useEditor, type Editor } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import Link from "@tiptap/extension-link";
import { Node, mergeAttributes } from "@tiptap/core";
import FormatBoldIcon from "@mui/icons-material/FormatBold";
import FormatItalicIcon from "@mui/icons-material/FormatItalic";
import CodeIcon from "@mui/icons-material/Code";
import FormatListBulletedIcon from "@mui/icons-material/FormatListBulleted";
import FormatListNumberedIcon from "@mui/icons-material/FormatListNumbered";
import FormatQuoteIcon from "@mui/icons-material/FormatQuote";
import LinkIcon from "@mui/icons-material/Link";
import UndoIcon from "@mui/icons-material/Undo";
import RedoIcon from "@mui/icons-material/Redo";
import AlternateEmailIcon from "@mui/icons-material/AlternateEmail";
import { peopleApi, type Person } from "../../api/people";
import { docToMarkup, markupToHtml, safeHref, type DocNode } from "./workspaceLogic";

/** An inline @mention of a teammate. Stored as @[Name](user:id) in the message. */
const TicketMention = Node.create({
  name: "ticketMention",
  group: "inline",
  inline: true,
  atom: true,
  addAttributes() {
    return {
      id: { default: "", parseHTML: (el) => (el as HTMLElement).getAttribute("data-id") ?? "" },
      label: { default: "", parseHTML: (el) => (el as HTMLElement).getAttribute("data-label") ?? "" },
    };
  },
  parseHTML() {
    return [{ tag: "span[data-ticket-mention]" }];
  },
  renderHTML({ node, HTMLAttributes }) {
    return ["span", mergeAttributes(HTMLAttributes, { "data-ticket-mention": "", "data-id": node.attrs.id, "data-label": node.attrs.label, class: "ticket-mention" }), `@${node.attrs.label}`];
  },
});

export interface ReplyEditorHandle {
  focus: () => void;
  insertText: (text: string) => void;
  clear: () => void;
}

interface Props {
  /** Message in composer markup. Changes from outside (a loaded draft, a suggestion) replace the content. */
  value: string;
  onChange: (markup: string) => void;
  placeholder?: string;
  tone?: "neutral" | "warning";
  onSubmit?: () => void;
  footer?: React.ReactNode;
}

/** Rich editor for ticket replies and notes. Output stays in the light markup the desk renders safely. */
export const ReplyEditor = forwardRef<ReplyEditorHandle, Props>(function ReplyEditor({ value, onChange, placeholder, tone = "neutral", onSubmit, footer }, ref) {
  const last = useRef(value);
  const submit = useRef(onSubmit);
  submit.current = onSubmit;
  const [mention, setMention] = useState<{ query: string; from: number; to: number } | null>(null);
  const [hits, setHits] = useState<Person[]>([]);
  const editor = useEditor({
    extensions: [
      StarterKit.configure({ heading: false, codeBlock: false, horizontalRule: false, strike: false }),
      Link.configure({ openOnClick: false, autolink: true, validate: (h) => !!safeHref(h) }),
      TicketMention,
    ],
    content: markupToHtml(value),
    editorProps: {
      attributes: { "data-testid": "composer", role: "textbox", "aria-multiline": "true", "aria-label": "Message", class: "reply-editor-content" },
      handleKeyDown: (_v, e) => {
        if ((e.ctrlKey || e.metaKey) && e.key === "Enter") {
          e.preventDefault();
          submit.current?.();
          return true;
        }
        return false;
      },
    },
    onUpdate: ({ editor: ed }) => {
      const md = ed.isEmpty ? "" : docToMarkup(ed.getJSON() as DocNode);
      last.current = md;
      onChange(md);
      const { from } = ed.state.selection;
      const before = ed.state.doc.textBetween(Math.max(0, from - 40), from, "\n", "\0");
      const m = /(?:^|\s)@([\p{L}\p{N}._-]{0,30})$/u.exec(before);
      setMention(m ? { query: m[1], from: from - m[1].length - 1, to: from } : null);
    },
  });

  useEffect(() => {
    if (!editor || value === last.current) return;
    last.current = value;
    editor.commands.setContent(markupToHtml(value), false);
  }, [value, editor]);

  useEffect(() => {
    if (!mention) return setHits([]);
    const t = setTimeout(() => peopleApi.search(mention.query, 6).then(setHits).catch(() => setHits([])), 150);
    return () => clearTimeout(t);
  }, [mention]);

  useImperativeHandle(ref, () => ({
    focus: () => editor?.commands.focus("end"),
    insertText: (text: string) => editor?.chain().focus("end").insertContent(markupToHtml(text)).run(),
    clear: () => {
      last.current = "";
      editor?.commands.clearContent(true);
    },
  }), [editor]);

  function pick(p: Person) {
    if (!editor || !mention) return;
    editor.chain().focus().deleteRange({ from: mention.from, to: mention.to }).insertContent([{ type: "ticketMention", attrs: { id: p.id, label: p.name } }, { type: "text", text: " " }]).run();
    setMention(null);
  }
  function setLink(ed: Editor) {
    const prev = ed.getAttributes("link").href as string | undefined;
    const url = window.prompt("Link address (https://…)", prev ?? "https://");
    if (url === null) return;
    if (url.trim() === "") return void ed.chain().focus().unsetLink().run();
    const href = safeHref(url);
    if (!href) return void window.alert("Only http, https, mailto and in-app links are allowed.");
    ed.chain().focus().extendMarkRange("link").setLink({ href }).run();
  }

  const btn = (label: string, icon: React.ReactNode, active: boolean, run: () => void, disabled = false) => (
    <Tooltip key={label} title={label}>
      <span>
        <IconButton size="sm" variant={active ? "soft" : "plain"} color={active ? "primary" : "neutral"} disabled={disabled} aria-label={label} aria-pressed={active} onMouseDown={(e) => e.preventDefault()} onClick={run}>{icon}</IconButton>
      </span>
    </Tooltip>
  );
  return (
    <Sheet variant="outlined" color={tone} sx={{ borderRadius: "md", position: "relative", "&:focus-within": { borderColor: tone === "warning" ? "warning.500" : "primary.500" } }}>
      {editor && (
        <Box sx={{ display: "flex", alignItems: "center", gap: 0.25, px: 0.75, py: 0.4, borderBottom: "1px solid", borderColor: "divider", flexWrap: "wrap" }} data-testid="editor-toolbar">
          {btn("Bold", <FormatBoldIcon fontSize="small" />, editor.isActive("bold"), () => editor.chain().focus().toggleBold().run())}
          {btn("Italic", <FormatItalicIcon fontSize="small" />, editor.isActive("italic"), () => editor.chain().focus().toggleItalic().run())}
          {btn("Code", <CodeIcon fontSize="small" />, editor.isActive("code"), () => editor.chain().focus().toggleCode().run())}
          <Box sx={{ width: 1, height: 18, bgcolor: "divider", mx: 0.5 }} />
          {btn("Bulleted list", <FormatListBulletedIcon fontSize="small" />, editor.isActive("bulletList"), () => editor.chain().focus().toggleBulletList().run())}
          {btn("Numbered list", <FormatListNumberedIcon fontSize="small" />, editor.isActive("orderedList"), () => editor.chain().focus().toggleOrderedList().run())}
          {btn("Quote", <FormatQuoteIcon fontSize="small" />, editor.isActive("blockquote"), () => editor.chain().focus().toggleBlockquote().run())}
          {btn("Link", <LinkIcon fontSize="small" />, editor.isActive("link"), () => setLink(editor))}
          {btn("Mention a teammate", <AlternateEmailIcon fontSize="small" />, false, () => editor.chain().focus().insertContent("@").run())}
          <Box sx={{ width: 1, height: 18, bgcolor: "divider", mx: 0.5 }} />
          {btn("Undo", <UndoIcon fontSize="small" />, false, () => editor.chain().focus().undo().run(), !editor.can().undo())}
          {btn("Redo", <RedoIcon fontSize="small" />, false, () => editor.chain().focus().redo().run(), !editor.can().redo())}
        </Box>
      )}
      <Box
        sx={{
          position: "relative", px: 1.5, py: 1, minHeight: 84, maxHeight: 260, overflow: "auto", fontSize: "sm", lineHeight: 1.55,
          "& .reply-editor-content": { outline: "none", minHeight: 64 },
          "& p": { m: 0, mb: 0.75 },
          "& ul, & ol": { my: 0.5, pl: 3 },
          "& blockquote": { m: 0, my: 0.5, pl: 1.5, borderLeft: "3px solid", borderColor: "neutral.300", color: "text.secondary" },
          "& code": { bgcolor: "neutral.softBg", px: 0.5, borderRadius: 4, fontFamily: "monospace" },
          "& a": { color: "primary.600", textDecoration: "underline" },
          "& .ticket-mention": { bgcolor: "primary.softBg", color: "primary.700", borderRadius: 6, px: 0.5 },
        }}
      >
        {editor?.isEmpty && <Typography level="body-sm" sx={{ position: "absolute", color: "text.tertiary", pointerEvents: "none" }}>{placeholder ?? "Write a reply"}</Typography>}
        <EditorContent editor={editor} />
      </Box>
      {footer && <Box sx={{ px: 1, py: 0.5, borderTop: "1px solid", borderColor: "divider" }}>{footer}</Box>}
      {mention && hits.length > 0 && (
        <Sheet variant="outlined" sx={{ position: "absolute", bottom: "100%", left: 8, zIndex: 20, borderRadius: "md", boxShadow: "md", minWidth: 220, mb: 0.5 }} data-testid="mention-list">
          {hits.map((p) => (
            <Box key={p.id} onMouseDown={(e) => { e.preventDefault(); pick(p); }} sx={{ px: 1.25, py: 0.75, cursor: "pointer", "&:hover": { bgcolor: "neutral.softHoverBg" }, display: "flex", gap: 1, alignItems: "center" }}>
              <Avatar size="sm">{p.name.slice(0, 1)}</Avatar><Typography level="body-sm">{p.name}</Typography>
            </Box>
          ))}
        </Sheet>
      )}
    </Sheet>
  );
});
