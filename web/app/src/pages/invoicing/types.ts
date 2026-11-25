export interface Line {
  description: string;
  qty: number;
  unit: string;
  price: number;
  discount_pct: number;
  tax_rate: number;
  tax_name: string;
  /** Price already contains the tax. */
  tax_inclusive?: boolean;
  product_id?: string;
  hsn_sac?: string;
  ref?: string;
  so_line?: string;
}

export interface Schedule {
  label: string;
  due_date: string;
  amount: number;
}

/** One payment-schedule row with what has been applied to it (derived by the server, never stored). */
export interface Instalment {
  label: string;
  due_date: string;
  amount: number;
  paid: number;
  open: number;
}

export interface Attachment {
  name: string;
  url: string;
}

export interface Payment {
  id: string;
  document_id: string;
  kind: "payment" | "refund" | "writeoff" | "credit" | "applied";
  amount: number;
  method: string;
  reference: string;
  note: string;
  paid_at: string;
  doc_number?: string;
  client_name?: string;
  currency?: string;
}

export type DisplayStatus =
  | "draft"
  | "sent"
  | "partial"
  | "paid"
  | "overdue"
  | "written_off"
  | "accepted"
  | "declined"
  | "converted"
  | "credited"
  | "expired"
  | "superseded"
  | "open"
  | "partly_used"
  | "refunded"
  | "applied";

export interface DocInput {
  kind: "invoice" | "estimate" | "credit_note";
  contact_id: string;
  client_name: string;
  client_email: string;
  client_address: string;
  currency: string;
  issue_date: string;
  due_date: string;
  lines: Line[];
  discount_kind: "percent" | "amount";
  discount_value: number;
  notes: string;
  terms: string;
  tax_note: string;
  schedule: Schedule[];
  attachments: Attachment[];
  number?: string;
  valid_until?: string;
  place_of_supply?: string;
  client_gstin?: string;
  price_list_id?: string;
  source_ref?: string;
}

export interface InvDocument extends DocInput {
  id: string;
  number: string;
  status: string;
  display_status: DisplayStatus;
  subtotal: number;
  discount_total: number;
  tax_total: number;
  total: number;
  share_token: string;
  recurring_id: string;
  converted_from: string;
  stock_deducted: boolean;
  sent_at: string;
  viewed_at: string;
  reminders_sent: number;
  deleted: boolean;
  created_at: string;
  paid: number;
  written_off: number;
  balance: number;
  instalments?: Instalment[];
  effective_due?: string;
  overdue_amount?: number;
  payments: Payment[];
  version?: number;
  version_group?: string;
  accepted_at?: string;
  accepted_name?: string;
  accepted_hash?: string;
  declined_reason?: string;
  sales_order_id?: string;
  credit_for?: string;
  supplier_state?: string;
  cgst_total?: number;
  sgst_total?: number;
  igst_total?: number;
  payment_link_url?: string;
  credited?: number;
  credit_remaining?: number;
  latest_id?: string;
  latest_token?: string;
  gst?: { intra: boolean; ut: boolean; supplier: string; place_of_supply: string; lines: { taxable: number; rate: number; cgst: number; sgst: number; igst: number; tax: number }[] };
}

export interface ReminderRule {
  days: number;
}

export interface SalesSettings {
  state_code: string;
  upi_id: string;
  upi_name: string;
  bank_details: string;
  pdf_accent: string;
  pdf_footer: string;
  show_upi_qr: boolean;
  show_bank: boolean;
  estimate_valid_days: number;
  order_prefix: string;
  delivery_prefix: string;
  credit_prefix: string;
  reminders_auto: boolean;
  reminder_rules: ReminderRule[];
  reminder_subject: string;
  reminder_body: string;
  whatsapp_body: string;
  public_url: string;
}

export interface Settings {
  business_name: string;
  address: string;
  tax_id: string;
  email: string;
  phone: string;
  logo_url: string;
  invoice_prefix: string;
  estimate_prefix: string;
  next_invoice: number;
  next_estimate: number;
  default_currency: string;
  due_days: number;
  default_notes: string;
  default_terms: string;
  tax_note: string;
  payment_info: string;
  units: string[];
  rates: Record<string, number>;
  deduct_stock: boolean;
  reminder_days: number;
  sales: SalesSettings;
}

export interface Item {
  id: string;
  name: string;
  description: string;
  unit: string;
  price: number;
  tax_rate: number;
}

export interface Recurring {
  id: string;
  name: string;
  frequency: "weekly" | "monthly" | "quarterly" | "yearly";
  next_run: string;
  end_date: string;
  active: boolean;
  auto_send: boolean;
  template: DocInput;
  generated: number;
}

export interface ClientRow {
  contact_id: string;
  client_name: string;
  client_email: string;
  invoices: number;
  billed: number;
  paid: number;
  outstanding: number;
}

export interface ReportRow {
  label: string;
  amount: number;
  count: number;
}

export interface Report {
  currency: string;
  invoiced: number;
  collected: number;
  outstanding: number;
  overdue: number;
  written_off: number;
  by_month: ReportRow[];
  by_client: ReportRow[];
  by_status: ReportRow[];
  aging: ReportRow[];
  by_currency: ReportRow[];
}

export interface Business {
  business_name: string;
  address: string;
  tax_id: string;
  email: string;
  phone: string;
  logo_url: string;
  payment_info: string;
  accent?: string;
  footer?: string;
  state_code?: string;
  bank_details?: string;
  upi_id?: string;
  upi_name?: string;
}

export interface TaxRate {
  id: string;
  name: string;
  rate: number;
  inclusive: boolean;
  is_default: boolean;
}
