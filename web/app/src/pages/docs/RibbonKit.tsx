// Small building blocks for the Docs ribbon: a group with a caption, a
// button (icon + optional label, "big" = stacked), and a dropdown button.
import { Dropdown, IconButton, Menu, MenuButton, Tooltip } from "@mui/joy";
import ArrowDropDownIcon from "@mui/icons-material/ArrowDropDown";
import type { ReactNode } from "react";
import ContentPasteIcon from "@mui/icons-material/ContentPaste";
import FormatBoldIcon from "@mui/icons-material/FormatBold";
import FormatAlignLeftIcon from "@mui/icons-material/FormatAlignLeft";
import PaletteIcon from "@mui/icons-material/Palette";
import SearchIcon from "@mui/icons-material/Search";
import UndoIcon from "@mui/icons-material/Undo";
import TableChartIcon from "@mui/icons-material/TableChart";
import ImageIcon from "@mui/icons-material/Image";
import LinkIcon from "@mui/icons-material/Link";
import CommentIcon from "@mui/icons-material/Comment";
import DescriptionIcon from "@mui/icons-material/Description";
import { ResponsiveGroup, useRibbonCompact } from "../../components/ribbon/ResponsiveRibbon";

const GROUP_ICONS: Record<string, ReactNode> = {
  Clipboard: <ContentPasteIcon />,
  Font: <FormatBoldIcon />,
  Paragraph: <FormatAlignLeftIcon />,
  Styles: <PaletteIcon />,
  Editing: <SearchIcon />,
  Undo: <UndoIcon />,
  Tables: <TableChartIcon />,
  Illustrations: <ImageIcon />,
  Links: <LinkIcon />,
  Comments: <CommentIcon />,
  Zoom: <SearchIcon />,
  "Page setup": <DescriptionIcon />,
  "Header & footer": <DescriptionIcon />,
};

// Lower numbers collapse first when the ribbon is narrow; Clipboard, Font and Editing stay visible longest.
const GROUP_PRIORITY: Record<string, number> = { Editing: 90, Clipboard: 80, Font: 70, Paragraph: 60, Undo: 55 };

export function Group({ label, children }: { label: string; children: ReactNode }) {
  return (
    <ResponsiveGroup title={label} icon={GROUP_ICONS[label]} priority={GROUP_PRIORITY[label] ?? 50} maxWidth={520} sx={{ "&:last-of-type": { borderRight: 0 } }}>
      {children}
    </ResponsiveGroup>
  );
}

const bigSx = { flexDirection: "column", height: 58, minWidth: 52, px: 0.75, gap: 0.25, fontSize: 11, fontWeight: 400, lineHeight: 1.1, "& svg": { fontSize: 24 } };

export function RBtn({
  icon,
  label,
  onClick,
  active,
  big: bigWanted,
  disabled,
  testid,
  hideLabel,
}: {
  icon: ReactNode;
  label: string;
  onClick: () => void;
  active?: boolean;
  big?: boolean;
  disabled?: boolean;
  testid?: string;
  hideLabel?: boolean;
}) {
  const big = bigWanted && !useRibbonCompact();
  return (
    <Tooltip title={label} size="sm">
      <span>
        <IconButton
          size="sm"
          variant={active ? "soft" : "plain"}
          color={active ? "primary" : "neutral"}
          aria-label={label}
          aria-pressed={active === undefined ? undefined : active}
          disabled={disabled}
          data-testid={testid}
          onMouseDown={(e) => e.preventDefault()}
          onClick={onClick}
          sx={big ? bigSx : { "& svg": { fontSize: 20 } }}
        >
          {icon}
          {big && !hideLabel && <span>{label}</span>}
        </IconButton>
      </span>
    </Tooltip>
  );
}

/** RMenu: a ribbon button that opens a menu (children are MenuItems etc.). */
export function RMenu({
  icon,
  label,
  big: bigWanted,
  children,
  testid,
  open,
  onOpenChange,
  sx,
}: {
  icon: ReactNode;
  label: string;
  big?: boolean;
  children: ReactNode;
  testid?: string;
  open?: boolean;
  onOpenChange?: (o: boolean) => void;
  sx?: object;
}) {
  const big = bigWanted && !useRibbonCompact();
  return (
    <Dropdown open={open} onOpenChange={onOpenChange ? (_, o) => onOpenChange(o) : undefined}>
      <Tooltip title={label} size="sm">
        <MenuButton
          slots={{ root: IconButton }}
          slotProps={{
            root: {
              size: "sm",
              variant: "plain",
              color: "neutral",
              "aria-label": label,
              "data-testid": testid,
              onMouseDown: (e: React.MouseEvent) => e.preventDefault(),
              sx: big ? bigSx : { "& svg": { fontSize: 20 } },
            },
          }}
        >
          {icon}
          {big ? (
            <span>
              {label}
              <ArrowDropDownIcon sx={{ fontSize: "16px !important", verticalAlign: "middle" }} />
            </span>
          ) : (
            <ArrowDropDownIcon sx={{ fontSize: "16px !important", ml: -0.5 }} />
          )}
        </MenuButton>
      </Tooltip>
      <Menu size="sm" placement="bottom-start" sx={{ minWidth: 180, maxHeight: "70vh", overflowY: "auto", zIndex: 1300, ...sx }}>
        {children}
      </Menu>
    </Dropdown>
  );
}
