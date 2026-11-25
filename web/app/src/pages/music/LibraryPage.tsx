import { useEffect, useMemo, useState } from "react";
import { Box, Typography, Button, Select, Option, CircularProgress } from "@mui/joy";
import PlayArrowRoundedIcon from "@mui/icons-material/PlayArrowRounded";
import ShuffleRoundedIcon from "@mui/icons-material/ShuffleRounded";
import HistoryRoundedIcon from "@mui/icons-material/HistoryRounded";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import { useLibrary } from "./library";
import { usePlayer } from "./player";
import { getHistory, getSmart } from "./api";
import { albumKey, artistOf, groupAlbums, groupArtists, humanDuration, shuffled, totalDuration } from "./logic";
import { ChipRow, Cover, EmptyNote, MediaCard, PageTitle, SongRow } from "./ui";
import type { HistoryPlay, Track } from "./types";

type Tab = "songs" | "albums" | "artists" | "liked" | "history";
const TABS: { value: Tab; label: string }[] = [
  { value: "songs", label: "Songs" },
  { value: "albums", label: "Albums" },
  { value: "artists", label: "Artists" },
  { value: "liked", label: "Liked" },
  { value: "history", label: "History" },
];

type SortKey = "added" | "title" | "artist" | "plays";

/** SongList renders a list with a play/shuffle header. */
export function SongList({ tracks, showAlbum = true, numbered = false }: { tracks: Track[]; showAlbum?: boolean; numbered?: boolean }) {
  const p = usePlayer();
  const nav = useNavigate();
  return (
    <Box>
      {tracks.map((t, i) => (
        <SongRow
          key={t.id}
          track={t}
          index={numbered ? i : undefined}
          showAlbum={showAlbum}
          onPlay={() => p.playQueue(tracks, i)}
          onGoAlbum={() => nav(`/music/album/${encodeURIComponent(albumKey(t))}`)}
          onGoArtist={() => nav(`/music/artist/${encodeURIComponent(artistOf(t))}`)}
        />
      ))}
    </Box>
  );
}

export function PlayShuffle({ tracks }: { tracks: Track[] }) {
  const p = usePlayer();
  return (
    <Box sx={{ display: "flex", gap: 1, alignItems: "center", flexWrap: "wrap" }}>
      <Button size="sm" startDecorator={<PlayArrowRoundedIcon />} disabled={!tracks.length} onClick={() => p.playQueue(tracks, 0)}>
        Play
      </Button>
      <Button size="sm" variant="outlined" color="neutral" startDecorator={<ShuffleRoundedIcon />} disabled={!tracks.length} onClick={() => p.playQueue(shuffled(tracks), 0)}>
        Shuffle
      </Button>
      <Typography level="body-xs" sx={{ color: "text.tertiary", ml: 0.5 }}>
        {tracks.length} song{tracks.length === 1 ? "" : "s"} · {humanDuration(totalDuration(tracks))}
      </Typography>
    </Box>
  );
}

/** LibraryPage: songs, albums, artists, liked and play history. */
export function LibraryPage() {
  const lib = useLibrary();
  const nav = useNavigate();
  const [sp, setSp] = useSearchParams();
  const tab = (TABS.find((t) => t.value === sp.get("tab"))?.value ?? "songs") as Tab;
  const [sort, setSort] = useState<SortKey>("added");
  const [history, setHistory] = useState<HistoryPlay[] | null>(null);
  const tracks = lib.tracks;

  useEffect(() => {
    if (tab === "history") getHistory(100).then(setHistory).catch(() => setHistory([]));
  }, [tab]);

  const sorted = useMemo(() => {
    const l = (tracks ?? []).slice();
    if (sort === "title") l.sort((a, b) => a.title.localeCompare(b.title));
    else if (sort === "artist") l.sort((a, b) => artistOf(a).localeCompare(artistOf(b)) || a.title.localeCompare(b.title));
    else if (sort === "plays") l.sort((a, b) => (b.play_count ?? 0) - (a.play_count ?? 0));
    else l.sort((a, b) => b.created_at.localeCompare(a.created_at));
    return l;
  }, [tracks, sort]);
  const liked = useMemo(() => (tracks ?? []).filter((t) => t.liked), [tracks]);
  const albums = useMemo(() => groupAlbums(tracks ?? []), [tracks]);
  const artists = useMemo(() => groupArtists(tracks ?? []), [tracks]);
  const p = usePlayer();

  if (tracks === null) return <Box sx={{ display: "flex", justifyContent: "center", py: 10 }}><CircularProgress /></Box>;

  return (
    <Box data-testid="music-library">
      <PageTitle title="Library" sub={`${tracks.length} songs, ${albums.length} albums, ${artists.length} artists`} />
      <Box sx={{ display: "flex", alignItems: "center", gap: 2, flexWrap: "wrap", mb: 2 }}>
        <ChipRow label="Library sections" value={tab} options={TABS} onChange={(v) => setSp(v === "songs" ? {} : { tab: v })} />
        {tab === "songs" && (
          <Select size="sm" value={sort} onChange={(_, v) => v && setSort(v)} sx={{ ml: "auto", minWidth: 170 }} slotProps={{ button: { "aria-label": "Sort songs" } }}>
            <Option value="added">Recently added</Option>
            <Option value="title">Title</Option>
            <Option value="artist">Artist</Option>
            <Option value="plays">Most played</Option>
          </Select>
        )}
      </Box>

      {tab === "songs" && (tracks.length ? (
        <>
          <Box sx={{ mb: 1.5 }}><PlayShuffle tracks={sorted} /></Box>
          <SongList tracks={sorted} />
        </>
      ) : (
        <EmptyNote title="No songs yet" text="Upload music or add it from Drive." action={<Button onClick={() => nav("/music/upload")}>Add music</Button>} />
      ))}

      {tab === "albums" && (
        <Box sx={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(150px, 1fr))", gap: 2.5 }}>
          {albums.map((a) => (
            <MediaCard key={a.key} width={150} art={a.tracks.find((t) => t.art_url)?.art_url} title={a.name} sub={a.sub} onPlay={() => p.playQueue(a.tracks, 0)} onOpen={() => nav(`/music/album/${encodeURIComponent(a.key)}`)} />
          ))}
        </Box>
      )}

      {tab === "artists" && (
        <Box sx={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(140px, 1fr))", gap: 2.5 }}>
          {artists.map((a) => (
            <Box key={a.key} sx={{ textAlign: "center", cursor: "pointer" }} onClick={() => nav(`/music/artist/${encodeURIComponent(a.name)}`)}>
              <Box sx={{ display: "flex", justifyContent: "center" }}>
                <Cover url={a.tracks.find((t) => t.art_url)?.art_url} name={a.name} size={120} round />
              </Box>
              <Typography level="title-sm" noWrap sx={{ mt: 1, fontWeight: 600 }}>{a.name}</Typography>
              <Typography level="body-xs" sx={{ color: "text.tertiary" }}>{a.sub}</Typography>
            </Box>
          ))}
        </Box>
      )}

      {tab === "liked" && (liked.length ? (
        <>
          <Box sx={{ mb: 1.5 }}><PlayShuffle tracks={liked} /></Box>
          <SongList tracks={liked} />
        </>
      ) : (
        <EmptyNote title="No liked songs yet" text="Tap the heart on a song and it appears here." />
      ))}

      {tab === "history" && (
        history === null ? (
          <Box sx={{ display: "flex", justifyContent: "center", py: 8 }}><CircularProgress /></Box>
        ) : history.length === 0 ? (
          <EmptyNote icon={<HistoryRoundedIcon />} title="No listening history yet" text="Songs you play for more than half a minute show up here." />
        ) : (
          <Box data-testid="history-list">
            {history.map((h) => {
              const t = tracks.find((x) => x.id === h.track_id);
              return (
                <Box key={h.play_id} sx={{ display: "flex", alignItems: "center", gap: 1.5, px: 1, py: 0.75, borderRadius: "10px", cursor: t ? "pointer" : "default", "&:hover": { bgcolor: "background.level1" } }} onClick={() => t && p.playQueue([t], 0)}>
                  <Cover url={h.art_url} name={h.album || h.title} size={44} radius={8} />
                  <Box sx={{ minWidth: 0, flex: 1 }}>
                    <Typography level="title-sm" noWrap sx={{ fontWeight: 600 }}>{h.title}</Typography>
                    <Typography level="body-xs" noWrap sx={{ color: "text.tertiary" }}>{h.artist}</Typography>
                  </Box>
                  <Typography level="body-xs" sx={{ color: "text.tertiary" }}>{new Date(h.played_at).toLocaleString([], { dateStyle: "medium", timeStyle: "short" })}</Typography>
                </Box>
              );
            })}
          </Box>
        )
      )}
    </Box>
  );
}

const SMART: Record<string, { title: string; sub: string }> = {
  most_played: { title: "Most played", sub: "The songs you play the most" },
  recently_played: { title: "Recently played", sub: "What you listened to lately" },
  recently_added: { title: "Recently added", sub: "The newest songs in the library" },
  liked: { title: "Liked songs", sub: "Everything you hearted" },
};
export const SMART_KINDS = Object.entries(SMART).map(([k, v]) => ({ kind: k, ...v }));

/** SmartPlaylistPage shows one computed list. */
export function SmartPlaylistPage() {
  const { kind = "" } = useParams();
  const meta = SMART[kind];
  const [tracks, setTracks] = useState<Track[] | null>(null);
  const lib = useLibrary();
  useEffect(() => {
    if (!meta) return;
    setTracks(null);
    getSmart(kind as "most_played", 100)
      .then(setTracks)
      .catch(() => setTracks([]));
  }, [kind, meta]);
  // Keep likes in sync with what the library knows.
  const merged = useMemo(() => (tracks ?? []).map((t) => lib.tracks?.find((x) => x.id === t.id) ?? t), [tracks, lib.tracks]);
  if (!meta) return <EmptyNote title="Unknown list" />;
  return (
    <Box data-testid="smart-page">
      <PageTitle title={meta.title} sub={meta.sub} />
      {tracks === null ? (
        <Box sx={{ display: "flex", justifyContent: "center", py: 8 }}><CircularProgress /></Box>
      ) : merged.length === 0 ? (
        <EmptyNote title="Nothing here yet" text="Play some songs and this list fills itself." />
      ) : (
        <>
          <Box sx={{ mb: 1.5 }}><PlayShuffle tracks={merged} /></Box>
          <SongList tracks={merged} numbered />
        </>
      )}
    </Box>
  );
}
