// Speech to text for the meeting transcript, using the browser's own recogniser (Chrome, Edge, Safari).
// Only the speaker's own microphone is transcribed, in their browser, and only while the host has the
// transcript switched on (everyone sees a banner). Firefox has no recogniser: those users are simply not transcribed.

interface RecognitionResultLike {
  isFinal: boolean;
  0: { transcript: string };
}
interface RecognitionEventLike {
  resultIndex: number;
  results: ArrayLike<RecognitionResultLike>;
}
interface RecognitionLike {
  continuous: boolean;
  interimResults: boolean;
  lang: string;
  onresult: ((e: RecognitionEventLike) => void) | null;
  onend: (() => void) | null;
  onerror: ((e: { error: string }) => void) | null;
  start: () => void;
  stop: () => void;
}
type Ctor = new () => RecognitionLike;

export function recognitionCtor(w: unknown = typeof window === "undefined" ? undefined : window): Ctor | null {
  const x = w as { SpeechRecognition?: Ctor; webkitSpeechRecognition?: Ctor } | undefined;
  return x?.SpeechRecognition ?? x?.webkitSpeechRecognition ?? null;
}
export const transcriptionSupported = () => recognitionCtor() !== null;

/** Final utterances from one recogniser event, trimmed, empty ones dropped. */
export function finalSegments(e: RecognitionEventLike): string[] {
  const out: string[] = [];
  for (let i = e.resultIndex; i < e.results.length; i++) {
    const r = e.results[i];
    if (r.isFinal) {
      const t = r[0].transcript.replace(/\s+/g, " ").trim();
      if (t) out.push(t);
    }
  }
  return out;
}

/** Starts continuous recognition; restarts itself when the browser ends a session. Returns a stop function. */
export function startTranscribing(onSegment: (text: string) => void, lang = navigator.language || "en-US"): () => void {
  const C = recognitionCtor();
  if (!C) return () => undefined;
  let stopped = false;
  const rec = new C();
  rec.continuous = true;
  rec.interimResults = false;
  rec.lang = lang;
  rec.onresult = (e) => finalSegments(e).forEach(onSegment);
  rec.onerror = (e) => {
    if (e.error === "not-allowed" || e.error === "service-not-allowed") stopped = true;
  };
  rec.onend = () => {
    if (!stopped) {
      try {
        rec.start();
      } catch {
        /* already started */
      }
    }
  };
  try {
    rec.start();
  } catch {
    /* ignore */
  }
  return () => {
    stopped = true;
    rec.onend = null;
    try {
      rec.stop();
    } catch {
      /* ignore */
    }
  };
}
