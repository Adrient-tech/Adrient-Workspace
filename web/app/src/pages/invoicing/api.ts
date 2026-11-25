import type { Business, ClientRow, DocInput, InvDocument, Item, Line, Payment, Recurring, Report, Settings, TaxRate } from "./types";

const BASE = "/api/v1/invoicing";

export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
  ) {
    super(message);
  }
}

async function jf<T>(url: string, init?: RequestInit): Promise<T> {
  const resp = await fetch(url, {
    credentials: "same-origin",
    headers: { Accept: "application/json", "Content-Type": "application/json" },
    ...init,
  });
  if (!resp.ok) {
    const text = (await resp.text().catch(() => "")).trim();
    throw new ApiError(text || `HTTP ${resp.status}`, resp.status);
  }
  return (await resp.json()) as T;
}

const send = (method: string, body?: unknown): RequestInit => ({
  method,
  body: body === undefined ? undefined : JSON.stringify(body),
});

export const getSettings = () => jf<Settings>(`${BASE}/settings`);
export const saveSettings = (s: Settings) => jf<Settings>(`${BASE}/settings`, send("PUT", s));
export const getCapabilities = () => jf<{ email: boolean; stock: boolean; payment_gateway?: boolean; payment_gateway_mode?: string }>(`${BASE}/capabilities`);

export const listItems = async () => (await jf<{ items: Item[] }>(`${BASE}/items`)).items ?? [];
export const createItem = (i: Partial<Item>) => jf<Item>(`${BASE}/items`, send("POST", i));
export const updateItem = (id: string, i: Partial<Item>) => jf<Item>(`${BASE}/items/${id}`, send("PUT", i));
export const deleteItem = (id: string) => jf<unknown>(`${BASE}/items/${id}`, send("DELETE"));

export async function listDocuments(q: { kind?: string; status?: string; q?: string; contact_id?: string; trash?: boolean; limit?: number } = {}) {
  return (await listDocumentsPage(q)).items;
}

/** One server page of documents (100 by default, 500 max); `next` is the next page number or "". */
export async function listDocumentsPage(q: { kind?: string; status?: string; q?: string; contact_id?: string; trash?: boolean; limit?: number; page?: number } = {}) {
  const p = new URLSearchParams();
  if (q.limit) p.set("limit", String(q.limit));
  if (q.page) p.set("page", String(q.page));
  if (q.kind) p.set("kind", q.kind);
  if (q.status) p.set("status", q.status);
  if (q.q) p.set("q", q.q);
  if (q.contact_id) p.set("contact_id", q.contact_id);
  if (q.trash) p.set("trash", "1");
  const s = p.toString();
  const r = await jf<{ documents: InvDocument[]; next_cursor?: string }>(`${BASE}/documents${s ? `?${s}` : ""}`);
  return { items: r.documents ?? [], next: r.next_cursor ?? "" };
}
export const getDocument = (id: string) => jf<InvDocument>(`${BASE}/documents/${id}`);
export const createDocument = (d: Partial<DocInput>) => jf<InvDocument>(`${BASE}/documents`, send("POST", d));
export const updateDocument = (id: string, d: Partial<DocInput>) => jf<InvDocument>(`${BASE}/documents/${id}`, send("PUT", d));
export const trashDocument = (id: string) => jf<unknown>(`${BASE}/documents/${id}`, send("DELETE"));
export const purgeDocument = (id: string) => jf<unknown>(`${BASE}/documents/${id}?purge=1`, send("DELETE"));
export const restoreDocument = (id: string) => jf<unknown>(`${BASE}/documents/${id}/restore`, send("POST"));
export const duplicateDocument = (id: string) => jf<InvDocument>(`${BASE}/documents/${id}/duplicate`, send("POST"));
export const convertEstimate = (id: string) => jf<InvDocument>(`${BASE}/documents/${id}/convert`, send("POST"));
export const setEstimateStatus = (id: string, status: string) =>
  jf<InvDocument>(`${BASE}/documents/${id}/status`, send("POST", { status }));

export interface SendResult {
  document: InvDocument;
  emailed: boolean;
  stock_warnings: string[];
}
export const sendDocument = (id: string, body: { message?: string; mark_only?: boolean }) =>
  jf<SendResult>(`${BASE}/documents/${id}/send`, send("POST", { ...body, origin: window.location.origin }));
export const remindDocument = (id: string, message?: string) =>
  jf<SendResult>(`${BASE}/documents/${id}/remind`, send("POST", { message, origin: window.location.origin }));
export const addPayment = (id: string, p: Partial<Payment>) => jf<InvDocument>(`${BASE}/documents/${id}/payments`, send("POST", p));
export const deletePayment = (id: string, payId: string) => jf<InvDocument>(`${BASE}/documents/${id}/payments/${payId}`, send("DELETE"));
export const listPayments = async (from = "", to = "") =>
  (await jf<{ payments: Payment[] }>(`${BASE}/payments?from=${from}&to=${to}`)).payments ?? [];

export const listRecurring = async () => (await jf<{ recurring: Recurring[] }>(`${BASE}/recurring`)).recurring ?? [];
export const createRecurring = (r: Partial<Recurring>) => jf<Recurring>(`${BASE}/recurring`, send("POST", r));
export const updateRecurring = (id: string, r: Partial<Recurring>) => jf<Recurring>(`${BASE}/recurring/${id}`, send("PUT", r));
export const deleteRecurring = (id: string) => jf<unknown>(`${BASE}/recurring/${id}`, send("DELETE"));
export const runRecurring = async () =>
  (await jf<{ generated: InvDocument[] }>(`${BASE}/recurring/run?origin=${encodeURIComponent(window.location.origin)}`, send("POST"))).generated ?? [];

export const listClients = async () => (await jf<{ clients: ClientRow[] }>(`${BASE}/clients`)).clients ?? [];
export const getReport = (from = "", to = "") => jf<Report>(`${BASE}/report?from=${from}&to=${to}`);

export const getPublic = (token: string) =>
  jf<PublicData>(`/api/v1/public/invoices/${encodeURIComponent(token)}`);

// Other modules (Contacts, Inventory, POS taxes). Failures degrade to empty lists.
export interface ContactLite {
  id: string;
  display_name: string;
  company: string;
  emails: string[];
}
export async function listContacts(): Promise<ContactLite[]> {
  try {
    return (await jf<{ contacts: ContactLite[] }>("/api/v1/contacts")).contacts ?? [];
  } catch {
    return [];
  }
}
export const createContact = (c: { display_name: string; company?: string; emails?: string[] }) =>
  jf<ContactLite>("/api/v1/contacts", send("POST", c));
export interface ProductLite {
  id: string;
  name: string;
  sku: string;
  price: number;
  unit?: string;
}
export async function listProducts(q: string): Promise<ProductLite[]> {
  try {
    return ((await jf<{ products: ProductLite[] }>(`/api/v1/inventory/products?query=${encodeURIComponent(q)}&flat=true`)).products ?? []).slice(0, 20);
  } catch {
    return [];
  }
}
export async function listTaxRates(): Promise<TaxRate[]> {
  try {
    return (await jf<{ rates?: TaxRate[] }>("/api/v1/pos/taxes")).rates ?? [];
  } catch {
    return [];
  }
}

// ---- wave 3: sales ----
export const newVersion = (id: string) => jf<InvDocument>(`${BASE}/documents/${id}/version`, send("POST"));
export const listVersions = async (id: string) => (await jf<{ versions: InvDocument[] }>(`${BASE}/documents/${id}/versions`)).versions ?? [];
export const estimateToOrder = (id: string) => jf<SalesOrder>(`${BASE}/documents/${id}/order`, send("POST"));
export const createCreditNote = (id: string, body: { lines?: Line[]; reason?: string; apply?: boolean }) =>
  jf<InvDocument>(`${BASE}/documents/${id}/credit-note`, send("POST", body));
export const applyCreditNote = (id: string, invoice_id: string, amount = 0) =>
  jf<InvDocument>(`${BASE}/documents/${id}/apply`, send("POST", { invoice_id, amount }));
export const refundCreditNote = (id: string, body: { amount?: number; method?: string; reference?: string }) =>
  jf<InvDocument>(`${BASE}/documents/${id}/refund`, send("POST", body));
export const paymentLink = (id: string) => jf<{ url: string }>(`${BASE}/documents/${id}/payment-link`, send("POST"));
export const getEInvoice = (id: string) => jf<{ ready: boolean; issues: string[]; json: unknown }>(`${BASE}/documents/${id}/einvoice`);
export const getWhatsApp = (id: string, phone = "") =>
  jf<{ text: string; url: string }>(`${BASE}/documents/${id}/whatsapp?origin=${encodeURIComponent(window.location.origin)}&phone=${encodeURIComponent(phone)}`);
export interface BulkResult {
  id: string;
  ok: boolean;
  error?: string;
}
export const bulkAction = async (ids: string[], action: string) =>
  (await jf<{ results: BulkResult[] }>(`${BASE}/documents/bulk`, send("POST", { ids, action, origin: window.location.origin }))).results;

export interface GatewayStatus {
  configured: boolean;
  source: string;
  key_id: string;
  webhook_secret: boolean;
  mode: string;
}
export const getGateway = () => jf<GatewayStatus>(`${BASE}/gateway`);
export const saveGateway = (b: { key_id: string; key_secret: string; webhook_secret: string }) => jf<GatewayStatus>(`${BASE}/gateway`, send("PUT", b));

export interface PriceRow {
  price_list_id?: string;
  product_id: string;
  contact_id: string;
  min_qty: number;
  price: number;
}
export interface PriceList {
  id: string;
  name: string;
  currency: string;
  is_default: boolean;
  active: boolean;
  customers: string[];
  rows: PriceRow[];
}
export const listPriceLists = async () => (await jf<{ price_lists: PriceList[] }>(`${BASE}/price-lists`)).price_lists ?? [];
export const savePriceList = (id: string, p: Partial<PriceList>) =>
  id ? jf<PriceList>(`${BASE}/price-lists/${id}`, send("PUT", p)) : jf<PriceList>(`${BASE}/price-lists`, send("POST", p));
export const deletePriceList = (id: string) => jf<unknown>(`${BASE}/price-lists/${id}`, send("DELETE"));
export interface DiscountRule {
  id: string;
  name: string;
  scope: "line" | "order";
  kind: "percent" | "amount";
  value: number;
  min_qty: number;
  min_amount: number;
  product_id: string;
  contact_id: string;
  valid_from: string;
  valid_to: string;
  priority: number;
  active: boolean;
}
export const listDiscountRules = async () => (await jf<{ rules: DiscountRule[] }>(`${BASE}/discount-rules`)).rules ?? [];
export const saveDiscountRule = (id: string, r: Partial<DiscountRule>) =>
  id ? jf<DiscountRule>(`${BASE}/discount-rules/${id}`, send("PUT", r)) : jf<DiscountRule>(`${BASE}/discount-rules`, send("POST", r));
export const deleteDiscountRule = (id: string) => jf<unknown>(`${BASE}/discount-rules/${id}`, send("DELETE"));
export interface PricingResult {
  lines: Line[];
  discount_kind: "percent" | "amount";
  discount_value: number;
  sources: string[];
  applied: { rule: string; scope: string; line: number; value: number; kind: string }[];
}
export const resolvePricing = (body: { contact_id: string; price_list_id?: string; date?: string; lines: Line[] }) =>
  jf<PricingResult>(`${BASE}/pricing/resolve`, send("POST", body));

export interface LineProgress {
  ref: string;
  ordered: number;
  delivered: number;
  packed: number;
  invoiced: number;
  to_pack: number;
  to_invoice: number;
  billable: number;
}
export interface DeliveryLine {
  ref: string;
  description: string;
  unit: string;
  product_id: string;
  hsn_sac: string;
  qty: number;
}
export interface Delivery {
  id: string;
  order_id: string;
  order_number: string;
  number: string;
  status: "draft" | "dispatched" | "cancelled";
  lines: DeliveryLine[];
  ship_to: string;
  carrier: string;
  tracking: string;
  notes: string;
  delivery_date: string;
  stock_moved: boolean;
  client_name: string;
  client_address: string;
  created_at: string;
}
export interface SalesOrder {
  id: string;
  number: string;
  status: "draft" | "confirmed" | "cancelled";
  contact_id: string;
  client_name: string;
  client_email: string;
  client_address: string;
  client_gstin: string;
  ship_to: string;
  currency: string;
  order_date: string;
  expected_date: string;
  place_of_supply: string;
  lines: Line[];
  discount_kind: "percent" | "amount";
  discount_value: number;
  subtotal: number;
  discount_total: number;
  tax_total: number;
  total: number;
  notes: string;
  terms: string;
  progress: LineProgress[];
  fulfilment: "not_delivered" | "partially_delivered" | "delivered";
  billing: "not_invoiced" | "partially_invoiced" | "invoiced";
  deliveries: Delivery[];
  invoices: { id: string; number: string; total: number; status: string }[];
  estimate_id: string;
  estimate_number: string;
}
export const listOrders = async (status = "", q = "") =>
  (await jf<{ orders: SalesOrder[] }>(`${BASE}/orders?status=${status}&q=${encodeURIComponent(q)}`)).orders ?? [];
export const getOrder = (id: string) => jf<SalesOrder>(`${BASE}/orders/${id}`);
export const saveOrder = (id: string, o: Partial<SalesOrder>) =>
  id ? jf<SalesOrder>(`${BASE}/orders/${id}`, send("PUT", o)) : jf<SalesOrder>(`${BASE}/orders`, send("POST", o));
export const setOrderStatus = (id: string, status: string) => jf<SalesOrder>(`${BASE}/orders/${id}/status`, send("POST", { status }));
export const createDelivery = (orderId: string, body: { lines: { ref: string; qty: number }[]; ship_to?: string; carrier?: string; tracking?: string; notes?: string }) =>
  jf<Delivery>(`${BASE}/orders/${orderId}/deliveries`, send("POST", body));
export const invoiceOrder = (orderId: string, body: { mode?: string; lines?: { ref: string; qty: number }[] }) =>
  jf<InvDocument>(`${BASE}/orders/${orderId}/invoice`, send("POST", body));
export const listDeliveries = async () => (await jf<{ deliveries: Delivery[] }>(`${BASE}/deliveries`)).deliveries ?? [];
export const dispatchDelivery = (id: string) => jf<{ delivery: Delivery; stock_warnings: string[] }>(`${BASE}/deliveries/${id}/dispatch`, send("POST", {}));
export const cancelDelivery = (id: string) => jf<Delivery>(`${BASE}/deliveries/${id}/cancel`, send("POST"));

export interface AgingRow {
  customer_key: string;
  customer: string;
  email: string;
  current: number;
  d1_30: number;
  d31_60: number;
  d61_90: number;
  d90_plus: number;
  unapplied_credit: number;
  total: number;
  invoices: number;
}
export const getAging = async () => (await jf<{ rows: AgingRow[] }>(`${BASE}/customers/aging`)).rows ?? [];
export interface Statement {
  customer: string;
  email: string;
  address: string;
  currency: string;
  from: string;
  to: string;
  opening: number;
  closing: number;
  overdue: number;
  lines: { date: string; type: string; number: string; description: string; debit: number; credit: number; balance: number }[];
}
export const getStatement = (customer: string, from = "", to = "") =>
  jf<Statement>(`${BASE}/statement?customer=${encodeURIComponent(customer)}&from=${from}&to=${to}`);
export const portalLink = (customer: string) => jf<{ token: string; path: string }>(`${BASE}/portal-link`, send("POST", { customer }));

export interface ReminderLogRow {
  document_id: string;
  number: string;
  client_name: string;
  rule: string;
  channel: string;
  status: string;
  detail: string;
  at: string;
}
export const getReminderLog = async () => (await jf<{ log: ReminderLogRow[] }>(`${BASE}/reminders/log`)).log ?? [];
export const runReminders = () => jf<{ sent: number; email: boolean }>(`${BASE}/reminders/run?origin=${encodeURIComponent(window.location.origin)}`, send("POST"));

export interface Plan {
  id: string;
  name: string;
  description: string;
  interval: "weekly" | "monthly" | "quarterly" | "yearly";
  price: number;
  tax_rate: number;
  hsn_sac: string;
  currency: string;
  active: boolean;
}
export interface Subscription {
  id: string;
  plan_id: string;
  plan_name: string;
  contact_id: string;
  client_name: string;
  client_email: string;
  place_of_supply: string;
  quantity: number;
  status: "active" | "past_due" | "paused" | "cancelled";
  start_date: string;
  next_renewal: string;
  end_date: string;
  cancel_at_period_end: boolean;
  dunning_days: number;
  renewals: number;
  price: number;
  interval: string;
}
export const listPlans = async () => (await jf<{ plans: Plan[] }>(`${BASE}/plans`)).plans ?? [];
export const savePlan = (id: string, p: Partial<Plan>) => (id ? jf<Plan>(`${BASE}/plans/${id}`, send("PUT", p)) : jf<Plan>(`${BASE}/plans`, send("POST", p)));
export const listSubscriptions = async () => (await jf<{ subscriptions: Subscription[] }>(`${BASE}/subscriptions`)).subscriptions ?? [];
export const createSubscription = (s: Partial<Subscription>) => jf<Subscription>(`${BASE}/subscriptions`, send("POST", s));
export const subscriptionAction = (id: string, action: string) => jf<Subscription>(`${BASE}/subscriptions/${id}/action`, send("POST", { action }));
export const runSubscriptions = () => jf<{ invoiced: InvDocument[]; past_due: number; paused: number; resumed: number; ended: number }>(`${BASE}/subscriptions/run`, send("POST"));

// Public (signed-out) actions on a share link.
export type PublicData = { document: InvDocument; business: Business; can_pay_online?: boolean };
export const publicAccept = (token: string, name: string, agree: boolean) =>
  jf<{ document: InvDocument }>(`/api/v1/public/invoices/${encodeURIComponent(token)}/accept`, send("POST", { name, agree }));
export const publicDecline = (token: string, reason: string) =>
  jf<{ document: InvDocument }>(`/api/v1/public/invoices/${encodeURIComponent(token)}/decline`, send("POST", { reason }));
export const publicPay = (token: string) => jf<{ url: string }>(`/api/v1/public/invoices/${encodeURIComponent(token)}/pay`, send("POST", {}));
export interface PortalData {
  business: Business;
  customer: string;
  invoices: { number: string; issue_date: string; due_date: string; currency: string; total: number; balance: number; status: string; share_token: string }[];
  estimates: { number: string; issue_date: string; due_date: string; currency: string; total: number; balance: number; status: string; share_token: string }[];
  total_due: number;
  currency: string;
}
export const getPortal = (token: string) => jf<PortalData>(`/api/v1/public/portal/${encodeURIComponent(token)}`);
