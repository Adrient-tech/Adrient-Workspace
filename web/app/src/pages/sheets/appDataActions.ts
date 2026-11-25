/* eslint-disable @typescript-eslint/no-explicit-any -- FortuneSheet ref API is loosely typed. */
import { uniqueSheetName, type AppTable } from "./appData";

/** Adds a sheet tab holding `table` (header bolded) and activates it. Returns its id. */
export function appTableToSheet(wb: any, table: AppTable): string {
  const all: any[] = wb.getAllSheets?.() ?? [];
  const id = `app_${Date.now().toString(36)}`;
  wb.addSheet?.(id);
  // Activating builds the new sheet's cell grid; writes before that fail.
  wb.activateSheet?.({ id });
  try {
    wb.setSheetName?.(
      uniqueSheetName(table.title, all.map((s) => s.name)),
      { id },
    );
  } catch {
    /* keep the default name */
  }
  // The grid is built on the next render; write once the new sheet is current and has data.
  let tries = 0;
  const write = () => {
    const cur = wb.getSheet?.();
    if ((String(cur?.id) !== id || !Array.isArray(cur?.data)) && tries++ < 40) {
      window.setTimeout(write, 50);
      return;
    }
    // Same write path as Data cleanup (current sheet), which is known to render.
    const nRows = table.rows.length + 1;
    const nCols = table.header.length;
    const have = Number(cur?.row ?? 0);
    if (have && have < nRows + 5) {
      try {
        wb.insertRowOrColumn?.("row", have - 1, nRows + 5 - have, "rightbottom", { id });
      } catch {
        /* sheet keeps its default size */
      }
    }
    const data = [
      table.header.map((h) => ({ v: h, m: h, bl: 1, bg: "#E3F1EC", fc: "#00261F" })),
      ...table.rows.map((row) => row.map((v) => (v === "" ? null : { v, m: String(v), ...(typeof v === "number" ? { ct: { fa: "General", t: "n" } } : {}) }))),
    ];
    wb.setCellValuesByRange?.(data, { row: [0, nRows - 1], column: [0, nCols - 1] });
  };
  window.setTimeout(write, 0);
  return id;
}
