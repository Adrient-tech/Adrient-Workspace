import { describe, it, expect, vi } from "vitest";
import { actionHref, availableActions, runAction, type ActionDeps } from "./personActions";

const p = { id: "u1", name: "Maya Rao", email: "maya@x.test" };

function deps(over: Partial<ActionDeps> = {}): ActionDeps {
  return {
    navigate: vi.fn(),
    pathname: "/drive",
    createCall: vi.fn(async () => "/meet/abc-defg-hij?return=%2Fchat"),
    openWindow: vi.fn(),
    dispatch: vi.fn(),
    ...over,
  };
}

describe("actionHref", () => {
  it("builds app URLs", () => {
    expect(actionHref("chat", p)).toBe("/chat?dm=u1");
    expect(actionHref("email", p)).toBe("/mail?compose=1&to=maya%40x.test");
    expect(actionHref("schedule", p)).toBe("/calendar?new=1&attendee=maya%40x.test");
    expect(actionHref("call", p)).toBeNull();
  });
  it("returns null without the needed field", () => {
    expect(actionHref("chat", { email: "a@b.c" })).toBeNull();
    expect(actionHref("email", { id: "u1" })).toBeNull();
  });
});

describe("availableActions", () => {
  it("offers all four for an org user", () => {
    expect(availableActions(p)).toEqual(["chat", "call", "email", "schedule"]);
  });
  it("offers only email and schedule for an external address", () => {
    expect(availableActions({ email: "a@b.c" })).toEqual(["email", "schedule"]);
  });
  it("offers nothing for an empty target", () => {
    expect(availableActions({})).toEqual([]);
  });
});

describe("runAction", () => {
  it("navigates to the app when it is not open", async () => {
    const d = deps();
    await runAction("chat", p, d);
    expect(d.navigate).toHaveBeenCalledWith("/chat?dm=u1");
    expect(d.dispatch).not.toHaveBeenCalled();
  });
  it("dispatches an event when the app is already open", async () => {
    const d = deps({ pathname: "/mail/inbox" });
    await runAction("email", p, d);
    expect(d.navigate).not.toHaveBeenCalled();
    expect(d.dispatch).toHaveBeenCalledWith("adrient:person-action", {
      action: "email",
      id: "u1",
      email: "maya@x.test",
    });
  });
  it("rings the person and opens the call in the same tab", async () => {
    const d = deps();
    await runAction("call", p, d);
    expect(d.createCall).toHaveBeenCalledWith("Call with Maya Rao", "u1");
    expect(d.navigate).toHaveBeenCalledWith("/meet/abc-defg-hij?return=%2Fchat");
    expect(d.openWindow).not.toHaveBeenCalled();
  });
  it("ignores an action the target cannot do", async () => {
    const d = deps();
    await runAction("chat", { email: "a@b.c" }, d);
    expect(d.navigate).not.toHaveBeenCalled();
  });
});
