// Ribbon ▸ Insert and References tabs.
import { useState } from "react";
import { MenuItem } from "@mui/joy";
import ImageIcon from "@mui/icons-material/Image";
import CategoryIcon from "@mui/icons-material/Category";
import CropSquareIcon from "@mui/icons-material/CropSquare";
import GridOnIcon from "@mui/icons-material/GridOn";
import InsertChartIcon from "@mui/icons-material/InsertChart";
import FunctionsIcon from "@mui/icons-material/Functions";
import BrushIcon from "@mui/icons-material/Brush";
import LinkIcon from "@mui/icons-material/Link";
import BookmarkIcon from "@mui/icons-material/BookmarkBorder";
import CompareArrowsIcon from "@mui/icons-material/CompareArrows";
import VerticalAlignTopIcon from "@mui/icons-material/VerticalAlignTop";
import NumbersIcon from "@mui/icons-material/Numbers";
import InsertPageBreakIcon from "@mui/icons-material/InsertPageBreak";
import DashboardIcon from "@mui/icons-material/Dashboard";
import AlternateEmailIcon from "@mui/icons-material/AlternateEmail";
import TranslateIcon from "@mui/icons-material/Translate";
import EventIcon from "@mui/icons-material/Event";
import AddCommentIcon from "@mui/icons-material/AddComment";
import AppsIcon from "@mui/icons-material/Apps";
import ContactsIcon from "@mui/icons-material/Contacts";
import VideocamIcon from "@mui/icons-material/Videocam";
import FileOpenIcon from "@mui/icons-material/FileOpen";
import TextFieldsIcon from "@mui/icons-material/TextFields";
import TocIcon from "@mui/icons-material/Toc";
import UpdateIcon from "@mui/icons-material/Update";
import NoteIcon from "@mui/icons-material/StickyNote2";
import LibraryBooksIcon from "@mui/icons-material/LibraryBooks";
import SubtitlesIcon from "@mui/icons-material/Subtitles";
import CodeIcon from "@mui/icons-material/Code";
import ViewListIcon from "@mui/icons-material/ViewList";
import { Group, RBtn, RMenu } from "./RibbonKit";
import { promptLink } from "./links";
import { TableSizePicker, insertPickedTable, openConvertTextDialog } from "./TableUI";
import { openInsertObject } from "./ObjectsUI";
import { openReferenceDialog } from "./ReferenceDialogs";
import { insertTableOfContents, toggleFieldCodes, updateFields } from "./references";
import { openLayoutDialog } from "./LayoutDialogs";
import { openInsertDialog, pickTextFromFile } from "./InsertDialogs";
import { openAppInsert } from "./AppInsertDialogs";
import { openExtraDialog } from "./ExtraDialogs";
import type { TabProps } from "./ribbonShared";

export function InsertTab({ editor: ed, actions }: TabProps) {
  const [tbl, setTbl] = useState(false);
  return (
    <>
      <Group label="Pages">
        <RBtn icon={<DashboardIcon />} label="Building blocks" big onClick={() => openExtraDialog("blocks")} testid="rb-blocks" />
        <RBtn icon={<InsertPageBreakIcon />} label="Page break" big onClick={() => void (ed.chain().focus().insertPageBreak().run() || ed.chain().focus().setPageBreak().run())} testid="rb-pagebreak" />
      </Group>
      <Group label="Tables">
        <RMenu icon={<GridOnIcon />} label="Table" big open={tbl} onOpenChange={setTbl} testid="rb-table">
          <TableSizePicker onPick={(r, c) => { setTbl(false); insertPickedTable(ed, r, c); }} />
          <MenuItem onClick={() => openConvertTextDialog()}>Convert text to table…</MenuItem>
        </RMenu>
      </Group>
      <Group label="Illustrations">
        <RMenu icon={<ImageIcon />} label="Picture" big testid="rb-picture">
          <MenuItem onClick={() => openInsertObject("picture")}>From this device…</MenuItem>
          <MenuItem onClick={() => openInsertObject("pictureUrl")}>From a URL…</MenuItem>
        </RMenu>
        <RBtn icon={<CategoryIcon />} label="Shapes" big onClick={() => openInsertObject("shape")} />
        <RBtn icon={<CropSquareIcon />} label="Text box" big onClick={() => openInsertObject("textBox")} />
        <RBtn icon={<BrushIcon />} label="Drawing" big onClick={actions.insertDrawing} />
        <RBtn icon={<InsertChartIcon />} label="Chart" big onClick={() => openInsertObject("chart")} />
        <RBtn icon={<FunctionsIcon />} label="Equation" big onClick={() => actions.insertEquation?.()} testid="rb-equation" />
      </Group>
      <Group label="Links">
        <RBtn icon={<LinkIcon />} label="Link" big onClick={() => promptLink(ed)} />
        <RBtn icon={<BookmarkIcon />} label="Bookmark" big onClick={() => openReferenceDialog("bookmarks")} />
        <RBtn icon={<CompareArrowsIcon />} label="Cross-reference" big onClick={() => openReferenceDialog("crossref")} />
      </Group>
      <Group label="Header & footer">
        <RBtn icon={<VerticalAlignTopIcon />} label="Header & footer" big onClick={actions.toggleHeaderFooter} testid="rb-hf" />
        <RBtn icon={<NumbersIcon />} label="Page numbers" big onClick={() => openLayoutDialog("pagenumbers")} />
      </Group>
      <Group label="Text">
        <RBtn icon={<AlternateEmailIcon />} label="Smart chips" big onClick={() => openExtraDialog("chips")} testid="rb-chips" />
        <RBtn icon={<EventIcon />} label="Date & time" big onClick={() => openInsertDialog("datetime")} />
        <RMenu icon={<TranslateIcon />} label="Symbols" big>
          <MenuItem onClick={() => openInsertDialog("symbol")}>Symbol…</MenuItem>
          <MenuItem onClick={actions.specialChars}>Special characters…</MenuItem>
          <MenuItem onClick={actions.emoji}>Emoji…</MenuItem>
        </RMenu>
        <RBtn icon={<TextFieldsIcon />} label="Drop cap" big onClick={() => openInsertDialog("dropcap")} />
        <RBtn icon={<FileOpenIcon />} label="Text from file" big onClick={() => pickTextFromFile(ed)} />
      </Group>
      <Group label="Notes">
        <RBtn icon={<NoteIcon />} label="Footnote" big onClick={actions.insertFootnote} testid="rb-footnote" />
        <RBtn icon={<NoteIcon />} label="Endnote" big onClick={actions.insertEndnote} />
        <RBtn icon={<AddCommentIcon />} label="Comment" big onClick={actions.commentOnSelection} testid="rb-comment" />
      </Group>
      <Group label="From Adrient">
        <RBtn icon={<AppsIcon />} label="From app" big onClick={() => openAppInsert("app")} />
        <RBtn icon={<ContactsIcon />} label="Contact" big onClick={() => openAppInsert("contact")} />
        <RBtn icon={<VideocamIcon />} label="Meeting link" big onClick={() => openAppInsert("meeting")} />
      </Group>
    </>
  );
}

export function ReferencesTab({ editor: ed, actions }: TabProps) {
  return (
    <>
      <Group label="Table of contents">
        <RBtn icon={<TocIcon />} label="Table of contents" big onClick={() => void insertTableOfContents(ed)} testid="rb-toc" />
        <RBtn icon={<UpdateIcon />} label="Update table" big onClick={() => void updateFields(ed, "all")} testid="rb-update-fields" />
        <RBtn icon={<TocIcon />} label="TOC options" big onClick={() => openReferenceDialog("toc")} />
      </Group>
      <Group label="Footnotes">
        <RBtn icon={<NoteIcon />} label="Insert footnote" big onClick={actions.insertFootnote} />
        <RBtn icon={<NoteIcon />} label="Insert endnote" big onClick={actions.insertEndnote} />
      </Group>
      <Group label="Citations & bibliography">
        <RBtn icon={<LibraryBooksIcon />} label="Citations" big onClick={() => openExtraDialog("citations")} testid="rb-citations" />
      </Group>
      <Group label="Captions">
        <RBtn icon={<SubtitlesIcon />} label="Insert caption" big onClick={() => openReferenceDialog("caption")} />
        <RBtn icon={<CompareArrowsIcon />} label="Cross-reference" big onClick={() => openReferenceDialog("crossref")} />
        <RBtn icon={<ViewListIcon />} label="Table of figures" big onClick={() => openReferenceDialog("tof")} />
      </Group>
      <Group label="Links">
        <RBtn icon={<BookmarkIcon />} label="Bookmarks" big onClick={() => openReferenceDialog("bookmarks")} />
        <RBtn icon={<LinkIcon />} label="Link settings" big onClick={() => openReferenceDialog("hyperlink")} />
      </Group>
      <Group label="Fields">
        <RBtn icon={<NumbersIcon />} label="Page number" big onClick={() => void ed.chain().focus().insertField("PAGE").run()} />
        <RBtn icon={<EventIcon />} label="Other fields" big onClick={() => openReferenceDialog("field")} />
        <RBtn icon={<CodeIcon />} label="Field codes" big onClick={() => void toggleFieldCodes(ed)} />
      </Group>
    </>
  );
}
