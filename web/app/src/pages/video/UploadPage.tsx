import { useEffect, useRef, useState } from "react";
import { Box, Typography, Button, Input, Textarea, FormControl, FormLabel, Select, Option, LinearProgress, Radio, RadioGroup, Chip, CircularProgress } from "@mui/joy";
import CloudUploadRoundedIcon from "@mui/icons-material/CloudUploadRounded";
import CheckCircleRoundedIcon from "@mui/icons-material/CheckCircleRounded";
import FolderOpenRoundedIcon from "@mui/icons-material/FolderOpenRounded";
import AddPhotoAlternateRoundedIcon from "@mui/icons-material/AddPhotoAlternateRounded";
import { useNavigate } from "react-router-dom";
import { captionUploadUrl, getVideoInfo, importVideoFromDrive, listDriveVideos, listGroups, uploadVideo } from "./api";
import type { DriveVideo, GroupRef, Visibility, VideoInfo } from "./types";
import { extractFrames, formatBytes, imageToThumbnail, probeVideo } from "./media";
import { previewTimes } from "./logic";
import { ChipRow, EmptyNote, PageTitle } from "./ui";

type Src = "computer" | "drive";

function VisibilityPicker({ vis, setVis, group, setGroup }: { vis: Visibility; setVis: (v: Visibility) => void; group: string; setGroup: (g: string) => void }) {
  const [groups, setGroups] = useState<GroupRef[]>([]);
  useEffect(() => {
    listGroups().then(setGroups).catch(() => setGroups([]));
  }, []);
  return (
    <FormControl>
      <FormLabel>Who can watch</FormLabel>
      <RadioGroup value={vis} onChange={(e) => setVis(e.target.value as Visibility)} sx={{ gap: 1 }}>
        <Radio value="private" label="Private: only you" />
        <Radio value="org" label="Organisation: everyone in your organisation" />
        <Radio value="group" label="Group: members of one group" />
      </RadioGroup>
      {vis === "group" && (
        groups.length === 0 ? <Typography level="body-sm" color="warning" sx={{ mt: 1 }}>You are not in any group yet.</Typography> : (
          <Select sx={{ mt: 1 }} placeholder="Choose a group" value={group || null} onChange={(_, v) => setGroup(v ?? "")} slotProps={{ button: { "aria-label": "Group" } }}>
            {groups.map((g) => <Option key={g.id} value={g.id}>{g.name}</Option>)}
          </Select>
        )
      )}
    </FormControl>
  );
}

/** UploadPage: upload a file or import from Drive, with details and a thumbnail choice. */
export function UploadPage() {
  const nav = useNavigate();
  const [src, setSrc] = useState<Src>("computer");
  const [file, setFile] = useState<File | null>(null);
  const [title, setTitle] = useState("");
  const [desc, setDesc] = useState("");
  const [vis, setVis] = useState<Visibility>("org");
  const [group, setGroup] = useState("");
  const [frames, setFrames] = useState<{ time: number; dataUrl: string }[]>([]);
  const [thumb, setThumb] = useState("");
  const [duration, setDuration] = useState(0);
  const [vtt, setVtt] = useState<File | null>(null);
  const [progress, setProgress] = useState<number | null>(null);
  const [created, setCreated] = useState<VideoInfo | null>(null);
  const [err, setErr] = useState("");
  const [drag, setDrag] = useState(false);
  const input = useRef<HTMLInputElement>(null);

  const pick = async (f: File) => {
    setFile(f);
    setErr("");
    setCreated(null);
    setTitle(f.name.replace(/\.[^.]+$/, ""));
    setFrames([]);
    setThumb("");
    const p = await probeVideo(f);
    setDuration(p.durationSeconds);
    const fr = await extractFrames(f, previewTimes(p.durationSeconds, 4));
    setFrames(fr);
    setThumb(fr[0]?.dataUrl ?? p.thumbnailDataUrl);
  };

  const submit = async () => {
    if (!file) return;
    setErr("");
    setProgress(0);
    try {
      const v = await uploadVideo(file, { title: title.trim() || file.name, description: desc, duration_seconds: duration, thumbnail_data_url: thumb, visibility: vis, group_id: vis === "group" ? group : "" }, setProgress);
      if (vtt) {
        const form = new FormData();
        form.append("file", vtt);
        form.append("lang", "en");
        form.append("label", "English");
        await fetch(captionUploadUrl(v.id), { method: "POST", body: form, credentials: "same-origin" }).catch(() => undefined);
      }
      setCreated(await getVideoInfo(v.id));
    } catch (e) {
      setErr((e as Error).message || "Upload failed");
    } finally {
      setProgress(null);
    }
  };

  // Poll while the server is still preparing the video.
  useEffect(() => {
    if (!created || created.status !== "processing") return;
    let n = 0;
    const t = window.setInterval(async () => {
      n++;
      const v = await getVideoInfo(created.id).catch(() => null);
      if (v && v.status === "ready") {
        setCreated(v);
        window.clearInterval(t);
      } else if (n > 40) window.clearInterval(t);
    }, 1500);
    return () => window.clearInterval(t);
  }, [created]);

  return (
    <Box data-testid="video-upload" sx={{ maxWidth: 920 }}>
      <PageTitle title="Upload video" sub="Add a video from your computer or from Drive. Put chapters in the description as lines like “0:00 Intro”." />
      <Box sx={{ mb: 2.5 }}>
        <ChipRow<Src> label="Source" value={src} options={[{ value: "computer", label: "From your computer" }, { value: "drive", label: "From Drive" }]} onChange={setSrc} />
      </Box>

      {src === "computer" && !created && (
        <>
          {!file && (
            <Box
              role="button"
              tabIndex={0}
              aria-label="Choose a video file"
              onClick={() => input.current?.click()}
              onKeyDown={(e) => (e.key === "Enter" || e.key === " ") && input.current?.click()}
              onDragOver={(e) => { e.preventDefault(); setDrag(true); }}
              onDragLeave={() => setDrag(false)}
              onDrop={(e) => { e.preventDefault(); setDrag(false); const f = e.dataTransfer.files[0]; if (f) void pick(f); }}
              sx={{ border: "2px dashed", borderColor: drag ? "primary.solidBg" : "neutral.outlinedBorder", bgcolor: drag ? "primary.softBg" : "background.level1", borderRadius: "20px", py: 9, textAlign: "center", cursor: "pointer" }}
            >
              <CloudUploadRoundedIcon sx={{ fontSize: 54, color: "primary.solidBg" }} />
              <Typography level="title-lg">Drop a video here, or click to choose</Typography>
              <Typography level="body-sm" sx={{ color: "text.tertiary" }}>MP4, WebM or MOV</Typography>
              <input ref={input} hidden type="file" accept="video/*" data-testid="video-file-input" onChange={(e) => e.target.files?.[0] && void pick(e.target.files[0])} />
            </Box>
          )}
          {file && (
            <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "1fr 320px" }, gap: 3 }}>
              <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
                <FormControl required><FormLabel>Title</FormLabel><Input value={title} onChange={(e) => setTitle(e.target.value)} slotProps={{ input: { "data-testid": "video-title-input" } }} /></FormControl>
                <FormControl><FormLabel>Description</FormLabel><Textarea minRows={5} value={desc} onChange={(e) => setDesc(e.target.value)} placeholder={"What is this video about?\n\n0:00 Intro\n0:45 First topic"} /></FormControl>
                <FormControl>
                  <FormLabel>Thumbnail</FormLabel>
                  <Box sx={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(120px, 1fr))", gap: 1.25 }} data-testid="thumb-choices">
                    {frames.length === 0 && <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}><CircularProgress size="sm" /><Typography level="body-xs">Reading frames…</Typography></Box>}
                    {frames.map((f) => (
                      <Box key={f.time} component="button" type="button" aria-label={`Use the frame at ${Math.round(f.time)} seconds`} aria-pressed={thumb === f.dataUrl} onClick={() => setThumb(f.dataUrl)} sx={{ all: "unset", cursor: "pointer", borderRadius: "10px", overflow: "hidden", lineHeight: 0, outline: thumb === f.dataUrl ? "3px solid" : "1px solid", outlineColor: thumb === f.dataUrl ? "var(--joy-palette-primary-solidBg)" : "var(--joy-palette-divider)", aspectRatio: "16/9" }}>
                        <img src={f.dataUrl} alt="" style={{ width: "100%", height: "100%", objectFit: "cover" }} />
                      </Box>
                    ))}
                    <Button component="label" variant="outlined" color="neutral" startDecorator={<AddPhotoAlternateRoundedIcon />} sx={{ aspectRatio: "16/9", height: "auto" }}>
                      Your own
                      <input hidden type="file" accept="image/*" onChange={async (e) => { const f = e.target.files?.[0]; if (f) { try { setThumb(await imageToThumbnail(f)); } catch (x) { setErr((x as Error).message); } } }} />
                    </Button>
                  </Box>
                </FormControl>
                <FormControl>
                  <FormLabel>Captions (optional, .vtt file)</FormLabel>
                  <Input type="file" slotProps={{ input: { accept: ".vtt,text/vtt", "aria-label": "Captions file" } }} onChange={(e) => setVtt(e.target.files?.[0] ?? null)} />
                </FormControl>
                <VisibilityPicker vis={vis} setVis={setVis} group={group} setGroup={setGroup} />
              </Box>
              <Box>
                <Box sx={{ border: "1px solid", borderColor: "divider", borderRadius: "16px", overflow: "hidden", position: "sticky", top: 130 }}>
                  <Box sx={{ aspectRatio: "16/9", bgcolor: "#0E2420", display: "flex", alignItems: "center", justifyContent: "center" }}>
                    {thumb ? <img src={thumb} alt="Thumbnail preview" style={{ width: "100%", height: "100%", objectFit: "cover" }} /> : <CircularProgress />}
                  </Box>
                  <Box sx={{ p: 1.5 }}>
                    <Typography level="title-sm" noWrap>{file.name}</Typography>
                    <Typography level="body-xs" sx={{ color: "text.tertiary" }}>{formatBytes(file.size)}{duration ? ` · ${Math.round(duration)} s` : ""}</Typography>
                    {progress !== null && <LinearProgress determinate value={progress * 100} sx={{ mt: 1.25 }} />}
                    {err && <Typography color="danger" level="body-sm" sx={{ mt: 1 }}>{err}</Typography>}
                    <Box sx={{ display: "flex", gap: 1, mt: 1.5 }}>
                      <Button fullWidth onClick={submit} loading={progress !== null} disabled={!title.trim() || (vis === "group" && !group)} data-testid="video-upload-submit">Upload</Button>
                      <Button variant="plain" color="neutral" disabled={progress !== null} onClick={() => setFile(null)}>Change</Button>
                    </Box>
                  </Box>
                </Box>
              </Box>
            </Box>
          )}
        </>
      )}

      {src === "computer" && created && (
        <Box sx={{ textAlign: "center", py: 6, border: "1px solid", borderColor: "divider", borderRadius: "20px" }} data-testid="upload-done">
          {created.status === "processing" ? <CircularProgress sx={{ mb: 1 }} /> : <CheckCircleRoundedIcon color="success" sx={{ fontSize: 54 }} />}
          <Typography level="title-lg">{created.status === "processing" ? "Processing your video…" : "Your video is ready"}</Typography>
          <Typography level="body-sm" sx={{ color: "text.tertiary", mb: 2 }}>{created.status === "processing" ? "Preparing the thumbnail and length. You can watch it already." : created.title}</Typography>
          <Box sx={{ display: "flex", gap: 1, justifyContent: "center" }}>
            <Button onClick={() => nav(`/video/v/${created.id}`)}>Watch</Button>
            <Button variant="outlined" color="neutral" onClick={() => { setCreated(null); setFile(null); }}>Upload another</Button>
          </Box>
        </Box>
      )}

      {src === "drive" && <DriveImport />}
    </Box>
  );
}

function DriveImport() {
  const nav = useNavigate();
  const [files, setFiles] = useState<DriveVideo[] | null>(null);
  const [q, setQ] = useState("");
  const [vis, setVis] = useState<"private" | "org">("private");
  const [busy, setBusy] = useState("");
  const [err, setErr] = useState("");
  const load = (query: string) => listDriveVideos(query).then(setFiles).catch(() => setFiles([]));
  useEffect(() => {
    const t = setTimeout(() => void load(q), 200);
    return () => clearTimeout(t);
  }, [q]);
  return (
    <Box data-testid="drive-import" sx={{ border: "1px solid", borderColor: "divider", borderRadius: "18px", p: 2.5 }}>
      <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap", mb: 1.5 }}>
        <Input placeholder="Search video files in Drive" value={q} onChange={(e) => setQ(e.target.value)} sx={{ flex: 1, minWidth: 220 }} slotProps={{ input: { "aria-label": "Search Drive" } }} />
        <Select value={vis} onChange={(_, v) => v && setVis(v)} sx={{ minWidth: 190 }} slotProps={{ button: { "aria-label": "Visibility of imported videos" } }}>
          <Option value="private">Private</Option>
          <Option value="org">Organisation</Option>
        </Select>
      </Box>
      {files === null && <CircularProgress size="sm" />}
      {files !== null && files.length === 0 && <EmptyNote icon={<FolderOpenRoundedIcon />} title="No video files in Drive" text="Videos are not copied: the video points at the Drive file. A .vtt file with the same name in the same folder becomes its captions." />}
      {(files ?? []).map((f) => (
        <Box key={f.id} sx={{ display: "flex", alignItems: "center", gap: 1.5, py: 0.9 }}>
          <Box sx={{ minWidth: 0, flex: 1 }}>
            <Typography level="title-sm" noWrap>{f.name}</Typography>
            <Typography level="body-xs" sx={{ color: "text.tertiary" }}>{formatBytes(f.size)}</Typography>
          </Box>
          {f.imported ? <Chip color="success" variant="soft" size="sm" startDecorator={<CheckCircleRoundedIcon />}>Added</Chip> : (
            <Button size="sm" variant="soft" loading={busy === f.id} onClick={async () => {
              setBusy(f.id); setErr("");
              try { const v = await importVideoFromDrive(f.id, "", vis); nav(`/video/v/${v.id}`); } catch (e) { setErr((e as Error).message); } finally { setBusy(""); }
            }}>Add</Button>
          )}
        </Box>
      ))}
      {err && <Typography color="danger" level="body-sm">{err}</Typography>}
    </Box>
  );
}
