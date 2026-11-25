/** Mirrors adrient.v1 inventory.proto (snake_case via the gateway). */

export interface InvLocation {
  id: string;
  name: string;
  code: string;
  is_default: boolean;
  created_at: string;
}

export interface InvSupplier {
  id: string;
  name: string;
  /** Optional Contacts record id. */
  contact_id: string;
  email: string;
  phone: string;
  notes: string;
  product_count: number;
  created_at: string;
}

export interface InvProductSupplier {
  supplier_id: string;
  supplier_name: string;
  supplier_sku: string;
  cost: number;
  lead_days: number;
  pack_size: number;
  preferred: boolean;
}

export interface InvStockLevel {
  location_id: string;
  location_name: string;
  qty: number;
  shelf_code: string;
}

export interface InvProduct {
  id: string;
  parent_id: string;
  variant_label: string;
  name: string;
  sku: string;
  barcode: string;
  description: string;
  category: string;
  unit: string;
  price: number;
  cost: number;
  min_stock: number;
  reorder_qty: number;
  photo_file_id: string;
  archived: boolean;
  total_stock: number;
  stock: InvStockLevel[];
  variants: InvProduct[];
  suppliers: InvProductSupplier[];
  created_at: string;
  updated_at: string;
}

export interface ProductInput {
  parent_id?: string;
  variant_label?: string;
  name: string;
  sku?: string;
  barcode?: string;
  description?: string;
  category?: string;
  unit?: string;
  price?: number;
  cost?: number;
  min_stock?: number;
  reorder_qty?: number;
  photo_file_id?: string;
  opening_qty?: number;
  opening_location_id?: string;
}

export type StockReason =
  | "sale"
  | "refund"
  | "receive"
  | "count"
  | "remove"
  | "correction";

export interface AdjustInput {
  location_id?: string;
  qty: number;
  absolute?: boolean;
  reason: StockReason;
  note?: string;
  ref?: string;
  shelf_code?: string;
  unit_cost?: number;
}

export interface InvHistoryEntry {
  id: string;
  product_id: string;
  product_name: string;
  sku: string;
  location_id: string;
  location_name: string;
  delta: number;
  qty_after: number;
  reason: string;
  note: string;
  ref: string;
  user_id: string;
  created_at: string;
}

export interface ReorderLine {
  product: InvProduct;
  stock: number;
  suggested_qty: number;
  supplier_id: string;
  supplier_name: string;
  supplier_sku: string;
  unit_cost: number;
}

export interface InvReportRow {
  label: string;
  units: number;
  cost_value: number;
  retail_value: number;
}

export interface InvReport {
  product_count: number;
  units: number;
  cost_value: number;
  retail_value: number;
  low_stock_count: number;
  out_of_stock_count: number;
  by_category: InvReportRow[];
  by_location: InvReportRow[];
  movement: InvReportRow[];
  top_movers: InvReportRow[];
}

export interface ImportResult {
  created: number;
  updated: number;
  skipped: number;
  errors: string[];
}
