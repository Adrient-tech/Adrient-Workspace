import { useMemo, useState } from "react";
import {
  Box,
  Button,
  Chip,
  Dropdown,
  IconButton,
  Input,
  Menu,
  MenuButton,
  MenuItem,
  Modal,
  ModalClose,
  ModalDialog,
  Typography,
  ListDivider,
  Tooltip,
} from "@mui/joy";
import AddIcon from "@mui/icons-material/Add";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import ChevronRightIcon from "@mui/icons-material/ChevronRight";
import TagIcon from "@mui/icons-material/Tag";
import LockOutlinedIcon from "@mui/icons-material/LockOutlined";
import MoreHorizIcon from "@mui/icons-material/MoreHoriz";
import NotificationsNoneIcon from "@mui/icons-material/NotificationsNone";
import BookmarkBorderIcon from "@mui/icons-material/BookmarkBorder";
import EditNoteIcon from "@mui/icons-material/EditNote";
import PeopleAltOutlinedIcon from "@mui/icons-material/PeopleAltOutlined";
import ExploreOutlinedIcon from "@mui/icons-material/ExploreOutlined";
import SearchIcon from "@mui/icons-material/Search";
import EventOutlinedIcon from "@mui/icons-material/EventOutlined";
import VideocamOutlinedIcon from "@mui/icons-material/VideocamOutlined";
import GroupsOutlinedIcon from "@mui/icons-material/GroupsOutlined";
import { UserAvatar } from "./Message";
import { channelLabel, groupSidebar, type SidebarGroup } from "./lib";
import { teams } from "./teamsApi";
import type { TeamsState } from "./useTeams";
import type { TChannel, View } from "./types";

const LS_KEY = "chat.collapsed";
function loadCollapsed(): Record<string, boolean> {
  try {
    return JSON.parse(localStorage.getItem(LS_KEY) ?? "{}") as Record<string, boolean>;
  } catch {
    return {};
  }
}

function Badge({ n, mention }: { n: number; mention?: boolean }) {
  if (n <= 0) return null;
  return (
    <Chip
      size="sm"
      variant="solid"
      color={mention ? "danger" : "primary"}
      sx={{ "--Chip-minHeight": "18px", fontSize: 11, px: 0.75 }}
      data-testid={mention ? "mention-badge" : "unread-badge"}
    >
      {n > 99 ? "99+" : n}
    </Chip>
  );
}

interface Props {
  state: TeamsState;
  view: View;
  go: (v: View) => void;
  onNewChannel: (tab: number) => void;
  onProfile: () => void;
  onQuery: (q: string) => void;
  section?: "chat" | "teams";
  onSchedule?: () => void;
  onMeetNow?: () => void;
}

export function Sidebar({ state, view, go, onNewChannel, onProfile, onQuery, section, onSchedule, onMeetNow }: Props) {
  const { me, channels, sections, peopleMap, counts, presence } = state;
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>(loadCollapsed);
  const [sectionDlg, setSectionDlg] = useState<{ id?: string; name: string } | null>(null);
  const [q, setQ] = useState("");
  // Empty custom sections stay hidden, except one the user just created.
  const [freshSection, setFreshSection] = useState("");
  const allGroups = useMemo(() => groupSidebar(channels, sections, me, peopleMap), [channels, sections, me, peopleMap]);
  // The rail's Chat / Teams switch narrows the list to chats or to team channels.
  const groups = useMemo(() => {
    const nonEmpty = allGroups.filter((g) => g.builtin !== "custom" || g.channels.length > 0 || g.sectionId === freshSection);
    if (!section) return nonEmpty;
    const isChannel = (c: TChannel) => c.kind === "public" || c.kind === "private";
    const keep = (c: TChannel) => (section === "teams" ? isChannel(c) : !isChannel(c));
    return nonEmpty
      .map((g) => ({ ...g, channels: g.channels.filter(keep) }))
      .filter((g) => g.channels.length > 0 || g.sectionId === freshSection || g.builtin === (section === "teams" ? "channels" : "dms"));
  }, [allGroups, section, freshSection]);

  const toggle = async (g: SidebarGroup) => {
    if (g.builtin === "custom") {
      await teams.updateSection(g.sectionId, { collapsed: !g.collapsed });
      await state.reload();
      return;
    }
    const next = { ...collapsed, [g.key]: !collapsed[g.key] };
    setCollapsed(next);
    localStorage.setItem(LS_KEY, JSON.stringify(next));
  };

  const navItem = (key: View["kind"], label: string, icon: React.ReactNode, count?: number, mention?: boolean) => (
    <Box
      role="button"
      tabIndex={0}
      aria-label={label}
      onClick={() => go({ kind: key } as View)}
      onKeyDown={(e) => e.key === "Enter" && go({ kind: key } as View)}
      sx={{
        display: "flex", alignItems: "center", gap: 1, px: 1.5, py: 0.6, cursor: "pointer", borderRadius: "sm", mx: 0.75,
        bgcolor: view.kind === key ? "primary.softBg" : "transparent",
        fontWeight: view.kind === key || (count ?? 0) > 0 ? 700 : 400,
        "&:hover": { bgcolor: view.kind === key ? "primary.softBg" : "background.level2" },
      }}
    >
      {icon}
      <Typography level="body-sm" sx={{ flex: 1, color: "inherit", fontWeight: "inherit" }}>
        {label}
      </Typography>
      <Badge n={count ?? 0} mention={mention} />
    </Box>
  );

  const row = (c: TChannel) => {
    const active = view.kind === "channel" && view.id === c.id;
    const label = channelLabel(c, me, peopleMap);
    const unread = c.unread_count > 0 && !c.muted;
    const other = c.kind === "dm" ? c.member_ids.find((id) => id !== me) ?? me : "";
    return (
      <Box
        key={c.id}
        role="button"
        tabIndex={0}
        aria-label={label}
        data-testid="channel-row"
        data-unread={unread ? "1" : "0"}
        onClick={() => go({ kind: "channel", id: c.id })}
        onKeyDown={(e) => e.key === "Enter" && go({ kind: "channel", id: c.id })}
        sx={{
          display: "flex", alignItems: "center", gap: 1, px: 1.5, py: 0.45, mx: 0.75, cursor: "pointer", borderRadius: "sm",
          bgcolor: active ? "primary.solidBg" : "transparent",
          color: active ? "#fff" : "inherit",
          opacity: c.muted && !active ? 0.55 : 1,
          "&:hover": { bgcolor: active ? "primary.solidBg" : "background.level2" },
          "&:hover .row-menu": { visibility: "visible" },
        }}
      >
        {c.kind === "public" && <TagIcon sx={{ fontSize: 16 }} />}
        {c.kind === "private" && <LockOutlinedIcon sx={{ fontSize: 16 }} />}
        {c.kind === "dm" && (
          <Box sx={{ position: "relative", display: "flex" }}>
            <UserAvatar id={other} name={peopleMap[other]?.name ?? "?"} size={20} />
            <Box
              data-testid={`presence-${presence(other)}`}
              sx={{
                position: "absolute", right: -2, bottom: -2, width: 8, height: 8, borderRadius: "50%", border: "1.5px solid",
                borderColor: active ? "primary.solidBg" : "background.surface",
                bgcolor: presence(other) === "online" ? "#2eb67d" : presence(other) === "away" ? "#e8a317" : "#9aa5a0",
              }}
            />
          </Box>
        )}
        {c.kind === "group" && <GroupsOutlinedIcon sx={{ fontSize: 18 }} />}
        <Typography
          level="body-sm"
          sx={{ flex: 1, color: "inherit", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", fontWeight: unread || c.mention_count > 0 ? 800 : 400 }}
        >
          {label}
        </Typography>
        <Badge n={c.mention_count} mention />
        {c.mention_count === 0 && !c.muted && <Badge n={c.unread_count} />}
        <Dropdown>
          <MenuButton
            slots={{ root: IconButton }}
            slotProps={{ root: { size: "sm", variant: "plain", className: "row-menu", "aria-label": `Options for ${label}`, onClick: (e: React.MouseEvent) => e.stopPropagation(), sx: { visibility: "hidden", minHeight: 20, minWidth: 20, p: 0, color: "inherit" } } }}
          >
            <MoreHorizIcon sx={{ fontSize: 16 }} />
          </MenuButton>
          <Menu size="sm" placement="bottom-end" sx={{ zIndex: 1500 }}>
            <MenuItem onClick={async () => { await teams.setPrefs(c.id, { starred: !c.starred }); state.patchChannel(c.id, { starred: !c.starred }); }}>
              {c.starred ? "Unstar" : "Star"}
            </MenuItem>
            <MenuItem onClick={async () => { await teams.setPrefs(c.id, { muted: !c.muted }); state.patchChannel(c.id, { muted: !c.muted }); }}>
              {c.muted ? "Unmute" : "Mute"}
            </MenuItem>
            <ListDivider />
            {sections.map((s) => (
              <MenuItem key={s.id} onClick={async () => { await teams.setPrefs(c.id, { section_id: s.id }); state.patchChannel(c.id, { section_id: s.id }); }}>
                Move to {s.name}
              </MenuItem>
            ))}
            {c.section_id && (
              <MenuItem onClick={async () => { await teams.setPrefs(c.id, { section_id: "" }); state.patchChannel(c.id, { section_id: "" }); }}>
                Remove from section
              </MenuItem>
            )}
            <MenuItem onClick={() => setSectionDlg({ name: "" })}>New section…</MenuItem>
          </Menu>
        </Dropdown>
      </Box>
    );
  };

  return (
    <Box
      component="nav"
      aria-label="Chat sidebar"
      sx={{ width: 280, flexShrink: 0, display: "flex", flexDirection: "column", borderRight: "1px solid", borderColor: "divider", bgcolor: "background.level1", minHeight: 0, overflowY: "auto" }}
    >
      <Box sx={{ display: "flex", alignItems: "center", gap: 1, px: 1.5, pt: 1.5, pb: 1 }}>
        <Typography level="title-md" sx={{ flex: 1, fontWeight: 800 }}>
          Chat
        </Typography>
        <Tooltip title={state.live ? "Live" : "Reconnecting…"}>
          <Box sx={{ width: 8, height: 8, borderRadius: "50%", bgcolor: state.live ? "#2eb67d" : "#e8a317" }} />
        </Tooltip>
        <Tooltip title="Your profile & status">
          <IconButton size="sm" variant="plain" onClick={onProfile} aria-label="Your profile">
            <UserAvatar id={me || "x"} name={state.meName || "?"} size={24} />
          </IconButton>
        </Tooltip>
        <Tooltip title="New message or channel">
          <IconButton size="sm" variant="solid" color="primary" onClick={() => onNewChannel(0)} aria-label="New conversation">
            <EditNoteIcon />
          </IconButton>
        </Tooltip>
      </Box>

      <Box sx={{ px: 1.25, pb: 1, display: "grid", gridTemplateColumns: "1fr 1fr", gap: 0.75 }} data-testid="chat-quick-actions">
        <Button size="sm" startDecorator={<AddIcon />} onClick={() => onNewChannel(1)} data-testid="new-chat">
          New chat
        </Button>
        <Button size="sm" variant="soft" startDecorator={<GroupsOutlinedIcon />} onClick={() => onNewChannel(0)} data-testid="new-team">
          New channel
        </Button>
        <Button size="sm" variant="soft" color="neutral" startDecorator={<VideocamOutlinedIcon />} onClick={onMeetNow} data-testid="meet-now">
          Meet now
        </Button>
        <Button size="sm" variant="soft" color="neutral" startDecorator={<EventOutlinedIcon />} onClick={onSchedule} data-testid="schedule-meeting">
          Schedule
        </Button>
        <Button size="sm" variant="plain" color="neutral" startDecorator={<ExploreOutlinedIcon />} onClick={() => go({ kind: "browse" })} sx={{ gridColumn: "1 / span 2" }}>
          Join or browse channels
        </Button>
      </Box>
      <Box sx={{ px: 1.25, pb: 1 }}>
        <Input
          size="sm"
          placeholder="Search messages"
          startDecorator={<SearchIcon sx={{ fontSize: 18 }} />}
          value={q}
          onChange={(e) => setQ(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && q.trim()) onQuery(q.trim());
          }}
          slotProps={{ input: { "aria-label": "Search messages" } }}
        />
      </Box>

      {navItem("activity", "Activity", <NotificationsNoneIcon sx={{ fontSize: 18 }} />, counts.activity, true)}
      {navItem("later", "Later", <BookmarkBorderIcon sx={{ fontSize: 18 }} />, 0)}
      {navItem("drafts", "Drafts & sent", <EditNoteIcon sx={{ fontSize: 18 }} />, counts.drafts + counts.scheduled)}
      {navItem("people", "People", <PeopleAltOutlinedIcon sx={{ fontSize: 18 }} />)}
      {navItem("browse", "Browse channels", <ExploreOutlinedIcon sx={{ fontSize: 18 }} />)}
      <ListDivider sx={{ my: 1 }} />

      {groups.map((g) => {
        const isCollapsed = g.builtin === "custom" ? !!g.collapsed : !!collapsed[g.key];
        const visible = isCollapsed
          ? g.channels.filter((c) => (c.unread_count > 0 && !c.muted) || c.mention_count > 0 || (view.kind === "channel" && view.id === c.id))
          : g.channels;
        return (
          <Box key={g.key} sx={{ mb: 1 }} data-testid={`section-${g.key}`}>
            <Box sx={{ display: "flex", alignItems: "center", px: 1, mx: 0.75, "&:hover .sec-actions": { visibility: "visible" } }}>
              <Box
                role="button"
                tabIndex={0}
                aria-label={`${isCollapsed ? "Expand" : "Collapse"} ${g.title}`}
                onClick={() => void toggle(g)}
                onKeyDown={(e) => e.key === "Enter" && void toggle(g)}
                sx={{ display: "flex", alignItems: "center", gap: 0.25, flex: 1, cursor: "pointer", py: 0.4 }}
              >
                {isCollapsed ? <ChevronRightIcon sx={{ fontSize: 18 }} /> : <ExpandMoreIcon sx={{ fontSize: 18 }} />}
                <Typography level="body-xs" sx={{ fontWeight: 700, textTransform: "uppercase", letterSpacing: 0.4, opacity: 0.75 }}>
                  {g.title}
                </Typography>
                {isCollapsed && <Badge n={g.mentions || g.unread} mention={g.mentions > 0} />}
              </Box>
              <Box className="sec-actions" sx={{ visibility: "hidden", display: "flex" }}>
                {g.builtin === "channels" && (
                  <Dropdown>
                    <MenuButton slots={{ root: IconButton }} slotProps={{ root: { size: "sm", variant: "plain", "aria-label": "Channel options", sx: { minHeight: 20, minWidth: 20, p: 0 } } }}>
                      <AddIcon sx={{ fontSize: 16 }} />
                    </MenuButton>
                    <Menu size="sm" sx={{ zIndex: 1500 }}>
                      <MenuItem onClick={() => onNewChannel(0)}>Create channel</MenuItem>
                      <MenuItem onClick={() => go({ kind: "browse" })}>Browse channels</MenuItem>
                      <MenuItem onClick={() => setSectionDlg({ name: "" })}>Create section</MenuItem>
                    </Menu>
                  </Dropdown>
                )}
                {g.builtin === "dms" && (
                  <IconButton size="sm" variant="plain" aria-label="New direct message" onClick={() => onNewChannel(1)} sx={{ minHeight: 20, minWidth: 20, p: 0 }}>
                    <AddIcon sx={{ fontSize: 16 }} />
                  </IconButton>
                )}
                {g.builtin === "custom" && (
                  <Dropdown>
                    <MenuButton slots={{ root: IconButton }} slotProps={{ root: { size: "sm", variant: "plain", "aria-label": `Section options for ${g.title}`, sx: { minHeight: 20, minWidth: 20, p: 0 } } }}>
                      <MoreHorizIcon sx={{ fontSize: 16 }} />
                    </MenuButton>
                    <Menu size="sm" sx={{ zIndex: 1500 }}>
                      <MenuItem onClick={() => setSectionDlg({ id: g.sectionId, name: g.title })}>Rename section</MenuItem>
                      <MenuItem
                        color="danger"
                        onClick={async () => {
                          await teams.deleteSection(g.sectionId);
                          await state.reload();
                        }}
                      >
                        Delete section
                      </MenuItem>
                    </Menu>
                  </Dropdown>
                )}
              </Box>
            </Box>
            {visible.map(row)}
            {!isCollapsed && g.channels.length === 0 && (
              <Typography level="body-xs" sx={{ px: 3, py: 0.5, opacity: 0.55 }}>
                {g.builtin === "custom" ? "Use ⋯ on a channel to move it here" : g.builtin === "dms" ? "No conversations yet" : "No channels yet"}
              </Typography>
            )}
          </Box>
        );
      })}

      <Modal open={!!sectionDlg} onClose={() => setSectionDlg(null)}>
        <ModalDialog sx={{ minWidth: 320 }}>
          <ModalClose />
          <Typography level="title-md">{sectionDlg?.id ? "Rename section" : "New section"}</Typography>
          <Input
            autoFocus
            value={sectionDlg?.name ?? ""}
            onChange={(e) => setSectionDlg((s) => (s ? { ...s, name: e.target.value } : s))}
            slotProps={{ input: { "aria-label": "Section name", maxLength: 60 } }}
          />
          <Button
            disabled={!sectionDlg?.name.trim()}
            onClick={async () => {
              if (!sectionDlg) return;
              if (sectionDlg.id) await teams.updateSection(sectionDlg.id, { name: sectionDlg.name });
              else {
                const created = (await teams.createSection(sectionDlg.name)) as { id?: string } | undefined;
                if (created?.id) setFreshSection(created.id);
              }
              setSectionDlg(null);
              await state.reload();
            }}
          >
            Save
          </Button>
        </ModalDialog>
      </Modal>
    </Box>
  );
}
