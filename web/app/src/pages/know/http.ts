// Small JSON client shared by the knowledge apps (records, wiki, contracts, signatures, legal, cap table).
export class ApiError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.status = status;
  }
}

export async function jf<T>(url: string, init?: { method?: string; body?: unknown; raw?: BodyInit; headers?: Record<string, string> }): Promise<T> {
  const hasBody = init?.body !== undefined;
  const resp = await fetch(url, {
    method: init?.method ?? "GET",
    credentials: "same-origin",
    headers: { Accept: "application/json", ...(hasBody ? { "Content-Type": "application/json" } : {}), ...(init?.headers ?? {}) },
    body: hasBody ? JSON.stringify(init!.body) : init?.raw,
  });
  const text = await resp.text();
  if (!resp.ok) {
    let msg = text.trim();
    try {
      const j = JSON.parse(text);
      msg = j.error || j.message || msg;
    } catch {
      /* plain text */
    }
    throw new ApiError(msg || `HTTP ${resp.status}`, resp.status);
  }
  return (text ? JSON.parse(text) : {}) as T;
}

export const errText = (e: unknown): string => (e instanceof Error ? e.message : String(e));

/** Formats a number as money with the currency symbol; exact 2-decimals. */
export function money(n: number | null | undefined, currency = "INR"): string {
  if (n === null || n === undefined || Number.isNaN(n)) return "";
  try {
    return new Intl.NumberFormat("en-IN", { style: "currency", currency, minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(n);
  } catch {
    return n.toFixed(2);
  }
}

export function fmtDate(s: string | null | undefined): string {
  if (!s) return "";
  const d = new Date(s.length === 10 ? s + "T00:00:00" : s);
  return Number.isNaN(d.getTime()) ? s : d.toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" });
}
