import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import AssistantPanel from "./AssistantPanel";
import type { AssistantTool, StreamEvent } from "./types";

function sse(events: StreamEvent[]) {
  const enc = new TextEncoder();
  return {
    ok: true,
    status: 200,
    body: new ReadableStream<Uint8Array>({
      start(c) {
        events.forEach((e) => c.enqueue(enc.encode(`data: ${JSON.stringify(e)}\n\n`)));
        c.close();
      },
    }),
    json: async () => ({}),
  };
}

let turns: StreamEvent[][] = [];
let status: Record<string, unknown> = {};
let bodies: { messages: { role: string; content: string }[]; context?: { label: string } }[] = [];

beforeEach(() => {
  localStorage.clear();
  turns = [];
  bodies = [];
  status = { enabled: true, configured: true, notice: "Data notice", is_admin: false };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init?: RequestInit) => {
      if (url.startsWith("/api/v1/ai/status")) return { ok: true, status: 200, json: async () => status };
      if (url === "/api/v1/ai/chat") {
        bodies.push(JSON.parse(String(init?.body)));
        return sse(turns.shift() ?? [{ type: "delta", text: "ok" }, { type: "done", finish: "stop" }]);
      }
      return { ok: false, status: 404, json: async () => ({}) };
    }),
  );
});
afterEach(() => vi.unstubAllGlobals());

function grid() {
  const cells: Record<string, string> = {};
  const write: AssistantTool = {
    name: "sheet_write_cells",
    description: "write",
    risk: "write",
    parameters: { type: "object", properties: { cell: { type: "string" }, value: { type: "string" } }, required: ["cell", "value"] },
    handler: (a) => {
      const cell = String(a.cell);
      const before = cells[cell];
      cells[cell] = String(a.value);
      return { result: { ok: true }, summary: `Wrote ${cell}`, undo: () => { if (before === undefined) delete cells[cell]; else cells[cell] = before; } };
    },
  };
  const clear: AssistantTool = {
    name: "sheet_clear_range",
    description: "clear",
    risk: "destructive",
    parameters: { type: "object", properties: {} },
    label: () => "Clear A1:D20",
    handler: () => {
      Object.keys(cells).forEach((k) => delete cells[k]);
      return { result: { ok: true } };
    },
  };
  return { cells, tools: [write, clear] };
}

const call = (id: string, name: string, args: unknown): StreamEvent => ({ type: "tool_call", call: { id, name, arguments: JSON.stringify(args) } });

describe("AssistantPanel", () => {
  it("shows the not-configured state with an admin link for admins", async () => {
    status = { enabled: false, configured: false, reason: "not_configured", is_admin: true };
    render(<AssistantPanel app="sheets" documentId="d1" />);
    expect(await screen.findByText(/AI is not configured/)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open AI settings" })).toHaveAttribute("href", "/admin/ai");
    expect(screen.getByLabelText("Message the assistant")).toBeDisabled();
  });

  it("hides the admin link for members and explains the kill switch", async () => {
    status = { enabled: false, configured: true, reason: "kill_switch", is_admin: false };
    render(<AssistantPanel app="sheets" documentId="d1" />);
    expect(await screen.findByText(/switched off for your organisation/)).toBeInTheDocument();
    expect(screen.queryByRole("link")).toBeNull();
  });

  it("sends a suggestion chip with the selection, applies tool edits and undoes them", async () => {
    const g = grid();
    turns = [
      [{ type: "delta", text: "Writing. " }, call("c1", "sheet_write_cells", { cell: "A1", value: "Income" })],
      [call("c2", "sheet_write_cells", { cell: "A2", value: "Rent" })],
      [{ type: "delta", text: "Budget **done**." }, { type: "done", finish: "stop" }],
    ];
    render(<AssistantPanel app="sheets" documentId="d1" tools={g.tools} getContext={() => ({ label: "Sheet1!A1", text: "" })} />);
    expect(await screen.findByTestId("ai-context-chip")).toHaveTextContent("Sheet1!A1");
    fireEvent.click(await screen.findByText("Build a monthly budget with income, expenses and totals"));
    expect(await screen.findByText("done")).toBeInTheDocument();
    expect(g.cells).toEqual({ A1: "Income", A2: "Rent" });
    expect(bodies[0].context?.label).toBe("Sheet1!A1");
    expect(bodies).toHaveLength(3);
    const rows = screen.getAllByTestId("ai-activity");
    expect(rows.map((r) => r.textContent)).toEqual(["✓Wrote A1Undo", "✓Wrote A2Undo"]);
    fireEvent.click(screen.getAllByRole("button", { name: "Undo" })[1]);
    await waitFor(() => expect(g.cells).toEqual({ A1: "Income" }));
    fireEvent.click(screen.getByRole("button", { name: "Undo all AI changes" }));
    await waitFor(() => expect(g.cells).toEqual({}));
    expect(screen.queryByRole("button", { name: "Undo all AI changes" })).toBeNull();
  });

  it("asks for confirmation before a destructive tool", async () => {
    const g = grid();
    g.cells.A1 = "keep";
    turns = [[call("c1", "sheet_clear_range", {})], [{ type: "delta", text: "Left unchanged." }]];
    render(<AssistantPanel app="sheets" documentId="d2" tools={g.tools} />);
    fireEvent.change(await screen.findByLabelText("Message the assistant"), { target: { value: "clear everything" } });
    fireEvent.click(screen.getByLabelText("Send"));
    expect(await screen.findByRole("alertdialog")).toHaveTextContent("Confirm: Clear A1:D20");
    fireEvent.click(screen.getByRole("button", { name: "Decline" }));
    expect(await screen.findByText("Left unchanged.")).toBeInTheDocument();
    expect(g.cells).toEqual({ A1: "keep" });
    expect(bodies[1].messages.at(-1)?.content).toContain("declined");
  });

  it("keeps history per document and clears it with New chat", async () => {
    const { unmount } = render(<AssistantPanel app="docs" documentId="doc-a" />);
    fireEvent.change(await screen.findByLabelText("Message the assistant"), { target: { value: "hello" } });
    await act(async () => {
      fireEvent.click(screen.getByLabelText("Send"));
    });
    expect(await screen.findByText("ok")).toBeInTheDocument();
    unmount();
    render(<AssistantPanel app="docs" documentId="doc-a" />);
    expect(await screen.findByText("hello")).toBeInTheDocument();
    fireEvent.click(screen.getByLabelText("New chat"));
    expect(screen.queryByText("hello")).toBeNull();
  });

  it("keeps separate activity rows and undo buttons when a provider reuses tool-call ids", async () => {
    const g = grid();
    turns = [
      [call("same", "sheet_write_cells", { cell: "A1", value: "x" })],
      [{ type: "delta", text: "one" }],
      [call("same", "sheet_write_cells", { cell: "B1", value: "y" })],
      [{ type: "delta", text: "two" }],
    ];
    render(<AssistantPanel app="sheets" documentId="d3" tools={g.tools} />);
    for (const q of ["first", "second"]) {
      fireEvent.change(await screen.findByLabelText("Message the assistant"), { target: { value: q } });
      fireEvent.click(screen.getByLabelText("Send"));
      await screen.findByText(q === "first" ? "one" : "two");
    }
    expect(screen.getAllByTestId("ai-activity").map((r) => r.textContent)).toEqual(["✓Wrote A1Undo", "✓Wrote B1Undo"]);
    fireEvent.click(screen.getAllByRole("button", { name: "Undo" })[0]);
    await waitFor(() => expect(g.cells).toEqual({ B1: "y" }));
  });

  it("shows provider errors inline", async () => {
    turns = [[{ type: "error", code: "quota", message: "you have used today's AI allowance" }]];
    render(<AssistantPanel app="docs" documentId="doc-b" />);
    fireEvent.change(await screen.findByLabelText("Message the assistant"), { target: { value: "hi" } });
    fireEvent.click(screen.getByLabelText("Send"));
    expect(await screen.findByText(/today's AI allowance/)).toBeInTheDocument();
  });
});
