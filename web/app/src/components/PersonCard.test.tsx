import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor, cleanup } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { PersonCard } from "./PersonCard";

const profile = {
  person: {
    id: "u1", name: "Maya Rao", email: "maya@x.test", job_title: "Engineer", phone: "+91 99",
    location: "Pune", employee_id: "", start_date: "", department_id: "d1", department_name: "Eng",
    manager_id: "u2", manager_name: "Sam Lee",
  },
  manager: { id: "u2", name: "Sam Lee" },
  manager_chain: [], direct_reports: [],
  department: { id: "d1", name: "Eng" },
  avatar_url: "",
};

function Where() {
  return <span data-testid="where">{useLocation().pathname + useLocation().search}</span>;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function mount(props: Omit<Parameters<typeof PersonCard>[0], "children">) {
  render(
    <MemoryRouter initialEntries={["/drive"]}>
      <PersonCard {...props}>
        <span>Maya</span>
      </PersonCard>
      <Where />
    </MemoryRouter>,
  );
}

describe("PersonCard", () => {
  it("shows profile details and the four action buttons", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify(profile), { status: 200 })));
    mount({ userId: "u1", name: "Maya Rao" });
    fireEvent.click(screen.getByTestId("person-card-trigger"));
    await waitFor(() => expect(screen.getByTestId("person-card-title").textContent).toBe("Engineer"));
    expect(screen.getByTestId("person-card-department").textContent).toBe("Eng");
    expect(screen.getByTestId("person-card-phone").textContent).toBe("+91 99");
    expect(screen.getByTestId("person-card-manager").textContent).toContain("Sam Lee");
    for (const a of ["chat", "call", "email", "schedule"]) {
      expect(screen.getByTestId(`person-card-${a}`)).toBeTruthy();
    }
  });

  it("Chat button navigates to the DM URL", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify(profile), { status: 200 })));
    mount({ userId: "u1" });
    fireEvent.click(screen.getByTestId("person-card-trigger"));
    const chat = await screen.findByTestId("person-card-chat");
    fireEvent.click(chat);
    await waitFor(() => expect(screen.getByTestId("where").textContent).toBe("/chat?dm=u1"));
  });

  it("resolves by email and falls back to Email/Schedule for outsiders", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ people: [] }), { status: 200 })));
    mount({ email: "ext@other.test", name: "Ext" });
    fireEvent.click(screen.getByTestId("person-card-trigger"));
    await screen.findByTestId("person-card-email");
    expect(screen.queryByTestId("person-card-chat")).toBeNull();
    expect(screen.getByTestId("person-card-schedule")).toBeTruthy();
  });
});
