export interface Settings {
  business_name: string;
  gstin: string;
  currency: string;
  fy_start_month: number;
  intrastate: boolean;
  closing_date: string;
  pos_synced_at: string;
  use_undeposited: boolean;
  approval_limit: number;
  fx_auto_fetch: boolean;
}

export type AccountType = "asset" | "liability" | "equity" | "income" | "expense";

export interface Account {
  id: string;
  code: string;
  name: string;
  type: AccountType;
  subtype: string;
  system_key: string;
  parent_id: string;
  active: boolean;
  balance: number;
}

export interface JLine {
  id?: string;
  account_id: string;
  account_name?: string;
  debit: number;
  credit: number;
  memo: string;
}

export interface Journal {
  id: string;
  number: number;
  date: string;
  due_date: string;
  memo: string;
  ref: string;
  kind: string;
  source: string;
  source_id: string;
  contact_id: string;
  contact_name: string;
  voided: boolean;
  total: number;
  lines: JLine[];
}

export interface JournalInput {
  date: string;
  memo: string;
  ref: string;
  kind?: string;
  contact_id: string;
  contact_name: string;
  lines: JLine[];
}

export interface SyncResult {
  created: number;
  updated: number;
  skipped: number;
  voided: number;
  locked: number;
  foreign: number;
  warnings: string[];
}

export interface Summary {
  cash: number;
  receivables: number;
  payables: number;
  gst_payable: number;
  month_income: number;
  month_expense: number;
  year_profit: number;
  pending_bank_lines: number;
  journals: number;
  balanced: boolean;
}

export interface Receipt {
  id: string;
  name: string;
}

export interface Expense {
  id: string;
  date: string;
  vendor_id: string;
  vendor_name: string;
  category_id: string;
  category_name: string;
  amount: number;
  gst_rate: number;
  gst_amount: number;
  total: number;
  paid_from_id: string;
  out_of_pocket: boolean;
  paid: boolean;
  notes: string;
  receipts: Receipt[];
  report_id: string;
  report_title: string;
  posted: boolean;
}

export interface ExpenseReport {
  id: string;
  title: string;
  status: "draft" | "submitted" | "approved" | "rejected" | "reimbursed";
  submitter_name: string;
  notes: string;
  decision_note: string;
  reimbursed_at: string;
  created_at: string;
  count: number;
  total: number;
}

export interface BankLine {
  id: string;
  account_id: string;
  date: string;
  description: string;
  amount: number;
  status: "pending" | "added" | "excluded";
  category_id: string;
  journal_id: string;
}

export interface Rule {
  id: string;
  name: string;
  account_id: string;
  direction: "in" | "out" | "any";
  contains: string;
  category_id: string;
  priority: number;
  auto_add: boolean;
}

export interface ImportResult {
  imported: number;
  duplicates: number;
  categorized: number;
  added: number;
}

export interface ReconLine {
  line_id: string;
  journal_id: string;
  date: string;
  memo: string;
  amount: number;
}
export interface ReconDraft {
  account_id: string;
  statement_date: string;
  opening_balance: number;
  lines: ReconLine[];
}
export interface Reconciliation {
  id: string;
  account_id: string;
  statement_date: string;
  ending_balance: number;
  cleared: number;
  created_at: string;
}

export interface RRow {
  account_id: string;
  code: string;
  name: string;
  type?: string;
  debit: number;
  credit: number;
  amount: number;
}
export interface TrialBalance {
  as_of: string;
  rows: RRow[];
  total_debit: number;
  total_credit: number;
  balanced: boolean;
}
export interface PnL {
  from: string;
  to: string;
  income: RRow[];
  expenses: RRow[];
  total_income: number;
  total_expenses: number;
  net_profit: number;
}
export interface BalanceSheet {
  as_of: string;
  assets: RRow[];
  liabilities: RRow[];
  equity: RRow[];
  current_year_earnings: number;
  retained_earnings: number;
  total_assets: number;
  total_liabilities: number;
  total_equity: number;
  balanced: boolean;
}
export interface CashFlow {
  from: string;
  to: string;
  opening: number;
  closing: number;
  net_change: number;
  ties: boolean;
  groups: { name: string; rows: RRow[]; total: number }[];
}
export interface AgingRow {
  contact_id: string;
  contact_name: string;
  current: number;
  d1_30: number;
  d31_60: number;
  d61_90: number;
  d90_plus: number;
  total: number;
}
export interface Aging {
  kind: string;
  as_of: string;
  rows: AgingRow[];
  total: AgingRow;
}
export interface GSTReport {
  from: string;
  to: string;
  output: RRow[];
  input: RRow[];
  output_total: number;
  input_total: number;
  net_payable: number;
  taxable_sales: number;
}
export interface RegisterRow {
  date: string;
  journal_id: string;
  number: number;
  memo: string;
  contact: string;
  debit: number;
  credit: number;
  balance: number;
}
export interface AuditRow {
  id: number;
  user_id: string;
  action: string;
  entity: string;
  entity_id: string;
  detail: string;
  at: string;
  before?: unknown;
  after?: unknown;
}
export interface ContactLite {
  id: string;
  display_name: string;
}
