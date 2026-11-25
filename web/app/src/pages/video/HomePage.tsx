import { useCallback, useEffect, useState } from "react";
import { Box, Typography, Button, CircularProgress } from "@mui/joy";
import UploadRoundedIcon from "@mui/icons-material/UploadRounded";
import VideocamRoundedIcon from "@mui/icons-material/VideocamRounded";
import PlayCircleRoundedIcon from "@mui/icons-material/PlayCircleRounded";
import { useNavigate } from "react-router-dom";
import { getFeed, getHistory, getMeetings, openMeeting } from "./api";
import type { MeetingRecording, VideoInfo } from "./types";
import { useVideoCtx } from "./ctx";
import { ChipRow, EmptyNote, VideoCard, VideoGrid, Poster } from "./ui";
import { timeAgo } from "./logic";

type Tab = "all" | "new" | "popular" | "unwatched";
const TABS: { value: Tab; label: string }[] = [
  { value: "all", label: "All" },
  { value: "new", label: "Newest" },
  { value: "popular", label: "Most viewed" },
  { value: "unwatched", label: "Not watched yet" },
];

/** MeetingsShelf shows the user's Meet recordings; opening one makes it a (private) video. */
export function MeetingsShelf({ recordings, onOpen, busy }: { recordings: MeetingRecording[]; onOpen: (r: MeetingRecording) => void; busy: string }) {
  if (!recordings.length) return null;
  return (
    <Box component="section" sx={{ mb: 4 }} data-testid="meetings-shelf">
      <Typography level="title-lg" sx={{ mb: 1.5 }}>Meetings</Typography>
      <Box sx={{ display: "flex", gap: 2, overflowX: "auto", pb: 1 }}>
        {recordings.map((r) => (
          <Box key={r.id} role="button" tabIndex={0} data-testid={`meeting-${r.id}`} onClick={() => onOpen(r)} onKeyDown={(e) => e.key === "Enter" && onOpen(r)} sx={{ width: 260, flexShrink: 0, cursor: "pointer", border: "1px solid", borderColor: "divider", borderRadius: "14px", p: 1.5, bgcolor: "background.surface", "&:hover": { bgcolor: "background.level1" } }}>
            <Box sx={{ display: "flex", alignItems: "center", gap: 1.25 }}>
              <Box sx={{ width: 44, height: 44, borderRadius: "12px", bgcolor: "primary.softBg", color: "primary.solidBg", display: "flex", alignItems: "center", justifyContent: "center" }}>
                {busy === r.id ? <CircularProgress size="sm" /> : <VideocamRoundedIcon />}
              </Box>
              <Box sx={{ minWidth: 0 }}>
                <Typography level="title-sm" noWrap sx={{ fontWeight: 650 }}>{r.room || "Meeting"}</Typography>
                <Typography level="body-xs" sx={{ color: "text.tertiary" }}>{new Date(r.started_at).toLocaleString([], { dateStyle: "medium", timeStyle: "short" })} · {timeAgo(r.started_at)}</Typography>
              </Box>
            </Box>
          </Box>
        ))}
      </Box>
    </Box>
  );
}

export function useMeetings() {
  const nav = useNavigate();
  const ctx = useVideoCtx();
  const [recs, setRecs] = useState<MeetingRecording[]>([]);
  const [busy, setBusy] = useState("");
  useEffect(() => {
    getMeetings().then(setRecs).catch(() => setRecs([]));
  }, []);
  const open = useCallback(
    async (r: MeetingRecording) => {
      setBusy(r.id);
      try {
        const v = r.video_id ? { id: r.video_id } : await openMeeting(r.id);
        nav(`/video/v/${v.id}`);
      } catch {
        ctx.notify("Could not open this recording");
      } finally {
        setBusy("");
      }
    },
    [nav, ctx],
  );
  return { recs, open, busy };
}

/** HomePage: continue watching, the Meetings shelf and the grid. */
export function HomePage() {
  const nav = useNavigate();
  const ctx = useVideoCtx();
  const [videos, setVideos] = useState<VideoInfo[] | null>(null);
  const [cont, setCont] = useState<VideoInfo[]>([]);
  const [tab, setTab] = useState<Tab>("all");
  const m = useMeetings();

  useEffect(() => {
    getFeed({ sort: tab === "popular" ? "views" : "new" })
      .then(setVideos)
      .catch(() => setVideos([]));
  }, [tab]);
  useEffect(() => {
    getHistory()
      .then((h) => setCont(h.filter((v) => v.progress_percent > 0.03 && !v.watched).slice(0, 8)))
      .catch(() => setCont([]));
  }, []);

  const shown = (videos ?? []).filter((v) => (tab === "unwatched" ? !v.watched && v.progress_percent < 0.03 : true));
  const patch = (id: string, p: Partial<VideoInfo>) => setVideos((cur) => (cur ? cur.map((v) => (v.id === id ? { ...v, ...p } : v)) : cur));

  return (
    <Box data-testid="video-home">
      <Box sx={{ mb: 2.5 }}>
        <ChipRow label="Video filters" value={tab} options={TABS} onChange={setTab} />
      </Box>
      <MeetingsShelf recordings={m.recs} onOpen={m.open} busy={m.busy} />
      {cont.length > 0 && tab === "all" && (
        <Box component="section" sx={{ mb: 4 }} data-testid="continue-shelf">
          <Typography level="title-lg" sx={{ mb: 1.5 }}>Continue watching</Typography>
          <Box sx={{ display: "flex", gap: 2, overflowX: "auto", pb: 1 }}>
            {cont.map((v) => (
              <Box key={v.id} onClick={() => nav(`/video/v/${v.id}`)} sx={{ width: 250, flexShrink: 0, cursor: "pointer" }}>
                <Poster v={v} />
                <Typography level="title-sm" noWrap sx={{ mt: 0.75, fontWeight: 650 }}>{v.title}</Typography>
                <Typography level="body-xs" sx={{ color: "text.tertiary" }} noWrap>{v.owner_name}</Typography>
              </Box>
            ))}
          </Box>
        </Box>
      )}
      {videos === null ? (
        <Box sx={{ display: "flex", justifyContent: "center", py: 10 }}><CircularProgress /></Box>
      ) : shown.length === 0 ? (
        <EmptyNote
          icon={<PlayCircleRoundedIcon />}
          title={tab === "unwatched" ? "You are all caught up" : "No videos yet"}
          text={tab === "unwatched" ? "Everything has been watched." : "Upload a video or add one from Drive. Thumbnails, chapters and captions are supported."}
          action={tab === "unwatched" ? undefined : <Button startDecorator={<UploadRoundedIcon />} onClick={() => nav("/video/upload")}>Upload a video</Button>}
        />
      ) : (
        <VideoGrid>
          {shown.map((v) => (
            <VideoCard key={v.id} v={v} onWatchLater={(x) => ctx.toggleWatchLater(x, (on) => patch(x.id, { watch_later: on }))} onAddToPlaylist={ctx.addToPlaylist} />
          ))}
        </VideoGrid>
      )}
    </Box>
  );
}
