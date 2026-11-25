import type { ChatRequest, StreamEvent } from "./types";

/** Parses an SSE byte stream into events. Exported for tests. */
export async function* parseSSE(body: ReadableStream<Uint8Array>): AsyncGenerator<StreamEvent> {
  const reader = body.getReader();
  const dec = new TextDecoder();
  let buf = "";
  for (;;) {
    const { value, done } = await reader.read();
    if (value) buf += dec.decode(value, { stream: true });
    let i: number;
    while ((i = buf.search(/\r?\n\r?\n/)) >= 0) {
      const block = buf.slice(0, i);
      buf = buf.slice(i).replace(/^\r?\n\r?\n/, "");
      const data = block
        .split(/\r?\n/)
        .filter((l) => l.startsWith("data:"))
        .map((l) => l.slice(5).replace(/^ /, ""))
        .join("\n");
      if (!data) continue;
      try {
        yield JSON.parse(data) as StreamEvent;
      } catch {
        /* ignore malformed keep-alive */
      }
    }
    if (done) break;
  }
}

/** POST /api/v1/ai/chat and deliver streamed events. Resolves when the stream ends. */
export function postChat(req: ChatRequest, signal: AbortSignal, onEvent: (e: StreamEvent) => void): Promise<void> {
  return postStream("/api/v1/ai/chat", req, signal, onEvent);
}

/** POST /api/v1/ai/ask (Ask Adrient: answers from the user's own data, with sources). */
export function postAsk(
  req: { question: string; history?: { role: "user" | "assistant"; content: string }[] },
  signal: AbortSignal,
  onEvent: (e: StreamEvent) => void,
): Promise<void> {
  return postStream("/api/v1/ai/ask", req, signal, onEvent);
}

async function postStream(url: string, req: unknown, signal: AbortSignal, onEvent: (e: StreamEvent) => void): Promise<void> {
  const resp = await fetch(url, {
    method: "POST",
    credentials: "include",
    signal,
    headers: { "Content-Type": "application/json", Accept: "text/event-stream" },
    body: JSON.stringify(req),
  });
  if (!resp.ok || !resp.body) {
    let code = `http_${resp.status}`;
    let message = `HTTP ${resp.status}`;
    try {
      const j = (await resp.json()) as { code?: string; message?: string };
      code = j.code ?? code;
      message = j.message ?? message;
    } catch {
      /* not json */
    }
    onEvent({ type: "error", code, message });
    return;
  }
  for await (const e of parseSSE(resp.body)) onEvent(e);
}

export interface AIStatus {
  enabled: boolean;
  configured: boolean;
  reason?: string; // "not_configured" | "disabled_for_app" | "disabled_for_user" | "kill_switch"
  model?: string;
  local_only?: boolean;
  notice?: string;
  is_admin?: boolean;
}

export async function fetchAIStatus(app: string): Promise<AIStatus> {
  try {
    const r = await fetch(`/api/v1/ai/status?app=${encodeURIComponent(app)}`, {
      credentials: "include",
      headers: { Accept: "application/json" },
    });
    if (!r.ok) return { enabled: false, configured: false, reason: "not_configured" };
    return (await r.json()) as AIStatus;
  } catch {
    return { enabled: false, configured: false, reason: "not_configured" };
  }
}
