import { describe, expect, it } from "vitest";
import type { Field, Rec, Table, View } from "./api";
import { cellText, cleanFilter, editText, filterCount, kanbanColumns, monthGrid, parsePasted, recordsByDay, summarize, viewQuery, visibleFields } from "./logic";

const f = (o: Partial<Field>): Field => ({ id: "f", table_id: "t", name: "F", type: "text", options: {}, position: 0, ...o });
const rec = (fields: Record<string, unknown>): Rec => ({ id: Math.random().toString(), seq: 1, fields, created_at: "", updated_at: "" });

describe("cellText", () => {
  it("formats numbers by type", () => {
    expect(cellText(f({ type: "currency", options: { precision: 2 } }), 1234.5)).toBe("₹1,234.50");
    expect(cellText(f({ type: "percent", options: { precision: 1 } }), 0.255)).toBe("25.5%");
    expect(cellText(f({ type: "duration" }), 3723)).toBe("1:02:03");
    expect(cellText(f({ type: "rating" }), 3)).toBe("★★★");
    expect(cellText(f({ type: "number" }), 12.5)).toBe("12.5");
  });
  it("shows options, links and errors", () => {
    const sel = f({ type: "single_select", options: { choices: [{ id: "a", name: "Open" }] } });
    expect(cellText(sel, "a")).toBe("Open");
    expect(cellText(f({ type: "link" }), [{ id: "1", name: "A" }, { id: "2", name: "B" }])).toBe("A, B");
    expect(cellText(f({ type: "formula" }), { error: "#DIV/0!" })).toBe("#ERROR");
    expect(cellText(f({ type: "checkbox" }), true)).toBe("✓");
    expect(cellText(f({}), null)).toBe("");
  });
  it("round trips edit text", () => {
    expect(editText(f({ type: "percent" }), 0.5)).toBe("50%");
    expect(editText(f({ type: "date" }), "2026-03-04T10:00:00Z")).toBe("2026-03-04");
  });
});

describe("filters", () => {
  it("drops incomplete conditions", () => {
    expect(cleanFilter({ field: "a", op: "contains", value: "" })).toBeUndefined();
    expect(cleanFilter({ field: "a", op: "isEmpty" })).toBeTruthy();
    const n = cleanFilter({ logic: "or", conds: [{ field: "a", op: "is", value: true }, { field: "b", op: "contains" }] });
    expect(n?.conds).toHaveLength(1);
    expect(filterCount({ conds: [{ field: "a", op: "isEmpty" }, { field: "b", op: "isNotEmpty" }] })).toBe(2);
  });
  it("builds the query of a view", () => {
    const v: View = { id: "v", table_id: "t", name: "v", type: "grid", position: 0, config: { sort: [{ field: "a", desc: true }], filter: { field: "a", op: "isEmpty" } } };
    const q = viewQuery(v, " hi ");
    expect(q.search).toBe("hi");
    expect(q.sort).toEqual([{ field: "a", desc: true }]);
    expect(viewQuery(undefined, "").filter).toBeUndefined();
  });
});

describe("visibleFields", () => {
  const t: Table = { id: "t", base_id: "b", name: "T", description: "", position: 0, record_count: 0, views: [], fields: ["a", "b", "c", "d"].map((id) => f({ id, name: id })) };
  it("keeps the first field, applies order and hides", () => {
    const v: View = { id: "v", table_id: "t", name: "v", type: "grid", position: 0, config: { field_order: ["d", "b"], hidden: ["c"] } };
    expect(visibleFields(t, v).map((x) => x.id)).toEqual(["a", "d", "b"]);
    expect(visibleFields(t, v, true).map((x) => x.id)).toEqual(["a", "d", "b", "c"]);
  });
});

describe("kanban and calendar", () => {
  const sel = f({ id: "s", type: "single_select", options: { choices: [{ id: "x", name: "X" }, { id: "y", name: "Y" }] } });
  it("groups records into stacks", () => {
    const cols = kanbanColumns(sel, [rec({ s: "x" }), rec({ s: "y" }), rec({ s: "x" }), rec({})]);
    expect(cols.map((c) => [c.name, c.recs.length])).toEqual([["No value", 1], ["X", 2], ["Y", 1]]);
  });
  it("builds a 6-week month grid", () => {
    const g = monthGrid(2026, 2);
    expect(g).toHaveLength(42);
    expect(g.find((d) => d.date === "2026-03-01")?.inMonth).toBe(true);
    expect(g[0].date <= "2026-03-01").toBe(true);
  });
  it("spreads spans over days", () => {
    const m = recordsByDay([rec({ a: "2026-03-02", b: "2026-03-04" }), rec({ a: "2026-03-03" })], "a", "b");
    expect([...m.keys()].sort()).toEqual(["2026-03-02", "2026-03-03", "2026-03-04"]);
    expect(m.get("2026-03-03")).toHaveLength(2);
  });
});

describe("paste and summary", () => {
  it("parses TSV and quoted CSV", () => {
    expect(parsePasted("a\tb\nc\td\n")).toEqual([["a", "b"], ["c", "d"]]);
    expect(parsePasted('x,"y, z","q ""w"""')).toEqual([["x", "y, z", 'q "w"']]);
  });
  it("summarises numbers", () => {
    const n = f({ id: "n", type: "number", options: { precision: 1 } });
    const rs = [rec({ n: 2 }), rec({ n: 4 }), rec({})];
    expect(summarize(n, rs, "sum")).toBe("6.0");
    expect(summarize(n, rs, "avg")).toBe("3.0");
    expect(summarize(n, rs, "empty")).toBe("1");
    expect(summarize(n, rs, "count")).toBe("3");
  });
});
