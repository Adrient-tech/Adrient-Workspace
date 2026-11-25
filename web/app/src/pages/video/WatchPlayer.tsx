import { forwardRef, useCallback, useEffect, useImperativeHandle, useRef, useState } from "react";
import { Box, IconButton, Typography, Dropdown, Menu, MenuButton, MenuItem, Slider } from "@mui/joy";
import PlayArrowRoundedIcon from "@mui/icons-material/PlayArrowRounded";
import PauseRoundedIcon from "@mui/icons-material/PauseRounded";
import VolumeUpRoundedIcon from "@mui/icons-material/VolumeUpRounded";
import VolumeOffRoundedIcon from "@mui/icons-material/VolumeOffRounded";
import ClosedCaptionRoundedIcon from "@mui/icons-material/ClosedCaptionRounded";
import ClosedCaptionDisabledRoundedIcon from "@mui/icons-material/ClosedCaptionDisabledRounded";
import PictureInPictureAltRoundedIcon from "@mui/icons-material/PictureInPictureAltRounded";
import FullscreenRoundedIcon from "@mui/icons-material/FullscreenRounded";
import FullscreenExitRoundedIcon from "@mui/icons-material/FullscreenExitRounded";
import SettingsRoundedIcon from "@mui/icons-material/SettingsRounded";
import CropLandscapeRoundedIcon from "@mui/icons-material/CropLandscapeRounded";
import ReplayRoundedIcon from "@mui/icons-material/ReplayRounded";
import { streamUrl, setVideoProgress, recordView, reportDuration } from "./api";
import type { VideoCaption, VideoInfo } from "./types";
import { chapterAt, keyAction, SPEEDS, stepSpeed, type Chapter } from "./logic";
import { formatDuration } from "./media";

export interface WatchPlayerHandle {
  seek: (seconds: number) => void;
  play: () => void;
}

interface Props {
  video: VideoInfo;
  captions: VideoCaption[];
  chapters: Chapter[];
  startAt: number;
  theatre: boolean;
  onToggleTheatre: () => void;
  onEnded?: () => void;
  autoPlay?: boolean;
  /** Where the video bytes come from; defaults to the signed-in stream. */
  src?: string;
}

const clock = (s: number) => formatDuration(Math.max(0, Math.floor(s))) || "0:00";

/** WatchPlayer is the custom player: seek with preview, speed, captions,
 *  chapters, theatre, picture-in-picture, fullscreen, keyboard and resume. */
export const WatchPlayer = forwardRef<WatchPlayerHandle, Props>(function WatchPlayer({ video, captions, chapters, startAt, theatre, onToggleTheatre, onEnded, autoPlay = true, src }, ref) {
  const wrap = useRef<HTMLDivElement>(null);
  const vid = useRef<HTMLVideoElement>(null);
  const prev = useRef<HTMLVideoElement>(null);
  const canvas = useRef<HTMLCanvasElement>(null);
  const bar = useRef<HTMLDivElement>(null);
  const [playing, setPlaying] = useState(false);
  const [pos, setPos] = useState(0);
  const [dur, setDur] = useState(video.duration_seconds || 0);
  const [buffered, setBuffered] = useState(0);
  const [volume, setVolume] = useState(1);
  const [muted, setMuted] = useState(false);
  const [speed, setSpeed] = useState(1);
  const [ccOn, setCcOn] = useState(false);
  const [fs, setFs] = useState(false);
  const [uiVisible, setUiVisible] = useState(true);
  const [hover, setHover] = useState<{ x: number; t: number } | null>(null);
  const [ended, setEnded] = useState(false);
  const [toast, setToast] = useState("");
  const [err, setErr] = useState(false);
  const hideTimer = useRef<number | undefined>(undefined);
  const watched = useRef(0); // seconds actually watched
  const lastT = useRef(0);
  const lastReport = useRef(0);
  const viewSent = useRef(false);
  const toastTimer = useRef<number | undefined>(undefined);

  const url = src ?? streamUrl(video.id);

  const say = useCallback((m: string) => {
    setToast(m);
    window.clearTimeout(toastTimer.current);
    toastTimer.current = window.setTimeout(() => setToast(""), 1200);
  }, []);

  const flush = useCallback(
    (final: boolean) => {
      const v = vid.current;
      if (!v || src) return; // shared links do not record
      if (v.duration > 0 && (final || v.currentTime - lastReport.current >= 5 || v.currentTime < lastReport.current)) {
        lastReport.current = v.currentTime;
        void setVideoProgress(video.id, v.currentTime, Math.min(1, v.currentTime / v.duration)).catch(() => undefined);
      }
    },
    [src, video.id],
  );

  // Send the watch time when leaving the page or switching videos.
  useEffect(() => {
    watched.current = 0;
    viewSent.current = false;
    lastReport.current = 0;
    setEnded(false);
    setErr(false);
    return () => {
      if (src) return;
      flush(true);
      if (viewSent.current && watched.current > 0) void recordView(video.id, watched.current);
    };
  }, [video.id, src, flush]);

  const toggle = useCallback(() => {
    const v = vid.current;
    if (!v) return;
    if (v.ended) v.currentTime = 0;
    if (v.paused) void v.play().catch(() => undefined);
    else v.pause();
  }, []);
  const seekTo = useCallback((t: number) => {
    const v = vid.current;
    if (!v) return;
    v.currentTime = Math.max(0, Math.min(v.duration || t, t));
    setPos(v.currentTime);
  }, []);
  useImperativeHandle(ref, () => ({ seek: (s) => { seekTo(s); void vid.current?.play().catch(() => undefined); }, play: () => void vid.current?.play().catch(() => undefined) }), [seekTo]);

  const setCaptions = useCallback((on: boolean) => {
    setCcOn(on);
    const v = vid.current;
    if (!v) return;
    for (const t of Array.from(v.textTracks)) t.mode = on ? "showing" : "hidden";
  }, []);

  const toggleFs = useCallback(() => {
    const el = wrap.current;
    if (!el) return;
    if (document.fullscreenElement) void document.exitFullscreen();
    else void el.requestFullscreen?.().catch(() => undefined);
  }, []);
  const togglePip = useCallback(async () => {
    const v = vid.current;
    if (!v || !document.pictureInPictureEnabled) return say("Picture-in-picture is not available");
    try {
      if (document.pictureInPictureElement) await document.exitPictureInPicture();
      else await v.requestPictureInPicture();
    } catch {
      say("Picture-in-picture is not available");
    }
  }, [say]);
  const changeSpeed = useCallback(
    (s: number) => {
      setSpeed(s);
      if (vid.current) vid.current.playbackRate = s;
      say(`${s}x`);
    },
    [say],
  );

  useEffect(() => {
    const onFs = () => setFs(document.fullscreenElement === wrap.current);
    document.addEventListener("fullscreenchange", onFs);
    return () => document.removeEventListener("fullscreenchange", onFs);
  }, []);

  // Keyboard shortcuts while the watch page is open.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.ctrlKey || e.metaKey || e.altKey) return;
      const el = e.target as HTMLElement | null;
      if (el && (el.tagName === "INPUT" || el.tagName === "TEXTAREA" || el.tagName === "SELECT" || el.isContentEditable)) return;
      const a = keyAction(e.key, e.shiftKey);
      const v = vid.current;
      if (!a || !v) return;
      e.preventDefault();
      switch (a.type) {
        case "toggle":
          toggle();
          break;
        case "seekBy":
          seekTo(v.currentTime + a.by);
          say(`${a.by > 0 ? "+" : ""}${a.by} s`);
          break;
        case "seekTo":
          if (v.duration) seekTo(v.duration * a.fraction);
          break;
        case "fullscreen":
          toggleFs();
          break;
        case "theatre":
          onToggleTheatre();
          break;
        case "mute":
          v.muted = !v.muted;
          setMuted(v.muted);
          break;
        case "captions":
          if (captions.length) {
            setCaptions(!ccOn);
            say(ccOn ? "Captions off" : "Captions on");
          } else say("No captions for this video");
          break;
        case "pip":
          void togglePip();
          break;
        case "speed":
          changeSpeed(stepSpeed(speed, a.by));
          break;
        case "volume":
          v.volume = Math.min(1, Math.max(0, v.volume + a.by));
          setVolume(v.volume);
          say(`Volume ${Math.round(v.volume * 100)}%`);
          break;
        case "frame":
          if (v.paused) seekTo(v.currentTime + a.by / 25);
          break;
        case "chapter": {
          const i = chapterAt(chapters, v.currentTime) + a.by;
          if (chapters[i]) seekTo(chapters[i].time);
          break;
        }
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [captions.length, ccOn, chapters, changeSpeed, onToggleTheatre, say, seekTo, setCaptions, speed, toggle, toggleFs, togglePip]);

  const wake = () => {
    setUiVisible(true);
    window.clearTimeout(hideTimer.current);
    hideTimer.current = window.setTimeout(() => setUiVisible(false), 2800);
  };
  useEffect(() => () => window.clearTimeout(hideTimer.current), []);

  // Seek-bar hover preview: a second muted element draws the frame.
  const onBarMove = (e: React.PointerEvent) => {
    const r = bar.current?.getBoundingClientRect();
    if (!r || !dur) return;
    const f = Math.min(1, Math.max(0, (e.clientX - r.left) / r.width));
    const t = f * dur;
    setHover({ x: f * r.width, t });
    const p = prev.current;
    if (p && p.readyState >= 1) {
      try {
        p.currentTime = t;
      } catch {
        /* preview is best effort */
      }
    }
  };
  const drawPreview = () => {
    const p = prev.current;
    const c = canvas.current;
    if (!p || !c || !p.videoWidth) return;
    const ctx = c.getContext("2d");
    if (ctx) ctx.drawImage(p, 0, 0, c.width, c.height);
  };
  const dragSeek = (e: React.PointerEvent) => {
    const r = bar.current?.getBoundingClientRect();
    if (!r || !dur) return;
    seekTo(Math.min(1, Math.max(0, (e.clientX - r.left) / r.width)) * dur);
  };

  const cur = chapterAt(chapters, pos);
  const pct = dur ? (pos / dur) * 100 : 0;
  const showUi = uiVisible || !playing || hover !== null;

  return (
    <Box
      ref={wrap}
      data-testid="watch-player"
      data-theatre={theatre || undefined}
      onMouseMove={wake}
      onTouchStart={wake}
      onMouseLeave={() => playing && setUiVisible(false)}
      sx={{ position: "relative", bgcolor: "#000", borderRadius: fs || theatre ? 0 : "16px", overflow: "hidden", aspectRatio: fs ? "auto" : "16 / 9", width: "100%", maxHeight: fs ? "100vh" : theatre ? "72vh" : "none", cursor: playing && !showUi ? "none" : "default", userSelect: "none" }}
    >
      <video
        ref={vid}
        src={url}
        preload="metadata"
        playsInline
        autoPlay={autoPlay}
        crossOrigin="use-credentials"
        data-testid="watch-video"
        onClick={toggle}
        onDoubleClick={toggleFs}
        onLoadedMetadata={(e) => {
          const v = e.currentTarget;
          setDur(v.duration);
          if (!video.duration_seconds && !src) void reportDuration(video.id, v.duration);
          if (startAt > 0 && startAt < v.duration - 5) v.currentTime = startAt;
          for (const t of Array.from(v.textTracks)) t.mode = ccOn ? "showing" : "hidden";
        }}
        onPlay={() => {
          setPlaying(true);
          setEnded(false);
          wake();
        }}
        onPause={() => {
          setPlaying(false);
          flush(true);
        }}
        onEnded={() => {
          setEnded(true);
          setPlaying(false);
          flush(true);
          onEnded?.();
        }}
        onError={() => setErr(true)}
        onProgress={(e) => {
          const v = e.currentTarget;
          if (v.buffered.length) setBuffered(v.buffered.end(v.buffered.length - 1));
        }}
        onTimeUpdate={(e) => {
          const v = e.currentTarget;
          setPos(v.currentTime);
          const d = v.currentTime - lastT.current;
          if (d > 0 && d < 1.5 && !v.paused) watched.current += d;
          lastT.current = v.currentTime;
          if (!viewSent.current && watched.current >= 3 && !src) {
            viewSent.current = true;
            void recordView(video.id, 0);
          }
          flush(false);
        }}
        style={{ width: "100%", height: "100%", display: "block", objectFit: "contain", background: "#000" }}
      >
        {captions.map((c, i) => (
          <track key={c.id} kind="captions" src={c.stream_url} srcLang={c.lang} label={c.label || c.lang} default={i === 0 && ccOn} />
        ))}
      </video>
      <video ref={prev} src={url} muted preload="metadata" crossOrigin="use-credentials" onSeeked={drawPreview} style={{ display: "none" }} />

      {err && (
        <Box sx={{ position: "absolute", inset: 0, display: "flex", alignItems: "center", justifyContent: "center", color: "#fff", textAlign: "center", p: 2 }}>
          <Typography level="body-md" sx={{ color: "#fff" }}>This video cannot be played in your browser. You can still download it.</Typography>
        </Box>
      )}
      {ended && !err && (
        <Box sx={{ position: "absolute", inset: 0, display: "flex", alignItems: "center", justifyContent: "center", bgcolor: "rgba(0,0,0,.45)" }}>
          <IconButton aria-label="Replay" onClick={toggle} sx={{ width: 64, height: 64, bgcolor: "rgba(255,255,255,.18)", color: "#fff", borderRadius: "50%", "&:hover": { bgcolor: "rgba(255,255,255,.3)" } }}>
            <ReplayRoundedIcon sx={{ fontSize: 34 }} />
          </IconButton>
        </Box>
      )}
      {!playing && !ended && !err && (
        <IconButton aria-label="Play" onClick={toggle} sx={{ position: "absolute", left: "50%", top: "50%", transform: "translate(-50%,-50%)", width: 68, height: 68, borderRadius: "50%", bgcolor: "rgba(14,36,32,.62)", color: "#fff", "&:hover": { bgcolor: "primary.solidBg" } }}>
          <PlayArrowRoundedIcon sx={{ fontSize: 40 }} />
        </IconButton>
      )}
      {toast && (
        <Box role="status" sx={{ position: "absolute", top: 16, left: "50%", transform: "translateX(-50%)", px: 1.5, py: 0.5, borderRadius: "999px", bgcolor: "rgba(0,0,0,.7)", color: "#fff", fontSize: 13, fontWeight: 600 }}>
          {toast}
        </Box>
      )}

      {/* Controls */}
      <Box
        sx={{
          position: "absolute",
          left: 0,
          right: 0,
          bottom: 0,
          px: { xs: 1, sm: 1.5 },
          pt: 4,
          pb: 0.5,
          background: "linear-gradient(transparent, rgba(0,0,0,.72))",
          opacity: showUi ? 1 : 0,
          transition: "opacity .2s",
          pointerEvents: showUi ? "auto" : "none",
        }}
      >
        <Box
          ref={bar}
          role="slider"
          tabIndex={0}
          aria-label="Seek"
          aria-valuemin={0}
          aria-valuemax={Math.round(dur)}
          aria-valuenow={Math.round(pos)}
          data-testid="watch-seek"
          onPointerMove={onBarMove}
          onPointerLeave={() => setHover(null)}
          onPointerDown={(e) => {
            (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
            dragSeek(e);
          }}
          onPointerMoveCapture={(e) => e.buttons === 1 && dragSeek(e)}
          sx={{ position: "relative", height: 18, display: "flex", alignItems: "center", cursor: "pointer", touchAction: "none", "&:hover .sk-track, &:focus-visible .sk-track": { height: 6 } }}
        >
          <Box className="sk-track" sx={{ position: "relative", width: "100%", height: 4, borderRadius: 3, bgcolor: "rgba(255,255,255,.28)", transition: "height .1s", display: "flex", gap: "2px", overflow: "visible" }}>
            {(chapters.length ? chapters : [{ time: 0, label: "" }]).map((c, i, all) => {
              const end = all[i + 1]?.time ?? dur;
              const w = dur ? ((end - c.time) / dur) * 100 : 100;
              const fill = Math.min(100, Math.max(0, dur ? ((pos - c.time) / (end - c.time)) * 100 : 0));
              const buf = Math.min(100, Math.max(0, dur ? ((buffered - c.time) / (end - c.time)) * 100 : 0));
              return (
                <Box key={i} sx={{ width: `${w}%`, position: "relative", bgcolor: "rgba(255,255,255,.28)", borderRadius: 3, overflow: "hidden" }}>
                  <Box sx={{ position: "absolute", inset: 0, width: `${buf}%`, bgcolor: "rgba(255,255,255,.35)" }} />
                  <Box sx={{ position: "absolute", inset: 0, width: `${fill}%`, bgcolor: "primary.400" }} />
                </Box>
              );
            })}
            <Box sx={{ position: "absolute", left: `${pct}%`, top: "50%", width: 13, height: 13, borderRadius: "50%", bgcolor: "primary.300", transform: "translate(-50%,-50%)", boxShadow: "0 1px 4px rgba(0,0,0,.4)" }} />
          </Box>
          {hover && (
            <Box sx={{ position: "absolute", bottom: 26, left: Math.min(Math.max(hover.x, 80), (bar.current?.clientWidth ?? 0) - 80), transform: "translateX(-50%)", textAlign: "center", pointerEvents: "none" }}>
              <canvas ref={canvas} width={160} height={90} style={{ width: 160, height: 90, borderRadius: 8, background: "#111", display: "block", border: "2px solid #fff" }} />
              <Typography level="body-xs" sx={{ color: "#fff", mt: 0.5, textShadow: "0 1px 3px #000", fontWeight: 600 }}>
                {chapters.length ? chapters[chapterAt(chapters, hover.t)]?.label : ""}
              </Typography>
              <Typography level="body-xs" sx={{ color: "#fff", textShadow: "0 1px 3px #000" }}>{clock(hover.t)}</Typography>
            </Box>
          )}
        </Box>

        <Box sx={{ display: "flex", alignItems: "center", gap: { xs: 0, sm: 0.5 }, color: "#fff" }}>
          <IconButton aria-label={playing ? "Pause" : "Play"} data-testid="watch-toggle" onClick={toggle} sx={{ color: "#fff" }}>
            {playing ? <PauseRoundedIcon /> : <PlayArrowRoundedIcon />}
          </IconButton>
          <Box sx={{ display: { xs: "none", sm: "flex" }, alignItems: "center" }}>
            <IconButton
              aria-label={muted ? "Unmute" : "Mute"}
              onClick={() => {
                const v = vid.current;
                if (v) {
                  v.muted = !v.muted;
                  setMuted(v.muted);
                }
              }}
              sx={{ color: "#fff" }}
            >
              {muted || volume === 0 ? <VolumeOffRoundedIcon /> : <VolumeUpRoundedIcon />}
            </IconButton>
            <Slider
              aria-label="Volume"
              size="sm"
              min={0}
              max={1}
              step={0.02}
              value={muted ? 0 : volume}
              onChange={(_, val) => {
                const v = vid.current;
                if (!v) return;
                v.volume = val as number;
                v.muted = false;
                setVolume(val as number);
                setMuted(false);
              }}
              sx={{ width: 72, color: "#fff", "--Slider-trackSize": "3px", "--Slider-thumbSize": "11px", mx: 1 }}
            />
          </Box>
          <Typography level="body-xs" sx={{ color: "#fff", fontVariantNumeric: "tabular-nums", ml: 0.5, whiteSpace: "nowrap" }}>
            {clock(pos)} / {clock(dur)}
          </Typography>
          {cur >= 0 && (
            <Typography level="body-xs" noWrap sx={{ color: "rgba(255,255,255,.85)", ml: 1, display: { xs: "none", md: "block" }, minWidth: 0, flex: "0 1 220px" }}>
              · {chapters[cur].label}
            </Typography>
          )}
          <Box sx={{ flex: 1 }} />
          {captions.length > 0 && (
            <IconButton aria-label={ccOn ? "Turn captions off" : "Turn captions on"} aria-pressed={ccOn} data-testid="watch-cc" onClick={() => setCaptions(!ccOn)} sx={{ color: "#fff", borderBottom: ccOn ? "2px solid" : "2px solid transparent", borderColor: "primary.300", borderRadius: 0 }}>
              {ccOn ? <ClosedCaptionRoundedIcon /> : <ClosedCaptionDisabledRoundedIcon />}
            </IconButton>
          )}
          <Dropdown>
            <MenuButton slots={{ root: IconButton }} slotProps={{ root: { "aria-label": `Playback speed ${speed}x`, sx: { color: "#fff" } } }}>
              {speed === 1 ? <SettingsRoundedIcon /> : <Typography level="body-xs" sx={{ color: "#fff", fontWeight: 700 }}>{speed}x</Typography>}
            </MenuButton>
            <Menu size="sm" placement="top-end" sx={{ zIndex: 1500 }}>
              {SPEEDS.map((s) => (
                <MenuItem key={s} selected={s === speed} onClick={() => changeSpeed(s)}>
                  {s === 1 ? "Normal" : `${s}x`}
                </MenuItem>
              ))}
            </Menu>
          </Dropdown>
          <IconButton aria-label="Picture in picture" onClick={() => void togglePip()} sx={{ color: "#fff", display: { xs: "none", sm: "inline-flex" } }}>
            <PictureInPictureAltRoundedIcon />
          </IconButton>
          <IconButton aria-label={theatre ? "Default view" : "Theatre mode"} aria-pressed={theatre} data-testid="watch-theatre" onClick={onToggleTheatre} sx={{ color: "#fff", display: { xs: "none", md: "inline-flex" } }}>
            <CropLandscapeRoundedIcon />
          </IconButton>
          <IconButton aria-label={fs ? "Exit full screen" : "Full screen"} data-testid="watch-fullscreen" onClick={toggleFs} sx={{ color: "#fff" }}>
            {fs ? <FullscreenExitRoundedIcon /> : <FullscreenRoundedIcon />}
          </IconButton>
        </Box>
      </Box>
    </Box>
  );
});
