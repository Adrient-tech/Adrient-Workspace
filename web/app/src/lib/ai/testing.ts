/** Test helper: a scripted fetch for assistant tool tests (vitest only). */
import { vi } from "vitest";

export interface Call { method: string; url: string; body?: unknown }
type Handler = unknown | ((c: Call) => unknown);

/**
 * routes: "GET /api/v1/crm/deals" -> JSON (or a function of the call). The longest
 * matching "METHOD path-prefix" wins. A handler returning {__status: 403} fails with that status.
 * Unmatched calls return 404. POSTs to /api/v1/ai/audit are recorded in `audits`.
 */
export function mockApi(routes: Record<string, Handler>) {
  const calls: Call[] = [];
  const audits: Record<string, unknown>[] = [];
  const keys = Object.keys(routes).sort((a, b) => b.length - a.length);
  vi.stubGlobal("fetch", vi.fn(async (input: string, init?: RequestInit) => {
    const method = (init?.method ?? "GET").toUpperCase();
    const url = String(input);
    const body = init?.body ? JSON.parse(String(init.body)) : undefined;
    if (url === "/api/v1/ai/audit") {
      audits.push(body as Record<string, unknown>);
      return new Response(JSON.stringify({ ok: true }), { status: 200 });
    }
    const call = { method, url, body };
    calls.push(call);
    const k = keys.find((x) => {
      const [m, p] = x.split(" ");
      return m === method && (url === p || url.startsWith(p + "?") || (p.endsWith("/") && url.startsWith(p)));
    });
    if (!k) return new Response("not found", { status: 404 });
    const h = routes[k];
    const v = typeof h === "function" ? (h as (c: Call) => unknown)(call) : h;
    if (v && typeof v === "object" && "__status" in (v as object)) return new Response((v as { __status: number }).__status === 403 ? "forbidden" : "error", { status: (v as { __status: number }).__status });
    return new Response(JSON.stringify(v ?? {}), { status: 200 });
  }));
  return { calls, audits };
}

/** Runs a tool by name the way the panel would: confirmation is the caller's job (tools with risk destructive). */
export async function run(tools: { name: string; risk: string; handler: (a: Record<string, unknown>) => unknown }[], name: string, args: Record<string, unknown> = {}) {
  const t = tools.find((x) => x.name === name);
  if (!t) throw new Error(`no tool ${name}`);
  return (await t.handler(args)) as { result?: any; error?: string; summary?: string; undo?: () => Promise<void> };
}
