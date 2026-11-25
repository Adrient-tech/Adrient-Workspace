import { useMemo } from "react";
import { Box, Typography, CircularProgress } from "@mui/joy";
import QueueMusicRoundedIcon from "@mui/icons-material/QueueMusicRounded";
import RadioIcon from "@mui/icons-material/Radio";
import TrendingUpRoundedIcon from "@mui/icons-material/TrendingUpRounded";
import NewReleasesRoundedIcon from "@mui/icons-material/NewReleasesRounded";
import FavoriteRoundedIcon from "@mui/icons-material/FavoriteRounded";
import HistoryRoundedIcon from "@mui/icons-material/HistoryRounded";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import { useLibrary } from "./library";
import { usePlayer } from "./player";
import { artistOf, groupAlbums, groupArtists, humanDuration, matchesQuery, totalDuration } from "./logic";
import { ChipRow, Cover, coverColors, EmptyNote, MediaCard, PageTitle, Shelf, SongRow } from "./ui";
import { PlayShuffle, SMART_KINDS, SongList } from "./LibraryPage";

const SMART_ICONS: Record<string, JSX.Element> = {
  most_played: <TrendingUpRoundedIcon />,
  recently_played: <HistoryRoundedIcon />,
  recently_added: <NewReleasesRoundedIcon />,
  liked: <FavoriteRoundedIcon />,
};

/** ExplorePage: genres, smart playlists, new albums and the radio. */
export function ExplorePage() {
  const lib = useLibrary();
  const nav = useNavigate();
  const p = usePlayer();
  const genres = useMemo(() => {
    const m = new Map<string, number>();
    for (const t of lib.tracks ?? []) if (t.genre) m.set(t.genre, (m.get(t.genre) ?? 0) + 1);
    return [...m.entries()].sort((a, b) => b[1] - a[1]);
  }, [lib.tracks]);
  const newest = useMemo(() => groupAlbums(lib.tracks ?? []).sort((a, b) => Math.max(...b.tracks.map((t) => t.year ?? 0)) - Math.max(...a.tracks.map((t) => t.year ?? 0))), [lib.tracks]);
  if (lib.tracks === null) return <Box sx={{ display: "flex", justifyContent: "center", py: 10 }}><CircularProgress /></Box>;
  return (
    <Box data-testid="music-explore">
      <PageTitle title="Explore" sub="Browse by genre, smart lists and new albums" />
      <Typography level="title-lg" sx={{ mb: 1.5 }}>Browse genres</Typography>
      {genres.length === 0 ? (
        <Typography level="body-sm" sx={{ color: "text.tertiary", mb: 3 }}>Genres appear once songs carry a genre tag.</Typography>
      ) : (
        <Box sx={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(180px, 1fr))", gap: 1.5, mb: 4 }}>
          {genres.map(([g, n]) => {
            const [a, b] = coverColors(g);
            return (
              <Box
                key={g}
                role="button"
                tabIndex={0}
                data-testid={`genre-${g}`}
                onClick={() => nav(`/music/search?q=${encodeURIComponent(g)}`)}
                onKeyDown={(e) => e.key === "Enter" && nav(`/music/search?q=${encodeURIComponent(g)}`)}
                sx={{ height: 84, borderRadius: "14px", p: 1.75, cursor: "pointer", color: "#fff", background: `linear-gradient(135deg, ${b}, ${a})`, display: "flex", flexDirection: "column", justifyContent: "flex-end", transition: "transform .12s", "&:hover": { transform: "translateY(-2px)" } }}
              >
                <Typography level="title-md" sx={{ color: "#0E2420", fontWeight: 800 }}>{g}</Typography>
                <Typography level="body-xs" sx={{ color: "rgba(14,36,32,.7)" }}>{n} songs</Typography>
              </Box>
            );
          })}
        </Box>
      )}
      <Typography level="title-lg" sx={{ mb: 1.5 }}>Smart playlists</Typography>
      <Box sx={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(220px, 1fr))", gap: 1.5, mb: 4 }}>
        {SMART_KINDS.map((s) => (
          <Box key={s.kind} role="button" tabIndex={0} data-testid={`smart-${s.kind}`} onClick={() => nav(`/music/smart/${s.kind}`)} onKeyDown={(e) => e.key === "Enter" && nav(`/music/smart/${s.kind}`)} sx={{ display: "flex", alignItems: "center", gap: 1.5, p: 1.5, border: "1px solid", borderColor: "divider", borderRadius: "14px", cursor: "pointer", bgcolor: "background.surface", "&:hover": { bgcolor: "background.level1" } }}>
            <Box sx={{ width: 44, height: 44, borderRadius: "12px", bgcolor: "primary.softBg", color: "primary.solidBg", display: "flex", alignItems: "center", justifyContent: "center" }}>{SMART_ICONS[s.kind]}</Box>
            <Box sx={{ minWidth: 0 }}>
              <Typography level="title-sm" sx={{ fontWeight: 700 }}>{s.title}</Typography>
              <Typography level="body-xs" sx={{ color: "text.tertiary" }} noWrap>{s.sub}</Typography>
            </Box>
          </Box>
        ))}
        <Box role="button" tabIndex={0} onClick={() => nav("/music/radio")} onKeyDown={(e) => e.key === "Enter" && nav("/music/radio")} sx={{ display: "flex", alignItems: "center", gap: 1.5, p: 1.5, border: "1px solid", borderColor: "divider", borderRadius: "14px", cursor: "pointer", "&:hover": { bgcolor: "background.level1" } }}>
          <Box sx={{ width: 44, height: 44, borderRadius: "12px", bgcolor: "warning.softBg", color: "warning.solidBg", display: "flex", alignItems: "center", justifyContent: "center" }}><RadioIcon /></Box>
          <Box>
            <Typography level="title-sm" sx={{ fontWeight: 700 }}>Live radio</Typography>
            <Typography level="body-xs" sx={{ color: "text.tertiary" }}>Stations, recorded as you listen</Typography>
          </Box>
        </Box>
      </Box>
      {newest.length > 0 && (
        <Shelf title="Newest albums" subtitle="By release year">
          {newest.slice(0, 14).map((a) => (
            <MediaCard key={a.key} art={a.tracks.find((t) => t.art_url)?.art_url} title={a.name} sub={`${a.sub}${a.tracks[0]?.year ? ` · ${a.tracks[0].year}` : ""}`} onPlay={() => p.playQueue(a.tracks, 0)} onOpen={() => nav(`/music/album/${encodeURIComponent(a.key)}`)} />
          ))}
        </Shelf>
      )}
    </Box>
  );
}

function Hero({ art, name, kind, title, sub, round, children }: { art?: string; name: string; kind: string; title: string; sub: string; round?: boolean; children: React.ReactNode }) {
  const [a] = coverColors(name);
  return (
    <Box sx={{ display: "flex", gap: { xs: 2, md: 3.5 }, alignItems: "flex-end", flexWrap: { xs: "wrap", sm: "nowrap" }, p: { xs: 2, md: 3 }, mb: 3, borderRadius: "20px", background: `linear-gradient(135deg, ${a.replace("72%", "90%")}, #fff 70%)`, border: "1px solid", borderColor: "divider" }}>
      <Cover url={art} name={name} size={{ xs: 132, md: 190 }} radius={16} round={round} />
      <Box sx={{ minWidth: 0 }}>
        <Typography level="body-xs" sx={{ textTransform: "uppercase", letterSpacing: ".08em", fontWeight: 700, color: "text.tertiary" }}>{kind}</Typography>
        <Typography level="h1" sx={{ fontSize: { xs: "xl3", md: "xl4" }, letterSpacing: "-0.02em", lineHeight: 1.1 }}>{title}</Typography>
        <Typography level="body-sm" sx={{ color: "text.secondary", mt: 0.5, mb: 1.5 }}>{sub}</Typography>
        {children}
      </Box>
    </Box>
  );
}

/** AlbumPage: an album built from file tags. */
export function AlbumPage() {
  const { key = "" } = useParams();
  const lib = useLibrary();
  const nav = useNavigate();
  const p = usePlayer();
  const album = useMemo(() => groupAlbums(lib.tracks ?? []).find((g) => g.key === decodeURIComponent(key)), [lib.tracks, key]);
  if (lib.tracks === null) return <Box sx={{ display: "flex", justifyContent: "center", py: 10 }}><CircularProgress /></Box>;
  if (!album) return <EmptyNote title="Album not found" text="It may have been removed from the library." />;
  const year = Math.max(...album.tracks.map((t) => t.year ?? 0));
  return (
    <Box data-testid="album-page">
      <Hero art={album.tracks.find((t) => t.art_url)?.art_url} name={album.name} kind="Album" title={album.name} sub={`${album.sub}${year ? ` · ${year}` : ""} · ${album.tracks.length} songs · ${humanDuration(totalDuration(album.tracks))}`}>
        <PlayShuffle tracks={album.tracks} />
      </Hero>
      <Box>
        {album.tracks.map((t, i) => (
          <SongRow key={t.id} track={t} index={i} showAlbum={false} showArt={false} onPlay={() => p.playQueue(album.tracks, i)} onGoArtist={() => nav(`/music/artist/${encodeURIComponent(artistOf(t))}`)} />
        ))}
      </Box>
    </Box>
  );
}

/** ArtistPage: top songs and albums of one artist. */
export function ArtistPage() {
  const { name = "" } = useParams();
  const lib = useLibrary();
  const nav = useNavigate();
  const p = usePlayer();
  const artist = useMemo(() => groupArtists(lib.tracks ?? []).find((g) => g.name === decodeURIComponent(name)), [lib.tracks, name]);
  const top = useMemo(() => (artist ? artist.tracks.slice().sort((a, b) => (b.play_count ?? 0) - (a.play_count ?? 0) || a.title.localeCompare(b.title)) : []), [artist]);
  const albums = useMemo(() => (artist ? groupAlbums(artist.tracks) : []), [artist]);
  if (lib.tracks === null) return <Box sx={{ display: "flex", justifyContent: "center", py: 10 }}><CircularProgress /></Box>;
  if (!artist) return <EmptyNote title="Artist not found" />;
  return (
    <Box data-testid="artist-page">
      <Hero art={artist.tracks.find((t) => t.art_url)?.art_url} name={artist.name} kind="Artist" title={artist.name} sub={`${artist.tracks.length} songs · ${albums.length} albums`} round>
        <PlayShuffle tracks={top} />
      </Hero>
      <Typography level="title-lg" sx={{ mb: 1 }}>Songs</Typography>
      <Box sx={{ mb: 4 }}>
        <SongList tracks={top.slice(0, 8)} numbered />
      </Box>
      <Shelf title="Albums">
        {albums.map((a) => (
          <MediaCard key={a.key} art={a.tracks.find((t) => t.art_url)?.art_url} title={a.name} sub={`${a.tracks[0]?.year || ""} · ${a.tracks.length} songs`} onPlay={() => p.playQueue(a.tracks, 0)} onOpen={() => nav(`/music/album/${encodeURIComponent(a.key)}`)} />
        ))}
      </Shelf>
    </Box>
  );
}

type SearchType = "songs" | "albums" | "artists" | "playlists";

/** SearchPage: results for the query, narrowed with chips. */
export function SearchPage() {
  const lib = useLibrary();
  const nav = useNavigate();
  const p = usePlayer();
  const [sp, setSp] = useSearchParams();
  const q = sp.get("q") ?? "";
  const type = (sp.get("type") as SearchType | null) ?? null;
  const songs = useMemo(() => (lib.tracks ?? []).filter((t) => matchesQuery(t, q)), [lib.tracks, q]);
  const albums = useMemo(() => groupAlbums(songs), [songs]);
  const artists = useMemo(() => groupArtists((lib.tracks ?? []).filter((t) => matchesQuery({ ...t, album: "", title: "", genre: "" }, q))), [lib.tracks, q]);
  const playlists = useMemo(() => (lib.playlists ?? []).filter((pl) => q.trim() === "" || `${pl.name} ${pl.description}`.toLowerCase().includes(q.trim().toLowerCase())), [lib.playlists, q]);
  const setType = (v: SearchType) => {
    const n = new URLSearchParams(sp);
    if (type === v) n.delete("type");
    else n.set("type", v);
    setSp(n, { replace: true });
  };
  const show = (t: SearchType) => type === null || type === t;
  const none = songs.length + artists.length + playlists.length === 0;
  return (
    <Box data-testid="music-search">
      <PageTitle title={q ? `Results for "${q}"` : "Search"} sub={q ? undefined : "Type in the search box to find songs, albums, artists and playlists"} />
      <Box sx={{ mb: 2.5 }}>
        <ChipRow<SearchType | "none"> label="Result types" value={type ?? "none"} options={[{ value: "songs", label: "Songs" }, { value: "albums", label: "Albums" }, { value: "artists", label: "Artists" }, { value: "playlists", label: "Playlists" }]} onChange={(v) => v !== "none" && setType(v)} />
      </Box>
      {lib.tracks === null && <CircularProgress />}
      {lib.tracks !== null && q && none && <EmptyNote title="No matches" text={`Nothing in the library matches "${q}". Try a shorter word or another spelling.`} />}
      {show("songs") && songs.length > 0 && (
        <Box sx={{ mb: 3 }} data-testid="results-songs">
          <Typography level="title-lg" sx={{ mb: 1 }}>Songs</Typography>
          <SongList tracks={type === "songs" ? songs : songs.slice(0, 8)} />
        </Box>
      )}
      {show("albums") && albums.length > 0 && (
        <Box sx={{ mb: 3 }} data-testid="results-albums">
          <Typography level="title-lg" sx={{ mb: 1.5 }}>Albums</Typography>
          <Box sx={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(150px, 1fr))", gap: 2.5 }}>
            {albums.map((a) => <MediaCard key={a.key} width={150} art={a.tracks.find((t) => t.art_url)?.art_url} title={a.name} sub={a.sub} onPlay={() => p.playQueue(a.tracks, 0)} onOpen={() => nav(`/music/album/${encodeURIComponent(a.key)}`)} />)}
          </Box>
        </Box>
      )}
      {show("artists") && artists.length > 0 && (
        <Box sx={{ mb: 3 }} data-testid="results-artists">
          <Typography level="title-lg" sx={{ mb: 1.5 }}>Artists</Typography>
          <Box sx={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(130px, 1fr))", gap: 2.5 }}>
            {artists.map((a) => (
              <Box key={a.key} sx={{ textAlign: "center", cursor: "pointer" }} onClick={() => nav(`/music/artist/${encodeURIComponent(a.name)}`)}>
                <Box sx={{ display: "flex", justifyContent: "center" }}><Cover url={a.tracks.find((t) => t.art_url)?.art_url} name={a.name} size={110} round /></Box>
                <Typography level="title-sm" noWrap sx={{ mt: 1, fontWeight: 600 }}>{a.name}</Typography>
                <Typography level="body-xs" sx={{ color: "text.tertiary" }}>{a.sub}</Typography>
              </Box>
            ))}
          </Box>
        </Box>
      )}
      {show("playlists") && playlists.length > 0 && (
        <Box sx={{ mb: 3 }} data-testid="results-playlists">
          <Typography level="title-lg" sx={{ mb: 1.5 }}>Playlists</Typography>
          <Box sx={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(150px, 1fr))", gap: 2.5 }}>
            {playlists.map((pl) => <MediaCard key={pl.id} width={150} art={pl.cover_art[0]} title={pl.name} sub={`${pl.track_count} songs`} onOpen={() => nav(`/music/playlists/${pl.id}`)} coverName={pl.name} />)}
          </Box>
        </Box>
      )}
      {!q && <Box sx={{ color: "text.tertiary", textAlign: "center", py: 6 }}><QueueMusicRoundedIcon sx={{ fontSize: 44 }} /></Box>}
    </Box>
  );
}
