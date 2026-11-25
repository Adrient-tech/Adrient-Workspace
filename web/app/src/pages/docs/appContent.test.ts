import { describe, expect, it } from "vitest";
import { appTableHtml, contactBlockHtml, filterContacts, meetingLinkHtml } from "./appContent";

describe("docs app content", () => {
  it("builds a table with escaped cells and a row cap", () => {
    const t = { title: "T", header: ["A<b>", "N"], rows: [["x&y", 1234.5], ["z", 2], ["w", 3]] as (string | number)[][] };
    const r = appTableHtml(t, 2);
    expect(r.shown).toBe(2);
    expect(r.total).toBe(3);
    expect(r.html).toContain("<th><p>A&lt;b&gt;</p></th>");
    expect(r.html).toContain("x&amp;y");
    expect(r.html).not.toContain("<p>w</p>");
  });
  it("builds a contact block, skipping empty lines", () => {
    const h = contactBlockHtml({ display_name: "Ann <A>", first_name: "", last_name: "", company: "Acme", job_title: "", emails: ["a@x.io"], phones: [], notes: "" });
    expect(h).toBe("<p>Ann &lt;A&gt;<br>Acme<br>a@x.io</p>");
    expect(contactBlockHtml({ display_name: "", first_name: "", last_name: "", company: "", job_title: "", emails: [], phones: [], notes: "" })).toBe("");
  });
  it("builds a meeting link paragraph", () => {
    expect(meetingLinkHtml("Sync", "http://h/meet/abc")).toContain('<a href="http://h/meet/abc">');
  });
  it("filters contacts", () => {
    const l = [{ display_name: "Ann", company: "Acme", emails: ["a@x.io"] }, { display_name: "Bob", company: "", emails: [] }];
    expect(filterContacts(l, "acme")).toHaveLength(1);
    expect(filterContacts(l, "")).toHaveLength(2);
  });
});
