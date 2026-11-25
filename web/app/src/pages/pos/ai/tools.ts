/**
 * POS assistant tools. Read-only: a day summary from the POS report and a
 * product lookup. The assistant never rings up, refunds or changes a sale.
 */
import type { AssistantTool } from "@/lib/ai/types";
import { bizTool, capList, clip, int, obj, round2, str, today } from "@/lib/ai/biz";
import { listProducts } from "../../inventory/api";
import * as api from "../api";
import { getProfit } from "../extApi";

export const POS_SUGGESTIONS = [
  "Summarise today's sales",
  "Which products sold best this week?",
  "How much do we have of the blue notebook, and what is its price?",
  "Compare today with yesterday",
];
export const POS_HINT = "Answer from the sales report and product tools. Amounts are exact figures from the tool results. You cannot ring up, refund or change sales.";

const n = (v: number | string) => Number(v) || 0;

export function createPosTools(): AssistantTool[] {
  const A = "pos";
  return [
    bizTool(A, {
      name: "pos_day_summary",
      description: "Sales summary for a day or date range: number of sales, gross, discounts, tax, total, refunds, net, average sale, payment methods and top products. Default: today.",
      parameters: obj({ from: str("YYYY-MM-DD (default today)"), to: str("YYYY-MM-DD (default = from)") }),
      sensitive: true,
      run: async (a) => {
        const from = /^\d{4}-\d{2}-\d{2}$/.test(String(a.from ?? "")) ? String(a.from) : today();
        const to = /^\d{4}-\d{2}-\d{2}$/.test(String(a.to ?? "")) ? String(a.to) : from;
        const r = await api.getReport(from, to);
        const row = (x: { label: string; count: number | string; qty: number; amount: number }) => ({ label: clip(x.label, 60), count: n(x.count), qty: x.qty, amount: round2(x.amount) });
        return {
          result: {
            from, to, sales: n(r.sales_count), gross: r.gross, discounts: r.discounts, tax: r.tax, total: r.total, refunds: r.refunds, refund_count: n(r.refunds_count), net: r.net, average_sale: round2(r.average_sale),
            by_method: r.by_method.map(row), top_products: r.top_products.slice(0, 10).map(row), by_day: r.by_day.slice(-14).map(row),
          },
        };
      },
    }),
    bizTool(A, {
      name: "pos_profit",
      description: "Gross profit and margin over a date range from the cost frozen on each sale line, net of tax and refunds. Says how much of the trade had a recorded cost (coverage) and lists the top products by profit. Default: last 30 days.",
      parameters: obj({ from: str("YYYY-MM-DD"), to: str("YYYY-MM-DD") }),
      sensitive: true,
      run: async (a) => {
        const to = /^\d{4}-\d{2}-\d{2}$/.test(String(a.to ?? "")) ? String(a.to) : today();
        const from = /^\d{4}-\d{2}-\d{2}$/.test(String(a.from ?? "")) ? String(a.from) : new Date(Date.parse(to) - 29 * 86400000).toISOString().slice(0, 10);
        const p = await getProfit(from, to);
        if (!p.any) return { result: { from, to, note: "No sale in this period has a recorded cost, so profit cannot be worked out.", net_sales: p.net } };
        return { result: { from, to, net_sales: p.net, costed_sales: p.known_net, cost: p.cost, profit: p.profit, margin_percent: p.margin, coverage_percent: Math.round(p.coverage * 100), lines_without_cost: p.unknown_lines, top_by_profit: (p.top_by_profit ?? []).map((r) => ({ product: clip(r.name, 60), net: r.net, cost: r.cost, profit: r.profit })) } };
      },
    }),
    bizTool(A, {
      name: "pos_find_product",
      description: "Look up products for sale by name, SKU or barcode: price, stock and unit.",
      parameters: obj({ query: str("name, SKU or barcode"), limit: int("max rows (default 10)") }, ["query"]),
      run: async (a) => {
        const rows = await listProducts({ query: String(a.query), flat: true });
        return { result: capList(rows.filter((p) => !p.archived).map((p) => ({ name: clip(p.name, 80), sku: p.sku, barcode: p.barcode, price: p.price, stock: p.total_stock, unit: p.unit, category: p.category })), Math.min(Number(a.limit) || 10, 25)) };
      },
    }),
  ];
}
