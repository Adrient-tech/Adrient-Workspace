/** Helpers of the Stories studio (plain functions, unit tested). */

const esc = (s: string) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");

/** Turns assistant text (Markdown-ish) into the simple HTML the editor stores. */
export function textToHtml(text: string): string {
  const blocks = text.replace(/\r/g, "").trim().split(/\n{2,}/);
  const out: string[] = [];
  for (const raw of blocks) {
    const b = raw.trim();
    if (!b) continue;
    const lines = b.split("\n");
    if (lines.every((l) => /^\s*[-*•]\s+/.test(l))) {
      out.push("<ul>" + lines.map((l) => `<li>${inline(l.replace(/^\s*[-*•]\s+/, ""))}</li>`).join("") + "</ul>");
    } else if (/^#{1,3}\s+/.test(b)) {
      out.push(`<h2>${inline(b.replace(/^#{1,3}\s+/, "").split("\n")[0])}</h2>`);
      const rest = lines.slice(1).join("\n").trim();
      if (rest) out.push(`<p>${inline(rest).replace(/\n/g, "<br>")}</p>`);
    } else {
      out.push(`<p>${inline(b).replace(/\n/g, "<br>")}</p>`);
    }
  }
  return out.join("");
}

function inline(s: string): string {
  return esc(s).replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>").replace(/(^|[\s(])\*([^*\n]+)\*/g, "$1<em>$2</em>");
}

/** Plain text of editor HTML, one paragraph per line (what the narrator reads). */
export function htmlToPlain(html: string): string {
  const d = new DOMParser().parseFromString(html.replace(/<(br|\/p|\/h[1-3]|\/li|\/blockquote)\s*\/?>/gi, "$&\n"), "text/html");
  return (d.body.textContent ?? "").split("\n").map((l) => l.replace(/\s+/g, " ").trim()).filter(Boolean).join("\n");
}

/** Character names used with [Name] at the start of a paragraph. */
export function charactersIn(plain: string): string[] {
  const names = new Set<string>();
  for (const line of plain.split("\n")) {
    const m = /^\[([^\[\]]{1,40})\]/.exec(line.trim());
    if (m && !/^(narrator|\/)$/i.test(m[1].trim())) names.add(m[1].trim());
  }
  return [...names];
}

/** Rough reading time at 160 words per minute. */
export function readingMinutes(plain: string): number {
  const words = plain.split(/\s+/).filter(Boolean).length;
  return Math.max(0, Math.round(words / 160));
}

export function wordCount(plain: string): number {
  return plain.split(/\s+/).filter(Boolean).length;
}

/** Value for <input type="datetime-local"> from an ISO string (local time). */
export function toLocalInput(iso?: string): string {
  const d = iso ? new Date(iso) : new Date(Date.now() + 3600_000);
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`;
}
