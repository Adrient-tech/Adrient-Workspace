// API for the wave-3 accounting features: bills, credits, deposits, currencies, recurring,
// assets, taxes, budgets, controls and migration imports.
import { BASE, jf, qs, send } from "./api";

export interface BillLine {
  account_id: string;
  description: string;
  hsn: string;
  amount: number;
  tax_rate: number;
  class: string;
}
export interface BillPayment {
  id: string;
  date: string;
  amount: number;
  account_id: string;
  credit_id: string;
  rate: number;
  reference: string;
}
export interface Bill {
  id: string;
  kind: "bill" | "vendor_credit";
  number: string;
  vendor_id: string;
  vendor_name: string;
  bill_date: string;
  due_date: string;
  currency: string;
  rate: number;
  subtotal: number;
  tax_total: number;
  total: number;
  paid: number;
  balance: number;
  status: string;
  approved_by: string;
  approval_note: string;
  notes: string;
  lines: BillLine[];
  payments: BillPayment[];
}
export interface BillInput {
  kind: string;
  number: string;
  vendor_name: string;
  bill_date: string;
  due_date: string;
  currency: string;
  lines: Partial<BillLine>[];
  notes: string;
  draft?: boolean;
}
export interface CreditMemo {
  id: string;
  number: string;
  customer_name: string;
  date: string;
  currency: string;
  amount: number;
  gst_rate: number;
  tax_total: number;
  total: number;
  refunded: number;
  balance: number;
  reason: string;
  status: string;
}
export interface Receipt {
  line_id: string;
  date: string;
  memo: string;
  contact_name: string;
  amount: number;
}
export interface Deposit {
  id: string;
  date: string;
  account_id: string;
  total: number;
  reference: string;
  items: number;
}
export interface FxRate {
  id: string;
  currency: string;
  date: string;
  rate: number;
  source: string;
}
export interface RevalEntry {
  currency: string;
  account: string;
  foreign_balance: number;
  carrying: number;
  revalued: number;
  adjustment: number;
  rate: number;
}
export interface Recurring {
  id: string;
  kind: string;
  name: string;
  frequency: string;
  every: number;
  start_date: string;
  end_date: string;
  max_runs: number;
  active: boolean;
  auto_post: boolean;
  template: unknown;
  last_run: string;
  runs: number;
  next_run: string;
  last_error: string;
}
export interface Asset {
  id: string;
  name: string;
  tag: string;
  category: string;
  purchase_date: string;
  dep_start: string;
  cost: number;
  salvage: number;
  life_months: number;
  method: string;
  wdv_rate: number;
  class: string;
  status: string;
  accumulated: number;
  book_value: number;
  paid_from_id?: string;
}
export interface DepRow {
  period: string;
  opening: number;
  depreciation: number;
  accumulated: number;
  closing: number;
  posted: boolean;
}
export interface TaxRate {
  id: string;
  kind: "gst" | "tds";
  code: string;
  name: string;
  rate: number;
  section: string;
  active: boolean;
}
export interface GstParty {
  contact_id: string;
  name: string;
  gstin: string;
  state_code: string;
  pan: string;
}
export interface TdsEntry {
  id: string;
  direction: string;
  section: string;
  party_name: string;
  pan: string;
  date: string;
  base_amount: number;
  rate: number;
  tds_amount: number;
  status: string;
  challan: string;
}
export interface BudgetCell {
  account_id: string;
  month: string;
  amount: number;
}
export interface CheckItem {
  id: string;
  label: string;
  status: "ok" | "warn" | "fail";
  count: number;
  detail: string;
}
export interface Checklist {
  date: string;
  items: CheckItem[];
  can_close: boolean;
  closed_through: string;
}
export interface TableData {
  title: string;
  subtitle: string;
  headers: string[];
  rows: (string | number | null)[][];
  links: string[];
}
export interface ImportRow {
  line: number;
  status: "ok" | "existing" | "error";
  message: string;
  summary: string;
}
export interface ImportReport {
  type: string;
  source: string;
  dry_run: boolean;
  total: number;
  ok: number;
  existing: number;
  errors: number;
  applied: number;
  rows: ImportRow[];
  mapping: Record<string, string>;
  notes: string[];
}

const list = async <T>(url: string, key: string): Promise<T[]> => ((await jf<Record<string, T[]>>(url))[key] ?? []) as T[];

// bills
export const listBills = (kind = "", status = "") => list<Bill>(`${BASE}/bills${qs({ kind, status })}`, "bills");
export const saveBill = (b: Partial<BillInput>, id = "") => jf<Bill>(`${BASE}/bills${id ? "/" + id : ""}`, send(id ? "PUT" : "POST", b));
export const billAction = (id: string, action: string, body: unknown = {}) => jf<Bill>(`${BASE}/bills/${id}/${action}`, send("POST", body));
export const deleteBillPayment = (id: string, pid: string) => jf<Bill>(`${BASE}/bills/${id}/payments/${pid}`, send("DELETE"));
// credit memos, deposits
export const listCreditMemos = () => list<CreditMemo>(`${BASE}/credit-memos`, "credit_memos");
export const saveCreditMemo = (b: Record<string, unknown>) => jf<CreditMemo>(`${BASE}/credit-memos`, send("POST", b));
export const creditMemoAction = (id: string, action: string, body: unknown = {}) => jf<CreditMemo>(`${BASE}/credit-memos/${id}/${action}`, send("POST", body));
export const listReceipts = () => list<Receipt>(`${BASE}/undeposited`, "receipts");
export const listDeposits = () => list<Deposit>(`${BASE}/deposits`, "deposits");
export const createDeposit = (b: Record<string, unknown>) => jf<Deposit>(`${BASE}/deposits`, send("POST", b));
export const deleteDeposit = (id: string) => jf<unknown>(`${BASE}/deposits/${id}`, send("DELETE"));
export const writeOffReceivable = (b: Record<string, unknown>) => jf<unknown>(`${BASE}/write-off`, send("POST", b));
// currencies
export const listRates = () => list<FxRate>(`${BASE}/rates`, "rates");
export const saveRate = (r: Partial<FxRate>) => jf<FxRate>(`${BASE}/rates`, send("POST", r));
export const deleteRate = (id: string) => jf<unknown>(`${BASE}/rates/${id}`, send("DELETE"));
export const fetchRates = () => jf<{ stored: number }>(`${BASE}/rates/fetch`, send("POST"));
export const revalue = (date: string) => jf<{ date: string; entries: RevalEntry[] }>(`${BASE}/revalue`, send("POST", { date }));
// recurring
export const listRecurring = () => list<Recurring>(`${BASE}/recurring`, "recurring");
export const saveRecurring = (r: Partial<Recurring>) => jf<Recurring>(`${BASE}/recurring${r.id ? "/" + r.id : ""}`, send(r.id ? "PUT" : "POST", r));
export const deleteRecurring = (id: string) => jf<unknown>(`${BASE}/recurring/${id}`, send("DELETE"));
export const runRecurring = () => jf<{ created: number; failed: number; errors: string[] }>(`${BASE}/recurring/run`, send("POST"));
// assets
export const listAssets = () => list<Asset>(`${BASE}/assets`, "assets");
export const createAsset = (a: Partial<Asset>) => jf<Asset>(`${BASE}/assets`, send("POST", a));
export const assetSchedule = (id: string) => list<DepRow>(`${BASE}/assets/${id}/schedule`, "schedule");
export const runDepreciation = (period: string) => jf<{ posted: number; locked: number; errors: string[] }>(`${BASE}/assets/depreciate`, send("POST", { period }));
export const disposeAsset = (id: string, b: Record<string, unknown>) => jf<Asset>(`${BASE}/assets/${id}/dispose`, send("POST", b));
// taxes
export const listTaxRates = () => list<TaxRate>(`${BASE}/tax-rates`, "rates");
export const saveTaxRate = (t: Partial<TaxRate>) => jf<TaxRate>(`${BASE}/tax-rates${t.id ? "/" + t.id : ""}`, send(t.id ? "PUT" : "POST", t));
export const deleteTaxRate = (id: string) => jf<unknown>(`${BASE}/tax-rates/${id}`, send("DELETE"));
export const listGstParties = () => list<GstParty>(`${BASE}/gst-parties`, "parties");
export const saveGstParty = (p: Partial<GstParty>) => jf<GstParty>(`${BASE}/gst-parties`, send("POST", p));
export const saveHsn = (match: string, hsn: string) => jf<unknown>(`${BASE}/hsn-map`, send("POST", { match, hsn }));
export const recordTds = (e: Record<string, unknown>) => jf<TdsEntry>(`${BASE}/tds`, send("POST", e));
export const settleTds = (id: string, b: Record<string, unknown>) => jf<TdsEntry>(`${BASE}/tds/${id}/settle`, send("POST", b));
export const importGstr2b = (period: string, csv: string) => jf<{ imported: number }>(`${BASE}/gstr2b/import`, send("POST", { period, csv }));
// budgets, classes
export const listBudgets = (from: string, to: string) => list<BudgetCell>(`${BASE}/budgets${qs({ from, to })}`, "budgets");
export const saveBudgets = (cells: BudgetCell[]) => jf<{ saved: number }>(`${BASE}/budgets`, send("PUT", { cells }));
export const listClasses = () => list<string>(`${BASE}/classes`, "classes");
// reports (generic table view and exports)
export const reportTable = (name: string, q: Record<string, string | undefined>) => jf<TableData>(`${BASE}/reports/${name}${qs({ ...q, format: "table" })}`);
export const reportExportURL = (name: string, format: string, q: Record<string, string | undefined>) => `${BASE}/reports/${name}${qs({ ...q, format })}`;
// controls
export const closeChecklist = (date: string) => jf<Checklist>(`${BASE}/close-checklist${qs({ date })}`);
export const closePeriod = (date: string, acknowledge: boolean) => jf<Checklist>(`${BASE}/close-period`, send("POST", { date, acknowledge }));
export const addJournalFile = (id: string, f: { id: string; name: string }) => jf<{ attachments: { id: string; name: string }[] }>(`${BASE}/journals/${id}/files`, send("POST", f));
export const removeJournalFile = (id: string, fid: string) => jf<{ attachments: { id: string; name: string }[] }>(`${BASE}/journals/${id}/files/${encodeURIComponent(fid)}`, send("DELETE"));
// migration
export const importColumns = () => jf<Record<string, { field: string; columns: string[] }[]>>(`${BASE}/import/columns`);
export const runImport = (mode: "preview" | "run", req: Record<string, unknown>) => jf<ImportReport>(`${BASE}/import/${mode}`, send("POST", req));
export const importOfx = (account_id: string, data: string) => jf<{ imported: number; duplicates: number; added: number; categorized: number }>(`${BASE}/bank/import-ofx`, send("POST", { account_id, data }));
