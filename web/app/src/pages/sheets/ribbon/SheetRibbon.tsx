/* eslint-disable @typescript-eslint/no-explicit-any -- FortuneSheet API is loosely typed. */
import { useEffect, useRef, useState, type ReactNode } from "react";
import { Box, Dropdown, Menu, MenuButton, MenuItem, ListDivider, Tooltip, Typography, Select, Option } from "@mui/joy";
import UndoIcon from "@mui/icons-material/Undo";
import RedoIcon from "@mui/icons-material/Redo";
import ContentCutIcon from "@mui/icons-material/ContentCut";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import ContentPasteIcon from "@mui/icons-material/ContentPaste";
import FormatPaintIcon from "@mui/icons-material/FormatPaint";
import FormatBoldIcon from "@mui/icons-material/FormatBold";
import FormatItalicIcon from "@mui/icons-material/FormatItalic";
import FormatUnderlinedIcon from "@mui/icons-material/FormatUnderlined";
import StrikethroughSIcon from "@mui/icons-material/StrikethroughS";
import FormatColorTextIcon from "@mui/icons-material/FormatColorText";
import FormatColorFillIcon from "@mui/icons-material/FormatColorFill";
import BorderAllIcon from "@mui/icons-material/BorderAll";
import FormatAlignLeftIcon from "@mui/icons-material/FormatAlignLeft";
import FormatAlignCenterIcon from "@mui/icons-material/FormatAlignCenter";
import FormatAlignRightIcon from "@mui/icons-material/FormatAlignRight";
import VerticalAlignCenterIcon from "@mui/icons-material/VerticalAlignCenter";
import WrapTextIcon from "@mui/icons-material/WrapText";
import MergeTypeIcon from "@mui/icons-material/MergeType";
import FormatClearIcon from "@mui/icons-material/FormatClear";
import AddIcon from "@mui/icons-material/Add";
import RemoveIcon from "@mui/icons-material/Remove";
import FunctionsIcon from "@mui/icons-material/Functions";
import SortByAlphaIcon from "@mui/icons-material/SortByAlpha";
import FilterAltIcon from "@mui/icons-material/FilterAlt";
import SearchIcon from "@mui/icons-material/Search";
import TableChartIcon from "@mui/icons-material/TableChart";
import BarChartIcon from "@mui/icons-material/BarChart";
import PivotTableChartIcon from "@mui/icons-material/PivotTableChart";
import ShowChartIcon from "@mui/icons-material/ShowChart";
import LinkIcon from "@mui/icons-material/Link";
import CommentIcon from "@mui/icons-material/Comment";
import PrintIcon from "@mui/icons-material/Print";
import LockIcon from "@mui/icons-material/Lock";
import HistoryIcon from "@mui/icons-material/History";
import RuleIcon from "@mui/icons-material/Rule";
import PaletteIcon from "@mui/icons-material/Palette";
import DataArraySharpIcon from "@mui/icons-material/DataArray";
import AccountTreeIcon from "@mui/icons-material/AccountTree";
import FlagIcon from "@mui/icons-material/Flag";
import KeyboardIcon from "@mui/icons-material/Keyboard";
import LabelIcon from "@mui/icons-material/Label";
import { useShortcutScheme } from "../../../lib/shortcutScheme";
import { ResponsiveGroup, ResponsiveRibbonBody, useRibbonCompact } from "../../../components/ribbon/ResponsiveRibbon";
import type { SheetMenuBarProps } from "../SheetMenuBar";
import { splitTextToColumns, sortRange, toggleFilter } from "../dataActions";
import { deleteRowsCols, fillSelection, insertRowsCols, shiftCells } from "../editActions";
import ArrowDownwardIcon from "@mui/icons-material/ArrowDownward";
import ViewColumnIcon from "@mui/icons-material/ViewColumn";
import { selectionRanges, applyNumberFormat } from "../numberFormatActions";
import { currentSheet } from "../sheetDataTools";
import { TABLE_STYLE_GALLERY } from "../tables";
import { OutlineMenuItems } from "../OutlineMenuItems";
import {
  BORDER_KINDS,
  CELL_STYLES,
  MERGE_KINDS,
  NUMBER_FORMAT_GALLERY,
  applyBorder,
  applyCellStyle,
  applyMerge,
  captureFormat,
  paintFormat,
  previewFormat,
  stepDecimals,
  visibleTabs,
  type CopiedFormat,
  type RibbonTabId,
} from "./ribbonLogic";

export interface SheetRibbonProps {
  menu: SheetMenuBarProps;
  onAutoSum: () => void;
  collapsed: boolean;
  onCollapsedChange: (v: boolean) => void;
  /** Called to toggle the Classic menus layout. */
  onClassic: () => void;
  /** Data ▸ What-if analysis tools. */
  onWhatIf?: (tab: "goalseek" | "datatable" | "scenarios" | "solver") => void;
  /** Data ▸ Summary statistics of the selection. */
  onSummaryStats?: () => void;
}

const FONT_SIZES = [8, 9, 10, 11, 12, 14, 16, 18, 20, 24, 28, 36, 48, 72];
const PALETTE = [
  "#000000", "#434343", "#666666", "#999999", "#CCCCCC", "#FFFFFF",
  "#E6B8AF", "#F4CCCC", "#FCE5CD", "#FFF2CC", "#D9EAD3", "#D0E0E3", "#C9DAF8", "#D9D2E9",
  "#CC0000", "#E69138", "#F1C232", "#6AA84F", "#126C5A", "#3C78D8", "#674EA7", "#A61C00",
];

function Btn({ label, icon, onClick, disabled, big: bigWanted, active, keys, labelled: labelWanted }: { label: string; icon: ReactNode; onClick?: () => void; disabled?: boolean; big?: boolean; active?: boolean; keys?: string; labelled?: boolean }) {
  const compact = useRibbonCompact();
  const big = bigWanted && !compact;
  // A small button with its label beside the icon (Excel's stacked AutoSum / Fill / Clear).
  const labelled = labelWanted && !compact;
  return (
    <Tooltip title={keys ? `${label} (${keys})` : label} size="sm" enterDelay={400}>
      <Box
        component="button"
        type="button"
        aria-label={label}
        aria-pressed={active}
        data-rb="1"
        disabled={disabled}
        onMouseDown={(e: any) => e.preventDefault()}
        onClick={onClick}
        sx={{
          display: "flex",
          flexDirection: big ? "column" : "row",
          alignItems: "center",
          justifyContent: labelled ? "flex-start" : "center",
          gap: 0.25,
          minWidth: big ? 52 : 28,
          height: big ? 54 : labelWanted ? 24 : 28,
          px: 0.5,
          border: "none",
          borderRadius: "4px",
          bgcolor: active ? "primary.softBg" : "transparent",
          color: active ? "primary.softColor" : "text.primary",
          boxShadow: active ? "inset 0 0 0 1px var(--joy-palette-primary-outlinedBorder)" : "none",
          cursor: disabled ? "default" : "pointer",
          opacity: disabled ? 0.4 : 1,
          font: "inherit",
          "&:hover:not(:disabled)": { bgcolor: "background.level1" },
          "& svg": { fontSize: big ? 22 : 18 },
        }}
      >
        {icon}
        {big && <Typography level="body-xs" sx={{ lineHeight: 1.1, textAlign: "center", maxWidth: 72, whiteSpace: "normal" }}>{label}</Typography>}
        {labelled && <Typography level="body-xs" sx={{ whiteSpace: "nowrap" }}>{label}</Typography>}
      </Box>
    </Tooltip>
  );
}

function MenuBtn({ label, icon, big: bigWanted, children, width = 220, labelled: labelWanted }: { label: string; icon: ReactNode; big?: boolean; children: ReactNode; width?: number; labelled?: boolean }) {
  const compact = useRibbonCompact();
  const big = bigWanted && !compact;
  const labelled = labelWanted && !compact;
  return (
    <Dropdown>
      <Tooltip title={label} size="sm" enterDelay={400}>
        <MenuButton
          aria-label={label}
          data-rb="1"
          variant="plain"
          size="sm"
          sx={{
            flexDirection: big ? "column" : "row",
            justifyContent: labelled ? "flex-start" : "center",
            minWidth: big ? 52 : 36,
            height: big ? 54 : labelWanted ? 24 : 28,
            minHeight: labelWanted ? 24 : undefined,
            px: 0.5,
            gap: 0.25,
            fontWeight: 400,
            "& svg": { fontSize: big ? 22 : 18 },
          }}
        >
          {icon}
          {big && <Typography level="body-xs" sx={{ lineHeight: 1.1 }}>{label} ▾</Typography>}
          {labelled && <Typography level="body-xs" sx={{ whiteSpace: "nowrap" }}>{label} ▾</Typography>}
        </MenuButton>
      </Tooltip>
      <Menu size="sm" sx={{ minWidth: width, maxHeight: "70vh", overflowY: "auto", zIndex: 2000 }}>
        {children}
      </Menu>
    </Dropdown>
  );
}

function Group({ title, children, icon, priority }: { title: string; children: ReactNode; icon?: ReactNode; priority?: number }) {
  return (
    <ResponsiveGroup title={title} icon={icon} priority={priority}>
      {children}
    </ResponsiveGroup>
  );
}

function ColorMenu({ label, icon, onPick, onClear }: { label: string; icon: ReactNode; onPick: (c: string) => void; onClear: () => void }) {
  return (
    <MenuBtn label={label} icon={icon} width={170}>
      <Box sx={{ display: "grid", gridTemplateColumns: "repeat(6, 22px)", gap: 0.5, p: 1 }}>
        {PALETTE.map((c) => (
          <Box
            key={c}
            role="button"
            aria-label={`${label} ${c}`}
            onClick={() => onPick(c)}
            sx={{ width: 22, height: 22, bgcolor: c, border: "1px solid", borderColor: "neutral.outlinedBorder", borderRadius: "3px", cursor: "pointer" }}
          />
        ))}
      </Box>
      <ListDivider />
      <MenuItem onClick={onClear}>No colour</MenuItem>
    </MenuBtn>
  );
}

export function SheetRibbon({ menu, onAutoSum, collapsed, onCollapsedChange, onClassic, onWhatIf, onSummaryStats }: SheetRibbonProps) {
  const [tab, setTab] = useState<RibbonTabId>("home");
  const [painter, setPainter] = useState<CopiedFormat | null>(null);
  useShortcutScheme("sheets");
  const wb = (): any => {
    try {
      return menu.getWb();
    } catch {
      return null;
    }
  };
  const run = (fn: (w: any) => void) => () => {
    const w = wb();
    if (!w) return;
    try {
      fn(w);
    } catch {
      /* ignore */
    }
  };
  const fmt = (attr: string, value: any) => run((w) => selectionRanges(w).forEach((r) => w.setCellFormatByRange(attr, value, r)));
  const activeCell = (w: any): any => {
    const r = selectionRanges(w)[0];
    return r ? (w.getCellsByRange({ row: [r.row[0], r.row[0]], column: [r.column[0], r.column[0]] })?.[0]?.[0] ?? {}) : {};
  };
  const toggle = (attr: string) =>
    run((w) => {
      const on = activeCell(w)?.[attr] ? 0 : 1;
      selectionRanges(w).forEach((r) => w.setCellFormatByRange(attr, on, r));
    });
  const stepFont = (d: 1 | -1) =>
    run((w) => {
      const cur = Number(activeCell(w)?.fs) || 10;
      const next = Math.max(1, Math.min(96, cur + d));
      selectionRanges(w).forEach((r) => w.setCellFormatByRange("fs", next, r));
    });
  const decimals = (d: 1 | -1) =>
    run((w) => applyNumberFormat(w, stepDecimals(String(activeCell(w)?.ct?.fa ?? "General"), d)));
  // Re-read the active cell after clicks and keys so toggles show their state.
  const [, bump] = useState(0);
  useEffect(() => {
    const f = () => window.setTimeout(() => bump((n) => n + 1), 0);
    window.addEventListener("mouseup", f);
    window.addEventListener("keyup", f);
    return () => {
      window.removeEventListener("mouseup", f);
      window.removeEventListener("keyup", f);
    };
  }, []);
  const on = (attr: string): any => {
    const w = wb();
    if (!w) return undefined;
    try {
      return activeCell(w)?.[attr];
    } catch {
      return undefined;
    }
  };
  const selRow = (w: any) => selectionRanges(w)[0]?.row ?? [0, 0];
  const selCol = (w: any) => selectionRanges(w)[0]?.column ?? [0, 0];
  const span = ([a, b]: number[]) => Array.from({ length: Math.min(5000, b - a + 1) }, (_, i) => String(a + i));
  const clearFormats = (w: any) =>
    selectionRanges(w).forEach((r) => {
      for (const a of ["bl", "it", "un", "cl", "fc", "bg", "fs", "ff", "ht", "vt", "tb"]) w.setCellFormatByRange(a, undefined, r);
      applyNumberFormat(w, "General", [r]);
    });
  const clearContents = (w: any) => {
    const ops: { name: string; args: any[] }[] = [];
    for (const r of selectionRanges(w))
      for (let i = r.row[0]; i <= r.row[1] && ops.length < 20000; i++)
        for (let j = r.column[0]; j <= r.column[1] && ops.length < 20000; j++) ops.push({ name: "clearCell", args: [i, j] });
    if (ops.length) w.batchCallApis?.(ops);
  };
  const askSize = (w: any, axis: "row" | "col") => {
    const [a, b] = axis === "row" ? selRow(w) : selCol(w);
    const v = window.prompt(axis === "row" ? "Row height in pixels" : "Column width in pixels", axis === "row" ? "20" : "73");
    const n = Math.round(Number(v));
    if (!v || !Number.isFinite(n) || n < 2 || n > 2000) return;
    const sizes = Object.fromEntries(span([a, b]).map((k) => [k, n]));
    if (axis === "row") w.setRowHeight?.(sizes);
    else w.setColumnWidth?.(sizes);
  };
  const view = menu.view;
  const tabs = visibleTabs();

  const home = (
    <>
      <Group title="Clipboard" priority={80} icon={<ContentPasteIcon />}>
        <Btn big label="Paste special" icon={<ContentPasteIcon />} onClick={menu.onPasteSpecial} disabled={!menu.onPasteSpecial} />
        <Btn label="Cut" keys="Ctrl+X" icon={<ContentCutIcon />} onClick={() => document.execCommand("cut")} />
        <Btn label="Copy" keys="Ctrl+C" icon={<ContentCopyIcon />} onClick={() => document.execCommand("copy")} />
        <Btn
          label={painter ? "Format painter (click a cell range to apply)" : "Format painter"}
          icon={<FormatPaintIcon />}
          active={!!painter}
          onClick={run((w) => {
            if (painter) {
              paintFormat(w, painter);
              setPainter(null);
            } else {
              setPainter(captureFormat(w));
            }
          })}
        />
      </Group>
      <Group title="Font" priority={75} icon={<FormatBoldIcon />}>
        <Btn label="Decrease font size" icon={<RemoveIcon />} onClick={stepFont(-1)} />
        <Select size="sm" variant="plain" placeholder="11" defaultValue={null} sx={{ minWidth: 64 }} aria-label="Font size" onChange={(_, v) => v && run((w) => selectionRanges(w).forEach((r) => w.setCellFormatByRange("fs", v, r)))()}>
          {FONT_SIZES.map((s) => (
            <Option key={s} value={s}>{s}</Option>
          ))}
        </Select>
        <Btn label="Increase font size" icon={<AddIcon />} onClick={stepFont(1)} />
        <Btn label="Bold" keys="Ctrl+B" icon={<FormatBoldIcon />} active={!!on("bl")} onClick={toggle("bl")} />
        <Btn label="Italic" keys="Ctrl+I" icon={<FormatItalicIcon />} active={!!on("it")} onClick={toggle("it")} />
        <Btn label="Underline" keys="Ctrl+U" icon={<FormatUnderlinedIcon />} active={!!on("un")} onClick={toggle("un")} />
        <Btn label="Strikethrough" keys="Ctrl+5" icon={<StrikethroughSIcon />} active={!!on("cl")} onClick={toggle("cl")} />
        <MenuBtn label="Borders" icon={<BorderAllIcon />}>
          {BORDER_KINDS.map((b) => (
            <MenuItem key={b.id} onClick={run((w) => applyBorder(w, b.id))}>{b.label}</MenuItem>
          ))}
        </MenuBtn>
        <ColorMenu label="Fill colour" icon={<FormatColorFillIcon />} onPick={(c) => fmt("bg", c)()} onClear={fmt("bg", undefined)} />
        <ColorMenu label="Text colour" icon={<FormatColorTextIcon />} onPick={(c) => fmt("fc", c)()} onClear={fmt("fc", undefined)} />
        <Btn
          label="Clear formatting"
          icon={<FormatClearIcon />}
          onClick={run((w) =>
            selectionRanges(w).forEach((r) => {
              for (const a of ["bl", "it", "un", "cl", "fc", "bg", "fs", "ff", "ht", "vt", "tb"]) w.setCellFormatByRange(a, undefined, r);
              applyNumberFormat(w, "General", [r]);
            }),
          )}
        />
      </Group>
      <Group title="Alignment" priority={50} icon={<FormatAlignCenterIcon />}>
        <Btn label="Align left" keys="Ctrl+Shift+L" active={on("ht") === "1"} icon={<FormatAlignLeftIcon />} onClick={fmt("ht", "1")} />
        <Btn label="Align centre" keys="Ctrl+Shift+E" active={on("ht") === "0"} icon={<FormatAlignCenterIcon />} onClick={fmt("ht", "0")} />
        <Btn label="Align right" keys="Ctrl+Shift+R" active={on("ht") === "2"} icon={<FormatAlignRightIcon />} onClick={fmt("ht", "2")} />
        <MenuBtn label="Vertical alignment" icon={<VerticalAlignCenterIcon />} width={160}>
          <MenuItem selected={on("vt") === "1"} onClick={fmt("vt", "1")}>Align top</MenuItem>
          <MenuItem selected={on("vt") === "0"} onClick={fmt("vt", "0")}>Align middle</MenuItem>
          <MenuItem selected={on("vt") === "2"} onClick={fmt("vt", "2")}>Align bottom</MenuItem>
        </MenuBtn>
        <Btn label="Wrap text" active={String(on("tb")) === "2"} icon={<WrapTextIcon />} onClick={run((w) => {
          const on = String(activeCell(w)?.tb) === "2" ? "0" : "2";
          selectionRanges(w).forEach((r) => w.setCellFormatByRange("tb", on, r));
        })} />
        <MenuBtn label="Merge" icon={<MergeTypeIcon />}>
          {MERGE_KINDS.map((m) => (
            <MenuItem key={m.id} onClick={run((w) => applyMerge(w, m.id))}>{m.label}</MenuItem>
          ))}
        </MenuBtn>
      </Group>
      <Group title="Number" priority={30} icon={<Typography>123</Typography>}>
        <Select
          size="sm"
          variant="plain"
          placeholder="General"
          aria-label="Number format"
          sx={{ minWidth: 112 }}
          onChange={(_, v) => v && run((w) => applyNumberFormat(w, String(v)))()}
          slotProps={{ listbox: { sx: { zIndex: 2000 } } }}
        >
          {NUMBER_FORMAT_GALLERY.map((g) => (
            <Option key={g.id} value={g.fa}>
              <Box sx={{ display: "flex", justifyContent: "space-between", width: 240 }}>
                <span>{g.label}</span>
                <Typography level="body-xs">{previewFormat(g.fa, g.sample)}</Typography>
              </Box>
            </Option>
          ))}
        </Select>
        <Btn label="Currency" icon={<Typography>$</Typography>} onClick={run((w) => applyNumberFormat(w, "$#,##0.00"))} />
        <Btn label="Percent" icon={<Typography>%</Typography>} onClick={run((w) => applyNumberFormat(w, "0.00%"))} />
        <Btn label="Comma style" icon={<Typography>,</Typography>} onClick={run((w) => applyNumberFormat(w, "#,##0.00"))} />
        <Btn label="Decrease decimals" icon={<Typography level="body-xs">.0←</Typography>} onClick={decimals(-1)} />
        <Btn label="Increase decimals" icon={<Typography level="body-xs">.00→</Typography>} onClick={decimals(1)} />
        <Btn label="More number formats" icon={<Typography level="body-xs">123…</Typography>} onClick={menu.onCustomNumberFormat} />
      </Group>
      <Group title="Styles" priority={10} icon={<PaletteIcon />}>
        <Btn big label="Conditional formatting" icon={<RuleIcon />} onClick={menu.onConditionalFormat} />
        <MenuBtn big label="Format as table" icon={<TableChartIcon />} width={260}>
          {TABLE_STYLE_GALLERY.map((g) => (
            <Box key={g.group}>
              <Typography level="body-xs" sx={{ px: 1.5, pt: 0.5, fontWeight: 600 }}>{g.group}</Typography>
              {g.names.slice(0, 7).map((n) => (
                <MenuItem key={n} onClick={() => menu.onFormatAsTable?.(n)}>{n.replace("TableStyle", "")}</MenuItem>
              ))}
            </Box>
          ))}
        </MenuBtn>
        <MenuBtn big label="Cell styles" icon={<PaletteIcon />} width={240}>
          {CELL_STYLES.map((s) => (
            <MenuItem key={s.id} onClick={run((w) => applyCellStyle(w, s))}>
              <Box sx={{ px: 1, py: 0.25, bgcolor: s.bg, color: s.fc, fontWeight: s.bl ? 700 : 400, fontSize: s.fs ? Math.min(s.fs, 16) : undefined, flex: 1 }}>{s.label}</Box>
            </MenuItem>
          ))}
        </MenuBtn>
      </Group>
      <Group title="Cells" priority={45} icon={<TableChartIcon />}>
        <MenuBtn big label="Insert" icon={<AddIcon />} width={200}>
          <MenuItem onClick={run((w) => shiftCells(w, "right"))}>Insert cells, shift right</MenuItem>
          <MenuItem onClick={run((w) => shiftCells(w, "down"))}>Insert cells, shift down</MenuItem>
          <ListDivider />
          <MenuItem onClick={run((w) => insertRowsCols(w, "row", selRow(w)[0], 1, "before"))}>Insert row above</MenuItem>
          <MenuItem onClick={run((w) => insertRowsCols(w, "row", selRow(w)[1], 1, "after"))}>Insert row below</MenuItem>
          <MenuItem onClick={run((w) => insertRowsCols(w, "col", selCol(w)[0], 1, "before"))}>Insert column left</MenuItem>
          <MenuItem onClick={run((w) => insertRowsCols(w, "col", selCol(w)[1], 1, "after"))}>Insert column right</MenuItem>
        </MenuBtn>
        <MenuBtn big label="Delete" icon={<RemoveIcon />} width={200}>
          <MenuItem onClick={run((w) => shiftCells(w, "up"))}>Delete cells, shift up</MenuItem>
          <MenuItem onClick={run((w) => shiftCells(w, "left"))}>Delete cells, shift left</MenuItem>
          <ListDivider />
          <MenuItem onClick={run((w) => deleteRowsCols(w, "row", selRow(w)[0], selRow(w)[1]))}>Delete rows</MenuItem>
          <MenuItem onClick={run((w) => deleteRowsCols(w, "col", selCol(w)[0], selCol(w)[1]))}>Delete columns</MenuItem>
        </MenuBtn>
        <MenuBtn big label="Format" icon={<ViewColumnIcon />} width={220}>
          <MenuItem onClick={run((w) => askSize(w, "row"))}>Row height…</MenuItem>
          <MenuItem onClick={run((w) => askSize(w, "col"))}>Column width…</MenuItem>
          <ListDivider />
          <MenuItem onClick={run((w) => w.hideRowOrColumn(span(selRow(w)), "row"))}>Hide rows</MenuItem>
          <MenuItem onClick={run((w) => w.showRowOrColumn(span(selRow(w)), "row"))}>Unhide rows</MenuItem>
          <MenuItem onClick={run((w) => w.hideRowOrColumn(span(selCol(w)), "column"))}>Hide columns</MenuItem>
          <MenuItem onClick={run((w) => w.showRowOrColumn(span(selCol(w)), "column"))}>Unhide columns</MenuItem>
          <ListDivider />
          <MenuItem onClick={menu.onProtect} disabled={!menu.onProtect}>Protect sheet…</MenuItem>
        </MenuBtn>
      </Group>
      <Group title="Editing" priority={90} icon={<FunctionsIcon />}>
        <Box sx={{ display: "flex", flexDirection: "column", alignItems: "stretch" }}>
          <Btn labelled label="AutoSum" keys="Alt+=" icon={<FunctionsIcon />} onClick={onAutoSum} />
          <MenuBtn labelled label="Fill" icon={<ArrowDownwardIcon />} width={180}>
          <MenuItem onClick={run((w) => fillSelection(w, "down"))}>Down (Ctrl+D)</MenuItem>
          <MenuItem onClick={run((w) => fillSelection(w, "right"))}>Right (Ctrl+R)</MenuItem>
          <MenuItem onClick={run((w) => fillSelection(w, "up"))}>Up</MenuItem>
          <MenuItem onClick={run((w) => fillSelection(w, "left"))}>Left</MenuItem>
          <ListDivider />
          <MenuItem onClick={menu.onFillSeries} disabled={!menu.onFillSeries}>Series…</MenuItem>
        </MenuBtn>
        <MenuBtn labelled label="Clear" icon={<FormatClearIcon />} width={180}>
          <MenuItem onClick={run((w) => { clearContents(w); clearFormats(w); })}>Clear all</MenuItem>
          <MenuItem onClick={run(clearFormats)}>Clear formats</MenuItem>
          <MenuItem onClick={run(clearContents)}>Clear contents (Delete)</MenuItem>
        </MenuBtn>
        </Box>
        <MenuBtn big label="Sort & filter" icon={<SortByAlphaIcon />} width={200}>
          <MenuItem onClick={run((w) => sortRange(w, true))}>Sort A to Z</MenuItem>
          <MenuItem onClick={run((w) => sortRange(w, false))}>Sort Z to A</MenuItem>
          <MenuItem onClick={() => menu.onSortDialog?.()} disabled={!menu.onSortDialog}>Custom sort…</MenuItem>
          <ListDivider />
          <MenuItem onClick={run((w) => toggleFilter(w))}>Toggle filter</MenuItem>
        </MenuBtn>
        <MenuBtn big label="Find & select" icon={<SearchIcon />} width={200}>
          <MenuItem onClick={menu.onFindReplace}>Find and replace… (Ctrl+F)</MenuItem>
          <MenuItem onClick={run((w) => w.setSelection?.([{ row: [0, Math.max(0, (currentSheet(w)?.data?.length ?? 1) - 1)], column: [0, Math.max(0, (currentSheet(w)?.data?.[0]?.length ?? 1) - 1)] }]))}>Select all (Ctrl+A)</MenuItem>
          <MenuItem onClick={menu.onNamedRanges}>Go to a named range…</MenuItem>
        </MenuBtn>
      </Group>
    </>
  );

  const insert = (
    <>
      <Group title="Tables" priority={30} icon={<TableChartIcon />}>
        <Btn big label="Table" icon={<TableChartIcon />} onClick={menu.onInsertTable} disabled={!menu.onInsertTable} />
        <Btn big label="PivotTable" icon={<PivotTableChartIcon />} onClick={menu.onInsertPivot} />
      </Group>
      <Group title="Charts" priority={40} icon={<BarChartIcon />}>
        <Btn big label="Chart" icon={<BarChartIcon />} onClick={menu.onInsertChart} />
        <Btn big label="Sparklines" icon={<ShowChartIcon />} onClick={menu.onInsertSparklines} disabled={!menu.onInsertSparklines} />
      </Group>
      <Group title="Links & comments" priority={20} icon={<LinkIcon />}>
        <Btn big label="Link" icon={<LinkIcon />} onClick={menu.onInsertLink} disabled={!menu.onInsertLink} />
        <Btn big label="Comment" icon={<CommentIcon />} onClick={menu.onInsertComment} disabled={!menu.onInsertComment} />
        <Btn big label="From app" icon={<AccountTreeIcon />} onClick={menu.onInsertFromApp} disabled={!menu.onInsertFromApp} />
      </Group>
      <Group title="Functions" priority={60} icon={<FunctionsIcon />}>
        <Btn big label="Function" icon={<FunctionsIcon />} onClick={menu.onInsertFunction} disabled={!menu.onInsertFunction} />
      </Group>
      <Group title="Page" priority={10} icon={<FlagIcon />}>
        <MenuBtn big label="Page break" icon={<FlagIcon />}>
          <MenuItem onClick={() => menu.onPageBreak?.("insert")}>Insert page break</MenuItem>
          <MenuItem onClick={() => menu.onPageBreak?.("remove")}>Remove page break</MenuItem>
          <MenuItem onClick={() => menu.onPageBreak?.("reset")}>Reset all page breaks</MenuItem>
        </MenuBtn>
      </Group>
    </>
  );

  const layout = (
    <>
      <Group title="Page setup" priority={50} icon={<PrintIcon />}>
        <Btn big label="Page setup & print" icon={<PrintIcon />} onClick={menu.actions.print} disabled={!menu.actions.print} />
      </Group>
      <Group title="Sheet options" priority={40} icon={<BorderAllIcon />}>
        <Btn label="Gridlines" icon={<BorderAllIcon />} active={view?.showGridLines} onClick={() => menu.onView?.({ showGridLines: !view?.showGridLines })} />
        <Btn label="Headings" icon={<LabelIcon />} active={view?.showHeadings} onClick={() => menu.onView?.({ showHeadings: !view?.showHeadings })} />
        <Btn big label="Page break preview" icon={<FlagIcon />} active={view?.pageBreakPreview} onClick={() => menu.onView?.({ pageBreakPreview: !view?.pageBreakPreview })} />
      </Group>
    </>
  );

  const formulas = (
    <>
      <Group title="Function library" priority={60} icon={<FunctionsIcon />}>
        <Btn big label="Insert function" icon={<FunctionsIcon />} onClick={menu.onInsertFunction} disabled={!menu.onInsertFunction} />
        <Btn big label="AutoSum" icon={<FunctionsIcon />} onClick={onAutoSum} />
      </Group>
      <Group title="Defined names" priority={20} icon={<LabelIcon />}>
        <Btn big label="Name manager" icon={<LabelIcon />} onClick={menu.onNamedRanges} />
      </Group>
      <Group title="Formula auditing" priority={30} icon={<AccountTreeIcon />}>
        <Btn big label="Trace precedents" icon={<AccountTreeIcon />} onClick={() => menu.onTrace?.("precedents")} disabled={!menu.onTrace} />
        <Btn big label="Trace dependents" icon={<AccountTreeIcon />} onClick={() => menu.onTrace?.("dependents")} disabled={!menu.onTrace} />
        <Btn label="Remove arrows" icon={<FormatClearIcon />} onClick={() => menu.onTrace?.("clear")} disabled={!menu.onTrace} />
        <Btn big label="Show formulas" icon={<FunctionsIcon />} active={view?.showFormulas} onClick={() => menu.onView?.({ showFormulas: !view?.showFormulas })} />
      </Group>
      <Group title="Calculation" priority={10} icon={<FlagIcon />}>
        <Btn big label="Goal seek" icon={<FlagIcon />} onClick={menu.onGoalSeek} disabled={!menu.onGoalSeek} />
        <Btn big label="Solver-lite" icon={<FlagIcon />} onClick={() => onWhatIf?.("solver")} disabled={!onWhatIf} />
      </Group>
    </>
  );

  const data = (
    <>
      <Group title="Sort & filter" priority={60} icon={<SortByAlphaIcon />}>
        <Btn big label="Sort A→Z" icon={<SortByAlphaIcon />} onClick={run((w) => sortRange(w, true))} />
        <Btn big label="Sort Z→A" icon={<SortByAlphaIcon />} onClick={run((w) => sortRange(w, false))} />
        <Btn big label="Sort range…" icon={<SortByAlphaIcon />} onClick={menu.onSortDialog} disabled={!menu.onSortDialog} />
        <Btn big label="Filter" icon={<FilterAltIcon />} onClick={run((w) => toggleFilter(w))} />
        <Btn big label="Filter column…" icon={<FilterAltIcon />} onClick={menu.onFilterColumn} disabled={!menu.onFilterColumn} />
      </Group>
      <Group title="Data tools" priority={40} icon={<RuleIcon />}>
        <Btn big label="Text to columns" icon={<DataArraySharpIcon />} onClick={run((w) => splitTextToColumns(w))} />
        <Btn big label="Remove duplicates" icon={<RuleIcon />} onClick={menu.onRemoveDuplicates} disabled={!menu.onRemoveDuplicates} />
        <Btn big label="Trim whitespace" icon={<FormatClearIcon />} onClick={menu.onTrimWhitespace} disabled={!menu.onTrimWhitespace} />
        <Btn big label="Data validation" icon={<RuleIcon />} onClick={menu.onDataValidation} />
        <Btn big label="Circle invalid" icon={<RuleIcon />} onClick={menu.onCircleInvalid} disabled={!menu.onCircleInvalid} />
      </Group>
      <Group title="Analysis" priority={20} icon={<PivotTableChartIcon />}>
        <Btn big label="PivotTable" icon={<PivotTableChartIcon />} onClick={menu.onInsertPivot} />
        <MenuBtn big label="What-if analysis" icon={<FlagIcon />}>
          <MenuItem onClick={() => onWhatIf?.("goalseek")}>Goal seek…</MenuItem>
          <MenuItem onClick={() => onWhatIf?.("datatable")}>Data table…</MenuItem>
          <MenuItem onClick={() => onWhatIf?.("scenarios")}>Scenario manager…</MenuItem>
          <MenuItem onClick={() => onWhatIf?.("solver")}>Solver-lite…</MenuItem>
        </MenuBtn>
        <Btn big label="Summary statistics" icon={<FunctionsIcon />} onClick={onSummaryStats} disabled={!onSummaryStats} />
      </Group>
      <Group title="Outline" priority={30} icon={<AccountTreeIcon />}>
        <MenuBtn big label="Group" icon={<AccountTreeIcon />} width={240}>
          <OutlineMenuItems getWb={menu.getWb} />
        </MenuBtn>
      </Group>
    </>
  );

  const review = (
    <>
      <Group title="Comments" priority={30} icon={<CommentIcon />}>
        <Btn big label="New comment" icon={<CommentIcon />} onClick={menu.onInsertComment} disabled={!menu.onInsertComment} />
        <Btn big label="Show comments" icon={<CommentIcon />} onClick={menu.onShowComments} disabled={!menu.onShowComments} />
      </Group>
      <Group title="Protect" priority={20} icon={<LockIcon />}>
        <Btn big label="Protect sheet / range" icon={<LockIcon />} onClick={menu.onProtect} disabled={!menu.onProtect} />
      </Group>
      <Group title="History" priority={10} icon={<HistoryIcon />}>
        <Btn big label="Version history" icon={<HistoryIcon />} onClick={menu.actions.versionHistory} disabled={!menu.actions.versionHistory} />
      </Group>
    </>
  );

  const zoomSteps = [50, 75, 90, 100, 125, 150, 200];
  const viewTab = (
    <>
      <Group title="Freeze" priority={50} icon={<FlagIcon />}>
        <Btn big label="Freeze top row" icon={<FlagIcon />} onClick={run((w) => w.freeze("row", { row: 0, column: 0 }))} />
        <Btn big label="Freeze first column" icon={<FlagIcon />} onClick={run((w) => w.freeze("column", { row: 0, column: 0 }))} />
        <Btn big label="Freeze at cell" icon={<FlagIcon />} onClick={run((w) => w.freeze("both", { row: selRow(w)[0], column: selCol(w)[0] }))} />
        <Btn big label="Unfreeze" icon={<FlagIcon />} onClick={run((w) => w.freeze("both", { row: 0, column: 0 }))} />
      </Group>
      <Group title="Show" priority={40} icon={<BorderAllIcon />}>
        <Btn big label="Gridlines" icon={<BorderAllIcon />} active={view?.showGridLines} onClick={() => menu.onView?.({ showGridLines: !view?.showGridLines })} />
        <Btn big label="Headings" icon={<LabelIcon />} active={view?.showHeadings} onClick={() => menu.onView?.({ showHeadings: !view?.showHeadings })} />
        <Btn big label="Formulas" icon={<FunctionsIcon />} active={view?.showFormulas} onClick={() => menu.onView?.({ showFormulas: !view?.showFormulas })} />
      </Group>
      <Group title="Zoom" priority={30} icon={<SearchIcon />}>
        <Select size="sm" variant="plain" aria-label="Zoom" value={view?.zoom ?? 100} onChange={(_, v) => v && menu.onView?.({ zoom: Number(v) })} sx={{ minWidth: 84 }}>
          {zoomSteps.map((z) => (
            <Option key={z} value={z}>{z}%</Option>
          ))}
        </Select>
      </Group>
      <Group title="Window" priority={10} icon={<KeyboardIcon />}>
        <Btn big label="Keyboard shortcuts" icon={<KeyboardIcon />} onClick={menu.onShortcuts} disabled={!menu.onShortcuts} />
        <Btn big label="Classic menus" icon={<KeyboardIcon />} onClick={onClassic} />
      </Group>
    </>
  );

  const body: Record<string, ReactNode> = { home, insert, layout, formulas, data, review, view: viewTab };
  void currentSheet;

  return (
    <Box data-testid="sheet-ribbon" sx={{ bgcolor: "background.surface", borderBottom: "1px solid", borderColor: "divider" }}>
      <Box role="tablist" aria-label="Ribbon tabs" sx={{ display: "flex", alignItems: "center", gap: 0.25, px: 1, overflowX: "auto", scrollbarWidth: "none" }}>
        <Tooltip title="Undo (Ctrl+Z)" size="sm"><Box component="button" aria-label="Undo" onClick={run((w) => w.handleUndo())} sx={undoSx}><UndoIcon sx={{ fontSize: 18 }} /></Box></Tooltip>
        <Tooltip title="Redo (Ctrl+Y)" size="sm"><Box component="button" aria-label="Redo" onClick={run((w) => w.handleRedo())} sx={undoSx}><RedoIcon sx={{ fontSize: 18 }} /></Box></Tooltip>
        {tabs.map((t) => (
          <Box
            key={t.id}
            component="button"
            role="tab"
            aria-selected={tab === t.id}
            tabIndex={tab === t.id ? 0 : -1}
            onKeyDown={(e: any) => {
              const i = tabs.findIndex((x) => x.id === t.id);
              const n = e.key === "ArrowRight" ? tabs[(i + 1) % tabs.length] : e.key === "ArrowLeft" ? tabs[(i - 1 + tabs.length) % tabs.length] : null;
              if (!n) return;
              e.preventDefault();
              const list = e.currentTarget.parentElement as HTMLElement | null;
              setTab(n.id);
              window.setTimeout(() => (list?.querySelector('[aria-selected="true"]') as HTMLElement | null)?.focus(), 0);
            }}
            onClick={() => {
              setTab(t.id);
              if (collapsed) onCollapsedChange(false);
            }}
            sx={{
              border: "none",
              bgcolor: "transparent",
              color: tab === t.id ? "primary.solidBg" : "text.secondary",
              fontWeight: tab === t.id ? 700 : 500,
              borderBottom: "2px solid",
              borderColor: tab === t.id && !collapsed ? "primary.solidBg" : "transparent",
              px: 1.25,
              py: 0.5,
              cursor: "pointer",
              fontSize: 13,
              whiteSpace: "nowrap",
            }}
          >
            {t.label}
          </Box>
        ))}
        <Box sx={{ flex: 1 }} />
        <Tooltip title={collapsed ? "Expand the ribbon" : "Collapse the ribbon"} size="sm">
          <Box component="button" aria-label={collapsed ? "Expand the ribbon" : "Collapse the ribbon"} onClick={() => onCollapsedChange(!collapsed)} sx={undoSx}>
            {collapsed ? "▾" : "▴"}
          </Box>
        </Tooltip>
      </Box>
      {!collapsed && (
        <PanelScroller resetKey={tab}>{body[tab]}</PanelScroller>
      )}
    </Box>
  );
}

const undoSx = {
  border: "none",
  bgcolor: "transparent",
  color: "text.primary",
  cursor: "pointer",
  borderRadius: "4px",
  width: 28,
  height: 28,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  "&:hover": { bgcolor: "background.level1" },
} as const;

/**
 * Ribbon body. Groups that do not fit are never cut off silently: when the
 * content is wider than the screen a chevron button appears on that side and
 * scrolls the next group into view. Arrow keys move focus between tools.
 */
function PanelScroller({ children, resetKey }: { children: ReactNode; resetKey: string }) {
  const ref = useRef<HTMLDivElement | null>(null);
  const [edge, setEdge] = useState({ left: false, right: false });
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const measure = () => setEdge({ left: el.scrollLeft > 2, right: el.scrollLeft + el.clientWidth < el.scrollWidth - 2 });
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    el.addEventListener("scroll", measure, { passive: true });
    return () => {
      ro.disconnect();
      el.removeEventListener("scroll", measure);
    };
  }, [children]);
  const go = (d: 1 | -1) => ref.current?.scrollBy({ left: d * Math.max(200, (ref.current?.clientWidth ?? 600) * 0.6), behavior: "smooth" });
  const onKey = (e: any) => {
    if (e.key !== "ArrowRight" && e.key !== "ArrowLeft") return;
    if (e.target?.tagName === "INPUT" || e.target?.getAttribute?.("role") === "combobox") return;
    const items = Array.from(ref.current?.querySelectorAll<HTMLElement>('[data-rb="1"]:not(:disabled)') ?? []);
    const i = items.indexOf(e.target.closest('[data-rb="1"]'));
    if (i < 0) return;
    e.preventDefault();
    const n = items[(i + (e.key === "ArrowRight" ? 1 : -1) + items.length) % items.length];
    n?.focus();
    n?.scrollIntoView({ block: "nearest", inline: "nearest" });
  };
  const chev = (side: "left" | "right") => ({
    position: "absolute" as const,
    top: 0,
    bottom: 0,
    [side]: 0,
    width: 24,
    border: "none",
    cursor: "pointer",
    zIndex: 2,
    color: "text.primary",
    bgcolor: "background.surface",
    boxShadow: side === "left" ? "4px 0 6px -4px rgba(0,0,0,.25)" : "-4px 0 6px -4px rgba(0,0,0,.25)",
    "&:hover": { bgcolor: "background.level1" },
  });
  return (
    <Box sx={{ position: "relative" }}>
      {edge.left && (
        <Box component="button" type="button" aria-label="Scroll ribbon left" onClick={() => go(-1)} sx={chev("left")}>‹</Box>
      )}
      <ResponsiveRibbonBody scrollerRef={ref} resetKey={resetKey} onKeyDown={onKey} sx={{ py: 0.5, minHeight: 74 }}>
        {children}
      </ResponsiveRibbonBody>
      {edge.right && (
        <Box component="button" type="button" aria-label="Scroll ribbon right" onClick={() => go(1)} sx={chev("right")}>›</Box>
      )}
    </Box>
  );
}
