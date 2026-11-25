import { describe, it, expect, vi, beforeEach } from "vitest";
import {
  render,
  screen,
  waitFor,
  fireEvent,
  within,
} from "@testing-library/react";
import MigrationWizard from "./MigrationWizard";
import {
  wizardApi,
  type Provider,
  type Connection,
  type Run,
} from "./wizardApi";

const google: Provider = {
  id: "google",
  name: "Google Workspace / Gmail",
  short: "Google",
  auth: "oauth",
  scopes: ["me", "org"],
  kinds: ["users", "groups", "files", "mail", "calendar", "contacts", "chat"],
  needs_app: true,
  configured: false,
  notes: "",
  setup_doc: "docs/migrating-from-google.md",
  fields: null,
};
const nextcloud: Provider = {
  id: "nextcloud",
  name: "Nextcloud",
  short: "Nextcloud",
  auth: "password",
  scopes: ["me"],
  kinds: ["files", "calendar", "contacts"],
  needs_app: false,
  configured: true,
  notes: "WebDAV",
  setup_doc: "",
  fields: [
    {
      key: "server",
      label: "Nextcloud address",
      type: "url",
      required: true,
      secret: false,
    },
    {
      key: "username",
      label: "Username",
      type: "text",
      required: true,
      secret: false,
    },
    {
      key: "password",
      label: "App password",
      type: "password",
      required: true,
      secret: true,
    },
  ],
};
const conn: Connection = {
  id: "c1",
  provider: "nextcloud",
  label: "Nextcloud (ann)",
  scope: "me",
  config: { server: "https://cloud.example", username: "ann" },
  status: "ready",
  created_at: "2026-11-18T00:00:00Z",
  mine: true,
};
const doneRun: Run = {
  id: "r1",
  connection_id: "c1",
  kinds: ["files"],
  dry_run: true,
  status: "done",
  counters: {
    files: { seen: 4, imported: 3, skipped: 0, failed: 1 },
    chat: {
      seen: 0,
      imported: 0,
      skipped: 0,
      failed: 0,
      note: "not supported by this provider",
    },
  },
  message: "dry run: nothing was written",
  created_at: "2026-11-18T00:00:00Z",
  updated_at: "2026-11-18T00:00:00Z",
  error_count: 1,
};

beforeEach(() => vi.restoreAllMocks());

describe("MigrationWizard", () => {
  it("marks OAuth providers as not configured and disables Connect for non-admins", async () => {
    vi.spyOn(wizardApi, "providers").mockResolvedValue({
      providers: [google, nextcloud],
      is_admin: false,
      redirect_url: "",
    });
    vi.spyOn(wizardApi, "connections").mockResolvedValue([]);
    render(<MigrationWizard />);
    const card = await screen.findByTestId("provider-google");
    expect(card.textContent).toContain("Not configured");
    expect(card.textContent).toContain("Ask an administrator");
    expect(
      within(card).getByRole("button", { name: "Connect" }),
    ).toBeDisabled();
    expect(
      within(card).queryByRole("button", { name: "Provider settings" }),
    ).toBeNull();
    expect(
      within(screen.getByTestId("provider-nextcloud")).getByRole("button", {
        name: "Connect",
      }),
    ).not.toBeDisabled();
  });

  it("admins see Provider settings", async () => {
    vi.spyOn(wizardApi, "providers").mockResolvedValue({
      providers: [google],
      is_admin: true,
      redirect_url: "",
    });
    vi.spyOn(wizardApi, "connections").mockResolvedValue([]);
    render(<MigrationWizard />);
    const card = await screen.findByTestId("provider-google");
    expect(
      within(card).getByRole("button", { name: "Provider settings" }),
    ).toBeInTheDocument();
  });

  it("connects a password provider with the form fields and shows server errors", async () => {
    vi.spyOn(wizardApi, "providers").mockResolvedValue({
      providers: [nextcloud],
      is_admin: false,
      redirect_url: "",
    });
    vi.spyOn(wizardApi, "connections").mockResolvedValue([]);
    const create = vi
      .spyOn(wizardApi, "createConnection")
      .mockRejectedValue(new Error("connection test failed: HTTP 401"));
    render(<MigrationWizard />);
    fireEvent.click(
      within(await screen.findByTestId("provider-nextcloud")).getByRole(
        "button",
        { name: "Connect" },
      ),
    );
    const submit = await screen.findByRole("button", {
      name: "Test and connect",
    });
    expect(submit).toBeDisabled();
    fireEvent.change(screen.getByLabelText("Nextcloud address"), {
      target: { value: "https://cloud.example" },
    });
    fireEvent.change(screen.getByLabelText("Username"), {
      target: { value: "ann" },
    });
    fireEvent.change(screen.getByLabelText("App password"), {
      target: { value: "x" },
    });
    fireEvent.click(submit);
    await waitFor(() =>
      expect(screen.getByTestId("connect-error").textContent).toContain("401"),
    );
    expect(create).toHaveBeenCalledWith({
      provider: "nextcloud",
      scope: "me",
      config: {
        server: "https://cloud.example",
        username: "ann",
        password: "x",
      },
    });
  });

  it("starts a dry run and shows counters, notes and the error report", async () => {
    vi.spyOn(wizardApi, "providers").mockResolvedValue({
      providers: [nextcloud],
      is_admin: false,
      redirect_url: "",
    });
    vi.spyOn(wizardApi, "connections").mockResolvedValue([conn]);
    const runs = vi
      .spyOn(wizardApi, "runs")
      .mockResolvedValueOnce([])
      .mockResolvedValue([doneRun]);
    const start = vi.spyOn(wizardApi, "startRun").mockResolvedValue(doneRun);
    vi.spyOn(wizardApi, "errors").mockResolvedValue([
      {
        kind: "files",
        remote_id: "x",
        label: "big.iso",
        message: "too large",
        at: "",
      },
    ]);
    render(<MigrationWizard />);
    fireEvent.click(
      await screen.findByRole("button", { name: "Start dry run" }),
    );
    await waitFor(() =>
      expect(start).toHaveBeenCalledWith(
        "c1",
        ["files", "calendar", "contacts"],
        true,
      ),
    );
    const counter = await screen.findByTestId("counter-files");
    expect(counter.textContent).toBe("Drive files4301");
    expect(screen.getByTestId("counter-chat").textContent).toContain(
      "not supported",
    );
    fireEvent.click(screen.getByRole("button", { name: "1 problem" }));
    await screen.findByText("too large");
    expect(
      screen.getByRole("link", { name: "Download CSV" }).getAttribute("href"),
    ).toBe("/api/v1/import/wizard/runs/r1/errors?format=csv");
    expect(runs).toHaveBeenCalled();
  });
});
