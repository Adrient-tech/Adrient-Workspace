import type { Page } from "./api";

export interface TreeNode { page: Page; children: TreeNode[]; depth: number }

/** Builds the page tree of a space from the flat list (ordered by position). */
export function buildTree(pages: Page[]): TreeNode[] {
  const byId = new Map<string, TreeNode>();
  for (const p of pages) byId.set(p.id, { page: p, children: [], depth: 0 });
  const roots: TreeNode[] = [];
  for (const p of pages) {
    const n = byId.get(p.id)!;
    const parent = p.parent_id ? byId.get(p.parent_id) : undefined;
    if (parent) parent.children.push(n);
    else roots.push(n);
  }
  const setDepth = (n: TreeNode, d: number) => { n.depth = d; n.children.sort((a, b) => a.page.position - b.page.position).forEach((c) => setDepth(c, d + 1)); };
  roots.sort((a, b) => a.page.position - b.page.position).forEach((r) => setDepth(r, 0));
  return roots;
}

/** Flattens the visible rows of a tree given the set of expanded page ids. */
export function visibleRows(roots: TreeNode[], expanded: Set<string>): TreeNode[] {
  const out: TreeNode[] = [];
  const walk = (n: TreeNode) => { out.push(n); if (expanded.has(n.page.id)) n.children.forEach(walk); };
  roots.forEach(walk);
  return out;
}

/** Ancestors of a page, root first (the page itself is not included). */
export function breadcrumb(pages: Page[], id: string): Page[] {
  const byId = new Map(pages.map((p) => [p.id, p]));
  const out: Page[] = [];
  let cur = byId.get(id)?.parent_id;
  for (let i = 0; cur && i < 50; i++) {
    const p = byId.get(cur);
    if (!p) break;
    out.unshift(p);
    cur = p.parent_id;
  }
  return out;
}

/** A page may not move into itself or its own descendants. */
export function canMoveInto(pages: Page[], id: string, parentId: string): boolean {
  if (!parentId) return true;
  if (id === parentId) return false;
  return !breadcrumb(pages, parentId).some((p) => p.id === id);
}

export function pageLink(id: string): string { return `/wiki?page=${id}`; }

/** Plain text of a TipTap document (for AI tools and word counts). */
export function docText(doc: unknown): string {
  const out: string[] = [];
  const walk = (n: unknown) => {
    if (!n || typeof n !== "object") return;
    const o = n as { text?: string; type?: string; content?: unknown[] };
    if (o.text) out.push(o.text);
    o.content?.forEach(walk);
    if (["paragraph", "heading", "listItem", "taskItem", "codeBlock", "blockquote"].includes(o.type ?? "")) out.push("\n");
  };
  walk(doc);
  return out.join("").replace(/\n{3,}/g, "\n\n").trim();
}

export function wordCount(doc: unknown): number {
  const t = docText(doc);
  return t ? t.split(/\s+/).length : 0;
}

/** Paragraph(s) of text as a TipTap document, for assistant edits. */
export function textToDoc(text: string): unknown {
  const paras = text.split(/\n{2,}|\n/).map((s) => s.trim()).filter(Boolean);
  return { type: "doc", content: paras.length ? paras.map((t) => ({ type: "paragraph", content: [{ type: "text", text: t }] })) : [{ type: "paragraph" }] };
}

export function appendText(doc: unknown, text: string): unknown {
  const d = (doc as { type: string; content?: unknown[] }) ?? { type: "doc", content: [] };
  const add = (textToDoc(text) as { content: unknown[] }).content;
  const cur = (d.content ?? []).filter((b, i, a) => !(i === a.length - 1 && (b as { type: string; content?: unknown[] }).type === "paragraph" && !(b as { content?: unknown[] }).content));
  return { type: "doc", content: [...cur, ...add] };
}
