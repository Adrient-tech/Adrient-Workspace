import type {
  Account,
  Aging,
  AuditRow,
  BalanceSheet,
  BankLine,
  CashFlow,
  ContactLite,
  Expense,
  ExpenseReport,
  GSTReport,
  ImportResult,
  Journal,
  JournalInput,
  PnL,
  ReconDraft,
  Reconciliation,
  RegisterRow,
  Rule,
  Settings,
  Summary,
  SyncResult,
  TrialBalance,
} from "./types";

export const BASE = "/api/v1/accounting";

export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
  ) {
    super(message);
  }
}

export async function jf<T>(url: string, init?: RequestInit): Promise<T> {
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

export const send = (method: string, body?: unknown): RequestInit => ({
  method,
  body: body === undefined ? undefined : JSON.stringify(body),
});

export const qs = (o: Record<string, string | number | undefined>) => {
  const p = new URLSearchParams();
  Object.entries(o).forEach(([k, v]) => v !== undefined && v !== "" && p.set(k, String(v)));
  const s = p.toString();
  return s ? `?${s}` : "";
};

export const getSettings = () => jf<Settings>(`${BASE}/settings`);
export const saveSettings = (s: Settings) => jf<Settings>(`${BASE}/settings`, send("PUT", s));
export const getCapabilities = () => jf<{ pos: boolean; invoicing: boolean }>(`${BASE}/capabilities`);
export const getSummary = () => jf<Summary>(`${BASE}/summary`);
export const syncApps = (full = false) => jf<Record<string, SyncResult>>(`${BASE}/sync${full ? "?full=1" : ""}`, send("POST"));

export const listAccounts = async () => (await jf<{ accounts: Account[] }>(`${BASE}/accounts`)).accounts ?? [];
export const createAccount = (a: Partial<Account>) => jf<Account>(`${BASE}/accounts`, send("POST", a));
export const updateAccount = (id: string, a: Partial<Account>) => jf<Account>(`${BASE}/accounts/${id}`, send("PUT", a));
export const deleteAccount = (id: string) => jf<unknown>(`${BASE}/accounts/${id}`, send("DELETE"));
export const getRegister = (id: string, from = "", to = "") =>
  jf<{ account: Account; opening: number; rows: RegisterRow[] }>(`${BASE}/accounts/${id}/register${qs({ from, to })}`);

export const listJournals = async (q: { from?: string; to?: string; q?: string; source?: string; account_id?: string; limit?: number } = {}) =>
  (await jf<{ journals: Journal[] }>(`${BASE}/journals${qs(q)}`)).journals ?? [];
export const getJournal = (id: string) => jf<Journal>(`${BASE}/journals/${id}`);
export const createJournal = (j: JournalInput) => jf<Journal>(`${BASE}/journals`, send("POST", j));
export const updateJournal = (id: string, j: JournalInput) => jf<Journal>(`${BASE}/journals/${id}`, send("PUT", j));
export const deleteJournal = (id: string) => jf<unknown>(`${BASE}/journals/${id}`, send("DELETE"));

export type ReportName = "trial-balance" | "pnl" | "balance-sheet" | "cash-flow" | "aged-receivables" | "aged-payables" | "gst" | "pnl-by-class" | "budget-vs-actual" | "general-ledger" | "journal" | "transaction-list" | "account-list" | "customer-balances" | "vendor-balances" | "sales-by-customer" | "expenses-by-vendor" | "pnl-columns";
export function getReport<T = TrialBalance | PnL | BalanceSheet | CashFlow | Aging | GSTReport>(name: ReportName, q: { from?: string; to?: string; as_of?: string }) {
  return jf<T>(`${BASE}/reports/${name}${qs(q)}`);
}

export const listBankLines = async (account_id: string, status = "") => (await listBankLinesPage(account_id, status)).items;
/** One server page of bank lines (100 per page); `next` is the next page number or "". */
export const listBankLinesPage = async (account_id: string, status = "", page = 1) => {
  const r = await jf<{ lines: BankLine[]; next_cursor?: string }>(`${BASE}/bank/lines${qs({ account_id, status, page: String(page) })}`);
  return { items: r.lines ?? [], next: r.next_cursor ?? "" };
};
export const importBankCSV = (account_id: string, csv: string) => jf<ImportResult>(`${BASE}/bank/import`, send("POST", { account_id, csv }));
export const addBankLine = (id: string, category_id: string, memo = "") => jf<BankLine>(`${BASE}/bank/lines/${id}/add`, send("POST", { category_id, memo }));
export const excludeBankLine = (id: string) => jf<unknown>(`${BASE}/bank/lines/${id}/exclude`, send("POST"));
export type BankMatch = { line_id: string; journal_id: string; score: number; reasons: string[]; days: number; date: string; memo: string; contact: string; amount: number };
export const listBankMatches = async (account_id: string) => (await jf<{ matches: BankMatch[] }>(`${BASE}/bank/matches${qs({ account_id })}`)).matches ?? [];
export const matchBankLine = (id: string, journal_id: string) => jf<BankLine>(`${BASE}/bank/lines/${id}/match`, send("POST", { journal_id }));
export const undoBankLine = (id: string) => jf<unknown>(`${BASE}/bank/lines/${id}/undo`, send("POST"));
export const setBankLineCategory = (id: string, category_id: string) => jf<unknown>(`${BASE}/bank/lines/${id}/category`, send("PUT", { category_id }));
export const applyRules = () => jf<{ categorized: number; added: number }>(`${BASE}/bank/apply-rules`, send("POST"));
export const listRules = async () => (await jf<{ rules: Rule[] }>(`${BASE}/bank/rules`)).rules ?? [];
export const saveRule = (r: Partial<Rule>) => (r.id ? jf<Rule>(`${BASE}/bank/rules/${r.id}`, send("PUT", r)) : jf<Rule>(`${BASE}/bank/rules`, send("POST", r)));
export const deleteRule = (id: string) => jf<unknown>(`${BASE}/bank/rules/${id}`, send("DELETE"));

export const getReconDraft = (account_id: string, statement_date: string) => jf<ReconDraft>(`${BASE}/reconcile${qs({ account_id, statement_date })}`);
export const finishRecon = (account_id: string, statement_date: string, ending_balance: number, line_ids: string[]) =>
  jf<Reconciliation>(`${BASE}/reconcile`, send("POST", { account_id, statement_date, ending_balance, line_ids }));
export const reconHistory = async (account_id: string) => (await jf<{ history: Reconciliation[] }>(`${BASE}/reconcile/history${qs({ account_id })}`)).history ?? [];
export const undoRecon = (id: string) => jf<unknown>(`${BASE}/reconcile/${id}/undo`, send("POST"));

export const listExpenses = async (q: { report_id?: string; q?: string; from?: string; to?: string } = {}) =>
  (await jf<{ expenses: Expense[] }>(`${BASE}/expenses${qs(q)}`)).expenses ?? [];
export const saveExpense = (e: Partial<Expense>) => (e.id ? jf<Expense>(`${BASE}/expenses/${e.id}`, send("PUT", e)) : jf<Expense>(`${BASE}/expenses`, send("POST", e)));
export const deleteExpense = (id: string) => jf<unknown>(`${BASE}/expenses/${id}`, send("DELETE"));
export const payBill = (id: string, account_id: string, date: string) => jf<Expense>(`${BASE}/expenses/${id}/pay`, send("POST", { account_id, date }));

export const listReports = async () => (await jf<{ reports: ExpenseReport[] }>(`${BASE}/expense-reports`)).reports ?? [];
export const saveReport = (r: Partial<ExpenseReport>) =>
  r.id ? jf<ExpenseReport>(`${BASE}/expense-reports/${r.id}`, send("PUT", r)) : jf<ExpenseReport>(`${BASE}/expense-reports`, send("POST", r));
export const deleteReport = (id: string) => jf<unknown>(`${BASE}/expense-reports/${id}`, send("DELETE"));
export const reportAction = (id: string, action: "submit" | "approve" | "reject" | "reimburse", body: { note?: string; account_id?: string; date?: string } = {}) =>
  jf<ExpenseReport>(`${BASE}/expense-reports/${id}/${action}`, send("POST", body));

export const listAudit = async () => (await jf<{ audit: AuditRow[] }>(`${BASE}/audit?limit=200`)).audit ?? [];
export const search = (q: string) => jf<{ journals: Journal[]; accounts: Account[]; expenses: Expense[] }>(`${BASE}/search${qs({ q })}`);

// Other modules (Contacts, Drive). Failures degrade to empty lists.
export async function listContacts(): Promise<ContactLite[]> {
  try {
    return (await jf<{ contacts: ContactLite[] }>("/api/v1/contacts")).contacts ?? [];
  } catch {
    return [];
  }
}

/** Upload a receipt photo or PDF into Drive; returns the file id and name. */
export async function uploadReceipt(file: File): Promise<{ id: string; name: string }> {
  const fd = new FormData();
  fd.append("file", file);
  const r = await fetch("/api/v1/drive/files/upload", { method: "POST", credentials: "same-origin", body: fd });
  if (!r.ok) throw new ApiError((await r.text().catch(() => "")) || `upload failed (${r.status})`, r.status);
  const f = (await r.json()) as { id: string; name?: string };
  return { id: f.id, name: f.name || file.name };
}
export const receiptURL = (id: string) => `/api/v1/drive/files/${id}/content`;
