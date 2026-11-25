/** Client-side media helpers: extract a poster thumbnail and duration from a
 *  selected video file before upload, so the library grid has thumbnails
 *  without a server-side transcoding pipeline. */

export interface Probe {
  durationSeconds: number;
  thumbnailDataUrl: string;
}

/** probeVideo loads the file into an off-screen <video>, seeks ~1s in, and
 *  draws a frame to a canvas for a JPEG poster. Falls back gracefully if the
 *  browser can't decode the file (returns zeros / empty thumbnail). */
export function probeVideo(file: File): Promise<Probe> {
  return new Promise((resolve) => {
    const url = URL.createObjectURL(file);
    const video = document.createElement("video");
    video.preload = "metadata";
    video.muted = true;
    video.src = url;

    let settled = false;
    const done = (p: Probe) => {
      if (settled) return;
      settled = true;
      URL.revokeObjectURL(url);
      resolve(p);
    };

    // Safety timeout — never block the upload on a flaky decode.
    const timer = window.setTimeout(
      () => done({ durationSeconds: 0, thumbnailDataUrl: "" }),
      8000,
    );

    video.onloadedmetadata = () => {
      const duration = isFinite(video.duration) ? video.duration : 0;
      // Seek a touch in to avoid black leader frames.
      const seekTo = Math.min(1, duration / 2 || 0);
      const capture = () => {
        try {
          const canvas = document.createElement("canvas");
          const w = video.videoWidth || 320;
          const h = video.videoHeight || 180;
          const scale = Math.min(1, 640 / w);
          canvas.width = Math.round(w * scale);
          canvas.height = Math.round(h * scale);
          const ctx = canvas.getContext("2d");
          let thumb = "";
          if (ctx) {
            ctx.drawImage(video, 0, 0, canvas.width, canvas.height);
            thumb = canvas.toDataURL("image/jpeg", 0.7);
            // Guard against oversized data URLs (column is TEXT but keep it sane).
            if (thumb.length > 400_000) thumb = "";
          }
          window.clearTimeout(timer);
          done({ durationSeconds: duration, thumbnailDataUrl: thumb });
        } catch {
          window.clearTimeout(timer);
          done({ durationSeconds: duration, thumbnailDataUrl: "" });
        }
      };
      video.onseeked = capture;
      try {
        video.currentTime = seekTo;
      } catch {
        capture();
      }
    };

    video.onerror = () => {
      window.clearTimeout(timer);
      done({ durationSeconds: 0, thumbnailDataUrl: "" });
    };
  });
}

/** formatDuration renders seconds as H:MM:SS or M:SS. */
export function formatDuration(seconds: number): string {
  if (!seconds || seconds < 0 || !isFinite(seconds)) return "";
  const s = Math.floor(seconds % 60);
  const m = Math.floor((seconds / 60) % 60);
  const h = Math.floor(seconds / 3600);
  const pad = (n: number) => String(n).padStart(2, "0");
  return h > 0 ? `${h}:${pad(m)}:${pad(s)}` : `${m}:${pad(s)}`;
}

/** formatBytes renders a byte count as a human-readable size. */
export function formatBytes(bytes: number): string {
  if (!bytes || bytes < 0) return "";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let i = 0;
  let n = bytes;
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024;
    i++;
  }
  return `${n.toFixed(n >= 10 || i === 0 ? 0 : 1)} ${units[i]}`;
}

/** extractFrames draws JPEG frames of a local video file at the given times
 *  (seconds), for the thumbnail choice at upload. Frames that fail are skipped. */
export function extractFrames(file: File, times: number[], width = 480): Promise<{ time: number; dataUrl: string }[]> {
  return new Promise((resolve) => {
    const url = URL.createObjectURL(file);
    const video = document.createElement("video");
    video.muted = true;
    video.preload = "auto";
    video.src = url;
    const out: { time: number; dataUrl: string }[] = [];
    const finish = () => {
      URL.revokeObjectURL(url);
      resolve(out);
    };
    const guard = window.setTimeout(finish, 15000);
    let i = 0;
    const next = () => {
      if (i >= times.length) {
        window.clearTimeout(guard);
        finish();
        return;
      }
      try {
        video.currentTime = times[i];
      } catch {
        i++;
        next();
      }
    };
    video.onloadeddata = next;
    video.onerror = () => {
      window.clearTimeout(guard);
      finish();
    };
    video.onseeked = () => {
      try {
        const scale = Math.min(1, width / (video.videoWidth || width));
        const c = document.createElement("canvas");
        c.width = Math.round((video.videoWidth || width) * scale);
        c.height = Math.round((video.videoHeight || (width * 9) / 16) * scale);
        const ctx = c.getContext("2d");
        if (ctx) {
          ctx.drawImage(video, 0, 0, c.width, c.height);
          out.push({ time: times[i], dataUrl: c.toDataURL("image/jpeg", 0.72) });
        }
      } catch {
        /* skip this frame */
      }
      i++;
      next();
    };
  });
}

/** imageToThumbnail scales a chosen image file to a 640 px wide JPEG data URL. */
export function imageToThumbnail(file: File, width = 640): Promise<string> {
  return new Promise((resolve, reject) => {
    const url = URL.createObjectURL(file);
    const img = new Image();
    img.onload = () => {
      const scale = Math.min(1, width / img.width);
      const c = document.createElement("canvas");
      c.width = Math.round(img.width * scale);
      c.height = Math.round(img.height * scale);
      c.getContext("2d")?.drawImage(img, 0, 0, c.width, c.height);
      URL.revokeObjectURL(url);
      resolve(c.toDataURL("image/jpeg", 0.8));
    };
    img.onerror = () => {
      URL.revokeObjectURL(url);
      reject(new Error("That file is not an image"));
    };
    img.src = url;
  });
}
