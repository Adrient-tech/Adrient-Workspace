import type { ParsedContact } from "./vcard";

const EMAIL = /[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}/g;
const PHONE = /\+?\d[\d\s().-]{6,}\d/g;
const URL_RE = /\b(?:https?:\/\/|www\.)[^\s,;<>]+/gi;
const TITLE_WORDS =
  /\b(manager|director|engineer|developer|designer|ceo|cto|cfo|coo|founder|co-founder|president|vp|head|lead|officer|consultant|analyst|executive|partner|owner|specialist|architect|administrator|assistant|accountant|sales|marketing)\b/i;

function blank(): ParsedContact {
  return { display_name: "", first_name: "", last_name: "", company: "", job_title: "", emails: [], phones: [], labels: [], notes: "" };
}

function setName(c: ParsedContact, name: string) {
  const n = name.replace(/["']/g, "").replace(/\s+/g, " ").trim();
  if (!n) return;
  c.display_name = n;
  const parts = n.split(" ");
  c.first_name = parts[0];
  c.last_name = parts.slice(1).join(" ");
}

/** "jane.doe@x.com" -> "Jane Doe" when nothing better is known. */
export function nameFromEmail(email: string): string {
  const local = email.split("@")[0].replace(/[0-9]+/g, " ").split(/[._\-\s]+/).filter(Boolean);
  return local.map((w) => w[0].toUpperCase() + w.slice(1).toLowerCase()).join(" ");
}

function pushUniq(arr: string[], v: string) {
  const t = v.trim();
  if (t && !arr.some((x) => x.toLowerCase() === t.toLowerCase())) arr.push(t);
}

/** One line of recipients: `Jane Doe <jane@x.com>, bob@y.org; "Lee, Ann" <ann@z.io>`. */
function parseRecipients(line: string): ParsedContact[] | null {
  if (!line.includes("@")) return null;
  const parts: string[] = [];
  let cur = "";
  let quoted = false;
  for (const ch of line) {
    if (ch === '"') quoted = !quoted;
    if ((ch === "," || ch === ";") && !quoted) {
      parts.push(cur);
      cur = "";
    } else cur += ch;
  }
  parts.push(cur);
  const items = parts.map((p) => p.trim()).filter(Boolean);
  const out: ParsedContact[] = [];
  for (const p of items) {
    const m = p.match(EMAIL);
    if (!m || m.length !== 1) return null;
    const name = p.replace(EMAIL, "").replace(/[<>()]/g, "").replace(/"/g, "").trim();
    if (name.split(" ").length > 5) return null;
    const c = blank();
    c.emails.push(m[0]);
    setName(c, name || nameFromEmail(m[0]));
    out.push(c);
  }
  return out;
}

function csvRow(line: string): ParsedContact | null {
  const c = blank();
  for (const p of line.split(",").map((x) => x.trim()).filter(Boolean)) {
    const e = p.match(EMAIL);
    if (e) pushUniq(c.emails, e[0]);
    else if (/^\+?[\d\s().-]{7,}$/.test(p)) pushUniq(c.phones, p);
    else if (!c.display_name) setName(c, p);
    else if (!c.company) c.company = p;
  }
  if (!c.display_name && c.emails[0]) setName(c, nameFromEmail(c.emails[0]));
  return c.display_name || c.emails.length || c.phones.length ? c : null;
}

function signature(lines: string[]): ParsedContact {
  const c = blank();
  const text: string[] = [];
  for (const raw of lines) {
    if (/^(--+|best regards?|regards|thanks|sincerely|cheers|sent from)\b/i.test(raw)) continue;
    let l = raw;
    l.match(EMAIL)?.forEach((e) => pushUniq(c.emails, e));
    l = l.replace(EMAIL, " ");
    const urls = l.match(URL_RE);
    if (urls) {
      c.notes = [c.notes, ...urls].filter(Boolean).join("\n");
      l = l.replace(URL_RE, " ");
    }
    const phones = l.match(PHONE);
    phones?.forEach((p) => pushUniq(c.phones, p));
    l = l.replace(PHONE, " ").replace(/\b(?:tel|phone|mobile|cell|email|e-mail|web|website|m|t|e|w)\s*[:.]/gi, " ").replace(/[<>|]/g, " ").replace(/\s+/g, " ").trim();
    if (l && /[A-Za-z]/.test(l)) text.push(l);
  }
  for (const l of text) {
    if (!c.display_name && !TITLE_WORDS.test(l) && l.split(" ").length <= 4 && /^[\p{L} .'-]+$/u.test(l)) setName(c, l);
    else if (!c.job_title && TITLE_WORDS.test(l)) {
      const [t, co] = l.split(/\s*(?:,|\bat\b|@|\|)\s*/i);
      c.job_title = t.trim();
      if (co && !c.company) c.company = co.trim();
    } else if (!c.company && l.length < 80) c.company = l;
  }
  if (!c.display_name && c.emails[0]) setName(c, nameFromEmail(c.emails[0]));
  return c;
}

/**
 * Turns pasted text into contacts. Blank lines separate people. Understands
 * recipient lists ("Name <email>, ..."), rows ("Name, email, phone, company")
 * and e-mail signature blocks.
 */
export function parseSmart(text: string): ParsedContact[] {
  const clean = text.replace(/\r/g, "").trim();
  if (!clean) return [];
  const out: ParsedContact[] = [];
  for (const block of clean.split(/\n\s*\n/)) {
    const lines = block.split("\n").map((l) => l.trim()).filter(Boolean);
    if (!lines.length) continue;
    const lists = lines.map(parseRecipients);
    if (lists.every((x) => x !== null)) {
      out.push(...(lists as ParsedContact[][]).flat());
      continue;
    }
    const rowLike = (l: string) => l.includes(",") && l.length < 200 && (l.includes("@") || (l.match(/\d/g) ?? []).length >= 7);
    if (lines.every(rowLike)) {
      for (const l of lines) {
        const r = csvRow(l);
        if (r) out.push(r);
      }
      continue;
    }
    out.push(signature(lines));
  }
  return out.filter((c) => c.display_name || c.emails.length || c.phones.length);
}
