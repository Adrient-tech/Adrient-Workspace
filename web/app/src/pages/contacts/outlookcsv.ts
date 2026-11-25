import { parseCSVRows, parseGoogleCSV } from "./googlecsv";
import type { ParsedContact } from "./vcard";
import { normDate } from "./dates";

/** True when the CSV header looks like an Outlook / Outlook.com export. */
export function isOutlookHeader(header: string[]): boolean {
  const h = header.map((x) => x.trim().toLowerCase());
  return h.includes("e-mail address") || (h.includes("first name") && h.includes("last name") && !h.includes("given name"));
}

function uniq(arr: string[], v: string) {
  const t = v.trim();
  if (t && !arr.some((x) => x.toLowerCase() === t.toLowerCase())) arr.push(t);
}

/** Parse an Outlook contacts CSV export. */
export function parseOutlookCSV(text: string): ParsedContact[] {
  const rows = parseCSVRows(text);
  if (rows.length < 2) return [];
  const header = rows[0].map((h) => h.trim().toLowerCase());
  const get = (row: string[], ...names: string[]): string => {
    for (const n of names) {
      const i = header.indexOf(n.toLowerCase());
      if (i >= 0 && i < row.length && row[i].trim()) return row[i].trim();
    }
    return "";
  };
  const out: ParsedContact[] = [];
  for (const row of rows.slice(1)) {
    const first = get(row, "First Name");
    const last = get(row, "Last Name");
    const emails: string[] = [];
    for (const n of ["E-mail Address", "E-mail 2 Address", "E-mail 3 Address", "Email Address", "Email"]) uniq(emails, get(row, n));
    const phones: string[] = [];
    for (const n of ["Mobile Phone", "Primary Phone", "Home Phone", "Home Phone 2", "Business Phone", "Business Phone 2", "Other Phone", "Phone"]) uniq(phones, get(row, n));
    const labels: string[] = [];
    for (const c of get(row, "Categories").split(/[;,]/)) uniq(labels, c);
    const display = `${first} ${last}`.trim() || get(row, "Display Name", "Name") || emails[0] || "";
    if (!display && !phones.length) continue;
    const extras = {
      birthday: normDate(get(row, "Birthday")),
      anniversary: normDate(get(row, "Anniversary")),
      website: get(row, "Web Page", "Website"),
      address: [get(row, "Home Street", "Business Street"), get(row, "Home City", "Business City"), get(row, "Home Postal Code", "Business Postal Code"), get(row, "Home Country/Region", "Business Country/Region")]
        .filter(Boolean)
        .join(", "),
    };
    out.push({
      display_name: display,
      first_name: first,
      last_name: last,
      company: get(row, "Company"),
      job_title: get(row, "Job Title"),
      emails,
      phones,
      labels,
      notes: get(row, "Notes"),
      extras: extras.birthday || extras.anniversary || extras.website || extras.address ? extras : undefined,
    });
  }
  return out;
}

/** Picks the right CSV reader (Outlook or Google) from the header row. */
export function parseContactsCSV(text: string): { contacts: ParsedContact[]; source: "outlook" | "google" } {
  const rows = parseCSVRows(text);
  if (rows.length && isOutlookHeader(rows[0])) return { contacts: parseOutlookCSV(text), source: "outlook" };
  return { contacts: parseGoogleCSV(text), source: "google" };
}
