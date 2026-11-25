import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AISection } from "./AISection";

const PID = "11111111-1111-4111-8111-111111111111";
let calls: { url: string; method: string; body?: unknown }[] = [];

beforeEach(() => {
  calls = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init?: RequestInit) => {
      const method = init?.method ?? "GET";
      calls.push({ url, method, body: init?.body ? JSON.parse(String(init.body)) : undefined });
      const ok = (v: unknown) => ({ ok: true, status: 200, json: async () => v });
      if (url.endsWith("/settings") && method === "GET")
        return ok({ settings: { kill_switch: false, local_only: false, default_model: "", notice: "Notice", org_daily_tokens: 0, user_daily_tokens: 0, user_per_minute: 0, max_output_tokens: 0 }, apps: ["sheets", "docs", "global"], server_key: true });
      if (url.endsWith("/settings")) return ok(JSON.parse(String(init?.body)));
      if (url.endsWith("/providers"))
        return ok([{ id: PID, name: "Local Ollama", kind: "openai", base_url: "http://127.0.0.1:11434/v1", models: ["llama3.1:8b"], local: true, enabled: true, has_key: true, key_source: "stored" }]);
      if (url.endsWith("/test")) return ok({ ok: true, model: "llama3.1:8b", latency_ms: 42 });
      if (url.includes("/usage")) return ok({ rows: [{ key: "sheets", requests: 3, input_tokens: 1200, output_tokens: 300, cost_usd: 0.01 }] });
      return { ok: false, status: 404, json: async () => ({}) };
    }),
  );
});
afterEach(() => vi.unstubAllGlobals());

describe("AISection", () => {
  it("lists providers without keys, tests a connection and saves per-app switches", async () => {
    render(<MemoryRouter><AISection /></MemoryRouter>);
    expect(await screen.findByText("Local Ollama")).toBeInTheDocument();
    expect(screen.getByText("Stored (encrypted)")).toBeInTheDocument();
    expect((await screen.findAllByText("Sheets")).length).toBe(2); // per-app row + usage row
    fireEvent.click(screen.getByRole("button", { name: "Test connection" }));
    expect(await screen.findByText("OK · llama3.1:8b · 42 ms")).toBeInTheDocument();
    fireEvent.click(screen.getByLabelText("Assistant in Docs"));
    fireEvent.click(screen.getByRole("button", { name: "Save settings" }));
    await waitFor(() => expect(calls.some((c) => c.method === "PUT")).toBe(true));
    const put = calls.find((c) => c.method === "PUT")!;
    expect(put.body).toMatchObject({ disabled_apps: ["docs"] });
    expect(screen.getByRole("link", { name: /App controls/ })).toHaveAttribute("href", "/admin/controls");
  });

  it("saves the kill switch at once", async () => {
    render(<MemoryRouter><AISection /></MemoryRouter>);
    fireEvent.click(await screen.findByLabelText("Kill switch"));
    await waitFor(() => expect(calls.find((c) => c.method === "PUT")?.body).toMatchObject({ kill_switch: true }));
  });

  it("sends a new key only when typed", async () => {
    render(<MemoryRouter><AISection /></MemoryRouter>);
    fireEvent.click(await screen.findByLabelText("Edit"));
    fireEvent.click(await screen.findByRole("button", { name: "Save" }));
    await waitFor(() => expect(calls.some((c) => c.method === "PUT" && c.url.includes(`/providers/${PID}`))).toBe(true));
    const put = calls.find((c) => c.method === "PUT" && c.url.includes("/providers/"))!;
    expect(put.body).not.toHaveProperty("api_key");
  });
});
