import { describe, expect, it } from "vitest";
import {
  applyIncoming,
  channelLabel,
  continuesRun,
  foldReactions,
  groupSidebar,
  parsePath,
  presets,
  shouldNotify,
  totals,
  viewPath,
} from "./lib";
import type { TChannel, TMessage } from "./types";

const ch = (o: Partial<TChannel>): TChannel => ({
  id: "c1", kind: "public", name: "general", topic: "", description: "", archived: false,
  created_by: "", huddle_code: "", member_ids: ["me"], last_message_at: "", created_at: "2026-01-01T00:00:00Z",
  unread_count: 0, mention_count: 0, starred: false, muted: false, section_id: "", is_member: true, ...o,
});
const msg = (o: Partial<TMessage>): TMessage => ({
  id: "m1", channel_id: "c1", org_id: "", sender_id: "u2", sender_name: "U2", body: "hi", reactions: "{}",
  sent_at: "2026-01-01T10:00:00Z", ...o,
});
const people = {
  me: { id: "me", name: "Me Self", email: "", title: "", status_text: "", status_emoji: "" },
  u2: { id: "u2", name: "Asha Rao", email: "", title: "", status_text: "", status_emoji: "" },
  u3: { id: "u3", name: "Bob Lee", email: "", title: "", status_text: "", status_emoji: "" },
};

describe("sidebar", () => {
  it("groups starred, custom, channels and DMs without duplicates", () => {
    const list = [
      ch({ id: "a", name: "zeta" }),
      ch({ id: "b", name: "alpha", starred: true }),
      ch({ id: "c", name: "proj", section_id: "s1" }),
      ch({ id: "d", kind: "dm", name: "", member_ids: ["me", "u2"], last_message_at: "2026-02-01T00:00:00Z" }),
      ch({ id: "e", name: "old", archived: true }),
      ch({ id: "f", name: "notmine", is_member: false }),
    ];
    const g = groupSidebar(list, [{ id: "s1", name: "Projects", position: 1, collapsed: false }], "me", people);
    expect(g.map((x) => x.key)).toEqual(["starred", "s:s1", "channels", "dms"]);
    expect(g[0].channels.map((c) => c.id)).toEqual(["b"]);
    expect(g[1].channels.map((c) => c.id)).toEqual(["c"]);
    expect(g[2].channels.map((c) => c.id)).toEqual(["a"]);
    expect(g[3].channels.map((c) => c.id)).toEqual(["d"]);
  });
  it("labels DMs by the other person and self-DM as you", () => {
    expect(channelLabel(ch({ kind: "dm", member_ids: ["me", "u2"] }), "me", people)).toBe("Asha");
    expect(channelLabel(ch({ kind: "dm", member_ids: ["me"] }), "me", people)).toBe("Me Self (you)");
    expect(channelLabel(ch({ kind: "group", name: "", member_ids: ["me", "u2", "u3"] }), "me", people)).toBe("Asha, Bob");
  });
});

describe("unread + notifications", () => {
  it("increments unread and mention counts only for others' messages when not viewing", () => {
    const m = msg({ mention_ids: ["me"] });
    const out = applyIncoming([ch({})], m, "me", false)[0];
    expect([out.unread_count, out.mention_count]).toEqual([1, 1]);
    const seen = applyIncoming([ch({})], m, "me", true)[0];
    expect([seen.unread_count, seen.mention_count]).toEqual([0, 0]);
    const own = applyIncoming([ch({})], msg({ sender_id: "me" }), "me", false)[0];
    expect(own.unread_count).toBe(0);
    const reply = applyIncoming([ch({})], msg({ parent_id: "p" }), "me", false)[0];
    expect(reply.unread_count).toBe(0);
  });
  it("totals ignore muted unread but keep mentions", () => {
    const t = totals([ch({ unread_count: 3, mention_count: 1, muted: true }), ch({ id: "x", unread_count: 2 })]);
    expect(t).toEqual({ unread: 2, mentions: 1 });
  });
  it("shouldNotify requires being in notify list and respects mute", () => {
    expect(shouldNotify(["me"], msg({}), ch({}), "me")).toBe(true);
    expect(shouldNotify([], msg({}), ch({}), "me")).toBe(false);
    expect(shouldNotify(["me"], msg({}), ch({ muted: true }), "me")).toBe(false);
    expect(shouldNotify(["me"], msg({ mention_ids: ["me"] }), ch({ muted: true }), "me")).toBe(true);
    expect(shouldNotify(["me"], msg({ sender_id: "me" }), ch({}), "me")).toBe(false);
  });
});

describe("misc", () => {
  it("folds reactions with the me flag", () => {
    const m = foldReactions(msg({}), [{ emoji: "👍", user_ids: ["me", "u2"] }, { emoji: "🎉", user_ids: ["u2"] }], "me");
    expect(m.reaction_details).toEqual([{ emoji: "👍", count: 2, me: true }, { emoji: "🎉", count: 1, me: false }]);
  });
  it("round-trips permalinks", () => {
    expect(parsePath("/chat/c/abc/m/def")).toEqual({ kind: "channel", id: "abc", msgId: "def" });
    expect(parsePath("/chat/c/abc")).toEqual({ kind: "channel", id: "abc" });
    expect(parsePath("/chat")).toBeNull();
    expect(parsePath("/chat/search", "?q=hi")).toEqual({ kind: "search", q: "hi" });
    expect(viewPath({ kind: "channel", id: "abc", msgId: "def" })).toBe("/chat/c/abc/m/def");
    expect(viewPath({ kind: "later" })).toBe("/chat/later");
  });
  it("groups runs by sender within 5 minutes", () => {
    const a = msg({});
    expect(continuesRun(a, msg({ id: "2", sent_at: "2026-01-01T10:03:00Z" }))).toBe(true);
    expect(continuesRun(a, msg({ id: "3", sent_at: "2026-01-01T10:09:00Z" }))).toBe(false);
    expect(continuesRun(a, msg({ id: "4", sender_id: "u3" }))).toBe(false);
  });
  it("offers future presets", () => {
    const now = new Date("2026-03-04T12:00:00");
    for (const p of presets(now)) expect(p.at.getTime()).toBeGreaterThan(now.getTime());
  });
});
