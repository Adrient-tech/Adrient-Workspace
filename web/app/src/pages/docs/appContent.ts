// Insert ▸ From app / Contact / Meeting link: turns data from other Adrient
// modules into HTML the editor can insert. Pure helpers, no editor access.
import type { AppTable } from "../sheets/appData";
import type { Contact } from "../contacts/types";

export function escapeHtml(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
}

const cell = (v: string | number): string =>
  typeof v === "number" ? v.toLocaleString(undefined, { maximumFractionDigits: 2 }) : escapeHtml(v);

/** Max body rows inserted into a document (a table of thousands of rows would freeze the editor). */
export const MAX_DOC_ROWS = 200;

/** HTML for a heading-less table: bold header row, then data rows. */
export function appTableHtml(tbl: AppTable, maxRows = MAX_DOC_ROWS): { html: string; shown: number; total: number } {
  const rows = tbl.rows.slice(0, maxRows);
  const head = `<tr>${tbl.header.map((h) => `<th><p>${escapeHtml(h)}</p></th>`).join("")}</tr>`;
  const body = rows
    .map((r) => `<tr>${r.map((v) => `<td><p>${cell(v)}</p></td>`).join("")}</tr>`)
    .join("");
  return { html: `<table><tbody>${head}${body}</tbody></table><p></p>`, shown: rows.length, total: tbl.rows.length };
}

/** Name / company / email / phone block for a contact. */
export function contactBlockHtml(c: Pick<Contact, "display_name" | "first_name" | "last_name" | "company" | "job_title" | "emails" | "phones" | "notes">): string {
  const name = c.display_name || [c.first_name, c.last_name].filter(Boolean).join(" ") || c.emails?.[0] || "";
  const lines = [name, c.job_title, c.company, c.emails?.[0], c.phones?.[0]].filter((x): x is string => !!x);
  if (!lines.length) return "";
  return `<p>${lines.map(escapeHtml).join("<br>")}</p>`;
}

/** Shareable-meeting paragraph with a link. */
export function meetingLinkHtml(name: string, url: string): string {
  const u = escapeHtml(url);
  return `<p>${escapeHtml(name)}: <a href="${u}">${u}</a></p>`;
}

/** Case-insensitive contact filter over name, company and email. */
export function filterContacts<T extends Pick<Contact, "display_name" | "company" | "emails">>(list: T[], q: string): T[] {
  const s = q.trim().toLowerCase();
  if (!s) return list;
  return list.filter((c) => [c.display_name, c.company, ...(c.emails ?? [])].some((x) => (x ?? "").toLowerCase().includes(s)));
}
