/**
 * Small helpers shared by the per-app tool sets (mail, calendar, tasks, ...).
 * Tools run in the browser with the user's own session, so the server's own
 * authorisation applies to every call they make.
 */
import type { AssistantTool, ToolOutcome } from "./types";

/** Fires after an AI tool changed data so the open page can reload. */
export const AI_CHANGED_EVENT = "adrient:ai-changed";
export function notifyAIChanged(app: string): void {
  if (typeof window !== "undefined") window.dispatchEvent(new CustomEvent(AI_CHANGED_EVENT, { detail: { app } }));
}
/** Subscribe the page to AI-made changes of `app`; returns an unsubscribe. */
export function onAIChanged(app: string, cb: () => void): () => void {
  const h = (e: Event) => {
    if ((e as CustomEvent).detail?.app === app) cb();
  };
  window.addEventListener(AI_CHANGED_EVENT, h);
  return () => window.removeEventListener(AI_CHANGED_EVENT, h);
}

/** Cut text to n characters (keeps results small). */
export function clip(s: string | undefined | null, n: number): string {
  const t = (s ?? "").replace(/\r/g, "");
  return t.length > n ? t.slice(0, n) + "…[truncated]" : t;
}

/** Wrap third-party text so the model treats it as data (the server also wraps results). */
export function untrusted(s: string | undefined | null, n = 1500): string {
  return clip(s, n);
}

export function str(v: unknown): string {
  return typeof v === "string" ? v.trim() : "";
}
export function strList(v: unknown, max = 50): string[] {
  return Array.isArray(v) ? v.filter((x): x is string => typeof x === "string").map((x) => x.trim()).filter(Boolean).slice(0, max) : [];
}
export function num(v: unknown, d: number, lo: number, hi: number): number {
  const n = typeof v === "number" && Number.isFinite(v) ? v : d;
  return Math.min(hi, Math.max(lo, n));
}
export function errMsg(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}

/** Run an async body; turn a thrown error into a tool error the model can read. */
export async function guard(fn: () => Promise<ToolOutcome>): Promise<ToolOutcome> {
  try {
    return await fn();
  } catch (e) {
    return { error: errMsg(e) };
  }
}

/** Run undo steps newest first, ignoring individual failures after the first throw is rethrown at the end. */
export function chainUndo(steps: Array<() => void | Promise<void>>): () => Promise<void> {
  return async () => {
    let first: unknown;
    for (const s of [...steps].reverse()) {
      try {
        await s();
      } catch (e) {
        first ??= e;
      }
    }
    if (first) throw first;
  };
}

/** Convenience for building a tool with the common shape. */
export function defineTool(t: AssistantTool): AssistantTool {
  return t;
}
