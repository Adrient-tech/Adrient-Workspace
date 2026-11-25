#!/usr/bin/env node
// Synthesises Adrient's built-in sound sets. Every tone is original: short
// note sequences played by a small additive synthesiser (harmonics, ADSR
// envelope, light chorus) through a Schroeder reverb, normalised to a safe
// peak and encoded to mp3 with ffmpeg.
//
//   node scripts/gen-sounds.mjs            # writes web/app/public/sounds/<set>/<id>.mp3
//   node scripts/gen-sounds.mjs --wav      # keep wav instead of mp3 (no ffmpeg needed)
//
// Output is deterministic. To change a tone, edit the melody table or a set's
// timbre below and re-run; or just drop your own file with the same name into
// public/sounds/<set>/ (see docs/services/sounds.md).
import { spawnSync } from "node:child_process";
import { mkdirSync, writeFileSync, rmSync, mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const SR = 44100;
const OUT = join(dirname(fileURLToPath(import.meta.url)), "..", "web", "app", "public", "sounds");
const WAV_ONLY = process.argv.includes("--wav");

// ---- melodies: [midi note, start (s), length (s)] ---------------------------
// Written for the "classic" set; the other sets transpose and re-time them.
const SOUNDS = {
  // meetings
  join: { level: 0.8, notes: [[76, 0, 0.18], [83, 0.1, 0.36]] },
  leave: { level: 0.7, notes: [[83, 0, 0.16], [76, 0.1, 0.36]] },
  hand: { level: 0.75, notes: [[86, 0, 0.14], [91, 0.13, 0.34]] },
  mute: { level: 0.55, notes: [[69, 0, 0.1]] },
  unmute: { level: 0.55, notes: [[76, 0, 0.1]] },
  share: { level: 0.8, notes: [[72, 0, 0.14], [79, 0.09, 0.14], [84, 0.18, 0.14], [91, 0.27, 0.4]] },
  rec_start: { level: 0.8, notes: [[84, 0, 0.14], [84, 0.2, 0.14], [91, 0.4, 0.5]] },
  rec_stop: { level: 0.75, notes: [[91, 0, 0.14], [84, 0.17, 0.14], [79, 0.34, 0.5]] },
  meeting_ended: { level: 0.85, notes: [[79, 0, 0.25], [74, 0.26, 0.25], [67, 0.52, 0.25], [60, 0.78, 0.9]] },
  // notifications
  message: { level: 0.8, notes: [[79, 0, 0.16], [84, 0.1, 0.34]] },
  dm: { level: 0.85, notes: [[76, 0, 0.14], [79, 0.1, 0.14], [83, 0.2, 0.42]] },
  mention: { level: 0.9, notes: [[84, 0, 0.12], [88, 0.1, 0.12], [91, 0.2, 0.18], [96, 0.32, 0.45]] },
  mail: { level: 0.8, notes: [[74, 0, 0.18], [81, 0.15, 0.46]] },
  calendar: { level: 0.85, notes: [[79, 0, 0.22], [76, 0.22, 0.22], [72, 0.44, 0.22], [79, 0.74, 0.6]] },
  task: { level: 0.8, notes: [[72, 0, 0.14], [76, 0.12, 0.14], [79, 0.24, 0.14], [84, 0.36, 0.5]] },
  approval: { level: 0.85, notes: [[79, 0, 0.16], [79, 0.26, 0.16], [86, 0.52, 0.55]] },
  alert: { level: 0.9, notes: [[84, 0, 0.2], [77, 0.22, 0.2], [84, 0.44, 0.2], [77, 0.66, 0.4]] },
  success: { level: 0.8, notes: [[76, 0, 0.14], [83, 0.1, 0.5]] },
  error: { level: 0.8, notes: [[67, 0, 0.18], [62, 0.17, 0.5]] },
  // looping tones (silence at the end is part of the loop)
  ringtone: {
    level: 0.85,
    length: 6.0,
    notes: [
      [84, 0, 0.3], [88, 0.3, 0.3], [91, 0.6, 0.3], [88, 0.9, 0.3], [81, 1.2, 0.3], [84, 1.5, 0.3], [88, 1.8, 0.5],
      [79, 2.6, 0.3], [83, 2.9, 0.3], [86, 3.2, 0.3], [83, 3.5, 0.3], [76, 3.8, 0.3], [79, 4.1, 0.3], [84, 4.4, 0.7],
    ],
  },
  ringback: {
    level: 0.6,
    length: 4.0,
    notes: [[69, 0, 0.55], [76, 0, 0.55], [69, 0.75, 0.55], [76, 0.75, 0.55]],
  },
};

// ---- sets: timbre, transposition, tempo ------------------------------------
const SETS = {
  classic: { shift: 0, tempo: 1, wet: 0.2, voice: { h: [[1, 1], [2, 0.42], [3, 0.2], [4.1, 0.1]], atk: 0.004, decay: 5.5, detune: 0 } },
  soft: { shift: -5, tempo: 1.22, wet: 0.3, voice: { h: [[1, 1], [2, 0.12], [3, 0.04]], atk: 0.02, decay: 3.4, detune: 0.0015 } },
  bright: { shift: 5, tempo: 0.86, wet: 0.16, voice: { h: [[1, 1], [2, 0.6], [3, 0.4], [4, 0.28], [5, 0.18], [6, 0.1]], atk: 0.002, decay: 8, detune: 0.003 } },
};

const mtof = (m) => 440 * 2 ** ((m - 69) / 12);

function renderNotes(def, set) {
  const tail = def.length ? 0 : 0.9 * set.tempo;
  const total = def.length ?? Math.max(...def.notes.map((n) => (n[1] + n[2]) * set.tempo)) + tail;
  const buf = new Float32Array(Math.ceil(total * SR));
  for (const [midi, st, len] of def.notes) {
    const f0 = mtof(midi + set.shift);
    const start = Math.floor(st * set.tempo * SR);
    const dur = len * set.tempo + 0.5; // ring on past the written length
    const n = Math.min(Math.floor(dur * SR), buf.length - start);
    const v = set.voice;
    for (let c = 0; c < (v.detune ? 2 : 1); c++) {
      const f = f0 * (c === 0 ? 1 : 1 + v.detune);
      const g = v.detune ? 0.55 : 1;
      for (let i = 0; i < n; i++) {
        const t = i / SR;
        const env = Math.min(1, t / v.atk) * Math.exp((-t * v.decay) / Math.max(0.4, len * set.tempo * 1.2 + 0.3));
        const rel = t > len * set.tempo ? Math.exp(-(t - len * set.tempo) * 9) : 1; // note-off
        let s = 0;
        for (const [m, a] of v.h) s += a * Math.sin(2 * Math.PI * f * m * t) * (1 / (1 + (m - 1) * 0.15));
        buf[start + i] += s * env * rel * g * 0.25;
      }
    }
  }
  return buf;
}

// Schroeder reverb: 4 parallel combs into 2 series all-passes.
function reverb(x, wet) {
  const combs = [1557, 1617, 1491, 1422];
  const aps = [225, 556];
  const out = new Float32Array(x.length);
  const fb = 0.78;
  const damp = 0.35;
  const acc = new Float32Array(x.length);
  for (const d of combs) {
    const line = new Float32Array(d);
    let idx = 0;
    let lp = 0;
    for (let i = 0; i < x.length; i++) {
      const y = line[idx];
      lp = y * (1 - damp) + lp * damp;
      line[idx] = x[i] + lp * fb;
      idx = (idx + 1) % d;
      acc[i] += y * 0.25;
    }
  }
  let sig = acc;
  for (const d of aps) {
    const line = new Float32Array(d);
    let idx = 0;
    const o = new Float32Array(sig.length);
    for (let i = 0; i < sig.length; i++) {
      const y = line[idx];
      line[idx] = sig[i] + y * 0.5;
      o[i] = y - sig[i] * 0.5;
      idx = (idx + 1) % d;
    }
    sig = o;
  }
  for (let i = 0; i < x.length; i++) out[i] = x[i] * (1 - wet * 0.5) + sig[i] * wet * 2.2;
  return out;
}

function finish(x, level, loop) {
  let peak = 0;
  for (const s of x) peak = Math.max(peak, Math.abs(s));
  const gain = (0.89 * level) / (peak || 1); // peak <= -1 dBFS, never clips
  const fade = Math.floor(0.03 * SR);
  for (let i = 0; i < x.length; i++) {
    x[i] *= gain;
    if (!loop && i < 40) x[i] *= i / 40; // no click at the start
    if (i > x.length - fade) x[i] *= Math.max(0, (x.length - i) / fade);
  }
  if (loop) return x; // a loop's trailing silence is its gap
  let end = x.length;
  while (end > SR / 10 && Math.abs(x[end - 1]) < 0.0008) end--;
  return x.subarray(0, end);
}

function wav(x) {
  const b = Buffer.alloc(44 + x.length * 2);
  b.write("RIFF", 0);
  b.writeUInt32LE(36 + x.length * 2, 4);
  b.write("WAVEfmt ", 8);
  b.writeUInt32LE(16, 16);
  b.writeUInt16LE(1, 20);
  b.writeUInt16LE(1, 22);
  b.writeUInt32LE(SR, 24);
  b.writeUInt32LE(SR * 2, 28);
  b.writeUInt16LE(2, 32);
  b.writeUInt16LE(16, 34);
  b.write("data", 36);
  b.writeUInt32LE(x.length * 2, 40);
  for (let i = 0; i < x.length; i++) b.writeInt16LE(Math.round(Math.max(-1, Math.min(1, x[i])) * 32767), 44 + i * 2);
  return b;
}

const tmp = mkdtempSync(join(tmpdir(), "adrient-sounds-"));
let count = 0;
for (const [setName, set] of Object.entries(SETS)) {
  const dir = join(OUT, setName);
  mkdirSync(dir, { recursive: true });
  for (const [id, def] of Object.entries(SOUNDS)) {
    const pcm = finish(reverb(renderNotes(def, set), set.wet), def.level, !!def.length);
    if (WAV_ONLY) {
      writeFileSync(join(dir, `${id}.wav`), wav(pcm));
    } else {
      const w = join(tmp, `${setName}-${id}.wav`);
      writeFileSync(w, wav(pcm));
      const r = spawnSync("ffmpeg", ["-y", "-loglevel", "error", "-i", w, "-codec:a", "libmp3lame", "-b:a", "64k", "-ac", "1",
        "-map_metadata", "-1", "-id3v2_version", "0", "-write_xing", "0", join(dir, `${id}.mp3`)]);
      if (r.status !== 0) {
        console.error(String(r.stderr));
        process.exit(1);
      }
    }
    count++;
  }
}
rmSync(tmp, { recursive: true, force: true });
console.log(`wrote ${count} tones to ${OUT}`);
