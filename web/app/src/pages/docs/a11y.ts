// Accessibility checker (Tools ▸ Accessibility). Pure scan of a ProseMirror
// document for issues a screen-reader user would hit.
import type { Node as PMNode } from "@tiptap/pm/model";

export interface A11yIssue {
  pos: number;
  rule: "image-alt" | "heading-empty" | "heading-skip" | "table-header" | "link-text" | "empty-runs";
  severity: "error" | "warning";
  message: string;
}

const VAGUE_LINK = /^(click here|here|link|read more|more|this)$/i;
const IMAGE_TYPES = new Set(["inlineImage", "image", "shape", "textBox", "chart", "drawing"]);

export function checkAccessibility(doc: PMNode): A11yIssue[] {
  const out: A11yIssue[] = [];
  let lastLevel = 0;
  let emptyRun = 0;
  doc.descendants((node, pos) => {
    if (node.type.name === "heading") {
      const level = Number(node.attrs.level) || 1;
      if (!node.textContent.trim()) out.push({ pos, rule: "heading-empty", severity: "error", message: "Empty heading" });
      if (lastLevel && level > lastLevel + 1)
        out.push({ pos, rule: "heading-skip", severity: "warning", message: `Heading level jumps from ${lastLevel} to ${level}: “${node.textContent.slice(0, 40)}”` });
      lastLevel = level;
    }
    if (IMAGE_TYPES.has(node.type.name) && node.type.name !== "textBox" && !String(node.attrs.alt ?? "").trim()) {
      out.push({ pos, rule: "image-alt", severity: "error", message: `${node.type.name === "inlineImage" ? "Image" : "Object"} has no alt text` });
    }
    if (node.type.name === "table") {
      const first = node.firstChild;
      let hasHeader = false;
      first?.forEach((c) => {
        if (c.type.name === "tableHeader") hasHeader = true;
      });
      if (!hasHeader && node.childCount > 1) out.push({ pos, rule: "table-header", severity: "warning", message: "Table has no header row" });
    }
    if (node.isText && node.marks.some((m) => m.type.name === "link") && VAGUE_LINK.test((node.text ?? "").trim()))
      out.push({ pos, rule: "link-text", severity: "warning", message: `Link text “${node.text}” is not descriptive` });
    if (node.type.name === "paragraph") {
      if (!node.textContent.trim() && node.childCount === 0) {
        emptyRun++;
        if (emptyRun === 3) out.push({ pos, rule: "empty-runs", severity: "warning", message: "Several empty paragraphs in a row (use spacing instead)" });
      } else emptyRun = 0;
    }
    return true;
  });
  return out;
}
