// The Docs ribbon shell: tab row (Home, Insert, Layout, References, Review,
// View, plus contextual Table and Object tabs), collapse control and mode
// switch. Each tab lives in its own file; every button calls an existing
// editor command or dialog.
import { useEffect, useState } from "react";
import { Box, Dropdown, IconButton, Menu, MenuButton, MenuItem, Tooltip } from "@mui/joy";
import type { Editor } from "@tiptap/react";
import ViewAgendaIcon from "@mui/icons-material/ViewAgenda";
import ViewDayIcon from "@mui/icons-material/ViewDay";
import type { EditorMode } from "./Toolbar";
import { ResponsiveRibbonBody } from "../../components/ribbon/ResponsiveRibbon";
import { selectedObject } from "./objectNodes";
import { tableCtx } from "./tables";
import { HomeTab } from "./RibbonHome";
import { InsertTab, ReferencesTab } from "./RibbonInsert";
import { LayoutTab, ViewTab } from "./RibbonLayout";
import { ReviewTab, TableTab, ObjectTab } from "./RibbonReview";
import type { RibbonProps } from "./ribbonShared";

export type { RibbonProps, RibbonTab } from "./ribbonShared";

const TABS = [
  { id: "home", label: "Home" },
  { id: "insert", label: "Insert" },
  { id: "layout", label: "Layout" },
  { id: "references", label: "References" },
  { id: "review", label: "Review" },
  { id: "view", label: "View" },
];

function useEditorTick(editor: Editor | null) {
  const [, setN] = useState(0);
  useEffect(() => {
    if (!editor) return;
    const f = () => setN((n) => n + 1);
    editor.on("selectionUpdate", f);
    editor.on("transaction", f);
    return () => {
      editor.off("selectionUpdate", f);
      editor.off("transaction", f);
    };
  }, [editor]);
}

function ModeChip({ mode, onChange }: { mode: EditorMode; onChange: (m: EditorMode) => void }) {
  const label = mode === "editing" ? "Editing" : mode === "suggesting" ? "Suggesting" : "Viewing";
  return (
    <Dropdown>
      <MenuButton size="sm" variant="plain" color={mode === "suggesting" ? "success" : "neutral"} data-testid="ribbon-mode" sx={{ fontWeight: 500, mr: 0.5 }}>
        {label}
      </MenuButton>
      <Menu size="sm" placement="bottom-end" sx={{ zIndex: 1300 }}>
        <MenuItem selected={mode === "editing"} onClick={() => onChange("editing")}>Editing: edit the document directly</MenuItem>
        <MenuItem selected={mode === "suggesting"} onClick={() => onChange("suggesting")} data-testid="ribbon-mode-suggesting">Suggesting: edits become suggestions</MenuItem>
        <MenuItem selected={mode === "viewing"} onClick={() => onChange("viewing")}>Viewing: read only</MenuItem>
      </Menu>
    </Dropdown>
  );
}

export function Ribbon(p: RibbonProps) {
  const { editor } = p;
  useEditorTick(editor);
  const inTable = !!editor && !!tableCtx(editor.state);
  const obj = !!editor && !!selectedObject(editor.state);
  const tabs = [...TABS, ...(inTable ? [{ id: "table", label: "Table" }] : []), ...(obj ? [{ id: "object", label: "Object" }] : [])];
  const active = (tabs.find((t) => t.id === p.tab) ?? tabs[0]).id;
  const [open, setOpen] = useState(false);
  const showPanel = !p.collapsed || open;
  const pick = (id: string) => {
    p.onTab(id);
    setOpen(true);
  };

  return (
    <Box data-testid="ribbon" sx={{ bgcolor: "background.surface", borderBottom: "1px solid", borderColor: "divider", position: "sticky", top: 0, zIndex: 10 }}>
      <Box role="tablist" aria-label="Ribbon tabs" sx={{ display: "flex", alignItems: "center", px: 1, gap: 0.25, overflowX: "auto" }}>
        {tabs.map((t) => (
          <Box
            key={t.id}
            role="tab"
            aria-selected={active === t.id}
            tabIndex={0}
            data-testid={`ribbon-tab-${t.id}`}
            onClick={() => pick(t.id)}
            onKeyDown={(e) => (e.key === "Enter" || e.key === " ") && pick(t.id)}
            sx={{
              px: 1.5,
              py: 0.75,
              fontSize: 13,
              cursor: "pointer",
              userSelect: "none",
              whiteSpace: "nowrap",
              borderBottom: "2px solid",
              borderColor: active === t.id && showPanel ? "primary.solidBg" : "transparent",
              fontWeight: active === t.id ? 600 : 400,
              color: t.id === "table" || t.id === "object" ? "primary.plainColor" : "text.primary",
              "&:hover": { bgcolor: "background.level1" },
            }}
          >
            {t.label}
          </Box>
        ))}
        <Box sx={{ flex: 1 }} />
        <ModeChip mode={p.mode} onChange={p.onModeChange} />
        <Tooltip title={p.collapsed ? "Pin the ribbon" : "Collapse the ribbon"} size="sm">
          <IconButton
            size="sm"
            variant="plain"
            aria-label={p.collapsed ? "Pin the ribbon" : "Collapse the ribbon"}
            data-testid="ribbon-collapse"
            onClick={() => {
              p.onCollapsed(!p.collapsed);
              setOpen(false);
            }}
          >
            {p.collapsed ? <ViewAgendaIcon /> : <ViewDayIcon />}
          </IconButton>
        </Tooltip>
      </Box>
      {showPanel && editor && (
        <ResponsiveRibbonBody
          resetKey={active}
          testId={`ribbon-panel-${active}`}
          rowProps={{
            onClick: (e: React.MouseEvent) => {
              if (p.collapsed && (e.target as HTMLElement).closest("button")) setTimeout(() => setOpen(false), 150);
            },
          }}
          sx={{ px: 0.5, minHeight: 86, borderTop: "1px solid", borderColor: "divider", bgcolor: "background.level1" }}
        >
          {active === "home" && <HomeTab {...p} editor={editor} />}
          {active === "insert" && <InsertTab {...p} editor={editor} />}
          {active === "layout" && <LayoutTab {...p} editor={editor} />}
          {active === "references" && <ReferencesTab {...p} editor={editor} />}
          {active === "review" && <ReviewTab {...p} editor={editor} />}
          {active === "view" && <ViewTab {...p} editor={editor} />}
          {active === "table" && <TableTab editor={editor} />}
          {active === "object" && <ObjectTab />}
        </ResponsiveRibbonBody>
      )}
    </Box>
  );
}
