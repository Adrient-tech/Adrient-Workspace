import type {
  PosBalance,
  PosDiscountPreset,
  PosRegister,
  PosReport,
  PosSale,
  PosSaleInput,
  PosSettings,
  PosShift,
  PosTaxRate,
} from "./types";

const BASE = "/api/v1/pos";

async function jsonFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const resp = await fetch(`${BASE}${path}`, {
    credentials: "same-origin",
    headers: { Accept: "application/json", "Content-Type": "application/json" },
    ...init,
  });
  if (!resp.ok) {
    let msg = `HTTP ${resp.status}`;
    try {
      const body = (await resp.json()) as { message?: string };
      if (body.message) msg = body.message;
    } catch {
      /* keep the status text */
    }
    throw new Error(msg);
  }
  return (await resp.json()) as T;
}

const send = (method: string, body?: unknown): RequestInit => ({
  method,
  body: body === undefined ? undefined : JSON.stringify(body),
});

const qs = (o: Record<string, string | number | boolean | undefined>) => {
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(o)) if (v !== undefined && v !== "" && v !== false) p.set(k, String(v));
  const s = p.toString();
  return s ? `?${s}` : "";
};

// Registers and shifts
export const listRegisters = async () => (await jsonFetch<{ registers?: PosRegister[] }>("/registers")).registers ?? [];
export const createRegister = (r: Partial<PosRegister>) => jsonFetch<PosRegister>("/registers", send("POST", r));
export const updateRegister = (id: string, r: Partial<PosRegister>) => jsonFetch<PosRegister>(`/registers/${id}`, send("PATCH", r));
export const archiveRegister = (id: string) => jsonFetch<unknown>(`/registers/${id}`, send("DELETE"));
export const openShift = (registerId: string, openingCash: number) =>
  jsonFetch<PosShift>(`/registers/${registerId}/shift`, send("POST", { opening_cash: openingCash }));
export const closeShift = (id: string, countedCash: number, note: string) =>
  jsonFetch<PosShift>(`/shifts/${id}/close`, send("POST", { counted_cash: countedCash, note }));
export const cashMove = (id: string, amount: number, note: string) =>
  jsonFetch<PosShift>(`/shifts/${id}/cash`, send("POST", { amount, note }));
export const getShift = (id: string) => jsonFetch<PosShift>(`/shifts/${id}`);
export const listShifts = async (registerId = "") =>
  (await jsonFetch<{ shifts?: PosShift[] }>(`/shifts${qs({ register_id: registerId })}`)).shifts ?? [];

// Sales
export const holdSale = (s: PosSaleInput) => jsonFetch<PosSale>("/sales/hold", send("POST", s));
export const completeSale = (s: PosSaleInput) => jsonFetch<PosSale>("/sales/complete", send("POST", s));
export const getSale = (id: string) => jsonFetch<PosSale>(`/sales/${id}`);
export const deleteHeldSale = (id: string) => jsonFetch<unknown>(`/sales/${id}`, send("DELETE"));
export const listSales = async (q: { status?: string; query?: string; since?: string; until?: string; register_id?: string; limit?: number } = {}) =>
  (await jsonFetch<{ sales?: PosSale[] }>(`/sales${qs(q)}`)).sales ?? [];
export const refundSale = (
  id: string,
  body: { register_id: string; lines: { line_id: string; qty: number }[]; method: string; restock: boolean; note?: string },
) => jsonFetch<PosSale>(`/sales/${id}/refund`, send("POST", body));

// Taxes, discounts, settings
export const listTaxRates = async () => (await jsonFetch<{ rates?: PosTaxRate[] }>("/taxes")).rates ?? [];
export const createTaxRate = (t: Partial<PosTaxRate>) => jsonFetch<PosTaxRate>("/taxes", send("POST", t));
export const updateTaxRate = (id: string, t: Partial<PosTaxRate>) => jsonFetch<PosTaxRate>(`/taxes/${id}`, send("PATCH", t));
export const deleteTaxRate = (id: string) => jsonFetch<unknown>(`/taxes/${id}`, send("DELETE"));
export const listDiscountPresets = async () => (await jsonFetch<{ presets?: PosDiscountPreset[] }>("/discounts")).presets ?? [];
export const createDiscountPreset = (d: Partial<PosDiscountPreset>) => jsonFetch<PosDiscountPreset>("/discounts", send("POST", d));
export const updateDiscountPreset = (id: string, d: Partial<PosDiscountPreset>) =>
  jsonFetch<PosDiscountPreset>(`/discounts/${id}`, send("PATCH", d));
export const deleteDiscountPreset = (id: string) => jsonFetch<unknown>(`/discounts/${id}`, send("DELETE"));
export const getSettings = () => jsonFetch<PosSettings>("/settings");
export const saveSettings = (s: PosSettings) => jsonFetch<PosSettings>("/settings", send("PUT", s));

// Balances
export const listBalances = async () => (await jsonFetch<{ balances?: PosBalance[] }>("/balances")).balances ?? [];
export const getBalance = (contactId: string) => jsonFetch<PosBalance>(`/balances/${encodeURIComponent(contactId)}`);
export const adjustBalance = (contactId: string, kind: "loyalty" | "store_credit", delta: number, note: string) =>
  jsonFetch<PosBalance>(`/balances/${encodeURIComponent(contactId)}/adjust`, send("POST", { kind, delta, note }));

export const getReport = (since: string, until: string) => jsonFetch<PosReport>(`/report${qs({ since, until })}`);
