// Pure logic for the Meet call UI (layouts, speaking detection, adaptive video).

export interface Rect {
  x: number;
  y: number;
  w: number;
  h: number;
}

export type LayoutMode = "auto" | "tiled" | "spotlight" | "sidebar";

const round2 = (n: number) => Math.round(n * 100) / 100;
const clampSize = (n: number) => (n > 0 ? n : 0);

interface GridCandidate {
  cols: number;
  rows: number;
  tileW: number;
  tileH: number;
  area: number;
  gap: number;
}

function bestGrid(
  count: number,
  width: number,
  height: number,
  gap: number,
  aspect: number,
): GridCandidate | null {
  let best: GridCandidate | null = null;
  for (let cols = 1; cols <= count; cols++) {
    const rows = Math.ceil(count / cols);
    const cellW = (width - gap * (cols - 1)) / cols;
    const cellH = (height - gap * (rows - 1)) / rows;
    if (cellW <= 0 || cellH <= 0) continue;
    let tileW = Math.min(cellW, cellH * aspect);
    let tileH = tileW / aspect;
    if (tileH > cellH) {
      tileH = cellH;
      tileW = tileH * aspect;
    }
    const area = tileW * tileH;
    if (!best || area > best.area) {
      best = { cols, rows, tileW, tileH, area, gap };
    }
  }
  return best;
}

/** Uniform grid maximising tile area; a partial last row is centred. */
export function computeGridLayout(
  count: number,
  width: number,
  height: number,
  opts: { gap?: number; aspect?: number } = {},
): { cols: number; rows: number; tileW: number; tileH: number; rects: Rect[] } {
  const { gap = 12, aspect = 16 / 9 } = opts;
  if (count <= 0 || width <= 0 || height <= 0) {
    return { cols: 0, rows: 0, tileW: 0, tileH: 0, rects: [] };
  }
  const best =
    bestGrid(count, width, height, gap, aspect) ??
    bestGrid(count, width, height, 0, aspect)!;
  const { cols, rows, tileW, tileH } = best;
  const g = best.gap;
  const gridH = rows * tileH + (rows - 1) * g;
  const offsetY = Math.max(0, (height - gridH) / 2);
  const rects: Rect[] = [];
  for (let i = 0; i < count; i++) {
    const row = Math.floor(i / cols);
    const inRow = row === rows - 1 ? count - row * cols : cols;
    const rowW = inRow * tileW + (inRow - 1) * g;
    const offsetX = Math.max(0, (width - rowW) / 2);
    const col = i - row * cols;
    rects.push({
      x: round2(offsetX + col * (tileW + g)),
      y: round2(offsetY + row * (tileH + g)),
      w: round2(tileW),
      h: round2(tileH),
    });
  }
  return { cols, rows, tileW: round2(tileW), tileH: round2(tileH), rects };
}

/** One featured tile plus a filmstrip of the rest (right in landscape, below in portrait). */
export function computeSpotlightLayout(
  stripCount: number,
  width: number,
  height: number,
  opts: { gap?: number; stripSize?: number; aspect?: number } = {},
): { featured: Rect; strip: Rect[] } {
  const { gap = 12, stripSize = 200, aspect = 16 / 9 } = opts;
  const landscape = width >= height;
  if (stripCount <= 0) {
    return {
      featured: { x: 0, y: 0, w: round2(width), h: round2(height) },
      strip: [],
    };
  }
  if (landscape) {
    const stripW = Math.min(stripSize, width * 0.25);
    const featured = {
      x: 0,
      y: 0,
      w: round2(Math.max(0, width - stripW - gap)),
      h: round2(height),
    };
    const tileH = clampSize(
      Math.min(stripW / aspect, (height - gap * (stripCount - 1)) / stripCount),
    );
    const totalH = stripCount * tileH + (stripCount - 1) * gap;
    const startY = Math.max(0, (height - totalH) / 2);
    const strip: Rect[] = [];
    for (let i = 0; i < stripCount; i++) {
      strip.push({
        x: round2(width - stripW),
        y: round2(startY + i * (tileH + gap)),
        w: round2(stripW),
        h: round2(tileH),
      });
    }
    return { featured, strip };
  }
  const stripH = Math.min(stripSize * 0.75, height * 0.25);
  const featured = {
    x: 0,
    y: 0,
    w: round2(width),
    h: round2(Math.max(0, height - stripH - gap)),
  };
  const tileW = clampSize(
    Math.min(stripH * aspect, (width - gap * (stripCount - 1)) / stripCount),
  );
  const totalW = stripCount * tileW + (stripCount - 1) * gap;
  const startX = Math.max(0, (width - totalW) / 2);
  const strip: Rect[] = [];
  for (let i = 0; i < stripCount; i++) {
    strip.push({
      x: round2(startX + i * (tileW + gap)),
      y: round2(height - stripH),
      w: round2(tileW),
      h: round2(stripH),
    });
  }
  return { featured, strip };
}

export interface TileLike {
  id: string;
  sharing?: boolean;
}

/**
 * Decide the featured tile id (or null for a plain grid).
 * pinned wins; then a screen share; then (spotlight/sidebar modes) the active
 * speaker; auto mode switches to spotlight only when someone shares or is pinned.
 */
export function pickFeatured(
  mode: LayoutMode,
  tiles: TileLike[],
  pinnedId: string | null,
  activeSpeakerId: string | null,
): string | null {
  if (tiles.length < 2 && mode !== "spotlight" && mode !== "sidebar") {
    return null;
  }
  if (pinnedId && tiles.some((t) => t.id === pinnedId)) return pinnedId;
  const sharer = tiles.find((t) => t.sharing);
  if (sharer) return sharer.id;
  if (mode === "tiled" || mode === "auto") return null;
  if (activeSpeakerId && tiles.some((t) => t.id === activeSpeakerId)) {
    return activeSpeakerId;
  }
  return tiles[0]?.id ?? null;
}

/** Speaking detection with hysteresis. */
export class SpeakingDetector {
  private speakingNow = false;
  private belowSince: number | null = null;
  constructor(
    private onThreshold = 0.02,
    private offThreshold = 0.01,
    private holdMs = 900,
  ) {}
  get speaking() {
    return this.speakingNow;
  }
  update(level: number, now = Date.now()): boolean {
    if (level >= this.onThreshold) {
      this.speakingNow = true;
      this.belowSince = null;
    } else if (this.speakingNow && level < this.offThreshold) {
      if (this.belowSince === null) this.belowSince = now;
      if (now - this.belowSince >= this.holdMs) {
        this.speakingNow = false;
        this.belowSince = null;
      }
    } else {
      this.belowSince = null;
    }
    return this.speakingNow;
  }
}

/** Loudest currently-speaking participant, ties broken by the previous speaker. */
export function pickActiveSpeaker(
  levels: Record<string, number>,
  speaking: Record<string, boolean>,
  previous: string | null,
): string | null {
  let best: string | null = null;
  let bestLevel = -1;
  for (const id of Object.keys(speaking)) {
    if (!speaking[id]) continue;
    const lvl = levels[id] ?? 0;
    if (lvl > bestLevel || (lvl === bestLevel && id === previous)) {
      best = id;
      bestLevel = lvl;
    }
  }
  return best ?? (previous && speaking[previous] !== undefined ? previous : null);
}

/**
 * Bandwidth-adaptive video: the more tiles on screen, the lower the capture
 * resolution and the outbound bitrate cap.
 */
export interface VideoQuality {
  width: number;
  height: number;
  frameRate: number;
  maxBitrate: number; // bits per second, applied to RTCRtpSender
}

export function videoQualityFor(participants: number): VideoQuality {
  if (participants <= 2) {
    return { width: 1280, height: 720, frameRate: 30, maxBitrate: 1_500_000 };
  }
  if (participants <= 4) {
    return { width: 960, height: 540, frameRate: 24, maxBitrate: 800_000 };
  }
  if (participants <= 9) {
    return { width: 640, height: 360, frameRate: 20, maxBitrate: 400_000 };
  }
  return { width: 320, height: 180, frameRate: 15, maxBitrate: 150_000 };
}

export type ShortcutAction = "mic" | "cam" | null;

/** Ctrl/Cmd+D toggles the mic, Ctrl/Cmd+E the camera (Google Meet bindings). */
export function shortcutAction(e: {
  key: string;
  ctrlKey: boolean;
  metaKey: boolean;
  altKey: boolean;
  shiftKey: boolean;
}): ShortcutAction {
  if (!(e.ctrlKey || e.metaKey) || e.altKey || e.shiftKey) return null;
  const k = e.key.toLowerCase();
  if (k === "d") return "mic";
  if (k === "e") return "cam";
  return null;
}

export const REACTION_EMOJIS = ["👍", "👏", "❤️", "😂", "😮", "🎉"];
