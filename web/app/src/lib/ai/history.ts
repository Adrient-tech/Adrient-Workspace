/** Per-document chat history kept in localStorage (browser only, never sent except as the live conversation). */
export interface UIMessage {
  id: string;
  role: "user" | "assistant" | "activity";
  text: string;
  /** activity rows */
  status?: "ok" | "error" | "declined" | "pending";
  changeId?: string;
  sources?: { title: string; url?: string; app?: string }[];
}

const KEY = (app: string, doc: string) => `adrient.ai.history.${app}.${doc}`;
const MAX = 60;

export function loadHistory(app: string, doc: string, store: Storage = localStorage): UIMessage[] {
  try {
    const raw = store.getItem(KEY(app, doc));
    if (!raw) return [];
    const v = JSON.parse(raw) as UIMessage[];
    return Array.isArray(v) ? v.filter((m) => m && typeof m.text === "string") : [];
  } catch {
    return [];
  }
}

export function saveHistory(app: string, doc: string, msgs: UIMessage[], store: Storage = localStorage): void {
  try {
    store.setItem(KEY(app, doc), JSON.stringify(msgs.filter((m) => m.status !== "pending").slice(-MAX)));
  } catch {
    /* quota: history is a convenience */
  }
}

export function clearHistory(app: string, doc: string, store: Storage = localStorage): void {
  store.removeItem(KEY(app, doc));
}

/** Rebuild the wire conversation (user/assistant text only) from stored UI messages. */
export function toWireHistory(msgs: UIMessage[]) {
  return msgs
    .filter((m) => m.role === "user" || (m.role === "assistant" && m.text))
    .map((m) => ({ role: m.role as "user" | "assistant", content: m.text }));
}
