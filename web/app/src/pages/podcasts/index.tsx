/**
 * Stories — colleagues write stories, the server turns the text into audio, and
 * everyone listens: shelves on the home page, browse and search, a library, a
 * writing studio, and a player with read-along text. Route stays /podcasts.
 */
import { useEffect, useState } from "react";
import { Box } from "@mui/joy";
import { NavLink, Navigate, Route, Routes, useLocation, useNavigate } from "react-router-dom";
import SearchRounded from "@mui/icons-material/SearchRounded";
import { Header } from "../../components/Header";
import type { User } from "../../api/types";
import { MiniPlayer, PlayerProvider, usePlayer } from "./player";
import { C, FONT_DISPLAY, ToastProvider, TextInput } from "./ui";
import { EpisodeRedirect, HomePage, LibraryPage, SearchPage, ShowPage, ShowsPage } from "./pages";
import { EditorPage, WritePage } from "./Studio";
import { WebShowsPage } from "./WebShows";

const TABS: [string, string, string][] = [
  ["/podcasts", "Home", "nav-home"],
  ["/podcasts/shows", "Shows", "nav-shows"],
  ["/podcasts/library", "Library", "nav-library"],
  ["/podcasts/write", "Write", "nav-write"],
  ["/podcasts/web", "From the web", "nav-web"],
];

function SearchBox() {
  const nav = useNavigate();
  const loc = useLocation();
  const [q, setQ] = useState(() => (loc.pathname.endsWith("/search") ? new URLSearchParams(loc.search).get("q") ?? "" : ""));
  return (
    <Box component="form" role="search" onSubmit={(e: React.FormEvent) => { e.preventDefault(); if (q.trim()) nav(`/podcasts/search?q=${encodeURIComponent(q.trim())}`); }} sx={{ position: "relative", width: { xs: "100%", md: 300 } }}>
      <SearchRounded sx={{ position: "absolute", left: 11, top: 9, fontSize: 20, color: C.faint, pointerEvents: "none" }} />
      <TextInput value={q} onChange={(e) => setQ(e.target.value)} placeholder="Search stories and authors" aria-label="Search stories" testId="story-search" style={{ paddingLeft: 38, borderRadius: 999 }} />
    </Box>
  );
}

function Shell({ user }: { user: User | null }) {
  const p = usePlayer();
  return (
    <Box sx={{ minHeight: "100vh", background: C.bg, color: C.ink, colorScheme: "light", pb: p.ep ? 14 : 6 }} data-testid="stories-app">
      <Header user={user} />
      <Box sx={{ maxWidth: 1240, mx: "auto", px: { xs: 2, md: 4 }, pt: 3 }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 3, flexWrap: "wrap", mb: 3.5 }}>
          <Box sx={{ fontFamily: FONT_DISPLAY, fontWeight: 800, fontSize: 24, letterSpacing: "-0.04em", color: C.accent, mr: 1 }}>Stories</Box>
          <Box component="nav" aria-label="Stories sections" sx={{ display: "flex", gap: 0.4, overflowX: "auto", flex: 1, minWidth: 0 }}>
            {TABS.map(([to, label, id]) => (
              <NavLink key={to} to={to} end={to === "/podcasts"} data-testid={id} style={({ isActive }) => ({
                padding: "7px 14px", borderRadius: 999, fontSize: 14.5, fontWeight: 650, textDecoration: "none", whiteSpace: "nowrap",
                background: isActive ? C.ink : "transparent", color: isActive ? "#fff" : C.ink,
              })}>{label}</NavLink>
            ))}
          </Box>
          <SearchBox />
        </Box>
        <Routes>
          <Route index element={<HomePage />} />
          <Route path="shows" element={<ShowsPage />} />
          <Route path="show/:id" element={<ShowPage />} />
          <Route path="episode/:id" element={<EpisodeRedirect />} />
          <Route path="library" element={<LibraryPage />} />
          <Route path="search" element={<SearchPage />} />
          <Route path="write" element={<WritePage />} />
          <Route path="write/:showId" element={<EditorPage />} />
          <Route path="web" element={<WebShowsPage />} />
          <Route path="*" element={<Navigate to="/podcasts" replace />} />
        </Routes>
      </Box>
      <MiniPlayer />
    </Box>
  );
}

export default function PodcastsApp({ user }: { user: User | null }) {
  useEffect(() => { document.title = "Stories"; }, []);
  return (
    <ToastProvider>
      <PlayerProvider>
        <Shell user={user} />
      </PlayerProvider>
    </ToastProvider>
  );
}
