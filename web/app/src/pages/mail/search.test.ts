import { describe, expect, it } from "vitest";
import { applyQuickFilters, filterThreads, hasOperators, parseSearch, sortThreads } from "./search";
import type { MailThread } from "./types";

function th(over: Partial<MailThread["latest"]> & { unread?: boolean; starred?: boolean; labels?: string[] }): MailThread {
  const { unread, starred, labels, ...m } = over;
  return {
    thread_id: String(Math.random()),
    message_count: 1,
    any_unread: !!unread,
    starred: !!starred,
    labels: labels ?? [],
    participants: [],
    latest: {
      id: "m", thread_id: "t", folder: "inbox", from_addr: "a@x.com", from_name: "", to_addrs: ["me@x.com"], cc_addrs: [],
      subject: "Hello", body: "", snippet: "", is_read: true, starred: !!starred, labels: labels ?? [],
      sent_at: "2026-06-15T10:00:00Z", attachments: [], ...m,
    },
  };
}

describe("parseSearch", () => {
  it("splits operators from free text", () => {
    const p = parseSearch('from:boss has:attachment before:2026-07-01 after:2026-06-01 label:Work invoice "q3 plan"');
    expect(p.from).toEqual(["boss"]);
    expect(p.hasAttachment).toBe(true);
    expect(p.label).toEqual(["work"]);
    expect(p.before?.getMonth()).toBe(6);
    expect(p.text).toBe("invoice q3 plan");
  });
  it("keeps quoted operator values and rejects bad dates as text", () => {
    const p = parseSearch('subject:"big news" before:soon');
    expect(p.subject).toEqual(["big news"]);
    expect(p.text).toBe("before:soon");
    expect(hasOperators(parseSearch("plain words"))).toBe(false);
  });
});

describe("filterThreads", () => {
  const a = th({ from_addr: "boss@x.com", attachments: [{ id: "1", filename: "a.pdf", content_type: "application/pdf", size: 1 }] });
  const b = th({ from_addr: "bob@x.com", unread: true, sent_at: "2026-05-01T10:00:00Z" });
  it("applies from and has:attachment", () => {
    expect(filterThreads([a, b], parseSearch("from:boss has:attachment"))).toEqual([a]);
  });
  it("applies before/after", () => {
    expect(filterThreads([a, b], parseSearch("before:2026-06-01"))).toEqual([b]);
    expect(filterThreads([a, b], parseSearch("after:2026-06-01"))).toEqual([a]);
  });
  it("applies is:unread and label", () => {
    expect(filterThreads([a, b], parseSearch("is:unread"))).toEqual([b]);
    const l = th({ labels: ["Work"] });
    expect(filterThreads([a, l], parseSearch("label:work"))).toEqual([l]);
  });
});

describe("quick filters and sort", () => {
  const a = th({ unread: true, to_addrs: ["me@x.com"] });
  const b = th({ starred: true, to_addrs: ["other@x.com"], body: "ping @me please" });
  it("filters To me / Flagged / Mentions", () => {
    expect(applyQuickFilters([a, b], new Set(["tome"]), "me@x.com")).toEqual([a]);
    expect(applyQuickFilters([a, b], new Set(["flagged"]), "me@x.com")).toEqual([b]);
    expect(applyQuickFilters([a, b], new Set(["mentions"]), "me@x.com")).toEqual([b]);
    expect(applyQuickFilters([a, b], new Set(), "me@x.com")).toEqual([a, b]);
  });
  it("sorts pinned first, then by key", () => {
    const old = th({ sent_at: "2026-01-01T00:00:00Z" });
    const nw = th({ sent_at: "2026-09-01T00:00:00Z" });
    expect(sortThreads([nw, old], "date-asc", () => false)).toEqual([old, nw]);
    expect(sortThreads([nw, old], "date-desc", (t) => t === old)).toEqual([old, nw]);
  });
});
