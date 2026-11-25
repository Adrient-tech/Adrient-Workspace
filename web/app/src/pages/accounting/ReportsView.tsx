import { useCallback, useEffect, useState } from "react";
import { Box, Button, Chip, Input, Option, Select, Sheet, Table, Typography } from "@mui/joy";
import { getReport } from "./api";
import type { ReportName } from "./api";
import { auditExport } from "../../lib/auditExport";
import { cell, downloadCSV, fyEnd, fyStart, money, todayISO } from "./logic";
import type { Aging, BalanceSheet, CashFlow, GSTReport, PnL, RRow, Settings, TrialBalance } from "./types";
import { ErrorAlert, rightCell, useErr } from "./ui";
import { DrillDown, ExportButtons, GenericReport } from "./ReportTable";

const REPORTS: { id: ReportName; label: string; range: boolean }[] = [
  { id: "pnl", label: "Profit and Loss", range: true },
  { id: "balance-sheet", label: "Balance Sheet", range: false },
  { id: "trial-balance", label: "Trial Balance", range: false },
  { id: "cash-flow", label: "Cash Flow", range: true },
  { id: "aged-receivables", label: "Aged Receivables", range: false },
  { id: "aged-payables", label: "Aged Payables", range: false },
  { id: "gst", label: "GST summary", range: true },
  { id: "pnl-by-class", label: "Profit and Loss by class", range: true },
  { id: "budget-vs-actual", label: "Budget vs actual", range: true },
  { id: "pnl-columns", label: "Profit and Loss by month, quarter or year", range: true },
  { id: "general-ledger", label: "General Ledger", range: true },
  { id: "journal", label: "Journal", range: true },
  { id: "transaction-list", label: "Transaction List", range: true },
  { id: "account-list", label: "Account List", range: false },
  { id: "customer-balances", label: "Customer Balance Summary", range: false },
  { id: "vendor-balances", label: "Vendor Balance Summary", range: false },
  { id: "sales-by-customer", label: "Sales by Customer", range: true },
  { id: "expenses-by-vendor", label: "Expenses by Vendor", range: true },
];
const GENERIC: ReportName[] = ["pnl-by-class", "budget-vs-actual", "pnl-columns", "general-ledger", "journal", "transaction-list", "account-list", "customer-balances", "vendor-balances", "sales-by-customer", "expenses-by-vendor"];

type Data = TrialBalance | PnL | BalanceSheet | CashFlow | Aging | GSTReport;

function Section({ title, rows, total, totalLabel, onDrill }: { title: string; rows: RRow[]; total?: number; totalLabel?: string; onDrill?: (id: string, name: string) => void }) {
  return (
    <>
      <tr>
        <td colSpan={2}>
          <Typography level="title-sm">{title}</Typography>
        </td>
      </tr>
      {rows.map((r) => (
        <tr key={r.account_id} onClick={onDrill ? () => onDrill(r.account_id, `${r.code} ${r.name}`) : undefined} style={{ cursor: onDrill ? "pointer" : undefined }} data-testid="acct-report-row">
          <td style={{ paddingLeft: 24 }}>
            {r.code} {r.name}
          </td>
          <td style={rightCell}>{money(r.amount)}</td>
        </tr>
      ))}
      {total !== undefined && (
        <tr>
          <td>
            <b>{totalLabel ?? `Total ${title}`}</b>
          </td>
          <td style={rightCell}>
            <b>{money(total)}</b>
          </td>
        </tr>
      )}
    </>
  );
}

export function ReportsView({ settings, version, onOpenJournal }: { settings: Settings; version: number; onOpenJournal?: (id: string) => void }) {
  const today = todayISO();
  const [name, setName] = useState<ReportName>("pnl");
  const [from, setFrom] = useState(fyStart(today, settings.fy_start_month));
  const [to, setTo] = useState(today);
  const [loaded, setLoaded] = useState<{ name: ReportName; d: Data } | null>(null);
  const data = loaded && loaded.name === name ? loaded.d : null;
  const { err, run } = useErr();
  const def = REPORTS.find((r) => r.id === name)!;
  const [compare, setCompare] = useState("");
  const [drill, setDrill] = useState<{ id: string; name: string } | null>(null);
  const canCompare = name === "pnl" || name === "balance-sheet";
  const cmp = canCompare ? compare : "";
  const generic = GENERIC.includes(name) || cmp !== "";
  const [by, setBy] = useState("month");
  const gq: Record<string, string | undefined> = def.range ? { from, to, compare: cmp || undefined, by: name === "pnl-columns" ? by : undefined } : { as_of: to, compare: cmp || undefined };
  const onDrill = (id: string, label: string) => setDrill({ id, name: label });

  const load = useCallback(async () => {
    if (generic) return;
    const d = await run(() => getReport<Data>(name, def.range ? { from, to } : { as_of: to }));
    if (d) setLoaded({ name, d });
  }, [name, from, to, def.range, run, generic]);
  useEffect(() => {
    load();
  }, [load, version]);

  const fy = () => {
    const s = fyStart(today, settings.fy_start_month);
    setFrom(s);
    setTo(fyEnd(s) < today ? fyEnd(s) : today);
  };
  const lastFy = () => {
    const s = fyStart(today, settings.fy_start_month);
    const [y, m] = s.split("-").map(Number);
    const prev = `${y - 1}-${String(m).padStart(2, "0")}-01`;
    setFrom(prev);
    setTo(fyEnd(prev));
  };

  const exportCSV = () => {
    if (!data) return;
    const rows: (string | number)[][] = [[def.label, def.range ? `${from} to ${to}` : `as of ${to}`]];
    if (name === "trial-balance") {
      const d = data as TrialBalance;
      rows.push(["Code", "Account", "Debit", "Credit"], ...d.rows.map((r) => [r.code, r.name, r.debit, r.credit]), ["", "Total", d.total_debit, d.total_credit]);
    } else if (name === "pnl") {
      const d = data as PnL;
      rows.push(["Income"], ...d.income.map((r) => [r.name, r.amount]), ["Total income", d.total_income], ["Expenses"], ...d.expenses.map((r) => [r.name, r.amount]), ["Total expenses", d.total_expenses], ["Net profit", d.net_profit]);
    } else if (name === "balance-sheet") {
      const d = data as BalanceSheet;
      rows.push(["Assets"], ...d.assets.map((r) => [r.name, r.amount]), ["Total assets", d.total_assets], ["Liabilities"], ...d.liabilities.map((r) => [r.name, r.amount]), ["Equity"], ...d.equity.map((r) => [r.name, r.amount]), ["Retained earnings", d.retained_earnings], ["Current year earnings", d.current_year_earnings], ["Total liabilities and equity", d.total_liabilities + d.total_equity]);
    } else if (name === "cash-flow") {
      const d = data as CashFlow;
      rows.push(["Opening cash and bank", d.opening]);
      d.groups.forEach((g) => rows.push([g.name], ...g.rows.map((r) => [r.name, r.amount]), ["Total", g.total]));
      rows.push(["Net change", d.net_change], ["Closing cash and bank", d.closing]);
    } else if (name === "gst") {
      const d = data as GSTReport;
      rows.push(["Output tax"], ...d.output.map((r) => [r.name, r.amount]), ["Input tax credit"], ...d.input.map((r) => [r.name, r.amount]), ["Net GST payable", d.net_payable]);
    } else {
      const d = data as Aging;
      rows.push(["Contact", "Current", "1-30", "31-60", "61-90", "90+", "Total"], ...[...d.rows, d.total].map((r) => [r.contact_name, r.current, r.d1_30, r.d31_60, r.d61_90, r.d90_plus, r.total]));
    }
    auditExport("accounting", `report:${name}`, rows.length);
    downloadCSV(`${name}-${to}.csv`, rows);
  };

  return (
    <Box>
      <Box className="no-print" sx={{ display: "flex", gap: 1, alignItems: "flex-end", flexWrap: "wrap", mb: 1.5 }}>
        <Typography level="h3" sx={{ mr: "auto" }}>
          Reports
        </Typography>
        <Select size="sm" value={name} onChange={(_, v) => setName((v as ReportName) ?? "pnl")} sx={{ minWidth: 190 }} slotProps={{ button: { "data-testid": "acct-report-select" } as never }}>
          {REPORTS.map((r) => (
            <Option key={r.id} value={r.id}>
              {r.label}
            </Option>
          ))}
        </Select>
        {canCompare && (
          <Select size="sm" value={compare} onChange={(_, v) => setCompare(String(v ?? ""))} sx={{ minWidth: 190 }} slotProps={{ button: { "data-testid": "acct-report-compare" } as never }}>
            <Option value="">No comparison</Option>
            <Option value="prev_year">Compare with previous year</Option>
            {name === "pnl" && <Option value="prev_period">Compare with previous period</Option>}
            {name === "pnl" && <Option value="budget">Compare with budget</Option>}
          </Select>
        )}
        {name === "pnl-columns" && (
          <Select size="sm" value={by} onChange={(_, v) => setBy(String(v ?? "month"))} sx={{ minWidth: 130 }} slotProps={{ button: { "data-testid": "acct-report-by" } as never }}>
            <Option value="month">By month</Option>
            <Option value="quarter">By quarter</Option>
            <Option value="year">By year</Option>
          </Select>
        )}
        {def.range && <Input size="sm" type="date" value={from} onChange={(e) => setFrom(e.target.value)} aria-label="From date" data-testid="acct-report-from" />}
        <Input size="sm" type="date" value={to} onChange={(e) => setTo(e.target.value)} aria-label={def.range ? "To date" : "As of date"} data-testid="acct-report-to" />
        {def.range && (
          <>
            <Button size="sm" variant="plain" onClick={fy}>
              This FY
            </Button>
            <Button size="sm" variant="plain" onClick={lastFy}>
              Last FY
            </Button>
          </>
        )}
        {!generic && (
          <Button size="sm" variant="soft" onClick={exportCSV} disabled={!data} data-testid="acct-report-csv">
            Export CSV
          </Button>
        )}
        <ExportButtons name={name} q={gq} />
        <Button size="sm" variant="soft" onClick={() => window.print()}>
          Print
        </Button>
      </Box>
      <ErrorAlert err={err} />
      {generic && <GenericReport name={name} q={gq} version={version} drillFrom={def.range ? from : undefined} drillTo={to} onOpenJournal={onOpenJournal} />}
      {!generic && data && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", p: 2, overflow: "auto" }} data-testid="acct-report-body">
          <Typography level="title-lg">{settings.business_name || "Your business"}</Typography>
          <Typography level="title-md">{def.label}</Typography>
          <Typography level="body-sm" sx={{ mb: 1 }}>
            {def.range ? `${from} to ${to}` : `As of ${to}`}
          </Typography>
          {name === "pnl" && <PnLTable d={data as PnL} onDrill={onDrill} />}
          {name === "balance-sheet" && <BSTable d={data as BalanceSheet} onDrill={onDrill} />}
          {name === "trial-balance" && <TBTable d={data as TrialBalance} onDrill={onDrill} />}
          {name === "cash-flow" && <CFTable d={data as CashFlow} />}
          {(name === "aged-receivables" || name === "aged-payables") && <AgingTable d={data as Aging} />}
          {name === "gst" && <GSTTable d={data as GSTReport} />}
        </Sheet>
      )}
      {drill && <DrillDown accountId={drill.id} name={drill.name} from={name === "balance-sheet" || name === "trial-balance" ? undefined : from} to={to} onClose={() => setDrill(null)} onOpenJournal={onOpenJournal} />}
    </Box>
  );
}

function PnLTable({ d, onDrill }: { d: PnL; onDrill: (id: string, name: string) => void }) {
  return (
    <Table size="sm" data-testid="acct-pnl">
      <tbody>
        <Section title="Income" rows={d.income} total={d.total_income} onDrill={onDrill} />
        <Section title="Expenses" rows={d.expenses} total={d.total_expenses} onDrill={onDrill} />
        <tr>
          <td>
            <b>Net profit</b>
          </td>
          <td style={rightCell} data-testid="acct-net-profit">
            <b>{money(d.net_profit)}</b>
          </td>
        </tr>
      </tbody>
    </Table>
  );
}

function BSTable({ d, onDrill }: { d: BalanceSheet; onDrill: (id: string, name: string) => void }) {
  return (
    <>
      <Table size="sm" data-testid="acct-bs">
        <tbody>
          <Section title="Assets" rows={d.assets} total={d.total_assets} onDrill={onDrill} />
          <Section title="Liabilities" rows={d.liabilities} total={d.total_liabilities} onDrill={onDrill} />
          <Section title="Equity" rows={d.equity} onDrill={onDrill} />
          <tr>
            <td style={{ paddingLeft: 24 }}>Retained earnings (previous years)</td>
            <td style={rightCell}>{money(d.retained_earnings)}</td>
          </tr>
          <tr>
            <td style={{ paddingLeft: 24 }}>Current year earnings</td>
            <td style={rightCell}>{money(d.current_year_earnings)}</td>
          </tr>
          <tr>
            <td>
              <b>Total equity</b>
            </td>
            <td style={rightCell}>
              <b>{money(d.total_equity)}</b>
            </td>
          </tr>
          <tr>
            <td>
              <b>Total liabilities and equity</b>
            </td>
            <td style={rightCell} data-testid="acct-bs-total">
              <b>{money(d.total_liabilities + d.total_equity)}</b>
            </td>
          </tr>
        </tbody>
      </Table>
      <Chip size="sm" color={d.balanced ? "success" : "danger"} variant="soft" sx={{ mt: 1 }} data-testid="acct-bs-balanced">
        {d.balanced ? "Balanced: assets equal liabilities plus equity" : "Out of balance"}
      </Chip>
    </>
  );
}

function TBTable({ d, onDrill }: { d: TrialBalance; onDrill: (id: string, name: string) => void }) {
  return (
    <>
      <Table size="sm" data-testid="acct-tb">
        <thead>
          <tr>
            <th>Account</th>
            <th style={rightCell}>Debit</th>
            <th style={rightCell}>Credit</th>
          </tr>
        </thead>
        <tbody>
          {d.rows.map((r) => (
            <tr key={r.account_id} onClick={() => onDrill(r.account_id, `${r.code} ${r.name}`)} style={{ cursor: "pointer" }} data-testid="acct-report-row">
              <td>
                {r.code} {r.name}
              </td>
              <td style={rightCell}>{cell(r.debit)}</td>
              <td style={rightCell}>{cell(r.credit)}</td>
            </tr>
          ))}
          <tr>
            <td>
              <b>Total</b>
            </td>
            <td style={rightCell} data-testid="acct-tb-debit">
              <b>{money(d.total_debit)}</b>
            </td>
            <td style={rightCell} data-testid="acct-tb-credit">
              <b>{money(d.total_credit)}</b>
            </td>
          </tr>
        </tbody>
      </Table>
      <Chip size="sm" color={d.balanced ? "success" : "danger"} variant="soft" sx={{ mt: 1 }} data-testid="acct-tb-balanced">
        {d.balanced ? "Debits equal credits" : "Debits and credits differ"}
      </Chip>
    </>
  );
}

function CFTable({ d }: { d: CashFlow }) {
  return (
    <>
      <Table size="sm" data-testid="acct-cf">
        <tbody>
          <tr>
            <td>Opening cash and bank</td>
            <td style={rightCell}>{money(d.opening)}</td>
          </tr>
          {d.groups.map((g) => (
            <Section key={g.name} title={g.name} rows={g.rows} total={g.total} totalLabel={`Net ${g.name.toLowerCase()}`} />
          ))}
          <tr>
            <td>
              <b>Net change in cash</b>
            </td>
            <td style={rightCell}>
              <b>{money(d.net_change)}</b>
            </td>
          </tr>
          <tr>
            <td>
              <b>Closing cash and bank</b>
            </td>
            <td style={rightCell}>
              <b>{money(d.closing)}</b>
            </td>
          </tr>
        </tbody>
      </Table>
      <Chip size="sm" color={d.ties ? "success" : "danger"} variant="soft" sx={{ mt: 1 }}>
        {d.ties ? "Ties to cash and bank balances" : "Does not tie to cash and bank balances"}
      </Chip>
    </>
  );
}

function AgingTable({ d }: { d: Aging }) {
  const all = [...d.rows, d.total];
  return (
    <Table size="sm" data-testid="acct-aging">
      <thead>
        <tr>
          <th>{d.kind === "receivable" ? "Customer" : "Vendor"}</th>
          <th style={rightCell}>Current</th>
          <th style={rightCell}>1-30</th>
          <th style={rightCell}>31-60</th>
          <th style={rightCell}>61-90</th>
          <th style={rightCell}>90+</th>
          <th style={rightCell}>Total</th>
        </tr>
      </thead>
      <tbody>
        {all.map((r, i) => (
          <tr key={i} style={i === all.length - 1 ? { fontWeight: 700 } : undefined}>
            <td>{r.contact_name}</td>
            <td style={rightCell}>{cell(r.current)}</td>
            <td style={rightCell}>{cell(r.d1_30)}</td>
            <td style={rightCell}>{cell(r.d31_60)}</td>
            <td style={rightCell}>{cell(r.d61_90)}</td>
            <td style={rightCell}>{cell(r.d90_plus)}</td>
            <td style={rightCell}>{money(r.total)}</td>
          </tr>
        ))}
      </tbody>
    </Table>
  );
}

function GSTTable({ d }: { d: GSTReport }) {
  return (
    <Table size="sm" data-testid="acct-gst">
      <tbody>
        <tr>
          <td>Sales in period (excluding GST)</td>
          <td style={rightCell}>{money(d.taxable_sales)}</td>
        </tr>
        <Section title="Output tax (collected)" rows={d.output} total={d.output_total} />
        <Section title="Input tax credit (paid)" rows={d.input} total={d.input_total} />
        <tr>
          <td>
            <b>{d.net_payable >= 0 ? "Net GST payable" : "Net input credit to carry forward"}</b>
          </td>
          <td style={rightCell}>
            <b>{money(Math.abs(d.net_payable))}</b>
          </td>
        </tr>
      </tbody>
    </Table>
  );
}
