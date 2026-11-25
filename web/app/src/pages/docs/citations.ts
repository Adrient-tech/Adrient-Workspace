// Simple citations and bibliography (APA 7, MLA 9, Chicago author-date).
// Sources live in this browser's localStorage per document (and are also
// written into the bibliography the user inserts, so the text is self
// contained). Pure formatting helpers are unit tested.
export type SourceKind = "book" | "article" | "website";
export type CiteStyle = "apa" | "mla" | "chicago";

export interface Source {
  id: string;
  kind: SourceKind;
  /** "Last, First; Last, First" */
  authors: string;
  title: string;
  year: string;
  publisher?: string;
  container?: string; // journal / website name
  volume?: string;
  pages?: string;
  url?: string;
  accessed?: string;
}

export const STYLE_LABEL: Record<CiteStyle, string> = { apa: "APA 7", mla: "MLA 9", chicago: "Chicago (author-date)" };

interface Person {
  last: string;
  first: string;
}

export function parseAuthors(s: string): Person[] {
  return s
    .split(/;|\band\b|&/)
    .map((x) => x.trim())
    .filter(Boolean)
    .map((x) => {
      if (x.includes(",")) {
        const [last, ...rest] = x.split(",");
        return { last: last.trim(), first: rest.join(",").trim() };
      }
      const parts = x.split(/\s+/);
      return { last: parts.pop() ?? "", first: parts.join(" ") };
    });
}

const initials = (first: string) =>
  first
    .split(/[\s-]+/)
    .filter(Boolean)
    .map((p) => p[0].toUpperCase() + ".")
    .join(" ");

/** In-text citation, e.g. "(Smith & Lee, 2020, p. 4)" / "(Smith and Lee 4)". */
export function inTextCitation(src: Source, style: CiteStyle, page?: string): string {
  const people = parseAuthors(src.authors);
  const lasts = people.map((p) => p.last);
  const pg = page?.trim();
  const who = (amp: string) =>
    lasts.length === 0
      ? ""
      : lasts.length === 1
        ? lasts[0]
        : lasts.length === 2
          ? `${lasts[0]} ${amp} ${lasts[1]}`
          : `${lasts[0]} et al.`;
  if (style === "mla") {
    const name = who("and") || `"${shortTitle(src.title)}"`;
    return `(${name}${pg ? " " + pg : ""})`;
  }
  const name = who(style === "apa" ? "&" : "and") || shortTitle(src.title);
  const yr = src.year || "n.d.";
  if (style === "apa") return `(${name}, ${yr}${pg ? ", p. " + pg : ""})`;
  return `(${name} ${yr}${pg ? ", " + pg : ""})`;
}

const shortTitle = (t: string) => (t.split(/\s+/).length > 4 ? t.split(/\s+/).slice(0, 4).join(" ") + "…" : t);

function listApa(p: Person[]): string {
  const f = p.map((x) => `${x.last}, ${initials(x.first)}`.replace(/, $/, ""));
  if (f.length <= 1) return f[0] ?? "";
  if (f.length <= 20) return f.slice(0, -1).join(", ") + ", & " + f[f.length - 1];
  return f.slice(0, 19).join(", ") + ", … " + f[f.length - 1];
}
function listMla(p: Person[]): string {
  if (p.length === 0) return "";
  const a = `${p[0].last}, ${p[0].first}`.replace(/, $/, "");
  if (p.length === 1) return a;
  if (p.length === 2) return `${a}, and ${p[1].first} ${p[1].last}`;
  return `${a}, et al`;
}
function listChicago(p: Person[]): string {
  if (p.length === 0) return "";
  const a = [`${p[0].last}, ${p[0].first}`.replace(/, $/, ""), ...p.slice(1).map((x) => `${x.first} ${x.last}`)];
  return a.length === 1 ? a[0] : a.slice(0, -1).join(", ") + ", and " + a[a.length - 1];
}

const endDot = (s: string) => (/[.?!]$/.test(s) ? s : s + ".");

/** Reference-list entry as plain text with *italic* spans marked by "*". */
export function formatReference(src: Source, style: CiteStyle): string {
  const p = parseAuthors(src.authors);
  const yr = src.year || "n.d.";
  const url = src.url ? ` ${src.url}` : "";
  if (style === "apa") {
    const au = p.length ? endDot(listApa(p)) + " " : "";
    if (src.kind === "article")
      return `${au}(${yr}). ${endDot(src.title)} *${src.container ?? ""}*${src.volume ? `, *${src.volume}*` : ""}${src.pages ? `, ${src.pages}` : ""}.${url}`;
    if (src.kind === "website")
      return `${au}(${yr}). *${endDot(src.title)}* ${src.container ? src.container + "." : ""}${url}`.replace(/\s+/g, " ").trim();
    return `${au}(${yr}). *${endDot(src.title)}* ${src.publisher ? src.publisher + "." : ""}${url}`.replace(/\s+/g, " ").trim();
  }
  if (style === "mla") {
    const au = p.length ? endDot(listMla(p)) + " " : "";
    if (src.kind === "article")
      return `${au}"${endDot(src.title)}" *${src.container ?? ""}*${src.volume ? `, vol. ${src.volume}` : ""}, ${yr}${src.pages ? `, pp. ${src.pages}` : ""}.${url}`;
    if (src.kind === "website")
      return `${au}"${endDot(src.title)}" *${src.container ?? ""}*, ${yr},${url}.`.replace(/\s+/g, " ").replace(/,\./, ".");
    return `${au}*${endDot(src.title)}* ${src.publisher ? src.publisher + ", " : ""}${yr}.${url}`;
  }
  const au = p.length ? endDot(listChicago(p)) + " " : "";
  if (src.kind === "article")
    return `${au}${yr}. "${endDot(src.title)}" *${src.container ?? ""}*${src.volume ? ` ${src.volume}` : ""}${src.pages ? `: ${src.pages}` : ""}.${url}`;
  if (src.kind === "website")
    return `${au}${yr}. "${endDot(src.title)}" ${src.container ? src.container + "." : ""}${url}`.replace(/\s+/g, " ").trim();
  return `${au}${yr}. *${endDot(src.title)}* ${src.publisher ? src.publisher + "." : ""}${url}`.replace(/\s+/g, " ").trim();
}

const esc = (s: string) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");

/** "*x*" spans to <em>. */
export function refToHtml(text: string): string {
  return esc(text).replace(/\*([^*]+)\*/g, "<em>$1</em>");
}

/** Alphabetical (by first author, then year) bibliography HTML. */
export function bibliographyHtml(sources: Source[], style: CiteStyle): string {
  const sorted = [...sources].sort((a, b) => {
    const x = (parseAuthors(a.authors)[0]?.last ?? a.title).toLowerCase();
    const y = (parseAuthors(b.authors)[0]?.last ?? b.title).toLowerCase();
    return x.localeCompare(y) || a.year.localeCompare(b.year);
  });
  const head = style === "apa" ? "References" : style === "mla" ? "Works Cited" : "References";
  return `<h1>${head}</h1>` + sorted.map((s) => `<p>${refToHtml(formatReference(s, style))}</p>`).join("");
}

const KEY = (docId: string) => `adrient.docs.sources.${docId}`;
export function loadSources(docId: string): Source[] {
  try {
    const v = JSON.parse(localStorage.getItem(KEY(docId)) ?? "[]");
    return Array.isArray(v) ? v : [];
  } catch {
    return [];
  }
}
export function saveSources(docId: string, list: Source[]) {
  try {
    localStorage.setItem(KEY(docId), JSON.stringify(list));
  } catch {
    /* quota */
  }
}
