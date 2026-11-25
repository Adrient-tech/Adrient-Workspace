// Shared title bar for the Sheets, Docs and Slides editors: back arrow, app
// icon, title, menu row on the left; save status, collaborators, assistant
// and share on the right. One layout so the three editors look the same and
// wrap the same way from 1440px down to a phone.
import type { ReactNode } from "react";
import { Box, Button, Chip, IconButton, Input, Sheet, Tooltip } from "@mui/joy";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import AutoAwesomeIcon from "@mui/icons-material/AutoAwesome";
import PersonAddIcon from "@mui/icons-material/PersonAdd";

export interface EditorHeaderProps {
  backLabel: string;
  onBack: () => void;
  icon: ReactNode;
  title: string;
  titleLabel: string;
  onTitle: (v: string) => void;
  onTitleCommit: () => void;
  /** Menu bar (File / Edit / ...). Rendered under the title and scrolls sideways on narrow screens. */
  menu: ReactNode;
  /** Controls on the right, in order. */
  children?: ReactNode;
  /** Extra nodes rendered once after the header (dialogs mounted by the menu). */
  extra?: ReactNode;
}

export function EditorHeader(p: EditorHeaderProps) {
  return (
    <Sheet
      component="header"
      variant="plain"
      data-testid="editor-header"
      sx={{ px: { xs: 1, md: 2 }, py: 0.5, bgcolor: "background.body", borderBottom: "1px solid", borderColor: "divider" }}
    >
      <Box sx={{ display: "flex", alignItems: "center", gap: { xs: 0.5, md: 1 } }}>
        <IconButton variant="plain" size="sm" aria-label={p.backLabel} onClick={p.onBack}>
          <ArrowBackIcon />
        </IconButton>
        <Box sx={{ display: "flex", flexShrink: 0 }}>{p.icon}</Box>
        <Box sx={{ minWidth: 0, flex: { xs: 1, lg: "0 1 auto" }, display: "flex", flexDirection: "column" }}>
          <Input
            value={p.title}
            variant="plain"
            size="sm"
            onChange={(e) => p.onTitle(e.target.value)}
            onBlur={p.onTitleCommit}
            onKeyDown={(e) => {
              if (e.key === "Enter") (e.target as HTMLInputElement).blur();
            }}
            sx={{
              "--Input-minHeight": "28px",
              "--Input-focusedThickness": "1px",
              fontSize: "1rem",
              fontWeight: 600,
              px: 0.5,
              bgcolor: "transparent",
              width: { xs: "100%", lg: 360 },
              maxWidth: "100%",
              "&:hover": { bgcolor: "background.level1" },
            }}
            slotProps={{ input: { "aria-label": p.titleLabel } }}
          />
          <Box sx={{ overflowX: { xs: "auto", lg: "visible" }, whiteSpace: "nowrap", scrollbarWidth: "none", "&::-webkit-scrollbar": { display: "none" } }}>
            {p.menu}
          </Box>
        </Box>
        <Box sx={{ flex: 1, display: { xs: "none", lg: "block" } }} />
        <Box sx={{ display: "flex", alignItems: "center", gap: { xs: 0.5, md: 1 }, flexShrink: 0 }}>{p.children}</Box>
      </Box>
      {p.extra}
    </Sheet>
  );
}

/** The one assistant button. Icon only on phones. */
export function AssistantButton({ open, onClick }: { open: boolean; onClick: () => void }) {
  return (
    <Tooltip title="AI Assistant" size="sm">
      <Box sx={{ display: "flex" }}>
        <Button
          size="sm"
          variant={open ? "solid" : "outlined"}
          startDecorator={<AutoAwesomeIcon fontSize="small" />}
          onClick={onClick}
          aria-label="AI Assistant"
          aria-pressed={open}
          data-testid="assistant-toggle"
          sx={{ display: { xs: "none", md: "inline-flex" } }}
        >
          Assistant
        </Button>
        <IconButton
          size="sm"
          variant={open ? "solid" : "outlined"}
          aria-label="AI Assistant"
          aria-pressed={open}
          onClick={onClick}
          sx={{ display: { xs: "inline-flex", md: "none" } }}
        >
          <AutoAwesomeIcon fontSize="small" />
        </IconButton>
      </Box>
    </Tooltip>
  );
}

/** Share button: label from sm up, icon on phones. */
export function ShareButton({ onClick }: { onClick: () => void }) {
  return (
    <>
      <Button size="sm" startDecorator={<PersonAddIcon />} onClick={onClick} data-testid="share-button" sx={{ display: { xs: "none", sm: "inline-flex" } }}>
        Share
      </Button>
      <IconButton size="sm" variant="solid" color="primary" aria-label="Share" onClick={onClick} sx={{ display: { xs: "inline-flex", sm: "none" } }}>
        <PersonAddIcon />
      </IconButton>
    </>
  );
}

export type ConnStatus = "connecting" | "live" | "offline";

/** Shows only when collaboration is NOT live; live is the normal state and needs no chip. */
export function ConnectionChip({ status }: { status: ConnStatus }) {
  if (status === "live") return null;
  return (
    <Tooltip
      size="sm"
      title={status === "offline" ? "Offline: your edits are kept here and sync when the connection returns" : "Connecting to the live session…"}
    >
      <Chip size="sm" variant="soft" color={status === "offline" ? "danger" : "warning"} data-testid="conn-chip">
        {status === "offline" ? "Offline" : "Connecting…"}
      </Chip>
    </Tooltip>
  );
}
