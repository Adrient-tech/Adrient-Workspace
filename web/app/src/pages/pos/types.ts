/** Mirrors adrient.v1 pos.proto (snake_case via the gateway). int64 fields arrive as strings. */

export interface PosRegister {
  id: string;
  name: string;
  location_id: string;
  archived: boolean;
  open_shift_id: string;
  created_at: string;
}

export interface PosCashEntry {
  id: string;
  amount: number;
  note: string;
  created_at: string;
}

export interface PosShift {
  id: string;
  register_id: string;
  register_name: string;
  opened_by: string;
  opened_at: string;
  opening_cash: number;
  closed_at: string;
  closed: boolean;
  counted_cash: number;
  expected_cash: number;
  variance: number;
  note: string;
  cash_sales: number;
  cash_refunds: number;
  cash_in: number;
  cash_out: number;
  sales_count: number;
  sales_total: number;
  entries: PosCashEntry[];
}

export type DiscountKind = "percent" | "amount" | "";
export type PayMethod = "cash" | "card" | "other" | "store_credit" | "loyalty";

export interface PosLineInput {
  product_id?: string;
  name: string;
  sku?: string;
  qty: number;
  unit_price: number;
  discount_kind?: DiscountKind;
  discount_value?: number;
  tax_rate_id?: string;
  note?: string;
}

export interface PosPaymentInput {
  method: PayMethod;
  amount: number;
  ref?: string;
}

export interface PosSaleInput {
  id?: string;
  register_id: string;
  customer_id?: string;
  customer_name?: string;
  lines: PosLineInput[];
  discount_kind?: DiscountKind;
  discount_value?: number;
  note?: string;
  hold_label?: string;
  payments?: PosPaymentInput[];
}

export interface PosSaleLine {
  id: string;
  product_id: string;
  name: string;
  sku: string;
  qty: number;
  unit_price: number;
  discount_kind: DiscountKind;
  discount_value: number;
  tax_rate_id: string;
  tax_name: string;
  tax_rate: number;
  tax_inclusive: boolean;
  discount_amount: number;
  tax_amount: number;
  line_total: number;
  refunded_qty: number;
  refunded_amount: number;
  note: string;
}

export interface PosPayment {
  id: string;
  method: PayMethod;
  amount: number;
  tendered: number;
  ref: string;
  created_at: string;
}

export interface PosRefundLine {
  line_id: string;
  name: string;
  qty: number;
  amount: number;
}

export interface PosRefund {
  id: string;
  number: string;
  sale_id: string;
  total: number;
  method: string;
  note: string;
  restock: boolean;
  created_at: string;
  lines: PosRefundLine[];
}

export type SaleStatus = "held" | "completed" | "partially_refunded" | "refunded";

export interface PosSale {
  id: string;
  number: number | string;
  register_id: string;
  register_name: string;
  shift_id: string;
  customer_id: string;
  customer_name: string;
  status: SaleStatus;
  subtotal: number;
  discount_total: number;
  tax_total: number;
  total: number;
  paid: number;
  change_due: number;
  discount_kind: DiscountKind;
  discount_value: number;
  note: string;
  hold_label: string;
  refunded_total: number;
  loyalty_earned: number;
  created_by: string;
  created_at: string;
  completed_at: string;
  lines: PosSaleLine[];
  payments: PosPayment[];
  refunds: PosRefund[];
}

export interface PosTaxRate {
  id: string;
  name: string;
  rate: number;
  inclusive: boolean;
  is_default: boolean;
}

export interface PosDiscountPreset {
  id: string;
  name: string;
  kind: "percent" | "amount";
  value: number;
}

export interface PosSettings {
  store_name: string;
  currency: string;
  receipt_header: string;
  receipt_footer: string;
  loyalty_earn_per_unit: number;
  loyalty_point_value: number;
}

export interface PosBalanceEntry {
  id: string;
  kind: string;
  delta: number;
  reason: string;
  sale_id: string;
  note: string;
  created_at: string;
}

export interface PosBalance {
  contact_id: string;
  loyalty_points: number;
  store_credit: number;
  updated_at: string;
  entries: PosBalanceEntry[];
}

export interface PosReportRow {
  key: string;
  label: string;
  count: number | string;
  qty: number;
  amount: number;
  extra: number;
  base: number;
}

export interface PosReport {
  sales_count: number | string;
  gross: number;
  discounts: number;
  tax: number;
  total: number;
  refunds: number;
  refunds_count: number | string;
  net: number;
  average_sale: number;
  by_day: PosReportRow[];
  by_method: PosReportRow[];
  top_products: PosReportRow[];
  tax_summary: PosReportRow[];
}

/** A cart line in the register (before it is sent to the server). */
export interface CartLine {
  key: string;
  product_id: string;
  name: string;
  sku: string;
  qty: number;
  unit_price: number;
  discount_kind: DiscountKind;
  discount_value: number;
  tax_rate_id: string;
  note: string;
}
