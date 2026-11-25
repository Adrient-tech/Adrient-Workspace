import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { ControlsSection } from "./ControlsSection";
import { ExternalSection } from "./ExternalSection";
import { controlsApi, externalApi, type ControlDef } from "./controlsApi";

const sharing: ControlDef = {
  key: "drive.external_sharing", app: "drive", label: "External sharing", help: "h", type: "enum",
  enum: ["off", "domains", "anyone"], default: "domains",
};
const dl: ControlDef = {
  key: "drive.guest_download", app: "drive", label: "Guests can download files", help: "h", type: "bool",
  default: true, type_defaults: { guest: false },
};

beforeEach(() => {
  vi.restoreAllMocks();
});

describe("ControlsSection", () => {
  it("shows defaults and existing rules, and saves a user type rule", async () => {
    vi.spyOn(controlsApi, "list").mockResolvedValue({
      catalog: [sharing, dl],
      rules: [{ id: "r1", control: "drive.guest_download", value: true, subject_type: "user_type", subject_id: "guest", priority: 100, effect_mode: "set" }],
      user_types: ["member", "guest"],
    });
    const create = vi.spyOn(controlsApi, "create").mockResolvedValue({ ok: true, id: "r2" });
    render(<ControlsSection />);
    await waitFor(() => expect(screen.getByTestId("control-drive.external_sharing")).toBeTruthy());
    expect(screen.getByTestId("rule-drive.guest_download-r1").textContent).toContain("User type: guest");
    expect(screen.getByText(/guests Off/)).toBeTruthy();
    fireEvent.click(screen.getByTestId("control-add-drive.external_sharing"));
    fireEvent.change(screen.getByTestId("rule-ips"), { target: { value: "10.0.0.0/8" } });
    fireEvent.click(screen.getByTestId("rule-save"));
    await waitFor(() => expect(create).toHaveBeenCalled());
    expect(create.mock.calls[0][0]).toMatchObject({
      control: "drive.external_sharing", value: "off", subject_type: "user_type", subject_id: "guest",
      condition: { ip_cidrs: ["10.0.0.0/8"] }, effect_mode: "set",
    });
  });

  it("removes a rule", async () => {
    vi.spyOn(controlsApi, "list").mockResolvedValue({
      catalog: [dl],
      rules: [{ id: "r1", control: "drive.guest_download", value: true, subject_type: "everyone", subject_id: "", priority: 100, effect_mode: "set" }],
      user_types: [],
    });
    const remove = vi.spyOn(controlsApi, "remove").mockResolvedValue({ ok: true });
    render(<ControlsSection />);
    const chip = await screen.findByTestId("rule-drive.guest_download-r1");
    fireEvent.click(chip.querySelector("button")!);
    await waitFor(() => expect(remove).toHaveBeenCalledWith("r1"));
  });
});

describe("ExternalSection", () => {
  it("blocks a partner domain", async () => {
    vi.spyOn(externalApi, "policy").mockResolvedValue({
      guest_invites: "admins", domain_mode: "allow_all", guest_visibility: "limited",
      guest_default_expiry_days: 30, external_member_default_expiry_days: 90, require_mfa_for_guests: true,
    });
    vi.spyOn(externalApi, "partners").mockResolvedValue({ partners: [{ domain: "ok.com", effect: "allow", note: "" }] });
    const add = vi.spyOn(externalApi, "addPartner").mockResolvedValue({ ok: true });
    render(<ExternalSection />);
    await screen.findByTestId("partner-ok.com");
    fireEvent.change(screen.getByTestId("partner-domain"), { target: { value: "evil.com" } });
    fireEvent.click(screen.getByTestId("partner-add"));
    await waitFor(() => expect(add).toHaveBeenCalledWith({ domain: "evil.com", effect: "block", note: "" }));
  });
});
