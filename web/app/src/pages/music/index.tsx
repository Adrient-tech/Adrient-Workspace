import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Routes, Route, NavLink, useLocation, useNavigate, Navigate } from "react-router-dom";
import { Box, Typography, Input, Button, IconButton } from "@mui/joy";
import HomeRoundedIcon from "@mui/icons-material/HomeRounded";
import ExploreRoundedIcon from "@mui/icons-material/ExploreRounded";
import LibraryMusicRoundedIcon from "@mui/icons-material/LibraryMusicRounded";
import QueueMusicRoundedIcon from "@mui/icons-material/QueueMusicRounded";
import CloudUploadRoundedIcon from "@mui/icons-material/CloudUploadRounded";
import RadioIcon from "@mui/icons-material/Radio";
import SearchIcon from "@mui/icons-material/Search";
import AddRoundedIcon from "@mui/icons-material/AddRounded";
import CloseRoundedIcon from "@mui/icons-material/CloseRounded";
import SpeakerGroupRoundedIcon from "@mui/icons-material/SpeakerGroupRounded";
import type { User } from "../../api/types";
import { Header } from "../../components/Header";
import { BizAssistant } from "../../components/BizAssistant";
import { PlayerProvider, usePlayer } from "./player";
import { LibraryProvider, useLibrary } from "./library";
import { PlayerBar, PLAYER_H } from "./PlayerBar";
import { QueuePanel } from "./QueuePanel";
import { NowPlaying } from "./NowPlaying";
import { HomePage } from "./HomePage";
import { LibraryPage, SmartPlaylistPage } from "./LibraryPage";
import { AlbumPage, ArtistPage, ExplorePage, SearchPage } from "./CatalogPages";
import { PlaylistPage, PlaylistsPage } from "./PlaylistsPage";
import { UploadPage } from "./UploadPage";
import { RadioStations } from "./RadioStations";
import { DevicesDialog } from "./DevicesDialog";
import { PlaylistFormDialog } from "./dialogs";
import { Cover, PageTitle } from "./ui";
import { createMusicTools, MUSIC_HINT, MUSIC_SUGGESTIONS } from "./ai/tools";

interface MusicAppProps {
  user: User;
}

const NAV = [
  { to: "/music", end: true, label: "Home", icon: <HomeRoundedIcon /> },
  { to: "/music/explore", label: "Explore", icon: <ExploreRoundedIcon /> },
  { to: "/music/library", label: "Library", icon: <LibraryMusicRoundedIcon /> },
  { to: "/music/playlists", label: "Playlists", icon: <QueueMusicRoundedIcon /> },
  { to: "/music/upload", label: "Upload", icon: <CloudUploadRoundedIcon /> },
];

/** MusicApp: left rail, pages, queue drawer and the persistent player bar. */
export default function MusicApp({ user }: MusicAppProps) {
  return (
    <PlayerProvider>
      <LibraryProvider userId={user.id}>
        <Shell user={user} />
      </LibraryProvider>
    </PlayerProvider>
  );
}

function isTyping(t: EventTarget | null): boolean {
  const el = t as HTMLElement | null;
  if (!el) return false;
  return el.tagName === "INPUT" || el.tagName === "TEXTAREA" || el.tagName === "SELECT" || el.isContentEditable || el.getAttribute?.("role") === "slider";
}

function Shell({ user }: { user: User }) {
  const p = usePlayer();
  const lib = useLibrary();
  const nav = useNavigate();
  const loc = useLocation();
  const [queueOpen, setQueueOpen] = useState(false);
  const [devices, setDevices] = useState(false);
  const [creating, setCreating] = useState(false);
  const [q, setQ] = useState(() => new URLSearchParams(loc.search).get("q") ?? "");
  const searchRef = useRef<HTMLInputElement>(null);
  const nowOpen = loc.pathname.replace(/\/$/, "") === "/music/now";
  const hasPlayer = !!p.current || !!p.radioStation;

  // Keyboard shortcuts (ignored while typing).
  const pRef = useRef(p);
  pRef.current = p;
  const libRef = useRef(lib);
  libRef.current = lib;
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.ctrlKey || e.metaKey || e.altKey || isTyping(e.target)) return;
      const pl = pRef.current;
      const k = e.key;
      let used = true;
      if (k === " " || k === "k") pl.toggle();
      else if (k === "ArrowRight") e.shiftKey ? pl.next() : pl.seekBy(5);
      else if (k === "ArrowLeft") e.shiftKey ? pl.prev() : pl.seekBy(-5);
      else if (k === "ArrowUp") pl.setVolume(pl.volume + 0.05);
      else if (k === "ArrowDown") pl.setVolume(pl.volume - 0.05);
      else if (k === "n" || k === "N") (pl.current ? (nowOpen ? nav(-1) : nav("/music/now")) : undefined);
      else if (k === "q" || k === "Q") setQueueOpen((o) => !o);
      else if (k === "m" || k === "M") pl.toggleMute();
      else if (k === "s" || k === "S") pl.toggleShuffle();
      else if (k === "r" || k === "R") pl.cycleRepeat();
      else if (k === "l" || k === "L") pl.current && libRef.current.actions.toggleLike(pl.current);
      else if (k === "/") searchRef.current?.focus();
      else if (k === "," || k === "<") pl.setSpeed(pl.speed - 0.25);
      else if (k === "." || k === ">") pl.setSpeed(pl.speed + 0.25);
      else used = false;
      if (used) e.preventDefault();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [nav, nowOpen]);

  const onSearch = (v: string) => {
    setQ(v);
    const target = `/music/search?q=${encodeURIComponent(v)}`;
    if (loc.pathname.startsWith("/music/search")) nav(target, { replace: true });
    else if (v.trim()) nav(target);
  };
  useEffect(() => {
    if (!loc.pathname.startsWith("/music/search")) setQ("");
  }, [loc.pathname]);

  const tools = useMemo(
    () =>
      createMusicTools({
        current: () => pRef.current.current,
        queueNext: (t) => pRef.current.playNext(t),
        queueEnd: (t) => pRef.current.addToQueue(t),
        playNow: (l) => pRef.current.playQueue(l, 0),
        onPlaylistCreated: () => void libRef.current.refreshPlaylists(),
      }),
    [],
  );
  const aiContext = useCallback(() => ({ label: `Music · ${loc.pathname.replace("/music", "") || "home"}`, text: pRef.current.current ? `Now playing: ${pRef.current.current.title} by ${pRef.current.current.artist}` : undefined }), [loc.pathname]);

  const railItem = (n: (typeof NAV)[number]) => (
    <Box
      key={n.to}
      component={NavLink}
      to={n.to}
      end={n.end}
      data-testid={`rail-${n.label.toLowerCase()}`}
      sx={{
        display: "flex",
        alignItems: "center",
        gap: 1.5,
        px: 1.5,
        py: 1,
        borderRadius: "12px",
        textDecoration: "none",
        color: "text.secondary",
        fontWeight: 600,
        fontSize: "sm",
        "&:hover": { bgcolor: "background.level1" },
        "&.active": { bgcolor: "primary.softBg", color: "primary.solidBg" },
        "& svg": { fontSize: 22 },
      }}
    >
      {n.icon}
      {n.label}
    </Box>
  );

  return (
    <>
      <Header user={user} />
      <Box sx={{ display: "flex", alignItems: "flex-start" }}>
        {/* Left rail (desktop) */}
        <Box
          component="nav"
          aria-label="Music sections"
          sx={{
            display: { xs: "none", md: "flex" },
            flexDirection: "column",
            gap: 0.5,
            width: 236,
            flexShrink: 0,
            position: "sticky",
            top: "var(--ad-topbar-h, 56px)",
            height: "calc(100vh - var(--ad-topbar-h, 56px))",
            overflowY: "auto",
            p: 1.5,
            pb: hasPlayer ? `${PLAYER_H + 12}px` : 2,
            borderRight: "1px solid",
            borderColor: "divider",
            bgcolor: "background.surface",
          }}
        >
          <Typography level="title-lg" sx={{ px: 1.5, py: 1, fontWeight: 800, display: "flex", alignItems: "center", gap: 1 }}>
            <Box sx={{ width: 26, height: 26, borderRadius: "8px", bgcolor: "primary.solidBg", color: "#fff", display: "flex", alignItems: "center", justifyContent: "center" }}>
              <LibraryMusicRoundedIcon sx={{ fontSize: 16 }} />
            </Box>
            Music
          </Typography>
          {NAV.map(railItem)}
          {railItem({ to: "/music/radio", label: "Radio", icon: <RadioIcon /> })}
          <Box sx={{ display: "flex", alignItems: "center", mt: 2, px: 1.5 }}>
            <Typography level="body-xs" sx={{ flex: 1, textTransform: "uppercase", letterSpacing: ".08em", fontWeight: 700, color: "text.tertiary" }}>
              Playlists
            </Typography>
            <IconButton size="sm" variant="plain" color="neutral" aria-label="New playlist" onClick={() => setCreating(true)}>
              <AddRoundedIcon />
            </IconButton>
          </Box>
          {(lib.playlists ?? []).slice(0, 8).map((pl) => (
            <Box key={pl.id} component={NavLink} to={`/music/playlists/${pl.id}`} sx={{ display: "flex", alignItems: "center", gap: 1.25, px: 1.5, py: 0.6, borderRadius: "10px", textDecoration: "none", color: "text.secondary", fontSize: "sm", "&:hover": { bgcolor: "background.level1" }, "&.active": { bgcolor: "primary.softBg", color: "primary.solidBg" } }}>
              <Cover url={pl.cover_art[0]} name={pl.name} size={26} radius={6} />
              <Box component="span" sx={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                {pl.name}
              </Box>
            </Box>
          ))}
          <Box sx={{ mt: "auto", pt: 2 }}>
            <Button size="sm" variant="plain" color="neutral" startDecorator={<SpeakerGroupRoundedIcon />} onClick={() => setDevices(true)}>
              Speakers
            </Button>
          </Box>
        </Box>

        {/* Main */}
        <Box sx={{ flex: 1, minWidth: 0, pb: hasPlayer ? { xs: `${PLAYER_H + 72}px`, md: `${PLAYER_H + 24}px` } : { xs: "72px", md: 3 }, pr: { md: queueOpen ? "380px" : 0 } }}>
          <Box
            sx={{
              position: "sticky",
              top: "var(--ad-topbar-h, 56px)",
              zIndex: 20,
              display: "flex",
              alignItems: "center",
              gap: 1.5,
              px: { xs: 1.5, md: 3 },
              py: 1.25,
              bgcolor: "rgba(246,248,247,.92)",
              backdropFilter: "blur(8px)",
              borderBottom: "1px solid",
              borderColor: "divider",
            }}
          >
            <Input
              slotProps={{ input: { ref: searchRef, "aria-label": "Search music" } }}
              startDecorator={<SearchIcon />}
              endDecorator={q ? <IconButton size="sm" variant="plain" aria-label="Clear search" onClick={() => onSearch("")}><CloseRoundedIcon /></IconButton> : undefined}
              placeholder="Search songs, albums, artists, playlists"
              value={q}
              onChange={(e) => onSearch(e.target.value)}
              data-testid="music-search-input"
              sx={{ flex: 1, maxWidth: 560, borderRadius: "999px", "--Input-focusedThickness": "2px" }}
            />
            <Box sx={{ ml: "auto" }}>
              <BizAssistant app="music" title="Music" tools={tools} suggestions={MUSIC_SUGGESTIONS} systemHint={MUSIC_HINT} getContext={aiContext} />
            </Box>
          </Box>
          <Box sx={{ px: { xs: 1.5, md: 3 }, py: { xs: 2, md: 3 }, maxWidth: 1320 }}>
            <Routes>
              <Route path="/" element={<HomePage />} />
              <Route path="/explore" element={<ExplorePage />} />
              <Route path="/library" element={<LibraryPage />} />
              <Route path="/playlists" element={<PlaylistsPage />} />
              <Route path="/playlists/:id" element={<PlaylistPage />} />
              <Route path="/smart/:kind" element={<SmartPlaylistPage />} />
              <Route path="/album/:key" element={<AlbumPage />} />
              <Route path="/artist/:name" element={<ArtistPage />} />
              <Route path="/search" element={<SearchPage />} />
              <Route path="/upload" element={<UploadPage />} />
              <Route path="/now" element={<NowPlaying />} />
              <Route
                path="/radio"
                element={
                  <>
                    <PageTitle title="Radio" sub="Live stations. Songs are recorded as you listen." />
                    <RadioStations query="" />
                  </>
                }
              />
              <Route path="*" element={<Navigate to="/music" replace />} />
            </Routes>
          </Box>
        </Box>
      </Box>

      {/* Phone bottom navigation */}
      <Box
        component="nav"
        aria-label="Music sections"
        sx={{ display: { xs: "flex", md: "none" }, position: "fixed", bottom: 0, left: 0, right: 0, height: 56, zIndex: 1110, bgcolor: "background.surface", borderTop: "1px solid", borderColor: "divider" }}
      >
        {NAV.map((n) => (
          <Box key={n.to} component={NavLink} to={n.to} end={n.end} sx={{ flex: 1, display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", textDecoration: "none", color: "text.tertiary", fontSize: 11, fontWeight: 600, "&.active": { color: "primary.solidBg" }, "& svg": { fontSize: 22 } }}>
            {n.icon}
            {n.label}
          </Box>
        ))}
      </Box>

      {queueOpen && p.queue.length > 0 && <QueuePanel onClose={() => setQueueOpen(false)} />}
      <PlayerBar queueOpen={queueOpen} onToggleQueue={() => setQueueOpen((o) => !o)} nowOpen={nowOpen} onOpenNow={() => (nowOpen ? nav(-1) : nav("/music/now"))} />
      <DevicesDialog open={devices} onClose={() => setDevices(false)} />
      {creating && (
        <PlaylistFormDialog
          onClose={() => setCreating(false)}
          onSave={async (v) => {
            setCreating(false);
            const id = await lib.newPlaylist(v.name, v.description);
            nav(`/music/playlists/${id}`);
          }}
        />
      )}
    </>
  );
}
