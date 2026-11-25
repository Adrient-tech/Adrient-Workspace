import type { BookingPage } from "./logic";

async function call<T>(path: string, init?: RequestInit): Promise<T> {
  const r = await fetch(path, {
    credentials: "same-origin",
    headers: { Accept: "application/json", "Content-Type": "application/json" },
    ...init,
  });
  if (!r.ok) {
    const t = (await r.text().catch(() => "")).trim();
    throw new Error(t || `HTTP ${r.status}`);
  }
  if (r.status === 204) return undefined as T;
  return (await r.json()) as T;
}

export interface Appointment {
  id: string;
  page_id: string;
  page_title: string;
  start_at: string;
  end_at: string;
  guest_name: string;
  guest_email: string;
  guest_tz: string;
  answers: Record<string, string>;
  meet_code?: string;
  status: string;
}

export const listPages = () => call<{ pages: BookingPage[]; base_url: string }>("/api/v1/booking/pages");
export const createPage = (p: Partial<BookingPage>) => call<BookingPage>("/api/v1/booking/pages", { method: "POST", body: JSON.stringify(p) });
export const updatePage = (id: string, p: Partial<BookingPage>) =>
  call<BookingPage>(`/api/v1/booking/pages/${id}`, { method: "PUT", body: JSON.stringify(p) });
export const deletePage = (id: string) => call<void>(`/api/v1/booking/pages/${id}`, { method: "DELETE" });
export const listAppointments = () => call<{ appointments: Appointment[] }>("/api/v1/booking/appointments");
export const cancelAppointment = (id: string) => call<{ ok: boolean }>(`/api/v1/booking/appointments/${id}/cancel`, { method: "POST" });

export interface PublicPage {
  slug: string;
  title: string;
  description: string;
  location: string;
  duration_min: number;
  timezone: string;
  questions: BookingPage["questions"];
  has_meet: boolean;
  owner_name: string;
  max_days_ahead: number;
}

export const publicPage = (slug: string) => call<PublicPage>(`/api/v1/public/booking/${encodeURIComponent(slug)}`);
export const publicSlots = (slug: string, from: string, to: string) =>
  call<{ slots: string[] }>(
    `/api/v1/public/booking/${encodeURIComponent(slug)}/slots?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`,
  );
export const publicBook = (
  slug: string,
  body: { start: string; name: string; email: string; tz: string; answers: Record<string, string>; website?: string },
) =>
  call<{ ok: boolean; start_at: string; end_at: string; cancel_token: string; meet_url?: string; email_sent?: boolean }>(
    `/api/v1/public/booking/${encodeURIComponent(slug)}/book`,
    { method: "POST", body: JSON.stringify(body) },
  );
export const publicAppointment = (token: string) =>
  call<{ title: string; start_at: string; end_at: string; status: string; guest_name: string; guest_tz: string }>(
    `/api/v1/public/booking/cancel/${encodeURIComponent(token)}`,
  );
export const publicCancel = (token: string) =>
  call<{ ok: boolean }>(`/api/v1/public/booking/cancel/${encodeURIComponent(token)}`, { method: "POST" });
