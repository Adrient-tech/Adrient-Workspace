import { describe, expect, it } from "vitest";
import {
  closeTab,
  docToMarkup,
  markupToHtml,
  fillTemplate,
  keyAction,
  mentionQuery,
  mentionToken,
  openTab,
  parseBlocks,
  parseInline,
  safeHref,
  visibleWindow,
  wrapSelection,
  MAX_TABS,
} from "./workspaceLogic";

const press = (key: string, typing = false, pendingG = false) => keyAction({ pendingG }, { key, typing });

describe("keyboard shortcuts", () => {
  it("maps j/k/r/n/a", () => {
    expect(press("j").action).toEqual({ type: "move", delta: 1 });
    expect(press("k").action).toEqual({ type: "move", delta: -1 });
    expect(press("r").action).toEqual({ type: "reply" });
    expect(press("n").action).toEqual({ type: "note" });
    expect(press("a").action).toEqual({ type: "assign" });
  });
  it("ignores plain keys while typing", () => {
    expect(press("r", true).action).toEqual({ type: "none" });
    expect(press("j", true).action).toEqual({ type: "none" });
  });
  it("opens the command bar with Ctrl+K even while typing", () => {
    expect(keyAction({ pendingG: false }, { key: "K", ctrl: true, shift: true, typing: true }).action).toEqual({ type: "palette" });
    expect(keyAction({ pendingG: false }, { key: "K", meta: true, shift: true, typing: false }).action).toEqual({ type: "palette" });
  });
  it("handles g then a view key", () => {
    const first = press("g");
    expect(first.next.pendingG).toBe(true);
    expect(keyAction(first.next, { key: "u", typing: false }).action).toEqual({ type: "goto", view: "unassigned" });
    expect(keyAction(first.next, { key: "q", typing: false }).action).toEqual({ type: "none" });
    expect(keyAction(first.next, { key: "q", typing: false }).next.pendingG).toBe(false);
  });
  it("leaves plain Ctrl+K to the app-wide search", () => {
    expect(keyAction({ pendingG: false }, { key: "k", ctrl: true, typing: false }).action).toEqual({ type: "none" });
  });
  it("does not hijack browser shortcuts", () => {
    expect(keyAction({ pendingG: false }, { key: "r", ctrl: true, typing: false }).action).toEqual({ type: "none" });
  });
});

describe("list windowing", () => {
  it("renders only what is visible plus overscan", () => {
    const w = visibleWindow(100000, 64, 64 * 5000, 640, 5);
    expect(w.end - w.start).toBeLessThan(30);
    expect(w.padTop + (w.end - w.start) * 64 + w.padBottom).toBe(100000 * 64);
  });
  it("is empty for no rows and clamps at the end", () => {
    expect(visibleWindow(0, 64, 0, 500).end).toBe(0);
    const w = visibleWindow(20, 40, 5000, 400);
    expect(w.end).toBe(20);
    expect(w.padBottom).toBe(0);
  });
});

describe("tabs", () => {
  const t = (n: number) => ({ id: `t${n}`, ref: `HELP-${n}`, title: `Ticket ${n}` });
  it("opens each ticket once and caps the strip", () => {
    let tabs = [] as ReturnType<typeof t>[];
    for (let i = 1; i <= MAX_TABS + 3; i++) tabs = openTab(tabs, t(i));
    expect(tabs).toHaveLength(MAX_TABS);
    expect(tabs[tabs.length - 1].id).toBe(`t${MAX_TABS + 3}`);
    expect(openTab(tabs, t(MAX_TABS + 3))).toHaveLength(MAX_TABS);
  });
  it("moves focus to the neighbour on close", () => {
    const tabs = [t(1), t(2), t(3)];
    expect(closeTab(tabs, "t2", "t2")).toEqual({ tabs: [t(1), t(3)], active: "t3" });
    expect(closeTab(tabs, "t3", "t3").active).toBe("t2");
    expect(closeTab(tabs, "t1", "t3").active).toBe("t3");
    expect(closeTab([t(1)], "t1", "t1")).toEqual({ tabs: [], active: null });
  });
});

describe("safe formatting", () => {
  it("parses bold, italic, code, links and mentions", () => {
    const parts = parseInline("a **b** _c_ `d` [x](https://e.example/p) @[Priya N](user:123e4567-e89b-12d3-a456-426614174000)");
    expect(parts.map((p) => p.t)).toEqual(["text", "bold", "text", "italic", "text", "code", "text", "link", "text", "mention"]);
  });
  it("refuses dangerous link targets", () => {
    expect(safeHref("javascript:alert(1)")).toBeNull();
    expect(safeHref("data:text/html,x")).toBeNull();
    expect(safeHref("//evil.example")).toBeNull();
    expect(safeHref("https://ok.example/a?b=1")).toBe("https://ok.example/a?b=1");
    expect(safeHref("/tickets?ticket=1")).toBe("/tickets?ticket=1");
    const p = parseInline("[click](javascript:alert(1))");
    expect(p.some((x) => x.t === "link")).toBe(false);
  });
  it("never yields markup: html stays text", () => {
    const p = parseInline("<img src=x onerror=alert(1)>");
    expect(p).toEqual([{ t: "text", v: "<img src=x onerror=alert(1)>" }]);
  });
  it("builds lists and paragraphs", () => {
    const b = parseBlocks("Hello\n\n- one\n- two\n\n1. first\n2. second");
    expect(b.map((x) => x.kind)).toEqual(["p", "ul", "ol"]);
  });
  it("wraps a selection", () => {
    expect(wrapSelection("hello world", 6, 11, "**")).toEqual({ value: "hello **world**", start: 8, end: 13 });
    expect(wrapSelection("x", 1, 1, "_").value).toBe("x_text_");
  });
});

describe("mentions and templates", () => {
  it("detects an open mention before the caret", () => {
    expect(mentionQuery("ping @pri", 9)).toEqual({ query: "pri", from: 5 });
    expect(mentionQuery("mail me@x.com", 13)).toBeNull();
    expect(mentionQuery("@", 1)).toEqual({ query: "", from: 0 });
  });
  it("builds a token the server understands", () => {
    expect(mentionToken("Priya", "123e4567-e89b-12d3-a456-426614174000")).toBe("@[Priya](user:123e4567-e89b-12d3-a456-426614174000)");
  });
  it("fills known placeholders and keeps unknown ones", () => {
    expect(fillTemplate("Hi {{requester.first_name}}, {{ticket.ref}} {{x.y}}", { "requester.first_name": "Ana", "ticket.ref": "HELP-1" })).toBe("Hi Ana, HELP-1 {{x.y}}");
  });
});

describe("editor markup round trip", () => {
  it("keeps snake_case words out of italics", () => {
    expect(parseInline("see file_name_here and _real_")[1]).toEqual({ t: "italic", v: "real" });
    expect(parseInline("file_name_here").every((p) => p.t === "text")).toBe(true);
  });
  it("renders markup to editor html without letting markup through", () => {
    const h = markupToHtml("Hi **Ana**, <script>x</script>\n\n- one\n- two\n\n> quoted");
    expect(h).toContain("<strong>Ana</strong>");
    expect(h).toContain("&lt;script&gt;");
    expect(h).not.toContain("<script>");
    expect(h).toContain("<ul><li><p>one</p></li>");
    expect(h).toContain("<blockquote>");
  });
  it("turns an editor document back into markup", () => {
    const doc = {
      type: "doc",
      content: [
        { type: "paragraph", content: [{ type: "text", text: "Hello " }, { type: "text", text: "Ana", marks: [{ type: "bold" }] }, { type: "hardBreak" }, { type: "text", text: "see " }, { type: "text", text: "docs", marks: [{ type: "link", attrs: { href: "https://x.example/a" } }] }] },
        { type: "bulletList", content: [{ type: "listItem", content: [{ type: "paragraph", content: [{ type: "text", text: "one" }] }] }, { type: "listItem", content: [{ type: "paragraph", content: [{ type: "text", text: "two" }] }] }] },
        { type: "blockquote", content: [{ type: "paragraph", content: [{ type: "text", text: "old text" }] }] },
        { type: "paragraph", content: [{ type: "ticketMention", attrs: { id: "123e4567-e89b-12d3-a456-426614174000", label: "Priya" } }, { type: "text", text: " please check" }] },
      ],
    };
    expect(docToMarkup(doc)).toBe("Hello **Ana**\nsee [docs](https://x.example/a)\n\n- one\n- two\n\n> old text\n\n@[Priya](user:123e4567-e89b-12d3-a456-426614174000) please check");
  });
  it("refuses unsafe links when serialising", () => {
    const md = docToMarkup({ type: "doc", content: [{ type: "paragraph", content: [{ type: "text", text: "x", marks: [{ type: "link", attrs: { href: "javascript:alert(1)" } }] }] }] });
    expect(md).toBe("x");
  });
  it("parses lists and quotes as blocks", () => {
    expect(parseBlocks("Hi **Ana**\n\n1. first\n2. second\n\n> quote").map((b) => b.kind)).toEqual(["p", "ol", "q"]);
  });
});
