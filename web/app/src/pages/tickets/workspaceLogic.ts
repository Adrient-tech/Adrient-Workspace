// Pure logic of the agent workspace: keyboard shortcuts, list windowing, open-ticket tabs,
// safe text formatting and @mentions. No React here so it can be unit tested.

export type Density = "compact" | "comfortable" | "spacious";
export const ROW_HEIGHT: Record<Density, number> = { compact: 40, comfortable: 64, spacious: 88 };

/** Which rows of a fixed-height list to render for a scroll position. */
export function visibleWindow(total: number, rowH: number, scrollTop: number, viewportH: number, overscan = 6): { start: number; end: number; padTop: number; padBottom: number } {
  if (total <= 0 || rowH <= 0) return { start: 0, end: 0, padTop: 0, padBottom: 0 };
  const first = Math.max(0, Math.floor(scrollTop / rowH) - overscan);
  const last = Math.min(total, Math.ceil((scrollTop + viewportH) / rowH) + overscan);
  return { start: first, end: last, padTop: first * rowH, padBottom: (total - last) * rowH };
}

// ---- keyboard ----------------------------------------------------------------

export type ShortcutAction =
  | { type: "move"; delta: 1 | -1 }
  | { type: "open" }
  | { type: "reply" }
  | { type: "note" }
  | { type: "assign" }
  | { type: "solve" }
  | { type: "snooze" }
  | { type: "follow" }
  | { type: "palette" }
  | { type: "search" }
  | { type: "help" }
  | { type: "close-tab" }
  | { type: "goto"; view: string }
  | { type: "none" };

/** Second keys after "g": g then i = inbox (mine) and so on. */
export const GOTO_KEYS: Record<string, { view: string; label: string }> = {
  m: { view: "mine", label: "Assigned to me" },
  u: { view: "unassigned", label: "Unassigned" },
  a: { view: "all_open", label: "All unsolved" },
  h: { view: "urgent", label: "Urgent and high" },
  i: { view: "incidents", label: "Open incidents" },
  s: { view: "stale", label: "Stale" },
  z: { view: "snoozed", label: "Snoozed" },
};

export const SHORTCUTS: { keys: string; does: string }[] = [
  { keys: "j / k", does: "Next / previous ticket" },
  { keys: "Enter", does: "Open the selected ticket" },
  { keys: "r", does: "Reply to the requester" },
  { keys: "n", does: "Add an internal note" },
  { keys: "a", does: "Assign to me" },
  { keys: "e", does: "Mark resolved" },
  { keys: "z", does: "Snooze until tomorrow" },
  { keys: "f", does: "Follow or unfollow" },
  { keys: "/", does: "Search tickets" },
  { keys: "Ctrl Shift K", does: "Desk command bar (Ctrl K is the app-wide search)" },
  { keys: "g then m / u / a / h / i / s / z", does: "Go to a view" },
  { keys: "x", does: "Close the open tab" },
  { keys: "h", does: "Show this list (the ? key opens the general shortcut list)" },
];

export interface KeyState {
  /** Set after "g" was pressed; cleared by the next key or after a second. */
  pendingG: boolean;
}

export interface KeyInput {
  key: string;
  shift?: boolean;
  ctrl?: boolean;
  meta?: boolean;
  alt?: boolean;
  /** The event target is a text field: only Ctrl/Cmd shortcuts and Escape apply. */
  typing: boolean;
}

/** Map a key press to an action; returns the next state as well. */
export function keyAction(state: KeyState, k: KeyInput): { action: ShortcutAction; next: KeyState } {
  const none = { action: { type: "none" } as ShortcutAction, next: { pendingG: false } };
  if ((k.ctrl || k.meta) && k.shift && k.key.toLowerCase() === "k") return { action: { type: "palette" }, next: { pendingG: false } };
  if (k.typing || k.ctrl || k.meta || k.alt) return none;
  if (state.pendingG) {
    const g = GOTO_KEYS[k.key.toLowerCase()];
    return g ? { action: { type: "goto", view: g.view }, next: { pendingG: false } } : none;
  }
  switch (k.key) {
    case "j":
    case "ArrowDown":
      return { action: { type: "move", delta: 1 }, next: { pendingG: false } };
    case "k":
    case "ArrowUp":
      return { action: { type: "move", delta: -1 }, next: { pendingG: false } };
    case "Enter":
      return { action: { type: "open" }, next: { pendingG: false } };
    case "r":
      return { action: { type: "reply" }, next: { pendingG: false } };
    case "n":
      return { action: { type: "note" }, next: { pendingG: false } };
    case "a":
      return { action: { type: "assign" }, next: { pendingG: false } };
    case "e":
      return { action: { type: "solve" }, next: { pendingG: false } };
    case "z":
      return { action: { type: "snooze" }, next: { pendingG: false } };
    case "f":
      return { action: { type: "follow" }, next: { pendingG: false } };
    case "/":
      return { action: { type: "search" }, next: { pendingG: false } };
    case "h":
      return { action: { type: "help" }, next: { pendingG: false } };
    case "x":
      return { action: { type: "close-tab" }, next: { pendingG: false } };
    case "g":
      return { action: { type: "none" }, next: { pendingG: true } };
  }
  return none;
}

// ---- open ticket tabs ---------------------------------------------------------

export interface Tab {
  id: string;
  ref: string;
  title: string;
}
export const MAX_TABS = 8;

/** Open (or focus) a ticket tab; the oldest unfocused tab is dropped beyond MAX_TABS. */
export function openTab(tabs: Tab[], t: Tab): Tab[] {
  if (tabs.some((x) => x.id === t.id)) return tabs.map((x) => (x.id === t.id ? { ...x, title: t.title } : x));
  const next = [...tabs, t];
  return next.length > MAX_TABS ? next.slice(next.length - MAX_TABS) : next;
}

/** Close a tab and say which tab should be shown next (the neighbour, or none). */
export function closeTab(tabs: Tab[], id: string, active: string | null): { tabs: Tab[]; active: string | null } {
  const i = tabs.findIndex((x) => x.id === id);
  if (i < 0) return { tabs, active };
  const rest = tabs.filter((x) => x.id !== id);
  if (active !== id) return { tabs: rest, active };
  const neighbour = rest[Math.min(i, rest.length - 1)];
  return { tabs: rest, active: neighbour ? neighbour.id : null };
}

// ---- safe text formatting -------------------------------------------------------

export type Inline = { t: "text"; v: string } | { t: "bold"; v: string } | { t: "italic"; v: string } | { t: "code"; v: string } | { t: "link"; v: string; href: string } | { t: "mention"; v: string; user: string };

/** Safe link targets only: http(s), mailto and in-app absolute paths. */
export function safeHref(h: string): string | null {
  const s = h.trim();
  if (/^https?:\/\/[^\s]+$/i.test(s) || /^mailto:[^\s]+$/i.test(s)) return s;
  if (/^\/(?!\/)[^\s]*$/.test(s)) return s;
  return null;
}

const INLINE_RE = /(@\[([^\]]{1,80})\]\(user:([0-9a-fA-F-]{36})\))|(\[([^\]]{1,200})\]\(([^)\s]{1,500})\))|(\*\*([^*]{1,500})\*\*)|((?<![\w])_([^_\n]{1,500})_(?![\w]))|(`([^`]{1,300})`)/g;

/** Parse the composer's light markup into inline tokens. Never produces HTML. */
export function parseInline(src: string): Inline[] {
  const out: Inline[] = [];
  let last = 0;
  for (const m of src.matchAll(INLINE_RE)) {
    const i = m.index ?? 0;
    if (i > last) out.push({ t: "text", v: src.slice(last, i) });
    if (m[1]) out.push({ t: "mention", v: m[2], user: m[3] });
    else if (m[4]) {
      const href = safeHref(m[6]);
      out.push(href ? { t: "link", v: m[5], href } : { t: "text", v: m[0] });
    } else if (m[7]) out.push({ t: "bold", v: m[8] });
    else if (m[9]) out.push({ t: "italic", v: m[10] });
    else if (m[11]) out.push({ t: "code", v: m[12] });
    last = i + m[0].length;
  }
  if (last < src.length) out.push({ t: "text", v: src.slice(last) });
  return out;
}

export type Block = { kind: "p"; lines: Inline[][] } | { kind: "ul"; items: Inline[][] } | { kind: "ol"; items: Inline[][] } | { kind: "q"; lines: Inline[][] };

/** Split text into paragraphs and lists ("- " and "1. " lines). */
export function parseBlocks(src: string): Block[] {
  const blocks: Block[] = [];
  let cur: Block | null = null;
  for (const line of src.replace(/\r/g, "").split("\n")) {
    const qt = /^\s*>\s?(.*)$/.exec(line);
    if (qt) {
      if (!cur || cur.kind !== "q") {
        const nb: Block = { kind: "q", lines: [] };
        blocks.push(nb);
        cur = nb;
      }
      cur.lines.push(parseInline(qt[1]));
      continue;
    }
    const ul = /^\s*[-*]\s+(.*)$/.exec(line);
    const ol = /^\s*\d+[.)]\s+(.*)$/.exec(line);
    if (ul || ol) {
      const kind = ul ? "ul" : "ol";
      if (!cur || cur.kind !== kind) {
        const nb: Block = { kind, items: [] };
        blocks.push(nb);
        cur = nb;
      }
      (cur as { items: Inline[][] }).items.push(parseInline((ul ?? ol)![1]));
    } else if (line.trim() === "") {
      cur = null;
    } else {
      if (!cur || cur.kind !== "p") {
        const nb: Block = { kind: "p", lines: [] };
        blocks.push(nb);
        cur = nb;
      }
      cur.lines.push(parseInline(line));
    }
  }
  return blocks;
}

/** Wrap the selection of a textarea value with markers; returns the new value and caret range. */
export function wrapSelection(value: string, start: number, end: number, before: string, after = before): { value: string; start: number; end: number } {
  const sel = value.slice(start, end) || "text";
  const v = value.slice(0, start) + before + sel + after + value.slice(end);
  return { value: v, start: start + before.length, end: start + before.length + sel.length };
}

/** Insert a mention token for a user. */
export function mentionToken(name: string, userId: string): string {
  return `@[${name.replace(/[\]\n]/g, " ").slice(0, 80)}](user:${userId})`;
}

/** Find an open "@query" before the caret, for the mention picker. */
export function mentionQuery(value: string, caret: number): { query: string; from: number } | null {
  const m = /(?:^|\s)@([\p{L}\p{N}._-]{0,30})$/u.exec(value.slice(0, caret));
  if (!m) return null;
  return { query: m[1], from: caret - m[1].length - 1 };
}

/** Replace {{placeholders}} in a macro or canned reply. Unknown ones stay visible. */
export function fillTemplate(text: string, vars: Record<string, string>): string {
  return text.replace(/\{\{\s*([a-z_.]+)\s*\}\}/gi, (all, k: string) => (k in vars ? vars[k] : all));
}

// ---- editor <-> markup -----------------------------------------------------------

const esc = (s: string) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");

function inlineHtml(parts: Inline[]): string {
  return parts
    .map((p) => {
      switch (p.t) {
        case "bold": return `<strong>${esc(p.v)}</strong>`;
        case "italic": return `<em>${esc(p.v)}</em>`;
        case "code": return `<code>${esc(p.v)}</code>`;
        case "link": return `<a href="${esc(p.href)}">${esc(p.v)}</a>`;
        case "mention": return `<span data-ticket-mention data-id="${esc(p.user)}" data-label="${esc(p.v)}">@${esc(p.v)}</span>`;
        default: return esc(p.v);
      }
    })
    .join("");
}

/** Composer markup to the HTML the editor loads. Text is escaped; only markup the parser knows becomes tags. */
export function markupToHtml(src: string): string {
  const blocks = parseBlocks(src);
  if (!blocks.length) return "<p></p>";
  return blocks
    .map((b) => {
      if (b.kind === "p") return `<p>${b.lines.map(inlineHtml).join("<br>")}</p>`;
      if (b.kind === "q") return `<blockquote><p>${b.lines.map(inlineHtml).join("<br>")}</p></blockquote>`;
      return `<${b.kind}>${b.items.map((it) => `<li><p>${inlineHtml(it)}</p></li>`).join("")}</${b.kind}>`;
    })
    .join("");
}

export interface DocNode {
  type?: string;
  text?: string;
  attrs?: Record<string, unknown>;
  marks?: { type: string; attrs?: Record<string, unknown> }[];
  content?: DocNode[];
}

function textOf(n: DocNode): string {
  if (n.type === "ticketMention") return mentionToken(String(n.attrs?.label ?? ""), String(n.attrs?.id ?? ""));
  if (n.type === "hardBreak") return "\n";
  let t = n.text ?? "";
  if (!t && n.content) return n.content.map(textOf).join("");
  const marks = (n.marks ?? []).map((m) => m.type);
  if (marks.includes("code")) t = "`" + t.replace(/`/g, "'") + "`";
  else {
    if (marks.includes("bold")) t = "**" + t.replace(/\*\*/g, "") + "**";
    if (marks.includes("italic")) t = "_" + t.replace(/_/g, " ") + "_";
  }
  const link = (n.marks ?? []).find((m) => m.type === "link");
  if (link) {
    const href = safeHref(String(link.attrs?.href ?? ""));
    if (href) t = `[${t.replace(/[\]\n]/g, " ")}](${href})`;
  }
  return t;
}

/** Editor document back to the composer markup stored in the ticket. */
export function docToMarkup(doc: DocNode): string {
  const out: string[] = [];
  const itemText = (li: DocNode) => (li.content ?? []).map((c) => (c.content ?? []).map(textOf).join("")).join(" ").replace(/\n/g, " ");
  const block = (n: DocNode, prefix = "") => {
    switch (n.type) {
      case "paragraph": {
        const t = (n.content ?? []).map(textOf).join("");
        out.push(t.split("\n").map((l) => prefix + l).join("\n"));
        out.push("");
        break;
      }
      case "bulletList":
        (n.content ?? []).forEach((li) => out.push("- " + itemText(li)));
        out.push("");
        break;
      case "orderedList":
        (n.content ?? []).forEach((li, i) => out.push(`${i + 1}. ` + itemText(li)));
        out.push("");
        break;
      case "blockquote":
        (n.content ?? []).forEach((c) => block(c, "> "));
        break;
      default:
        (n.content ?? []).forEach((c) => block(c, prefix));
    }
  };
  (doc.content ?? []).forEach((n) => block(n));
  return out.join("\n").replace(/\n{3,}/g, "\n\n").trim();
}
