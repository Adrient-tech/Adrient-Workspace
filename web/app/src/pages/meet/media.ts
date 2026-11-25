import { useEffect, useRef, useState } from "react";
import { SpeakingDetector, pickActiveSpeaker, videoQualityFor } from "./lib";
import type { VideoQuality } from "./lib";
import { playSound } from "../../lib/sounds";

// ---------------------------------------------------------------------------
// Sounds
// ---------------------------------------------------------------------------
/** In-call sounds come from the shared sound system (tone sets, volume, Do Not Disturb). */
export const sounds = {
  join: () => playSound("join"),
  leave: () => playSound("leave"),
  message: () => playSound("message"),
  hand: () => playSound("hand"),
};

// ---------------------------------------------------------------------------
// Devices
// ---------------------------------------------------------------------------
export interface DeviceLists {
  mics: MediaDeviceInfo[];
  cams: MediaDeviceInfo[];
  speakers: MediaDeviceInfo[];
}

export async function listDevices(): Promise<DeviceLists> {
  if (!navigator.mediaDevices?.enumerateDevices) {
    return { mics: [], cams: [], speakers: [] };
  }
  const all = await navigator.mediaDevices.enumerateDevices();
  return {
    mics: all.filter((d) => d.kind === "audioinput"),
    cams: all.filter((d) => d.kind === "videoinput"),
    speakers: all.filter((d) => d.kind === "audiooutput"),
  };
}

export type BgSpec =
  | { type: "none" }
  | { type: "blur"; blur: number }
  | { type: "image"; src: string; id: string };

export const BG_PRESETS = [
  { id: "office", label: "Office", src: "/meet-backgrounds/office.webp" },
  { id: "lake", label: "Lake", src: "/meet-backgrounds/lake.webp" },
  { id: "forest", label: "Forest", src: "/meet-backgrounds/forest.webp" },
  { id: "library", label: "Library", src: "/meet-backgrounds/library.webp" },
];

type Publish = (kind: "audio" | "video", track: MediaStreamTrack | null) => void;

/**
 * LocalMedia owns the user's mic + camera for green room and call.
 *
 * Mic mute uses track.enabled (instant). Camera off stops the track (LED off).
 * Noise suppression uses the browser's built-in noiseSuppression constraint
 * (no ML suppressor is bundled, to keep the app light).
 * Blur / virtual background lazy-loads MediaPipe only when first requested.
 */
export class LocalMedia {
  micTrack: MediaStreamTrack | null = null;
  rawCam: MediaStreamTrack | null = null;
  camTrack: MediaStreamTrack | null = null; // published (raw or processed)
  micOn = true;
  camOn = true;
  micId = "";
  camId = "";
  bg: BgSpec = { type: "none" };
  bgBusy = false;
  noiseSuppression = true;
  error: string | null = null;
  quality: VideoQuality = videoQualityFor(2);
  onPublish: Publish = () => {};

  private effect: {
    track: MediaStreamTrack;
    stop(): void;
    setSpec(s: unknown): Promise<void>;
  } | null = null;
  private listeners = new Set<() => void>();
  private stopped = false;

  subscribe(fn: () => void) {
    this.listeners.add(fn);
    return () => {
      this.listeners.delete(fn);
    };
  }
  private emit() {
    this.listeners.forEach((l) => l());
  }

  private setError(e: unknown, what: string) {
    const name = e instanceof DOMException ? e.name : "";
    if (name === "NotAllowedError" || name === "SecurityError") {
      this.error = `${what} access was blocked. Allow it in the browser to be seen or heard.`;
    } else if (name === "NotFoundError" || name === "OverconstrainedError") {
      this.error = `No ${what.toLowerCase()} found.`;
    } else {
      this.error = `${what} error: ${e instanceof Error ? e.message : String(e)}`;
    }
  }

  async start(micOn: boolean, camOn: boolean) {
    this.micOn = micOn;
    this.camOn = camOn;
    if (!window.isSecureContext || !navigator.mediaDevices?.getUserMedia) {
      this.error =
        "Camera and microphone need a secure context (HTTPS or localhost). You can still join to watch and listen.";
      this.emit();
      return;
    }
    await Promise.all([this.acquireMic(), camOn ? this.acquireCam() : null]);
    this.emit();
  }

  async acquireMic() {
    try {
      const s = await navigator.mediaDevices.getUserMedia({
        audio: {
          deviceId: this.micId ? { exact: this.micId } : undefined,
          echoCancellation: true,
          noiseSuppression: this.noiseSuppression,
          autoGainControl: true,
        },
      });
      const t = s.getAudioTracks()[0] ?? null;
      if (this.stopped) {
        t?.stop();
        return;
      }
      this.micTrack?.stop();
      this.micTrack = t;
      if (t) {
        t.enabled = this.micOn;
        this.micId = t.getSettings().deviceId ?? this.micId;
      }
      this.onPublish("audio", t);
    } catch (e) {
      this.setError(e, "Microphone");
    }
  }

  async acquireCam() {
    try {
      const q = this.quality;
      const s = await navigator.mediaDevices.getUserMedia({
        video: {
          deviceId: this.camId ? { exact: this.camId } : undefined,
          width: { ideal: q.width },
          height: { ideal: q.height },
          frameRate: { ideal: q.frameRate },
        },
      });
      const t = s.getVideoTracks()[0] ?? null;
      if (this.stopped || !t) {
        t?.stop();
        return;
      }
      this.stopEffect();
      this.rawCam?.stop();
      this.rawCam = t;
      this.camId = t.getSettings().deviceId ?? this.camId;
      await this.publishCam();
    } catch (e) {
      this.setError(e, "Camera");
      this.camOn = false;
    }
  }

  /** (Re)build the published camera track from rawCam + current bg spec. */
  private async publishCam() {
    if (!this.rawCam) return;
    let pub: MediaStreamTrack = this.rawCam;
    if (this.bg.type !== "none") {
      try {
        this.bgBusy = true;
        this.emit();
        const mod = await import("./background-effect");
        const spec =
          this.bg.type === "blur"
            ? { type: "blur", blur: this.bg.blur }
            : { type: "image", src: this.bg.src };
        if (this.effect) {
          await this.effect.setSpec(spec);
          pub = this.effect.track;
        } else {
          const eff = await mod.BackgroundEffect.start(this.rawCam, spec);
          if (this.stopped) {
            eff.stop();
            return;
          }
          this.effect = eff;
          pub = eff.track;
        }
      } catch (e) {
        console.warn("[meet] background effect failed", e);
        this.error = "Background effects are not available in this browser.";
        this.bg = { type: "none" };
        this.stopEffect();
        pub = this.rawCam;
      } finally {
        this.bgBusy = false;
      }
    } else {
      this.stopEffect();
    }
    this.camTrack = pub;
    this.onPublish("video", pub);
    this.emit();
  }

  private stopEffect() {
    this.effect?.stop();
    this.effect = null;
  }

  setMic(on: boolean) {
    this.micOn = on;
    if (this.micTrack) this.micTrack.enabled = on;
    this.emit();
  }

  async setCam(on: boolean) {
    this.camOn = on;
    if (on) {
      await this.acquireCam();
    } else {
      this.stopEffect();
      this.rawCam?.stop();
      this.rawCam = null;
      this.camTrack = null;
      this.onPublish("video", null);
    }
    this.emit();
  }

  async switchMic(id: string) {
    this.micId = id;
    await this.acquireMic();
    this.emit();
  }

  async switchCam(id: string) {
    this.camId = id;
    if (this.camOn) await this.acquireCam();
    this.emit();
  }

  async setBackground(bg: BgSpec) {
    this.bg = bg;
    if (this.camOn && this.rawCam) await this.publishCam();
    else this.emit();
  }

  async setNoiseSuppression(on: boolean) {
    this.noiseSuppression = on;
    if (this.micTrack) {
      try {
        await this.micTrack.applyConstraints({ noiseSuppression: on });
      } catch {
        await this.acquireMic();
      }
    }
    this.emit();
  }

  /** Lower/raise capture resolution as the call grows (skipped while an effect runs). */
  async setQuality(q: VideoQuality) {
    const prev = this.quality;
    this.quality = q;
    if (this.rawCam && !this.effect && prev.height !== q.height) {
      try {
        await this.rawCam.applyConstraints({
          width: { ideal: q.width },
          height: { ideal: q.height },
          frameRate: { ideal: q.frameRate },
        });
      } catch {
        /* keep current */
      }
    }
  }

  private stopTimer: number | null = null;

  /** Deferred stop so a React dev double-mount does not kill the devices. */
  scheduleStop() {
    this.stopTimer = window.setTimeout(() => this.stop(), 300);
  }
  cancelStop() {
    if (this.stopTimer !== null) window.clearTimeout(this.stopTimer);
    this.stopTimer = null;
  }

  stop() {
    this.stopped = true;
    this.stopEffect();
    this.micTrack?.stop();
    this.rawCam?.stop();
    this.micTrack = null;
    this.rawCam = null;
    this.camTrack = null;
    this.listeners.clear();
  }
}

/** Re-render when a LocalMedia changes. */
export function useLocalMedia(lm: LocalMedia): number {
  const [v, setV] = useState(0);
  useEffect(() => lm.subscribe(() => setV((n) => n + 1)), [lm]);
  return v;
}

// ---------------------------------------------------------------------------
// Audio levels / active speaker
// ---------------------------------------------------------------------------
export interface SpeakerState {
  speaking: Record<string, boolean>;
  active: string | null;
}

/** Analyse every stream's audio; reports who is speaking and the active speaker. */
export function useSpeakers(streams: Record<string, MediaStream | null>): SpeakerState {
  const [state, setState] = useState<SpeakerState>({ speaking: {}, active: null });
  const sig = Object.entries(streams)
    .map(
      ([id, s]) =>
        `${id}:${s?.getAudioTracks().map((t) => t.id).join(",") ?? ""}`,
    )
    .sort()
    .join("|");
  const streamsRef = useRef(streams);
  streamsRef.current = streams;

  useEffect(() => {
    const Ctx =
      window.AudioContext ??
      (window as unknown as { webkitAudioContext?: typeof AudioContext })
        .webkitAudioContext;
    if (!Ctx) return;
    const ac = new Ctx();
    const nodes = new Map<
      string,
      { an: AnalyserNode; src: MediaStreamAudioSourceNode; det: SpeakingDetector; buf: Uint8Array }
    >();
    for (const [id, s] of Object.entries(streamsRef.current)) {
      if (!s || s.getAudioTracks().length === 0) continue;
      try {
        const src = ac.createMediaStreamSource(new MediaStream(s.getAudioTracks()));
        const an = ac.createAnalyser();
        an.fftSize = 512;
        src.connect(an);
        nodes.set(id, {
          an,
          src,
          det: new SpeakingDetector(),
          buf: new Uint8Array(an.fftSize),
        });
      } catch {
        /* ignore stream */
      }
    }
    let prevActive: string | null = null;
    let last = "";
    const timer = window.setInterval(() => {
      const levels: Record<string, number> = {};
      const speaking: Record<string, boolean> = {};
      const now = Date.now();
      nodes.forEach((n, id) => {
        n.an.getByteTimeDomainData(n.buf as Uint8Array<ArrayBuffer>);
        let sum = 0;
        for (let i = 0; i < n.buf.length; i++) {
          const v = (n.buf[i] - 128) / 128;
          sum += v * v;
        }
        const rms = Math.sqrt(sum / n.buf.length);
        levels[id] = rms;
        speaking[id] = n.det.update(rms, now);
      });
      const active = pickActiveSpeaker(levels, speaking, prevActive);
      prevActive = active;
      const key = JSON.stringify([Object.keys(speaking).filter((k) => speaking[k]).sort(), active]);
      if (key !== last) {
        last = key;
        setState({ speaking, active });
      }
    }, 120);
    return () => {
      window.clearInterval(timer);
      nodes.forEach((n) => n.src.disconnect());
      ac.close().catch(() => {});
    };
  }, [sig]);

  return state;
}
