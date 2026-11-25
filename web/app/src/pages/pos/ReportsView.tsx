import { useEffect, useState } from "react";
import { Box, Button, Sheet, Table, Typography } from "@mui/joy";
import { getReport } from "./api";
import { getProfit } from "./extApi";
import type { ProfitSummary } from "./extApi";
import { dayRange, methodLabel, money, num, qtyText } from "./logic";
import type { PosReport, PosReportRow, PosSettings } from "./types";
import { ErrorAlert, Stat, rightCell, useErr } from "./ui";

const RANGES: { id: number; label: string }[] = [
  { id: 0, label: "Today" },
  { id: 6, label: "7 days" },
  { id: 29, label: "30 days" },
  { id: 89, label: "90 days" },
];

export function ReportsView({ settings, version }: { settings: PosSettings; version: number }) {
  const [days, setDays] = useState(29);
  const [rep, setRep] = useState<PosReport | null>(null);
  const [profit, setProfit] = useState<ProfitSummary | null>(null);
  const { err, run } = useErr();
  const m = (n: number) => money(n, settings.currency);
  useEffect(() => {
    const { since, until } = dayRange(days);
    run(() => getReport(since, until)).then((r) => r && setRep(r));
    getProfit(since, until).then(setProfit).catch(() => setProfit(null));
  }, [days, version, run]);

  const table = (title: string, head: string[], rows: PosReportRow[], cells: (r: PosReportRow) => (string | number)[], testid: string) => (
    <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto", flex: "1 1 320px" }} data-testid={testid}>
      <Typography level="title-sm" sx={{ p: 1.5 }}>
        {title}
      </Typography>
      {rows.length === 0 ? (
        <Typography level="body-sm" sx={{ px: 1.5, pb: 1.5 }}>
          Nothing in this period.
        </Typography>
      ) : (
        <Table size="sm" aria-label={title}>
          <thead>
            <tr>
              {head.map((h, i) => (
                <th key={h} style={i ? rightCell : undefined}>
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <tr key={r.key + r.label}>
                {cells(r).map((c, i) => (
                  <td key={i} style={i ? rightCell : undefined}>
                    {c}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </Table>
      )}
    </Sheet>
  );

  return (
    <Box>
      <Box sx={{ display: "flex", gap: 1, mb: 2 }}>
        {RANGES.map((r) => (
          <Button key={r.id} size="sm" variant={days === r.id ? "solid" : "outlined"} onClick={() => setDays(r.id)} data-testid={`pos-range-${r.id}`}>
            {r.label}
          </Button>
        ))}
      </Box>
      <ErrorAlert err={err} />
      {rep && (
        <>
          <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap", mb: 2 }}>
            <Stat label="Sales" value={String(num(rep.sales_count))} testid="pos-rep-count" />
            <Stat label="Total sales" value={m(rep.total)} testid="pos-rep-total" />
            <Stat label="Refunds" value={m(rep.refunds)} />
            <Stat label="Net" value={m(rep.net)} testid="pos-rep-net" />
            <Stat label="Average sale" value={m(rep.average_sale)} />
            <Stat label="Discounts" value={m(rep.discounts)} />
            <Stat label="Tax collected" value={m(rep.tax)} />
          </Box>
          {profit?.any && (
            <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap", mb: 2 }} data-testid="pos-rep-profit">
              <Stat label="Gross profit" value={m(profit.profit)} testid="pos-rep-profit-value" />
              <Stat label="Margin" value={profit.margin === null ? "n/a" : `${profit.margin}%`} testid="pos-rep-margin" />
              <Stat label="Cost of goods" value={m(profit.cost)} />
              <Stat label="Cost known for" value={`${Math.round(profit.coverage * 100)}% of sales`} testid="pos-rep-coverage" />
            </Box>
          )}
          <Box sx={{ display: "flex", gap: 2, flexWrap: "wrap", mb: 2 }}>
            {table("Sales by day", ["Day", "Sales", "Total"], rep.by_day ?? [], (r) => [r.label, num(r.count), m(r.amount)], "pos-rep-days")}
            {table("Payments by method", ["Method", "Payments", "Amount"], rep.by_method ?? [], (r) => [methodLabel(r.label), num(r.count), m(r.amount)], "pos-rep-methods")}
          </Box>
          <Box sx={{ display: "flex", gap: 2, flexWrap: "wrap" }}>
            {table("Top products", ["Product", "Units", "Sales"], rep.top_products ?? [], (r) => [r.label, qtyText(r.qty), m(r.amount)], "pos-rep-top")}
            {profit?.any &&
              table("Top products by profit", ["Product", "Net sales", "Cost", "Profit"], (profit.top_by_profit ?? []).map((r) => ({ key: r.key, label: r.name, count: 0, qty: 0, amount: r.profit, extra: r.cost, base: r.net }) as PosReportRow), (r) => [r.label, m(r.base), m(r.extra), m(r.amount)], "pos-rep-top-profit")}
            {table("Tax summary", ["Tax", "Rate", "Taxable", "Tax"], rep.tax_summary ?? [], (r) => [r.label, `${r.extra}%`, m(r.base), m(r.amount)], "pos-rep-tax")}
          </Box>
        </>
      )}
    </Box>
  );
}
