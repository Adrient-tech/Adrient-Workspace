export type SlaState = "" | "met" | "ok" | "at_risk" | "breached";
export interface SlaClock {
  state: SlaState;
  due?: string;
}
export interface SlaInfo {
  first_response: SlaClock;
  resolution: SlaClock;
}

/** Worst state of a ticket, for the single badge on a list row. */
export function worstState(i?: SlaInfo): SlaState {
  if (!i) return "";
  const order: SlaState[] = ["breached", "at_risk", "ok", "met", ""];
  const states = [i.first_response.state, i.resolution.state];
  for (const s of order) if (states.includes(s)) return s;
  return "";
}

/** The clock that matters now: first response until answered, then resolution. */
export function activeClock(i?: SlaInfo): { name: string; clock: SlaClock } | null {
  if (!i) return null;
  if (i.first_response.state && i.first_response.state !== "met") return { name: "First response", clock: i.first_response };
  if (i.resolution.state) return { name: "Resolution", clock: i.resolution };
  return null;
}

/** "2 h 5 min left", "30 min overdue", "Met". */
export function slaText(c: SlaClock, now = new Date()): string {
  if (!c.state) return "";
  if (c.state === "met") return "Met";
  if (!c.due) return "";
  const diff = Math.round((new Date(c.due).getTime() - now.getTime()) / 60000);
  const abs = Math.abs(diff);
  const h = Math.floor(abs / 60);
  const m = abs % 60;
  const span = h >= 24 ? `${Math.floor(h / 24)} d ${h % 24} h` : h > 0 ? `${h} h ${m} min` : `${m} min`;
  return diff >= 0 ? `${span} left` : `${span} overdue`;
}

export const SLA_COLOR: Record<string, "success" | "warning" | "danger" | "neutral"> = {
  met: "success",
  ok: "neutral",
  at_risk: "warning",
  breached: "danger",
};

/** Minutes as the unit a person would type: 90 -> {value: 90, unit: "min"}, 120 -> {2,"h"}, 2880 -> {2,"d"}. */
export function splitMinutes(min: number): { value: number; unit: "min" | "h" | "d" } {
  if (min > 0 && min % 1440 === 0) return { value: min / 1440, unit: "d" };
  if (min > 0 && min % 60 === 0) return { value: min / 60, unit: "h" };
  return { value: min, unit: "min" };
}
export function toMinutes(value: number, unit: "min" | "h" | "d"): number {
  const v = Math.max(0, Math.floor(value || 0));
  return unit === "d" ? v * 1440 : unit === "h" ? v * 60 : v;
}

/** Block model of a help article body. Rendered with React elements, never as raw HTML. */
export type Block =
  | { kind: "h"; text: string }
  | { kind: "p"; text: string }
  | { kind: "ul"; items: string[] };

export function parseArticle(body: string): Block[] {
  const blocks: Block[] = [];
  let list: string[] | null = null;
  let para: string[] = [];
  const flushPara = () => {
    if (para.length) blocks.push({ kind: "p", text: para.join(" ") });
    para = [];
  };
  const flushList = () => {
    if (list) blocks.push({ kind: "ul", items: list });
    list = null;
  };
  for (const raw of body.replace(/\r/g, "").split("\n")) {
    const line = raw.trim();
    if (!line) {
      flushPara();
      flushList();
      continue;
    }
    const h = /^#{1,3}\s+(.*)$/.exec(line);
    if (h) {
      flushPara();
      flushList();
      blocks.push({ kind: "h", text: h[1] });
      continue;
    }
    const li = /^[-*]\s+(.*)$/.exec(line);
    if (li) {
      flushPara();
      (list ??= []).push(li[1]);
      continue;
    }
    flushList();
    para.push(line);
  }
  flushPara();
  flushList();
  return blocks;
}

/** Only http(s) and mailto links are made clickable in article text. */
export function splitLinks(text: string): { text: string; href?: string }[] {
  const out: { text: string; href?: string }[] = [];
  const re = /\bhttps?:\/\/[^\s<>"')]+/g;
  let last = 0;
  for (const m of text.matchAll(re)) {
    const i = m.index ?? 0;
    if (i > last) out.push({ text: text.slice(last, i) });
    out.push({ text: m[0], href: m[0] });
    last = i + m[0].length;
  }
  if (last < text.length) out.push({ text: text.slice(last) });
  return out;
}

/** Height in percent for a bar series (the tallest bar is 100). */
export function barHeights(counts: number[]): number[] {
  const max = Math.max(1, ...counts);
  return counts.map((c) => Math.round((c / max) * 100));
}

export function fmtMinutes(min: number): string {
  if (!min) return "-";
  if (min < 60) return `${Math.round(min)} min`;
  if (min < 1440) return `${(min / 60).toFixed(1)} h`;
  return `${(min / 1440).toFixed(1)} d`;
}
