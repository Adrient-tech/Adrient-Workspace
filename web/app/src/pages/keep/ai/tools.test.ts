import { describe, expect, it } from "vitest";
import { createKeepTools, type KNote, type KeepAdapter } from "./tools";

const n = (id: string, o: Partial<KNote> = {}): KNote => ({ id, title: id, body: "", color: "", pinned: false, archived: false, labels: [], checklist: [], ...o });

function setup() {
  const notes = [n("a", { title: "Trip", body: "socks, shirts" })];
  const ad: KeepAdapter = {
    list: async () => notes.map((x) => ({ ...x })),
    create: async (i) => { const r = n("n" + notes.length, i as Partial<KNote>); notes.push(r); return r; },
    update: async (id, i) => { const k = notes.findIndex((x) => x.id === id); notes[k] = { ...i, id }; return notes[k]; },
    trash: async (id) => { notes.splice(notes.findIndex((x) => x.id === id), 1); },
  };
  return { notes, tools: Object.fromEntries(createKeepTools(ad).map((t) => [t.name, t])) };
}

describe("keep tools", () => {
  it("finds notes by text", async () => {
    const { tools } = setup();
    expect(((await tools.keep_find_notes.handler({ query: "socks" })) as { result: { count: number } }).result.count).toBe(1);
    expect(((await tools.keep_find_notes.handler({ query: "zzz" })) as { result: { count: number } }).result.count).toBe(0);
  });
  it("creates a checklist note; undo trashes it", async () => {
    const { tools, notes } = setup();
    const r = await tools.keep_create_note.handler({ title: "Packing", checklist: ["Socks", "Shirts"] });
    expect(notes[1].checklist).toEqual([{ text: "Socks", checked: false }, { text: "Shirts", checked: false }]);
    await r.undo!();
    expect(notes).toHaveLength(1);
  });
  it("rewrites a note into a checklist; undo restores the original", async () => {
    const { tools, notes } = setup();
    expect(tools.keep_update_note.risk).toBe("destructive");
    const r = await tools.keep_update_note.handler({ note_id: "a", checklist: [{ text: "socks" }, { text: "shirts", checked: true }] });
    expect(notes[0].checklist).toHaveLength(2);
    expect(notes[0].body).toBe("socks, shirts");
    await r.undo!();
    expect(notes[0].checklist).toEqual([]);
    expect((await tools.keep_update_note.handler({ note_id: "zz" })).error).toContain("not found");
  });
});
