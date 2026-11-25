// Sort the selected paragraphs (Docs wave 6): A to Z, Z to A, or by the
// number each one starts with. Useful for lists of names and figures.
import type { Editor } from "@tiptap/core";
import type { Node as PMNode } from "@tiptap/pm/model";

export const PARAGRAPH_SORTS = [
  { id: "textAsc", label: "A to Z" },
  { id: "textDesc", label: "Z to A" },
  { id: "numberAsc", label: "Smallest number first" },
  { id: "numberDesc", label: "Largest number first" },
] as const;
export type ParagraphSort = (typeof PARAGRAPH_SORTS)[number]["id"];

/** The number a paragraph leads with ("3. Pears" -> 3, "$1,200" -> 1200), or null. */
export function leadingNumber(text: string): number | null {
  // The first number in the text; ".75" counts, the dots of an ellipsis do not.
  // A minus sign counts only when nothing attaches it to a word: "Room A-5" is room five, not minus five.
  const m = /(?:(?<![\p{L}\p{N}])-)?(?:\d[\d,.]*|(?<![.\d])\.\d+)/u.exec(String(text || ""));
  if (!m) return null;
  // "1,5" and "1.234,5" are European decimals; "1,200" and "1,200.50" use the comma for thousands.
  let digits = m[0].replace(/[.,]+$/, "");
  if (/^-?\d+,\d{1,2}$/.test(digits) || /^-?\d{1,3}(?:\.\d{3})+,\d+$/.test(digits)) digits = digits.replace(/\./g, "").replace(",", ".");
  else digits = digits.replace(/,/g, "");
  const n = Number.parseFloat(digits);
  return Number.isFinite(n) ? n : null;
}

/**
 * The ids of `items` in sorted order. Text sorts are locale-aware and
 * case-insensitive with numbers in natural order; number sorts put paragraphs
 * without a number last. Ties keep document order.
 */
export function sortedIds<T extends string | number>(items: { id: T; text: string }[], choiceId: ParagraphSort): T[] {
  const choice = PARAGRAPH_SORTS.find((c) => c.id === choiceId);
  if (!choice) throw new Error("Unknown sort");
  const desc = choice.id.endsWith("Desc");
  const byNumber = choice.id.startsWith("number");
  const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: "base" });
  const decorated = items.map((it, index) => ({ id: it.id, text: String(it.text || "").trim(), n: byNumber ? leadingNumber(it.text) : null, index }));
  decorated.sort((a, b) => {
    let cmp: number;
    if (byNumber) {
      if (a.n == null && b.n == null) cmp = 0;
      else if (a.n == null) return 1;
      else if (b.n == null) return -1;
      else cmp = a.n - b.n;
    } else cmp = collator.compare(a.text, b.text);
    if (desc) cmp = -cmp;
    return cmp || a.index - b.index;
  });
  return decorated.map((d) => d.id);
}

/**
 * Sorts the paragraphs the selection covers, when they are siblings in one
 * parent (body, table cell, list item). Resolves the number of paragraphs
 * that changed place; throws when fewer than two are selected. Formatting,
 * list membership and marks travel with each paragraph, and one undo step
 * restores the order.
 */
export function sortSelectedParagraphs(editor: Editor, choice: ParagraphSort): number {
  const { state } = editor;
  const { $from, $to } = state.selection;
  const depth = Math.min($from.sharedDepth($to.pos), $from.depth);
  const parent = $from.node(depth);
  const base = $from.start(depth);
  const picked: { node: PMNode; pos: number; index: number }[] = [];
  let at = base;
  parent.forEach((child, _o, index) => {
    const end = at + child.nodeSize;
    if (child.isTextblock && end > $from.pos && at < $to.pos + (state.selection.empty ? 1 : 0)) picked.push({ node: child, pos: at, index });
    at = end;
  });
  const sameRun = picked.every((p, i) => i === 0 || p.index === picked[i - 1].index + 1);
  if (picked.length < 2 || !sameRun) throw new Error("Select at least two paragraphs in a row to sort");
  const ids = sortedIds(picked.map((p, i) => ({ id: i, text: p.node.textContent })), choice);
  const moved = ids.filter((id, i) => id !== i).length;
  if (!moved) return 0;
  const from = picked[0].pos;
  const to = picked[picked.length - 1].pos + picked[picked.length - 1].node.nodeSize;
  editor.view.dispatch(state.tr.replaceWith(from, to, ids.map((i) => picked[i].node)).scrollIntoView());
  return moved;
}
