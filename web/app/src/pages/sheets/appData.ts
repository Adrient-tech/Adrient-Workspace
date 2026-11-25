// Insert ▸ From app: pull a table from another Adrient module through its
// existing REST API and lay it out as a header row plus data rows.

export type AppSourceId = "inventory" | "pos" | "invoicing" | "crm";

export interface AppTable {
  title: string;
  header: string[];
  rows: (string | number)[][];
}

export interface AppSource {
  id: AppSourceId;
  label: string;
  description: string;
  /** Material icon name (AppIcon). */
  icon: string;
  url: string;
  /** Key of the array in the JSON response. */
  key: string;
  map: (items: any[]) => AppTable;
}

/* eslint-disable @typescript-eslint/no-explicit-any -- API payloads are mapped field by field. */

const d = (s: unknown) => (typeof s === "string" && s ? s.slice(0, 10) : "");
const n = (x: unknown) => (typeof x === "number" ? x : Number(x) || 0);
const t = (x: unknown) => (x == null ? "" : String(x));

export const APP_SOURCES: AppSource[] = [
  {
    id: "inventory",
    label: "Inventory products",
    description: "SKU, category, price, cost and stock for every product.",
    icon: "inventory_2",
    url: "/api/v1/inventory/products?flat=true",
    key: "products",
    map: (items) => ({
      title: "Products",
      header: ["Name", "SKU", "Barcode", "Category", "Unit", "Price", "Cost", "Stock", "Min stock"],
      rows: items
        .filter((p) => !p.archived)
        .map((p) => [t(p.name), t(p.sku), t(p.barcode), t(p.category), t(p.unit), n(p.price), n(p.cost), n(p.total_stock), n(p.min_stock)]),
    }),
  },
  {
    id: "pos",
    label: "POS sales",
    description: "The latest 1000 sales with totals, customer and register.",
    icon: "point_of_sale",
    url: "/api/v1/pos/sales?limit=1000",
    key: "sales",
    map: (items) => ({
      title: "POS sales",
      header: ["Number", "Date", "Register", "Customer", "Status", "Subtotal", "Discount", "Tax", "Total", "Paid"],
      rows: items.map((s) => [t(s.number), d(s.completed_at || s.created_at), t(s.register_name), t(s.customer_name), t(s.status), n(s.subtotal), n(s.discount_total), n(s.tax_total), n(s.total), n(s.paid)]),
    }),
  },
  {
    id: "invoicing",
    label: "Invoicing documents",
    description: "Invoices and estimates with status, totals and balance due.",
    icon: "receipt_long",
    url: "/api/v1/invoicing/documents",
    key: "documents",
    map: (items) => ({
      title: "Invoices",
      header: ["Number", "Type", "Client", "Issued", "Due", "Status", "Currency", "Subtotal", "Tax", "Total", "Paid", "Balance"],
      rows: items.map((x) => [t(x.number), t(x.kind), t(x.client_name), d(x.issue_date), d(x.due_date), t(x.display_status || x.status), t(x.currency), n(x.subtotal), n(x.tax_total), n(x.total), n(x.paid), n(x.balance)]),
    }),
  },
  {
    id: "crm",
    label: "CRM deals",
    description: "Pipeline deals with stage, value and expected close date.",
    icon: "handshake",
    url: "/api/v1/crm/deals",
    key: "deals",
    map: (items) => ({
      title: "Deals",
      header: ["Title", "Stage", "Value", "Currency", "Expected close", "Source", "Created"],
      rows: items.map((x) => [t(x.title), t(x.stage), n(x.value), t(x.currency), d(x.expected_close), t(x.source), d(x.created_at)]),
    }),
  },
];

export async function fetchAppTable(src: AppSource, fetcher: typeof fetch = fetch): Promise<AppTable> {
  const r = await fetcher(src.url, { credentials: "same-origin", headers: { Accept: "application/json" } });
  if (!r.ok) throw new Error(r.status === 403 ? "You do not have access to that app." : `The app answered HTTP ${r.status}.`);
  const j = await r.json();
  const items = Array.isArray(j?.[src.key]) ? j[src.key] : [];
  return src.map(items);
}

/** Sheet-tab-safe name, unique among `taken`. */
export function uniqueSheetName(base: string, taken: string[]): string {
  const clean = base.replace(/[\\/?*[\]:]/g, " ").slice(0, 28).trim() || "Data";
  if (!taken.includes(clean)) return clean;
  let i = 2;
  while (taken.includes(`${clean} ${i}`)) i++;
  return `${clean} ${i}`;
}

/** The API calls (FortuneSheet batchCallApis shape) that write a table at A1 of sheet `id`. */
export function tableWrites(tbl: AppTable, id: string): { name: string; args: any[] }[] {
  const out: { name: string; args: any[] }[] = [];
  tbl.header.forEach((h, c) => out.push({ name: "setCellValue", args: [0, c, { v: h, m: h, bl: 1, bg: "#E3F1EC", fc: "#00261F" }, null, { id }] }));
  tbl.rows.forEach((row, i) =>
    row.forEach((v, c) => {
      if (v === "") return;
      out.push({ name: "setCellValue", args: [i + 1, c, { v, m: String(v), ...(typeof v === "number" ? { ct: { fa: "General", t: "n" } } : {}) }, null, { id }] });
    }),
  );
  return out;
}
