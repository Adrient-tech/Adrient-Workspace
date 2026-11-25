/**
 * A small, safe Markdown → HTML converter for text the assistant inserts
 * into a document: headings, bullet/numbered lists, quotes, paragraphs,
 * **bold**, *italic* and `code`. All input is HTML-escaped first, so model
 * output can never inject markup, scripts or links.
 */
function esc(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
}

function inline(s: string): string {
  return esc(s)
    .replace(/`([^`]+)`/g, "<code>$1</code>")
    .replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>")
    .replace(/(^|[^*])\*([^*\s][^*]*)\*/g, "$1<em>$2</em>");
}

export function markdownToHtml(md: string): string {
  const out: string[] = [];
  let list: "ul" | "ol" | null = null;
  let para: string[] = [];
  const flushPara = () => {
    if (para.length) out.push(`<p>${para.map(inline).join("<br>")}</p>`);
    para = [];
  };
  const closeList = () => {
    if (list) out.push(`</${list}>`);
    list = null;
  };
  for (const raw of md.replace(/\r\n?/g, "\n").split("\n")) {
    const line = raw.trimEnd();
    let m: RegExpExecArray | null;
    if (!line.trim()) {
      flushPara();
      closeList();
    } else if ((m = /^(#{1,3})\s+(.*)$/.exec(line))) {
      flushPara();
      closeList();
      out.push(`<h${m[1].length}>${inline(m[2])}</h${m[1].length}>`);
    } else if ((m = /^\s*[-*•]\s+(.*)$/.exec(line)) || (m = /^\s*\d+[.)]\s+(.*)$/.exec(line))) {
      flushPara();
      const kind = /^\s*\d/.test(line) ? "ol" : "ul";
      if (list !== kind) {
        closeList();
        out.push(`<${kind}>`);
        list = kind;
      }
      out.push(`<li><p>${inline(m[1])}</p></li>`);
    } else if ((m = /^>\s?(.*)$/.exec(line))) {
      flushPara();
      closeList();
      out.push(`<blockquote><p>${inline(m[1])}</p></blockquote>`);
    } else {
      closeList();
      para.push(line);
    }
  }
  flushPara();
  closeList();
  return out.join("");
}

/** True when the text is one plain paragraph (insert inline, no block). */
export function isInlineText(md: string): boolean {
  return !/\n|^#{1,3}\s|^\s*([-*•]|\d+[.)])\s|^>/.test(md.trim());
}
