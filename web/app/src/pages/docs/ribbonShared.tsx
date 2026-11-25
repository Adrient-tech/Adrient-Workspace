// Shared types and helpers for the Docs ribbon tabs.
import { Box, Typography } from "@mui/joy";
import type { Editor } from "@tiptap/react";
import type { EditorMode } from "./Toolbar";
import type { DocActions } from "./MenuBar";

export type RibbonTab = "home" | "insert" | "layout" | "references" | "review" | "view" | "table" | "object";

export interface RibbonProps {
  editor: Editor | null;
  actions: DocActions;
  mode: EditorMode;
  onModeChange: (m: EditorMode) => void;
  tab: string;
  onTab: (t: string) => void;
  collapsed: boolean;
  onCollapsed: (c: boolean) => void;
  zoom: number;
  onZoom: (z: number) => void;
  ruler: boolean;
  onRuler: (on: boolean) => void;
  outline: boolean;
  thumbnails: boolean;
  pageNumbers: boolean;
  assistant: boolean;
  onAssistant?: () => void;
  onCheckSpell: (on: boolean) => void;
  voice?: { supported: boolean; active: boolean; toggle: () => void };
}

export type TabProps = RibbonProps & { editor: Editor };

export const swatch = (c: string, on: () => void) => (
  <Box
    key={c}
    role="button"
    aria-label={c}
    onClick={on}
    sx={{ width: 18, height: 18, borderRadius: "3px", bgcolor: c, cursor: "pointer", border: "1px solid", borderColor: "neutral.outlinedBorder", "&:hover": { transform: "scale(1.15)" } }}
  />
);

export const head = (t: string) => (
  <Typography level="body-xs" sx={{ px: 1.5, py: 0.5, opacity: 0.6 }}>
    {t}
  </Typography>
);
