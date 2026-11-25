import { useEffect, useMemo, useRef, useState } from "react";
import { Routes, Route, NavLink, useLocation, useNavigate, Navigate, matchPath } from "react-router-dom";
import { Box, Typography, Input, Button, IconButton } from "@mui/joy";
import HomeRoundedIcon from "@mui/icons-material/HomeRounded";
import SubscriptionsRoundedIcon from "@mui/icons-material/SubscriptionsRounded";
import VideocamRoundedIcon from "@mui/icons-material/VideocamRounded";
import HistoryRoundedIcon from "@mui/icons-material/HistoryRounded";
import WatchLaterRoundedIcon from "@mui/icons-material/WatchLaterRounded";
import PlaylistPlayRoundedIcon from "@mui/icons-material/PlaylistPlayRounded";
import VideoLibraryRoundedIcon from "@mui/icons-material/VideoLibraryRounded";
import UploadRoundedIcon from "@mui/icons-material/UploadRounded";
import SearchIcon from "@mui/icons-material/Search";
import CloseRoundedIcon from "@mui/icons-material/CloseRounded";
import type { User } from "../../api/types";
import { Header } from "../../components/Header";
import { BizAssistant } from "../../components/BizAssistant";
import { VideoProvider, useVideoCtx } from "./ctx";
import { HomePage } from "./HomePage";
import { WatchPage } from "./WatchPage";
import { ChannelPage, HistoryPage, MeetingsPage, PlaylistPage, PlaylistsPage, SearchPage, SubscriptionsPage, WatchLaterPage } from "./Pages";
import { UploadPage } from "./UploadPage";
import { StudioPage } from "./StudioPage";
import { listSubscriptions } from "./api";
import type { Channel } from "./types";
import { ChannelAvatar } from "./ui";
import { createVideoTools, VIDEO_HINT, VIDEO_SUGGESTIONS } from "./ai/tools";

interface VideoAppProps {
  user: User;
}

const NAV = [
  { to: "/video", end: true, label: "Home", icon: <HomeRoundedIcon /> },
  { to: "/video/subscriptions", label: "Subscriptions", icon: <SubscriptionsRoundedIcon /> },
  { to: "/video/meetings", label: "Meetings", icon: <VideocamRoundedIcon /> },
];
const YOU = [
  { to: "/video/history", label: "History", icon: <HistoryRoundedIcon /> },
  { to: "/video/watch-later", label: "Watch later", icon: <WatchLaterRoundedIcon /> },
  { to: "/video/playlists", label: "Playlists", icon: <PlaylistPlayRoundedIcon /> },
  { to: "/video/studio", label: "Your videos", icon: <VideoLibraryRoundedIcon /> },
];
const MOBILE = [NAV[0], NAV[1], { to: "/video/upload", label: "Upload", icon: <UploadRoundedIcon /> }, YOU[1], YOU[3]];

/** VideoApp: a video-sharing layout with a left rail, search bar and watch page. */
export default function VideoApp({ user }: VideoAppProps) {
  return (
    <VideoProvider userId={user.id}>
      <Shell user={user} />
    </VideoProvider>
  );
}

function Shell({ user }: { user: User }) {
  const nav = useNavigate();
  const loc = useLocation();
  const ctx = useVideoCtx();
  const [q, setQ] = useState(() => new URLSearchParams(loc.search).get("q") ?? "");
  const [subs, setSubs] = useState<Channel[]>([]);
  const searchRef = useRef<HTMLInputElement>(null);
  const onWatch = !!matchPath("/video/v/:id", loc.pathname);

  useEffect(() => {
    listSubscriptions().then(setSubs).catch(() => setSubs([]));
  }, [ctx.subsVersion]);
  useEffect(() => {
    if (!loc.pathname.startsWith("/video/search")) setQ("");
  }, [loc.pathname]);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null;
      if (e.key === "/" && el && !["INPUT", "TEXTAREA"].includes(el.tagName) && !el.isContentEditable) {
        e.preventDefault();
        searchRef.current?.focus();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const onSearch = (v: string) => {
    setQ(v);
    const target = `/video/search?q=${encodeURIComponent(v)}`;
    if (loc.pathname.startsWith("/video/search")) nav(target, { replace: true });
    else if (v.trim()) nav(target);
  };

  const tools = useMemo(
    () =>
      createVideoTools({
        currentVideoId: () => matchPath("/video/v/:id", window.location.pathname)?.params.id ?? null,
      }),
    [],
  );

  const item = (n: (typeof NAV)[number]) => (
    <Box
      key={n.to}
      component={NavLink}
      to={n.to}
      end={"end" in n ? n.end : undefined}
      data-testid={`rail-${n.label.toLowerCase().replace(/\s+/g, "-")}`}
      sx={{ display: "flex", alignItems: "center", gap: 1.75, px: 1.5, py: 0.95, borderRadius: "10px", textDecoration: "none", color: "text.primary", fontSize: "sm", fontWeight: 500, "&:hover": { bgcolor: "background.level1" }, "&.active": { bgcolor: "primary.softBg", color: "primary.solidBg", fontWeight: 700 }, "& svg": { fontSize: 22 } }}
    >
      {n.icon}
      {n.label}
    </Box>
  );

  return (
    <>
      <Header user={user} />
      <Box sx={{ display: "flex", alignItems: "flex-start" }}>
        <Box
          component="nav"
          aria-label="Video sections"
          sx={{ display: { xs: "none", md: onWatch ? "none" : "flex", xl: "flex" }, flexDirection: "column", gap: 0.25, width: 232, flexShrink: 0, position: "sticky", top: "var(--ad-topbar-h, 56px)", height: "calc(100vh - var(--ad-topbar-h, 56px))", overflowY: "auto", p: 1.5, borderRight: "1px solid", borderColor: "divider", bgcolor: "background.surface" }}
        >
          {NAV.map(item)}
          <Box sx={{ height: "1px", bgcolor: "divider", my: 1 }} />
          <Typography level="body-xs" sx={{ px: 1.5, py: 0.5, textTransform: "uppercase", letterSpacing: ".08em", fontWeight: 700, color: "text.tertiary" }}>You</Typography>
          {YOU.map(item)}
          {subs.length > 0 && (
            <>
              <Box sx={{ height: "1px", bgcolor: "divider", my: 1 }} />
              <Typography level="body-xs" sx={{ px: 1.5, py: 0.5, textTransform: "uppercase", letterSpacing: ".08em", fontWeight: 700, color: "text.tertiary" }}>Channels</Typography>
              {subs.slice(0, 8).map((c) => (
                <Box key={c.user_id} component={NavLink} to={`/video/channel/${c.user_id}`} sx={{ display: "flex", alignItems: "center", gap: 1.5, px: 1.5, py: 0.6, borderRadius: "10px", textDecoration: "none", color: "text.primary", fontSize: "sm", "&:hover": { bgcolor: "background.level1" }, "&.active": { bgcolor: "primary.softBg" } }}>
                  <ChannelAvatar name={c.name} size={24} />
                  <Box component="span" sx={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{c.name}</Box>
                </Box>
              ))}
            </>
          )}
        </Box>

        <Box sx={{ flex: 1, minWidth: 0, pb: { xs: "72px", md: 4 } }}>
          <Box sx={{ position: "sticky", top: "var(--ad-topbar-h, 56px)", zIndex: 20, display: "flex", alignItems: "center", gap: 1.5, px: { xs: 1.5, md: 3 }, py: 1.25, bgcolor: "rgba(246,248,247,.92)", backdropFilter: "blur(8px)", borderBottom: "1px solid", borderColor: "divider" }}>
            <Input
              slotProps={{ input: { ref: searchRef, "aria-label": "Search videos" } }}
              startDecorator={<SearchIcon />}
              endDecorator={q ? <IconButton size="sm" variant="plain" aria-label="Clear search" onClick={() => onSearch("")}><CloseRoundedIcon /></IconButton> : undefined}
              placeholder="Search videos and channels"
              value={q}
              onChange={(e) => onSearch(e.target.value)}
              data-testid="video-search-input"
              sx={{ flex: 1, maxWidth: 600, borderRadius: "999px", "--Input-focusedThickness": "2px" }}
            />
            <Box sx={{ ml: "auto", display: "flex", gap: 1, alignItems: "center" }}>
              <Button size="sm" startDecorator={<UploadRoundedIcon />} onClick={() => nav("/video/upload")} data-testid="top-upload" sx={{ display: { xs: "none", sm: "inline-flex" } }}>Upload</Button>
              <BizAssistant app="video" title="Video" tools={tools} suggestions={VIDEO_SUGGESTIONS} systemHint={VIDEO_HINT} />
            </Box>
          </Box>
          <Box sx={{ px: { xs: 1.5, md: 3 }, py: { xs: 2, md: 3 }, maxWidth: onWatch ? 1760 : 1500 }}>
            <Routes>
              <Route path="/" element={<HomePage />} />
              <Route path="/v/:id" element={<WatchPage />} />
              <Route path="/search" element={<SearchPage />} />
              <Route path="/channel/:userId" element={<ChannelPage />} />
              <Route path="/subscriptions" element={<SubscriptionsPage />} />
              <Route path="/meetings" element={<MeetingsPage />} />
              <Route path="/history" element={<HistoryPage />} />
              <Route path="/watch-later" element={<WatchLaterPage />} />
              <Route path="/playlists" element={<PlaylistsPage />} />
              <Route path="/playlist/:id" element={<PlaylistPage />} />
              <Route path="/studio" element={<StudioPage />} />
              <Route path="/upload" element={<UploadPage />} />
              <Route path="*" element={<Navigate to="/video" replace />} />
            </Routes>
          </Box>
        </Box>
      </Box>

      <Box component="nav" aria-label="Video sections" sx={{ display: { xs: "flex", md: "none" }, position: "fixed", bottom: 0, left: 0, right: 0, height: 56, zIndex: 1110, bgcolor: "background.surface", borderTop: "1px solid", borderColor: "divider" }}>
        {MOBILE.map((n) => (
          <Box key={n.to} component={NavLink} to={n.to} end={"end" in n ? (n.end as boolean) : undefined} sx={{ flex: 1, display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", textDecoration: "none", color: "text.tertiary", fontSize: 11, fontWeight: 600, "&.active": { color: "primary.solidBg" }, "& svg": { fontSize: 22 } }}>
            {n.icon}
            {n.label}
          </Box>
        ))}
      </Box>
    </>
  );
}
