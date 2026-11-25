// Ribbon tabs (Home … View). Every button calls an editor command; a tool the
// editor cannot do is not shown.
import type { ReactNode } from "react";
import { Box, Divider, MenuItem, Typography } from "@mui/joy";
import ContentPasteIcon from "@mui/icons-material/ContentPaste";
import ContentCutIcon from "@mui/icons-material/ContentCut";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import AddBoxOutlinedIcon from "@mui/icons-material/AddBoxOutlined";
import ViewQuiltOutlinedIcon from "@mui/icons-material/ViewQuiltOutlined";
import RestartAltIcon from "@mui/icons-material/RestartAlt";
import FormatBoldIcon from "@mui/icons-material/FormatBold";
import FormatItalicIcon from "@mui/icons-material/FormatItalic";
import FormatUnderlinedIcon from "@mui/icons-material/FormatUnderlined";
import StrikethroughSIcon from "@mui/icons-material/StrikethroughS";
import FormatListBulletedIcon from "@mui/icons-material/FormatListBulleted";
import FormatListNumberedIcon from "@mui/icons-material/FormatListNumbered";
import FormatAlignLeftIcon from "@mui/icons-material/FormatAlignLeft";
import FormatAlignCenterIcon from "@mui/icons-material/FormatAlignCenter";
import FormatAlignRightIcon from "@mui/icons-material/FormatAlignRight";
import InterestsIcon from "@mui/icons-material/Interests";
import TextFieldsIcon from "@mui/icons-material/TextFields";
import ImageIcon from "@mui/icons-material/Image";
import TableChartOutlinedIcon from "@mui/icons-material/TableChartOutlined";
import BarChartIcon from "@mui/icons-material/BarChart";
import AccountTreeOutlinedIcon from "@mui/icons-material/AccountTreeOutlined";
import VideocamOutlinedIcon from "@mui/icons-material/VideocamOutlined";
import AudiotrackOutlinedIcon from "@mui/icons-material/AudiotrackOutlined";
import TitleIcon from "@mui/icons-material/Title";
import EmojiSymbolsIcon from "@mui/icons-material/EmojiSymbols";
import AppsIcon from "@mui/icons-material/Apps";
import LinkIcon from "@mui/icons-material/Link";
import AddCommentOutlinedIcon from "@mui/icons-material/AddCommentOutlined";
import VerticalAlignBottomIcon from "@mui/icons-material/VerticalAlignBottom";
import SearchIcon from "@mui/icons-material/Search";
import PaletteOutlinedIcon from "@mui/icons-material/PaletteOutlined";
import ColorLensOutlinedIcon from "@mui/icons-material/ColorLensOutlined";
import WallpaperIcon from "@mui/icons-material/Wallpaper";
import AspectRatioIcon from "@mui/icons-material/AspectRatio";
import DashboardCustomizeOutlinedIcon from "@mui/icons-material/DashboardCustomizeOutlined";
import AnimationIcon from "@mui/icons-material/Animation";
import PlayArrowIcon from "@mui/icons-material/PlayArrow";
import SlideshowIcon from "@mui/icons-material/Slideshow";
import PresentToAllIcon from "@mui/icons-material/PresentToAll";
import TimerOutlinedIcon from "@mui/icons-material/TimerOutlined";
import LoopIcon from "@mui/icons-material/Loop";
import TvIcon from "@mui/icons-material/Tv";
import VisibilityOffOutlinedIcon from "@mui/icons-material/VisibilityOffOutlined";
import AccessibilityNewIcon from "@mui/icons-material/AccessibilityNew";
import HistoryIcon from "@mui/icons-material/History";
import CommentOutlinedIcon from "@mui/icons-material/CommentOutlined";
import ViewModuleIcon from "@mui/icons-material/ViewModule";
import ViewAgendaOutlinedIcon from "@mui/icons-material/ViewAgendaOutlined";
import NotesIcon from "@mui/icons-material/Notes";
import ZoomInIcon from "@mui/icons-material/ZoomIn";
import ZoomOutIcon from "@mui/icons-material/ZoomOut";
import FitScreenIcon from "@mui/icons-material/FitScreen";
import GridOnIcon from "@mui/icons-material/GridOn";
import DarkModeOutlinedIcon from "@mui/icons-material/DarkModeOutlined";
import LightModeOutlinedIcon from "@mui/icons-material/LightModeOutlined";
import AutoAwesomeIcon from "@mui/icons-material/AutoAwesome";
import FlipToFrontIcon from "@mui/icons-material/FlipToFront";
import GroupWorkOutlinedIcon from "@mui/icons-material/GroupWorkOutlined";
import AlignHorizontalLeftIcon from "@mui/icons-material/AlignHorizontalLeft";
import ViewStreamOutlinedIcon from "@mui/icons-material/ViewStreamOutlined";
import ShowChartIcon from "@mui/icons-material/ShowChart";
import CallMadeIcon from "@mui/icons-material/CallMade";
import SwapHorizIcon from "@mui/icons-material/SwapHoriz";
import SubdirectoryArrowRightIcon from "@mui/icons-material/SubdirectoryArrowRight";
import GestureIcon from "@mui/icons-material/Gesture";
import { RibbonBtn, RibbonGroup, RibbonMenu } from "./RibbonParts";
import { ShapeGallery } from "./ShapeGallery";
import { TableSizePicker } from "./TableControls";
import { LayoutGrid, ThemePalette } from "./DesignDialogs";
import { TextFormatControls } from "./TextFormatControls";
import { ShapeFormatControls } from "./ShapeFormatControls";
import { DIAGRAM_LAYOUTS } from "./diagrams";
import { TRANSITION_DEFS, slideTransition, transitionDef } from "./transitions";
import { isShape, type AnimClass, type AnimKind, type DeckDoc, type Slide, type SlideElement } from "./model";
import { layoutsOf } from "./layouts";
import { themeOf } from "./theme";
import { formatDuration } from "./showTiming";
import type { SlideActions } from "./SlideMenuBar";

export interface RibbonCtx {
  actions: SlideActions;
  doc: DeckDoc;
  slide: Slide | undefined;
  selected: SlideElement | undefined;
  updateSelected: (patch: Partial<SlideElement>) => void;
  setFill: (hex: string) => void;
  setFillRef: (ref: string) => void;
  insertTableSized: (rows: number, cols: number) => void;
  setTransition: (patch: Partial<Slide>) => void;
  applyTransitionAll: () => void;
  addAnimation: (cls: AnimClass, kind: AnimKind) => void;
  openIcons: () => void;
  openMaster: () => void;
  view: "normal" | "sorter";
  setView: (v: "normal" | "sorter") => void;
  notesOn: boolean;
  commentsOn: boolean;
  dark: boolean;
  toggleDark: () => void;
  assistantOn: boolean;
  toggleAssistant: () => void;
  hasAssistant: boolean;
  openHistory?: () => void;
}

export type RibbonTabId = "home" | "insert" | "draw" | "design" | "transitions" | "animations" | "slideshow" | "review" | "view";

const ANIM_QUICK: { cls: AnimClass; label: string; kinds: [AnimKind, string][] }[] = [
  { cls: "entr", label: "Entrance", kinds: [["appear", "Appear"], ["fade", "Fade"], ["fly", "Fly in"], ["float", "Float in"], ["wipe", "Wipe"], ["zoom", "Zoom"]] },
  { cls: "emph", label: "Emphasis", kinds: [["pulse", "Pulse"], ["colorPulse", "Colour pulse"], ["spin", "Spin"], ["grow", "Grow/shrink"], ["teeter", "Teeter"]] },
  { cls: "exit", label: "Exit", kinds: [["appear", "Disappear"], ["fade", "Fade out"], ["fly", "Fly out"], ["wipe", "Wipe"], ["zoom", "Zoom out"]] },
  { cls: "path", label: "Motion paths", kinds: [["pathLine", "Line"], ["pathArc", "Arc"], ["pathCircle", "Circle"], ["pathZigzag", "Zigzag"]] },
];

function Sep() {
  return <Divider orientation="vertical" sx={{ mx: 0.25, alignSelf: "stretch" }} />;
}

export function RibbonPanel({ tab, ctx }: { tab: RibbonTabId; ctx: RibbonCtx }): ReactNode {
  switch (tab) {
    case "home":
      return <HomePanel ctx={ctx} />;
    case "insert":
      return <InsertPanel ctx={ctx} />;
    case "draw":
      return <DrawPanel ctx={ctx} />;
    case "design":
      return <DesignPanel ctx={ctx} />;
    case "transitions":
      return <TransitionsPanel ctx={ctx} />;
    case "animations":
      return <AnimationsPanel ctx={ctx} />;
    case "slideshow":
      return <ShowPanel ctx={ctx} />;
    case "review":
      return <ReviewPanel ctx={ctx} />;
    case "view":
      return <ViewPanel ctx={ctx} />;
  }
}

function HomePanel({ ctx }: { ctx: RibbonCtx }) {
  const { actions: a, selected, doc } = ctx;
  const has = a.selection.count > 0;
  return (
    <>
      <RibbonGroup label="Clipboard">
        <RibbonBtn icon={<ContentPasteIcon />} label="Paste" onClick={a.paste} />
        <Box sx={{ display: "flex", flexDirection: "column" }}>
          <RibbonBtn small icon={<ContentCutIcon />} label="Cut" disabled={!has} onClick={() => document.execCommand("cut")} />
          <RibbonBtn small icon={<ContentCopyIcon />} label="Copy" disabled={!has} onClick={() => document.execCommand("copy")} />
        </Box>
      </RibbonGroup>
      <RibbonGroup label="Slides">
        <RibbonBtn icon={<AddBoxOutlinedIcon />} label="New slide" onClick={a.newSlide} testId="ribbon-new-slide" />
        <RibbonMenu icon={<ViewQuiltOutlinedIcon />} label="Layout" testId="ribbon-layout">
          <LayoutGrid layouts={layoutsOf(doc)} current={a.currentLayout} onPick={a.applyLayout} />
        </RibbonMenu>
        <Box sx={{ display: "flex", flexDirection: "column" }}>
          <RibbonBtn small icon={<RestartAltIcon />} label="Reset" disabled={!a.currentLayout} onClick={a.resetSlide} />
          <RibbonBtn small icon={<ViewStreamOutlinedIcon />} label="Section" onClick={a.addSection} disabled={!a.addSection} testId="ribbon-section" />
        </Box>
      </RibbonGroup>
      <RibbonGroup label="Font">
        <Box sx={{ display: "flex", flexDirection: "column", gap: 0.25 }}>
          <Box sx={{ display: "flex", alignItems: "center", flexWrap: "wrap" }}>
            <TextFormatControls el={selected} cmd={a.text} />
          </Box>
          <Box sx={{ display: "flex", gap: 0.25 }}>
            <RibbonBtn small icon={<FormatBoldIcon />} label="" title="Bold" disabled={!has} active={!!selected?.bold} onClick={() => a.toggle("bold")} />
            <RibbonBtn small icon={<FormatItalicIcon />} label="" title="Italic" disabled={!has} active={!!selected?.italic} onClick={() => a.toggle("italic")} />
            <RibbonBtn small icon={<FormatUnderlinedIcon />} label="" title="Underline" disabled={!has} active={!!selected?.underline} onClick={() => a.toggle("underline")} />
            <RibbonBtn small icon={<StrikethroughSIcon />} label="" title="Strikethrough" disabled={!has} onClick={() => a.toggle("strike")} />
          </Box>
        </Box>
      </RibbonGroup>
      <RibbonGroup label="Paragraph">
        <RibbonBtn small icon={<FormatListBulletedIcon />} label="" title="Bulleted list" disabled={!has} onClick={() => a.setList("bullet")} />
        <RibbonBtn small icon={<FormatListNumberedIcon />} label="" title="Numbered list" disabled={!has} onClick={() => a.setList("number")} />
        <Sep />
        <RibbonBtn small icon={<FormatAlignLeftIcon />} label="" title="Align left" disabled={!has} active={selected?.align === "left"} onClick={() => a.setAlign("left")} />
        <RibbonBtn small icon={<FormatAlignCenterIcon />} label="" title="Align centre" disabled={!has} active={selected?.align === "center"} onClick={() => a.setAlign("center")} />
        <RibbonBtn small icon={<FormatAlignRightIcon />} label="" title="Align right" disabled={!has} active={selected?.align === "right"} onClick={() => a.setAlign("right")} />
      </RibbonGroup>
      <RibbonGroup label="Drawing">
        <RibbonMenu icon={<InterestsIcon />} label="Shapes" testId="ribbon-shapes">
          <ShapeGallery onPick={a.drawShape} />
        </RibbonMenu>
        {selected && isShape(selected.type) && (
          <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
            <input
              type="color"
              aria-label="Fill colour"
              title="Fill colour"
              value={selected.fill && /^#[0-9a-f]{6}/i.test(selected.fill) ? selected.fill.slice(0, 7) : "#4285f4"}
              onChange={(e) => ctx.setFill(e.target.value)}
            />
            <RibbonMenu small icon={<PaletteOutlinedIcon />} label="Theme">
              <ThemePalette theme={themeOf(doc)} onPick={(ref) => ctx.setFillRef(ref)} />
            </RibbonMenu>
          </Box>
        )}
        <ShapeFormatControls el={selected} onChange={ctx.updateSelected} />
      </RibbonGroup>
      <RibbonGroup label="Arrange">
        <RibbonMenu icon={<FlipToFrontIcon />} label="Order" disabled={!has}>
          <MenuItem onClick={() => a.arrange("front")}>Bring to front</MenuItem>
          <MenuItem onClick={() => a.arrange("forward")}>Bring forward</MenuItem>
          <MenuItem onClick={() => a.arrange("backward")}>Send backward</MenuItem>
          <MenuItem onClick={() => a.arrange("back")}>Send to back</MenuItem>
        </RibbonMenu>
        <RibbonMenu icon={<AlignHorizontalLeftIcon />} label="Align" disabled={!has}>
          {(["left", "center", "right", "top", "middle", "bottom"] as const).map((h) => (
            <MenuItem key={h} onClick={() => a.align(h)}>
              Align {h}
            </MenuItem>
          ))}
          <MenuItem disabled={a.selection.count < 3} onClick={() => a.distribute("horizontal")}>Distribute horizontally</MenuItem>
          <MenuItem disabled={a.selection.count < 3} onClick={() => a.distribute("vertical")}>Distribute vertically</MenuItem>
        </RibbonMenu>
        <Box sx={{ display: "flex", flexDirection: "column" }}>
          <RibbonBtn small icon={<GroupWorkOutlinedIcon />} label="Group" disabled={!a.selection.canGroup} onClick={a.group} />
          <RibbonBtn small icon={<GroupWorkOutlinedIcon />} label="Ungroup" disabled={!a.selection.canUngroup} onClick={a.ungroup} />
        </Box>
      </RibbonGroup>
      <RibbonGroup label="Editing">
        <RibbonBtn icon={<SearchIcon />} label="Find" onClick={a.text.findReplace} />
      </RibbonGroup>
    </>
  );
}

function InsertPanel({ ctx }: { ctx: RibbonCtx }) {
  const { actions: a } = ctx;
  return (
    <>
      <RibbonGroup label="Slides">
        <RibbonBtn icon={<AddBoxOutlinedIcon />} label="New slide" onClick={a.newSlide} />
      </RibbonGroup>
      <RibbonGroup label="Tables">
        <RibbonMenu icon={<TableChartOutlinedIcon />} label="Table" testId="ribbon-table">
          <TableSizePicker onPick={ctx.insertTableSized} />
        </RibbonMenu>
      </RibbonGroup>
      <RibbonGroup label="Images">
        <RibbonBtn icon={<ImageIcon />} label="Picture" onClick={a.insertImageFile} testId="ribbon-picture" />
        <RibbonBtn icon={<EmojiSymbolsIcon />} label="Icons" onClick={ctx.openIcons} testId="ribbon-icons" />
      </RibbonGroup>
      <RibbonGroup label="Illustrations">
        <RibbonMenu icon={<InterestsIcon />} label="Shapes">
          <ShapeGallery onPick={a.drawShape} />
        </RibbonMenu>
        <RibbonMenu icon={<BarChartIcon />} label="Chart" testId="ribbon-chart">
          <MenuItem onClick={() => a.objects.insertChart("column")}>Column chart</MenuItem>
          <MenuItem onClick={() => a.objects.insertChart("bar")}>Bar chart</MenuItem>
          <MenuItem onClick={() => a.objects.insertChart("line")}>Line chart</MenuItem>
          <MenuItem onClick={() => a.objects.insertChart("pie")}>Pie chart</MenuItem>
        </RibbonMenu>
        <RibbonMenu icon={<AccountTreeOutlinedIcon />} label="Diagram" testId="ribbon-diagram">
          {DIAGRAM_LAYOUTS.map((l) => (
            <MenuItem key={l.value} onClick={() => a.objects.insertDiagram(l.value)}>
              {l.label}
            </MenuItem>
          ))}
        </RibbonMenu>
      </RibbonGroup>
      <RibbonGroup label="Text">
        <RibbonBtn icon={<TextFieldsIcon />} label="Text box" onClick={() => a.insert("text")} />
        <RibbonBtn icon={<TitleIcon />} label="WordArt" onClick={a.objects.insertWordArt} />
        <RibbonBtn icon={<VerticalAlignBottomIcon />} label="Header & footer" onClick={a.headerFooter} />
        <RibbonBtn icon={<EmojiSymbolsIcon />} label="Symbol" onClick={a.text.specialChars} />
      </RibbonGroup>
      <RibbonGroup label="Media">
        <RibbonBtn icon={<VideocamOutlinedIcon />} label="Video" onClick={() => a.objects.insertMedia("video")} />
        <RibbonBtn icon={<AudiotrackOutlinedIcon />} label="Audio" onClick={() => a.objects.insertMedia("audio")} />
      </RibbonGroup>
      <RibbonGroup label="Links">
        <RibbonBtn icon={<LinkIcon />} label="Link" disabled={a.selection.count === 0} onClick={a.setLink} />
        <RibbonBtn icon={<AddCommentOutlinedIcon />} label="Comment" disabled={!a.comment} onClick={a.comment} />
      </RibbonGroup>
      <RibbonGroup label="Adrient">
        <RibbonBtn icon={<AppsIcon />} label="From Adrient" onClick={a.insertFromApp} testId="ribbon-from-adrient" title="Insert data from Sheets, Docs, CRM, Inventory, POS or Invoicing" />
      </RibbonGroup>
    </>
  );
}

function DrawPanel({ ctx }: { ctx: RibbonCtx }) {
  const { actions: a, selected } = ctx;
  return (
    <>
      <RibbonGroup label="Lines and connectors">
        <RibbonBtn icon={<ShowChartIcon />} label="Line" onClick={() => a.drawShape("straightConnector1")} testId="draw-line" />
        <RibbonBtn icon={<CallMadeIcon />} label="Arrow" onClick={() => a.drawShape("straightConnector1:arrow")} />
        <RibbonBtn icon={<SwapHorizIcon />} label="Double arrow" onClick={() => a.drawShape("straightConnector1:double")} />
        <RibbonBtn icon={<SubdirectoryArrowRightIcon />} label="Elbow" onClick={() => a.drawShape("bentConnector3:arrow")} />
        <RibbonBtn icon={<GestureIcon />} label="Curved" onClick={() => a.drawShape("curvedConnector3:arrow")} />
      </RibbonGroup>
      <RibbonGroup label="Shapes">
        <RibbonMenu icon={<InterestsIcon />} label="All shapes">
          <ShapeGallery onPick={a.drawShape} />
        </RibbonMenu>
        <RibbonBtn icon={<TextFieldsIcon />} label="Text box" onClick={() => a.insert("text")} />
      </RibbonGroup>
      <RibbonGroup label="Line style">
        <ShapeFormatControls el={selected} onChange={ctx.updateSelected} />
        {!selected && <Typography level="body-xs" sx={{ color: "text.tertiary", px: 1 }}>Select a shape or line to style it</Typography>}
      </RibbonGroup>
      <RibbonGroup label="Snap">
        <RibbonBtn small icon={<GridOnIcon />} label="Guides" active={a.snapGuides} onClick={a.toggleSnapGuides} />
        <RibbonBtn small icon={<GridOnIcon />} label="Grid" active={a.snapGrid} onClick={a.toggleSnapGrid} />
      </RibbonGroup>
    </>
  );
}

function DesignPanel({ ctx }: { ctx: RibbonCtx }) {
  const { actions: a, doc } = ctx;
  return (
    <>
      <RibbonGroup label="Themes">
        <RibbonBtn icon={<PaletteOutlinedIcon />} label="Themes" onClick={a.changeTheme} testId="ribbon-themes" />
        <RibbonBtn icon={<ColorLensOutlinedIcon />} label="Colours & fonts" onClick={a.editTheme} />
      </RibbonGroup>
      <RibbonGroup label="Master">
        <RibbonBtn icon={<DashboardCustomizeOutlinedIcon />} label="Slide master" onClick={ctx.openMaster} testId="ribbon-master" title="Edit slide masters and layouts" />
        <RibbonMenu icon={<ViewQuiltOutlinedIcon />} label="Layout">
          <LayoutGrid layouts={layoutsOf(doc)} current={a.currentLayout} onPick={a.applyLayout} />
        </RibbonMenu>
      </RibbonGroup>
      <RibbonGroup label="Customize">
        <RibbonBtn icon={<WallpaperIcon />} label="Background" onClick={a.setBackground} />
        <RibbonBtn icon={<AspectRatioIcon />} label="Slide size" onClick={a.pageSetup} />
        <RibbonBtn icon={<VerticalAlignBottomIcon />} label="Header & footer" onClick={a.headerFooter} />
      </RibbonGroup>
    </>
  );
}

function TransitionsPanel({ ctx }: { ctx: RibbonCtx }) {
  const { slide } = ctx;
  const tr = slideTransition(slide);
  const def = transitionDef(tr.type);
  return (
    <>
      <RibbonGroup label="Transition to this slide">
        <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.25, maxWidth: 420 }}>
          {TRANSITION_DEFS.map((d) => (
            <RibbonBtn key={d.type} small label={d.label} icon={<AnimationIcon />} active={tr.type === d.type} testId={`tr-${d.type}`} onClick={() => ctx.setTransition({ transition: d.type, transitionDir: d.options[0]?.value, transitionDur: undefined })} />
          ))}
        </Box>
      </RibbonGroup>
      <RibbonGroup label="Options">
        <select
          aria-label="Transition effect options"
          disabled={!def.options.length}
          value={tr.dir ?? def.options[0]?.value ?? ""}
          onChange={(e) => ctx.setTransition({ transitionDir: e.target.value })}
        >
          {def.options.length ? def.options.map((o) => <option key={o.value} value={o.value}>{o.label}</option>) : <option value="">No options</option>}
        </select>
        <label style={{ fontSize: 12, display: "flex", alignItems: "center", gap: 4 }}>
          Duration (s)
          <input
            type="number"
            min={0.1}
            max={10}
            step={0.1}
            aria-label="Transition duration in seconds"
            disabled={tr.type === "none" || tr.type === "cut"}
            value={(tr.dur / 1000).toFixed(1)}
            onChange={(e) => {
              const v = Number(e.target.value);
              if (v > 0) ctx.setTransition({ transitionDur: Math.round(Math.min(10, v) * 1000) });
            }}
            style={{ width: 56 }}
          />
        </label>
      </RibbonGroup>
      <RibbonGroup label="Advance slide">
        <label style={{ fontSize: 12, display: "flex", alignItems: "center", gap: 4 }}>
          <input type="checkbox" checked={slide?.advanceOnClick !== false} onChange={(e) => ctx.setTransition({ advanceOnClick: e.target.checked ? undefined : false })} /> On click
        </label>
        <label style={{ fontSize: 12, display: "flex", alignItems: "center", gap: 4 }}>
          <input
            type="checkbox"
            aria-label="Advance automatically"
            checked={slide?.advanceAfter !== undefined}
            onChange={(e) => ctx.setTransition({ advanceAfter: e.target.checked ? 5000 : undefined })}
          />
          After
          <input
            type="number"
            min={1}
            max={3600}
            aria-label="Seconds before advancing"
            disabled={slide?.advanceAfter === undefined}
            value={slide?.advanceAfter !== undefined ? Math.round(slide.advanceAfter / 1000) : 5}
            onChange={(e) => ctx.setTransition({ advanceAfter: Math.max(1, Math.min(3600, Number(e.target.value) || 5)) * 1000 })}
            style={{ width: 52 }}
          />
          s
        </label>
      </RibbonGroup>
      <RibbonGroup label="Preview">
        <RibbonBtn icon={<PlayArrowIcon />} label="Preview" onClick={ctx.actions.openMotion} title="Open the motion pane and preview" />
        <RibbonBtn icon={<ViewModuleIcon />} label="Apply to all" onClick={ctx.applyTransitionAll} testId="tr-apply-all" />
      </RibbonGroup>
    </>
  );
}

function AnimationsPanel({ ctx }: { ctx: RibbonCtx }) {
  const has = ctx.actions.selection.count > 0;
  return (
    <>
      {ANIM_QUICK.map((g) => (
        <RibbonGroup key={g.cls} label={g.label}>
          {g.kinds.map(([k, label]) => (
            <RibbonBtn key={k} small icon={<AnimationIcon />} label={label} disabled={!has} testId={`anim-${g.cls}-${k}`} onClick={() => ctx.addAnimation(g.cls, k)} />
          ))}
        </RibbonGroup>
      ))}
      <RibbonGroup label="Timeline">
        <RibbonBtn icon={<ViewAgendaOutlinedIcon />} label="Animation pane" onClick={ctx.actions.openAnimations} testId="ribbon-anim-pane" title="Order, start (click / with / after), delay, duration and preview" />
      </RibbonGroup>
      {!has && <Typography level="body-xs" sx={{ alignSelf: "center", color: "text.tertiary", px: 1 }}>Select an object to animate it</Typography>}
    </>
  );
}

function ShowPanel({ ctx }: { ctx: RibbonCtx }) {
  const { actions: a, slide } = ctx;
  return (
    <>
      <RibbonGroup label="Start slide show">
        <RibbonBtn icon={<SlideshowIcon />} label="From start" onClick={a.presentFromStart} testId="show-from-start" />
        <RibbonBtn icon={<PlayArrowIcon />} label="From current" onClick={a.present} />
        <RibbonBtn icon={<PresentToAllIcon />} label="Presenter view" onClick={a.presenterView} testId="show-presenter" />
      </RibbonGroup>
      <RibbonGroup label="Set up">
        <RibbonBtn icon={<VisibilityOffOutlinedIcon />} label={a.slideHidden ? "Show slide" : "Hide slide"} active={a.slideHidden} onClick={a.toggleSkip} />
        <RibbonBtn icon={<LoopIcon />} label="Loop" active={a.showLoop} onClick={a.toggleLoop} />
        {a.kiosk && <RibbonBtn icon={<TvIcon />} label="Kiosk" onClick={a.kiosk} title="Self-running show: loops, advances by timings (6 s when a slide has none)" />}
      </RibbonGroup>
      <RibbonGroup label="Timings">
        {a.rehearse && <RibbonBtn icon={<TimerOutlinedIcon />} label="Rehearse" onClick={a.rehearse} testId="show-rehearse" title="Run through the deck; the time on each slide becomes its timing" />}
        {a.clearTimings && <RibbonBtn small icon={<RestartAltIcon />} label="Clear timings" disabled={!a.timingsTotal} onClick={a.clearTimings} />}
        <Typography level="body-xs" sx={{ px: 1 }} data-testid="timings-total">
          {a.timingsTotal ? `Total ${formatDuration(a.timingsTotal)}` : "No timings"}
          {slide?.advanceAfter ? ` · this slide ${Math.round(slide.advanceAfter / 1000)} s` : ""}
        </Typography>
      </RibbonGroup>
    </>
  );
}

function ReviewPanel({ ctx }: { ctx: RibbonCtx }) {
  const { actions: a } = ctx;
  return (
    <>
      <RibbonGroup label="Comments">
        <RibbonBtn icon={<AddCommentOutlinedIcon />} label="New comment" disabled={!a.comment} onClick={a.comment} />
        <RibbonBtn icon={<CommentOutlinedIcon />} label="Show comments" active={ctx.commentsOn} disabled={!a.showComments} onClick={a.showComments} />
      </RibbonGroup>
      <RibbonGroup label="Proofing">
        <RibbonBtn icon={<SearchIcon />} label="Find & replace" onClick={a.text.findReplace} />
        <RibbonBtn icon={<AccessibilityNewIcon />} label="Check accessibility" onClick={a.openOutline} />
        <RibbonBtn icon={<NotesIcon />} label="Alt text" disabled={a.selection.count === 0} onClick={a.altText} />
      </RibbonGroup>
      <RibbonGroup label="History">
        <RibbonBtn icon={<HistoryIcon />} label="Version history" disabled={!ctx.openHistory} onClick={ctx.openHistory} testId="review-history" />
      </RibbonGroup>
    </>
  );
}

function ViewPanel({ ctx }: { ctx: RibbonCtx }) {
  const { actions: a } = ctx;
  return (
    <>
      <RibbonGroup label="Presentation views">
        <RibbonBtn icon={<SlideshowIcon />} label="Normal" active={ctx.view === "normal"} onClick={() => ctx.setView("normal")} testId="view-normal" />
        <RibbonBtn icon={<ViewModuleIcon />} label="Slide sorter" active={ctx.view === "sorter"} onClick={() => ctx.setView("sorter")} testId="view-sorter" />
        <RibbonBtn icon={<DashboardCustomizeOutlinedIcon />} label="Slide master" onClick={ctx.openMaster} />
        <RibbonBtn icon={<NotesIcon />} label="Notes" active={ctx.notesOn} onClick={a.toggleNotes} testId="view-notes" />
        <RibbonBtn icon={<AccessibilityNewIcon />} label="Outline" onClick={a.openOutline} />
      </RibbonGroup>
      <RibbonGroup label="Show">
        <RibbonBtn small icon={<GridOnIcon />} label="Guides" active={a.snapGuides} onClick={a.toggleSnapGuides} />
        <RibbonBtn small icon={<GridOnIcon />} label="Grid" active={a.snapGrid} onClick={a.toggleSnapGrid} />
        <RibbonBtn small icon={<CommentOutlinedIcon />} label="Comments" active={ctx.commentsOn} disabled={!a.showComments} onClick={a.showComments} />
      </RibbonGroup>
      <RibbonGroup label={`Zoom${a.zoomPct ? ` ${a.zoomPct}%` : " (fit)"}`}>
        <RibbonBtn icon={<ZoomInIcon />} label="Zoom in" onClick={() => a.zoom("in")} />
        <RibbonBtn icon={<ZoomOutIcon />} label="Zoom out" onClick={() => a.zoom("out")} />
        <RibbonBtn icon={<FitScreenIcon />} label="Fit" onClick={() => a.zoom("fit")} />
      </RibbonGroup>
      <RibbonGroup label="Appearance">
        <RibbonBtn icon={ctx.dark ? <LightModeOutlinedIcon /> : <DarkModeOutlinedIcon />} label={ctx.dark ? "Light" : "Dark"} onClick={ctx.toggleDark} testId="view-dark" />
        {ctx.hasAssistant && <RibbonBtn icon={<AutoAwesomeIcon />} label="Assistant" active={ctx.assistantOn} onClick={ctx.toggleAssistant} />}
      </RibbonGroup>
    </>
  );
}
