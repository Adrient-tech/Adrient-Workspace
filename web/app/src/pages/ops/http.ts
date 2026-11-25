// Small JSON client shared by the wave-6 ops apps (Shifts, Time tracking, Field service, Fleet).
// Same-origin fetch with the user's session; error bodies become Error messages.

export async function call<T = unknown>(url: string, method = "GET", body?: unknown): Promise<T> {
  const r = await fetch(url, {
    method,
    credentials: "same-origin",
    headers: { Accept: "application/json", ...(body === undefined ? {} : { "Content-Type": "application/json" }) },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!r.ok) {
    const text = (await r.text().catch(() => "")).trim();
    throw new Error(text || `Request failed (${r.status})`);
  }
  if (r.status === 204) return undefined as T;
  const text = await r.text();
  return (text ? JSON.parse(text) : undefined) as T;
}

/** Builds "?a=1&b=2" from the non-empty values. */
export function qs(o: Record<string, string | number | boolean | undefined | null>): string {
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(o)) if (v !== undefined && v !== null && v !== "") p.set(k, String(v));
  const s = p.toString();
  return s ? `?${s}` : "";
}
