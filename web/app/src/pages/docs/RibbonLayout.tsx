// Ribbon ▸ Layout and View tabs.
import { Box, MenuItem, Option, Select } from "@mui/joy";
import AspectRatioIcon from "@mui/icons-material/AspectRatio";
import CropRotateIcon from "@mui/icons-material/CropRotate";
import DescriptionIcon from "@mui/icons-material/Description";
import ViewColumnIcon from "@mui/icons-material/ViewColumn";
import InsertPageBreakIcon from "@mui/icons-material/InsertPageBreak";
import NumbersIcon from "@mui/icons-material/Numbers";
import LanguageIcon from "@mui/icons-material/Language";
import SettingsIcon from "@mui/icons-material/Settings";
import WaterDropIcon from "@mui/icons-material/WaterDrop";
import PaletteIcon from "@mui/icons-material/Palette";
import CropSquareIcon from "@mui/icons-material/CropSquare";
import FormatLineSpacingIcon from "@mui/icons-material/FormatLineSpacing";
import FormatIndentIncreaseIcon from "@mui/icons-material/FormatIndentIncrease";
import VerticalAlignTopIcon from "@mui/icons-material/VerticalAlignTop";
import ViewAgendaIcon from "@mui/icons-material/ViewAgenda";
import ViewDayIcon from "@mui/icons-material/ViewDay";
import PrintIcon from "@mui/icons-material/Print";
import StraightenIcon from "@mui/icons-material/Straighten";
import ViewListIcon from "@mui/icons-material/ViewList";
import NotesIcon from "@mui/icons-material/Notes";
import ZoomInIcon from "@mui/icons-material/ZoomIn";
import ZoomOutIcon from "@mui/icons-material/ZoomOut";
import DarkModeIcon from "@mui/icons-material/DarkMode";
import FullscreenIcon from "@mui/icons-material/Fullscreen";
import AutoAwesomeIcon from "@mui/icons-material/AutoAwesome";
import { Group, RBtn, RMenu } from "./RibbonKit";
import { openParagraphDialog } from "./ParagraphDialogs";
import { openLayoutDialog } from "./LayoutDialogs";
import { MARGIN_PRESETS, PAGE_SIZES, COLUMN_PRESETS, SECTION_START_LABEL, SECTION_STARTS, setOrientation, setPageSize } from "./sections";
import { getSettings, setDocSettings, setSectionProps } from "./pageLayout";
import { isNonPrinting, loadDarkDocument, toggleDarkDocument } from "./viewModes";
import { swatch, type TabProps } from "./ribbonShared";

export function LayoutTab({ editor: ed, actions }: TabProps) {
  const sec = (fn: Parameters<typeof setSectionProps>[1]) => () => setSectionProps(ed, fn);
  const pageBreak = () => void (ed.chain().focus().insertPageBreak().run() || ed.chain().focus().setPageBreak().run());
  return (
    <>
      <Group label="Page setup">
        <RMenu icon={<AspectRatioIcon />} label="Margins" big testid="rb-margins">
          {MARGIN_PRESETS.map((m) => (
            <MenuItem
              key={m.id}
              data-testid={`layout-margins-${m.id}`}
              onClick={() => {
                setSectionProps(ed, (s) => ({ ...s, margins: { ...s.margins, ...m.margins } }));
                if (!!m.mirror !== getSettings(ed).mirror) setDocSettings(ed, { mirror: !!m.mirror });
              }}
            >
              {m.name}
            </MenuItem>
          ))}
          <MenuItem onClick={actions.pageSetup}>Custom margins…</MenuItem>
        </RMenu>
        <RMenu icon={<CropRotateIcon />} label="Orientation" big>
          <MenuItem data-testid="layout-portrait" onClick={sec((s) => setOrientation(s, "portrait"))}>Portrait</MenuItem>
          <MenuItem data-testid="layout-landscape" onClick={sec((s) => setOrientation(s, "landscape"))}>Landscape</MenuItem>
        </RMenu>
        <RMenu icon={<DescriptionIcon />} label="Size" big>
          {PAGE_SIZES.slice(0, 6).map((s) => (
            <MenuItem key={s.id} onClick={sec((x) => setPageSize(x, s.w, s.h))}>{s.name}</MenuItem>
          ))}
          <MenuItem onClick={actions.pageSetup}>More sizes…</MenuItem>
        </RMenu>
        <RMenu icon={<ViewColumnIcon />} label="Columns" big>
          {COLUMN_PRESETS.map((c) => (
            <MenuItem key={c.id} data-testid={`layout-columns-${c.id}`} onClick={sec(c.apply)}>{c.name}</MenuItem>
          ))}
          <MenuItem onClick={() => openLayoutDialog("columns")}>More columns…</MenuItem>
        </RMenu>
        <RMenu icon={<InsertPageBreakIcon />} label="Breaks" big>
          <MenuItem onClick={pageBreak}>Page break</MenuItem>
          <MenuItem onClick={() => void ed.chain().focus().insertColumnBreak().run()}>Column break</MenuItem>
          {SECTION_STARTS.map((k) => (
            <MenuItem key={k} onClick={() => void ed.chain().focus().insertSectionBreak(k).run()}>Section break: {SECTION_START_LABEL[k].toLowerCase()}</MenuItem>
          ))}
        </RMenu>
        <RBtn icon={<NumbersIcon />} label="Line numbers" big onClick={() => openLayoutDialog("linenumbers")} />
        <RBtn icon={<LanguageIcon />} label="Hyphenation" big onClick={() => openLayoutDialog("hyphenation")} />
        <RBtn icon={<SettingsIcon />} label="Page setup" big onClick={actions.pageSetup} testid="rb-pagesetup" />
      </Group>
      <Group label="Page background">
        <RBtn icon={<WaterDropIcon />} label="Watermark" big onClick={() => openLayoutDialog("watermark")} />
        <RMenu icon={<PaletteIcon />} label="Page color" big>
          <Box sx={{ display: "flex", gap: 0.5, p: 1 }}>
            {["#ffffff", "#fff8e1", "#e8f5e9", "#e3f2fd", "#fce4ec", "#f3e5f5", "#eceff1"].map((c) =>
              swatch(c, () => setDocSettings(ed, { pageColor: c === "#ffffff" ? null : c })),
            )}
          </Box>
        </RMenu>
        <RBtn
          icon={<CropSquareIcon />}
          label="Page borders"
          big
          onClick={sec((s) => ({ ...s, borders: s.borders ? null : { style: "single", color: "#000000", width: 0.75, space: 24, offsetFrom: "text", display: "all" } }))}
        />
      </Group>
      <Group label="Paragraph">
        <RBtn icon={<FormatLineSpacingIcon />} label="Spacing" big onClick={actions.customSpacing} />
        <RBtn icon={<FormatIndentIncreaseIcon />} label="Indents" big onClick={() => openParagraphDialog("paragraph")} />
      </Group>
      <Group label="Header & footer">
        <RBtn icon={<VerticalAlignTopIcon />} label="Options" big onClick={() => openLayoutDialog("headerfooter")} testid="rb-hf-options" />
      </Group>
    </>
  );
}

export function ViewTab({ editor: ed, actions, zoom, onZoom, ruler, onRuler, outline, thumbnails, pageNumbers, assistant, onAssistant }: TabProps) {
  const pageless = getSettings(ed).pageless;
  return (
    <>
      <Group label="Views">
        <RBtn icon={<ViewDayIcon />} label="Print layout" big active={!pageless} onClick={() => setDocSettings(ed, { pageless: false })} testid="rb-print-layout" />
        <RBtn icon={<ViewAgendaIcon />} label="Pageless" big active={!!pageless} onClick={() => setDocSettings(ed, { pageless: true })} />
        <RBtn icon={<PrintIcon />} label="Print preview" big onClick={() => (actions.printPreview ?? (() => window.print()))()} testid="rb-print-preview" />
      </Group>
      <Group label="Show">
        <RBtn icon={<StraightenIcon />} label="Ruler" big active={ruler} onClick={() => onRuler(!ruler)} testid="rb-ruler" />
        <RBtn icon={<ViewListIcon />} label="Navigation" big active={outline} onClick={actions.toggleOutline} testid="rb-outline" />
        <RBtn icon={<ViewAgendaIcon />} label="Thumbnails" big active={thumbnails} onClick={() => actions.togglePageThumbnails?.()} />
        <RBtn icon={<NumbersIcon />} label="Page numbers" big active={pageNumbers} onClick={actions.togglePageNumbers} />
        <RBtn icon={<NotesIcon />} label="Non-printing" big active={isNonPrinting(ed)} onClick={() => ed.chain().focus().toggleNonPrinting().run()} />
      </Group>
      <Group label="Zoom">
        <RBtn icon={<ZoomOutIcon />} label="Zoom out" big onClick={() => onZoom(Math.max(50, zoom - 10))} testid="rb-zoom-out" />
        <Box sx={{ display: "flex", alignItems: "center", height: 58 }}>
          <Select size="sm" value={zoom} onChange={(_, v) => v && onZoom(v)} sx={{ minWidth: 84 }} aria-label="Zoom level" data-testid="rb-zoom">
            {[...new Set([50, 75, 90, 100, 125, 150, 200, zoom])].sort((a, b) => a - b).map((z) => (
              <Option key={z} value={z}>{z}%</Option>
            ))}
          </Select>
        </Box>
        <RBtn icon={<ZoomInIcon />} label="Zoom in" big onClick={() => onZoom(Math.min(200, zoom + 10))} testid="rb-zoom-in" />
      </Group>
      <Group label="Display">
        <RBtn icon={<DarkModeIcon />} label="Dark document" big active={loadDarkDocument()} onClick={() => toggleDarkDocument()} />
        <RBtn icon={<FullscreenIcon />} label="Full screen" big onClick={() => void (document.fullscreenElement ? document.exitFullscreen() : document.documentElement.requestFullscreen?.())} />
      </Group>
      <Group label="Assistant">
        <RBtn icon={<AutoAwesomeIcon />} label="AI Assistant" big active={assistant} onClick={() => onAssistant?.()} testid="rb-assistant" disabled={!onAssistant} />
      </Group>
    </>
  );
}
