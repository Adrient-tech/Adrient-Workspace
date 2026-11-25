/**
 * Helpers for the assistant tool sets of the business apps (CRM, HR, Talent,
 * Accounting, Inventory, Invoicing, POS, Projects, Tickets, Admin).
 *
 * Rules every tool built with bizTool follows (docs/ai.md, "Business apps"):
 *  - Tools run in the browser with the user's own session, so the module API
 *    is always the permission check. A 403 comes back to the model as an error.
 *  - Reads run at once. Every write is risk "destructive": the panel asks the
 *    user to Allow/Decline first, and the call is audited (POST /api/v1/ai/audit,
 *    tool name + one-line summary, never arguments or results).
 *  - Free text from records (CRM notes, ticket bodies, resumes, memos) is data.
 *    clip()/untrusted() shorten it and mark it so the model treats it as such;
 *    the server wraps tool results as untrusted data and redacts secrets too.
 *  - Results stay small: lists are capped and trimmed.
 */
import type { AssistantTool, JSONSchema, ToolOutcome } from "./types";

export type Args = Record<string, unknown>;

export interface BizToolSpec {
  name: string;
  description: string;
  parameters?: JSONSchema;
  /** Changes data: needs confirmation in the panel and is audited. */
  write?: boolean;
  /** Reads personal or financial data: audited too. */
  sensitive?: boolean;
  label?: (a: Args) => string;
  run: (a: Args) => Promise<Omit<ToolOutcome, "summary"> & { summary?: string; target?: string }>;
}

export const obj = (properties: Record<string, JSONSchema> = {}, required: string[] = []): JSONSchema => ({
  type: "object",
  properties,
  required,
});
export const str = (description: string, enumValues?: string[]): JSONSchema => ({ type: "string", description, ...(enumValues ? { enum: enumValues } : {}) });
export const num = (description: string): JSONSchema => ({ type: "number", description });
export const int = (description: string): JSONSchema => ({ type: "integer", description });
export const bool = (description: string): JSONSchema => ({ type: "boolean", description });
export const arr = (items: JSONSchema, description: string): JSONSchema => ({ type: "array", items, description });

/** Posts the audit entry; never throws and never blocks the tool. */
export function auditTool(app: string, tool: string, risk: string, status: "ok" | "declined" | "error", summary: string, target = ""): void {
  try {
    void fetch("/api/v1/ai/audit", {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ app, tool, risk, status, summary: summary.slice(0, 300), target }),
    }).catch(() => undefined);
  } catch {
    /* auditing must not break the tool */
  }
}

/** "crm_stalled_deals" -> "Stalled deals" (the prefix is the app). */
export function humanize(name: string): string {
  const t = name.replace(/^[a-z]+_/, "").replace(/_/g, " ");
  return t.charAt(0).toUpperCase() + t.slice(1);
}

/** Builds an AssistantTool from a spec: confirmation for writes, audit, error handling. */
export function bizTool(app: string, spec: BizToolSpec): AssistantTool {
  const risk = spec.write ? "destructive" : "read";
  return {
    name: spec.name,
    description: spec.description,
    parameters: spec.parameters ?? obj(),
    risk,
    label: spec.label ?? (() => humanize(spec.name)),
    handler: async (args) => {
      try {
        const out = await spec.run(args);
        if (out.error) {
          if (spec.write) auditTool(app, spec.name, risk, "error", out.error);
          return { error: out.error };
        }
        const summary = out.summary ?? spec.label?.(args) ?? humanize(spec.name);
        if (spec.write || spec.sensitive) auditTool(app, spec.name, risk, "ok", summary, out.target ?? "");
        const { target: _t, ...rest } = out;
        void _t;
        if (!spec.write) return { ...rest, summary };
        notifyChanged(app);
        const undo = rest.undo;
        return { ...rest, summary, undo: undo ? async () => { await undo(); notifyChanged(app); } : undefined };
      } catch (e) {
        const msg = friendlyError(e);
        if (spec.write) auditTool(app, spec.name, risk, "error", msg);
        return { error: msg };
      }
    },
  };
}

const CHANGED = "adrient-ai-changed";
/** Tells the open page that the assistant changed data, so it can reload. */
export function notifyChanged(app: string): void {
  if (typeof window !== "undefined") window.dispatchEvent(new CustomEvent(CHANGED, { detail: { app } }));
}
/** Subscribes to notifyChanged for one app; returns the unsubscribe. */
export function onAiChanged(app: string, cb: () => void): () => void {
  const h = (e: Event) => { if ((e as CustomEvent<{ app: string }>).detail?.app === app) cb(); };
  window.addEventListener(CHANGED, h);
  return () => window.removeEventListener(CHANGED, h);
}

/** A readable message for the model: 403 = no permission, so it will not retry blindly. */
export function friendlyError(e: unknown): string {
  const m = e instanceof Error ? e.message : String(e);
  if (/\b403\b|forbidden|not allowed|permission denied|HR admin only|privileges required/i.test(m)) return "You do not have permission for this (the app refused the request).";
  if (/\b404\b|not found/i.test(m)) return "Not found.";
  return m.slice(0, 300);
}

/** Same-origin JSON request with the user's session. */
export async function api<T = unknown>(url: string, init?: { method?: string; body?: unknown }): Promise<T> {
  const r = await fetch(url, {
    method: init?.method ?? "GET",
    credentials: "same-origin",
    headers: { Accept: "application/json", "Content-Type": "application/json" },
    body: init?.body === undefined ? undefined : JSON.stringify(init.body),
  });
  if (!r.ok) {
    const text = (await r.text().catch(() => "")).trim();
    throw new Error(`HTTP ${r.status}${text ? `: ${text.slice(0, 200)}` : ""}`);
  }
  if (r.status === 204) return undefined as T;
  const text = await r.text();
  return (text ? JSON.parse(text) : undefined) as T;
}

/** Shortens free text from a record. It is still DATA, never instructions. */
export function clip(s: unknown, n = 400): string {
  const t = typeof s === "string" ? s : s == null ? "" : String(s);
  // eslint-disable-next-line no-control-regex -- strip control characters from untrusted text
  const clean = t.replace(/[\u0000-\u0008\u000b\u000c\u000e-\u001f]/g, " ").trim();
  return clean.length > n ? `${clean.slice(0, n)}…` : clean;
}

/** Free text from other people: marked so a model (and a reader of the result) knows it is data. */
export function untrusted(s: unknown, n = 600): { untrusted_text: string } {
  return { untrusted_text: clip(s, n) };
}

/** Keeps at most `limit` rows and tells the model when it cut the list. */
export function capList<T>(rows: T[], limit = 25): { rows: T[]; total: number; truncated: boolean } {
  return { rows: rows.slice(0, limit), total: rows.length, truncated: rows.length > limit };
}

export function money(n: number, currency = "INR"): string {
  try {
    return new Intl.NumberFormat("en-IN", { style: "currency", currency, maximumFractionDigits: 2 }).format(n);
  } catch {
    return `${currency} ${n.toFixed(2)}`;
  }
}

export const round2 = (n: number) => Math.round((n + Number.EPSILON) * 100) / 100;
export const today = () => new Date().toISOString().slice(0, 10);
export function daysBetween(a: string | Date, b: string | Date): number {
  return Math.floor((new Date(b).getTime() - new Date(a).getTime()) / 86400000);
}
export const lc = (s: unknown) => String(s ?? "").toLowerCase();
export const has = (hay: unknown, needle: string) => lc(hay).includes(lc(needle));

/** Confirmation text for a bulk list: "Acme renewal, Beta rollout and 3 more". */
export function listLabel(names: string[], max = 3): string {
  const shown = names.slice(0, max).join(", ");
  return names.length > max ? `${shown} and ${names.length - max} more` : shown;
}

/** Drops write tools for users who may only read (the server filters the same way). */
export function withAccess(tools: AssistantTool[], canWrite: boolean): AssistantTool[] {
  return canWrite ? tools : tools.filter((t) => t.risk === "read");
}
