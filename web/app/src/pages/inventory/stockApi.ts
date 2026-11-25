// Client for /api/v1/stock/* and /api/v1/purchasing/* (Inventory extensions).

async function call<T>(base: string, method: string, path: string, body?: unknown): Promise<T> {
  const resp = await fetch(`${base}${path}`, {
    method,
    credentials: "same-origin",
    headers: { Accept: "application/json", ...(body !== undefined ? { "Content-Type": "application/json" } : {}) },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!resp.ok) {
    const text = (await resp.text().catch(() => "")).trim();
    throw new Error(text || `HTTP ${resp.status}`);
  }
  return (await resp.json()) as T;
}

const S = "/api/v1/stock";
const P = "/api/v1/purchasing";
const stock = <T,>(m: string, p: string, b?: unknown) => call<T>(S, m, p, b);
const buy = <T,>(m: string, p: string, b?: unknown) => call<T>(P, m, p, b);

function qs(o: Record<string, string | number | boolean | undefined>): string {
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(o)) if (v !== undefined && v !== "" && v !== false) p.set(k, String(v));
  const s = p.toString();
  return s ? `?${s}` : "";
}

// ------------------------------------------------------------------ types

export interface Loc {
  id: string;
  parent_id: string;
  name: string;
  code: string;
  kind: "warehouse" | "zone" | "shelf" | "bin";
  address: string;
  is_default: boolean;
  active: boolean;
  path: string;
  depth: number;
  warehouse_id: string;
  units: number;
  subtree_units: number;
  items: number;
}

export interface Avail {
  product_id: string;
  name: string;
  sku: string;
  unit: string;
  category: string;
  tracking: "none" | "lot" | "serial";
  is_kit: boolean;
  on_hand: number;
  reserved: number;
  available: number;
  incoming: number;
  in_transit: number;
  forecast: number;
  min_stock: number;
}

export interface LocAvail {
  location_id: string;
  path: string;
  qty: number;
  reserved: number;
  shelf_code: string;
}

export interface Reservation {
  id: string;
  product_id: string;
  product_name: string;
  sku: string;
  location_id: string;
  qty: number;
  ref_type: string;
  ref: string;
  state: "active" | "fulfilled" | "released";
  note: string;
  created_at: string;
  expires_at: string | null;
}

export interface LotPick {
  lot_id?: string;
  lot_no: string;
  expiry?: string;
  qty: number;
}

export interface TransferLine {
  id: string;
  product_id: string;
  product_name: string;
  sku: string;
  qty: number;
  qty_shipped: number;
  qty_received: number;
  lots: LotPick[];
  serials: string[];
}

export interface Transfer {
  id: string;
  number: string;
  from_loc_id: string;
  from_name: string;
  to_loc_id: string;
  to_name: string;
  state: "draft" | "in_transit" | "received" | "cancelled";
  note: string;
  created_at: string;
  shipped_at: string | null;
  received_at: string | null;
  lines: TransferLine[];
}

export interface ProductSettings {
  product_id: string;
  valuation_method: "avg" | "fifo";
  tracking: "none" | "lot" | "serial";
  shelf_life_days: number;
}

export interface ValuationRow {
  product_id: string;
  name: string;
  sku: string;
  category: string;
  method: string;
  qty: number;
  value: number;
  avg_cost: number;
}

export interface Valuation {
  as_of: string;
  rows: ValuationRow[];
  total_qty: number;
  total_value: number;
  by_category: { category: string; qty: number; value: number }[];
  in_transit_qty: number;
}

export interface CostLayer {
  id: string;
  received_at: string;
  qty_in: number;
  qty_remaining: number;
  unit_cost: number;
  source: string;
  ref: string;
}

export interface LedgerRow {
  id: string;
  product_name: string;
  sku: string;
  ts: string;
  qty: number;
  unit_cost: number;
  value: number;
  reason: string;
  ref: string;
  journal_kind: string;
  journal_state: string;
  journal_error: string;
}

export interface Lot {
  id: string;
  product_id: string;
  product_name: string;
  sku: string;
  lot_no: string;
  expiry: string;
  mfg_date: string;
  note: string;
  recalled: boolean;
  on_hand: number;
  days_left: number | null;
  status: "ok" | "expiring" | "expired" | "recalled";
  locations: { location_id: string; path: string; qty: number }[];
}

export interface TraceRow {
  at: string;
  location: string;
  qty: number;
  reason: string;
  ref: string;
  party: string;
}

export interface Recall {
  lot: Lot;
  received: number;
  shipped: number;
  moves: TraceRow[];
  outbound: TraceRow[];
  origins: TraceRow[];
}

export interface Serial {
  id: string;
  product_id: string;
  product_name: string;
  serial_no: string;
  state: "in_stock" | "out";
  location: string;
  received_at: string;
  out_ref: string;
  out_at: string | null;
}

export interface BomLine {
  component_id: string;
  name: string;
  sku: string;
  unit: string;
  qty: number;
  cost: number;
  on_hand: number;
}

export interface Bom {
  id: string;
  product_id: string;
  product_name: string;
  sku: string;
  kind: "kit" | "assembly";
  qty: number;
  note: string;
  lines: BomLine[];
  unit_cost: number;
  buildable: number;
}

export interface Assembly {
  id: string;
  number: string;
  product_name: string;
  qty: number;
  labor_cost: number;
  unit_cost: number;
  note: string;
  created_at: string;
}

export interface CountLine {
  id: string;
  product_id: string;
  product_name: string;
  sku: string;
  shelf_code: string;
  expected_qty: number;
  counted_qty: number | null;
  unit_cost: number;
  variance: number;
  variance_value: number;
  accepted: boolean;
  note: string;
}

export interface Count {
  id: string;
  number: string;
  kind: "cycle" | "full";
  location_id: string;
  location: string;
  category: string;
  state: "counting" | "review" | "approved" | "cancelled";
  reason: string;
  note: string;
  created_at: string;
  approved_at: string | null;
  lines: CountLine[];
  counted: number;
  total: number;
  net_variance_value: number;
}

export interface AbcRow {
  product_id: string;
  name: string;
  sku: string;
  units: number;
  value: number;
  share_pct: number;
  cum_pct: number;
  class: "A" | "B" | "C";
}

export interface CoverRow {
  product_id: string;
  name: string;
  sku: string;
  on_hand: number;
  available: number;
  incoming: number;
  daily_demand: number;
  days_cover: number;
  weekly_sales: number[];
  forecast_4w: number;
  status: "out" | "low" | "ok" | "overstock" | "no_demand";
  lead_days: number;
}

export interface ShrinkRow {
  product_id: string;
  name: string;
  sku: string;
  units: number;
  value: number;
  reasons: Record<string, number>;
}
export interface ShrinkageReport {
  days: number;
  rows: ShrinkRow[] | null;
  total_units: number;
  total_value: number;
  internal_use_units: number;
  unspecified_removals: number;
}
export interface MarginRow {
  product_id: string;
  name: string;
  sku: string;
  price: number;
  cost: number;
  margin: number;
  percent: number;
  below_cost: boolean;
}
export type LossReason = "damaged" | "expired" | "lost" | "internal";

export interface DeadRow {
  product_id: string;
  name: string;
  sku: string;
  on_hand: number;
  value: number;
  last_out: string | null;
  days_idle: number;
}

// purchasing
export interface PurchaseSettings {
  company_name: string;
  company_address: string;
  company_gstin: string;
  currency: string;
  default_terms: string;
  require_approval: boolean;
  approval_threshold: number;
  over_receipt_pct: number;
  under_receipt_policy: "backorder" | "close";
  qty_tolerance_pct: number;
  price_tolerance_pct: number;
  tax_split: "cgst_sgst" | "igst";
}

export interface PoLine {
  id: string;
  product_id: string;
  product_name: string;
  sku: string;
  unit: string;
  tracking: string;
  description: string;
  qty: number;
  unit_cost: number;
  tax_pct: number;
  qty_received: number;
  qty_returned: number;
  qty_billed: number;
  line_total: number;
}

export interface Po {
  id: string;
  number: string;
  supplier_id: string;
  supplier_name: string;
  supplier_email: string;
  state: "draft" | "approved" | "sent" | "acknowledged" | "partial" | "received" | "closed" | "cancelled";
  order_date: string;
  expected_date: string;
  ship_to_loc_id: string;
  ship_to_name: string;
  currency: string;
  supplier_ref: string;
  notes: string;
  terms: string;
  subtotal: number;
  tax_total: number;
  total: number;
  approved_at: string | null;
  sent_at: string | null;
  sent_to: string;
  ack_at: string | null;
  ack_ref: string;
  ack_note: string;
  promised_date: string;
  created_at: string;
  lines: PoLine[];
  needs_approval: boolean;
}

export interface PoInput {
  supplier_id: string;
  expected_date?: string;
  ship_to_loc_id?: string;
  supplier_ref?: string;
  notes?: string;
  terms?: string;
  lines: { product_id: string; qty: number; unit_cost?: number | null; tax_pct?: number; description?: string }[];
}

export interface GrnLine {
  id: string;
  po_line_id: string;
  product_name: string;
  sku: string;
  qty: number;
  unit_cost: number;
  landed_alloc: number;
  lot_no: string;
  expiry: string;
  serials: string[];
  qty_returned: number;
}

export interface Grn {
  id: string;
  number: string;
  po_id: string;
  po_number: string;
  supplier_name: string;
  location_name: string;
  received_at: string;
  delivery_ref: string;
  landed_cost: number;
  landed_method: string;
  lines: GrnLine[];
}

export interface GrnInput {
  po_id?: string;
  supplier_id?: string;
  location_id?: string;
  delivery_ref?: string;
  note?: string;
  landed_cost?: number;
  landed_method?: "value" | "qty";
  close_short?: boolean;
  lines: { po_line_id?: string; product_id?: string; qty: number; unit_cost?: number; lot_no?: string; expiry?: string; serials?: string[] }[];
}

export interface BillLine {
  id: string;
  po_line_id: string;
  product_id: string;
  product_name: string;
  description: string;
  qty: number;
  unit_cost: number;
  tax_pct: number;
  line_total: number;
}

export interface MatchIssue {
  line_id: string;
  name: string;
  kind: string;
  detail: string;
}

export interface Bill {
  id: string;
  number: string;
  supplier_id: string;
  supplier_name: string;
  po_id: string;
  po_number: string;
  bill_ref: string;
  bill_date: string;
  due_date: string;
  state: "draft" | "posted" | "void";
  match_status: string;
  match_detail: MatchIssue[];
  override_reason: string;
  subtotal: number;
  tax_total: number;
  total: number;
  accounting_state: string;
  accounting_error: string;
  lines: BillLine[];
}

export interface BillInput {
  supplier_id?: string;
  po_id?: string;
  bill_ref?: string;
  bill_date?: string;
  due_date?: string;
  lines: { po_line_id?: string; product_id?: string; description?: string; qty: number; unit_cost: number; tax_pct: number }[];
}

export interface VendorReturn {
  id: string;
  number: string;
  supplier_name: string;
  reason: string;
  credit_note: string;
  returned_at: string;
  total: number;
  lines: { product_name: string; qty: number; unit_cost: number; lot_no: string }[];
}

export interface Request {
  id: string;
  number: string;
  state: "draft" | "submitted" | "approved" | "rejected" | "converted";
  note: string;
  needed_by: string;
  created_at: string;
  po_ids: string[];
  lines: { product_id: string; product_name: string; qty: number; note: string }[];
}

export interface Price {
  id: string;
  supplier_id: string;
  supplier_name: string;
  product_id: string;
  product_name: string;
  min_qty: number;
  unit_cost: number;
  lead_days: number;
  valid_from: string;
  valid_to: string;
  supplier_sku: string;
  note: string;
}

export interface Rule {
  id: string;
  product_id: string;
  product_name: string;
  sku: string;
  min_qty: number;
  max_qty: number;
  multiple: number;
  supplier_id: string;
  active: boolean;
}

export interface Proposal {
  product_id: string;
  product_name: string;
  sku: string;
  position: number;
  on_hand: number;
  incoming: number;
  reserved: number;
  min_qty: number;
  qty: number;
  unit_cost: number;
  supplier_id: string;
  supplier_name: string;
  lead_days: number;
  source: string;
}

export interface ReorderRun {
  proposals: Proposal[];
  unassigned: Proposal[];
  created: Po[];
  skipped: Proposal[];
}

export interface SupplierScore {
  supplier_id: string;
  supplier_name: string;
  orders: number;
  receipts: number;
  on_time_pct: number;
  fill_rate_pct: number;
  avg_lead_days: number;
  return_rate_pct: number;
  spend: number;
  price_variance: number;
  score: number;
}

export interface LookupResult {
  kind: "product" | "lot" | "serial";
  product_id: string;
  name: string;
  sku: string;
  barcode?: string;
  unit?: string;
  stock?: number;
  lot_id?: string;
  serial_id?: string;
  state?: string;
}

// -------------------------------------------------------------------- calls

export const api = {
  // warehouses
  locations: () => stock<{ locations: Loc[] }>("GET", "/locations").then((r) => r.locations ?? []),
  saveLocation: (id: string, b: Partial<Loc>) => (id ? stock<Loc>("PUT", `/locations/${id}`, b) : stock<Loc>("POST", "/locations", b)),
  deleteLocation: (id: string) => stock("DELETE", `/locations/${id}`),
  setShelf: (product: string, location_id: string, shelf_code: string) => stock("PUT", `/products/${product}/shelf`, { location_id, shelf_code }),
  productLocations: (id: string) => stock<{ locations: LocAvail[] }>("GET", `/products/${id}/locations`).then((r) => r.locations ?? []),
  productSettings: (id: string) => stock<ProductSettings>("GET", `/products/${id}/settings`),
  saveProductSettings: (id: string, b: Omit<ProductSettings, "product_id">) => stock<ProductSettings>("PUT", `/products/${id}/settings`, b),
  productLayers: (id: string) => stock<{ layers: CostLayer[] }>("GET", `/products/${id}/layers`).then((r) => r.layers ?? []),
  availability: (q = "", location_id = "") => stock<{ products: Avail[] }>("GET", `/availability${qs({ q, location_id })}`).then((r) => r.products ?? []),
  reservations: (state = "active") => stock<{ reservations: Reservation[] }>("GET", `/reservations${qs({ state })}`).then((r) => r.reservations ?? []),
  reserve: (b: { product_id: string; qty: number; ref: string; location_id?: string; note?: string; backorder?: boolean }) =>
    stock<{ reservations: Reservation[] }>("POST", "/reservations", b),
  release: (ref: string) => stock<{ released: number }>("POST", "/reservations/release", { ref }),
  transfers: (state = "") => stock<{ transfers: Transfer[] }>("GET", `/transfers${qs({ state })}`).then((r) => r.transfers ?? []),
  createTransfer: (b: { from_loc_id: string; to_loc_id: string; note?: string; ship?: boolean; lines: { product_id: string; qty: number }[] }) =>
    stock<Transfer>("POST", "/transfers", b),
  shipTransfer: (id: string) => stock<Transfer>("POST", `/transfers/${id}/ship`),
  receiveTransfer: (id: string, lines?: { line_id: string; qty: number }[]) => stock<Transfer>("POST", `/transfers/${id}/receive`, lines ? { lines } : {}),
  cancelTransfer: (id: string) => stock<Transfer>("POST", `/transfers/${id}/cancel`),
  // valuation
  valuation: (as_of = "", category = "") => stock<Valuation>("GET", `/valuation${qs({ as_of, category })}`),
  ledger: (o: { from?: string; to?: string; reason?: string; kind?: string }) => stock<{ rows: LedgerRow[] }>("GET", `/ledger${qs(o)}`).then((r) => r.rows ?? []),
  accountingStatus: () => stock<{ connected: boolean; pending: number; failed: number }>("GET", "/accounting/status"),
  accountingSync: () => stock<{ posted: number; failed: number; bills_retried: number; connected: boolean }>("POST", "/accounting/sync"),
  // lots
  lots: (o: { product_id?: string; only_stock?: boolean; expiring_days?: number; q?: string } = {}) => stock<{ lots: Lot[] }>("GET", `/lots${qs(o)}`).then((r) => r.lots ?? []),
  updateLot: (id: string, b: { expiry: string; mfg_date: string; note: string; recalled: boolean }) => stock<Lot>("PUT", `/lots/${id}`, b),
  recall: (id: string) => stock<Recall>("GET", `/lots/${id}/recall`),
  serials: (o: { product_id?: string; state?: string; q?: string } = {}) => stock<{ serials: Serial[] }>("GET", `/serials${qs(o)}`).then((r) => r.serials ?? []),
  serialHistory: (id: string) => stock<{ moves: TraceRow[] }>("GET", `/serials/${id}/history`).then((r) => r.moves ?? []),
  lookup: (code: string) => stock<LookupResult>("GET", `/lookup${qs({ code })}`),
  // bom
  boms: () => stock<{ boms: Bom[] }>("GET", "/boms").then((r) => r.boms ?? []),
  saveBom: (b: { product_id: string; kind: string; qty: number; note?: string; lines: { component_id: string; qty: number }[] }) => stock<Bom>("POST", "/boms", b),
  deleteBom: (id: string) => stock("DELETE", `/boms/${id}`),
  assemblies: () => stock<{ assemblies: Assembly[] }>("GET", "/assemblies").then((r) => r.assemblies ?? []),
  assemble: (b: { bom_id: string; qty: number; location_id?: string; labor_cost?: number; lot_no?: string; expiry?: string; serials?: string[]; note?: string }) =>
    stock<Assembly>("POST", "/assemblies", b),
  // counts
  counts: () => stock<{ counts: Count[] }>("GET", "/counts").then((r) => r.counts ?? []),
  count: (id: string) => stock<Count>("GET", `/counts/${id}`),
  startCount: (b: { kind: string; location_id: string; category?: string; abc?: string; note?: string }) => stock<Count>("POST", "/counts", b),
  recordCount: (id: string, entries: { line_id?: string; product_id?: string; counted_qty?: number | null; accepted?: boolean; note?: string }[]) =>
    stock<Count>("PUT", `/counts/${id}/lines`, { entries }),
  submitCount: (id: string) => stock<Count>("POST", `/counts/${id}/submit`),
  approveCount: (id: string, reason: string) => stock<Count>("POST", `/counts/${id}/approve`, { reason }),
  cancelCount: (id: string) => stock<Count>("POST", `/counts/${id}/cancel`),
  abc: (days = 365) => stock<{ rows: AbcRow[] }>("GET", `/reports/abc${qs({ days })}`).then((r) => r.rows ?? []),
  cover: (weeks = 8) => stock<{ rows: CoverRow[] }>("GET", `/reports/cover${qs({ weeks })}`).then((r) => r.rows ?? []),
  shrinkage: (days = 30) => stock<ShrinkageReport>("GET", `/reports/shrinkage${qs({ days })}`),
  margins: () => stock<{ rows: MarginRow[] }>("GET", "/reports/margin").then((r) => r.rows ?? []),
  writeOff: (b: { product_id: string; location_id: string; qty: number; loss_reason: LossReason; note?: string }) => stock<unknown>("POST", "/writeoffs", b),
  dead: (days = 90) => stock<{ rows: DeadRow[] }>("GET", `/reports/dead${qs({ days })}`).then((r) => r.rows ?? []),

  // purchasing
  settings: () => buy<PurchaseSettings>("GET", "/settings"),
  saveSettings: (b: PurchaseSettings) => buy<PurchaseSettings>("PUT", "/settings", b),
  prices: (o: { supplier_id?: string; product_id?: string } = {}) => buy<{ prices: Price[] }>("GET", `/prices${qs(o)}`).then((r) => r.prices ?? []),
  savePrice: (id: string, b: Partial<Price>) => (id ? buy<Price>("PUT", `/prices/${id}`, b) : buy<Price>("POST", "/prices", b)),
  deletePrice: (id: string) => buy("DELETE", `/prices/${id}`),
  requests: () => buy<{ requests: Request[] }>("GET", "/requests").then((r) => r.requests ?? []),
  createRequest: (b: { note?: string; needed_by?: string; submit?: boolean; lines: { product_id: string; qty: number; note?: string }[] }) => buy<Request>("POST", "/requests", b),
  decideRequest: (id: string, action: "submit" | "approve" | "reject") => buy<Request>("POST", `/requests/${id}/${action}`),
  convertRequest: (id: string) => buy<{ orders: Po[] }>("POST", `/requests/${id}/convert`).then((r) => r.orders ?? []),
  orders: (o: { state?: string; supplier_id?: string; q?: string } = {}) => buy<{ orders: Po[] }>("GET", `/orders${qs(o)}`).then((r) => r.orders ?? []),
  order: (id: string) => buy<Po>("GET", `/orders/${id}`),
  createOrder: (b: PoInput) => buy<Po>("POST", "/orders", b),
  updateOrder: (id: string, b: PoInput) => buy<Po>("PUT", `/orders/${id}`, b),
  orderAction: (id: string, action: "approve" | "cancel" | "close") => buy<Po>("POST", `/orders/${id}/${action}`),
  sendOrder: (id: string, b: { to?: string; subject?: string; message?: string; mark_only?: boolean }) => buy<Po>("POST", `/orders/${id}/send`, b),
  acknowledge: (id: string, b: { ref: string; note: string; promised_date: string }) => buy<Po>("POST", `/orders/${id}/acknowledge`, b),
  billDraft: (id: string) => buy<BillInput>("GET", `/orders/${id}/bill-draft`),
  pdfUrl: (id: string) => `${P}/orders/${id}/pdf`,
  receipts: (po_id = "") => buy<{ receipts: Grn[] }>("GET", `/receipts${qs({ po_id })}`).then((r) => r.receipts ?? []),
  receive: (b: GrnInput) => buy<Grn>("POST", "/receipts", b),
  bills: (state = "") => buy<{ bills: Bill[] }>("GET", `/bills${qs({ state })}`).then((r) => r.bills ?? []),
  createBill: (b: BillInput) => buy<Bill>("POST", "/bills", b),
  updateBill: (id: string, b: BillInput) => buy<Bill>("PUT", `/bills/${id}`, b),
  deleteBill: (id: string) => buy("DELETE", `/bills/${id}`),
  postBill: (id: string, override_reason = "") => buy<Bill>("POST", `/bills/${id}/post`, { override_reason }),
  returns: () => buy<{ returns: VendorReturn[] }>("GET", "/returns").then((r) => r.returns ?? []),
  createReturn: (b: { grn_id: string; reason: string; credit_note?: string; lines: { grn_line_id: string; qty: number; lot_no?: string; serials?: string[] }[] }) =>
    buy<VendorReturn>("POST", "/returns", b),
  rules: () => buy<{ rules: Rule[] }>("GET", "/rules").then((r) => r.rules ?? []),
  saveRule: (b: Partial<Rule>) => buy<Rule>("POST", "/rules", b),
  deleteRule: (id: string) => buy("DELETE", `/rules/${id}`),
  runReorder: (create: boolean) => buy<ReorderRun>("POST", "/reorder/run", { create }),
  performance: (from = "", to = "") => buy<{ suppliers: SupplierScore[] }>("GET", `/performance${qs({ from, to })}`).then((r) => r.suppliers ?? []),
};
