import { describe, expect, it } from "vitest";
import { firstUnreadIndex, isSlash, parseSlash } from "./slash";

describe("slash commands", () => {
  it("recognises commands, not paths", () => {
    expect(isSlash("/shrug")).toBe(true);
    expect(isSlash("/me waves")).toBe(true);
    expect(isSlash("/usr/bin")).toBe(false);
    expect(isSlash("see /shrug")).toBe(false);
    expect(parseSlash("hello")).toEqual({ kind: "none" });
  });
  it("builds messages", () => {
    expect(parseSlash("/shrug")).toEqual({ kind: "send", body: "¯\\_(ツ)_/¯" });
    expect(parseSlash("/shrug no idea")).toEqual({ kind: "send", body: "no idea ¯\\_(ツ)_/¯" });
    expect(parseSlash("/me waves")).toEqual({ kind: "send", body: "_waves_" });
    expect(parseSlash("/me")).toMatchObject({ kind: "error" });
  });
  it("channel actions", () => {
    expect(parseSlash("/topic Launch week")).toEqual({ kind: "topic", topic: "Launch week" });
    expect(parseSlash("/topic x", { isChannel: false })).toMatchObject({ kind: "error" });
    expect(parseSlash("/mute")).toEqual({ kind: "mute", muted: true });
    expect(parseSlash("/unmute")).toEqual({ kind: "mute", muted: false });
    expect(parseSlash("/huddle")).toEqual({ kind: "huddle" });
    expect(parseSlash("/nope")).toMatchObject({ kind: "error" });
    expect(parseSlash("/help")).toMatchObject({ kind: "help" });
  });
});

describe("firstUnreadIndex", () => {
  const msgs = [
    { sent_at: "2026-11-19T10:00:00Z", sender_id: "a" },
    { sent_at: "2026-11-19T11:00:00Z", sender_id: "me" },
    { sent_at: "2026-11-19T12:00:00Z", sender_id: "b" },
  ];
  it("finds the first message from someone else after the read time", () => {
    expect(firstUnreadIndex(msgs, "2026-11-19T10:30:00Z", "me")).toBe(2);
    expect(firstUnreadIndex(msgs, "2026-11-19 09:00:00+00", "me")).toBe(0);
    expect(firstUnreadIndex(msgs, "2026-11-19T13:00:00Z", "me")).toBe(-1);
    expect(firstUnreadIndex(msgs, "", "me")).toBe(-1);
  });
});
