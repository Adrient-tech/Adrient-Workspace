import { createContext, useContext, useRef, useState, useCallback, useEffect } from "react";
import type { ReactNode } from "react";
import { streamUrl, recordPlay } from "./api";
import type { Track, Station } from "./types";
import { appendTrack, insertNext, moveItem, nextIndex, prevIndex, removeAt, type RepeatMode } from "./logic";

export type { RepeatMode } from "./logic";

/** PlayerState is the public surface of the music player. One persistent
 *  <audio> element backs the whole app so playback survives navigation; a
 *  second, silent element preloads the next track so the hand-over is quick
 *  ("gapless-ish"). The queue, volume, shuffle, repeat and speed are kept in
 *  localStorage. */
export interface PlayerState {
  queue: Track[];
  index: number;
  current: Track | null;
  playing: boolean;
  position: number;
  duration: number;
  volume: number;
  muted: boolean;
  shuffle: boolean;
  repeat: RepeatMode;
  /** Playback speed (1 = normal). */
  speed: number;
  radioStation: Station | null;
  playQueue: (queue: Track[], startIndex: number) => void;
  /** Jump to an entry of the current queue. */
  playAt: (index: number) => void;
  playRadio: (station: Station) => void;
  stopRadio: () => void;
  playTrack: (track: Track) => void;
  playNext: (track: Track) => void;
  addToQueue: (track: Track) => void;
  /** Drag-to-reorder support: move one queue entry. */
  moveInQueue: (from: number, to: number) => void;
  /** Remove the entry at a queue position. */
  removeAtIndex: (at: number) => void;
  clearQueue: () => void;
  toggle: () => void;
  next: () => void;
  prev: () => void;
  seek: (seconds: number) => void;
  seekBy: (delta: number) => void;
  setVolume: (v: number) => void;
  toggleMute: () => void;
  toggleShuffle: () => void;
  cycleRepeat: () => void;
  setSpeed: (s: number) => void;
  removeFromQueue: (trackId: string) => void;
  /** Patch a track everywhere it is held (liked flag, edited title). */
  patchTrack: (id: string, patch: Partial<Track>) => void;
  airplayAvailable: boolean;
  showAirplay: () => void;
}

const Ctx = createContext<PlayerState | null>(null);

export function usePlayer(): PlayerState {
  const v = useContext(Ctx);
  if (!v) throw new Error("usePlayer must be used within <PlayerProvider>");
  return v;
}

const STORE = "adrient.music.player.v1";

interface Saved {
  queue: Track[];
  index: number;
  volume: number;
  shuffle: boolean;
  repeat: RepeatMode;
  speed: number;
}

function loadSaved(): Partial<Saved> {
  try {
    const raw = localStorage.getItem(STORE);
    return raw ? (JSON.parse(raw) as Partial<Saved>) : {};
  } catch {
    return {};
  }
}

export function PlayerProvider({ children }: { children: ReactNode }) {
  const saved = useRef<Partial<Saved>>(loadSaved());
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const preloadRef = useRef<HTMLAudioElement | null>(null);
  const [queue, setQueue] = useState<Track[]>(() => (Array.isArray(saved.current.queue) ? saved.current.queue.slice(0, 300) : []));
  const [index, setIndex] = useState(() => {
    const i = saved.current.index ?? -1;
    return Array.isArray(saved.current.queue) && i >= 0 && i < saved.current.queue.length ? i : -1;
  });
  const [playing, setPlaying] = useState(false);
  const [position, setPosition] = useState(0);
  const [duration, setDuration] = useState(0);
  const [volume, setVol] = useState(saved.current.volume ?? 1);
  const [muted, setMuted] = useState(false);
  const [shuffle, setShuffle] = useState(saved.current.shuffle ?? false);
  const [repeat, setRepeat] = useState<RepeatMode>(saved.current.repeat ?? "off");
  const [speed, setSpeedState] = useState(saved.current.speed ?? 1);
  const [airplayAvailable, setAirplayAvailable] = useState(false);
  const [radioStation, setRadioStation] = useState<Station | null>(null);

  const radioRef = useRef<Station | null>(null);
  const queueRef = useRef(queue);
  const indexRef = useRef(index);
  const repeatRef = useRef(repeat);
  const shuffleRef = useRef(shuffle);
  const speedRef = useRef(speed);
  const loadedId = useRef<string>(""); // track currently loaded into the element
  const recorded = useRef<string>(""); // track whose play was already logged
  const pendingNext = useRef(-1); // the index chosen to follow (so it can be preloaded)
  radioRef.current = radioStation;
  queueRef.current = queue;
  indexRef.current = index;
  repeatRef.current = repeat;
  shuffleRef.current = shuffle;
  speedRef.current = speed;

  const current = index >= 0 && index < queue.length ? queue[index] : null;

  if (!audioRef.current && typeof Audio !== "undefined") {
    audioRef.current = new Audio();
    audioRef.current.volume = volume;
    audioRef.current.playbackRate = speed;
    preloadRef.current = new Audio();
    preloadRef.current.preload = "auto";
    preloadRef.current.muted = true;
  }

  // Persist what should survive a reload.
  useEffect(() => {
    try {
      const slim = queue.slice(0, 300);
      localStorage.setItem(STORE, JSON.stringify({ queue: slim, index: index < 300 ? index : -1, volume, shuffle, repeat, speed }));
    } catch {
      /* private mode or quota: the queue just will not persist */
    }
  }, [queue, index, volume, shuffle, repeat, speed]);

  // AirPlay (Safari/iOS).
  useEffect(() => {
    const audio = audioRef.current;
    const supported = typeof (window as unknown as { WebKitPlaybackTargetAvailabilityEvent?: unknown }).WebKitPlaybackTargetAvailabilityEvent !== "undefined";
    if (!audio || !supported) return;
    const onAvail = (e: Event) => setAirplayAvailable((e as unknown as { availability?: string }).availability === "available");
    audio.addEventListener("webkitplaybacktargetavailabilitychanged", onAvail as EventListener);
    return () => audio.removeEventListener("webkitplaybacktargetavailabilitychanged", onAvail as EventListener);
  }, []);

  const showAirplay = useCallback(() => {
    (audioRef.current as (HTMLAudioElement & { webkitShowPlaybackTargetPicker?: () => void }) | null)?.webkitShowPlaybackTargetPicker?.();
  }, []);

  const loadAndPlay = useCallback((track: Track) => {
    const audio = audioRef.current;
    if (!audio) return;
    loadedId.current = track.id;
    recorded.current = "";
    audio.src = streamUrl(track.id);
    audio.playbackRate = speedRef.current;
    setPosition(0);
    audio.play().then(() => setPlaying(true)).catch(() => setPlaying(false));
  }, []);

  /** goTo selects a queue index and plays it. */
  const goTo = useCallback(
    (i: number) => {
      const t = queueRef.current[i];
      if (!t) return;
      setIndex(i);
      indexRef.current = i;
      loadAndPlay(t);
    },
    [loadAndPlay],
  );

  const playQueue = useCallback(
    (q: Track[], startIndex: number) => {
      setRadioStation(null);
      setQueue(q);
      queueRef.current = q;
      goTo(startIndex);
    },
    [goTo],
  );

  const playAt = useCallback((i: number) => goTo(i), [goTo]);
  const playTrack = useCallback((t: Track) => playQueue([t], 0), [playQueue]);

  const playRadio = useCallback((station: Station) => {
    const audio = audioRef.current;
    if (!audio) return;
    setRadioStation(station);
    setQueue([]);
    setIndex(-1);
    loadedId.current = "";
    audio.src = station.play_url;
    audio.playbackRate = 1;
    audio.play().then(() => setPlaying(true)).catch(() => setPlaying(false));
  }, []);

  const stopRadio = useCallback(() => {
    const audio = audioRef.current;
    if (audio) {
      audio.pause();
      audio.removeAttribute("src");
      audio.load();
    }
    setRadioStation(null);
    setPlaying(false);
  }, []);

  const applyQueue = useCallback(
    (nq: { queue: Track[]; index: number }, startIfIdle: boolean) => {
      const wasIdle = indexRef.current === -1;
      setQueue(nq.queue);
      queueRef.current = nq.queue;
      pendingNext.current = -1;
      if (wasIdle && startIfIdle && nq.queue.length) {
        goTo(nq.queue.length - 1); // the track just added

      } else {
        setIndex(nq.index);
        indexRef.current = nq.index;
      }
    },
    [goTo],
  );

  const playNext = useCallback((t: Track) => applyQueue(insertNext({ queue: queueRef.current, index: indexRef.current }, t), true), [applyQueue]);
  const addToQueue = useCallback((t: Track) => applyQueue(appendTrack({ queue: queueRef.current, index: indexRef.current }, t), true), [applyQueue]);
  const moveInQueue = useCallback(
    (from: number, to: number) => applyQueue(moveItem({ queue: queueRef.current, index: indexRef.current }, from, to), false),
    [applyQueue],
  );

  const stopPlayback = useCallback(() => {
    const audio = audioRef.current;
    if (audio) {
      audio.pause();
      audio.removeAttribute("src");
      audio.load();
    }
    loadedId.current = "";
    setPlaying(false);
    setPosition(0);
    setDuration(0);
  }, []);

  const removeAtIndex = useCallback(
    (at: number) => {
      const r = removeAt({ queue: queueRef.current, index: indexRef.current }, at);
      setQueue(r.queue);
      queueRef.current = r.queue;
      pendingNext.current = -1;
      setIndex(r.index);
      indexRef.current = r.index;
      if (r.removedCurrent) {
        if (r.index >= 0 && playing) loadAndPlay(r.queue[r.index]);
        else if (r.index < 0) stopPlayback();
        else loadedId.current = "";
      }
    },
    [loadAndPlay, playing, stopPlayback],
  );

  const removeFromQueue = useCallback(
    (trackId: string) => {
      const at = queueRef.current.findIndex((t) => t.id === trackId);
      if (at >= 0) removeAtIndex(at);
    },
    [removeAtIndex],
  );

  const clearQueue = useCallback(() => {
    setQueue([]);
    queueRef.current = [];
    setIndex(-1);
    indexRef.current = -1;
    stopPlayback();
  }, [stopPlayback]);

  const next = useCallback(() => {
    const s = { queue: queueRef.current, index: indexRef.current };
    // "Next" always moves on, even with repeat-one on.
    let ni = pendingNext.current >= 0 && pendingNext.current < s.queue.length && repeatRef.current !== "one" ? pendingNext.current : nextIndex(s, repeatRef.current === "one" ? "all" : repeatRef.current, shuffleRef.current);
    if (ni === s.index && s.queue.length > 1) ni = (s.index + 1) % s.queue.length;
    if (ni >= 0) goTo(ni);
  }, [goTo]);

  const prev = useCallback(() => {
    const audio = audioRef.current;
    if (audio && audio.currentTime > 3) {
      audio.currentTime = 0;
      return;
    }
    const pi = prevIndex({ queue: queueRef.current, index: indexRef.current }, repeatRef.current);
    if (pi >= 0) goTo(pi);
    else if (audio) audio.currentTime = 0;
  }, [goTo]);

  const toggle = useCallback(() => {
    const audio = audioRef.current;
    if (!audio) return;
    const cur = queueRef.current[indexRef.current];
    if (!cur && !radioRef.current) return;
    if (audio.paused) {
      if (cur && loadedId.current !== cur.id) {
        loadAndPlay(cur); // restored queue: nothing loaded yet
        return;
      }
      audio.play().then(() => setPlaying(true)).catch(() => undefined);
    } else {
      audio.pause();
      setPlaying(false);
    }
  }, [loadAndPlay]);

  const seek = useCallback((seconds: number) => {
    const audio = audioRef.current;
    if (audio && isFinite(seconds)) audio.currentTime = Math.max(0, seconds);
  }, []);
  const seekBy = useCallback((d: number) => {
    const audio = audioRef.current;
    if (audio) audio.currentTime = Math.max(0, Math.min(audio.duration || Infinity, audio.currentTime + d));
  }, []);

  const setVolume = useCallback((v: number) => {
    const audio = audioRef.current;
    const c = Math.min(1, Math.max(0, v));
    setVol(c);
    if (audio) {
      audio.volume = c;
      if (c > 0) {
        audio.muted = false;
        setMuted(false);
      }
    }
  }, []);
  const toggleMute = useCallback(() => {
    const audio = audioRef.current;
    setMuted((m) => {
      if (audio) audio.muted = !m;
      return !m;
    });
  }, []);
  const toggleShuffle = useCallback(() => {
    pendingNext.current = -1;
    setShuffle((s) => !s);
  }, []);
  const cycleRepeat = useCallback(() => setRepeat((r) => (r === "off" ? "all" : r === "all" ? "one" : "off")), []);
  const setSpeed = useCallback((s: number) => {
    const c = Math.min(2, Math.max(0.5, s));
    setSpeedState(c);
    if (audioRef.current && !radioRef.current) audioRef.current.playbackRate = c;
  }, []);

  const patchTrack = useCallback((id: string, patch: Partial<Track>) => {
    setQueue((q) => (q.some((t) => t.id === id) ? q.map((t) => (t.id === id ? { ...t, ...patch } : t)) : q));
  }, []);

  // Audio events.
  useEffect(() => {
    const audio = audioRef.current;
    if (!audio) return;
    const onTime = () => {
      setPosition(audio.currentTime);
      const cur = queueRef.current[indexRef.current];
      if (cur && recorded.current !== cur.id && !radioRef.current) {
        const dur = isFinite(audio.duration) && audio.duration > 0 ? audio.duration : cur.duration_seconds;
        const threshold = Math.min(30, dur > 0 ? dur / 2 : 30);
        if (audio.currentTime >= threshold) {
          recorded.current = cur.id;
          void recordPlay(cur.id, audio.currentTime);
          cur.play_count = (cur.play_count ?? 0) + 1;
        }
      }
    };
    const onMeta = () => setDuration(isFinite(audio.duration) ? audio.duration : 0);
    const onPlay = () => setPlaying(true);
    const onPause = () => setPlaying(false);
    const onEnded = () => {
      if (radioRef.current) {
        setPlaying(false);
        return;
      }
      if (repeatRef.current === "one") {
        audio.currentTime = 0;
        audio.play().catch(() => undefined);
        return;
      }
      const s = { queue: queueRef.current, index: indexRef.current };
      const ni = pendingNext.current >= 0 && pendingNext.current < s.queue.length ? pendingNext.current : nextIndex(s, repeatRef.current, shuffleRef.current);
      if (ni >= 0) goTo(ni);
      else setPlaying(false);
    };
    audio.addEventListener("timeupdate", onTime);
    audio.addEventListener("loadedmetadata", onMeta);
    audio.addEventListener("durationchange", onMeta);
    audio.addEventListener("play", onPlay);
    audio.addEventListener("pause", onPause);
    audio.addEventListener("ended", onEnded);
    return () => {
      audio.removeEventListener("timeupdate", onTime);
      audio.removeEventListener("loadedmetadata", onMeta);
      audio.removeEventListener("durationchange", onMeta);
      audio.removeEventListener("play", onPlay);
      audio.removeEventListener("pause", onPause);
      audio.removeEventListener("ended", onEnded);
    };
  }, [goTo]);

  // Preload the track that will follow, so the switch needs no new request.
  useEffect(() => {
    const pre = preloadRef.current;
    if (!pre || radioStation) return;
    if (index < 0 || repeat === "one") {
      pendingNext.current = -1;
      return;
    }
    const ni = nextIndex({ queue, index }, repeat, shuffle);
    pendingNext.current = ni;
    const t = ni >= 0 ? queue[ni] : undefined;
    if (t) {
      const url = streamUrl(t.id);
      if (!pre.src.endsWith(url)) {
        pre.src = url;
        pre.load();
      }
    }
  }, [queue, index, repeat, shuffle, radioStation]);

  // Lock-screen / hardware media keys.
  useEffect(() => {
    if (!("mediaSession" in navigator)) return;
    const ms = navigator.mediaSession;
    if (radioStation) {
      ms.metadata = new MediaMetadata({ title: radioStation.name, artist: "Live radio" });
    } else if (current) {
      ms.metadata = new MediaMetadata({
        title: current.title || "Untitled",
        artist: current.artist || "",
        album: current.album || "",
        artwork: current.art_url ? [{ src: current.art_url, sizes: "512x512" }] : [],
      });
    } else {
      ms.metadata = null;
    }
  }, [current, radioStation]);
  useEffect(() => {
    if (!("mediaSession" in navigator)) return;
    navigator.mediaSession.playbackState = current || radioStation ? (playing ? "playing" : "paused") : "none";
  }, [playing, current, radioStation]);
  useEffect(() => {
    if (!("mediaSession" in navigator)) return;
    const ms = navigator.mediaSession;
    const set = (a: MediaSessionAction, h: MediaSessionActionHandler | null) => {
      try {
        ms.setActionHandler(a, h);
      } catch {
        /* action not supported by this browser */
      }
    };
    set("play", () => toggle());
    set("pause", () => toggle());
    set("previoustrack", () => prev());
    set("nexttrack", () => next());
    set("seekbackward", (d) => seekBy(-(d.seekOffset ?? 10)));
    set("seekforward", (d) => seekBy(d.seekOffset ?? 10));
    set("seekto", (d) => {
      if (d.seekTime != null) seek(d.seekTime);
    });
    return () => (["play", "pause", "previoustrack", "nexttrack", "seekbackward", "seekforward", "seekto"] as MediaSessionAction[]).forEach((a) => set(a, null));
  }, [toggle, prev, next, seek, seekBy]);
  useEffect(() => {
    if (!("mediaSession" in navigator) || !duration || !isFinite(duration)) return;
    try {
      navigator.mediaSession.setPositionState({ duration, position: Math.min(position, duration), playbackRate: speed });
    } catch {
      /* position state is optional */
    }
  }, [position, duration, speed]);

  useEffect(
    () => () => {
      const a = audioRef.current;
      if (a) {
        a.pause();
        a.removeAttribute("src");
      }
      preloadRef.current?.removeAttribute("src");
    },
    [],
  );

  const value: PlayerState = {
    queue,
    index,
    current,
    playing,
    position,
    duration,
    volume,
    muted,
    shuffle,
    repeat,
    speed,
    radioStation,
    playQueue,
    playAt,
    playRadio,
    stopRadio,
    playTrack,
    playNext,
    addToQueue,
    moveInQueue,
    removeAtIndex,
    clearQueue,
    toggle,
    next,
    prev,
    seek,
    seekBy,
    setVolume,
    toggleMute,
    toggleShuffle,
    cycleRepeat,
    setSpeed,
    removeFromQueue,
    patchTrack,
    airplayAvailable,
    showAirplay,
  };
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}
