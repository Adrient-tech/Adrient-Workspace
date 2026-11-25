import { useEffect, useMemo, useState } from "react";
import { Box, Typography, Button, CircularProgress } from "@mui/joy";
import PlayArrowRoundedIcon from "@mui/icons-material/PlayArrowRounded";
import ShuffleRoundedIcon from "@mui/icons-material/ShuffleRounded";
import UploadFileRoundedIcon from "@mui/icons-material/UploadFileRounded";
import { useNavigate } from "react-router-dom";
import { useLibrary } from "./library";
import { usePlayer } from "./player";
import { getHistory } from "./api";
import { albumKey, artistOf, buildMixes, groupAlbums, quickPicks, shuffled } from "./logic";
import { Cover, EmptyNote, MediaCard, Shelf, TrackMenu } from "./ui";
import { PlaylistShelf } from "./PlaylistsPage";
import type { Track, HistoryPlay } from "./types";

function greeting(): string {
  const h = new Date().getHours();
  return h < 5 ? "Late night listening" : h < 12 ? "Good morning" : h < 18 ? "Good afternoon" : "Good evening";
}

/** HomePage: the shelves (quick picks, recently played, mixes, new uploads, albums). */
export function HomePage() {
  const lib = useLibrary();
  const p = usePlayer();
  const nav = useNavigate();
  const [history, setHistory] = useState<HistoryPlay[] | null>(null);
  useEffect(() => {
    getHistory(60)
      .then(setHistory)
      .catch(() => setHistory([]));
  }, []);

  const tracks = lib.tracks;
  const byId = useMemo(() => new Map((tracks ?? []).map((t) => [t.id, t])), [tracks]);
  const picks = useMemo(() => quickPicks(tracks ?? [], 12), [tracks]);
  const recent = useMemo(() => {
    const seen = new Set<string>();
    const out: Track[] = [];
    for (const h of history ?? []) {
      const t = byId.get(h.track_id);
      if (t && !seen.has(t.id)) {
        seen.add(t.id);
        out.push(t);
      }
    }
    return out.slice(0, 14);
  }, [history, byId]);
  const mixes = useMemo(() => buildMixes(tracks ?? []), [tracks]);
  const fresh = useMemo(() => (tracks ?? []).slice().sort((a, b) => b.created_at.localeCompare(a.created_at)).slice(0, 14), [tracks]);
  const albums = useMemo(() => {
    const plays = (a: { tracks: Track[] }) => a.tracks.reduce((s, t) => s + (t.play_count ?? 0), 0);
    return groupAlbums(tracks ?? []).sort((a, b) => plays(b) - plays(a) || a.name.localeCompare(b.name)).slice(0, 14);
  }, [tracks]);

  if (tracks === null) {
    return (
      <Box sx={{ display: "flex", justifyContent: "center", py: 10 }}>
        <CircularProgress />
      </Box>
    );
  }
  if (tracks.length === 0) {
    return (
      <EmptyNote
        title="Your music library is empty"
        text="Upload songs from your computer or add them from Drive. Tags and cover art are read from the files, and a .lrc file next to a song gives you synced lyrics."
        action={
          <Button startDecorator={<UploadFileRoundedIcon />} onClick={() => nav("/music/upload")}>
            Add music
          </Button>
        }
      />
    );
  }

  const play = (list: Track[], i = 0) => p.playQueue(list, i);
  const isPlayingList = (list: Track[]) => p.playing && list.some((t) => t.id === p.current?.id) && p.queue.length === list.length;

  return (
    <Box data-testid="music-home">
      <Box sx={{ display: "flex", alignItems: "flex-end", flexWrap: "wrap", gap: 2, mb: 3 }}>
        <Box sx={{ flex: 1, minWidth: 220 }}>
          <Typography level="body-sm" sx={{ color: "text.tertiary" }}>
            {greeting()}
          </Typography>
          <Typography level="h1" sx={{ fontSize: { xs: "xl3", sm: "xl4" }, letterSpacing: "-0.02em" }}>
            What are we listening to?
          </Typography>
        </Box>
        <Box sx={{ display: "flex", gap: 1 }}>
          <Button startDecorator={<PlayArrowRoundedIcon />} onClick={() => play(picks)}>
            Play quick picks
          </Button>
          <Button variant="outlined" color="neutral" startDecorator={<ShuffleRoundedIcon />} onClick={() => play(shuffled(tracks))}>
            Shuffle all
          </Button>
        </Box>
      </Box>

      <Shelf title="Quick picks" subtitle="Start here" testId="shelf-quick">
        <Box
          sx={{
            display: "grid",
            gridAutoFlow: "column",
            gridTemplateRows: "repeat(4, 56px)",
            gridAutoColumns: { xs: "86%", sm: "minmax(300px, 380px)" },
            gap: "4px 16px",
            width: "100%",
          }}
        >
          {picks.map((t, i) => (
            <Box
              key={t.id}
              data-testid={`quick-${t.id}`}
              sx={{ display: "flex", alignItems: "center", gap: 1.5, px: 0.5, borderRadius: "10px", cursor: "pointer", "&:hover": { bgcolor: "background.level1" } }}
              onClick={() => (p.current?.id === t.id ? p.toggle() : play(picks, i))}
            >
              <Cover url={t.art_url} name={t.album || t.title} size={48} radius={8} />
              <Box sx={{ minWidth: 0, flex: 1 }}>
                <Typography level="title-sm" noWrap sx={{ fontWeight: 600 }}>
                  {t.title}
                </Typography>
                <Typography level="body-xs" noWrap sx={{ color: "text.tertiary" }}>
                  {artistOf(t)}
                  {t.album ? ` · ${t.album}` : ""}
                </Typography>
              </Box>
              <Box onClick={(e) => e.stopPropagation()}>
                <TrackMenu track={t} onGoAlbum={() => nav(`/music/album/${encodeURIComponent(albumKey(t))}`)} onGoArtist={() => nav(`/music/artist/${encodeURIComponent(artistOf(t))}`)} />
              </Box>
            </Box>
          ))}
        </Box>
      </Shelf>

      {recent.length > 0 && (
        <Shelf title="Recently played" subtitle="Jump back in" testId="shelf-recent" action={<Button size="sm" variant="plain" onClick={() => nav("/music/library?tab=history")}>History</Button>}>
          {recent.map((t, i) => (
            <MediaCard key={t.id} art={t.art_url} title={t.title} sub={artistOf(t)} playing={p.playing && p.current?.id === t.id} onPlay={() => (p.current?.id === t.id ? p.toggle() : play(recent, i))} onOpen={() => nav(`/music/album/${encodeURIComponent(albumKey(t))}`)} coverName={t.album || t.title} />
          ))}
        </Shelf>
      )}

      {mixes.length > 0 && (
        <Shelf title="Mixes for you" subtitle="By genre and artist" testId="shelf-mixes">
          {mixes.map((m) => (
            <MediaCard key={m.key} art={m.tracks.find((t) => t.art_url)?.art_url} title={m.name} sub={m.sub} playing={isPlayingList(m.tracks)} onPlay={() => play(m.tracks)} coverName={m.name} />
          ))}
        </Shelf>
      )}

      <Shelf title="New uploads" subtitle="Added to the library" testId="shelf-new" action={<Button size="sm" variant="plain" onClick={() => nav("/music/smart/recently_added")}>See all</Button>}>
        {fresh.map((t, i) => (
          <MediaCard key={t.id} art={t.art_url} title={t.title} sub={artistOf(t)} playing={p.playing && p.current?.id === t.id} onPlay={() => (p.current?.id === t.id ? p.toggle() : play(fresh, i))} onOpen={() => nav(`/music/album/${encodeURIComponent(albumKey(t))}`)} coverName={t.album || t.title} />
        ))}
      </Shelf>

      <PlaylistShelf playlists={lib.playlists ?? []} />

      <Shelf title="Albums for you" subtitle="From your library" testId="shelf-albums" action={<Button size="sm" variant="plain" onClick={() => nav("/music/library?tab=albums")}>All albums</Button>}>
        {albums.map((a) => (
          <MediaCard key={a.key} art={a.tracks.find((t) => t.art_url)?.art_url} title={a.name} sub={`${a.sub} · ${a.tracks.length} songs`} playing={isPlayingList(a.tracks)} onPlay={() => play(a.tracks)} onOpen={() => nav(`/music/album/${encodeURIComponent(a.key)}`)} />
        ))}
      </Shelf>
    </Box>
  );
}
