import { usePermissions } from "../../lib/permissions";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Alert, Box, Button, Chip, CircularProgress, Container, Input, List, ListItem, ListItemButton, ListItemContent, ListItemDecorator, Sheet, Table, Typography } from "@mui/joy";
import AccountBalanceIcon from "@mui/icons-material/AccountBalance";
import HomeOutlined from "@mui/icons-material/HomeOutlined";
import SwapHorizOutlined from "@mui/icons-material/SwapHorizOutlined";
import ReceiptOutlined from "@mui/icons-material/ReceiptOutlined";
import AccountBalanceWalletOutlined from "@mui/icons-material/AccountBalanceWalletOutlined";
import ListAltOutlined from "@mui/icons-material/ListAltOutlined";
import BarChartIcon from "@mui/icons-material/BarChart";
import SettingsOutlined from "@mui/icons-material/SettingsOutlined";
import SyncIcon from "@mui/icons-material/Sync";
import RequestQuoteOutlined from "@mui/icons-material/RequestQuoteOutlined";
import GavelOutlined from "@mui/icons-material/GavelOutlined";
import PrecisionManufacturingOutlined from "@mui/icons-material/PrecisionManufacturingOutlined";
import CurrencyExchangeOutlined from "@mui/icons-material/CurrencyExchangeOutlined";
import EventRepeatOutlined from "@mui/icons-material/EventRepeatOutlined";
import SavingsOutlined from "@mui/icons-material/SavingsOutlined";
import UploadFileOutlined from "@mui/icons-material/UploadFileOutlined";
import LockClockOutlined from "@mui/icons-material/LockClockOutlined";
import { Header } from "../../components/Header";
import { BizAssistant } from "../../components/BizAssistant";
import { onAiChanged, withAccess } from "../../lib/ai/biz";
import { ACCT_HINT, ACCT_SUGGESTIONS, createAccountingTools } from "./ai/tools";
import type { User } from "../../api/types";
import { getCapabilities, getSettings, getSummary, listAccounts, search, syncApps } from "./api";
import { AccountsView } from "./AccountsView";
import { BankingView } from "./BankingView";
import { ExpensesView } from "./ExpensesView";
import { ReportsView } from "./ReportsView";
import { SettingsView } from "./SettingsView";
import { BillsView } from "./BillsView";
import { AssetsView, CurrenciesView, RecurringView } from "./CurrencyRecurringViews";
import { TaxView } from "./TaxView";
import { BudgetsView, CloseView, ImportView } from "./ToolsViews";
import { TransactionsView } from "./TransactionsView";
import { ACCENT, money } from "./logic";
import type { Account, Expense, Journal, Settings, Summary, SyncResult } from "./types";
import { ErrorAlert, Stat, rightCell } from "./ui";

type View = "home" | "transactions" | "expenses" | "bills" | "banking" | "accounts" | "reports" | "taxes" | "assets" | "currencies" | "recurring" | "budgets" | "import" | "close" | "search" | "settings";

const NAV: { id: View; label: string; icon: JSX.Element }[] = [
  { id: "home", label: "Home", icon: <HomeOutlined /> },
  { id: "transactions", label: "Transactions", icon: <SwapHorizOutlined /> },
  { id: "expenses", label: "Expenses", icon: <ReceiptOutlined /> },
  { id: "bills", label: "Bills and credits", icon: <RequestQuoteOutlined /> },
  { id: "banking", label: "Banking", icon: <AccountBalanceWalletOutlined /> },
  { id: "accounts", label: "Chart of accounts", icon: <ListAltOutlined /> },
  { id: "reports", label: "Reports", icon: <BarChartIcon /> },
  { id: "taxes", label: "Taxes (GST, TDS)", icon: <GavelOutlined /> },
  { id: "assets", label: "Fixed assets", icon: <PrecisionManufacturingOutlined /> },
  { id: "currencies", label: "Currencies", icon: <CurrencyExchangeOutlined /> },
  { id: "recurring", label: "Recurring", icon: <EventRepeatOutlined /> },
  { id: "budgets", label: "Budgets", icon: <SavingsOutlined /> },
  { id: "import", label: "Import data", icon: <UploadFileOutlined /> },
  { id: "close", label: "Close the books", icon: <LockClockOutlined /> },
  { id: "settings", label: "Settings", icon: <SettingsOutlined /> },
];

function syncText(r: Record<string, SyncResult>): string {
  const t = (k: string) => r[k] ?? ({ created: 0, updated: 0, voided: 0, locked: 0, foreign: 0, warnings: [] } as unknown as SyncResult);
  const parts = ["pos", "invoicing"].map((k) => {
    const x = t(k);
    return `${k === "pos" ? "POS" : "Invoicing"}: ${x.created} new, ${x.updated} updated, ${x.voided} voided`;
  });
  const locked = t("pos").locked + t("invoicing").locked;
  const foreign = t("invoicing").foreign;
  return parts.join(". ") + "." + (locked ? ` ${locked} skipped in the closed period.` : "") + (foreign ? ` ${foreign} foreign-currency documents skipped: add an exchange rate for their date under Currencies.` : "");
}

export default function AccountingApp({ user }: { user: User }) {
  const { can, loaded } = usePermissions();
  if (loaded && !can("accounting:ledger:read")) return <NoAccess user={user} />;
  return <AccountingAppInner user={user} />;
}

function NoAccess({ user }: { user: User }) {
  return (
    <>
      <Header user={user} />
      <Container maxWidth="sm" sx={{ py: 8 }}>
        <Alert color="warning" data-testid="accounting-no-access">
          You do not have access to Accounting. Ask an admin for the Accounting app admin role.
        </Alert>
      </Container>
    </>
  );
}

function AccountingAppInner({ user }: { user: User }) {
  const [view, setView] = useState<View>("home");
  const [settings, setSettings] = useState<Settings | null>(null);
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [summary, setSummary] = useState<Summary | null>(null);
  const [version, setVersion] = useState(0);
  const [error, setError] = useState("");
  const [syncing, setSyncing] = useState(false);
  const [syncNote, setSyncNote] = useState("");
  const [openJournal, setOpenJournal] = useState("");
  const [q, setQ] = useState("");
  const [results, setResults] = useState<{ journals: Journal[]; accounts: Account[]; expenses: Expense[] } | null>(null);
  const synced = useRef(false);
  const { can } = usePermissions();
  const canApprove = can("accounting:bills:approve");
  const openJ = (id: string) => {
    setOpenJournal(id);
    setView("transactions");
  };

  const reload = useCallback(async () => {
    try {
      const [s, a, sm] = await Promise.all([getSettings(), listAccounts(), getSummary()]);
      setSettings(s);
      setAccounts(a);
      setSummary(sm);
    } catch (e) {
      setError((e as Error).message);
    }
  }, []);
  useEffect(() => {
    reload();
  }, [reload, version]);

  const changed = useCallback(() => setVersion((v) => v + 1), []);
  const canLedger = can("accounting:ledger:read"), canPost = can("accounting:ledger:write");
  const aiTools = useMemo(() => withAccess(createAccountingTools(), canPost), [canPost]);
  useEffect(() => onAiChanged("accounting", changed), [changed]);

  const sync = useCallback(
    async (full = false) => {
      setSyncing(true);
      setError("");
      try {
        const r = await syncApps(full);
        setSyncNote(syncText(r));
        const warn = Object.values(r).flatMap((x) => x.warnings ?? []);
        if (warn.length) setError(warn.join("; "));
        setVersion((v) => v + 1);
      } catch (e) {
        setError((e as Error).message);
      } finally {
        setSyncing(false);
      }
    },
    [],
  );
  // Post anything new from POS and Invoicing once per visit.
  useEffect(() => {
    if (synced.current) return;
    synced.current = true;
    getCapabilities()
      .then(() => sync(false))
      .catch(() => undefined);
  }, [sync]);

  const go = (v: View) => {
    setOpenJournal("");
    setView(v);
  };
  const doSearch = async () => {
    if (!q.trim()) return;
    try {
      setResults(await search(q.trim()));
      setView("search");
    } catch (e) {
      setError((e as Error).message);
    }
  };

  return (
    <>
      <Header user={user} />
      <Container maxWidth="xl" sx={{ py: { xs: 2, sm: 3 }, px: { xs: 1.5, sm: 3 } }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, mb: 2, flexWrap: "wrap" }}>
          <AccountBalanceIcon sx={{ fontSize: 28, color: ACCENT }} />
          <Typography level="h2" sx={{ fontSize: { xs: "xl", sm: "xl3" }, mr: "auto" }}>
            Accounting
          </Typography>
          {canLedger && <BizAssistant app="accounting" title="Accounting" tools={aiTools} suggestions={ACCT_SUGGESTIONS} systemHint={ACCT_HINT} />}
          <Input
            size="sm"
            placeholder="Search transactions, accounts, expenses"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && doSearch()}
            slotProps={{ input: { "data-testid": "acct-search" } }}
            sx={{ width: { xs: "100%", sm: 300 } }}
          />
          <Button size="sm" variant="soft" startDecorator={syncing ? <CircularProgress size="sm" /> : <SyncIcon />} onClick={() => sync(false)} disabled={syncing} data-testid="acct-sync">
            Sync from apps
          </Button>
        </Box>
        <Box sx={{ display: "flex", flexDirection: { xs: "column", md: "row" }, gap: { xs: 1, md: 3 } }}>
          <Box component="nav" aria-label="Accounting sections" sx={{ width: { md: 210 }, flexShrink: 0 }}>
            <List orientation="horizontal" sx={{ flexWrap: "wrap", "@media (min-width: 900px)": { flexDirection: "column", flexWrap: "nowrap" }, gap: 0.5 }}>
              {NAV.map((n) => (
                <ListItem key={n.id} sx={{ width: { md: "100%" } }}>
                  <ListItemButton selected={view === n.id} onClick={() => go(n.id)} aria-current={view === n.id ? "page" : undefined} data-testid={`acct-nav-${n.id}`} sx={{ borderRadius: "md" }}>
                    <ListItemDecorator>{n.icon}</ListItemDecorator>
                    <ListItemContent>{n.label}</ListItemContent>
                  </ListItemButton>
                </ListItem>
              ))}
            </List>
          </Box>
          <Box sx={{ flex: 1, minWidth: 0 }}>
            <ErrorAlert err={error} />
            {syncNote && view === "home" && (
              <Alert color="success" variant="soft" sx={{ mb: 1 }} data-testid="acct-sync-note">
                {syncNote}
              </Alert>
            )}
            {!settings && !error && <CircularProgress />}
            {settings && view === "home" && <Home summary={summary} onGo={go} onFull={() => sync(true)} />}
            {settings && view === "transactions" && <TransactionsView accounts={accounts} version={version} onChanged={changed} openJournalId={openJournal} />}
            {settings && view === "expenses" && <ExpensesView accounts={accounts} version={version} onChanged={changed} />}
            {settings && view === "bills" && <BillsView accounts={accounts} settings={settings} version={version} onChanged={changed} canApprove={canApprove} />}
            {settings && view === "taxes" && <TaxView accounts={accounts} settings={settings} version={version} onChanged={changed} onOpenJournal={openJ} />}
            {settings && view === "assets" && <AssetsView accounts={accounts} version={version} onChanged={changed} />}
            {settings && view === "currencies" && <CurrenciesView settings={settings} version={version} onChanged={changed} />}
            {settings && view === "recurring" && <RecurringView accounts={accounts} version={version} onChanged={changed} />}
            {settings && view === "budgets" && <BudgetsView accounts={accounts} settings={settings} version={version} onChanged={changed} />}
            {settings && view === "import" && <ImportView accounts={accounts} version={version} onChanged={changed} />}
            {settings && view === "close" && <CloseView settings={settings} version={version} onChanged={changed} canApprove={canApprove} />}
            {settings && view === "banking" && <BankingView accounts={accounts} version={version} onChanged={changed} />}
            {settings && view === "accounts" && (
              <AccountsView
                accounts={accounts}
                onChanged={changed}
                onOpenJournal={(id) => {
                  setOpenJournal(id);
                  setView("transactions");
                }}
              />
            )}
            {settings && view === "reports" && <ReportsView settings={settings} version={version} onOpenJournal={openJ} />}
            {settings && view === "settings" && <SettingsView settings={settings} onSaved={changed} />}
            {view === "search" && results && (
              <SearchResults
                results={results}
                onOpen={(id) => {
                  setOpenJournal(id);
                  setView("transactions");
                }}
                onExpense={() => go("expenses")}
                onAccounts={() => go("accounts")}
              />
            )}
          </Box>
        </Box>
      </Container>
    </>
  );
}

function Home({ summary, onGo, onFull }: { summary: Summary | null; onGo: (v: View) => void; onFull: () => void }) {
  if (!summary) return <CircularProgress />;
  return (
    <Box>
      <Typography level="h3" sx={{ mb: 1.5 }}>
        Overview
      </Typography>
      <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap", mb: 2 }}>
        <Stat label="Cash and bank" value={money(summary.cash)} testid="acct-stat-cash" />
        <Stat label="Owed to you (receivable)" value={money(summary.receivables)} color={ACCENT} testid="acct-stat-ar" />
        <Stat label="You owe (payable)" value={money(summary.payables)} testid="acct-stat-ap" />
        <Stat label="GST payable (net)" value={money(summary.gst_payable)} testid="acct-stat-gst" />
      </Box>
      <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap", mb: 2 }}>
        <Stat label="Income this month" value={money(summary.month_income)} testid="acct-stat-income" />
        <Stat label="Expenses this month" value={money(summary.month_expense)} testid="acct-stat-expense" />
        <Stat label="Profit this financial year" value={money(summary.year_profit)} color={summary.year_profit < 0 ? "var(--joy-palette-danger-600)" : ACCENT} testid="acct-stat-profit" />
      </Box>
      <Sheet variant="outlined" sx={{ p: 2, borderRadius: "md", maxWidth: 640 }}>
        <Typography level="title-md" sx={{ mb: 1 }}>
          Books status
        </Typography>
        <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", alignItems: "center" }}>
          <Chip color={summary.balanced ? "success" : "danger"} variant="soft" data-testid="acct-books-balanced">
            {summary.balanced ? "Debits equal credits" : "Books out of balance"}
          </Chip>
          <Chip variant="soft">{summary.journals} journal entries</Chip>
          {summary.pending_bank_lines > 0 && (
            <Chip color="warning" variant="soft" onClick={() => onGo("banking")} sx={{ cursor: "pointer" }}>
              {summary.pending_bank_lines} bank lines to review
            </Chip>
          )}
        </Box>
        <Typography level="body-sm" sx={{ mt: 1.5 }}>
          Sales from POS and invoices and payments from Invoicing are posted automatically when you open this page or press Sync from apps. Nothing is posted twice.
        </Typography>
        <Button size="sm" variant="plain" sx={{ mt: 1 }} onClick={onFull}>
          Re-read all history
        </Button>
      </Sheet>
    </Box>
  );
}

function SearchResults({
  results,
  onOpen,
  onExpense,
  onAccounts,
}: {
  results: { journals: Journal[]; accounts: Account[]; expenses: Expense[] };
  onOpen: (id: string) => void;
  onExpense: () => void;
  onAccounts: () => void;
}) {
  const none = !results.journals.length && !results.accounts.length && !results.expenses.length;
  return (
    <Box data-testid="acct-search-results">
      <Typography level="h3" sx={{ mb: 1 }}>
        Search results
      </Typography>
      {none && <Typography>Nothing found.</Typography>}
      {results.journals.length > 0 && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", mb: 2, overflow: "auto" }}>
          <Table size="sm" hoverRow>
            <thead>
              <tr>
                <th colSpan={4}>Transactions</th>
              </tr>
            </thead>
            <tbody>
              {results.journals.map((j) => (
                <tr key={j.id} onClick={() => onOpen(j.id)} style={{ cursor: "pointer" }}>
                  <td>#{j.number}</td>
                  <td>{j.date}</td>
                  <td>{j.memo}</td>
                  <td style={rightCell}>{money(j.total)}</td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
      {results.accounts.length > 0 && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", mb: 2, overflow: "auto" }}>
          <Table size="sm" hoverRow>
            <thead>
              <tr>
                <th colSpan={3}>Accounts</th>
              </tr>
            </thead>
            <tbody>
              {results.accounts.map((a) => (
                <tr key={a.id} onClick={onAccounts} style={{ cursor: "pointer" }}>
                  <td>{a.code}</td>
                  <td>{a.name}</td>
                  <td style={rightCell}>{money(a.balance)}</td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
      {results.expenses.length > 0 && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table size="sm" hoverRow>
            <thead>
              <tr>
                <th colSpan={4}>Expenses</th>
              </tr>
            </thead>
            <tbody>
              {results.expenses.map((e) => (
                <tr key={e.id} onClick={onExpense} style={{ cursor: "pointer" }}>
                  <td>{e.date}</td>
                  <td>{e.vendor_name}</td>
                  <td>{e.category_name}</td>
                  <td style={rightCell}>{money(e.total)}</td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
    </Box>
  );
}
