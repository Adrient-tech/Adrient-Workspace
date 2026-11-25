/**
 * Player of the Stories app: one audio element for generated audio, and the
 * browser's own voice (speech synthesis) when an episode has no generated audio.
 * Keeps the position per user, runs the sleep timer and the keyboard shortcuts.
 */
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Box } from "@mui/joy";
import PlayArrowRounded from "@mui/icons-material/PlayArrowRounded";
import PauseRounded from "@mui/icons-material/PauseRounded";
import Replay10Rounded from "@mui/icons-material/Replay10Rounded";
import Forward10Rounded from "@mui/icons-material/Forward10Rounded";
import SkipNextRounded from "@mui/icons-material/SkipNextRounded";
import CloseRounded from "@mui/icons-material/CloseRounded";
import BedtimeOutlined from "@mui/icons-material/BedtimeOutlined";
import SubjectRounded from "@mui/icons-material/SubjectRounded";
import { stories, type EpisodeDetail } from "./storyApi";
import {
  SPEEDS, chooseVoice, clampSeek, clampSpeed, estimateTimings, fmtTime, paragraphs, resumePosition, sentenceAt, shortcutFor, shouldSave, sleepDue,
  sleepLabel, stepSpeed, type Sleep, type Timing,
} from "./playerLogic";
import { C, Cover, FONT_DISPLAY, IconBtn, Skeleton, useIsMobile, useToast, errMsg } from "./ui";

export interface PlayerApi {
  ep: EpisodeDetail | null;
  playing: boolean;
  loading: boolean;
  positionMs: number;
  durationMs: number;
  speed: number;
  sentence: number;
  mode: "audio" | "speech" | null;
  sleep: Sleep;
  error: string;
  timings: Timing[];
  play: (id: string, opts?: { fromMs?: number; sentence?: number }) => Promise<void>;
  toggle: () => void;
  seek: (ms: number) => void;
  seekSentence: (i: number) => void;
  skip: (deltaMs: number) => void;
  setSpeed: (v: number) => void;
  setSleep: (s: Sleep) => void;
  next: () => void;
  stop: () => void;
  readAlongOpen: boolean;
  setReadAlongOpen: (v: boolean) => void;
}

const Ctx = createContext<PlayerApi | null>(null);
export function usePlayer(): PlayerApi {
  const c = useContext(Ctx);
  if (!c) throw new Error("usePlayer outside PlayerProvider");
  return c;
}

const SPEED_KEY = "stories.speed";
const VOICE_KEY = "stories.browserVoice";

function pitchFor(name: string): number {
  if (!name) return 1;
  let h = 0;
  for (const ch of name) h = (h * 31 + ch.charCodeAt(0)) >>> 0;
  return 0.8 + (h % 9) * 0.06; // 0.8 .. 1.28
}

export function PlayerProvider({ children }: { children: ReactNode }) {
  const toast = useToast();
  const [ep, setEp] = useState<EpisodeDetail | null>(null);
  const [playing, setPlaying] = useState(false);
  const [loading, setLoading] = useState(false);
  const [positionMs, setPositionMs] = useState(0);
  const [durationMs, setDurationMs] = useState(0);
  const [speed, setSpeedState] = useState<number>(() => clampSpeed(Number(localStorage.getItem(SPEED_KEY)) || 1));
  const [sentence, setSentence] = useState(-1);
  const [mode, setMode] = useState<"audio" | "speech" | null>(null);
  const [sleep, setSleepState] = useState<Sleep>({ kind: "off" });
  const [error, setError] = useState("");
  const [timings, setTimings] = useState<Timing[]>([]);
  const [readAlongOpen, setReadAlongOpen] = useState(false);

  const audio = useRef<HTMLAudioElement | null>(null);
  const R = useRef({ ep: null as EpisodeDetail | null, mode: null as "audio" | "speech" | null, speed, timings: [] as Timing[], duration: 0, lastSaved: 0, speechIdx: 0, speechStart: 0, speechToken: 0, sleep: { kind: "off" } as Sleep, playing: false, pos: 0 });
  R.current.speed = speed;
  R.current.sleep = sleep;

  const saveNow = useCallback((completed = false) => {
    const e = R.current.ep;
    if (!e) return;
    R.current.lastSaved = R.current.pos;
    void stories.saveProgress(e.id, R.current.pos, R.current.duration, completed).catch(() => undefined);
  }, []);

  const setPos = useCallback((ms: number) => {
    R.current.pos = ms;
    setPositionMs(ms);
    if (R.current.mode === "audio") setSentence(sentenceAt(R.current.timings, ms));
    if (shouldSave(R.current.lastSaved, ms)) saveNow();
  }, [saveNow]);

  // ---- speech (browser voice) ----
  const cancelSpeech = useCallback(() => {
    R.current.speechToken++;
    if (typeof speechSynthesis !== "undefined") speechSynthesis.cancel();
  }, []);

  const finish = useCallback(() => {
    const e = R.current.ep;
    setPlaying(false);
    R.current.playing = false;
    if (!e) return;
    R.current.pos = R.current.duration;
    setPositionMs(R.current.duration);
    saveNow(true);
  }, [saveNow]);

  const nextRef = useRef<() => void>(() => undefined);

  const onEnded = useCallback(() => {
    finish();
    if (R.current.sleep.kind === "end") {
      setSleepState({ kind: "off" });
      return;
    }
    nextRef.current();
  }, [finish]);

  const speakFrom = useCallback((start: number) => {
    const e = R.current.ep;
    if (!e || typeof speechSynthesis === "undefined") return;
    cancelSpeech();
    const token = R.current.speechToken;
    const voices = speechSynthesis.getVoices();
    const base = chooseVoice(voices, e.effective.language || e.language || "en", localStorage.getItem(VOICE_KEY) ?? undefined);
    const say = (i: number) => {
      if (token !== R.current.speechToken) return;
      const s = e.sentences[i];
      if (!s) {
        onEnded();
        return;
      }
      const u = new SpeechSynthesisUtterance(s.text);
      if (base) u.voice = base;
      u.lang = base?.lang ?? (e.effective.language || "en");
      u.rate = R.current.speed;
      u.pitch = s.speaker ? pitchFor(s.speaker) : 1;
      u.onstart = () => {
        if (token !== R.current.speechToken) return;
        R.current.speechIdx = i;
        R.current.speechStart = Date.now();
        setSentence(i);
        const t = R.current.timings[i];
        if (t) setPos(t.s);
      };
      u.onend = () => say(i + 1);
      u.onerror = (ev) => {
        if (token !== R.current.speechToken) return;
        if ((ev as SpeechSynthesisErrorEvent).error === "interrupted" || (ev as SpeechSynthesisErrorEvent).error === "canceled") return;
        setError("The browser could not read this aloud.");
        setPlaying(false);
      };
      speechSynthesis.speak(u);
    };
    R.current.speechIdx = start;
    say(start);
  }, [cancelSpeech, onEnded, setPos]);

  // interpolate position between sentence starts while the browser voice speaks
  useEffect(() => {
    if (!playing || mode !== "speech") return;
    const id = setInterval(() => {
      const t = R.current.timings[R.current.speechIdx];
      if (!t) return;
      const el = (Date.now() - R.current.speechStart) * R.current.speed;
      setPos(Math.min(t.e, t.s + el));
    }, 300);
    return () => clearInterval(id);
  }, [playing, mode, setPos]);

  // ---- audio element ----
  useEffect(() => {
    const a = new Audio();
    a.preload = "metadata";
    audio.current = a;
    a.addEventListener("timeupdate", () => R.current.mode === "audio" && setPos(a.currentTime * 1000));
    a.addEventListener("loadedmetadata", () => {
      if (R.current.mode !== "audio" || !Number.isFinite(a.duration)) return;
      R.current.duration = Math.round(a.duration * 1000);
      setDurationMs(R.current.duration);
    });
    a.addEventListener("play", () => { if (R.current.mode === "audio") { R.current.playing = true; setPlaying(true); } });
    a.addEventListener("pause", () => { if (R.current.mode === "audio" && !a.ended) { R.current.playing = false; setPlaying(false); saveNow(); } });
    a.addEventListener("ended", () => R.current.mode === "audio" && onEnded());
    a.addEventListener("error", () => { if (R.current.mode === "audio") { setError("The audio could not be loaded."); setPlaying(false); } });
    const hide = () => document.visibilityState === "hidden" && R.current.ep && saveNow();
    document.addEventListener("visibilitychange", hide);
    window.addEventListener("pagehide", hide);
    return () => {
      document.removeEventListener("visibilitychange", hide);
      window.removeEventListener("pagehide", hide);
      a.pause();
      a.src = "";
      cancelSpeech();
    };
  }, [cancelSpeech, onEnded, saveNow, setPos]);

  // ---- sleep timer ----
  useEffect(() => {
    if (sleep.kind !== "minutes") return;
    const id = setInterval(() => {
      if (sleepDue(R.current.sleep, Date.now(), false)) {
        pauseInner();
        setSleepState({ kind: "off" });
        toast.show("Sleep timer: playback stopped");
      }
    }, 2000);
    return () => clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sleep]);

  const pauseInner = useCallback(() => {
    if (R.current.mode === "audio") audio.current?.pause();
    else if (R.current.mode === "speech") {
      cancelSpeech();
      R.current.playing = false;
      setPlaying(false);
      saveNow();
    }
  }, [cancelSpeech, saveNow]);

  const play = useCallback(async (id: string, opts?: { fromMs?: number; sentence?: number }) => {
    const cur = R.current.ep;
    if (cur && cur.id === id && opts?.fromMs === undefined && opts?.sentence === undefined) {
      if (!R.current.playing) {
        if (R.current.mode === "audio") void audio.current?.play();
        else speakFrom(Math.max(0, sentenceAt(R.current.timings, R.current.pos)));
        R.current.playing = true;
        setPlaying(true);
      }
      return;
    }
    setLoading(true);
    setError("");
    try {
      if (R.current.ep && R.current.playing) pauseInner();
      cancelSpeech();
      const d = await stories.episode(id);
      const useAudio = d.has_audio && !!d.audio_url;
      if (!useAudio && typeof speechSynthesis === "undefined") throw new Error("This episode has no generated audio and your browser cannot read it aloud.");
      if (!useAudio && d.sentences.length === 0) throw new Error("This episode has no text to read.");
      const dur = useAudio ? d.duration_ms : Math.max(d.duration_ms, 1000);
      const tm = useAudio && d.timings.length === d.sentences.length && d.timings.length > 0 ? d.timings : estimateTimings(d.sentences, dur);
      let from = opts?.fromMs ?? resumePosition(d.position_ms, dur, d.completed);
      if (opts?.sentence !== undefined && tm[opts.sentence]) from = tm[opts.sentence].s;
      R.current = { ...R.current, ep: d, mode: useAudio ? "audio" : "speech", timings: tm, duration: dur, lastSaved: from, pos: from };
      setEp(d);
      setMode(useAudio ? "audio" : "speech");
      setTimings(tm);
      setDurationMs(dur);
      setPositionMs(from);
      setSentence(sentenceAt(tm, from));
      void stories.play(d.id).catch(() => undefined);
      R.current.playing = true;
      setPlaying(true);
      if (useAudio) {
        const a = audio.current!;
        a.src = d.audio_url!;
        a.playbackRate = R.current.speed;
        a.currentTime = from / 1000;
        await a.play();
      } else {
        speakFrom(Math.max(0, sentenceAt(tm, from)));
      }
    } catch (e) {
      setError(errMsg(e));
      toast.show(errMsg(e), "error");
      R.current.playing = false;
      setPlaying(false);
    } finally {
      setLoading(false);
    }
  }, [cancelSpeech, pauseInner, speakFrom, toast]);

  const toggle = useCallback(() => {
    const e = R.current.ep;
    if (!e) return;
    if (R.current.playing) pauseInner();
    else void play(e.id);
  }, [pauseInner, play]);

  const seek = useCallback((ms: number) => {
    const to = clampSeek(ms, R.current.duration);
    if (R.current.mode === "audio" && audio.current) {
      audio.current.currentTime = to / 1000;
      setPos(to);
    } else if (R.current.mode === "speech") {
      const i = Math.max(0, sentenceAt(R.current.timings, to));
      setSentence(i);
      setPos(R.current.timings[i]?.s ?? to);
      if (R.current.playing) speakFrom(i);
      else R.current.speechIdx = i;
    }
  }, [setPos, speakFrom]);

  const skip = useCallback((d: number) => seek(R.current.pos + d), [seek]);
  const seekSentence = useCallback((i: number) => {
    const t = R.current.timings[i];
    if (t) seek(t.s);
  }, [seek]);

  const setSpeed = useCallback((v: number) => {
    const s = clampSpeed(v);
    setSpeedState(s);
    R.current.speed = s;
    localStorage.setItem(SPEED_KEY, String(s));
    if (audio.current) audio.current.playbackRate = s;
    if (R.current.mode === "speech" && R.current.playing) speakFrom(R.current.speechIdx);
  }, [speakFrom]);

  const next = useCallback(() => {
    const id = R.current.ep?.next_episode_id;
    if (id) void play(id, { fromMs: 0 });
  }, [play]);
  nextRef.current = next;

  const stop = useCallback(() => {
    pauseInner();
    cancelSpeech();
    if (audio.current) audio.current.src = "";
    R.current = { ...R.current, ep: null, mode: null, playing: false };
    setEp(null);
    setPlaying(false);
    setMode(null);
    setReadAlongOpen(false);
  }, [cancelSpeech, pauseInner]);

  const setSleep = useCallback((s: Sleep) => setSleepState(s), []);

  // keyboard shortcuts
  useEffect(() => {
    const f = (e: KeyboardEvent) => {
      if (!R.current.ep) return;
      const t = e.target as HTMLElement | null;
      const typing = !!t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.tagName === "SELECT" || t.isContentEditable);
      const act = shortcutFor(e, typing);
      if (!act) return;
      if ((act === "toggle") && t && (t.tagName === "BUTTON" || t.tagName === "A") && e.key === " ") return; // let the button act
      e.preventDefault();
      if (act === "toggle") toggle();
      else if (act === "back") skip(-10000);
      else if (act === "forward") skip(10000);
      else if (act === "next") next();
      else if (act === "previous") seek(0);
      else if (act === "faster") setSpeed(stepSpeed(R.current.speed, 1));
      else if (act === "slower") setSpeed(stepSpeed(R.current.speed, -1));
    };
    window.addEventListener("keydown", f);
    return () => window.removeEventListener("keydown", f);
  }, [toggle, skip, next, seek, setSpeed]);

  // lock-screen / media key controls
  useEffect(() => {
    if (!ep || !("mediaSession" in navigator)) return;
    navigator.mediaSession.metadata = new MediaMetadata({ title: ep.title, artist: ep.author.name, album: ep.show_title });
    navigator.mediaSession.setActionHandler("play", () => toggle());
    navigator.mediaSession.setActionHandler("pause", () => toggle());
    navigator.mediaSession.setActionHandler("seekbackward", () => skip(-10000));
    navigator.mediaSession.setActionHandler("seekforward", () => skip(10000));
    navigator.mediaSession.setActionHandler("nexttrack", ep.next_episode_id ? () => next() : null);
  }, [ep, toggle, skip, next]);

  const api: PlayerApi = useMemo(() => ({
    ep, playing, loading, positionMs, durationMs, speed, sentence, mode, sleep, error, timings, play, toggle, seek, seekSentence, skip, setSpeed, setSleep, next, stop,
    readAlongOpen, setReadAlongOpen,
  }), [ep, playing, loading, positionMs, durationMs, speed, sentence, mode, sleep, error, timings, play, toggle, seek, seekSentence, skip, setSpeed, setSleep, next, stop, readAlongOpen]);

  return <Ctx.Provider value={api}>{children}</Ctx.Provider>;
}

// ---- mini player ----------------------------------------------------------------

export function MiniPlayer() {
  const p = usePlayer();
  const mobile = useIsMobile();
  const [menu, setMenu] = useState<"" | "speed" | "sleep">("");
  if (!p.ep && !p.loading) return null;
  const ep = p.ep;
  const pct = p.durationMs > 0 ? Math.min(100, (p.positionMs / p.durationMs) * 100) : 0;
  return (
    <Box
      data-testid="mini-player"
      role="region"
      aria-label="Player"
      sx={{
        position: "fixed", left: { xs: 8, md: "50%" }, right: { xs: 8, md: "auto" }, bottom: { xs: 8, md: 16 }, transform: { md: "translateX(-50%)" },
        width: { md: "min(920px, calc(100vw - 48px))" }, zIndex: 1300, background: "rgba(255,255,255,.96)", backdropFilter: "blur(12px)", color: C.ink,
        borderRadius: "20px", boxShadow: "0 10px 40px rgba(22,33,30,.22), 0 0 0 1px rgba(22,33,30,.06)", overflow: "visible",
      }}
    >
      <Box sx={{ position: "absolute", left: 16, right: 16, top: 0, height: 3, background: C.line, borderRadius: 2, overflow: "hidden" }}>
        <Box sx={{ width: `${pct}%`, height: "100%", background: C.accent }} />
      </Box>
      <Box sx={{ display: "flex", alignItems: "center", gap: { xs: 1, md: 2 }, p: { xs: 1, md: 1.4 }, pt: { xs: 1.3, md: 1.7 } }}>
        {ep ? <Cover title={ep.show_title} url={ep.cover_url} style={ep.cover_style} size={mobile ? 44 : 52} radius={10} /> : <Skeleton w={52} h={52} r={10} />}
        <Box sx={{ minWidth: 0, flex: { xs: 1, md: "0 1 260px" } }}>
          <Box sx={{ fontWeight: 700, fontSize: 14.5, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }} data-testid="mini-title">{ep?.title ?? "Loading…"}</Box>
          <Box sx={{ fontSize: 12.5, color: C.muted, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>
            {p.error ? <span style={{ color: C.danger }}>{p.error}</span> : `${ep?.show_title ?? ""} · ${ep?.author.name ?? ""}`}
          </Box>
        </Box>
        <Box sx={{ display: "flex", alignItems: "center", gap: 0.3 }}>
          <IconBtn label="Back 10 seconds" onClick={() => p.skip(-10000)} testId="back10"><Replay10Rounded /></IconBtn>
          <IconBtn
            label={p.playing ? "Pause" : "Play"}
            onClick={p.toggle}
            size={48}
            testId="toggle"
            sx={{ background: C.accent, color: "#fff", "&:hover:not(:disabled)": { background: C.accentDark } }}
          >
            {p.playing ? <PauseRounded fontSize="large" /> : <PlayArrowRounded fontSize="large" />}
          </IconBtn>
          <IconBtn label="Forward 10 seconds" onClick={() => p.skip(10000)} testId="forward10"><Forward10Rounded /></IconBtn>
          {!mobile && <IconBtn label="Next episode" onClick={p.next} disabled={!ep?.next_episode_id}><SkipNextRounded /></IconBtn>}
        </Box>
        {!mobile && (
          <Box sx={{ flex: 1, display: "flex", alignItems: "center", gap: 1, minWidth: 0 }}>
            <Box sx={{ fontSize: 12, color: C.muted, width: 44, textAlign: "right", fontVariantNumeric: "tabular-nums" }}>{fmtTime(p.positionMs)}</Box>
            <Box
              component="input"
              type="range"
              min={0}
              max={Math.max(1, p.durationMs)}
              value={Math.min(p.positionMs, p.durationMs || 1)}
              aria-label="Seek"
              onChange={(e: React.ChangeEvent<HTMLInputElement>) => p.seek(Number(e.target.value))}
              sx={{ flex: 1, accentColor: C.accent, minWidth: 60 }}
            />
            <Box sx={{ fontSize: 12, color: C.muted, width: 44, fontVariantNumeric: "tabular-nums" }}>{fmtTime(p.durationMs)}</Box>
          </Box>
        )}
        <Box sx={{ display: "flex", alignItems: "center", position: "relative" }}>
          <Box component="button" type="button" onClick={() => setMenu(menu === "speed" ? "" : "speed")} aria-label="Playback speed" data-testid="speed-btn"
            sx={{ border: `1px solid ${C.line}`, background: C.surface, borderRadius: "999px", px: 1.2, py: 0.5, fontSize: 13, fontWeight: 700, cursor: "pointer", color: C.ink, fontFamily: "inherit" }}>
            {p.speed}×
          </Box>
          {!mobile && (
            <IconBtn label={p.sleep.kind === "off" ? "Sleep timer" : `Sleep timer: ${sleepLabel(p.sleep, Date.now())}`} onClick={() => setMenu(menu === "sleep" ? "" : "sleep")} active={p.sleep.kind !== "off"} testId="sleep-btn">
              <BedtimeOutlined fontSize="small" />
            </IconBtn>
          )}
          <IconBtn label="Read along" onClick={() => p.setReadAlongOpen(!p.readAlongOpen)} active={p.readAlongOpen} testId="readalong-toggle"><SubjectRounded /></IconBtn>
          <IconBtn label="Close player" onClick={p.stop} size={34}><CloseRounded fontSize="small" /></IconBtn>
          {menu === "speed" && (
            <Menu onClose={() => setMenu("")}>
              {SPEEDS.map((s) => (
                <MenuRow key={s} active={s === p.speed} onClick={() => { p.setSpeed(s); setMenu(""); }}>{s}×</MenuRow>
              ))}
            </Menu>
          )}
          {menu === "sleep" && (
            <Menu onClose={() => setMenu("")}>
              <MenuRow active={p.sleep.kind === "off"} onClick={() => { p.setSleep({ kind: "off" }); setMenu(""); }}>Off</MenuRow>
              {[5, 15, 30, 45, 60].map((m) => (
                <MenuRow key={m} onClick={() => { p.setSleep({ kind: "minutes", minutes: m, until: Date.now() + m * 60000 }); setMenu(""); }}>{m} minutes</MenuRow>
              ))}
              <MenuRow active={p.sleep.kind === "end"} onClick={() => { p.setSleep({ kind: "end" }); setMenu(""); }}>End of episode</MenuRow>
            </Menu>
          )}
        </Box>
      </Box>
      {p.readAlongOpen && ep && <ReadAlongSheet />}
    </Box>
  );
}

function Menu({ children, onClose }: { children: ReactNode; onClose: () => void }) {
  useEffect(() => {
    const f = () => onClose();
    const t = setTimeout(() => window.addEventListener("click", f), 0);
    return () => { clearTimeout(t); window.removeEventListener("click", f); };
  }, [onClose]);
  return (
    <Box role="menu" sx={{ position: "absolute", bottom: "calc(100% + 14px)", right: 0, background: C.surface, borderRadius: "14px", boxShadow: "0 8px 30px rgba(22,33,30,.22)", p: 0.6, minWidth: 150, zIndex: 5 }}>
      {children}
    </Box>
  );
}
function MenuRow({ children, onClick, active }: { children: ReactNode; onClick: () => void; active?: boolean }) {
  return (
    <Box component="button" type="button" role="menuitem" onClick={onClick}
      sx={{ display: "block", width: "100%", textAlign: "left", border: "none", background: active ? C.accentSoft : "transparent", color: active ? C.accentDark : C.ink, fontWeight: active ? 700 : 500, fontFamily: "inherit", fontSize: 14, padding: "8px 12px", borderRadius: "9px", cursor: "pointer", "&:hover": { background: C.chip } }}>
      {children}
    </Box>
  );
}

// ---- read along -------------------------------------------------------------------

/** The story text with the spoken sentence highlighted; click a sentence to jump to it. */
export function StoryText({ ep, current, onPick, maxHeight }: { ep: EpisodeDetail; current: number; onPick?: (i: number) => void; maxHeight?: number | string }) {
  const paras = useMemo(() => paragraphs(ep.sentences), [ep.sentences]);
  const active = useRef<HTMLSpanElement | null>(null);
  useEffect(() => {
    active.current?.scrollIntoView({ block: "nearest", behavior: "smooth" });
  }, [current]);
  if (ep.sentences.length === 0) return <Box sx={{ color: C.muted, fontSize: 14, py: 1 }}>This episode has no text yet.</Box>;
  return (
    <Box data-testid="story-text" sx={{ maxHeight, overflowY: maxHeight ? "auto" : "visible", pr: 1, fontSize: 16, lineHeight: 1.75, color: C.ink }}>
      {paras.map((p) => (
        <Box component="p" key={p.para} sx={{ m: 0, mb: 1.4 }}>
          {p.speaker && <Box component="span" sx={{ fontWeight: 800, color: C.accent, fontSize: 12.5, textTransform: "uppercase", letterSpacing: ".06em", mr: 0.8 }}>{p.speaker}</Box>}
          {p.items.map((it) => (
            <Box
              component="span"
              key={it.i}
              ref={it.i === current ? active : undefined}
              data-active={it.i === current ? "true" : undefined}
              onClick={onPick ? () => onPick(it.i) : undefined}
              sx={{
                cursor: onPick ? "pointer" : "default", borderRadius: "6px", padding: "1px 2px", background: it.i === current ? "#FFE9A8" : "transparent",
                transition: "background .2s", "&:hover": onPick ? { background: it.i === current ? "#FFE9A8" : C.chip } : {},
              }}
            >
              {it.text}{" "}
            </Box>
          ))}
        </Box>
      ))}
    </Box>
  );
}

function ReadAlongSheet() {
  const p = usePlayer();
  const mobile = useIsMobile();
  if (!p.ep) return null;
  return (
    <Box
      data-testid="readalong-sheet"
      sx={{ position: "absolute", left: 0, right: 0, bottom: "calc(100% + 10px)", background: C.surface, borderRadius: "20px", boxShadow: "0 10px 40px rgba(22,33,30,.22), 0 0 0 1px rgba(22,33,30,.06)", p: { xs: 2, md: 3 }, maxHeight: mobile ? "60vh" : "52vh", display: "flex", flexDirection: "column" }}
    >
      <Box sx={{ fontFamily: FONT_DISPLAY, fontWeight: 800, fontSize: 17, mb: 0.3 }}>{p.ep.title}</Box>
      {p.ep.summary && <Box sx={{ color: C.muted, fontSize: 13.5, mb: 1.5 }}>{p.ep.summary}</Box>}
      <StoryText ep={p.ep} current={p.sentence} onPick={p.seekSentence} maxHeight="100%" />
      <Box sx={{ fontSize: 12, color: C.faint, mt: 1 }}>
        {p.mode === "speech" ? "Read aloud by your browser's voice." : "Highlighting follows the audio."} Click a sentence to jump to it.
      </Box>
    </Box>
  );
}
