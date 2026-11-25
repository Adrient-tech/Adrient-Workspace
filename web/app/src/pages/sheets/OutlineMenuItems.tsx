/* eslint-disable @typescript-eslint/no-explicit-any -- FortuneSheet API is loosely typed. */
// Menu items for row/column grouping, shared by the ribbon (Data ▸ Outline) and the classic View menu.
import { ListDivider, MenuItem, Typography } from "@mui/joy";
import { currentSheet } from "./sheetDataTools";
import { applyLevel, groupLabel, groupSelection, maxLevel, sheetOutline, toggleGroup, ungroupSelection, type OutlineAxis } from "./outlineOps";

export function OutlineMenuItems({ getWb }: { getWb: () => any }) {
  const w = (() => {
    try {
      return getWb();
    } catch {
      return null;
    }
  })();
  const list = w ? sheetOutline(currentSheet(w)) : [];
  const run = (fn: (w: any) => string | null | void) => () => {
    const wb = getWb();
    if (!wb) return;
    const err = fn(wb);
    if (err) window.alert(err);
  };
  const levels = (axis: OutlineAxis) => maxLevel(list, axis);
  return (
    <>
      <MenuItem onClick={run((x) => groupSelection(x, "row"))}>Group rows</MenuItem>
      <MenuItem onClick={run((x) => groupSelection(x, "col"))}>Group columns</MenuItem>
      <MenuItem disabled={!list.some((g) => g.axis === "row")} onClick={run((x) => ungroupSelection(x, "row"))}>Ungroup rows</MenuItem>
      <MenuItem disabled={!list.some((g) => g.axis === "col")} onClick={run((x) => ungroupSelection(x, "col"))}>Ungroup columns</MenuItem>
      {list.length > 0 && <ListDivider />}
      {list.map((g) => (
        <MenuItem key={g.id} data-testid={`outline-${g.id}`} onClick={run((x) => toggleGroup(x, g.id))}>
          {g.collapsed ? "⊞ Expand" : "⊟ Collapse"} {groupLabel(g)}
        </MenuItem>
      ))}
      {(["row", "col"] as const).map((axis) =>
        levels(axis) > 1 ? (
          <MenuItem key={axis} disabled sx={{ opacity: "1 !important" }}>
            <Typography level="body-xs">{axis === "row" ? "Row" : "Column"} levels:</Typography>
          </MenuItem>
        ) : null,
      )}
      {(["row", "col"] as const).flatMap((axis) =>
        levels(axis) > 1
          ? Array.from({ length: levels(axis) + 1 }, (_, i) => (
              <MenuItem key={`${axis}${i}`} onClick={run((x) => applyLevel(x, axis, i + 1))}>
                Show {axis === "row" ? "row" : "column"} level {i + 1}
              </MenuItem>
            ))
          : [],
      )}
    </>
  );
}
