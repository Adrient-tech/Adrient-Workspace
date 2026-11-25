import { useEffect, useState } from "react";
import type {
  Activity,
  Company,
  ContactLite,
  Deal,
  DealInput,
  Followup,
  HistoryRow,
  LinkedDoc,
  Person,
  Settings,
} from "./types";

const BASE = "/api/v1/crm";

async function jf<T>(url: string, init?: RequestInit): Promise<T> {
  const resp = await fetch(url, {
    credentials: "same-origin",
    headers: { Accept: "application/json", "Content-Type": "application/json" },
    ...init,
  });
  if (!resp.ok) {
    const text = (await resp.text().catch(() => "")).trim();
    throw new Error(text || `HTTP ${resp.status}`);
  }
  return (await resp.json()) as T;
}
const send = (method: string, body?: unknown): RequestInit => ({
  method,
  body: body === undefined ? undefined : JSON.stringify(body),
});

export const getSettings = () => jf<Settings>(`${BASE}/settings`);
export const saveSettings = (s: Settings) => jf<Settings>(`${BASE}/settings`, send("PUT", s));

export const listCompanies = async () => (await jf<{ companies: Company[] }>(`${BASE}/companies`)).companies ?? [];
export const saveCompany = (id: string, c: Partial<Company>) =>
  jf<Company>(id ? `${BASE}/companies/${id}` : `${BASE}/companies`, send(id ? "PUT" : "POST", c));
export const deleteCompany = (id: string) => jf<unknown>(`${BASE}/companies/${id}`, send("DELETE"));

export const listPeople = async () => (await jf<{ people: Person[] }>(`${BASE}/people`)).people ?? [];
/** Server-side search over every lead the caller may see (the startup list holds the newest 1,000). */
export const searchPeople = async (q: string, status = "", limit = 200) =>
  (await jf<{ people: Person[] }>(`${BASE}/people?${new URLSearchParams({ q, status, limit: String(limit) })}`)).people ?? [];
export const searchCompanies = async (q: string, limit = 200) =>
  (await jf<{ companies: Company[] }>(`${BASE}/companies?${new URLSearchParams({ q, limit: String(limit) })}`)).companies ?? [];
export interface StageTotal { stage: string; count: number; value: number; mrr: number }
export interface CrmSummary { by_stage: StageTotal[]; won_this_month: number; people: number; leads: number; companies: number; deals: number }
/** Dashboard totals computed on the server over all records. */
export const getSummary = () => jf<CrmSummary>(`${BASE}/summary`);
export const savePerson = (id: string, p: Partial<Person>) =>
  jf<Person>(id ? `${BASE}/people/${id}` : `${BASE}/people`, send(id ? "PUT" : "POST", p));
export const deletePerson = (id: string) => jf<unknown>(`${BASE}/people/${id}`, send("DELETE"));

export const listDeals = async () => (await jf<{ deals: Deal[] }>(`${BASE}/deals`)).deals ?? [];
export const saveDeal = (id: string, d: Partial<DealInput>) =>
  jf<Deal>(id ? `${BASE}/deals/${id}` : `${BASE}/deals`, send(id ? "PUT" : "POST", d));
export const moveDeal = (id: string, stage: string, reason = "") =>
  jf<Deal>(`${BASE}/deals/${id}/stage`, send("POST", { stage, reason }));
export const deleteDeal = (id: string) => jf<unknown>(`${BASE}/deals/${id}`, send("DELETE"));
export const listHistory = async () => (await jf<{ history: HistoryRow[] }>(`${BASE}/history`)).history ?? [];

export const listFollowups = async () => (await jf<{ followups: Followup[] }>(`${BASE}/followups`)).followups ?? [];
export const saveFollowup = (id: string, f: Partial<Followup>) =>
  jf<Followup>(id ? `${BASE}/followups/${id}` : `${BASE}/followups`, send(id ? "PUT" : "POST", f));
export const deleteFollowup = (id: string) => jf<unknown>(`${BASE}/followups/${id}`, send("DELETE"));

export const listActivities = async () => (await jf<{ activities: Activity[] }>(`${BASE}/activities`)).activities ?? [];
export const saveActivity = (a: Partial<Activity>) => jf<Activity>(`${BASE}/activities`, send("POST", a));
export const deleteActivity = (id: string) => jf<unknown>(`${BASE}/activities/${id}`, send("DELETE"));

// ---- other modules ----

export async function listContacts(): Promise<ContactLite[]> {
  try {
    const r = await jf<{ contacts: ContactLite[] }>("/api/v1/contacts");
    return (r.contacts ?? []).map((c) => ({ ...c, emails: c.emails ?? [] }));
  } catch {
    return [];
  }
}
export const createContact = (c: { display_name: string; company?: string; emails?: string[] }) =>
  jf<ContactLite>("/api/v1/contacts", send("POST", c));

/** Invoicing documents for a contact (estimates and invoices). Empty if Invoicing is unavailable. */
export async function listLinkedDocs(contactId: string): Promise<LinkedDoc[]> {
  if (!contactId) return [];
  try {
    const r = await jf<{ documents: LinkedDoc[] }>(`/api/v1/invoicing/documents?contact_id=${encodeURIComponent(contactId)}`);
    return r.documents ?? [];
  } catch {
    return [];
  }
}

/** Create a draft estimate in Invoicing for a deal (one line = the deal value). */
export async function createEstimateForDeal(deal: Deal, contact: ContactLite | undefined): Promise<{ id: string }> {
  const today = new Date().toISOString().slice(0, 10);
  return jf<{ id: string }>("/api/v1/invoicing/documents", {
    method: "POST",
    body: JSON.stringify({
      kind: "estimate",
      contact_id: contact?.id ?? "",
      client_name: contact?.display_name ?? deal.title,
      client_email: contact?.emails?.[0] ?? "",
      client_address: "",
      currency: deal.currency,
      issue_date: today,
      due_date: deal.expected_close ?? today,
      lines: deal.items && deal.items.length > 0
        ? deal.items.map((i) => ({ description: i.name, qty: i.qty, unit: "", price: i.unit_price, discount_pct: i.discount_pct, tax_rate: 0, tax_name: "" }))
        : [{ description: deal.title, qty: 1, unit: "", price: deal.value, discount_pct: 0, tax_rate: 0, tax_name: "" }],
      discount_kind: "percent",
      discount_value: 0,
      notes: `CRM deal: ${deal.title}`,
      terms: "",
      tax_note: "",
      schedule: [],
      attachments: [],
    }),
  });
}

/** Create a Calendar event for a follow-up; returns its id or "" if Calendar rejects it. */
export async function createCalendarEvent(title: string, date: string, time: string, description: string): Promise<string> {
  const start = new Date(`${date}T${time || "09:00"}:00`);
  const end = new Date(start.getTime() + 30 * 60000);
  try {
    const ev = await jf<{ id: string }>("/api/v1/calendar/events", {
      method: "POST",
      body: JSON.stringify({
        title,
        description,
        location: "",
        start_at: start.toISOString(),
        end_at: end.toISOString(),
        all_day: false,
        color: "",
        recurrence: "",
        attendees: [],
        item_type: "event",
        reminders: [10],
        status: "busy",
        visibility: "default",
        task_done: false,
      }),
    });
    return ev.id;
  } catch {
    return "";
  }
}
export const deleteCalendarEvent = (id: string) =>
  fetch(`/api/v1/calendar/events/${id}`, { method: "DELETE", credentials: "same-origin" }).catch(() => undefined);

export async function getLeaderboard(period: string) {
  const r = await fetch(`${BASE}/leaderboard?period=${encodeURIComponent(period)}`, { credentials: "same-origin", headers: { Accept: "application/json" } });
  if (!r.ok) throw new Error((await r.text().catch(() => "")).trim() || `HTTP ${r.status}`);
  return ((await r.json()) as { rows: { user_id: string; name: string; won: number; won_count: number; open: number; target: number; pct: number; activities: number }[] }).rows ?? [];
}

export interface Handoff { estimate_id: string; created: boolean; project_id?: string; links: { app: string; label: string; href: string }[]; notes: string[] }
/** Idempotent: one estimate per deal; with project=true also one delivery project in Projects. */
export const dealHandoff = (dealId: string, project: boolean) =>
  jf<Handoff>("/api/v1/bridge/crm/deal-won", send("POST", { deal_id: dealId, create_project: project }));
export const dealLinks = async (dealId: string) =>
  (await jf<{ links: { app: string; label: string; href: string }[] }>(`/api/v1/bridge/links?app=crm&type=deal&id=${encodeURIComponent(dealId)}`)).links ?? [];

/**
 * Merges server search hits into a locally loaded window: with a query, the
 * server results (over every record) replace the window; without, the window
 * is used. Returns null while a search is in flight so callers can show the
 * window meanwhile.
 */
export function useServerSearch<T>(q: string, run: (q: string) => Promise<T[]>): T[] | null {
  const [hits, setHits] = useState<T[] | null>(null);
  useEffect(() => {
    const t = q.trim();
    if (!t) {
      setHits(null);
      return;
    }
    let live = true;
    const h = setTimeout(() => {
      run(t)
        .then((r) => live && setHits(r))
        .catch(() => live && setHits(null));
    }, 250);
    return () => {
      live = false;
      clearTimeout(h);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [q]);
  return hits;
}
