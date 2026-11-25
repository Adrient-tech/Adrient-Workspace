/**
 * Inventory assistant tools: low-stock summary, reorder suggestions and
 * purchase-order drafts. Stock figures come from the Inventory API only.
 * POs are created in state "draft"; approving and sending stay with a person.
 */
import type { AssistantTool } from "@/lib/ai/types";
import { arr, bizTool, bool, capList, clip, has, int, num, obj, round2, str } from "@/lib/ai/biz";
import { listProducts, listSuppliers, reorderList } from "../api";
import { api } from "../stockApi";

export const INV_SUGGESTIONS = [
  "What is low on stock right now?",
  "Suggest what to reorder, grouped by supplier",
  "Draft a purchase order for everything below minimum from our main supplier",
  "Which products have no stock movement for 90 days?",
];
export const INV_HINT =
  "Stock quantities, minimums and costs come from tool results. Group reorder suggestions by supplier and show quantity x unit cost. A purchase order you create is only a draft for a person to approve.";

export function createInventoryTools(): AssistantTool[] {
  const A = "inventory";
  return [
    bizTool(A, {
      name: "inv_low_stock",
      description: "Products at or below their minimum stock level: stock on hand, minimum, shortfall and reorder quantity.",
      parameters: obj({ category: str("category filter"), limit: int("max rows (default 40)") }),
      run: async (a) => {
        const rows = (await listProducts({ low_stock_only: true, flat: true })).filter((p) => !p.archived && (!a.category || has(p.category, String(a.category))));
        const list = rows
          .map((p) => ({ product_id: p.id, name: clip(p.name, 80), sku: p.sku, category: p.category, unit: p.unit, stock: p.total_stock, min_stock: p.min_stock, shortfall: Math.max(0, p.min_stock - p.total_stock), reorder_qty: p.reorder_qty }))
          .sort((x, y) => y.shortfall - x.shortfall);
        return { result: { count: list.length, ...capList(list, Math.min(Number(a.limit) || 40, 80)) } };
      },
    }),
    bizTool(A, {
      name: "inv_find_product",
      description: "Look up products by name, SKU or barcode: stock, price, cost, minimum and suppliers.",
      parameters: obj({ query: str("name, SKU or barcode") }, ["query"]),
      run: async (a) => {
        const rows = await listProducts({ query: String(a.query), flat: true });
        return {
          result: capList(
            rows.filter((p) => !p.archived).map((p) => ({ product_id: p.id, name: clip(p.name, 80), sku: p.sku, barcode: p.barcode, stock: p.total_stock, min_stock: p.min_stock, price: p.price, cost: p.cost, unit: p.unit })),
            15,
          ),
        };
      },
    }),
    bizTool(A, {
      name: "inv_shrinkage",
      description: "Stock written off as damaged, expired or lost in the last N days, per product with units and value at cost; in-house use is reported apart. Also says how many removals had no reason recorded.",
      parameters: obj({ days: int("period in days (default 30)") }),
      sensitive: true,
      run: async (a) => {
        const r = await api.shrinkage(Math.min(Math.max(Number(a.days) || 30, 1), 730));
        return { result: { days: r.days, total_units: r.total_units, total_value: r.total_value, internal_use_units: r.internal_use_units, removals_without_reason: r.unspecified_removals, ...capList((r.rows ?? []).map((x) => ({ name: clip(x.name, 80), sku: x.sku, units: x.units, value: x.value, reasons: x.reasons })), 25) } };
      },
    }),
    bizTool(A, {
      name: "inv_margins",
      description: "Selling margin per product (price, cost, margin, margin percent), thinnest first. Products priced below cost come first.",
      parameters: obj({ limit: int("max rows (default 25)"), below_cost_only: bool("only products priced below cost") }),
      sensitive: true,
      run: async (a) => {
        const rows = (await api.margins()).filter((r) => !a.below_cost_only || r.below_cost);
        return { result: capList(rows.map((r) => ({ name: clip(r.name, 80), sku: r.sku, price: r.price, cost: r.cost, margin: r.margin, percent: r.percent, below_cost: r.below_cost })), Math.min(Number(a.limit) || 25, 60)) };
      },
    }),
    bizTool(A, {
      name: "inv_reorder_suggestions",
      description: "Suggested purchase quantities for low-stock products with the supplier and unit cost to use, grouped by supplier with line and supplier totals. Based on the Inventory reorder list.",
      run: async () => {
        const lines = await reorderList();
        const g = new Map<string, { supplier_id: string; supplier: string; lines: { product_id: string; name: string; sku: string; stock: number; qty: number; unit_cost: number; line_total: number }[]; total: number }>();
        for (const l of lines) {
          const k = l.supplier_id || "none";
          const e = g.get(k) ?? { supplier_id: l.supplier_id, supplier: l.supplier_name || "No supplier set", lines: [], total: 0 };
          const lt = round2(l.suggested_qty * l.unit_cost);
          e.lines.push({ product_id: l.product.id, name: clip(l.product.name, 80), sku: l.product.sku, stock: l.stock, qty: l.suggested_qty, unit_cost: l.unit_cost, line_total: lt });
          e.total = round2(e.total + lt);
          g.set(k, e);
        }
        return { result: { suppliers: [...g.values()], lines: lines.length, note: "Products with no supplier cannot go on a purchase order until a supplier is set." } };
      },
    }),
    bizTool(A, {
      name: "inv_draft_po",
      description: "Create a DRAFT purchase order for one supplier (not approved, not sent). Needs the user's confirmation. Use supplier_id and product ids from inv_reorder_suggestions / inv_find_product.",
      write: true,
      parameters: obj(
        {
          supplier_id: str("supplier id"),
          supplier_name: str("supplier name, for the confirmation"),
          lines: arr(obj({ product_id: str("product id"), name: str("product name, for the confirmation"), qty: num("quantity"), unit_cost: num("unit cost, optional") }, ["product_id", "qty"]), "order lines (max 50)"),
          expected_date: str("YYYY-MM-DD"),
          notes: str("notes for the supplier"),
        },
        ["supplier_id", "lines"],
      ),
      label: (a) => {
        const l = (a.lines as { name?: string; qty?: number }[]) ?? [];
        return `Create draft PO for ${clip(a.supplier_name, 40) || "supplier"}: ${l.slice(0, 3).map((x) => `${x.qty} x ${clip(x.name, 25) || "item"}`).join(", ")}${l.length > 3 ? ` and ${l.length - 3} more` : ""}`;
      },
      run: async (a) => {
        const lines = (a.lines as { product_id: string; qty: number; unit_cost?: number }[]) ?? [];
        if (!lines.length || lines.length > 50) return { error: "A purchase order needs 1 to 50 lines." };
        if (lines.some((l) => !(Number(l.qty) > 0))) return { error: "Every line needs a quantity above zero." };
        if (a.expected_date && !/^\d{4}-\d{2}-\d{2}$/.test(String(a.expected_date))) return { error: "expected_date must be YYYY-MM-DD" };
        const sup = (await listSuppliers()).find((s) => s.id === a.supplier_id);
        if (!sup) return { error: "Unknown supplier id." };
        const po = await api.createOrder({
          supplier_id: sup.id,
          expected_date: a.expected_date ? String(a.expected_date) : undefined,
          notes: clip(a.notes, 500),
          lines: lines.map((l) => ({ product_id: l.product_id, qty: Number(l.qty), unit_cost: l.unit_cost == null ? null : Number(l.unit_cost) })),
        });
        return {
          target: po.id,
          result: { id: po.id, number: po.number, state: po.state, total: po.total, currency: po.currency, note: "Draft only: a person must approve and send it." },
          summary: `Created draft PO ${po.number} for ${sup.name} (${lines.length} lines)`,
          undo: async () => {
            await api.orderAction(po.id, "cancel");
          },
        };
      },
    }),
  ];
}
