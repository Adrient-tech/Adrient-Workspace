import type { HostAction, SfuState } from "./types";

/** Result of asking the backend for a LiveKit join token. */
export type JoinResult =
  | { status: "ok"; token: string; url: string; identity: string; isHost: boolean; name: string }
  | { status: "waiting" }
  | { status: "denied" | "locked" | "removed" | "unavailable" | "error"; message?: string };

const BASE = "/api/v1/meet/rooms";

export async function requestJoinToken(
  roomId: string,
  name: string,
  connId: string,
  signal?: AbortSignal,
): Promise<JoinResult> {
  let resp: Response;
  try {
    resp = await fetch(`${BASE}/${encodeURIComponent(roomId)}/join-token`, {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify({ name, conn_id: connId }),
      signal,
    });
  } catch (e) {
    if ((e as Error).name === "AbortError") throw e;
    return { status: "error", message: "Could not reach the server." };
  }
  const body = (await resp.json().catch(() => ({}))) as Record<string, unknown>;
  if (resp.status === 202) return { status: "waiting" };
  if (resp.ok && typeof body.token === "string") {
    return {
      status: "ok",
      token: body.token,
      url: String(body.url ?? ""),
      identity: String(body.identity ?? ""),
      isHost: !!body.is_host,
      name: String(body.name ?? name),
    };
  }
  switch (body.error) {
    case "denied":
      return { status: "denied" };
    case "room_locked":
      return { status: "locked" };
    case "removed_by_host":
      return { status: "removed" };
    case "sfu_not_configured":
      return { status: "unavailable", message: "Video calls are not set up on this server." };
  }
  return { status: "error", message: `Join failed (HTTP ${resp.status}).` };
}

/**
 * Polls join-token until the host lets us in (or says no). onWaiting fires once
 * the first "waiting" answer arrives so the UI can show the waiting screen.
 */
export async function joinWhenAdmitted(
  roomId: string,
  name: string,
  connId: string,
  signal: AbortSignal,
  onWaiting: () => void,
  pollMs = 2000,
): Promise<JoinResult> {
  let announced = false;
  for (;;) {
    const r = await requestJoinToken(roomId, name, connId, signal);
    if (r.status !== "waiting") return r;
    if (!announced) {
      announced = true;
      onWaiting();
    }
    await new Promise<void>((res, rej) => {
      const t = window.setTimeout(res, pollMs);
      signal.addEventListener(
        "abort",
        () => {
          window.clearTimeout(t);
          rej(new DOMException("aborted", "AbortError"));
        },
        { once: true },
      );
    });
  }
}

export async function hostControl(roomId: string, action: HostAction, identity?: string): Promise<boolean> {
  const r = await fetch(`${BASE}/${encodeURIComponent(roomId)}/control`, {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ action, identity }),
  });
  return r.ok;
}

export async function fetchSfuState(roomId: string): Promise<SfuState | null> {
  const r = await fetch(`${BASE}/${encodeURIComponent(roomId)}/sfu-state`, {
    credentials: "same-origin",
    headers: { Accept: "application/json" },
  });
  return r.ok ? ((await r.json()) as SfuState) : null;
}

export async function recordingControl(roomId: string, action: "start" | "stop"): Promise<boolean> {
  const r = await fetch(`${BASE}/${encodeURIComponent(roomId)}/recording`, {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ action }),
  });
  return r.ok;
}

/** Stable per-tab connection id, kept across refresh so a rejoin replaces the old session. */
export function connIdFor(roomId: string): string {
  const key = `meet:cid:${roomId}`;
  try {
    const v = sessionStorage.getItem(key);
    if (v) return v;
  } catch {
    /* storage blocked */
  }
  const id = (crypto.randomUUID?.() ?? String(Math.random())).replace(/[^a-z0-9]/gi, "").slice(0, 12);
  try {
    sessionStorage.setItem(key, id);
  } catch {
    /* ignore */
  }
  return id;
}

export async function postTranscript(roomId: string, text: string): Promise<boolean> {
  const r = await fetch(`${BASE}/${encodeURIComponent(roomId)}/transcript`, {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ text }),
  });
  return r.ok;
}

export const transcriptDownloadUrl = (roomId: string) => `${BASE}/${encodeURIComponent(roomId)}/transcript?format=txt`;
