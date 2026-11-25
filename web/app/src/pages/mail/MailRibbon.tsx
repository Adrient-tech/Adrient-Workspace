import ConfirmationNumberOutlinedIcon from "@mui/icons-material/ConfirmationNumberOutlined";
import { useState } from "react";
import { Box, Button, Chip, Divider, Dropdown, ListDivider, Menu, MenuButton, MenuItem, Tab, TabList, Tabs, Tooltip, Typography } from "@mui/joy";
import EditIcon from "@mui/icons-material/Edit";
import DeleteOutlineIcon from "@mui/icons-material/DeleteOutline";
import ArchiveOutlinedIcon from "@mui/icons-material/ArchiveOutlined";
import DriveFileMoveOutlinedIcon from "@mui/icons-material/DriveFileMoveOutlined";
import ReplyIcon from "@mui/icons-material/Reply";
import ReplyAllIcon from "@mui/icons-material/ReplyAll";
import ForwardIcon from "@mui/icons-material/Forward";
import LabelOutlinedIcon from "@mui/icons-material/LabelOutlined";
import FlagOutlinedIcon from "@mui/icons-material/FlagOutlined";
import FlagIcon from "@mui/icons-material/Flag";
import PushPinOutlinedIcon from "@mui/icons-material/PushPinOutlined";
import PushPinIcon from "@mui/icons-material/PushPin";
import AccessTimeIcon from "@mui/icons-material/AccessTime";
import MarkEmailReadIcon from "@mui/icons-material/MarkEmailRead";
import MarkEmailUnreadIcon from "@mui/icons-material/MarkEmailUnread";
import CleaningServicesOutlinedIcon from "@mui/icons-material/CleaningServicesOutlined";
import RuleIcon from "@mui/icons-material/Rule";
import PlayArrowIcon from "@mui/icons-material/PlayArrow";
import TaskAltIcon from "@mui/icons-material/TaskAlt";
import AddToDriveIcon from "@mui/icons-material/AddToDrive";
import BorderColorIcon from "@mui/icons-material/BorderColor";
import SpellcheckIcon from "@mui/icons-material/Spellcheck";
import ViewSidebarOutlinedIcon from "@mui/icons-material/ViewSidebarOutlined";
import ViewAgendaOutlinedIcon from "@mui/icons-material/ViewAgendaOutlined";
import DensityMediumIcon from "@mui/icons-material/DensityMedium";
import SortIcon from "@mui/icons-material/Sort";
import ForumOutlinedIcon from "@mui/icons-material/ForumOutlined";
import type { SortKey, QuickFilter } from "./search";

export type Density = "comfortable" | "compact";
export type PaneMode = "right" | "bottom" | "off";
export interface ViewPrefs {
  density: Density;
  pane: PaneMode;
  group: boolean;
  sort: SortKey;
}

export const SORTS: { id: SortKey; label: string }[] = [
  { id: "date-desc", label: "Newest on top" },
  { id: "date-asc", label: "Oldest on top" },
  { id: "unread-first", label: "Unread first" },
  { id: "from", label: "From (A–Z)" },
  { id: "subject", label: "Subject (A–Z)" },
];

export const QUICK_FILTERS: { id: QuickFilter; label: string }[] = [
  { id: "unread", label: "Unread" },
  { id: "tome", label: "To me" },
  { id: "flagged", label: "Flagged" },
  { id: "attachments", label: "Has attachments" },
  { id: "mentions", label: "Mentions" },
];

export const MOVE_TARGETS = [
  { id: "inbox", label: "Inbox" },
  { id: "archive", label: "Archive" },
  { id: "spam", label: "Spam" },
  { id: "trash", label: "Trash" },
];

export interface RibbonProps {
  hasTarget: boolean;
  canReply: boolean;
  folder: string;
  labels: { name: string; color: string }[];
  flagged: boolean;
  pinned: boolean;
  allRead: boolean;
  view: ViewPrefs;
  setView: (v: ViewPrefs) => void;
  filters: Set<QuickFilter>;
  toggleFilter: (f: QuickFilter) => void;
  spell: boolean;
  setSpell: (b: boolean) => void;
  onNew: () => void;
  onDelete: () => void;
  onArchive: () => void;
  onMove: (folder: string) => void;
  onReply: () => void;
  onReplyAll: () => void;
  onForward: () => void;
  onMakeTicket?: () => void;
  onLabel: (name: string) => void;
  onNewLabel: () => void;
  onFlag: () => void;
  onPin: () => void;
  onSnooze: (when: Date) => void;
  onMarkRead: () => void;
  onSweep: (mode: "sender-archive" | "sender-delete" | "older-delete") => void;
  onRules: () => void;
  onRunRules: () => void;
  onMakeTask: () => void;
  onAttachDrive: () => void;
  onSignatures: () => void;
}

const SNOOZES: { label: string; when: () => Date }[] = [
  { label: "Later today (3 hours)", when: () => new Date(Date.now() + 3 * 3600_000) },
  {
    label: "Tomorrow morning",
    when: () => {
      const d = new Date();
      d.setDate(d.getDate() + 1);
      d.setHours(8, 0, 0, 0);
      return d;
    },
  },
  {
    label: "Next week",
    when: () => {
      const d = new Date();
      d.setDate(d.getDate() + ((8 - d.getDay()) % 7 || 7));
      d.setHours(8, 0, 0, 0);
      return d;
    },
  },
];

function Cmd({
  icon,
  label,
  onClick,
  disabled,
  active,
  danger,
}: {
  icon: React.ReactNode;
  label: string;
  onClick?: () => void;
  disabled?: boolean;
  active?: boolean;
  danger?: boolean;
}) {
  return (
    <Tooltip title={label} placement="bottom" enterDelay={600}>
      <span>
        <Button
          size="sm"
          variant={active ? "soft" : "plain"}
          color={danger ? "danger" : active ? "primary" : "neutral"}
          disabled={disabled}
          onClick={onClick}
          startDecorator={icon}
          aria-label={label}
          sx={{ flexShrink: 0, whiteSpace: "nowrap", px: 1, "& .MuiButton-startDecorator": { mr: 0.5 } }}
        >
          <Box component="span" sx={{ display: { xs: "none", lg: "inline" } }}>
            {label}
          </Box>
        </Button>
      </span>
    </Tooltip>
  );
}

function MenuCmd({
  icon,
  label,
  disabled,
  children,
  active,
}: {
  icon: React.ReactNode;
  label: string;
  disabled?: boolean;
  active?: boolean;
  children: React.ReactNode;
}) {
  return (
    <Dropdown>
      <MenuButton
        disabled={disabled}
        size="sm"
        variant={active ? "soft" : "plain"}
        color="neutral"
        startDecorator={icon}
        aria-label={label}
        sx={{ flexShrink: 0, whiteSpace: "nowrap", px: 1 }}
      >
        <Box component="span" sx={{ display: { xs: "none", lg: "inline" } }}>
          {label}
        </Box>
      </MenuButton>
      <Menu size="sm" placement="bottom-start" sx={{ minWidth: 190, zIndex: 1500 }}>
        {children}
      </Menu>
    </Dropdown>
  );
}

const Sep = () => <Divider orientation="vertical" sx={{ mx: 0.5, my: 0.5, flexShrink: 0 }} />;

export function MailRibbon(p: RibbonProps) {
  const [tab, setTab] = useState<"home" | "view" | "insert" | "options">("home");
  const t = p.hasTarget;
  return (
    <Box sx={{ borderBottom: "1px solid", borderColor: "divider", bgcolor: "background.surface" }} data-testid="mail-ribbon">
      <Tabs value={tab} onChange={(_, v) => setTab(v as typeof tab)} size="sm" sx={{ bgcolor: "transparent" }}>
        <TabList disableUnderline sx={{ px: 1, gap: 0.5, "--ListItem-minHeight": "30px" }}>
          {(["home", "view", "insert", "options"] as const).map((k) => (
            <Tab key={k} value={k} disableIndicator variant={tab === k ? "soft" : "plain"} color={tab === k ? "primary" : "neutral"} sx={{ flex: "none", borderRadius: "6px" }}>
              {k[0].toUpperCase() + k.slice(1)}
            </Tab>
          ))}
        </TabList>
      </Tabs>
      <Box sx={{ display: "flex", alignItems: "center", gap: 0.25, px: 1, py: 0.5, overflowX: "auto", minHeight: 40 }}>
        {tab === "home" && (
          <>
            <Button size="sm" startDecorator={<EditIcon />} onClick={p.onNew} sx={{ flexShrink: 0, mr: 0.5 }}>
              New mail
            </Button>
            <Sep />
            <Cmd icon={<DeleteOutlineIcon />} label={p.folder === "trash" ? "Delete forever" : "Delete"} onClick={p.onDelete} disabled={!t} danger />
            <Cmd icon={<ArchiveOutlinedIcon />} label="Archive" onClick={p.onArchive} disabled={!t || p.folder === "archive"} />
            <MenuCmd icon={<CleaningServicesOutlinedIcon />} label="Sweep" disabled={!t}>
              <MenuItem onClick={() => p.onSweep("sender-archive")}>Archive all from this sender</MenuItem>
              <MenuItem onClick={() => p.onSweep("sender-delete")}>Delete all from this sender</MenuItem>
              <MenuItem onClick={() => p.onSweep("older-delete")}>Delete everything older than 10 days</MenuItem>
            </MenuCmd>
            <MenuCmd icon={<DriveFileMoveOutlinedIcon />} label="Move" disabled={!t}>
              {MOVE_TARGETS.filter((m) => m.id !== p.folder).map((m) => (
                <MenuItem key={m.id} onClick={() => p.onMove(m.id)}>
                  {m.label}
                </MenuItem>
              ))}
            </MenuCmd>
            <Sep />
            <Cmd icon={<ReplyIcon />} label="Reply" onClick={p.onReply} disabled={!p.canReply} />
            <Cmd icon={<ReplyAllIcon />} label="Reply all" onClick={p.onReplyAll} disabled={!p.canReply} />
            <Cmd icon={<ForwardIcon />} label="Forward" onClick={p.onForward} disabled={!p.canReply} />
            <Sep />
            <MenuCmd icon={<LabelOutlinedIcon />} label="Categories" disabled={!t}>
              {p.labels.map((l) => (
                <MenuItem key={l.name} onClick={() => p.onLabel(l.name)}>
                  <Box sx={{ width: 10, height: 10, borderRadius: "50%", bgcolor: l.color, mr: 1 }} />
                  {l.name}
                </MenuItem>
              ))}
              {p.labels.length > 0 && <ListDivider />}
              <MenuItem onClick={p.onNewLabel}>New category…</MenuItem>
            </MenuCmd>
            <Cmd icon={p.flagged ? <FlagIcon /> : <FlagOutlinedIcon />} label={p.flagged ? "Unflag" : "Flag"} onClick={p.onFlag} disabled={!t} active={p.flagged} />
            <Cmd icon={p.pinned ? <PushPinIcon /> : <PushPinOutlinedIcon />} label={p.pinned ? "Unpin" : "Pin"} onClick={p.onPin} disabled={!t} active={p.pinned} />
            <MenuCmd icon={<AccessTimeIcon />} label="Snooze" disabled={!t}>
              {SNOOZES.map((s) => (
                <MenuItem key={s.label} onClick={() => p.onSnooze(s.when())}>
                  {s.label}
                </MenuItem>
              ))}
            </MenuCmd>
            <Cmd icon={p.allRead ? <MarkEmailUnreadIcon /> : <MarkEmailReadIcon />} label={p.allRead ? "Mark unread" : "Mark read"} onClick={p.onMarkRead} disabled={!t} />
            <Cmd icon={<TaskAltIcon />} label="To Tasks" onClick={p.onMakeTask} disabled={!p.canReply} />
            {p.onMakeTicket && <Cmd icon={<ConfirmationNumberOutlinedIcon />} label="To Tickets" onClick={p.onMakeTicket} disabled={!p.canReply} />}
            <Sep />
            <MenuCmd icon={<RuleIcon />} label="Rules">
              <MenuItem onClick={p.onRules}>Manage rules…</MenuItem>
              <MenuItem onClick={p.onRunRules}>
                <PlayArrowIcon sx={{ mr: 1 }} /> Run rules now
              </MenuItem>
            </MenuCmd>
          </>
        )}
        {tab === "view" && (
          <>
            <MenuCmd icon={<DensityMediumIcon />} label={`Density: ${p.view.density}`}>
              {(["comfortable", "compact"] as Density[]).map((d) => (
                <MenuItem key={d} selected={p.view.density === d} onClick={() => p.setView({ ...p.view, density: d })}>
                  {d[0].toUpperCase() + d.slice(1)}
                </MenuItem>
              ))}
            </MenuCmd>
            <MenuCmd icon={p.view.pane === "bottom" ? <ViewAgendaOutlinedIcon /> : <ViewSidebarOutlinedIcon />} label={`Reading pane: ${p.view.pane}`}>
              {(["right", "bottom", "off"] as PaneMode[]).map((m) => (
                <MenuItem key={m} selected={p.view.pane === m} onClick={() => p.setView({ ...p.view, pane: m })}>
                  {m[0].toUpperCase() + m.slice(1)}
                </MenuItem>
              ))}
            </MenuCmd>
            <Cmd icon={<ForumOutlinedIcon />} label={p.view.group ? "Group by conversation: on" : "Group by conversation: off"} active={p.view.group} onClick={() => p.setView({ ...p.view, group: !p.view.group })} />
            <MenuCmd icon={<SortIcon />} label={`Sort: ${SORTS.find((s) => s.id === p.view.sort)?.label ?? ""}`}>
              {SORTS.map((s) => (
                <MenuItem key={s.id} selected={p.view.sort === s.id} onClick={() => p.setView({ ...p.view, sort: s.id })}>
                  {s.label}
                </MenuItem>
              ))}
            </MenuCmd>
          </>
        )}
        {tab === "insert" && (
          <>
            <Cmd icon={<AddToDriveIcon />} label="Attach from Drive" onClick={p.onAttachDrive} />
            <Cmd icon={<BorderColorIcon />} label="Signatures" onClick={p.onSignatures} />
          </>
        )}
        {tab === "options" && (
          <>
            <Cmd icon={<SpellcheckIcon />} label={p.spell ? "Spell check: on" : "Spell check: off"} active={p.spell} onClick={() => p.setSpell(!p.spell)} />
            <Typography level="body-xs" sx={{ ml: 1, opacity: 0.7, whiteSpace: "nowrap" }}>
              Read receipts and automatic replies aren’t available on this server.
            </Typography>
          </>
        )}
      </Box>
      <Box sx={{ display: "flex", alignItems: "center", gap: 0.75, px: 1.5, pb: 0.75, overflowX: "auto" }} data-testid="mail-quick-filters">
        <Typography level="body-xs" sx={{ opacity: 0.7, flexShrink: 0 }}>
          Filter:
        </Typography>
        {QUICK_FILTERS.map((f) => (
          <Chip
            key={f.id}
            size="sm"
            variant={p.filters.has(f.id) ? "solid" : "outlined"}
            color={p.filters.has(f.id) ? "primary" : "neutral"}
            onClick={() => p.toggleFilter(f.id)}
            sx={{ flexShrink: 0 }}
          >
            {f.label}
          </Chip>
        ))}
      </Box>
    </Box>
  );
}
