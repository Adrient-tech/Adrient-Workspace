import { useEffect, useState } from "react";
import { live, type LiveEvent } from "../../lib/live";
import { resolveCode } from "./api";
import type { MeetRoom } from "./types";

/** What a meeting card needs to know about a meeting code. */
export interface MeetInfo {
  name: string;
  ended: boolean;
  durationS: number;
  participants: number;
  endedAt: string;
}

const NONE: MeetInfo = { name: "", ended: false, durationS: 0, participants: 0, endedAt: "" };
const store = new Map<string, MeetInfo>();
const loading = new Map<string, Promise<void>>();
const subs = new Set<() => void>();
let wired = false;

function emit() {
  subs.forEach((f) => f());
}

export function infoFromRoom(r: MeetRoom): MeetInfo {
  return {
    name: r.name || "",
    ended: !!r.ended,
    durationS: r.duration_s ?? 0,
    participants: r.participant_count ?? 0,
    endedAt: r.ended_at ?? "",
  };
}

/** Applies a live event to the store (exported for tests). */
export function applyMeetEvent(e: LiveEvent) {
  const code = typeof e.code === "string" ? e.code : "";
  if (!code) return;
  if (e.type === "meet_ended") {
    store.set(code, {
      name: typeof e.name === "string" ? e.name : (store.get(code)?.name ?? ""),
      ended: true,
      durationS: Number(e.duration_s) || 0,
      participants: Number(e.participant_count) || 0,
      endedAt: typeof e.ended_at === "string" ? e.ended_at : "",
    });
    emit();
  } else if (e.type === "meet_restarted") {
    store.set(code, { ...(store.get(code) ?? NONE), ended: false, durationS: 0, participants: 0, endedAt: "" });
    emit();
  }
}

function wire() {
  if (wired) return;
  wired = true;
  live.subscribe(applyMeetEvent);
}

function load(code: string) {
  let p = loading.get(code);
  if (!p) {
    p = resolveCode(code)
      .then((r) => {
        // A live event that arrived meanwhile is newer than this answer.
        if (!store.has(code)) store.set(code, infoFromRoom(r));
      })
      .catch(() => {
        if (!store.has(code)) store.set(code, NONE);
      })
      .finally(emit);
    loading.set(code, p);
  }
  return p;
}

/** Test helper. */
export function resetMeetStatus() {
  store.clear();
  loading.clear();
}

/** Live state of a meeting code: resolved once, then kept current by push events. */
export function useMeetInfo(code: string): MeetInfo | null {
  const [, tick] = useState(0);
  useEffect(() => {
    wire();
    const f = () => tick((n) => n + 1);
    subs.add(f);
    void load(code);
    return () => {
      subs.delete(f);
    };
  }, [code]);
  return store.get(code) ?? null;
}

export function fmtMeetDuration(s: number): string {
  if (s >= 3600) return `${Math.floor(s / 3600)}h ${String(Math.floor((s % 3600) / 60)).padStart(2, "0")}m`;
  if (s >= 60) return `${Math.floor(s / 60)}m ${String(s % 60).padStart(2, "0")}s`;
  return `${s}s`;
}
