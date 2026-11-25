import { createMeeting } from "../pages/meet/api";
import { sounds } from "./sounds";

/** Path to open a meeting with a way back to where the user is now. */
export function meetPath(code: string, opts: { returnTo?: string; audioOnly?: boolean } = {}): string {
  const q = new URLSearchParams();
  if (opts.returnTo && opts.returnTo.startsWith("/") && !opts.returnTo.startsWith("/meet/")) {
    q.set("return", opts.returnTo);
  }
  if (opts.audioOnly) q.set("audio", "1");
  const qs = q.toString();
  return `/meet/${code}${qs ? `?${qs}` : ""}`;
}

async function post(path: string, body: unknown): Promise<void> {
  const r = await fetch(`/api/v1/chat/x/${path}`, {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(body),
  });
  if (!r.ok) {
    let msg = `HTTP ${r.status}`;
    try {
      msg = ((await r.json()) as { error?: string }).error ?? msg;
    } catch {
      /* keep status */
    }
    throw new Error(msg);
  }
}

/**
 * startCall creates a meeting, rings the given colleagues on every page they
 * have open, and returns the path to open the call (with a way back).
 */
export async function startCall(opts: {
  userIds: string[];
  title: string;
  video: boolean;
  returnTo?: string;
}): Promise<{ code: string; path: string }> {
  const room = await createMeeting(opts.title);
  const code = room.code ?? "";
  if (!code) throw new Error("meeting has no code");
  if (opts.userIds.length) {
    await post("calls/ring", { user_ids: opts.userIds, code, title: opts.title, video: opts.video });
    void sounds.loop("ringback"); // stops when they answer, decline, or the call ends
  }
  return { code, path: meetPath(code, { returnTo: opts.returnTo, audioOnly: !opts.video }) };
}

export function answerCall(toUserId: string, code: string, answer: "accepted" | "declined" | "cancelled") {
  return post("calls/respond", { to_user_id: toUserId, code, answer }).catch(() => {});
}

/** Tells the server this user left the call (stamps the call log's end time). */
export function logCallEnd(code: string) {
  return post("calls/end", { code }).catch(() => {});
}

export interface CallLogEntry {
  id: string;
  code: string;
  direction: "in" | "out";
  status: "answered" | "missed" | "declined" | "ringing";
  video: boolean;
  title: string;
  caller_id: string;
  peer_ids: string[];
  started_at: string;
  answered_at: string;
  ended_at: string;
  duration_s: number;
}

export async function listCallHistory(): Promise<CallLogEntry[]> {
  const r = await fetch("/api/v1/chat/x/calls/history", { credentials: "same-origin", headers: { Accept: "application/json" } });
  if (!r.ok) throw new Error(`HTTP ${r.status}`);
  return ((await r.json()) as { calls: CallLogEntry[] }).calls ?? [];
}
