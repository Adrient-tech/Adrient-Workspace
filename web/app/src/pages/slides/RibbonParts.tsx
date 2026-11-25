// Small building blocks of the ribbon: tab strip, groups and tool buttons.
import type { ReactNode } from "react";
import { Box, Tooltip } from "@mui/joy";
import ContentPasteIcon from "@mui/icons-material/ContentPaste";
import SlideshowIcon from "@mui/icons-material/Slideshow";
import TextFieldsIcon from "@mui/icons-material/TextFields";
import FormatAlignLeftIcon from "@mui/icons-material/FormatAlignLeft";
import InterestsIcon from "@mui/icons-material/Interests";
import FlipToFrontIcon from "@mui/icons-material/FlipToFront";
import SearchIcon from "@mui/icons-material/Search";
import TableChartIcon from "@mui/icons-material/TableChart";
import ImageIcon from "@mui/icons-material/Image";
import PlayCircleOutlineIcon from "@mui/icons-material/PlayCircleOutline";
import LinkIcon from "@mui/icons-material/Link";
import { ResponsiveGroup, useRibbonCompact } from "../../components/ribbon/ResponsiveRibbon";

export interface RibbonTabDef {
  id: string;
  label: string;
}

export function RibbonTabs({ tabs, active, onPick, right }: { tabs: RibbonTabDef[]; active: string; onPick: (id: string) => void; right?: ReactNode }) {
  return (
    <Box role="tablist" aria-label="Ribbon tabs" sx={{ display: "flex", alignItems: "center", gap: 0.25, px: 1, borderBottom: "1px solid", borderColor: "divider", overflowX: "auto" }}>
      {tabs.map((t) => (
        <Box
          key={t.id}
          role="tab"
          id={`ribbon-tab-${t.id}`}
          aria-selected={t.id === active}
          aria-controls="ribbon-panel"
          tabIndex={t.id === active ? 0 : -1}
          data-testid={`ribbon-tab-${t.id}`}
          onClick={() => onPick(t.id)}
          onKeyDown={(e) => {
            const i = tabs.findIndex((x) => x.id === active);
            if (e.key === "ArrowRight") onPick(tabs[(i + 1) % tabs.length].id);
            else if (e.key === "ArrowLeft") onPick(tabs[(i - 1 + tabs.length) % tabs.length].id);
          }}
          sx={{
            px: 1.5,
            py: 0.5,
            fontSize: 13,
            cursor: "pointer",
            whiteSpace: "nowrap",
            borderBottom: "2px solid",
            borderColor: t.id === active ? "primary.500" : "transparent",
            color: t.id === active ? "primary.600" : "text.secondary",
            fontWeight: t.id === active ? 600 : 400,
            "&:hover": { bgcolor: "background.level1" },
            "&:focus-visible": { outline: "2px solid", outlineColor: "focusVisible", outlineOffset: -2 },
          }}
        >
          {t.label}
        </Box>
      ))}
      <Box sx={{ flex: 1 }} />
      {right}
    </Box>
  );
}

const GROUP_ICONS: Record<string, ReactNode> = {
  Clipboard: <ContentPasteIcon />,
  Slides: <SlideshowIcon />,
  Font: <TextFieldsIcon />,
  Paragraph: <FormatAlignLeftIcon />,
  Drawing: <InterestsIcon />,
  Arrange: <FlipToFrontIcon />,
  Editing: <SearchIcon />,
  Tables: <TableChartIcon />,
  Images: <ImageIcon />,
  Illustrations: <InterestsIcon />,
  Text: <TextFieldsIcon />,
  Media: <PlayCircleOutlineIcon />,
  Links: <LinkIcon />,
};

// Collapse order when the ribbon is narrow: lower numbers collapse first. Editing
// (which holds Find) and Clipboard stay visible longest; the rest go right to left.
const GROUP_PRIORITY: Record<string, number> = { Editing: 90, Clipboard: 80, Font: 70, Slides: 60 };

export function RibbonGroup({ label, children }: { label: string; children: ReactNode }) {
  return (
    <ResponsiveGroup title={label} icon={GROUP_ICONS[label]} priority={GROUP_PRIORITY[label] ?? 50} maxWidth={560} sx={{ px: 1, minHeight: 74 }}>
      {children}
    </ResponsiveGroup>
  );
}

export function RibbonBtn({
  icon,
  label,
  onClick,
  disabled,
  active,
  title,
  testId,
  small: smallWanted,
}: {
  icon: ReactNode;
  label: string;
  onClick?: () => void;
  disabled?: boolean;
  active?: boolean;
  title?: string;
  testId?: string;
  small?: boolean;
}) {
  // A button drawn small on purpose keeps its label; a large one shrinks to its icon when the ribbon is narrow.
  const compact = useRibbonCompact() && !smallWanted;
  const small = smallWanted || compact;
  return (
    <Tooltip title={title ?? label} enterDelay={600}>
      <span>
        <Box
          component="button"
          type="button"
          data-testid={testId}
          aria-label={title ?? label}
          aria-pressed={active === undefined ? undefined : active}
          disabled={disabled}
          onClick={onClick}
          sx={{
            display: "flex",
            flexDirection: small ? "row" : "column",
            alignItems: "center",
            justifyContent: "center",
            gap: small ? 0.5 : 0,
            minWidth: small ? 0 : 52,
            px: small ? 0.75 : 0.5,
            py: small ? 0.25 : 0.25,
            border: "none",
            borderRadius: "6px",
            bgcolor: active ? "primary.softBg" : "transparent",
            color: "text.primary",
            font: "inherit",
            fontSize: 12,
            cursor: "pointer",
            "& svg": { fontSize: small ? 18 : 24 },
            "&:hover:not(:disabled)": { bgcolor: active ? "primary.softHoverBg" : "background.level1" },
            "&:disabled": { opacity: 0.4, cursor: "default" },
            "&:focus-visible": { outline: "2px solid", outlineColor: "focusVisible" },
          }}
        >
          {icon}
          {!compact && <span style={{ lineHeight: "14px", whiteSpace: "nowrap" }}>{label}</span>}
        </Box>
      </span>
    </Tooltip>
  );
}

import { Dropdown, Menu, MenuButton } from "@mui/joy";

/** A ribbon button that opens a menu (or a gallery) below it. */
export function RibbonMenu({
  icon,
  label,
  children,
  disabled,
  testId,
  small: smallWanted,
  onOpenChange,
  open,
}: {
  icon: ReactNode;
  label: string;
  children: ReactNode;
  disabled?: boolean;
  testId?: string;
  small?: boolean;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
}) {
  const compact = useRibbonCompact() && !smallWanted;
  const small = smallWanted || compact;
  return (
    <Dropdown open={open} onOpenChange={onOpenChange ? (_, o) => onOpenChange(o) : undefined}>
      <MenuButton
        disabled={disabled}
        data-testid={testId}
        aria-label={label}
        variant="plain"
        color="neutral"
        sx={{
          display: "flex",
          flexDirection: small ? "row" : "column",
          gap: small ? 0.5 : 0,
          minWidth: small ? 0 : 52,
          minHeight: 0,
          px: small ? 0.75 : 0.5,
          py: 0.25,
          fontSize: 12,
          fontWeight: 400,
          borderRadius: "6px",
          "& svg": { fontSize: small ? 18 : 24 },
        }}
      >
        {icon}
        <span style={{ lineHeight: "14px", whiteSpace: "nowrap" }}>{compact ? "▾" : `${label} ▾`}</span>
      </MenuButton>
      <Menu size="sm" placement="bottom-start" sx={{ zIndex: 1300, p: 0.5, maxHeight: "70vh", overflowY: "auto" }}>
        {children}
      </Menu>
    </Dropdown>
  );
}
