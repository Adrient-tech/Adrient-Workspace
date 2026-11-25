/**
 * Workspace-wide live feed: one WebSocket per tab to /api/v1/chat/x/live,
 * opened at sign-in. Every app can subscribe to server push events
 * (notifications, call rings, chat messages) without owning a socket.
 */
export type LiveEvent = { type: string } & Record<string, unknown>;
type Handler = (e: LiveEvent) => void;

const handlers = new Set<Handler>();
let ws: WebSocket | null = null;
let started = false;
let retry = 0;
let timer: number | undefined;

function connect() {
  const proto = window.location.protocol === "https:" ? "wss" : "ws";
  try {
    ws = new WebSocket(`${proto}://${window.location.host}/api/v1/chat/x/live`);
  } catch {
    schedule();
    return;
  }
  ws.onopen = () => {
    retry = 0;
  };
  ws.onmessage = (m) => {
    let e: LiveEvent;
    try {
      e = JSON.parse(String(m.data)) as LiveEvent;
    } catch {
      return;
    }
    if (!e || typeof e.type !== "string") return;
    handlers.forEach((h) => {
      try {
        h(e);
      } catch {
        /* one bad handler must not break the feed */
      }
    });
  };
  ws.onclose = () => {
    ws = null;
    if (started) schedule();
  };
}

function schedule() {
  window.clearTimeout(timer);
  const delay = Math.min(30_000, 1000 * 2 ** retry++);
  timer = window.setTimeout(connect, delay);
}

export const live = {
  start() {
    if (started) return;
    started = true;
    connect();
  },
  stop() {
    started = false;
    window.clearTimeout(timer);
    ws?.close();
    ws = null;
  },
  subscribe(fn: Handler) {
    handlers.add(fn);
    return () => {
      handlers.delete(fn);
    };
  },
};
