import { useCallback, useEffect, useMemo, useState } from "react";
import { Box, Button, Checkbox, Chip, FormControl, FormLabel, Input, Option, Select, Sheet, Table, Typography } from "@mui/joy";
import { closeChecklist, closePeriod, importColumns, importOfx, listBudgets, runImport, saveBudgets } from "./api2";
import type { BudgetCell, Checklist, ImportReport } from "./api2";
import { fyEnd, fyStart, todayISO } from "./logic";
import type { Account, Settings } from "./types";
import { AccountSelect, ErrorAlert, isBank, rightCell, useErr } from "./ui";
import { GenericReport, ExportButtons } from "./ReportTable";

function monthsOfFy(start: string): string[] {
  const [y, m] = start.split("-").map(Number);
  return Array.from({ length: 12 }, (_, i) => {
    const d = new Date(Date.UTC(y, m - 1 + i, 1));
    return d.toISOString().slice(0, 7);
  });
}

export function BudgetsView({ accounts, settings, version, onChanged }: { accounts: Account[]; settings: Settings; version: number; onChanged: () => void }) {
  const [fy, setFy] = useState(fyStart(todayISO(), settings.fy_start_month));
  const months = useMemo(() => monthsOfFy(fy), [fy]);
  const [cells, setCells] = useState<Record<string, number>>({});
  const [dirty, setDirty] = useState<Record<string, number>>({});
  const [msg, setMsg] = useState("");
  const [bump, setBump] = useState(0);
  const { err, run } = useErr();
  const lines = accounts.filter((a) => a.active && (a.type === "income" || a.type === "expense"));
  const load = useCallback(async () => {
    const r = await run(() => listBudgets(months[0], months[11]));
    if (r) {
      const m: Record<string, number> = {};
      r.forEach((c: BudgetCell) => (m[`${c.account_id}|${c.month}`] = c.amount));
      setCells(m);
      setDirty({});
    }
  }, [months, run]);
  useEffect(() => {
    load();
  }, [load, version]);
  const val = (k: string) => (k in dirty ? dirty[k] : (cells[k] ?? 0));
  const to = fyEnd(fy);
  return (
    <Box>
      <Typography level="h3" sx={{ mb: 0.5 }}>
        Budgets
      </Typography>
      <Typography level="body-sm" sx={{ mb: 1 }}>
        Plan income and expenses by account and month, then compare with actuals. Amounts are in {settings.currency}.
      </Typography>
      <Box sx={{ display: "flex", gap: 1, mb: 1, alignItems: "center", flexWrap: "wrap" }}>
        <Input size="sm" type="date" value={fy} onChange={(e) => setFy(fyStart(e.target.value, settings.fy_start_month))} aria-label="Financial year start" />
        <Typography level="body-sm">to {to}</Typography>
        <Button
          size="sm"
          disabled={Object.keys(dirty).length === 0}
          data-testid="acct-budget-save"
          onClick={async () => {
            const payload: BudgetCell[] = Object.entries(dirty).map(([k, v]) => {
              const [account_id, month] = k.split("|");
              return { account_id, month, amount: v };
            });
            const r = await run(() => saveBudgets(payload));
            if (r) {
              setMsg(`Saved ${r.saved} budget amounts.`);
              setBump((b) => b + 1);
              load();
              onChanged();
            }
          }}
        >
          Save budget
        </Button>
        {msg && <Typography level="body-sm" color="success">{msg}</Typography>}
      </Box>
      <ErrorAlert err={err} />
      <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto", mb: 2, maxHeight: 420 }}>
        <Table size="sm" data-testid="acct-budget-grid" stickyHeader>
          <thead>
            <tr>
              <th style={{ minWidth: 200 }}>Account</th>
              {months.map((m) => (
                <th key={m} style={{ ...rightCell, minWidth: 84 }}>
                  {m.slice(2)}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {lines.map((a) => (
              <tr key={a.id}>
                <td>
                  {a.code} {a.name}
                </td>
                {months.map((m) => {
                  const k = `${a.id}|${m}`;
                  return (
                    <td key={m} style={{ padding: 2 }}>
                      <Input
                        size="sm"
                        type="number"
                        variant="plain"
                        value={val(k) || ""}
                        onChange={(e) => setDirty({ ...dirty, [k]: Number(e.target.value) })}
                        slotProps={{ input: { style: { textAlign: "right" }, "aria-label": `${a.name} ${m}`, "data-testid": `acct-budget-${a.code}-${m}` } }}
                      />
                    </td>
                  );
                })}
              </tr>
            ))}
          </tbody>
        </Table>
      </Sheet>
      <Typography level="title-md" sx={{ mb: 0.5 }}>
        Budget vs actual
      </Typography>
      <Box sx={{ mb: 1 }}>
        <ExportButtons name="budget-vs-actual" q={{ from: fy, to }} />
      </Box>
      <GenericReport name="budget-vs-actual" q={{ from: fy, to: todayISO() < to ? todayISO() : to }} version={version + bump} drillFrom={fy} drillTo={to} />
    </Box>
  );
}

const TYPES = [
  { id: "coa", label: "Chart of accounts" },
  { id: "opening_balances", label: "Opening balances (trial balance)" },
  { id: "contacts", label: "Customers and vendors" },
  { id: "invoices", label: "Invoices" },
  { id: "bills", label: "Bills" },
];
const SOURCES = [
  { id: "zoho", label: "Zoho Books" },
  { id: "quickbooks", label: "QuickBooks Online" },
  { id: "tally", label: "Tally" },
  { id: "generic", label: "Other CSV" },
];
const HELP: Record<string, string> = {
  zoho: "Zoho Books: Settings > Data Backup, or Reports > export as CSV. Chart of accounts, Trial Balance, Invoices, Bills and Contacts exports are understood as they come.",
  quickbooks: "QuickBooks Online: Reports > export to CSV (Account List, Trial Balance, Invoice List, Bill List) or Settings > Export data.",
  tally: "Tally: Display > Account Books, or Gateway > Export > CSV of ledgers (Particulars, Debit, Credit), Sales Register and Purchase Register.",
  generic: "Any CSV whose header row uses the column names listed below.",
};

export function ImportView({ accounts, version, onChanged }: { accounts: Account[]; version: number; onChanged: () => void }) {
  const [type, setType] = useState("coa");
  const [source, setSource] = useState("zoho");
  const [mode, setMode] = useState("open_items");
  const [asOf, setAsOf] = useState(todayISO());
  const [csv, setCsv] = useState("");
  const [fileName, setFileName] = useState("");
  const [report, setReport] = useState<ImportReport | null>(null);
  const [cols, setCols] = useState<Record<string, { field: string; columns: string[] }[]>>({});
  const [ofxAcct, setOfxAcct] = useState("");
  const [ofxMsg, setOfxMsg] = useState("");
  const { err, run } = useErr();
  useEffect(() => {
    importColumns().then(setCols).catch(() => undefined);
  }, [version]);
  const go = async (m: "preview" | "run") => {
    const r = await run(() => runImport(m, { type, source, csv, mode, as_of: asOf }));
    if (r) {
      setReport(r);
      if (m === "run") onChanged();
    }
  };
  return (
    <Box>
      <Typography level="h3" sx={{ mb: 0.5 }}>
        Import data
      </Typography>
      <Typography level="body-sm" sx={{ mb: 1.5 }}>
        Move your books from Zoho Books, QuickBooks or Tally. Pick what to import, choose the file, and review the preview. Nothing is written until you press Import, and running the same file again does not duplicate anything. Suggested order: chart of accounts, opening balances, customers and vendors, then open invoices and bills.
      </Typography>
      <Sheet variant="outlined" sx={{ p: 1.5, borderRadius: "md", mb: 2, display: "grid", gap: 1.5, maxWidth: 900 }}>
        <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap" }}>
          <FormControl>
            <FormLabel>What</FormLabel>
            <Select size="sm" value={type} onChange={(_, v) => { setType(String(v)); setReport(null); }} sx={{ minWidth: 240 }} slotProps={{ button: { "data-testid": "acct-import-type" } as never }}>
              {TYPES.map((t) => (
                <Option key={t.id} value={t.id}>
                  {t.label}
                </Option>
              ))}
            </Select>
          </FormControl>
          <FormControl>
            <FormLabel>From</FormLabel>
            <Select size="sm" value={source} onChange={(_, v) => setSource(String(v))} sx={{ minWidth: 190 }} slotProps={{ button: { "data-testid": "acct-import-source" } as never }}>
              {SOURCES.map((t) => (
                <Option key={t.id} value={t.id}>
                  {t.label}
                </Option>
              ))}
            </Select>
          </FormControl>
          {type === "opening_balances" && (
            <FormControl>
              <FormLabel>Balances as of</FormLabel>
              <Input size="sm" type="date" value={asOf} onChange={(e) => setAsOf(e.target.value)} slotProps={{ input: { "data-testid": "acct-import-asof" } }} />
            </FormControl>
          )}
          {(type === "invoices" || type === "bills") && (
            <FormControl>
              <FormLabel>How</FormLabel>
              <Select size="sm" value={mode} onChange={(_, v) => setMode(String(v))} sx={{ minWidth: 260 }}>
                <Option value="open_items">Open items only (with opening balances)</Option>
                <Option value="history">Full history (sales, GST and payments)</Option>
              </Select>
            </FormControl>
          )}
        </Box>
        <Typography level="body-xs">{HELP[source]}</Typography>
        <Box sx={{ display: "flex", gap: 1, alignItems: "center", flexWrap: "wrap" }}>
          <Button size="sm" variant="soft" component="label">
            Choose CSV file
            <input
              type="file"
              accept=".csv,text/csv,text/plain"
              hidden
              data-testid="acct-import-file"
              onChange={async (e) => {
                const f = e.target.files?.[0];
                if (f) {
                  setCsv(await f.text());
                  setFileName(f.name);
                  setReport(null);
                }
              }}
            />
          </Button>
          <Typography level="body-sm">{fileName || "No file chosen"}</Typography>
          <Button size="sm" disabled={!csv} onClick={() => go("preview")} data-testid="acct-import-preview">
            Preview
          </Button>
          <Button size="sm" color="success" disabled={!csv || !report || !report.dry_run || report.ok === 0} onClick={() => go("run")} data-testid="acct-import-run">
            Import {report && report.dry_run ? `${report.ok} rows` : ""}
          </Button>
        </Box>
        <details>
          <summary>Recognised column names</summary>
          <Table size="sm">
            <tbody>
              {(cols[type] ?? []).map((c) => (
                <tr key={c.field}>
                  <td style={{ width: 120 }}>
                    <b>{c.field}</b>
                  </td>
                  <td>{c.columns.join(", ")}</td>
                </tr>
              ))}
            </tbody>
          </Table>
        </details>
      </Sheet>
      <ErrorAlert err={err} />
      {report && (
        <Sheet variant="outlined" sx={{ p: 1.5, borderRadius: "md", mb: 2 }} data-testid="acct-import-report">
          <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", mb: 1, alignItems: "center" }}>
            <Typography level="title-md">{report.dry_run ? "Preview" : "Imported"}</Typography>
            <Chip color="success" variant="soft" data-testid="acct-import-ok">{report.dry_run ? `${report.ok} ready` : `${report.applied} applied`}</Chip>
            <Chip variant="soft" data-testid="acct-import-existing">{report.existing} already there or skipped</Chip>
            <Chip color={report.errors ? "danger" : "neutral"} variant="soft" data-testid="acct-import-errors">{report.errors} errors</Chip>
          </Box>
          {Object.keys(report.mapping).length > 0 && (
            <Typography level="body-xs" sx={{ mb: 1 }}>
              Columns used: {Object.entries(report.mapping).map(([f, c]) => `${f} ← “${c}”`).join(", ")}
            </Typography>
          )}
          {report.notes.map((n, i) => (
            <Typography key={i} level="body-sm" sx={{ mb: 0.5 }}>
              {n}
            </Typography>
          ))}
          <Box sx={{ maxHeight: 360, overflow: "auto" }}>
            <Table size="sm">
              <thead>
                <tr>
                  <th style={{ width: 60 }}>Line</th>
                  <th style={{ width: 90 }}>Result</th>
                  <th>Record</th>
                  <th>Detail</th>
                </tr>
              </thead>
              <tbody>
                {report.rows.map((r, i) => (
                  <tr key={i}>
                    <td>{r.line}</td>
                    <td>
                      <Chip size="sm" variant="soft" color={r.status === "ok" ? "success" : r.status === "error" ? "danger" : "neutral"}>
                        {r.status}
                      </Chip>
                    </td>
                    <td>{r.summary}</td>
                    <td>{r.message}</td>
                  </tr>
                ))}
              </tbody>
            </Table>
          </Box>
        </Sheet>
      )}
      <Sheet variant="outlined" sx={{ p: 1.5, borderRadius: "md", maxWidth: 900 }}>
        <Typography level="title-md" sx={{ mb: 0.5 }}>
          Bank statement (OFX / QFX)
        </Typography>
        <Typography level="body-sm" sx={{ mb: 1 }}>
          Import a statement downloaded from your bank into a bank account. Lines already imported are skipped. They appear under Banking for review.
        </Typography>
        <Box sx={{ display: "flex", gap: 1, alignItems: "center", flexWrap: "wrap" }}>
          <AccountSelect accounts={accounts} value={ofxAcct} onChange={setOfxAcct} filter={(a) => isBank(a) && a.subtype === "bank"} placeholder="Bank account" testid="acct-ofx-account" />
          <Button size="sm" variant="soft" component="label" disabled={!ofxAcct}>
            Choose OFX / QFX file
            <input
              type="file"
              accept=".ofx,.qfx,text/plain"
              hidden
              data-testid="acct-ofx-file"
              onChange={async (e) => {
                const f = e.target.files?.[0];
                e.target.value = "";
                if (!f) return;
                const r = await run(async () => importOfx(ofxAcct, await f.text()));
                if (r) {
                  setOfxMsg(`${r.imported} lines imported, ${r.duplicates} duplicates skipped.`);
                  onChanged();
                }
              }}
            />
          </Button>
          {ofxMsg && (
            <Typography level="body-sm" color="success" data-testid="acct-ofx-msg">
              {ofxMsg}
            </Typography>
          )}
        </Box>
      </Sheet>
    </Box>
  );
}

export function CloseView({ settings, version, onChanged, canApprove }: { settings: Settings; version: number; onChanged: () => void; canApprove: boolean }) {
  const [date, setDate] = useState(() => {
    const t = new Date();
    const last = new Date(t.getFullYear(), t.getMonth(), 0); // end of last month
    return `${last.getFullYear()}-${String(last.getMonth() + 1).padStart(2, "0")}-${String(last.getDate()).padStart(2, "0")}`;
  });
  const [c, setC] = useState<Checklist | null>(null);
  const [ack, setAck] = useState(false);
  const [msg, setMsg] = useState("");
  const { err, run } = useErr();
  const load = useCallback(async () => {
    const r = await run(() => closeChecklist(date));
    if (r) setC(r);
  }, [date, run]);
  useEffect(() => {
    load();
  }, [load, version]);
  const warns = c?.items.filter((i) => i.status === "warn" && i.id !== "already").length ?? 0;
  return (
    <Box>
      <Typography level="h3" sx={{ mb: 0.5 }}>
        Close the books
      </Typography>
      <Typography level="body-sm" sx={{ mb: 1.5 }}>
        Closing locks every entry dated on or before the date: nothing can be added, changed or deleted there, including by sync, recurring transactions and imports. {settings.closing_date ? `Books are closed through ${settings.closing_date}.` : "Books are open."} To reopen, clear the closing date in Settings.
      </Typography>
      <Box sx={{ display: "flex", gap: 1, alignItems: "center", mb: 1.5 }}>
        <Input size="sm" type="date" value={date} onChange={(e) => setDate(e.target.value)} slotProps={{ input: { "data-testid": "acct-close-date" } }} />
        <Button size="sm" variant="soft" onClick={load}>
          Re-check
        </Button>
      </Box>
      <ErrorAlert err={err} />
      {c && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", mb: 1.5, maxWidth: 760 }} data-testid="acct-close-checklist">
          <Table size="sm">
            <tbody>
              {c.items.map((i) => (
                <tr key={i.id}>
                  <td style={{ width: 90 }}>
                    <Chip size="sm" variant="soft" color={i.status === "ok" ? "success" : i.status === "warn" ? "warning" : "danger"}>
                      {i.status === "ok" ? "ok" : i.status === "warn" ? "review" : "fix"}
                    </Chip>
                  </td>
                  <td>
                    <b>{i.label}</b>
                    <div style={{ fontSize: 12, opacity: 0.8 }}>{i.status === "ok" ? "" : i.detail}</div>
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
      {c && warns > 0 && <Checkbox sx={{ mb: 1 }} label={`I have reviewed the ${warns} warnings`} checked={ack} onChange={(e) => setAck(e.target.checked)} data-testid="acct-close-ack" />}
      {c && !canApprove && <Typography level="body-sm">Closing a period needs the accounting approver role.</Typography>}
      <Box>
        <Button
          disabled={!c || !c.can_close || !canApprove || (warns > 0 && !ack)}
          data-testid="acct-close-run"
          onClick={async () => {
            const r = await run(() => closePeriod(date, ack));
            if (r) {
              setMsg(`Books closed through ${r.closed_through}.`);
              onChanged();
              load();
            }
          }}
        >
          Close the books through {date}
        </Button>
      </Box>
      {msg && (
        <Typography level="body-sm" color="success" sx={{ mt: 1 }} data-testid="acct-close-msg">
          {msg}
        </Typography>
      )}
      <Typography level="title-sm" sx={{ mt: 3 }}>
        Trial balance at {date}
      </Typography>
      <Box sx={{ mt: 1 }}>
        <GenericReport name="trial-balance" q={{ as_of: date }} version={version} />
      </Box>
    </Box>
  );
}
