import type { PosSale } from "./types";

const BASE = "/api/v1/pos/x";

export class ExtError extends Error {
  constructor(
    message: string,
    public status: number,
    public code = "",
  ) {
    super(message);
  }
}

async function jf<T>(path: string, init?: RequestInit): Promise<T> {
  const resp = await fetch(`${BASE}${path}`, {
    credentials: "same-origin",
    headers: { Accept: "application/json", "Content-Type": "application/json" },
    ...init,
  });
  if (!resp.ok) {
    let msg = `HTTP ${resp.status}`;
    let code = "";
    try {
      const b = (await resp.json()) as { message?: string; error?: string; code?: string };
      msg = b.message || b.error || msg;
      code = b.code ?? "";
    } catch {
      /* keep status text */
    }
    throw new ExtError(msg, resp.status, code);
  }
  return (await resp.json()) as T;
}

const send = (method: string, body?: unknown): RequestInit => ({ method, body: body === undefined ? undefined : JSON.stringify(body) });

export interface Tier {
  name: string;
  min_spend: number;
  multiplier: number;
}
export interface ExtSettings {
  tiers: Tier[];
  stamps: { enabled: boolean; min_sale: number; goal: number; reward_credit: number };
  discount_pin_over_pct: number;
  refund_needs_pin: boolean;
  void_needs_pin: boolean;
  receipt_footer: string;
}
export const getExt = () => jf<ExtSettings>("/settings");
export const saveExt = (s: ExtSettings) => jf<ExtSettings>("/settings", send("PUT", s));

export interface PinInfo {
  set: boolean;
  can_void: boolean;
  can_refund: boolean;
  can_discount: boolean;
}
export const getPin = () => jf<PinInfo>("/pin");
export const setPin = (pin: string, f: Omit<PinInfo, "set">) => jf<PinInfo>("/pin", send("PUT", { pin, ...f }));
export const removePin = () => jf<unknown>("/pin", send("DELETE"));
export const verifyPin = (pin: string, scope: "void" | "refund" | "discount") =>
  jf<{ ok: boolean; approver: { user_id: string; name: string } }>("/pin/verify", send("POST", { pin, scope }));

export const voidSale = (id: string, body: { reason: string; pin: string; register_id?: string }) => jf<{ sale: PosSale }>(`/sales/${id}/void`, send("POST", body));
export const refundWithPin = (id: string, body: Record<string, unknown>) => jf<{ sale: PosSale }>(`/sales/${id}/refund`, send("POST", body));
export const emailReceipt = (id: string, to: string) => jf<{ ok: boolean }>(`/sales/${id}/email`, send("POST", { to }));
export const getExtCaps = () => jf<{ email: boolean }>("/capabilities");

export interface ModOption {
  id: string;
  name: string;
  price: number;
}
export interface ModGroup {
  id: string;
  name: string;
  required: boolean;
  multi: boolean;
  options: ModOption[];
}
export const getModifiers = () => jf<{ groups: ModGroup[]; products: Record<string, string[]> }>("/modifiers");
export const saveModGroup = (id: string, g: Partial<ModGroup>) => (id ? jf<ModGroup>(`/modifiers/${id}`, send("PUT", g)) : jf<ModGroup>("/modifiers", send("POST", g)));
export const deleteModGroup = (id: string) => jf<unknown>(`/modifiers/${id}`, send("DELETE"));
export const assignModGroups = (product_id: string, group_ids: string[]) => jf<unknown>("/modifiers/assign", send("PUT", { product_id, group_ids }));

export interface Favourite {
  product_id: string;
  label: string;
  color: string;
  register_id?: string;
}
export const getFavourites = async (registerId = "") => (await jf<{ favourites: Favourite[] }>(`/favourites?register_id=${registerId}`)).favourites ?? [];
export const saveFavourites = (favourites: Favourite[]) => jf<unknown>("/favourites", send("PUT", { favourites }));

export interface GiftCard {
  id: string;
  code: string;
  initial: number;
  balance: number;
  status: string;
  expires_on: string;
  holder_name: string;
  holder_email: string;
  created_at: string;
}
export const listGiftCards = async () => (await jf<{ cards: GiftCard[] }>("/gift-cards")).cards ?? [];
export const getGiftCard = (code: string) => jf<GiftCard>(`/gift-cards/${encodeURIComponent(code)}`);
export const issueGiftCard = (b: { amount: number; holder_name?: string; holder_email?: string; expires_on?: string }) => jf<GiftCard>("/gift-cards", send("POST", b));
export const topUpGiftCard = (code: string, amount: number) => jf<GiftCard>(`/gift-cards/${encodeURIComponent(code)}/topup`, send("POST", { amount }));
export const setGiftCardStatus = (code: string, status: string) => jf<GiftCard>(`/gift-cards/${encodeURIComponent(code)}/status`, send("POST", { status }));

export interface Loyalty {
  lifetime_spend: number;
  tier: string;
  multiplier: number;
  stamps: number;
  stamp_goal: number;
  rewards: number;
}
export const getLoyalty = (contactId: string) => jf<Loyalty>(`/loyalty/${contactId}`);

export interface Store {
  id: string;
  name: string;
  address: string;
  phone: string;
  receipt_header: string;
  receipt_footer: string;
  active: boolean;
}
export const getStores = () => jf<{ stores: Store[]; registers: Record<string, string> }>("/stores");
export const saveStore = (id: string, s: Partial<Store>) => (id ? jf<Store>(`/stores/${id}`, send("PUT", s)) : jf<Store>("/stores", send("POST", s)));
export const assignStore = (register_id: string, store_id: string) => jf<unknown>("/stores/assign", send("PUT", { register_id, store_id }));

export interface ZReport {
  shift_id: string;
  register: string;
  store: string;
  opened_at: string;
  closed_at: string;
  sales: number;
  gross: number;
  discounts: number;
  tax: number;
  refunds: number;
  refund_count: number;
  voids: number;
  void_total: number;
  net: number;
  by_method: { method: string; amount: number; count: number }[];
  refund_by_method: { method: string; amount: number; count: number }[];
  opening_cash: number;
  cash_sales: number;
  cash_refunds: number;
  paid_in_out: number;
  expected_cash: number;
  counted_cash: number;
  variance: number;
  closed: boolean;
}
export const getZReport = (shiftId: string) => jf<ZReport>(`/zreport?shift_id=${shiftId}`);

export interface ProfitSummary {
  net: number;
  known_net: number;
  cost: number;
  profit: number;
  margin: number | null;
  coverage: number;
  known_lines: number;
  unknown_lines: number;
  any: boolean;
  top_by_profit: { key: string; name: string; net: number; cost: number; profit: number }[] | null;
}
export const getProfit = (since: string, until: string) => jf<ProfitSummary>(`/profit?since=${encodeURIComponent(since)}&until=${encodeURIComponent(until)}`);
