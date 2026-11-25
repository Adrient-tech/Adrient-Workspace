import { describe, expect, it } from "vitest";
import { createContactTools, findDuplicates, type ContactRow, type ContactsAdapter } from "./tools";

const c = (id: string, name: string, o: Partial<ContactRow> = {}): ContactRow => ({ id, display_name: name, first_name: "", last_name: "", company: "", job_title: "", emails: [], phones: [], labels: [], notes: "", starred: false, ...o });

describe("findDuplicates", () => {
  it("groups by email, phone digits and full name", () => {
    const g = findDuplicates([c("1", "Ann Lee", { emails: ["a@x.io"] }), c("2", "A. Lee", { emails: ["A@x.io"] }), c("3", "Bob Ray", { phones: ["+91 98765 43210"] }), c("4", "Robert", { phones: ["098765-43210"] }), c("5", "Ann Lee"), c("6", "Solo")]);
    expect(g.map((x) => x.map((y) => y.id).sort())).toEqual([["1", "2", "5"], ["3", "4"]]);
  });
});

describe("contact tools", () => {
  const setup = () => {
    const rows = [c("1", "Ann Lee", { company: "Acme", notes: "old" }), c("2", "Ann Lee", { company: "Acme" })];
    const ad: ContactsAdapter = { all: async () => rows.map((r) => ({ ...r })), updateNotes: async (x, n) => { rows.find((r) => r.id === x.id)!.notes = n; }, selected: () => null };
    return { rows, tools: Object.fromEntries(createContactTools(ad).map((t) => [t.name, t])) };
  };
  it("finds contacts and reports duplicates without merging", async () => {
    const { tools } = setup();
    expect(((await tools.contacts_find.handler({ query: "acme" })) as { result: { count: number } }).result.count).toBe(2);
    expect(((await tools.contacts_find_duplicates.handler({})) as { result: { total_groups: number } }).result.total_groups).toBe(1);
    expect(Object.keys(tools).some((n) => /merge|delete/.test(n))).toBe(false);
  });
  it("updates notes with confirmation and undo", async () => {
    const { tools, rows } = setup();
    expect(tools.contacts_update_notes.risk).toBe("destructive");
    const r = await tools.contacts_update_notes.handler({ contact_id: "1", notes: "old\nMet at expo" });
    expect(rows[0].notes).toContain("expo");
    await r.undo!();
    expect(rows[0].notes).toBe("old");
    expect((await tools.contacts_update_notes.handler({ contact_id: "zz", notes: "x" })).error).toContain("not found");
  });
});
