import { useEffect, useState } from "react";
import { Box, Typography, Button, Input, Textarea, FormControl, FormLabel, Modal, ModalDialog, ModalClose, DialogTitle, Select, Option, CircularProgress, Chip, IconButton, Table } from "@mui/joy";
import VideoLibraryRoundedIcon from "@mui/icons-material/VideoLibraryRounded";
import EditRoundedIcon from "@mui/icons-material/EditRounded";
import InsightsRoundedIcon from "@mui/icons-material/InsightsRounded";
import DeleteOutlineRoundedIcon from "@mui/icons-material/DeleteOutlineRounded";
import UploadRoundedIcon from "@mui/icons-material/UploadRounded";
import { useNavigate, useSearchParams } from "react-router-dom";
import { deleteVideo, getFeed, getStats, listGroups, updateVideoInfo } from "./api";
import type { GroupRef, VideoInfo, VideoStats, Visibility } from "./types";
import { useVideoCtx } from "./ctx";
import { EmptyNote, PageTitle, Poster, VisibilityBadge } from "./ui";
import { formatCount, timeAgo, watchTime } from "./logic";

function StatsDialog({ v, onClose }: { v: VideoInfo; onClose: () => void }) {
  const [s, setS] = useState<VideoStats | null>(null);
  useEffect(() => {
    getStats(v.id).then(setS).catch(() => setS(null));
  }, [v.id]);
  const max = Math.max(1, ...(s?.daily.map((d) => d.views) ?? [1]));
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: { xs: "100vw", sm: 560 }, maxWidth: "100vw" }}>
        <ModalClose />
        <DialogTitle>Analytics: {v.title}</DialogTitle>
        {!s ? <CircularProgress /> : (
          <>
            <Box sx={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 1.5 }} data-testid="stats">
              {[["Views", formatCount(s.views)], ["Viewers", String(s.unique_viewers)], ["Watch time", watchTime(s.watch_seconds)], ["Average view", watchTime(s.avg_seconds)], ["Likes", String(s.likes)], ["Comments", String(s.comments)]].map(([k, val]) => (
                <Box key={k} sx={{ p: 1.5, borderRadius: "12px", bgcolor: "background.level1" }}>
                  <Typography level="body-xs" sx={{ color: "text.tertiary" }}>{k}</Typography>
                  <Typography level="title-lg">{val}</Typography>
                </Box>
              ))}
            </Box>
            <Typography level="title-sm" sx={{ mt: 1 }}>Views, last 14 days</Typography>
            <Box sx={{ display: "flex", alignItems: "flex-end", gap: "4px", height: 110 }} role="img" aria-label="Views per day">
              {s.daily.map((d) => (
                <Box key={d.day} title={`${d.day}: ${d.views}`} sx={{ flex: 1, height: `${Math.max(4, (d.views / max) * 100)}%`, bgcolor: d.views ? "primary.400" : "background.level2", borderRadius: "4px 4px 0 0" }} />
              ))}
            </Box>
          </>
        )}
      </ModalDialog>
    </Modal>
  );
}

function EditDialog({ v, onClose, onSaved }: { v: VideoInfo; onClose: () => void; onSaved: (v: VideoInfo) => void }) {
  const [title, setTitle] = useState(v.title);
  const [desc, setDesc] = useState(v.description);
  const [vis, setVis] = useState<Visibility>(v.visibility);
  const [group, setGroup] = useState(v.group_id);
  const [groups, setGroups] = useState<GroupRef[]>([]);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  useEffect(() => {
    listGroups().then(setGroups).catch(() => setGroups([]));
  }, []);
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: { xs: "100vw", sm: 560 }, maxWidth: "100vw", maxHeight: "92vh", overflowY: "auto" }}>
        <ModalClose />
        <DialogTitle>Edit video</DialogTitle>
        <FormControl><FormLabel>Title</FormLabel><Input value={title} onChange={(e) => setTitle(e.target.value)} slotProps={{ input: { "aria-label": "Title" } }} /></FormControl>
        <FormControl><FormLabel>Description</FormLabel><Textarea minRows={5} value={desc} onChange={(e) => setDesc(e.target.value)} slotProps={{ textarea: { "aria-label": "Description" } }} /></FormControl>
        <FormControl>
          <FormLabel>Who can watch</FormLabel>
          <Select value={vis} onChange={(_, x) => x && setVis(x)} slotProps={{ button: { "aria-label": "Visibility" } }}>
            <Option value="private">Private</Option>
            <Option value="org">Organisation</Option>
            <Option value="group">Group</Option>
          </Select>
        </FormControl>
        {vis === "group" && (
          <FormControl>
            <FormLabel>Group</FormLabel>
            <Select value={group || null} placeholder="Choose a group" onChange={(_, x) => setGroup(x ?? "")} slotProps={{ button: { "aria-label": "Group" } }}>
              {groups.map((g) => <Option key={g.id} value={g.id}>{g.name}</Option>)}
            </Select>
          </FormControl>
        )}
        {err && <Typography color="danger" level="body-sm">{err}</Typography>}
        <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1 }}>
          <Button variant="plain" color="neutral" onClick={onClose}>Cancel</Button>
          <Button loading={busy} disabled={!title.trim() || (vis === "group" && !group)} onClick={async () => {
            setBusy(true);
            try { onSaved(await updateVideoInfo(v.id, { title: title.trim(), description: desc, visibility: vis, group_id: vis === "group" ? group : "" })); } catch (e) { setErr((e as Error).message); setBusy(false); }
          }}>Save</Button>
        </Box>
      </ModalDialog>
    </Modal>
  );
}

/** StudioPage: your videos with views, watch time, visibility and edit tools. */
export function StudioPage() {
  const ctx = useVideoCtx();
  const nav = useNavigate();
  const [sp] = useSearchParams();
  const [videos, setVideos] = useState<VideoInfo[] | null>(null);
  const [watch, setWatch] = useState<Record<string, number>>({});
  const [edit, setEdit] = useState<VideoInfo | null>(null);
  const [stats, setStats] = useState<VideoInfo | null>(null);

  useEffect(() => {
    getFeed({ owner: ctx.userId })
      .then(async (v) => {
        setVideos(v);
        const entries = await Promise.all(v.map((x) => getStats(x.id).then((s) => [x.id, s.watch_seconds] as const).catch(() => [x.id, 0] as const)));
        setWatch(Object.fromEntries(entries));
        const open = sp.get("video");
        if (open) setStats(v.find((x) => x.id === open) ?? null);
      })
      .catch(() => setVideos([]));
  }, [ctx.userId]); // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <Box data-testid="video-studio">
      <PageTitle title="Your videos" icon={<VideoLibraryRoundedIcon />} sub="Views, watch time and who can watch each video" action={<Button startDecorator={<UploadRoundedIcon />} onClick={() => nav("/video/upload")}>Upload</Button>} />
      {videos === null ? <Box sx={{ display: "flex", justifyContent: "center", py: 10 }}><CircularProgress /></Box> : videos.length === 0 ? (
        <EmptyNote title="You have not uploaded anything" text="Upload a video or add one from Drive." action={<Button onClick={() => nav("/video/upload")}>Upload a video</Button>} />
      ) : (
        <Box sx={{ overflowX: "auto", border: "1px solid", borderColor: "divider", borderRadius: "16px" }}>
          <Table hoverRow sx={{ minWidth: 760, "--TableCell-paddingY": "10px" }}>
            <thead>
              <tr><th style={{ width: "38%" }}>Video</th><th>Visibility</th><th>Date</th><th>Views</th><th>Watch time</th><th>Comments</th><th style={{ width: 130 }} /></tr>
            </thead>
            <tbody>
              {videos.map((v) => (
                <tr key={v.id} data-testid={`studio-row-${v.id}`}>
                  <td>
                    <Box sx={{ display: "flex", gap: 1.5, alignItems: "center", cursor: "pointer" }} onClick={() => nav(`/video/v/${v.id}`)}>
                      <Box sx={{ width: 120, flexShrink: 0 }}><Poster v={v} radius={8} /></Box>
                      <Box sx={{ minWidth: 0 }}>
                        <Typography level="title-sm" sx={{ fontWeight: 650 }}>{v.title}</Typography>
                        <Typography level="body-xs" noWrap sx={{ color: "text.tertiary", maxWidth: 260 }}>{v.description.split("\n")[0]}</Typography>
                      </Box>
                    </Box>
                  </td>
                  <td>{v.visibility === "org" ? <Chip size="sm" variant="soft" color="primary">Organisation</Chip> : <VisibilityBadge v={v} />}{v.source_kind === "meet" && <Chip size="sm" sx={{ ml: 0.5 }}>Meeting</Chip>}</td>
                  <td>{timeAgo(v.created_at)}</td>
                  <td>{v.views}</td>
                  <td>{watchTime(watch[v.id] ?? 0)}</td>
                  <td>{v.comments}</td>
                  <td>
                    <IconButton size="sm" variant="plain" aria-label={`Analytics for ${v.title}`} onClick={() => setStats(v)}><InsightsRoundedIcon /></IconButton>
                    <IconButton size="sm" variant="plain" aria-label={`Edit ${v.title}`} onClick={() => setEdit(v)}><EditRoundedIcon /></IconButton>
                    <IconButton size="sm" variant="plain" color="danger" aria-label={`Delete ${v.title}`} onClick={async () => { if (window.confirm(`Delete “${v.title}”? This cannot be undone.`)) { await deleteVideo(v.id); setVideos((c) => (c ? c.filter((x) => x.id !== v.id) : c)); } }}><DeleteOutlineRoundedIcon /></IconButton>
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Box>
      )}
      {edit && <EditDialog v={edit} onClose={() => setEdit(null)} onSaved={(nv) => { setVideos((c) => (c ? c.map((x) => (x.id === nv.id ? nv : x)) : c)); setEdit(null); }} />}
      {stats && <StatsDialog v={stats} onClose={() => setStats(null)} />}
    </Box>
  );
}
