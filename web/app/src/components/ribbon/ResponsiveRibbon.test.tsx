import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { collapseOrder, collapsedAtStep, compactAtStep, ResponsiveGroup, ResponsiveRibbonBody, shrinkActions, useRibbonCompact } from "./ResponsiveRibbon";

describe("responsive ribbon order", () => {
  const prio: Record<string, number> = { clipboard: 80, styles: 10, cells: 45, editing: 90, number: 30 };
  const order = collapseOrder(Object.keys(prio).map((id, index) => ({ id, priority: prio[id], index })));
  const actions = shrinkActions(order, (id) => prio[id]);

  it("orders groups lowest priority first", () => {
    expect(order).toEqual(["styles", "number", "cells", "clipboard", "editing"]);
  });

  it("shrinks ordinary groups, folds minor ones, then shrinks key groups, then folds the rest", () => {
    expect(actions.map((a) => `${a.kind}:${a.id}`)).toEqual([
      "compact:styles", "compact:number", "compact:cells",
      "collapse:styles", "collapse:number",
      "compact:clipboard", "compact:editing",
      "collapse:cells", "collapse:clipboard", "collapse:editing",
    ]);
  });

  it("reports the small and folded groups at a step", () => {
    expect([...compactAtStep(actions, 0)]).toEqual([]);
    expect([...compactAtStep(actions, 3)]).toEqual(["styles", "number", "cells"]);
    expect([...collapsedAtStep(actions, 5)]).toEqual(["styles", "number"]);
    expect(compactAtStep(actions, 7).has("editing")).toBe(true);
    expect(collapsedAtStep(actions, actions.length).size).toBe(5);
  });
});

function Probe() {
  return <span>{useRibbonCompact() ? "small" : "large"}</span>;
}

describe("ResponsiveRibbonBody", () => {
  it("renders groups at full size when they fit", () => {
    render(
      <ResponsiveRibbonBody resetKey="home">
        <ResponsiveGroup title="Clipboard">
          <Probe />
        </ResponsiveGroup>
      </ResponsiveRibbonBody>,
    );
    expect(screen.getByRole("group", { name: "Clipboard" })).toBeTruthy();
    expect(screen.getByText("large")).toBeTruthy();
  });
});
