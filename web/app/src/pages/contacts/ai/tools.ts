/**
 * Assistant tools for Contacts. Contact fields and notes can be imported from
 * anywhere, so they are data only. Changing a contact asks the user to
 * confirm and can be undone. Merging and deleting contacts is not offered:
 * duplicates are only reported, the user merges them.
 */
import { chainUndo, clip, defineTool, guard, str, type AssistantTool } from "@/lib/ai";

export interface ContactRow {
  id: string;
  display_name: string;
  first_name: string;
  last_name: string;
  company: string;
  job_title: string;
  emails: string[];
  phones: string[];
  labels: string[];
  notes: string;
  starred: boolean;
}

export interface ContactsAdapter {
  all(): Promise<ContactRow[]>;
  updateNotes(c: ContactRow, notes: string): Promise<void>;
  selected(): ContactRow | null;
}

const row = (c: ContactRow) => ({ id: c.id, name: clip(c.display_name, 80), company: clip(c.company, 60), title: clip(c.job_title, 60), emails: c.emails.slice(0, 4), phones: c.phones.slice(0, 3), labels: c.labels.slice(0, 6), notes: clip(c.notes, 300) });

const norm = (s: string) => s.toLowerCase().replace(/\s+/g, " ").trim();
const digits = (s: string) => s.replace(/\D/g, "").slice(-10);

/** Groups of contacts that look like the same person (same email, phone or name). */
export function findDuplicates(cs: ContactRow[]): ContactRow[][] {
  const parent = new Map<string, string>(cs.map((c) => [c.id, c.id]));
  const find = (x: string): string => (parent.get(x) === x ? x : (parent.set(x, find(parent.get(x)!)), parent.get(x)!));
  const seen = new Map<string, string>();
  const link = (key: string, id: string) => {
    const o = seen.get(key);
    if (o) parent.set(find(id), find(o));
    else seen.set(key, id);
  };
  for (const c of cs) {
    c.emails.forEach((e) => e && link("e:" + norm(e), c.id));
    c.phones.forEach((p) => digits(p).length >= 7 && link("p:" + digits(p), c.id));
    const n = norm(c.display_name || `${c.first_name} ${c.last_name}`);
    if (n.includes(" ")) link("n:" + n, c.id);
  }
  const groups = new Map<string, ContactRow[]>();
  for (const c of cs) groups.set(find(c.id), [...(groups.get(find(c.id)) ?? []), c]);
  return [...groups.values()].filter((g) => g.length > 1);
}

export function createContactTools(ad: ContactsAdapter): AssistantTool[] {
  return [
    defineTool({
      name: "contacts_find",
      description: "Find contacts by name, company, email, phone, label or note text. Up to 20 matches.",
      risk: "read",
      parameters: { type: "object", properties: { query: { type: "string" } }, required: ["query"] },
      handler: (a) => guard(async () => {
        const q = norm(str(a.query));
        if (!q) return { error: "query is required" };
        const hit = (await ad.all()).filter((c) => norm([c.display_name, c.company, c.job_title, c.notes, ...c.emails, ...c.phones, ...c.labels].join(" ")).includes(q));
        return { result: { count: hit.length, contacts: hit.slice(0, 20).map(row) } };
      }),
    }),
    defineTool({
      name: "contacts_find_duplicates",
      description: "Report groups of contacts that look like duplicates (shared email, phone or full name). Read-only: tell the user which ones to merge; the assistant does not merge or delete.",
      risk: "read",
      parameters: { type: "object", properties: {} },
      handler: () => guard(async () => {
        const g = findDuplicates(await ad.all());
        return { result: { groups: g.slice(0, 20).map((x) => x.map(row)), total_groups: g.length } };
      }),
    }),
    defineTool({
      name: "contacts_selected",
      description: "The contact currently open on screen, if any, with all fields.",
      risk: "read",
      parameters: { type: "object", properties: {} },
      handler: () => guard(async () => {
        const c = ad.selected();
        return c ? { result: row(c) } : { error: "no contact is open" };
      }),
    }),
    defineTool({
      name: "contacts_update_notes",
      description: "Replace the notes of a contact (for example to add a summary or follow-up). Include the existing notes you want to keep. The user confirms first.",
      risk: "destructive",
      parameters: { type: "object", properties: { contact_id: { type: "string" }, notes: { type: "string" } }, required: ["contact_id", "notes"] },
      label: (a) => `Set notes of contact ${str(a.contact_id).slice(0, 8)} to "${clip(str(a.notes), 100)}"`,
      handler: (a) => guard(async () => {
        const c = (await ad.all()).find((x) => x.id === str(a.contact_id));
        if (!c) return { error: "contact not found" };
        const notes = clip(typeof a.notes === "string" ? a.notes : "", 4000);
        await ad.updateNotes(c, notes);
        return { result: { updated: true }, summary: `Updated notes of ${clip(c.display_name, 50)}`, undo: chainUndo([() => ad.updateNotes({ ...c, notes }, c.notes)]) };
      }),
    }),
  ];
}

export function contactsContext(c: ContactRow | null, total: number): { label: string; text: string } {
  if (!c) return { label: `${total} contacts`, text: "" };
  return { label: c.display_name || "Contact", text: `${c.display_name}\n${c.company} ${c.job_title}\n${c.emails.join(", ")}\n${clip(c.notes, 800)}` };
}

export const CONTACTS_SUGGESTIONS = ["Find duplicate contacts", "Find contacts at Acme", "Summarise this contact in two lines and add it to the notes", "Who has no email address?"];
