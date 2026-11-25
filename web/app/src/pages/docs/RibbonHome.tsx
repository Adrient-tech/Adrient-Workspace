// Ribbon ▸ Home tab.
import { Box, ListDivider, MenuItem, Option, Select, Input } from "@mui/joy";
import ContentPasteIcon from "@mui/icons-material/ContentPaste";
import ContentCutIcon from "@mui/icons-material/ContentCut";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import FormatBoldIcon from "@mui/icons-material/FormatBold";
import FormatItalicIcon from "@mui/icons-material/FormatItalic";
import FormatUnderlinedIcon from "@mui/icons-material/FormatUnderlined";
import StrikethroughSIcon from "@mui/icons-material/StrikethroughS";
import SubscriptIcon from "@mui/icons-material/Subscript";
import SuperscriptIcon from "@mui/icons-material/Superscript";
import FormatColorTextIcon from "@mui/icons-material/FormatColorText";
import BorderColorIcon from "@mui/icons-material/BorderColor";
import TextFieldsIcon from "@mui/icons-material/TextFields";
import FormatClearIcon from "@mui/icons-material/FormatClear";
import FormatListBulletedIcon from "@mui/icons-material/FormatListBulleted";
import FormatListNumberedIcon from "@mui/icons-material/FormatListNumbered";
import ChecklistIcon from "@mui/icons-material/Checklist";
import FormatIndentDecreaseIcon from "@mui/icons-material/FormatIndentDecrease";
import FormatIndentIncreaseIcon from "@mui/icons-material/FormatIndentIncrease";
import FormatAlignLeftIcon from "@mui/icons-material/FormatAlignLeft";
import FormatAlignCenterIcon from "@mui/icons-material/FormatAlignCenter";
import FormatAlignRightIcon from "@mui/icons-material/FormatAlignRight";
import FormatAlignJustifyIcon from "@mui/icons-material/FormatAlignJustify";
import FormatLineSpacingIcon from "@mui/icons-material/FormatLineSpacing";
import NotesIcon from "@mui/icons-material/Notes";
import SearchIcon from "@mui/icons-material/Search";
import FindReplaceIcon from "@mui/icons-material/FindReplace";
import UndoIcon from "@mui/icons-material/Undo";
import RedoIcon from "@mui/icons-material/Redo";
import AddIcon from "@mui/icons-material/Add";
import RemoveIcon from "@mui/icons-material/Remove";
import { PICKER_FONTS, fontStack } from "../../lib/fonts";
import { Group, RBtn, RMenu } from "./RibbonKit";
import { StyleGallery } from "./StyleGallery";
import { ListMenu } from "./ListMenu";
import { HIGHLIGHTS, LINE_SPACINGS, TEXT_COLORS } from "./Toolbar";
import { copySelection, cutSelection, paste } from "./editorActions";
import { CASE_MODES, changeCase } from "./textCase";
import { openParagraphDialog } from "./ParagraphDialogs";
import { isNonPrinting } from "./viewModes";
import { swatch, type TabProps } from "./ribbonShared";

export function HomeTab({ editor: ed, actions }: TabProps) {
  const fontValue = (ed.getAttributes("textStyle").fontFamily as string) || "Arial";
  const curSize = parseInt((ed.getAttributes("textStyle").fontSize as string) || "11", 10) || 11;
  const setSize = (n: number) => ed.chain().focus().setFontSize(`${Math.max(1, Math.min(96, n))}pt`).run();
  const al = (a: string) => ed.isActive({ textAlign: a });
  const row = { display: "flex", alignItems: "center", gap: 0.25 } as const;
  return (
    <>
      <Group label="Undo">
        <RBtn icon={<UndoIcon />} label="Undo" onClick={() => ed.chain().focus().undo().run()} testid="rb-undo" />
        <RBtn icon={<RedoIcon />} label="Redo" onClick={() => ed.chain().focus().redo().run()} testid="rb-redo" />
      </Group>
      <Group label="Clipboard">
        <RMenu icon={<ContentPasteIcon />} label="Paste" big>
          <MenuItem onClick={() => void paste(ed, false)}>Paste</MenuItem>
          <MenuItem onClick={() => void paste(ed, true)}>Paste as plain text</MenuItem>
        </RMenu>
        <RBtn icon={<ContentCutIcon />} label="Cut" onClick={cutSelection} />
        <RBtn icon={<ContentCopyIcon />} label="Copy" onClick={copySelection} />
      </Group>
      <Group label="Font">
        <Box sx={{ display: "flex", flexDirection: "column", gap: 0.25 }}>
          <Box sx={row}>
            <Select size="sm" value={fontValue} variant="outlined" sx={{ minWidth: 130 }} aria-label="Font" data-testid="rb-font" onChange={(_, v) => v && ed.chain().focus().setFontFamily(v).run()}>
              {[...new Set([...PICKER_FONTS, fontValue])].map((f) => (
                <Option key={f} value={f} sx={{ fontFamily: fontStack(f) }}>{f}</Option>
              ))}
            </Select>
            <RBtn icon={<RemoveIcon />} label="Decrease font size" onClick={() => setSize(curSize - 1)} />
            <Input size="sm" value={String(curSize)} variant="outlined" sx={{ width: 46, "--Input-minHeight": "30px" }} slotProps={{ input: { "aria-label": "Font size", style: { textAlign: "center" } } }} onChange={(e) => { const n = parseInt(e.target.value, 10); if (!Number.isNaN(n)) setSize(n); }} />
            <RBtn icon={<AddIcon />} label="Increase font size" onClick={() => setSize(curSize + 1)} />
          </Box>
          <Box sx={row}>
            <RBtn icon={<FormatBoldIcon />} label="Bold" active={ed.isActive("bold")} onClick={() => ed.chain().focus().toggleBold().run()} testid="rb-bold" />
            <RBtn icon={<FormatItalicIcon />} label="Italic" active={ed.isActive("italic")} onClick={() => ed.chain().focus().toggleItalic().run()} />
            <RBtn icon={<FormatUnderlinedIcon />} label="Underline" active={ed.isActive("underline")} onClick={() => ed.chain().focus().toggleUnderline().run()} />
            <RBtn icon={<StrikethroughSIcon />} label="Strikethrough" active={ed.isActive("strike")} onClick={() => ed.chain().focus().toggleStrike().run()} />
            <RBtn icon={<SubscriptIcon />} label="Subscript" active={ed.isActive("subscript")} onClick={() => ed.chain().focus().toggleSubscript().run()} />
            <RBtn icon={<SuperscriptIcon />} label="Superscript" active={ed.isActive("superscript")} onClick={() => ed.chain().focus().toggleSuperscript().run()} />
            <RMenu icon={<FormatColorTextIcon />} label="Text color">
              <Box sx={{ display: "grid", gridTemplateColumns: "repeat(7, 18px)", gap: 0.5, p: 1 }}>
                {TEXT_COLORS.map((c) => swatch(c, () => ed.chain().focus().setColor(c).run()))}
              </Box>
            </RMenu>
            <RMenu icon={<BorderColorIcon />} label="Highlight color">
              <Box sx={{ display: "grid", gridTemplateColumns: "repeat(5, 18px)", gap: 0.5, p: 1 }}>
                {HIGHLIGHTS.map((c) => swatch(c, () => ed.chain().focus().toggleHighlight({ color: c }).run()))}
              </Box>
            </RMenu>
            <RMenu icon={<TextFieldsIcon />} label="Change case">
              {CASE_MODES.map(({ mode: m, label }) => (
                <MenuItem key={m} onClick={() => changeCase(ed, m)}>{label}</MenuItem>
              ))}
            </RMenu>
            <RBtn icon={<FormatClearIcon />} label="Clear formatting" onClick={() => ed.chain().focus().clearNodes().unsetAllMarks().run()} />
          </Box>
        </Box>
      </Group>
      <Group label="Paragraph">
        <Box sx={{ display: "flex", flexDirection: "column", gap: 0.25 }}>
          <Box sx={row}>
            <RBtn icon={<FormatListBulletedIcon />} label="Bulleted list" active={ed.isActive("bulletList")} onClick={() => ed.chain().focus().toggleBulletList().run()} />
            <RBtn icon={<FormatListNumberedIcon />} label="Numbered list" active={ed.isActive("orderedList")} onClick={() => ed.chain().focus().toggleOrderedList().run()} />
            <RBtn icon={<ChecklistIcon />} label="Checklist" active={ed.isActive("taskList")} onClick={() => ed.chain().focus().toggleTaskList().run()} />
            <ListMenu editor={ed} />
            <RBtn icon={<FormatIndentDecreaseIcon />} label="Decrease indent" onClick={() => ed.chain().focus().liftListItem("listItem").run()} />
            <RBtn icon={<FormatIndentIncreaseIcon />} label="Increase indent" onClick={() => ed.chain().focus().sinkListItem("listItem").run()} />
          </Box>
          <Box sx={row}>
            <RBtn icon={<FormatAlignLeftIcon />} label="Align left" active={al("left")} onClick={() => ed.chain().focus().setTextAlign("left").run()} />
            <RBtn icon={<FormatAlignCenterIcon />} label="Align center" active={al("center")} onClick={() => ed.chain().focus().setTextAlign("center").run()} />
            <RBtn icon={<FormatAlignRightIcon />} label="Align right" active={al("right")} onClick={() => ed.chain().focus().setTextAlign("right").run()} />
            <RBtn icon={<FormatAlignJustifyIcon />} label="Justify" active={al("justify")} onClick={() => ed.chain().focus().setTextAlign("justify").run()} />
            <RMenu icon={<FormatLineSpacingIcon />} label="Line and paragraph spacing">
              {LINE_SPACINGS.map((s) => (
                <MenuItem key={s} onClick={() => ed.chain().focus().setLineHeight(s).run()}>{s}</MenuItem>
              ))}
              <MenuItem onClick={() => ed.chain().focus().unsetLineHeight().run()}>Reset</MenuItem>
              <ListDivider />
              <MenuItem onClick={actions.customSpacing}>Custom spacing…</MenuItem>
              <MenuItem onClick={() => openParagraphDialog("paragraph")}>Paragraph options…</MenuItem>
            </RMenu>
            <RBtn icon={<NotesIcon />} label="Show non-printing characters" active={isNonPrinting(ed)} onClick={() => ed.chain().focus().toggleNonPrinting().run()} />
          </Box>
        </Box>
      </Group>
      <Group label="Styles">
        <Box sx={{ display: "flex", alignItems: "center", height: "100%" }}>
          <StyleGallery editor={ed} />
        </Box>
      </Group>
      <Group label="Editing">
        <RBtn icon={<SearchIcon />} label="Find" big onClick={() => (actions.find ?? actions.findReplace)()} testid="rb-find" />
        <RBtn icon={<FindReplaceIcon />} label="Replace" big onClick={actions.findReplace} testid="rb-replace" />
      </Group>
    </>
  );
}
