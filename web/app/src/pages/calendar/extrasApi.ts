export type Win = { start: string; end: string };
export interface WorkingHours {
  timezone: string;
  days: Record<string, Win[]>;
}
export interface Suggestion {
  start: string;
  end: string;
  conflicts: string[];
}

async function call<T>(path: string, init?: RequestInit): Promise<T> {
  const r = await fetch(path, { credentials: "same-origin", headers: { Accept: "application/json" }, ...init });
  if (!r.ok) throw new Error((await r.text().catch(() => "")).trim() || `HTTP ${r.status}`);
  return (await r.json()) as T;
}

export const getSettings = () => call<{ working_hours: WorkingHours }>("/api/v1/calx/settings");
export const saveSettings = (working_hours: WorkingHours) =>
  call<{ working_hours: WorkingHours }>("/api/v1/calx/settings", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ working_hours }),
  });
export const findTime = (body: { emails: string[]; from: string; to: string; duration_min: number; limit?: number }) =>
  call<{ suggestions: Suggestion[]; not_in_org: string[] }>("/api/v1/calx/find-time", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
export const importIcs = (text: string) =>
  call<{ imported: number; duplicates: number; skipped: number }>("/api/v1/calx/import", {
    method: "POST",
    headers: { "Content-Type": "text/calendar" },
    body: text,
  });
export const getFeed = (rotate = false) =>
  call<{ token: string; path: string }>(rotate ? "/api/v1/calx/feed/rotate" : "/api/v1/calx/feed", rotate ? { method: "POST" } : undefined);
export const EXPORT_URL = "/api/v1/calx/export.ics";
