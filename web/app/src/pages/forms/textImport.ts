// Turn pasted text (a questionnaire from a document or e-mail) into form questions.
//
// Rules, in order: a blank line ends a question; without blank lines a new question starts at a line
// ending in "?" or at the next number in sequence. "---", "Page: Title", "Section 2: Title" or a Markdown
// heading starts a new section. Lines under a question starting with a bullet, letter, number, "[ ]" or
// "( )" are choices; two or more unmarked short lines are choices too. "[ ]" or "select all" make
// checkboxes (a dropdown past 12 choices). Without choices: "scale of 0 to 10" and "1 to N stars" make a
// scale, "describe/explain/comments" a paragraph, "email" a short answer that must be an email.
// A leading or trailing "*" or "(required)" marks a question required. "Note:" lines become sections.
import { blankQuestion } from "./helpers";
import type { FormQuestion } from "./types";

const MAX_QUESTIONS = 300;
const MAX_CHOICES = 200;

// "1.5 hours" and "U.S. residents" are answers, not choices: a letter or number plus a period needs a space.
const CHOICE_RE = /^\s*(?:(\[[ xX✓]?\])\s*|(\([ xX]?\))\s*|([-*•◦▪‣⁃○●☐□]|o(?=\s))\s*|([a-zA-Z])(?:\)\s*|\.\s+)|(\d{1,3})(?:\)\s*|\.\s+))(.+?)\s*$/;
const QNUM_RE = /^\s*(?:q(?:uestion)?\s*)?(\d{1,3})\s*([.):])\s+(.+)$/i;
const PAGE_RE = /^\s*(?:(?:page|section|part)\b\s*(?:\d+|[ivxlc]+|[a-z])?\s*(?::|\.(?=\s|$)|[-–—](?=\s|$)|\s[-–—])\s*(.*)|#{1,3}\s+(.+))$/i;
const RULE_RE = /^\s*(?:-{3,}|_{3,}|={3,})\s*$/;
const NOTE_RE = /^\s*(?:note|info|instructions?)\s*:\s*(.+)$/i;
const REQUIRED_RE = /\(\s*required\s*\)|\[\s*required\s*\]/i;
const MULTI_HINT = /\b(?:select|check|choose|tick|mark)\s+(?:all|any|every)|all that apply|more than one/i;
const LONG_HINT = /\b(?:describe|explain|comments?|tell us|feedback|suggestions?|elaborate|why\b|anything else|thoughts)/i;
const OTHER_RE = /^other\b/i;
const NONE_RE = /^(?:none of (?:the|these)(?: above)?|none)$/i;
const SCALE_RES = [
  /scale\s+(?:of|from)\s+(0|1)\s*(?:to|-|–)\s*(\d{1,2})\b/i,
  /\b(0|1)\s*(?:to|-|–)\s*(\d{1,2})\s*(?:stars?|(?:-?\s*point\s+)?scale)\b/i,
  /\brat(?:e|ing)\b.{0,80}?\b(0|1)\s*(?:to|-|–)\s*(\d{1,2})\b/i,
];

/** The [low, high] scale a question's wording names, or null. */
export function scaleOf(title: string): [number, number] | null {
  for (const re of SCALE_RES) {
    const m = re.exec(title);
    if (m) return [Number(m[1]), Number(m[2])];
  }
  return null;
}

interface Block {
  note?: string;
  title: string;
  required: boolean;
  num?: number | null;
  style?: string;
  choices: string[];
  loose: string[];
  description: string[];
  checkbox?: boolean;
  choiceStyle?: string;
}

function cleanTitle(raw: string) {
  let t = raw.trim();
  let required = false;
  if (/^\*\s*/.test(t)) {
    required = true;
    t = t.replace(/^\*\s*/, "");
  }
  if (/\s*\*$/.test(t)) {
    required = true;
    t = t.replace(/\s*\*$/, "");
  }
  if (REQUIRED_RE.test(t)) {
    required = true;
    t = t.replace(REQUIRED_RE, "").replace(/\s{2,}/g, " ").trim();
  }
  const m = QNUM_RE.exec(t);
  let num: number | null = null;
  let style = "";
  if (m) {
    num = Number(m[1]);
    style = m[2];
    t = m[3].trim();
  }
  return { title: t, required, num, style };
}

function parseChoice(line: string) {
  const m = CHOICE_RE.exec(line);
  if (!m) return null;
  const [, box, round, bullet, letter, number, label] = m;
  if (!label) return null;
  return {
    label,
    checkbox: !!box,
    numberStyle: number != null ? line.trim().charAt(String(number).length) : "",
    kind: box ? "box" : round ? "round" : bullet ? "bullet" : letter ? "letter" : "number",
  };
}

function toQuestion(b: Block): FormQuestion {
  if (b.note !== undefined) return { ...blankQuestion("short_answer"), is_section: true, title: b.note.slice(0, 500) };
  const base = { title: b.title.slice(0, 500), required: b.required, description: b.description.join("\n").slice(0, 1000) };
  if (b.choices.length) {
    const type = b.checkbox || MULTI_HINT.test(b.title) ? "checkboxes" : b.choices.length > 12 ? "dropdown" : "multiple_choice";
    // Other / None-of-the-above stay ordinary options: Forms has no built-in "other" slot.
    const options = b.choices.slice(0, MAX_CHOICES).map((c) => c.slice(0, 200));
    void OTHER_RE;
    void NONE_RE;
    return { ...blankQuestion(type), ...base, options };
  }
  const scale = scaleOf(b.title);
  if (scale) {
    const [lo, hi] = scale;
    if (lo === 0 && hi === 10) return { ...blankQuestion("linear_scale"), ...base, scale_min: 0, scale_max: 10, scale_min_label: "Not at all likely", scale_max_label: "Extremely likely" };
    if (lo === 1 && hi >= 2 && hi <= 10) return { ...blankQuestion("rating"), ...base, scale_min: 1, scale_max: hi };
  }
  if (/\bemail\b/i.test(b.title) && !LONG_HINT.test(b.title)) return { ...blankQuestion("short_answer"), ...base, validation: { kind: "text", op: "email" } };
  return { ...blankQuestion(LONG_HINT.test(b.title) ? "paragraph" : "short_answer"), ...base };
}

/** Parse pasted text into questions; each new page becomes a section divider. Never throws. */
export function parseQuestions(text: string): { questions: FormQuestion[]; count: number } {
  const lines = String(text || "").replace(/\r\n?/g, "\n").split("\n");
  const pages: { title: string; blocks: Block[] }[] = [{ title: "", blocks: [] }];
  let block: Block | null = null;
  let count = 0;

  const flush = () => {
    if (!block) return;
    if (!block.choices.length && block.loose.length >= 2 && block.loose.every((l) => l.length <= 120 && !/[?]$/.test(l))) block.choices = block.loose;
    else block.description.push(...block.loose);
    if (count < MAX_QUESTIONS && (block.title || block.note !== undefined)) {
      pages[pages.length - 1].blocks.push(block);
      count++;
    }
    block = null;
  };
  const start = (line: string) => {
    const note = NOTE_RE.exec(line);
    if (note) {
      block = { note: note[1].trim(), title: "", choices: [], loose: [], description: [], required: false };
      return;
    }
    const { title, required, num, style } = cleanTitle(line);
    block = { title, required, num, style, choices: [], loose: [], description: [], checkbox: false, choiceStyle: "" };
  };

  for (const raw of lines) {
    const line = raw.replace(/\s+/g, " ").trim();
    if (!line) {
      flush();
      continue;
    }
    if (RULE_RE.test(line)) {
      flush();
      pages.push({ title: "", blocks: [] });
      continue;
    }
    const pageMatch = PAGE_RE.exec(line);
    if (pageMatch && !/\?\s*$/.test(line)) {
      flush();
      const title = (pageMatch[1] || pageMatch[2] || "").trim();
      const current = pages[pages.length - 1];
      if (!current.blocks.length && !current.title) current.title = title;
      else pages.push({ title, blocks: [] });
      continue;
    }
    if (!block) {
      start(line);
      continue;
    }
    const b: Block = block;
    if (b.note !== undefined) {
      b.note += ` ${line}`;
      continue;
    }
    const numbered = QNUM_RE.exec(line);
    const choice = parseChoice(line);
    const endsQ = /\?\s*\**\s*(?:\(required\))?$/i.test(line);
    const nextNumber = numbered && b.num != null && Number(numbered[1]) === b.num + 1 && numbered[2] === b.style && (endsQ || b.choiceStyle !== `number${b.style}`);
    const asQuestion = endsQ && (!choice || (choice.kind === "bullet" && line.startsWith("*")) || (choice.kind === "number" && !b.choices.length));
    if (nextNumber || asQuestion) {
      flush();
      start(line);
      continue;
    }
    if (choice) {
      b.choices.push(...b.loose, choice.label);
      b.loose = [];
      if (choice.checkbox) b.checkbox = true;
      if (choice.kind === "number") b.choiceStyle = `number${choice.numberStyle}`;
      continue;
    }
    if (b.choices.length) b.choices.push(line);
    else b.loose.push(line);
  }
  flush();

  const out: FormQuestion[] = [];
  let n = 0;
  const kept = pages.filter((p, i) => p.blocks.length || (i === 0 && p.title));
  kept.forEach((p, i) => {
    if (p.title && (i > 0 || p.blocks.length)) out.push({ ...blankQuestion("short_answer"), is_section: true, title: p.title.slice(0, 200) });
    else if (!p.title && i > 0) out.push({ ...blankQuestion("short_answer"), is_section: true, title: `Section ${i + 1}` });
    for (const bl of p.blocks) {
      out.push(toQuestion(bl));
      n++;
    }
  });
  return { questions: out, count: n };
}
