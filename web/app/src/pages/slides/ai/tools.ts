/**
 * Assistant tools for Slides (reference set, see docs/ai.md). Tools talk to
 * the deck only through a SlidesAdapter; deckAdapter.ts implements it over
 * the editor's Slide[] model.
 */
import type { AssistantTool, ToolOutcome } from "@/lib/ai";

export type Undo = () => void;

export interface SlideOutline {
  /** 1-based slide number. */
  number: number;
  title: string;
  bullets: string[];
  notes: string;
  layout?: string;
}

export interface NewSlide {
  title: string;
  bullets?: string[];
  notes?: string;
}

export interface SlidesAdapter {
  outline(): SlideOutline[];
  /** 1-based number of the slide shown in the editor. */
  current(): number;
  addSlides(after: number, slides: NewSlide[]): Undo;
  setText(number: number, patch: { title?: string; bullets?: string[] }): Undo | string;
  setNotes(number: number, notes: string): Undo | string;
  layouts?(): string[];
  applyLayout?(number: number, layout: string): Undo | string;
}

const fail = (error: string): ToolOutcome => ({ error });
const str = (v: unknown, max: number) => String(v ?? "").slice(0, max);

function slideNo(ad: SlidesAdapter, v: unknown): number | string {
  const n = v === undefined ? ad.current() : Number(v);
  const count = ad.outline().length;
  return Number.isInteger(n) && n >= 1 && n <= count ? n : `slide must be between 1 and ${count}`;
}

const slideSchema = {
  type: "object" as const,
  properties: {
    title: { type: "string" as const },
    bullets: { type: "array" as const, items: { type: "string" as const } },
    notes: { type: "string" as const },
  },
  required: ["title"],
};

export function createSlideTools(ad: SlidesAdapter): AssistantTool[] {
  const tools: AssistantTool[] = [
    {
      name: "slide_read_outline",
      risk: "read",
      description: "Read the deck outline: every slide's number, title, bullet text, speaker notes and layout, plus the current slide.",
      parameters: { type: "object", properties: {} },
      label: () => "Read the deck outline",
      handler: () => ({ result: { current: ad.current(), slides: ad.outline() }, summary: "Read the deck outline" }),
    },
    {
      name: "slide_add_slides",
      risk: "write",
      description: "Add new title-and-bullets slides after a slide (default: the current slide). Keep titles short and at most 5 bullets.",
      parameters: { type: "object", properties: { after: { type: "integer", description: "Slide number to insert after (0 = at the start)" }, slides: { type: "array", items: slideSchema } }, required: ["slides"] },
      label: (a) => `Add ${(a.slides as unknown[]).length} slides`,
      handler: (a) => {
        const list = (a.slides as NewSlide[]).slice(0, 30).map((s) => ({
          title: str(s.title, 200),
          bullets: (s.bullets ?? []).slice(0, 12).map((b) => str(b, 300)),
          notes: s.notes ? str(s.notes, 4000) : undefined,
        }));
        if (!list.length) return fail("give at least one slide");
        const after = a.after === undefined ? ad.current() : Number(a.after);
        if (!Number.isInteger(after) || after < 0 || after > ad.outline().length) return fail("after is not a slide number");
        return { result: { added: list.length, after }, undo: ad.addSlides(after, list), summary: `Added ${list.length} slide${list.length > 1 ? "s" : ""}` };
      },
    },
    {
      name: "slide_set_text",
      risk: "write",
      description: "Replace the title and/or bullets of a slide (rewrite, shorten, translate).",
      parameters: { type: "object", properties: { slide: { type: "integer" }, title: { type: "string" }, bullets: { type: "array", items: { type: "string" } } } },
      label: (a) => `Edit slide ${String(a.slide ?? ad.current())}`,
      handler: (a) => {
        const n = slideNo(ad, a.slide);
        if (typeof n === "string") return fail(n);
        if (a.title === undefined && a.bullets === undefined) return fail("give a title or bullets");
        const res = ad.setText(n, { title: a.title === undefined ? undefined : str(a.title, 200), bullets: (a.bullets as string[] | undefined)?.slice(0, 12).map((b) => str(b, 300)) });
        return typeof res === "string" ? fail(res) : { result: { slide: n }, undo: res, summary: `Edited slide ${n}` };
      },
    },
    {
      name: "slide_set_notes",
      risk: "write",
      description: "Write the speaker notes of a slide.",
      parameters: { type: "object", properties: { slide: { type: "integer" }, notes: { type: "string" } }, required: ["notes"] },
      label: (a) => `Write notes for slide ${String(a.slide ?? ad.current())}`,
      handler: (a) => {
        const n = slideNo(ad, a.slide);
        if (typeof n === "string") return fail(n);
        const res = ad.setNotes(n, str(a.notes, 4000));
        return typeof res === "string" ? fail(res) : { result: { slide: n }, undo: res, summary: `Wrote speaker notes for slide ${n}` };
      },
    },
  ];
  const layouts = ad.layouts?.bind(ad);
  const apply = ad.applyLayout?.bind(ad);
  if (layouts && apply) {
    tools.push({
      name: "slide_apply_layout",
      risk: "write",
      description: "Apply one of the deck's layouts to a slide (use slide_read_outline and the layouts list to suggest a fitting one).",
      parameters: { type: "object", properties: { slide: { type: "integer" }, layout: { type: "string" } }, required: ["layout"] },
      label: (a) => `Apply layout "${String(a.layout)}"`,
      handler: (a) => {
        const n = slideNo(ad, a.slide);
        if (typeof n === "string") return fail(n);
        const names = layouts();
        if (!names.includes(String(a.layout))) return fail(`layout must be one of: ${names.join(", ")}`);
        const res = apply(n, String(a.layout));
        return typeof res === "string" ? fail(res) : { result: { slide: n, layout: a.layout }, undo: res, summary: `Applied "${String(a.layout)}" to slide ${n}` };
      },
    });
    tools.push({
      name: "slide_list_layouts",
      risk: "read",
      description: "List the layouts available in this deck.",
      parameters: { type: "object", properties: {} },
      handler: () => ({ result: { layouts: layouts() } }),
    });
  }
  return tools;
}

export function slideContext(ad: SlidesAdapter): { label: string; text?: string } {
  const n = ad.current();
  const s = ad.outline()[n - 1];
  if (!s) return { label: "Deck" };
  return { label: `Slide ${n}`, text: [`Slide ${n}: ${s.title}`, ...s.bullets.map((b) => `- ${b}`), s.notes ? `Notes: ${s.notes}` : ""].filter(Boolean).join("\n") };
}
