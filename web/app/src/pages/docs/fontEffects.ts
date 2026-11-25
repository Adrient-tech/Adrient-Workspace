// Font effects (Docs wave 6): the character formatting Word's Font dialog
// has beyond bold / italic / underline: All caps, Small caps, Hidden,
// character spacing (expanded / condensed), horizontal scale and a raised /
// lowered baseline. One `fontEffects` mark carries them, so a .docx's
// w:caps, w:smallCaps, w:vanish, w:spacing, w:w and w:position survive a
// round trip instead of being dropped.
//
// Character spacing, baseline shift and text width: spacing in points
// (expanded > 0, condensed < 0), scale 1-600 %, baseline shift in points.
import { Mark, mergeAttributes } from "@tiptap/core";
import type { EditorState } from "@tiptap/pm/state";

export interface FontEffects {
  allCaps: boolean;
  smallCaps: boolean;
  hidden: boolean;
  /** Extra space after each character, points (negative = condensed). */
  spacing: number | null;
  /** Horizontal scale, percent (100 = normal). */
  scale: number | null;
  /** Baseline shift, points (positive = raised). */
  raise: number | null;
}

export const NO_EFFECTS: FontEffects = { allCaps: false, smallCaps: false, hidden: false, spacing: null, scale: null, raise: null };

export const SPACING_RANGE = { min: -20, max: 100 } as const;
export const SCALE_RANGE = { min: 1, max: 600 } as const;
export const RAISE_RANGE = { min: -200, max: 200 } as const;

const clamp = (n: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, n));
const round2 = (n: number) => Math.round(n * 100) / 100;

/** Validates a field of the Font dialog; null = empty or not a number. */
export function parseEffectNumber(raw: string, range: { min: number; max: number }): number | null {
  const t = raw.trim().replace(",", ".");
  if (!t) return null;
  const n = Number(t.replace(/\s*(pt|%)$/i, ""));
  if (!Number.isFinite(n)) return null;
  return round2(clamp(n, range.min, range.max));
}

/** The CSS a set of effects paints (also what the editor's mark renders). */
export function effectsCss(e: Partial<FontEffects>): string {
  const css: string[] = [];
  if (e.allCaps) css.push("text-transform: uppercase");
  if (e.smallCaps) css.push("font-variant-caps: small-caps");
  if (e.spacing) css.push(`letter-spacing: ${round2(e.spacing)}pt`);
  if (e.scale && e.scale !== 100) css.push(`display: inline-block; transform: scaleX(${round2(e.scale / 100)}); transform-origin: left`);
  if (e.raise) css.push(`position: relative; top: ${round2(-e.raise)}pt`);
  return css.join("; ");
}

/** True when the effects carry nothing (the mark can be removed). */
export function isNoEffects(e: Partial<FontEffects>): boolean {
  return !e.allCaps && !e.smallCaps && !e.hidden && !e.spacing && (!e.scale || e.scale === 100) && !e.raise;
}

/** The fontEffects attrs at the caret, or of the first marked text in the selection. */
export function effectsInState(state: EditorState): Partial<FontEffects> | undefined {
  const type = state.schema.marks.fontEffects;
  if (!type) return undefined;
  const { from, to, empty } = state.selection;
  if (empty) return (state.storedMarks ?? state.selection.$from.marks()).find((m) => m.type === type)?.attrs as Partial<FontEffects> | undefined;
  let found: Partial<FontEffects> | undefined;
  state.doc.nodesBetween(from, to, (node) => {
    if (found) return false;
    if (node.isText) found = node.marks.find((m) => m.type === type)?.attrs as Partial<FontEffects> | undefined;
    return true;
  });
  return found;
}

declare module "@tiptap/core" {
  interface Commands<ReturnType> {
    fontEffects: {
      /** Set the given effects on the selection, keeping the others. */
      setFontEffects: (effects: Partial<FontEffects>) => ReturnType;
      /** Flip one boolean effect (all caps / small caps / hidden). */
      toggleFontEffect: (name: "allCaps" | "smallCaps" | "hidden") => ReturnType;
      /** Remove every font effect from the selection. */
      unsetFontEffects: () => ReturnType;
    };
  }
}

const numAttr = (name: string) => ({
  default: null as number | null,
  parseHTML: (el: HTMLElement) => {
    const v = el.getAttribute(`data-fx-${name}`);
    return v == null ? null : Number(v);
  },
  renderHTML: (a: Record<string, unknown>) => (a[name] == null ? {} : { [`data-fx-${name}`]: String(a[name]) }),
});
const boolAttr = (name: string) => ({
  default: false,
  parseHTML: (el: HTMLElement) => el.hasAttribute(`data-fx-${name}`),
  renderHTML: (a: Record<string, unknown>) => (a[name] ? { [`data-fx-${name}`]: "1" } : {}),
});

export const FontEffectsMark = Mark.create({
  name: "fontEffects",
  inclusive: true,
  addAttributes() {
    return {
      allCaps: boolAttr("allCaps"),
      smallCaps: boolAttr("smallCaps"),
      hidden: boolAttr("hidden"),
      spacing: numAttr("spacing"),
      scale: numAttr("scale"),
      raise: numAttr("raise"),
    };
  },
  parseHTML() {
    return [{ tag: "span[data-font-effects]", priority: 60, consuming: false }];
  },
  renderHTML({ mark, HTMLAttributes }) {
    const a = mark.attrs as Partial<FontEffects>;
    const css = effectsCss(a);
    return ["span", mergeAttributes(HTMLAttributes, { "data-font-effects": "1", ...(css ? { style: css } : {}), ...(a.hidden ? { class: "doc-hidden-text" } : {}) }), 0];
  },
  addCommands() {
    return {
      setFontEffects:
        (effects) =>
        ({ state, tr, dispatch }) => {
          const type = state.schema.marks.fontEffects;
          const { from, to, empty } = state.selection;
          const cur = effectsInState(state);
          const next = { ...NO_EFFECTS, ...cur, ...effects };
          if (empty) {
            tr.setStoredMarks((state.storedMarks ?? state.selection.$from.marks()).filter((m) => m.type !== type).concat(isNoEffects(next) ? [] : [type.create(next)]));
          } else {
            tr.removeMark(from, to, type);
            if (!isNoEffects(next)) tr.addMark(from, to, type.create(next));
          }
          if (dispatch) dispatch(tr);
          return true;
        },
      toggleFontEffect:
        (name) =>
        ({ state, commands }) => {
          const on = !!effectsInState(state)?.[name];
          return commands.setFontEffects({ [name]: !on });
        },
      unsetFontEffects:
        () =>
        ({ commands }) =>
          commands.unsetMark("fontEffects"),
    };
  },
});
