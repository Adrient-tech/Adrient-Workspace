import { useCallback, useEffect, useRef, useState } from "react";
import { Box, Typography, Button, Input, LinearProgress, Chip, CircularProgress } from "@mui/joy";
import CloudUploadRoundedIcon from "@mui/icons-material/CloudUploadRounded";
import FolderOpenRoundedIcon from "@mui/icons-material/FolderOpenRounded";
import CheckCircleRoundedIcon from "@mui/icons-material/CheckCircleRounded";
import SearchIcon from "@mui/icons-material/Search";
import { useNavigate } from "react-router-dom";
import { importFromDrive, listDriveAudio, uploadTrack } from "./api";
import { probeAudio, formatBytes } from "./media";
import { useLibrary } from "./library";
import { EmptyNote, PageTitle } from "./ui";
import type { DriveAudio } from "./types";

interface Item {
  key: string;
  file: File;
  lyricsFile?: File;
  status: "queued" | "uploading" | "done" | "error";
  progress: number;
  error?: string;
}

const AUDIO = /\.(mp3|m4a|aac|ogg|opus|flac|wav)$/i;
const LYRICS = /\.(lrc|txt)$/i;
const baseName = (n: string) => n.replace(/\.[^.]+$/, "").toLowerCase();

/** pairFiles matches each audio file with a same-named .lrc/.txt file. */
export function pairFiles(files: File[]): { audio: File; lyrics?: File }[] {
  const lyr = new Map(files.filter((f) => LYRICS.test(f.name)).map((f) => [baseName(f.name), f]));
  return files.filter((f) => AUDIO.test(f.name) || f.type.startsWith("audio/")).map((f) => ({ audio: f, lyrics: lyr.get(baseName(f.name)) }));
}

/** UploadPage adds music from the computer or from Drive. */
export function UploadPage() {
  const lib = useLibrary();
  const nav = useNavigate();
  const [items, setItems] = useState<Item[]>([]);
  const [drag, setDrag] = useState(false);
  const input = useRef<HTMLInputElement>(null);
  const [drive, setDrive] = useState<DriveAudio[] | null>(null);
  const [q, setQ] = useState("");
  const [busyDrive, setBusyDrive] = useState<string | null>(null);
  const [driveErr, setDriveErr] = useState("");

  const loadDrive = useCallback((query: string) => {
    listDriveAudio(query)
      .then(setDrive)
      .catch(() => setDrive([]));
  }, []);
  useEffect(() => {
    const t = setTimeout(() => loadDrive(q), 200);
    return () => clearTimeout(t);
  }, [q, loadDrive]);

  const add = (list: FileList | File[]) => {
    const pairs = pairFiles([...list]);
    setItems((cur) => [...cur, ...pairs.map((p, i) => ({ key: `${Date.now()}-${cur.length + i}-${p.audio.name}`, file: p.audio, lyricsFile: p.lyrics, status: "queued" as const, progress: 0 }))]);
  };

  const patch = (key: string, p: Partial<Item>) => setItems((cur) => cur.map((it) => (it.key === key ? { ...it, ...p } : it)));

  const run = async () => {
    for (const it of items.filter((i) => i.status === "queued" || i.status === "error")) {
      patch(it.key, { status: "uploading", progress: 0, error: undefined });
      try {
        const [probe, lyrics] = await Promise.all([probeAudio(it.file), it.lyricsFile ? it.lyricsFile.text() : Promise.resolve("")]);
        await uploadTrack(it.file, { duration_seconds: probe.durationSeconds, lyrics }, (f) => patch(it.key, { progress: f }));
        patch(it.key, { status: "done", progress: 1 });
      } catch (e) {
        patch(it.key, { status: "error", error: (e as Error).message });
      }
    }
    await lib.refresh();
  };

  const queued = items.filter((i) => i.status === "queued" || i.status === "error").length;
  const uploading = items.some((i) => i.status === "uploading");
  const done = items.filter((i) => i.status === "done").length;

  return (
    <Box data-testid="music-upload">
      <PageTitle title="Add music" sub="Titles, artists, albums and cover art are read from the files. A .lrc file with the same name as a song adds synced lyrics." />
      <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", lg: "1fr 1fr" }, gap: 3, alignItems: "start" }}>
        <Box sx={{ border: "1px solid", borderColor: "divider", borderRadius: "18px", p: 2.5, bgcolor: "background.surface" }}>
          <Typography level="title-lg" sx={{ mb: 1.5 }}>From your computer</Typography>
          <Box
            role="button"
            tabIndex={0}
            aria-label="Choose audio files"
            onClick={() => input.current?.click()}
            onKeyDown={(e) => (e.key === "Enter" || e.key === " ") && input.current?.click()}
            onDragOver={(e) => {
              e.preventDefault();
              setDrag(true);
            }}
            onDragLeave={() => setDrag(false)}
            onDrop={(e) => {
              e.preventDefault();
              setDrag(false);
              add(e.dataTransfer.files);
            }}
            sx={{ border: "2px dashed", borderColor: drag ? "primary.solidBg" : "neutral.outlinedBorder", bgcolor: drag ? "primary.softBg" : "background.level1", borderRadius: "16px", py: 5, textAlign: "center", cursor: "pointer" }}
          >
            <CloudUploadRoundedIcon sx={{ fontSize: 44, color: "primary.solidBg" }} />
            <Typography level="title-md">Drop songs here, or click to choose</Typography>
            <Typography level="body-xs" sx={{ color: "text.tertiary" }}>MP3, M4A, AAC, OGG, FLAC, WAV. Add .lrc files together with the songs.</Typography>
            <input ref={input} type="file" multiple hidden accept="audio/*,.lrc,.txt,.mp3,.m4a,.flac,.ogg,.opus,.wav" data-testid="music-file-input" onChange={(e) => e.target.files && add(e.target.files)} />
          </Box>
          {items.length > 0 && (
            <Box sx={{ mt: 2 }} data-testid="upload-list">
              {items.map((it) => (
                <Box key={it.key} sx={{ display: "flex", alignItems: "center", gap: 1.5, py: 0.75 }}>
                  <Box sx={{ minWidth: 0, flex: 1 }}>
                    <Typography level="title-sm" noWrap>{it.file.name}</Typography>
                    <Typography level="body-xs" sx={{ color: it.status === "error" ? "danger.plainColor" : "text.tertiary" }}>
                      {formatBytes(it.file.size)}
                      {it.lyricsFile ? " · with lyrics" : ""}
                      {it.status === "error" ? ` · ${it.error}` : ""}
                    </Typography>
                    {it.status === "uploading" && <LinearProgress determinate value={it.progress * 100} sx={{ mt: 0.5 }} />}
                  </Box>
                  {it.status === "done" && <CheckCircleRoundedIcon color="success" />}
                  {it.status === "queued" && <Chip size="sm" variant="soft">Ready</Chip>}
                </Box>
              ))}
              <Box sx={{ display: "flex", gap: 1, mt: 1.5, alignItems: "center" }}>
                <Button onClick={run} loading={uploading} disabled={queued === 0} data-testid="start-upload">
                  Upload {queued || ""} {queued === 1 ? "song" : "songs"}
                </Button>
                <Button variant="plain" color="neutral" disabled={uploading} onClick={() => setItems([])}>Clear</Button>
                {done > 0 && !uploading && <Button variant="soft" onClick={() => nav("/music/library")}>View library</Button>}
              </Box>
            </Box>
          )}
        </Box>

        <Box sx={{ border: "1px solid", borderColor: "divider", borderRadius: "18px", p: 2.5, bgcolor: "background.surface" }}>
          <Typography level="title-lg" sx={{ mb: 1.5 }}>From Drive</Typography>
          <Input startDecorator={<SearchIcon />} placeholder="Search audio files in Drive" value={q} onChange={(e) => setQ(e.target.value)} slotProps={{ input: { "aria-label": "Search Drive" } }} />
          <Box sx={{ mt: 1.5, maxHeight: 360, overflowY: "auto" }} data-testid="drive-list">
            {drive === null && <CircularProgress size="sm" />}
            {drive !== null && drive.length === 0 && <EmptyNote icon={<FolderOpenRoundedIcon />} title="No audio files in Drive" text="Upload songs to Drive and they appear here. Songs are not copied: the library points at the Drive file, with its .lrc lyrics from the same folder." />}
            {(drive ?? []).map((f) => (
              <Box key={f.id} sx={{ display: "flex", alignItems: "center", gap: 1.5, py: 0.75 }}>
                <Box sx={{ minWidth: 0, flex: 1 }}>
                  <Typography level="title-sm" noWrap>{f.name}</Typography>
                  <Typography level="body-xs" sx={{ color: "text.tertiary" }}>{formatBytes(f.size)}</Typography>
                </Box>
                {f.imported ? (
                  <Chip size="sm" color="success" variant="soft" startDecorator={<CheckCircleRoundedIcon />}>In library</Chip>
                ) : (
                  <Button
                    size="sm"
                    variant="soft"
                    loading={busyDrive === f.id}
                    onClick={async () => {
                      setBusyDrive(f.id);
                      setDriveErr("");
                      try {
                        await importFromDrive(f.id);
                        await lib.refresh();
                        loadDrive(q);
                      } catch (e) {
                        setDriveErr((e as Error).message);
                      } finally {
                        setBusyDrive(null);
                      }
                    }}
                  >
                    Add to library
                  </Button>
                )}
              </Box>
            ))}
          </Box>
          {driveErr && <Typography color="danger" level="body-sm" sx={{ mt: 1 }}>{driveErr}</Typography>}
        </Box>
      </Box>
    </Box>
  );
}
