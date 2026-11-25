import type {
  AssistantTool, ChatRequest, StreamEvent, WireMessage, WireToolCall, WireToolDef, JSONSchema,
} from "./types";

export type Transport = (req: ChatRequest, signal: AbortSignal, on: (e: StreamEvent) => void) => Promise<void>;

export interface AppliedChange {
  id: string;
  tool: string;
  summary: string;
  undo?: () => void | Promise<void>;
  undone?: boolean;
}

export interface LoopHooks {
  onText?: (delta: string) => void;
  onSource?: (s: { title: string; url?: string; app?: string }) => void;
  onToolStart?: (call: WireToolCall, tool: AssistantTool | undefined) => void;
  onToolEnd?: (call: WireToolCall, status: "ok" | "error" | "declined", summary: string, change?: AppliedChange) => void;
  /** Resolve true to run a destructive tool. */
  confirm?: (tool: AssistantTool, args: Record<string, unknown>) => Promise<boolean>;
}

export interface LoopResult {
  /** Messages produced by this turn (assistant + tool), to append to history. */
  messages: WireMessage[];
  error?: { code: string; message: string };
  stopped?: boolean;
}

export function toWireTools(tools: AssistantTool[]): WireToolDef[] {
  return tools.map((t) => ({ name: t.name, description: t.description, parameters: t.parameters, risk: t.risk }));
}

/** Minimal argument validation against the tool's schema. Returns an error text or "". */
export function validateArgs(schema: JSONSchema, args: unknown, path = "arguments"): string {
  if (schema.type === "object" || schema.properties) {
    if (typeof args !== "object" || args === null || Array.isArray(args)) return `${path} must be an object`;
    const o = args as Record<string, unknown>;
    for (const r of schema.required ?? []) if (o[r] === undefined) return `${path}.${r} is required`;
    for (const [k, s] of Object.entries(schema.properties ?? {})) {
      if (o[k] !== undefined) {
        const e = validateArgs(s, o[k], `${path}.${k}`);
        if (e) return e;
      }
    }
    return "";
  }
  switch (schema.type) {
    case "string":
      if (typeof args !== "string") return `${path} must be a string`;
      if (schema.enum && !schema.enum.includes(args)) return `${path} must be one of ${schema.enum.join(", ")}`;
      return "";
    case "number":
      return typeof args === "number" && Number.isFinite(args) ? "" : `${path} must be a number`;
    case "integer":
      return Number.isInteger(args) ? "" : `${path} must be an integer`;
    case "boolean":
      return typeof args === "boolean" ? "" : `${path} must be a boolean`;
    case "array":
      if (!Array.isArray(args)) return `${path} must be an array`;
      if (schema.items) {
        for (let i = 0; i < args.length; i++) {
          const e = validateArgs(schema.items, args[i], `${path}[${i}]`);
          if (e) return e;
        }
      }
      return "";
    default:
      return "";
  }
}

const MAX_RESULT = 6000;
/** Change ids are unique for the page lifetime (tool-call ids may repeat across turns). */
let changeSeq = 0;
function encodeResult(v: unknown): string {
  let s: string;
  try {
    s = JSON.stringify(v ?? null);
  } catch {
    s = '"unserialisable result"';
  }
  return s.length > MAX_RESULT ? s.slice(0, MAX_RESULT) + '…[truncated]' : s;
}

/**
 * Runs one user turn: stream the model, execute tool calls in the browser,
 * feed results back, repeat until the model answers without tool calls.
 */
export async function runAssistantTurn(opts: {
  transport: Transport;
  base: Omit<ChatRequest, "messages" | "tools">;
  history: WireMessage[];
  tools: AssistantTool[];
  hooks?: LoopHooks;
  signal: AbortSignal;
  maxSteps?: number;
}): Promise<LoopResult> {
  const { transport, base, tools, hooks, signal } = opts;
  const byName = new Map(tools.map((t) => [t.name, t]));
  const produced: WireMessage[] = [];
  const max = opts.maxSteps ?? 8;

  for (let step = 0; step < max; step++) {
    let text = "";
    const calls: WireToolCall[] = [];
    let err: { code: string; message: string } | undefined;
    try {
      await transport(
        { ...base, messages: [...opts.history, ...produced], tools: tools.length ? toWireTools(tools) : undefined },
        signal,
        (e) => {
          if (e.type === "delta") {
            text += e.text;
            hooks?.onText?.(e.text);
          } else if (e.type === "tool_call") calls.push(e.call);
          else if (e.type === "source") hooks?.onSource?.(e);
          else if (e.type === "error") err = { code: e.code, message: e.message };
        },
      );
    } catch (e) {
      if (signal.aborted) {
        if (text) produced.push({ role: "assistant", content: text });
        return { messages: produced, stopped: true };
      }
      return { messages: produced, error: { code: "network", message: (e as Error).message } };
    }
    if (err) {
      if (text) produced.push({ role: "assistant", content: text });
      return { messages: produced, error: err };
    }
    produced.push({ role: "assistant", content: text, tool_calls: calls.length ? calls : undefined });
    if (!calls.length) return { messages: produced };

    for (const call of calls) {
      if (signal.aborted) return { messages: produced, stopped: true };
      const tool = byName.get(call.name);
      hooks?.onToolStart?.(call, tool);
      const respond = (content: string, status: "ok" | "error" | "declined", summary: string, change?: AppliedChange) => {
        produced.push({ role: "tool", tool_call_id: call.id, content });
        hooks?.onToolEnd?.(call, status, summary, change);
      };
      if (!tool) {
        respond(encodeResult({ error: `unknown tool ${call.name}` }), "error", `Unknown tool ${call.name}`);
        continue;
      }
      let args: Record<string, unknown>;
      try {
        args = call.arguments ? (JSON.parse(call.arguments) as Record<string, unknown>) : {};
      } catch {
        respond(encodeResult({ error: "arguments are not valid JSON" }), "error", `${tool.name}: bad arguments`);
        continue;
      }
      const bad = validateArgs(tool.parameters, args);
      if (bad) {
        respond(encodeResult({ error: bad }), "error", `${tool.name}: ${bad}`);
        continue;
      }
      if (tool.risk === "destructive") {
        const ok = hooks?.confirm ? await hooks.confirm(tool, args) : false;
        if (!ok) {
          respond(encodeResult({ error: "the user declined this action" }), "declined", `Declined: ${tool.label?.(args) ?? tool.name}`);
          continue;
        }
      }
      try {
        const out = await tool.handler(args);
        const summary = out.summary ?? tool.label?.(args) ?? tool.name;
        if (out.error) {
          respond(encodeResult({ error: out.error }), "error", `${tool.name}: ${out.error}`);
        } else {
          const change: AppliedChange | undefined =
            tool.risk !== "read"
              ? { id: `chg${++changeSeq}-${call.id}`, tool: tool.name, summary, undo: out.undo }
              : undefined;
          respond(encodeResult(out.result ?? { ok: true }), "ok", summary, change);
        }
      } catch (e) {
        respond(encodeResult({ error: (e as Error).message }), "error", `${tool.name}: ${(e as Error).message}`);
      }
    }
  }
  return { messages: produced, error: { code: "too_many_steps", message: "The assistant used too many steps; ask again more specifically." } };
}
