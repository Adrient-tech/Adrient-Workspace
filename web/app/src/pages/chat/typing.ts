// Typing signal: report "typing" at most once per window while the user types.
export const TYPING_WINDOW_MS = 4000;
/** How long a person counts as typing after their last signal. */
export const TYPING_TTL_MS = 6000;

export function typingSignal(send: () => void, windowMs = TYPING_WINDOW_MS, now: () => number = Date.now) {
  let last = -Infinity;
  return {
    /** Call on every input event with the current text. Empty text sends nothing. */
    typed(text: string) {
      if (!text.trim()) return false;
      const t = now();
      if (t - last < windowMs) return false;
      last = t;
      send();
      return true;
    },
    reset() {
      last = -Infinity;
    },
  };
}

/** Names line under the conversation: "Ana is typing", "Ana and Ben are typing", "Several people are typing". */
export function typingLabel(names: string[]): string {
  const n = names.filter(Boolean);
  if (n.length === 0) return "";
  if (n.length === 1) return `${n[0]} is typing`;
  if (n.length === 2) return `${n[0]} and ${n[1]} are typing`;
  return "Several people are typing";
}

/** Drops entries older than the TTL. `map` is user id to last-signal time. */
export function activeTypers(map: Record<string, number>, now: number, ttl = TYPING_TTL_MS): string[] {
  return Object.entries(map).filter(([, at]) => now - at < ttl).map(([id]) => id);
}
