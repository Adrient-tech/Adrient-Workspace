// Ribbon ▸ Review tab, and the contextual Table and Object tabs.
import { useState } from "react";
import { Box, MenuItem } from "@mui/joy";
import SpellcheckIcon from "@mui/icons-material/Spellcheck";
import LanguageIcon from "@mui/icons-material/Language";
import PollIcon from "@mui/icons-material/Poll";
import AccessibilityIcon from "@mui/icons-material/AccessibilityNew";
import AddCommentIcon from "@mui/icons-material/AddComment";
import CommentIcon from "@mui/icons-material/ChatBubbleOutline";
import RateReviewIcon from "@mui/icons-material/RateReview";
import VisibilityIcon from "@mui/icons-material/Visibility";
import SkipPreviousIcon from "@mui/icons-material/SkipPrevious";
import SkipNextIcon from "@mui/icons-material/SkipNext";
import CheckIcon from "@mui/icons-material/Check";
import CloseIcon from "@mui/icons-material/Close";
import DoneAllIcon from "@mui/icons-material/DoneAll";
import RemoveDoneIcon from "@mui/icons-material/RemoveDone";
import CompareArrowsIcon from "@mui/icons-material/CompareArrows";
import MergeIcon from "@mui/icons-material/CallMerge";
import EmailIcon from "@mui/icons-material/Email";
import MicIcon from "@mui/icons-material/Mic";
import GridOnIcon from "@mui/icons-material/GridOn";
import TableRowsIcon from "@mui/icons-material/TableRows";
import ViewColumnIcon from "@mui/icons-material/ViewColumn";
import ViewListIcon from "@mui/icons-material/ViewList";
import SplitIcon from "@mui/icons-material/CallSplit";
import DeleteIcon from "@mui/icons-material/Delete";
import FormatColorFillIcon from "@mui/icons-material/FormatColorFill";
import FormatAlignLeftIcon from "@mui/icons-material/FormatAlignLeft";
import SettingsIcon from "@mui/icons-material/Settings";
import AspectRatioIcon from "@mui/icons-material/AspectRatio";
import SortIcon from "@mui/icons-material/SortByAlpha";
import CalculateIcon from "@mui/icons-material/Calculate";
import { Group, RBtn, RMenu } from "./RibbonKit";
import { TEXT_COLORS } from "./Toolbar";
import { openCompareDialog } from "./CompareDialog";
import { openMailMerge } from "./MailMergePanel";
import { openProofingDialog } from "./SpellMenu";
import { spellService } from "../../lib/spell/service";
import { openExtraDialog } from "./ExtraDialogs";
import { openTableSettings } from "./TableUI";
import { openObjectSettings } from "./ObjectsUI";
import { applyTableStyle, setLookOption, setCellBackground, toggleRepeatHeader, distributeColumns, distributeRows, autofitTable, splitTable, currentTableAttrs, setCellProps } from "./tables";
import { TABLE_TEMPLATES, parseLook } from "./tableModel";
import { insertTableFormula, sortTable } from "./tableOps";
import { head, swatch, type TabProps } from "./ribbonShared";
import type { Editor } from "@tiptap/react";

export function ReviewTab({ actions, mode, voice, onCheckSpell }: TabProps) {
  const [spell, setSpell] = useState(spellService().enabled);
  return (
    <>
      <Group label="Proofing">
        <RBtn icon={<SpellcheckIcon />} label="Spelling & grammar" big onClick={() => openProofingDialog("spelling")} testid="rb-spelling" />
        <RBtn icon={<SpellcheckIcon />} label={spell ? "Spell check: on" : "Spell check: off"} big active={spell} onClick={() => { const n = !spell; setSpell(n); onCheckSpell(n); }} testid="rb-spell-toggle" />
        <RBtn icon={<LanguageIcon />} label="Language" big onClick={() => openProofingDialog("language")} />
        <RBtn icon={<PollIcon />} label="Word count" big onClick={actions.wordCount} testid="rb-wordcount" />
        <RBtn icon={<AccessibilityIcon />} label="Accessibility" big onClick={() => openExtraDialog("a11y")} testid="rb-a11y" />
      </Group>
      <Group label="Comments">
        <RBtn icon={<AddCommentIcon />} label="New comment" big onClick={actions.commentOnSelection} />
        <RBtn icon={<CommentIcon />} label="Show comments" big onClick={actions.comments} testid="rb-show-comments" />
      </Group>
      <Group label="Tracking">
        <RBtn icon={<RateReviewIcon />} label="Suggesting" big active={mode === "suggesting"} onClick={actions.toggleSuggesting} testid="rb-suggesting" />
        <RBtn icon={<RateReviewIcon />} label="Review panel" big onClick={() => actions.reviewPanel?.()} />
        <RMenu icon={<VisibilityIcon />} label="Display" big>
          <MenuItem onClick={() => actions.setDisplayMode?.("markup")}>Markup</MenuItem>
          <MenuItem onClick={() => actions.setDisplayMode?.("simple")}>Simple markup</MenuItem>
          <MenuItem onClick={() => actions.setDisplayMode?.("final")}>Final</MenuItem>
          <MenuItem onClick={() => actions.setDisplayMode?.("original")}>Original</MenuItem>
        </RMenu>
      </Group>
      <Group label="Changes">
        <RBtn icon={<SkipPreviousIcon />} label="Previous" big onClick={() => actions.previousChange?.()} />
        <RBtn icon={<SkipNextIcon />} label="Next" big onClick={() => actions.nextChange?.()} />
        <RBtn icon={<CheckIcon />} label="Accept" big onClick={() => actions.acceptCurrentChange?.()} testid="rb-accept" />
        <RBtn icon={<CloseIcon />} label="Reject" big onClick={() => actions.rejectCurrentChange?.()} testid="rb-reject" />
        <RBtn icon={<DoneAllIcon />} label="Accept all" big onClick={() => actions.acceptAllChanges?.()} />
        <RBtn icon={<RemoveDoneIcon />} label="Reject all" big onClick={() => actions.rejectAllChanges?.()} />
      </Group>
      <Group label="Compare">
        <RBtn icon={<CompareArrowsIcon />} label="Compare" big onClick={() => openCompareDialog("compare")} />
        <RBtn icon={<MergeIcon />} label="Combine" big onClick={() => openCompareDialog("combine")} />
      </Group>
      <Group label="Mailings">
        <RBtn icon={<EmailIcon />} label="Mail merge" big onClick={() => openMailMerge()} testid="rb-mailmerge" />
      </Group>
      {voice?.supported && (
        <Group label="Dictation">
          <RBtn icon={<MicIcon />} label="Voice typing" big active={voice.active} onClick={voice.toggle} testid="rb-voice" />
        </Group>
      )}
    </>
  );
}

export function TableTab({ editor: ed }: { editor: Editor }) {
  const look = parseLook(currentTableAttrs(ed)?.look);
  const chain = () => ed.chain().focus();
  const fns = ["SUM", "AVERAGE", "COUNT", "MIN", "MAX", "PRODUCT"] as const;
  return (
    <>
      <Group label="Rows & columns">
        <RBtn icon={<TableRowsIcon />} label="Row above" big onClick={() => chain().addRowBefore().run()} testid="tb-row-above" />
        <RBtn icon={<TableRowsIcon />} label="Row below" big onClick={() => chain().addRowAfter().run()} testid="tb-row-below" />
        <RBtn icon={<ViewColumnIcon />} label="Column left" big onClick={() => chain().addColumnBefore().run()} />
        <RBtn icon={<ViewColumnIcon />} label="Column right" big onClick={() => chain().addColumnAfter().run()} testid="tb-col-right" />
        <RMenu icon={<DeleteIcon />} label="Delete" big>
          <MenuItem onClick={() => chain().deleteRow().run()}>Delete row</MenuItem>
          <MenuItem onClick={() => chain().deleteColumn().run()}>Delete column</MenuItem>
          <MenuItem color="danger" onClick={() => chain().deleteTable().run()}>Delete table</MenuItem>
        </RMenu>
      </Group>
      <Group label="Merge">
        <RBtn icon={<MergeIcon />} label="Merge cells" big onClick={() => chain().mergeCells().run()} testid="tb-merge" />
        <RBtn icon={<SplitIcon />} label="Split cell" big onClick={() => chain().splitCell().run()} />
        <RBtn icon={<SplitIcon />} label="Split table" big onClick={() => void splitTable(ed)} />
      </Group>
      <Group label="Table style">
        <RMenu icon={<GridOnIcon />} label="Styles" big testid="tb-styles">
          {TABLE_TEMPLATES.map((t) => (
            <MenuItem key={t.id} onClick={() => void applyTableStyle(ed, t.id)}>{t.name}</MenuItem>
          ))}
        </RMenu>
        <RBtn icon={<ViewListIcon />} label="Header row" big active={look.header} onClick={() => void setLookOption(ed, "header", !look.header)} testid="tb-header" />
        <RBtn icon={<TableRowsIcon />} label="Banded rows" big active={look.banded} onClick={() => void setLookOption(ed, "banded", !look.banded)} testid="tb-banded" />
        <RBtn icon={<ViewColumnIcon />} label="Banded columns" big active={look.bandedCols} onClick={() => void setLookOption(ed, "bandedCols", !look.bandedCols)} />
        <RBtn icon={<ViewListIcon />} label="Repeat header" big onClick={() => void toggleRepeatHeader(ed)} />
      </Group>
      <Group label="Cell">
        <RMenu icon={<FormatColorFillIcon />} label="Shading" big>
          <Box sx={{ display: "grid", gridTemplateColumns: "repeat(7, 18px)", gap: 0.5, p: 1 }}>
            {TEXT_COLORS.map((c) => swatch(c, () => void setCellBackground(ed, c)))}
          </Box>
          <MenuItem onClick={() => void setCellBackground(ed, null)}>No shading</MenuItem>
        </RMenu>
        <RMenu icon={<FormatAlignLeftIcon />} label="Cell alignment" big>
          <MenuItem onClick={() => void setCellProps(ed, { valign: "top" })}>Top</MenuItem>
          <MenuItem onClick={() => void setCellProps(ed, { valign: "center" })}>Middle</MenuItem>
          <MenuItem onClick={() => void setCellProps(ed, { valign: "bottom" })}>Bottom</MenuItem>
        </RMenu>
        <RBtn icon={<SettingsIcon />} label="Properties" big onClick={() => openTableSettings()} testid="tb-properties" />
      </Group>
      <Group label="Size">
        <RBtn icon={<ViewColumnIcon />} label="Even columns" big onClick={() => void distributeColumns(ed)} />
        <RBtn icon={<TableRowsIcon />} label="Even rows" big onClick={() => void distributeRows(ed)} />
        <RMenu icon={<AspectRatioIcon />} label="AutoFit" big>
          <MenuItem onClick={() => void autofitTable(ed, "contents")}>To contents</MenuItem>
          <MenuItem onClick={() => void autofitTable(ed, "window")}>To window</MenuItem>
          <MenuItem onClick={() => void autofitTable(ed, "fixed")}>Fixed width</MenuItem>
        </RMenu>
      </Group>
      <Group label="Data">
        <RMenu icon={<SortIcon />} label="Sort" big testid="tb-sort">
          {head("By the column of the cursor (first row is a header)")}
          <MenuItem data-testid="tb-sort-az" onClick={() => void sortTable(ed, { asc: true, type: "text" })}>Text A to Z</MenuItem>
          <MenuItem onClick={() => void sortTable(ed, { asc: false, type: "text" })}>Text Z to A</MenuItem>
          <MenuItem onClick={() => void sortTable(ed, { asc: true, type: "number" })}>Numbers, smallest first</MenuItem>
          <MenuItem onClick={() => void sortTable(ed, { asc: false, type: "number" })}>Numbers, largest first</MenuItem>
          <MenuItem onClick={() => void sortTable(ed, { asc: true, type: "date" })}>Dates, oldest first</MenuItem>
        </RMenu>
        <RMenu icon={<CalculateIcon />} label="Formula" big testid="tb-formula">
          {head("Result is written as text and is not updated later")}
          {fns.flatMap((f) => [
            <MenuItem key={f + "a"} data-testid={`tb-${f.toLowerCase()}-above`} onClick={() => void insertTableFormula(ed, f, "above")}>{f} of cells above</MenuItem>,
            <MenuItem key={f + "l"} onClick={() => void insertTableFormula(ed, f, "left")}>{f} of cells to the left</MenuItem>,
          ])}
        </RMenu>
      </Group>
    </>
  );
}

export function ObjectTab() {
  return (
    <Group label="Selected object">
      <RBtn icon={<SettingsIcon />} label="Format object" big onClick={() => openObjectSettings()} testid="ob-format" />
    </Group>
  );
}
