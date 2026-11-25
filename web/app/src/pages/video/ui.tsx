import { useState, type ReactNode } from "react";
import { Box, Typography, Avatar, Chip, IconButton, Dropdown, Menu, MenuButton, MenuItem, LinearProgress } from "@mui/joy";
import MoreVertRoundedIcon from "@mui/icons-material/MoreVertRounded";
import WatchLaterRoundedIcon from "@mui/icons-material/WatchLaterRounded";
import PlaylistAddRoundedIcon from "@mui/icons-material/PlaylistAddRounded";
import MovieRoundedIcon from "@mui/icons-material/MovieRounded";
import LockOutlinedIcon from "@mui/icons-material/LockOutlined";
import GroupsRoundedIcon from "@mui/icons-material/GroupsRounded";
import VideocamRoundedIcon from "@mui/icons-material/VideocamRounded";
import { useNavigate } from "react-router-dom";
import { tokens } from "../../theme";
import type { VideoInfo } from "./types";
import { formatDuration } from "./media";
import { formatCount, timeAgo } from "./logic";

/** hue of a name, for avatars and posters without an image. */
export function hueOf(name: string): number {
  let h = 0;
  for (let i = 0; i < name.length; i++) h = (h * 31 + name.charCodeAt(i)) >>> 0;
  return h % 360;
}

export function ChannelAvatar({ name, size = 36 }: { name: string; size?: number }) {
  const h = hueOf(name);
  return (
    <Avatar sx={{ width: size, height: size, fontSize: size * 0.42, fontWeight: 700, color: "#0E2420", bgcolor: `hsl(${h} 45% 78%)` }}>
      {(name.trim()[0] ?? "?").toUpperCase()}
    </Avatar>
  );
}

/** Poster shows the video's thumbnail; without one, a coloured tile. */
export function Poster({ v, radius = 12 }: { v: VideoInfo; radius?: number }) {
  const h = hueOf(v.title);
  return (
    <Box sx={{ position: "relative", aspectRatio: "16 / 9", borderRadius: `${radius}px`, overflow: "hidden", bgcolor: `hsl(${h} 35% 80%)`, background: v.thumb_url ? undefined : `linear-gradient(135deg, hsl(${h} 40% 78%), hsl(${(h + 50) % 360} 45% 60%))`, lineHeight: 0 }}>
      {v.thumb_url ? (
        <img src={v.thumb_url} alt="" loading="lazy" style={{ width: "100%", height: "100%", objectFit: "cover", display: "block" }} />
      ) : (
        <Box sx={{ position: "absolute", inset: 0, display: "flex", alignItems: "center", justifyContent: "center", color: "rgba(255,255,255,.85)" }}>
          <MovieRoundedIcon sx={{ fontSize: 40 }} />
        </Box>
      )}
      {v.status === "processing" && (
        <Box sx={{ position: "absolute", inset: 0, bgcolor: "rgba(14,36,32,.55)", display: "flex", alignItems: "center", justifyContent: "center", color: "#fff", fontSize: 13, fontWeight: 700, lineHeight: 1.2 }}>Processing…</Box>
      )}
      {v.duration_seconds > 0 && (
        <Box sx={{ position: "absolute", right: 6, bottom: 6, px: 0.75, py: 0.1, borderRadius: "6px", bgcolor: "rgba(0,0,0,.78)", color: "#fff", fontSize: 12, fontWeight: 600, lineHeight: 1.5 }}>
          {formatDuration(v.duration_seconds)}
        </Box>
      )}
      {v.progress_percent > 0.02 && !v.watched && (
        <LinearProgress determinate value={Math.min(100, v.progress_percent * 100)} sx={{ position: "absolute", left: 0, right: 0, bottom: 0, "--LinearProgress-radius": "0px", "--LinearProgress-thickness": "4px", bgcolor: "rgba(255,255,255,.4)" }} />
      )}
    </Box>
  );
}

export function VisibilityBadge({ v }: { v: Pick<VideoInfo, "visibility"> }) {
  if (v.visibility === "org") return null;
  return (
    <Chip size="sm" variant="soft" color="neutral" startDecorator={v.visibility === "private" ? <LockOutlinedIcon /> : <GroupsRoundedIcon />}>
      {v.visibility === "private" ? "Private" : "Group"}
    </Chip>
  );
}

export function metaLine(v: VideoInfo): string {
  return `${formatCount(v.views)} view${v.views === 1 ? "" : "s"} · ${timeAgo(v.created_at)}`;
}

/** VideoCard is a grid card: poster, title, channel and counts. */
export function VideoCard({ v, onWatchLater, onAddToPlaylist, compact }: { v: VideoInfo; onWatchLater?: (v: VideoInfo) => void; onAddToPlaylist?: (v: VideoInfo) => void; compact?: boolean }) {
  const nav = useNavigate();
  const open = () => nav(`/video/v/${v.id}`);
  return (
    <Box data-testid={`video-card-${v.id}`} sx={{ minWidth: 0, display: compact ? "flex" : "block", gap: compact ? 1.25 : 0, "&:hover .vc-more": { opacity: 1 } }}>
      <Box onClick={open} role="link" tabIndex={0} aria-label={v.title} onKeyDown={(e) => e.key === "Enter" && open()} sx={{ cursor: "pointer", width: compact ? 168 : "auto", flexShrink: 0, "&:hover img": { transform: "scale(1.02)" }, "& img": { transition: "transform .2s" }, borderRadius: "12px", overflow: "hidden" }}>
        <Poster v={v} />
      </Box>
      <Box sx={{ display: "flex", gap: 1.25, mt: compact ? 0 : 1.25, minWidth: 0, flex: 1 }}>
        {!compact && (
          <Box onClick={() => nav(`/video/channel/${v.owner_id}`)} sx={{ cursor: "pointer" }}>
            <ChannelAvatar name={v.owner_name} />
          </Box>
        )}
        <Box sx={{ minWidth: 0, flex: 1 }}>
          <Typography level="title-sm" onClick={open} sx={{ fontWeight: 650, lineHeight: 1.3, cursor: "pointer", display: "-webkit-box", WebkitLineClamp: 2, WebkitBoxOrient: "vertical", overflow: "hidden" }}>
            {v.title}
          </Typography>
          <Typography level="body-xs" onClick={() => nav(`/video/channel/${v.owner_id}`)} sx={{ color: "text.tertiary", mt: 0.25, cursor: "pointer", "&:hover": { color: "text.primary" } }} noWrap>
            {v.owner_name}
          </Typography>
          <Typography level="body-xs" sx={{ color: "text.tertiary" }} noWrap>
            {metaLine(v)}
          </Typography>
          {v.visibility !== "org" && <Box sx={{ mt: 0.5 }}><VisibilityBadge v={v} /></Box>}
        </Box>
        {(onWatchLater || onAddToPlaylist) && (
          <Dropdown>
            <MenuButton slots={{ root: IconButton }} slotProps={{ root: { size: "sm", variant: "plain", color: "neutral", className: "vc-more", "aria-label": `More for ${v.title}`, sx: { opacity: { xs: 1, md: 0 }, alignSelf: "flex-start" } } }}>
              <MoreVertRoundedIcon />
            </MenuButton>
            <Menu size="sm" placement="bottom-end" sx={{ zIndex: 1400 }}>
              {onWatchLater && (
                <MenuItem onClick={() => onWatchLater(v)}>
                  <WatchLaterRoundedIcon fontSize="small" /> {v.watch_later ? "Remove from Watch later" : "Save to Watch later"}
                </MenuItem>
              )}
              {onAddToPlaylist && (
                <MenuItem onClick={() => onAddToPlaylist(v)}>
                  <PlaylistAddRoundedIcon fontSize="small" /> Add to playlist
                </MenuItem>
              )}
            </Menu>
          </Dropdown>
        )}
      </Box>
    </Box>
  );
}

export function VideoGrid({ children }: { children: ReactNode }) {
  return <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "repeat(2, minmax(0,1fr))", lg: "repeat(3, minmax(0,1fr))", xl: "repeat(4, minmax(0,1fr))" }, gap: { xs: 2.5, md: "28px 20px" } }}>{children}</Box>;
}

export function PageTitle({ title, sub, action, icon }: { title: string; sub?: string; action?: ReactNode; icon?: ReactNode }) {
  return (
    <Box sx={{ display: "flex", alignItems: "flex-end", gap: 2, mb: 2.5, flexWrap: "wrap" }}>
      {icon && <Box sx={{ color: "primary.solidBg", display: "flex", "& svg": { fontSize: 32 } }}>{icon}</Box>}
      <Box sx={{ flex: 1, minWidth: 0 }}>
        <Typography level="h1" sx={{ fontFamily: tokens.font.display, fontSize: { xs: "xl3", sm: "xl4" }, letterSpacing: "-0.02em" }}>{title}</Typography>
        {sub && <Typography level="body-sm" sx={{ color: "text.tertiary", mt: 0.5 }}>{sub}</Typography>}
      </Box>
      {action}
    </Box>
  );
}

export function EmptyNote({ icon, title, text, action }: { icon?: ReactNode; title: string; text?: string; action?: ReactNode }) {
  return (
    <Box sx={{ textAlign: "center", py: 8, px: 2, border: "1px dashed", borderColor: "divider", borderRadius: "16px", bgcolor: "background.level1" }}>
      <Box sx={{ color: "text.tertiary", mb: 1, "& svg": { fontSize: 44 } }}>{icon ?? <VideocamRoundedIcon />}</Box>
      <Typography level="title-md">{title}</Typography>
      {text && <Typography level="body-sm" sx={{ color: "text.tertiary", mt: 0.5, maxWidth: 440, mx: "auto" }}>{text}</Typography>}
      {action && <Box sx={{ mt: 2 }}>{action}</Box>}
    </Box>
  );
}

export function ChipRow<T extends string>({ value, options, onChange, label }: { value: T; options: { value: T; label: string }[]; onChange: (v: T) => void; label: string }) {
  return (
    <Box role="tablist" aria-label={label} sx={{ display: "flex", gap: 1, flexWrap: "wrap" }}>
      {options.map((o) => (
        <Chip key={o.value} role="tab" aria-selected={value === o.value} size="lg" variant={value === o.value ? "solid" : "soft"} color={value === o.value ? "primary" : "neutral"} onClick={() => onChange(o.value)} sx={{ cursor: "pointer", "--Chip-radius": "10px", fontWeight: 600 }}>
          {o.label}
        </Chip>
      ))}
    </Box>
  );
}

/** Expandable text block ("Show more"), used by the description. */
export function useToggle(initial = false): [boolean, () => void] {
  const [v, set] = useState(initial);
  return [v, () => set((x) => !x)];
}
