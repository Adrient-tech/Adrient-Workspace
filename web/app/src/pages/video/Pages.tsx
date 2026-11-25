import { useEffect, useState } from "react";
import { Box, Typography, Button, CircularProgress, Select, Option, Input, Modal, ModalDialog, ModalClose, DialogTitle, IconButton } from "@mui/joy";
import HistoryRoundedIcon from "@mui/icons-material/HistoryRounded";
import WatchLaterRoundedIcon from "@mui/icons-material/WatchLaterRounded";
import SubscriptionsRoundedIcon from "@mui/icons-material/SubscriptionsRounded";
import PlaylistPlayRoundedIcon from "@mui/icons-material/PlaylistPlayRounded";
import AddRoundedIcon from "@mui/icons-material/AddRounded";
import DeleteOutlineRoundedIcon from "@mui/icons-material/DeleteOutlineRounded";
import VideocamRoundedIcon from "@mui/icons-material/VideocamRounded";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import {
  clearHistory, createVideoPlaylist, deleteVideoPlaylist, getChannel, getFeed, getHistory, getVideoInfo, getWatchLater, listSubscriptions, listVideoPlaylistVideos, listVideoPlaylists, removeFromVideoPlaylist, subscribe,
} from "./api";
import type { Channel, FeedParams, VideoInfo, VideoPlaylist } from "./types";
import { useVideoCtx } from "./ctx";
import { ChannelAvatar, EmptyNote, PageTitle, Poster, VideoCard, VideoGrid, metaLine } from "./ui";
import { MeetingsShelf, useMeetings } from "./HomePage";
import { formatCount } from "./logic";

function Loading() {
  return <Box sx={{ display: "flex", justifyContent: "center", py: 10 }}><CircularProgress /></Box>;
}

function useFeed(p: FeedParams) {
  const [v, setV] = useState<VideoInfo[] | null>(null);
  const key = JSON.stringify(p);
  useEffect(() => {
    setV(null);
    getFeed(p).then(setV).catch(() => setV([]));
  }, [key]); // eslint-disable-line react-hooks/exhaustive-deps
  return [v, setV] as const;
}

function Grid({ videos, setVideos }: { videos: VideoInfo[]; setVideos?: (f: (c: VideoInfo[] | null) => VideoInfo[] | null) => void }) {
  const ctx = useVideoCtx();
  return (
    <VideoGrid>
      {videos.map((v) => (
        <VideoCard key={v.id} v={v} onWatchLater={(x) => ctx.toggleWatchLater(x, (on) => setVideos?.((c) => (c ? c.map((y) => (y.id === x.id ? { ...y, watch_later: on } : y)) : c)))} onAddToPlaylist={ctx.addToPlaylist} />
      ))}
    </VideoGrid>
  );
}

/** SearchPage: results with date, duration and sort filters. */
export function SearchPage() {
  const nav = useNavigate();
  const [sp, setSp] = useSearchParams();
  const q = sp.get("q") ?? "";
  const duration = (sp.get("duration") ?? "") as FeedParams["duration"];
  const date = (sp.get("date") ?? "") as FeedParams["date"];
  const sort = (sp.get("sort") ?? "") as FeedParams["sort"];
  const [videos] = useFeed({ q, duration, date, sort });
  const set = (k: string, v: string | null) => {
    const n = new URLSearchParams(sp);
    if (v) n.set(k, v);
    else n.delete(k);
    setSp(n, { replace: true });
  };
  const sel = (label: string, value: string, key: string, opts: [string, string][]) => (
    <Select size="sm" value={value} onChange={(_, v) => set(key, v || null)} sx={{ minWidth: 150 }} slotProps={{ button: { "aria-label": label } }}>
      {opts.map(([v, l]) => <Option key={v} value={v}>{l}</Option>)}
    </Select>
  );
  return (
    <Box data-testid="video-search">
      <PageTitle title={q ? `Results for “${q}”` : "Search"} sub={videos ? `${videos.length} video${videos.length === 1 ? "" : "s"}` : undefined} />
      <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap", mb: 3 }}>
        {sel("Upload date", date ?? "", "date", [["", "Any time"], ["today", "Today"], ["week", "This week"], ["month", "This month"], ["year", "This year"]])}
        {sel("Duration", duration ?? "", "duration", [["", "Any length"], ["short", "Under 4 minutes"], ["medium", "4 to 20 minutes"], ["long", "Over 20 minutes"]])}
        {sel("Sort by", sort ?? "", "sort", [["", "Newest"], ["views", "Most viewed"], ["oldest", "Oldest"], ["title", "Title"]])}
      </Box>
      {videos === null ? <Loading /> : videos.length === 0 ? <EmptyNote title="No videos match" text="Try other words or remove a filter." /> : (
        <Box sx={{ display: "flex", flexDirection: "column", gap: 2.5 }}>
          {videos.map((v) => (
            <Box key={v.id} data-testid={`result-${v.id}`} onClick={() => nav(`/video/v/${v.id}`)} sx={{ display: "flex", gap: 2, cursor: "pointer", flexDirection: { xs: "column", sm: "row" } }}>
              <Box sx={{ width: { xs: "100%", sm: 320 }, flexShrink: 0 }}><Poster v={v} /></Box>
              <Box sx={{ minWidth: 0 }}>
                <Typography level="title-md" sx={{ fontWeight: 650 }}>{v.title}</Typography>
                <Typography level="body-xs" sx={{ color: "text.tertiary", mt: 0.25 }}>{metaLine(v)}</Typography>
                <Box sx={{ display: "flex", alignItems: "center", gap: 1, my: 1 }}><ChannelAvatar name={v.owner_name} size={24} /><Typography level="body-xs">{v.owner_name}</Typography></Box>
                <Typography level="body-sm" sx={{ color: "text.secondary", display: "-webkit-box", WebkitLineClamp: 2, WebkitBoxOrient: "vertical", overflow: "hidden" }}>{v.description}</Typography>
              </Box>
            </Box>
          ))}
        </Box>
      )}
    </Box>
  );
}

/** ChannelPage: one person's videos. */
export function ChannelPage() {
  const { userId = "" } = useParams();
  const ctx = useVideoCtx();
  const [ch, setCh] = useState<Channel | null>(null);
  const [videos, setVideos] = useFeed({ owner: userId });
  useEffect(() => {
    getChannel(userId).then(setCh).catch(() => setCh(null));
  }, [userId]);
  if (!ch) return <Loading />;
  const toggle = async () => {
    const on = !ch.subscribed;
    setCh({ ...ch, subscribed: on, subscribers: ch.subscribers + (on ? 1 : -1) });
    await subscribe(ch.user_id, on).catch(() => setCh(ch));
    ctx.bumpSubs();
  };
  return (
    <Box data-testid="channel-page">
      <Box sx={{ display: "flex", alignItems: "center", gap: 2.5, mb: 3, p: { xs: 1.5, md: 3 }, borderRadius: "20px", bgcolor: "background.level1", flexWrap: "wrap" }}>
        <ChannelAvatar name={ch.name} size={84} />
        <Box sx={{ flex: 1, minWidth: 200 }}>
          <Typography level="h2">{ch.name}</Typography>
          <Typography level="body-sm" sx={{ color: "text.tertiary" }}>{formatCount(ch.subscribers)} subscriber{ch.subscribers === 1 ? "" : "s"} · {ch.videos} video{ch.videos === 1 ? "" : "s"}</Typography>
        </Box>
        {!ch.mine && <Button color={ch.subscribed ? "neutral" : "primary"} variant={ch.subscribed ? "soft" : "solid"} onClick={toggle} sx={{ borderRadius: "999px" }}>{ch.subscribed ? "Subscribed" : "Subscribe"}</Button>}
      </Box>
      {videos === null ? <Loading /> : videos.length === 0 ? <EmptyNote title="No videos in this channel" /> : <Grid videos={videos} setVideos={setVideos} />}
    </Box>
  );
}

/** SubscriptionsPage: the channels you follow and their latest videos. */
export function SubscriptionsPage() {
  const nav = useNavigate();
  const ctx = useVideoCtx();
  const [chs, setChs] = useState<Channel[] | null>(null);
  const [videos, setVideos] = useFeed({ subs: true });
  useEffect(() => {
    listSubscriptions().then(setChs).catch(() => setChs([]));
  }, [ctx.subsVersion]);
  return (
    <Box data-testid="video-subscriptions">
      <PageTitle title="Subscriptions" icon={<SubscriptionsRoundedIcon />} sub="Latest videos from the channels you follow" />
      {chs && chs.length > 0 && (
        <Box sx={{ display: "flex", gap: 2.5, overflowX: "auto", pb: 1.5, mb: 2 }}>
          {chs.map((c) => (
            <Box key={c.user_id} onClick={() => nav(`/video/channel/${c.user_id}`)} sx={{ textAlign: "center", cursor: "pointer", width: 84, flexShrink: 0 }}>
              <ChannelAvatar name={c.name} size={64} />
              <Typography level="body-xs" noWrap sx={{ mt: 0.5 }}>{c.name}</Typography>
            </Box>
          ))}
        </Box>
      )}
      {videos === null ? <Loading /> : videos.length === 0 ? (
        <EmptyNote icon={<SubscriptionsRoundedIcon />} title="No subscriptions yet" text="Open a video and press Subscribe on its channel to follow it. New videos show up here." />
      ) : <Grid videos={videos} setVideos={setVideos} />}
    </Box>
  );
}

/** HistoryPage: videos you started, newest first. */
export function HistoryPage() {
  const [videos, setVideos] = useState<VideoInfo[] | null>(null);
  useEffect(() => {
    getHistory().then(setVideos).catch(() => setVideos([]));
  }, []);
  return (
    <Box data-testid="video-history">
      <PageTitle title="Watch history" icon={<HistoryRoundedIcon />} action={videos && videos.length > 0 ? <Button variant="outlined" color="neutral" size="sm" onClick={async () => { if (window.confirm("Clear your watch history? Saved positions are forgotten.")) { await clearHistory(); setVideos([]); } }}>Clear history</Button> : undefined} />
      {videos === null ? <Loading /> : videos.length === 0 ? <EmptyNote icon={<HistoryRoundedIcon />} title="No watch history" text="Videos you watch show up here, with your place saved." /> : <Grid videos={videos} setVideos={setVideos as never} />}
    </Box>
  );
}

/** WatchLaterPage: videos saved for later. */
export function WatchLaterPage() {
  const [videos, setVideos] = useState<VideoInfo[] | null>(null);
  const ctx = useVideoCtx();
  useEffect(() => {
    getWatchLater().then(setVideos).catch(() => setVideos([]));
  }, []);
  return (
    <Box data-testid="video-watch-later">
      <PageTitle title="Watch later" icon={<WatchLaterRoundedIcon />} sub={videos ? `${videos.length} saved` : undefined} />
      {videos === null ? <Loading /> : videos.length === 0 ? <EmptyNote icon={<WatchLaterRoundedIcon />} title="Nothing saved" text="Use “Save to Watch later” in a video's menu." /> : (
        <VideoGrid>
          {videos.map((v) => <VideoCard key={v.id} v={v} onWatchLater={(x) => ctx.toggleWatchLater(x, () => setVideos((c) => (c ? c.filter((y) => y.id !== x.id) : c)))} onAddToPlaylist={ctx.addToPlaylist} />)}
        </VideoGrid>
      )}
    </Box>
  );
}

/** MeetingsPage: every saved Meet recording. */
export function MeetingsPage() {
  const m = useMeetings();
  return (
    <Box data-testid="video-meetings">
      <PageTitle title="Meetings" icon={<VideocamRoundedIcon />} sub="Recordings of your Meet calls. Opening one adds it to the Video app as a private video." />
      {m.recs.length === 0 ? <EmptyNote icon={<VideocamRoundedIcon />} title="No recordings yet" text="Record a call in Meet and it will appear here once it is saved." /> : <MeetingsShelf recordings={m.recs} onOpen={m.open} busy={m.busy} />}
    </Box>
  );
}

/** PlaylistsPage: your playlists. */
export function PlaylistsPage() {
  const nav = useNavigate();
  const [pls, setPls] = useState<VideoPlaylist[] | null>(null);
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const load = () => listVideoPlaylists().then(setPls).catch(() => setPls([]));
  useEffect(() => void load(), []);
  return (
    <Box data-testid="video-playlists">
      <PageTitle title="Playlists" icon={<PlaylistPlayRoundedIcon />} action={<Button startDecorator={<AddRoundedIcon />} onClick={() => setOpen(true)} data-testid="new-video-playlist">New playlist</Button>} />
      {pls === null ? <Loading /> : pls.length === 0 ? <EmptyNote icon={<PlaylistPlayRoundedIcon />} title="No playlists" text="Group videos into a playlist to watch them in order." action={<Button onClick={() => setOpen(true)}>Create a playlist</Button>} /> : (
        <Box sx={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(240px, 1fr))", gap: 2.5 }}>
          {pls.map((p) => (
            <Box key={p.id} role="button" tabIndex={0} onClick={() => nav(`/video/playlist/${p.id}`)} onKeyDown={(e) => e.key === "Enter" && nav(`/video/playlist/${p.id}`)} sx={{ border: "1px solid", borderColor: "divider", borderRadius: "16px", p: 2, cursor: "pointer", "&:hover": { bgcolor: "background.level1" } }}>
              <Box sx={{ aspectRatio: "16/9", borderRadius: "12px", bgcolor: "primary.softBg", color: "primary.solidBg", display: "flex", alignItems: "center", justifyContent: "center", mb: 1.25 }}><PlaylistPlayRoundedIcon sx={{ fontSize: 44 }} /></Box>
              <Typography level="title-md" noWrap>{p.name}</Typography>
              <Typography level="body-xs" sx={{ color: "text.tertiary" }}>{p.item_count} video{p.item_count === 1 ? "" : "s"}</Typography>
            </Box>
          ))}
        </Box>
      )}
      {open && (
        <Modal open onClose={() => setOpen(false)}>
          <ModalDialog sx={{ width: { xs: "100vw", sm: 420 }, maxWidth: "100vw" }}>
            <ModalClose />
            <DialogTitle>New playlist</DialogTitle>
            <Input autoFocus placeholder="Playlist name" value={name} onChange={(e) => setName(e.target.value)} slotProps={{ input: { "aria-label": "Playlist name" } }} onKeyDown={(e) => e.key === "Enter" && name.trim() && document.getElementById("create-vpl")?.click()} />
            <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1, mt: 1 }}>
              <Button variant="plain" color="neutral" onClick={() => setOpen(false)}>Cancel</Button>
              <Button id="create-vpl" disabled={!name.trim()} onClick={async () => { const p = await createVideoPlaylist(name.trim()); setOpen(false); setName(""); nav(`/video/playlist/${p.id}`); }}>Create</Button>
            </Box>
          </ModalDialog>
        </Modal>
      )}
    </Box>
  );
}

/** PlaylistPage: the videos of one playlist, in order. */
export function PlaylistPage() {
  const { id = "" } = useParams();
  const nav = useNavigate();
  const [name, setName] = useState("");
  const [videos, setVideos] = useState<VideoInfo[] | null>(null);
  const load = async () => {
    const [pls, items] = await Promise.all([listVideoPlaylists(), listVideoPlaylistVideos(id)]);
    setName(pls.find((p) => p.id === id)?.name ?? "Playlist");
    const infos = await Promise.all(items.map((i) => getVideoInfo(i.id).catch(() => null)));
    setVideos(infos.filter((x): x is VideoInfo => !!x));
  };
  useEffect(() => {
    setVideos(null);
    load().catch(() => setVideos([]));
  }, [id]); // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <Box data-testid="video-playlist">
      <PageTitle title={name || "Playlist"} icon={<PlaylistPlayRoundedIcon />} sub={videos ? `${videos.length} videos` : undefined}
        action={<Box sx={{ display: "flex", gap: 1 }}>
          {videos && videos.length > 0 && <Button onClick={() => nav(`/video/v/${videos[0].id}`)}>Play all</Button>}
          <Button variant="outlined" color="danger" startDecorator={<DeleteOutlineRoundedIcon />} onClick={async () => { if (window.confirm(`Delete the playlist “${name}”? The videos stay.`)) { await deleteVideoPlaylist(id); nav("/video/playlists"); } }}>Delete</Button>
        </Box>} />
      {videos === null ? <Loading /> : videos.length === 0 ? <EmptyNote title="This playlist is empty" text="Open the menu of a video and choose Add to playlist." /> : (
        <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5, maxWidth: 900 }}>
          {videos.map((v, i) => (
            <Box key={v.id} sx={{ display: "flex", alignItems: "center", gap: 2 }}>
              <Typography level="body-sm" sx={{ width: 24, textAlign: "center", color: "text.tertiary" }}>{i + 1}</Typography>
              <Box sx={{ flex: 1 }}><VideoCard v={v} compact /></Box>
              <IconButton variant="plain" color="neutral" aria-label={`Remove ${v.title} from playlist`} onClick={async () => { await removeFromVideoPlaylist(id, v.id); setVideos((c) => (c ? c.filter((x) => x.id !== v.id) : c)); }}><DeleteOutlineRoundedIcon /></IconButton>
            </Box>
          ))}
        </Box>
      )}
    </Box>
  );
}

