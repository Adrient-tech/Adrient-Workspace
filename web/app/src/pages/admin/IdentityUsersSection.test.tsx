import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { IdentityUsersSection } from "./IdentityUsersSection";
import { DomainsSection } from "./DomainsSection";
import { identityApi } from "./identityApi";

const user = (over: Record<string, unknown> = {}) => ({
  id: "u1", email: "g@x.example", display_name: "Guest G", upn: "g@x.example", user_type: "guest",
  status: "active", expires_at: null, sponsor_email: "a@acme.test", blocked_reason: "",
  last_sign_in_at: null, guest_apps: ["drive"], ...over,
});

beforeEach(() => {
  vi.restoreAllMocks();
});

describe("IdentityUsersSection", () => {
  it("lists users with type chips, filters via the API, and blocks", async () => {
    const users = vi.spyOn(identityApi, "users").mockResolvedValue({ users: [user()] } as never);
    vi.spyOn(identityApi, "invitations").mockResolvedValue({ invitations: [] });
    const block = vi.spyOn(identityApi, "block").mockResolvedValue({ ok: true });
    render(
      <IdentityUsersSection />,
    );
    await waitFor(() => expect(screen.getByTestId("urow-g@x.example")).toBeTruthy());
    expect(screen.getAllByText("Guest").length).toBeGreaterThan(0);
    expect(users).toHaveBeenCalledWith({ type: "", status: "", q: "" });
    fireEvent.click(screen.getByRole("button", { name: "Block" }));
    await waitFor(() => expect(block).toHaveBeenCalledWith("u1", "blocked by admin"));
  });

  it("opens the invite dialog and sends the chosen type", async () => {
    vi.spyOn(identityApi, "users").mockResolvedValue({ users: [] });
    vi.spyOn(identityApi, "invitations").mockResolvedValue({ invitations: [] });
    const invite = vi.spyOn(identityApi, "invite").mockResolvedValue({
      invitation: {} as never, accept_url: "http://x/accept-invite?code=abc", email_sent: false,
    });
    render(
      <IdentityUsersSection />,
    );
    fireEvent.click(await screen.findByText("Invite"));
    fireEvent.change(await screen.findByLabelText("Invite email"), { target: { value: "p@partner.example" } });
    fireEvent.click(screen.getByText("Send invitation"));
    await waitFor(() => expect(invite).toHaveBeenCalled());
    expect(invite.mock.calls[0][0]).toMatchObject({ email: "p@partner.example", user_type: "guest", app_ids: ["drive"] });
    expect(await screen.findByTestId("invite-link")).toBeTruthy();
  });
});

describe("DomainsSection", () => {
  it("shows the TXT record for a pending domain and verifies", async () => {
    vi.spyOn(identityApi, "domains").mockResolvedValue({
      domains: [{ id: "d1", domain: "acme.test", status: "pending", verify_host: "_adrient-challenge.acme.test",
        verify_txt: "adrient-verify=abc", is_primary: false, allow_signup: false, auto_join: false, require_approval: true, user_count: 0 }],
    });
    vi.spyOn(identityApi, "settings").mockResolvedValue({ signup_mode: "invite_only", allowed_signin_domains: [], personal_orgs_allowed: true });
    const verify = vi.spyOn(identityApi, "verifyDomain").mockResolvedValue({ verified: true, domain: {} as never });
    render(
      <DomainsSection />,
    );
    expect((await screen.findByTestId("txt-value")).textContent).toBe("adrient-verify=abc");
    fireEvent.click(screen.getByText("Verify"));
    await waitFor(() => expect(verify).toHaveBeenCalledWith("d1"));
  });
});
