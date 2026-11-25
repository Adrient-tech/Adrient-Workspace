/** Teams-style left rail: Activity, Chat, Teams, Meetings, Calls, Files. */
import { Badge, Box, Typography } from "@mui/joy";
import NotificationsNoneIcon from "@mui/icons-material/NotificationsNone";
import ChatBubbleOutlineIcon from "@mui/icons-material/ChatBubbleOutline";
import GroupsOutlinedIcon from "@mui/icons-material/GroupsOutlined";
import EventOutlinedIcon from "@mui/icons-material/EventOutlined";
import CallOutlinedIcon from "@mui/icons-material/CallOutlined";
import FolderOutlinedIcon from "@mui/icons-material/FolderOutlined";
import type { View } from "./types";

export type Section = "chat" | "teams";

interface Props {
  view: View;
  section: Section;
  setSection: (s: Section) => void;
  go: (v: View) => void;
  counts: { activity: number; chat: number; teams: number };
}

export function Rail({ view, section, setSection, go, counts }: Props) {
  const inChannel = view.kind === "channel";
  const item = (id: string, label: string, icon: React.ReactNode, active: boolean, onClick: () => void, n = 0) => (
    <Box
      key={id}
      role="button"
      tabIndex={0}
      aria-label={label}
      aria-current={active ? "page" : undefined}
      data-testid={`rail-${id}`}
      onClick={onClick}
      onKeyDown={(e) => e.key === "Enter" && onClick()}
      sx={{
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        gap: 0.25,
        py: 0.9,
        mx: 0.5,
        borderRadius: "md",
        cursor: "pointer",
        color: active ? "primary.700" : "text.secondary",
        bgcolor: active ? "primary.softBg" : "transparent",
        borderLeft: "3px solid",
        borderColor: active ? "primary.500" : "transparent",
        "&:hover": { bgcolor: active ? "primary.softBg" : "background.level1" },
      }}
    >
      <Badge badgeContent={n} size="sm" color="danger" max={99}>
        {icon}
      </Badge>
      <Typography level="body-xs" sx={{ color: "inherit", fontSize: 11, fontWeight: active ? 700 : 500 }}>
        {label}
      </Typography>
    </Box>
  );
  return (
    <Box
      component="nav"
      aria-label="Chat navigation"
      data-testid="chat-rail"
      sx={{ width: 68, flexShrink: 0, borderRight: "1px solid", borderColor: "divider", pt: 1, display: "flex", flexDirection: "column", gap: 0.25, bgcolor: "background.level1" }}
    >
      {item("activity", "Activity", <NotificationsNoneIcon />, view.kind === "activity", () => go({ kind: "activity" }), counts.activity)}
      {item("chat", "Chat", <ChatBubbleOutlineIcon />, section === "chat" && (inChannel || view.kind === "later" || view.kind === "drafts" || view.kind === "people" || view.kind === "search"), () => setSection("chat"), counts.chat)}
      {item("teams", "Teams", <GroupsOutlinedIcon />, (section === "teams" && inChannel) || view.kind === "browse", () => setSection("teams"), counts.teams)}
      {item("meetings", "Meetings", <EventOutlinedIcon />, view.kind === "meetings", () => go({ kind: "meetings" }))}
      {item("calls", "Calls", <CallOutlinedIcon />, view.kind === "calls", () => go({ kind: "calls" }))}
      {item("files", "Files", <FolderOutlinedIcon />, view.kind === "files", () => go({ kind: "files" }))}
    </Box>
  );
}
