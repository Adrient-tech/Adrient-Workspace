import { describe, it, expect } from "vitest";
import { buildItems, greeting, matchRank, prefersAsk } from "./paletteLogic";
import { QUICK_ACTIONS } from "./quickActions";
import type { AppTile } from "../../catalog/apps";
import type { SearchGroup } from "../../api/types";

const app = (id: string, name: string, blurb = ""): AppTile => ({ id, name, blurb, accentColor: "#000000", phase: 1, comingSoon: false, iconName: id });
const APPS = [app("drive", "Drive", "Files"), app("docs", "Docs", "Documents"), app("mail", "Mail", "Email"), app("sheets", "Sheets"), app("meet", "Meet")];

describe("paletteLogic", () => {
  it("ranks prefix over substring and rejects non-matches", () => {
    expect(matchRank("dri", "Drive")).toBe(0);
    expect(matchRank("ive", "Drive")).toBe(1);
    expect(matchRank("zzz", "Drive")).toBe(-1);
    expect(matchRank("", "Drive")).toBe(0);
    expect(matchRank("new", "Start a New thing")).toBe(0);
  });

  it("treats long or question-shaped input as an Ask", () => {
    expect(prefersAsk("what did finance approve last week")).toBe(true);
    expect(prefersAsk("budget?")).toBe(true);
    expect(prefersAsk("budget")).toBe(false);
  });

  it("empty query lists apps and quick actions only for apps that are on", () => {
    const items = buildItems({ query: "", apps: APPS.filter((a) => a.id !== "meet"), pinned: ["mail"], actions: QUICK_ACTIONS, groups: [], people: [] });
    expect(items[0].target).toBe("mail"); // pinned first
    const actions = items.filter((i) => i.kind === "action").map((i) => i.target);
    expect(actions).toContain("new-doc");
    expect(actions).not.toContain("new-meeting"); // Meet is off
    expect(actions).not.toContain("new-invoice"); // Invoicing not in the list
    expect(items.some((i) => i.kind === "ask")).toBe(false);
  });

  it("a lookup puts matches first and Ask last; a question puts Ask first", () => {
    const groups: SearchGroup[] = [{ type: "SEARCH_RESULT_TYPE_DOCS" as never, results: [{ type: "SEARCH_RESULT_TYPE_DOCS" as never, id: "1", title: "Doc plan", snippet: "", url: "/docs/d/1" }] }];
    const people = [{ id: "u1", name: "Docia Lee", email: "docia@x.io" }];
    const items = buildItems({ query: "doc", apps: APPS, pinned: [], actions: QUICK_ACTIONS, groups, people });
    expect(items[0].kind).toBe("app");
    expect(items[items.length - 1].kind).toBe("ask");
    expect(items.map((i) => i.kind)).toEqual(expect.arrayContaining(["app", "action", "file", "person", "ask"]));
    const q = buildItems({ query: "what is the status of the Acme deal?", apps: APPS, pinned: [], actions: QUICK_ACTIONS, groups: [], people: [] });
    expect(q[0].kind).toBe("ask");
    expect(q[0].target).toBe("what is the status of the Acme deal?");
  });

  it("falls back to Ask alone when nothing matches", () => {
    const items = buildItems({ query: "qqqq", apps: APPS, pinned: [], actions: QUICK_ACTIONS, groups: [], people: [] });
    expect(items).toHaveLength(1);
    expect(items[0].kind).toBe("ask");
  });

  it("greets by time of day and first name", () => {
    expect(greeting(new Date(2026, 0, 1, 9), "Asha Rao")).toBe("Good morning, Asha");
    expect(greeting(new Date(2026, 0, 1, 14), "")).toBe("Good afternoon");
    expect(greeting(new Date(2026, 0, 1, 20), "Sam")).toBe("Good evening, Sam");
  });
});
