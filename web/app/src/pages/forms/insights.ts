// Derived numbers for the Responses summary: average, recommendation score (NPS), frequent words, responses per day.

/** Answer counts keyed by the answer text ("0".."10" for scales). */
export type Counts = Record<string, number>;

export function scaleAverage(counts: Counts): number | null {
  let n = 0;
  let sum = 0;
  for (const [k, c] of Object.entries(counts)) {
    const v = Number(k);
    if (!Number.isFinite(v) || k.trim() === "" || c <= 0) continue;
    n += c;
    sum += v * c;
  }
  return n ? Math.round((sum / n) * 100) / 100 : null;
}

export interface Nps {
  score: number; // -100..100, whole number
  promoters: number;
  passives: number;
  detractors: number;
  total: number;
}

/** Recommendation score from a 0 to 10 scale: promoters (9-10) minus detractors (0-6), as a percentage of answers. */
export function nps(counts: Counts): Nps | null {
  let promoters = 0;
  let passives = 0;
  let detractors = 0;
  for (const [k, c] of Object.entries(counts)) {
    const v = Number(k);
    if (!Number.isInteger(v) || v < 0 || v > 10 || c <= 0) continue;
    if (v >= 9) promoters += c;
    else if (v >= 7) passives += c;
    else detractors += c;
  }
  const total = promoters + passives + detractors;
  if (!total) return null;
  return { score: Math.round(((promoters - detractors) / total) * 100), promoters, passives, detractors, total };
}

const STOP = new Set(
  "a an and are as at be but by for from had has have he her his i if in is it its me my no not of on or our she so that the their them then there they this to was we were what when which who will with you your would could should about just very really also than too can do does did been being am".split(" "),
);

/** The most frequent meaningful words in free-text answers (lower case, 3+ letters, no stop words). */
export function topWords(texts: string[], limit = 12): { word: string; count: number }[] {
  const freq = new Map<string, number>();
  for (const t of texts) {
    const seen = new Set<string>();
    for (const w of t.toLowerCase().match(/[\p{L}][\p{L}'-]{2,}/gu) ?? []) {
      if (STOP.has(w) || seen.has(w)) continue;
      seen.add(w); // one vote per answer, so one long rant cannot dominate
      freq.set(w, (freq.get(w) ?? 0) + 1);
    }
  }
  return [...freq.entries()]
    .filter(([, c]) => c >= 2)
    .sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
    .slice(0, limit)
    .map(([word, count]) => ({ word, count }));
}

/** Responses per local day for the last `days` days (oldest first), zero-filled. */
export function perDay(createdAt: string[], days = 14, now = new Date()): { day: string; count: number }[] {
  const key = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
  const out: { day: string; count: number }[] = [];
  const idx = new Map<string, number>();
  for (let i = days - 1; i >= 0; i--) {
    const d = new Date(now.getFullYear(), now.getMonth(), now.getDate() - i);
    idx.set(key(d), out.length);
    out.push({ day: key(d), count: 0 });
  }
  for (const iso of createdAt) {
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) continue;
    const i = idx.get(key(d));
    if (i !== undefined) out[i].count++;
  }
  return out;
}
