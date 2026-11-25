// Table cell properties: cell background colour through the table commands
// (tables.ts). Adrient has no document theme, so theme colours are n/a.
import { describe, expect, it } from "vitest";
import { makeEditor } from "../harness";
import { getCellBackground, insertTableSized, setCellBackground } from "../../tables";
import { rgbHex } from "../../textOps";

describe("js-api: table cell", () => {
  it("SetColor, GetColor", () => {
    const e = makeEditor("<p></p>");
    insertTableSized(e, 2, 2, false);
    // The caret is in cell (0, 0).
    expect(getCellBackground(e)).toBeNull();

    setCellBackground(e, rgbHex(255, 127, 0));
    expect(getCellBackground(e)).toBe("#ff7f00");

    setCellBackground(e, "#BADA55");
    expect(getCellBackground(e)?.toUpperCase()).toBe("#BADA55");
    expect(e.view.dom.querySelector("td")!.style.backgroundColor).toBe("rgb(186, 218, 85)");

    setCellBackground(e, null);
    expect(getCellBackground(e)).toBeNull();
    expect(e.view.dom.querySelector("td")!.style.backgroundColor).toBe("");
  });
});
