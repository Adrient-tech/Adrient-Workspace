// Read aloud (Docs wave 6): the selected text, or the whole document, spoken
// through the browser's speech synthesis. Utterances take a few thousand
// characters at a time, so the text is cut into chunks at paragraph and
// sentence breaks and spoken one after the other; Stop halts the lot.

/** Characters per utterance; browsers stop long ones silently. */
export const SPEECH_CHUNK_MAX = 2800;
/** Reading a whole book at once is a mistake; this many chunks is plenty for one sitting. */
export const SPEECH_CHUNK_LIMIT = 40;

/** Sentences of a paragraph, keeping their punctuation. */
function sentenceParts(para: string): string[] {
  const parts: string[] = [];
  let start = 0;
  // Split only at existing whitespace after punctuation. Matching the text
  // between punctuation marks loses leading dots and breaks decimals/URLs.
  for (const match of para.matchAll(/(?:[.!?…]["'”’)]*\s+|[。！？][」』）]*)/g)) {
    const end = (match.index ?? 0) + match[0].length;
    parts.push(para.slice(start, end).trim());
    start = end;
  }
  if (start < para.length) parts.push(para.slice(start).trim());
  return parts;
}

/** Cut an overlong run at word boundaries. */
function hardSplit(text: string, max: number): string[] {
  const out: string[] = [];
  let rest = text;
  while (rest.length > max) {
    let at = rest.lastIndexOf(" ", max);
    // Text without spaces (Chinese, Japanese) breaks after its punctuation rather than mid-word.
    if (at < max / 2) {
      const punctuation = rest.slice(0, max).search(/[、，。！？；：」』）][^、，。！？；：」』）]*$/);
      at = punctuation >= max / 4 ? punctuation + 1 : max;
    }
    const before = rest.charCodeAt(at - 1);
    const after = rest.charCodeAt(at);
    if (before >= 0xd800 && before <= 0xdbff && after >= 0xdc00 && after <= 0xdfff) at += at === 1 ? 1 : -1;
    out.push(rest.slice(0, at).trim());
    rest = rest.slice(at).trim();
  }
  if (rest) out.push(rest);
  return out;
}

/**
 * Cut text into chunks of at most `max` UTF-16 units, preferring paragraph,
 * sentence, then word breaks. A single Unicode character is never split.
 */
export function splitForSpeech(text: string, max = SPEECH_CHUNK_MAX): string[] {
  const clean = String(text || "").replace(/[ \t]+/g, " ").trim();
  if (!clean) return [];
  const out: string[] = [];
  let buffer = "";
  const flush = () => {
    if (buffer.trim()) out.push(buffer.trim());
    buffer = "";
  };
  const pieces = clean.split(/\n+/).flatMap((para) => (para.trim() ? sentenceParts(para.trim()) : []));
  for (const piece of pieces) {
    if (piece.length > max) {
      flush();
      for (const part of hardSplit(piece, max)) out.push(part);
      continue;
    }
    if (buffer && buffer.length + 1 + piece.length > max) flush();
    buffer = buffer ? `${buffer} ${piece}` : piece;
  }
  flush();
  return out;
}

export type ReaderState = "idle" | "speaking";

/** Whether this browser can speak text. */
export const canReadAloud = () => typeof window !== "undefined" && "speechSynthesis" in window && typeof SpeechSynthesisUtterance !== "undefined";

let session = 0;
let listeners: ((s: ReaderState) => void)[] = [];
let state: ReaderState = "idle";
const set = (s: ReaderState) => {
  if (state === s) return;
  state = s;
  for (const l of listeners) l(s);
};
export const readerState = () => state;
export function onReaderState(l: (s: ReaderState) => void): () => void {
  listeners.push(l);
  return () => (listeners = listeners.filter((x) => x !== l));
}

/** Stops reading. */
export function stopReading(): void {
  session++;
  if (canReadAloud()) window.speechSynthesis.cancel();
  set("idle");
}

/** Reads `text` aloud; resolves true when it finished on its own. */
export function readAloud(text: string, lang?: string): Promise<boolean> {
  stopReading();
  if (!canReadAloud()) return Promise.resolve(false);
  const chunks = splitForSpeech(text).slice(0, SPEECH_CHUNK_LIMIT);
  if (!chunks.length) return Promise.resolve(false);
  const mine = ++session;
  set("speaking");
  return new Promise((resolve) => {
    let i = 0;
    const next = () => {
      if (mine !== session) return resolve(false);
      if (i >= chunks.length) {
        set("idle");
        return resolve(true);
      }
      const u = new SpeechSynthesisUtterance(chunks[i++]);
      if (lang) u.lang = lang;
      u.onend = next;
      u.onerror = () => {
        if (mine === session) set("idle");
        resolve(false);
      };
      window.speechSynthesis.speak(u);
    };
    next();
  });
}
