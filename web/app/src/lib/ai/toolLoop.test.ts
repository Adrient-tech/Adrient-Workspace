import { describe, expect, it, vi } from "vitest";
import { parseSSE } from "./stream";
import { runAssistantTurn, validateArgs, type Transport } from "./toolLoop";
import type { AssistantTool, ChatRequest, StreamEvent } from "./types";

/** A scripted transport: each call returns the next list of events. */
function scripted(turns: StreamEvent[][]): { transport: Transport; requests: ChatRequest[] } {
  const requests: ChatRequest[] = [];
  let i = 0;
  const transport: Transport = async (req, _signal, on) => {
    requests.push(JSON.parse(JSON.stringify(req)));
    for (const e of turns[i++] ?? [{ type: "done", finish: "stop" }]) on(e);
  };
  return { transport, requests };
}

const call = (id: string, name: string, args: unknown): StreamEvent => ({
  type: "tool_call",
  call: { id, name, arguments: JSON.stringify(args) },
});

function tool(name: string, risk: AssistantTool["risk"], handler: AssistantTool["handler"]): AssistantTool {
  return {
    name,
    risk,
    description: name,
    parameters: { type: "object", properties: { n: { type: "number" } }, required: ["n"] },
    handler,
  };
}

describe("runAssistantTurn", () => {
  it("runs tools, feeds results back and stops on a plain answer", async () => {
    const undo = vi.fn();
    const write = tool("write", "write", (a) => ({ result: { wrote: a.n }, undo, summary: `Wrote ${a.n}` }));
    const { transport, requests } = scripted([
      [{ type: "delta", text: "Working. " }, call("c1", "write", { n: 3 })],
      [{ type: "delta", text: "Done." }, { type: "done", finish: "stop" }],
    ]);
    const changes: string[] = [];
    const r = await runAssistantTurn({
      transport,
      base: { app: "sheets" },
      history: [{ role: "user", content: "go" }],
      tools: [write],
      signal: new AbortController().signal,
      hooks: { onToolEnd: (_c, st, summary, ch) => changes.push(`${st}:${summary}:${ch ? "undoable" : "-"}`) },
    });
    expect(r.error).toBeUndefined();
    expect(changes).toEqual(["ok:Wrote 3:undoable"]);
    expect(requests).toHaveLength(2);
    expect(requests[0].tools?.[0]).toMatchObject({ name: "write", risk: "write" });
    const second = requests[1].messages;
    expect(second[1]).toMatchObject({ role: "assistant", tool_calls: [{ id: "c1", name: "write" }] });
    expect(second[2]).toEqual({ role: "tool", tool_call_id: "c1", content: '{"wrote":3}' });
    expect(r.messages.at(-1)).toEqual({ role: "assistant", content: "Done." });
  });

  it("asks before destructive tools and reports a decline to the model", async () => {
    const handler = vi.fn(() => ({ result: "cleared" }));
    const clear = tool("clear", "destructive", handler);
    const { transport, requests } = scripted([[call("c1", "clear", { n: 1 })], [{ type: "delta", text: "OK" }]]);
    const confirm = vi.fn(async () => false);
    await runAssistantTurn({ transport, base: { app: "sheets" }, history: [{ role: "user", content: "clear" }], tools: [clear], signal: new AbortController().signal, hooks: { confirm } });
    expect(confirm).toHaveBeenCalledOnce();
    expect(handler).not.toHaveBeenCalled();
    expect(requests[1].messages.at(-1)?.content).toContain("declined");
  });

  it("runs destructive tools after confirmation", async () => {
    const handler = vi.fn(() => ({ result: "cleared" }));
    const { transport } = scripted([[call("c1", "clear", { n: 1 })], [{ type: "delta", text: "OK" }]]);
    await runAssistantTurn({ transport, base: { app: "sheets" }, history: [{ role: "user", content: "clear" }], tools: [tool("clear", "destructive", handler)], signal: new AbortController().signal, hooks: { confirm: async () => true } });
    expect(handler).toHaveBeenCalledOnce();
  });

  it("rejects unknown tools and invalid arguments without running anything", async () => {
    const handler = vi.fn(() => ({ result: 1 }));
    const { transport, requests } = scripted([
      [call("a", "nope", {}), call("b", "t", { n: "x" }), { type: "tool_call", call: { id: "c", name: "t", arguments: "{bad" } }],
      [{ type: "delta", text: "sorry" }],
    ]);
    await runAssistantTurn({ transport, base: { app: "docs" }, history: [{ role: "user", content: "x" }], tools: [tool("t", "read", handler)], signal: new AbortController().signal });
    expect(handler).not.toHaveBeenCalled();
    const results = requests[1].messages.filter((m) => m.role === "tool").map((m) => m.content);
    expect(results[0]).toContain("unknown tool");
    expect(results[1]).toContain("arguments.n must be a number");
    expect(results[2]).toContain("not valid JSON");
  });

  it("returns handler errors to the model and surfaces server errors", async () => {
    const { transport } = scripted([[call("a", "t", { n: 1 })], [{ type: "error", code: "quota", message: "limit" }]]);
    const r = await runAssistantTurn({ transport, base: { app: "docs" }, history: [{ role: "user", content: "x" }], tools: [tool("t", "read", () => { throw new Error("boom"); })], signal: new AbortController().signal });
    expect(r.messages.find((m) => m.role === "tool")?.content).toContain("boom");
    expect(r.error).toEqual({ code: "quota", message: "limit" });
  });

  it("stops after maxSteps", async () => {
    const turns = Array.from({ length: 5 }, (_, i) => [call(`c${i}`, "t", { n: i })]);
    const { transport } = scripted(turns);
    const r = await runAssistantTurn({ transport, base: { app: "docs" }, history: [{ role: "user", content: "x" }], tools: [tool("t", "read", () => ({}))], signal: new AbortController().signal, maxSteps: 3 });
    expect(r.error?.code).toBe("too_many_steps");
  });

  it("stops when aborted", async () => {
    const ac = new AbortController();
    const transport: Transport = async (_r, signal, on) => {
      on({ type: "delta", text: "partial" });
      ac.abort();
      if (signal.aborted) throw new DOMException("aborted", "AbortError");
    };
    const r = await runAssistantTurn({ transport, base: { app: "docs" }, history: [{ role: "user", content: "x" }], tools: [], signal: ac.signal });
    expect(r.stopped).toBe(true);
    expect(r.messages).toEqual([{ role: "assistant", content: "partial" }]);
  });
});

describe("validateArgs", () => {
  const schema = {
    type: "object" as const,
    required: ["range"],
    properties: {
      range: { type: "string" as const },
      mode: { type: "string" as const, enum: ["a", "b"] },
      rows: { type: "array" as const, items: { type: "array" as const } },
    },
  };
  it("accepts valid and rejects invalid", () => {
    expect(validateArgs(schema, { range: "A1", mode: "a", rows: [[1]] })).toBe("");
    expect(validateArgs(schema, {})).toContain("range is required");
    expect(validateArgs(schema, { range: "A1", mode: "c" })).toContain("one of");
    expect(validateArgs(schema, { range: "A1", rows: [1] })).toContain("rows[0]");
    expect(validateArgs(schema, [])).toContain("object");
  });
});

describe("parseSSE", () => {
  it("parses events split across chunks", async () => {
    const enc = new TextEncoder();
    const parts = ['data: {"type":"delta","te', 'xt":"Hi"}\n\n', 'data: {"type":"done","finish":"stop"}\n\n'];
    const body = new ReadableStream<Uint8Array>({
      start(c) {
        parts.forEach((p) => c.enqueue(enc.encode(p)));
        c.close();
      },
    });
    const out: StreamEvent[] = [];
    for await (const e of parseSSE(body)) out.push(e);
    expect(out).toEqual([{ type: "delta", text: "Hi" }, { type: "done", finish: "stop" }]);
  });
});
