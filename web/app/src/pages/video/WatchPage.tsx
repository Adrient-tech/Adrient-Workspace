import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { Box, Typography, Button, IconButton, Textarea, CircularProgress, Switch, Dropdown, Menu, MenuButton, MenuItem, Divider } from "@mui/joy";
import ThumbUpRoundedIcon from "@mui/icons-material/ThumbUpRounded";
import ThumbUpOffAltRoundedIcon from "@mui/icons-material/ThumbUpOffAltRounded";
import WatchLaterRoundedIcon from "@mui/icons-material/WatchLaterRounded";
import WatchLaterOutlinedIcon from "@mui/icons-material/WatchLaterOutlined";
import PlaylistAddRoundedIcon from "@mui/icons-material/PlaylistAddRounded";
import ShareRoundedIcon from "@mui/icons-material/ShareRounded";
import DownloadRoundedIcon from "@mui/icons-material/DownloadRounded";
import MoreHorizRoundedIcon from "@mui/icons-material/MoreHorizRounded";
import DeleteRoundedIcon from "@mui/icons-material/DeleteRounded";
import EditRoundedIcon from "@mui/icons-material/EditRounded";
import ChatBubbleOutlineRoundedIcon from "@mui/icons-material/ChatBubbleOutlineRounded";
import { deleteVideo, downloadUrl, getComments, getRelated, getVideoInfo, listVideoCaptions, addComment, deleteComment, likeComment, setLiked, subscribe, getChannel } from "./api";
import type { Channel, VideoCaption, VideoComment, VideoInfo } from "./types";
import { WatchPlayer, type WatchPlayerHandle } from "./WatchPlayer";
import { buildThreads, formatCount, parseChapters, resumeAt, segmentText, timeAgo } from "./logic";
import { ChannelAvatar, ChipRow, EmptyNote, VideoCard, VisibilityBadge } from "./ui";
import { useVideoCtx } from "./ctx";
import { VideoShareDialog } from "./VideoShareDialog";
import { formatDuration } from "./media";

/** Text with clickable timestamps and safe links. */
function RichText({ text, onSeek }: { text: string; onSeek: (s: number) => void }) {
  return (
    <>
      {segmentText(text).map((s, i) =>
        s.kind === "time" ? (
          <Box key={i} component="button" type="button" onClick={() => onSeek(s.seconds)} data-testid="timestamp-link" sx={{ all: "unset", cursor: "pointer", color: "primary.solidBg", fontWeight: 600, "&:hover": { textDecoration: "underline" } }}>
            {s.text}
          </Box>
        ) : s.kind === "link" ? (
          <a key={i} href={s.href} target="_blank" rel="noopener noreferrer nofollow" style={{ color: "var(--joy-palette-primary-solidBg)" }}>
            {s.text}
          </a>
        ) : (
          <span key={i}>{s.text}</span>
        ),
      )}
    </>
  );
}

function CommentItem({ c, onSeek, onLike, onReply, onDelete, reply }: { c: VideoComment; onSeek: (s: number) => void; onLike: (c: VideoComment) => void; onReply?: (c: VideoComment) => void; onDelete: (c: VideoComment) => void; reply?: boolean }) {
  return (
    <Box sx={{ display: "flex", gap: 1.5, mt: reply ? 1.5 : 2.5 }} data-testid={`comment-${c.id}`}>
      <ChannelAvatar name={c.user_name} size={reply ? 28 : 38} />
      <Box sx={{ minWidth: 0, flex: 1 }}>
        <Typography level="body-xs" sx={{ color: "text.tertiary" }}>
          <b style={{ color: "var(--joy-palette-text-primary)" }}>{c.user_name}</b> · {timeAgo(c.created_at)}
        </Typography>
        <Typography level="body-sm" sx={{ whiteSpace: "pre-wrap", wordBreak: "break-word", mt: 0.25 }}>
          <RichText text={c.body} onSeek={onSeek} />
        </Typography>
        <Box sx={{ display: "flex", alignItems: "center", gap: 0.5, mt: 0.5 }}>
          <IconButton size="sm" variant="plain" color={c.liked ? "primary" : "neutral"} aria-pressed={c.liked} aria-label={c.liked ? "Remove like" : "Like comment"} onClick={() => onLike(c)}>
            {c.liked ? <ThumbUpRoundedIcon fontSize="small" /> : <ThumbUpOffAltRoundedIcon fontSize="small" />}
          </IconButton>
          {c.likes > 0 && <Typography level="body-xs">{c.likes}</Typography>}
          {onReply && <Button size="sm" variant="plain" color="neutral" onClick={() => onReply(c)}>Reply</Button>}
          {c.can_delete && <Button size="sm" variant="plain" color="neutral" onClick={() => onDelete(c)}>Delete</Button>}
        </Box>
      </Box>
    </Box>
  );
}

/** Comments with replies and likes. */
function Comments({ videoId, onSeek, total }: { videoId: string; onSeek: (s: number) => void; total: number }) {
  const [list, setList] = useState<VideoComment[] | null>(null);
  const [text, setText] = useState("");
  const [replyTo, setReplyTo] = useState<VideoComment | null>(null);
  const [replyText, setReplyText] = useState("");
  const [sort, setSort] = useState<"new" | "top">("new");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const load = useCallback(() => getComments(videoId).then(setList).catch(() => setList([])), [videoId]);
  useEffect(() => {
    setList(null);
    void load();
  }, [load]);
  const threads = useMemo(() => buildThreads(list ?? [], sort), [list, sort]);
  const post = async (body: string, parent = "") => {
    if (!body.trim()) return;
    setBusy(true);
    setErr("");
    try {
      await addComment(videoId, body.trim(), parent);
      await load();
      return true;
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  const like = async (c: VideoComment) => {
    const r = await likeComment(c.id).catch(() => null);
    if (r) setList((cur) => (cur ? cur.map((x) => (x.id === c.id ? { ...x, liked: r.liked, likes: r.likes } : x)) : cur));
  };
  const del = async (c: VideoComment) => {
    if (!window.confirm("Delete this comment and its replies?")) return;
    await deleteComment(c.id).catch(() => undefined);
    void load();
  };
  const count = list?.length ?? total;
  return (
    <Box component="section" sx={{ mt: 3 }} data-testid="comments">
      <Box sx={{ display: "flex", alignItems: "center", gap: 2, mb: 1.5, flexWrap: "wrap" }}>
        <Typography level="title-lg">{count} comment{count === 1 ? "" : "s"}</Typography>
        <ChipRow<"new" | "top"> label="Sort comments" value={sort} options={[{ value: "new", label: "Newest" }, { value: "top", label: "Top" }]} onChange={setSort} />
      </Box>
      <Box sx={{ display: "flex", gap: 1.5 }}>
        <Textarea minRows={1} maxRows={6} placeholder="Add a comment" value={text} onChange={(e) => setText(e.target.value)} sx={{ flex: 1 }} slotProps={{ textarea: { "aria-label": "Add a comment", "data-testid": "comment-input" } }} />
        <Button disabled={!text.trim()} loading={busy} onClick={async () => (await post(text)) && setText("")} data-testid="comment-post" sx={{ alignSelf: "flex-end" }}>Comment</Button>
      </Box>
      {err && <Typography color="danger" level="body-sm">{err}</Typography>}
      {list === null && <CircularProgress size="sm" sx={{ mt: 2 }} />}
      {list !== null && threads.length === 0 && <Typography level="body-sm" sx={{ color: "text.tertiary", mt: 2 }}>No comments yet. Start the conversation.</Typography>}
      {threads.map(({ comment, replies }) => (
        <Box key={comment.id}>
          <CommentItem c={comment} onSeek={onSeek} onLike={like} onDelete={del} onReply={(c) => { setReplyTo(c); setReplyText(""); }} />
          <Box sx={{ pl: 6.5 }}>
            {replyTo?.id === comment.id && (
              <Box sx={{ display: "flex", gap: 1, mt: 1 }}>
                <Textarea autoFocus minRows={1} placeholder={`Reply to ${comment.user_name}`} value={replyText} onChange={(e) => setReplyText(e.target.value)} sx={{ flex: 1 }} slotProps={{ textarea: { "aria-label": "Write a reply", "data-testid": "reply-input" } }} />
                <Button size="sm" disabled={!replyText.trim()} loading={busy} onClick={async () => { if (await post(replyText, comment.id)) { setReplyTo(null); setReplyText(""); } }} data-testid="reply-post">Reply</Button>
                <Button size="sm" variant="plain" color="neutral" onClick={() => setReplyTo(null)}>Cancel</Button>
              </Box>
            )}
            {replies.map((r) => <CommentItem key={r.id} c={r} reply onSeek={onSeek} onLike={like} onDelete={del} />)}
          </Box>
        </Box>
      ))}
    </Box>
  );
}

/** WatchPage: player, details, chapters, comments and up next. */
export function WatchPage() {
  const { id = "" } = useParams();
  const nav = useNavigate();
  const ctx = useVideoCtx();
  const player = useRef<WatchPlayerHandle>(null);
  const [v, setV] = useState<VideoInfo | null>(null);
  const [missing, setMissing] = useState(false);
  const [captions, setCaptions] = useState<VideoCaption[]>([]);
  const [related, setRelated] = useState<VideoInfo[]>([]);
  const [channel, setChannel] = useState<Channel | null>(null);
  const [theatre, setTheatre] = useState(() => localStorage.getItem("adrient.video.theatre") === "1");
  const [autoplay, setAutoplay] = useState(() => localStorage.getItem("adrient.video.autoplayNext") !== "0");
  const [more, setMore] = useState(false);
  const [share, setShare] = useState(false);
  const [startAt, setStartAt] = useState<number | null>(null);

  useEffect(() => {
    setV(null);
    setMissing(false);
    setStartAt(null);
    setMore(false);
    getVideoInfo(id)
      .then((x) => {
        setV(x);
        setStartAt(resumeAt(x.progress_seconds, x.duration_seconds, x.watched));
        document.title = `${x.title} · Video`;
        getChannel(x.owner_id).then(setChannel).catch(() => setChannel(null));
      })
      .catch(() => setMissing(true));
    listVideoCaptions(id).then(setCaptions).catch(() => setCaptions([]));
    getRelated(id).then(setRelated).catch(() => setRelated([]));
    window.scrollTo({ top: 0 });
  }, [id]);

  const chapters = useMemo(() => (v ? parseChapters(v.description, v.duration_seconds) : []), [v]);
  const seek = useCallback((s: number) => {
    player.current?.seek(s);
    window.scrollTo({ top: 0, behavior: "smooth" });
  }, []);

  if (missing) return <EmptyNote title="Video not found" text="It may be private, or it was deleted." action={<Button onClick={() => nav("/video")}>Back to Home</Button>} />;
  if (!v || startAt === null) return <Box sx={{ display: "flex", justifyContent: "center", py: 12 }}><CircularProgress /></Box>;

  const toggleLike = async () => {
    const on = !v.liked;
    setV({ ...v, liked: on, likes: v.likes + (on ? 1 : -1) });
    await setLiked(v.id, on).catch(() => setV(v));
  };
  const toggleSub = async () => {
    if (!channel) return;
    const on = !channel.subscribed;
    setChannel({ ...channel, subscribed: on, subscribers: channel.subscribers + (on ? 1 : -1) });
    await subscribe(channel.user_id, on).catch(() => setChannel(channel));
    ctx.bumpSubs();
  };

  const side = (
    <Box data-testid="up-next">
      <Box sx={{ display: "flex", alignItems: "center", mb: 1.5 }}>
        <Typography level="title-md" sx={{ flex: 1 }}>Up next</Typography>
        <Typography level="body-xs" sx={{ mr: 1 }}>Autoplay</Typography>
        <Switch size="sm" checked={autoplay} onChange={(e) => { setAutoplay(e.target.checked); localStorage.setItem("adrient.video.autoplayNext", e.target.checked ? "1" : "0"); }} slotProps={{ input: { "aria-label": "Autoplay next video" } }} />
      </Box>
      <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
        {related.map((r) => (
          <VideoCard key={r.id} v={r} compact onWatchLater={(x) => ctx.toggleWatchLater(x, (on) => setRelated((cur) => cur.map((y) => (y.id === x.id ? { ...y, watch_later: on } : y))))} onAddToPlaylist={ctx.addToPlaylist} />
        ))}
        {related.length === 0 && <Typography level="body-sm" sx={{ color: "text.tertiary" }}>Nothing else to watch yet.</Typography>}
      </Box>
    </Box>
  );

  return (
    <Box data-testid="watch-page" sx={{ display: "grid", gridTemplateColumns: { xs: "minmax(0,1fr)", lg: theatre ? "minmax(0,1fr)" : "minmax(0,1fr) 400px" }, gap: 3, alignItems: "start" }}>
      <Box sx={{ minWidth: 0 }}>
        <Box sx={{ mx: theatre ? { xs: -1.5, md: -3 } : 0 }}>
          <WatchPlayer
            key={v.id}
            ref={player}
            video={v}
            captions={captions}
            chapters={chapters}
            startAt={startAt}
            theatre={theatre}
            onToggleTheatre={() => setTheatre((t) => { localStorage.setItem("adrient.video.theatre", t ? "0" : "1"); return !t; })}
            onEnded={() => autoplay && related[0] && nav(`/video/v/${related[0].id}`)}
          />
        </Box>
        <Typography level="h2" sx={{ fontSize: { xs: "xl", md: "xl2" }, mt: 1.75, lineHeight: 1.25 }} data-testid="watch-title">{v.title}</Typography>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, flexWrap: "wrap", mt: 1.25 }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1.25, cursor: "pointer" }} onClick={() => nav(`/video/channel/${v.owner_id}`)}>
            <ChannelAvatar name={v.owner_name} size={42} />
            <Box>
              <Typography level="title-sm" sx={{ fontWeight: 700 }}>{v.owner_name}</Typography>
              {channel && <Typography level="body-xs" sx={{ color: "text.tertiary" }}>{formatCount(channel.subscribers)} subscriber{channel.subscribers === 1 ? "" : "s"}</Typography>}
            </Box>
          </Box>
          {channel && !channel.mine && (
            <Button size="sm" color={channel.subscribed ? "neutral" : "primary"} variant={channel.subscribed ? "soft" : "solid"} onClick={toggleSub} data-testid="subscribe" sx={{ borderRadius: "999px" }}>
              {channel.subscribed ? "Subscribed" : "Subscribe"}
            </Button>
          )}
          <Box sx={{ ml: { md: "auto" }, display: "flex", gap: 1, flexWrap: "wrap", alignItems: "center" }}>
            <Button size="sm" variant={v.liked ? "solid" : "soft"} color={v.liked ? "primary" : "neutral"} aria-pressed={v.liked} onClick={toggleLike} startDecorator={v.liked ? <ThumbUpRoundedIcon /> : <ThumbUpOffAltRoundedIcon />} data-testid="like-video" sx={{ borderRadius: "999px" }}>
              {formatCount(v.likes)}
            </Button>
            <Button size="sm" variant="soft" color="neutral" startDecorator={v.watch_later ? <WatchLaterRoundedIcon /> : <WatchLaterOutlinedIcon />} onClick={() => ctx.toggleWatchLater(v, (on) => setV((x) => (x ? { ...x, watch_later: on } : x)))} data-testid="watch-later" sx={{ borderRadius: "999px", display: { xs: "none", sm: "inline-flex" } }}>
              {v.watch_later ? "Saved" : "Watch later"}
            </Button>
            <Button size="sm" variant="soft" color="neutral" startDecorator={<PlaylistAddRoundedIcon />} onClick={() => ctx.addToPlaylist(v)} sx={{ borderRadius: "999px", display: { xs: "none", sm: "inline-flex" } }}>Save</Button>
            {v.mine && <Button size="sm" variant="soft" color="neutral" startDecorator={<ShareRoundedIcon />} onClick={() => setShare(true)} sx={{ borderRadius: "999px" }}>Share</Button>}
            <Dropdown>
              <MenuButton slots={{ root: IconButton }} slotProps={{ root: { size: "sm", variant: "soft", color: "neutral", "aria-label": "More actions", sx: { borderRadius: "50%" } } }}><MoreHorizRoundedIcon /></MenuButton>
              <Menu size="sm" placement="bottom-end" sx={{ zIndex: 1400 }}>
                <MenuItem onClick={() => ctx.toggleWatchLater(v, (on) => setV((x) => (x ? { ...x, watch_later: on } : x)))} sx={{ display: { sm: "none" } }}>Watch later</MenuItem>
                <MenuItem onClick={() => ctx.addToPlaylist(v)} sx={{ display: { sm: "none" } }}>Add to playlist</MenuItem>
                <MenuItem component="a" href={downloadUrl(v.id)}><DownloadRoundedIcon fontSize="small" /> Download</MenuItem>
                {v.mine && <MenuItem onClick={() => nav("/video/studio")}><EditRoundedIcon fontSize="small" /> Edit in Studio</MenuItem>}
                {v.mine && (
                  <MenuItem color="danger" onClick={async () => { if (window.confirm(`Delete “${v.title}”?`)) { await deleteVideo(v.id); nav("/video"); } }}>
                    <DeleteRoundedIcon fontSize="small" /> Delete
                  </MenuItem>
                )}
              </Menu>
            </Dropdown>
          </Box>
        </Box>

        <Box onClick={() => !more && setMore(true)} sx={{ mt: 2, p: 1.75, borderRadius: "14px", bgcolor: "background.level1", cursor: more ? "default" : "pointer" }} data-testid="watch-description">
          <Typography level="title-sm" sx={{ fontWeight: 700 }}>
            {formatCount(v.views)} view{v.views === 1 ? "" : "s"} · {timeAgo(v.created_at)} · {formatDuration(v.duration_seconds)} <VisibilityBadge v={v} />
          </Typography>
          <Typography level="body-sm" sx={{ whiteSpace: "pre-wrap", mt: 0.75, wordBreak: "break-word", ...(more ? {} : { display: "-webkit-box", WebkitLineClamp: 3, WebkitBoxOrient: "vertical", overflow: "hidden" }) }}>
            {v.description ? <RichText text={v.description} onSeek={seek} /> : "No description."}
          </Typography>
          {more && chapters.length > 0 && <ChapterList chapters={chapters} onSeek={seek} />}
          <Box sx={{ mt: 0.5 }}>
            <Button size="sm" variant="plain" color="neutral" sx={{ px: 0 }} onClick={(e) => { e.stopPropagation(); setMore((m) => !m); }}>{more ? "Show less" : "Show more"}</Button>
          </Box>
        </Box>
        {!more && chapters.length > 0 && <ChapterList chapters={chapters} onSeek={seek} compact />}

        {v.mine && <OwnerNote id={v.id} />}
        <Divider sx={{ mt: 3 }} />
        <Comments videoId={v.id} onSeek={seek} total={v.comments} />
      </Box>
      <Box sx={{ minWidth: 0 }}>{side}</Box>
      {share && <VideoShareDialog open onClose={() => setShare(false)} videoId={v.id} videoTitle={v.title} />}
    </Box>
  );
}

function ChapterList({ chapters, onSeek, compact }: { chapters: { time: number; label: string }[]; onSeek: (s: number) => void; compact?: boolean }) {
  return (
    <Box sx={{ mt: compact ? 1.5 : 1, display: "flex", gap: 1, flexWrap: compact ? "nowrap" : "wrap", overflowX: compact ? "auto" : "visible" }} data-testid="chapters">
      {chapters.map((c, i) => (
        <Button key={i} size="sm" variant="outlined" color="neutral" onClick={(e) => { e.stopPropagation(); onSeek(c.time); }} sx={{ flexShrink: 0, borderRadius: "10px" }}>
          <span style={{ color: "var(--joy-palette-primary-solidBg)", marginRight: 6, fontWeight: 700 }}>{formatDuration(c.time) || "0:00"}</span>
          {c.label}
        </Button>
      ))}
    </Box>
  );
}

/** Small owner line pointing to the analytics in Studio. */
function OwnerNote({ id }: { id: string }) {
  const nav = useNavigate();
  return (
    <Box sx={{ mt: 2, display: "flex", alignItems: "center", gap: 1, color: "text.tertiary" }}>
      <ChatBubbleOutlineRoundedIcon fontSize="small" />
      <Typography level="body-xs">You own this video. See views and watch time in</Typography>
      <Button size="sm" variant="plain" onClick={() => nav(`/video/studio?video=${id}`)}>Studio</Button>
    </Box>
  );
}
