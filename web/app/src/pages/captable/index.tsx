import { PersonSelect } from "../../components/PeoplePicker";
import { useCallback, useEffect, useMemo, useState } from "react";
import { Alert, Box, Button, Card, Checkbox, Chip, CircularProgress, Container, Input, Option, Select, Tab, TabList, Tabs, Table, Typography } from "@mui/joy";
import DonutLargeOutlined from "@mui/icons-material/DonutLargeOutlined";
import { Header } from "../../components/Header";
import { BizAssistant } from "../../components/BizAssistant";
import type { User } from "../../api/types";
import { errText, fmtDate } from "../know/http";
import { Dialog } from "../legal/Dialogs";
import { captable, type AccessEntry, type Class, type Dashboard, type EventRec, type ExitOutcome, type Pool, type Round, type RoundModel, type Scenario, type Security, type Stakeholder, type Statement, type Table as Tbl } from "./api";
import { Donut, PayoutBars, VestingCurve } from "./charts";
import { ActionForm, HolderForm, IssueForm } from "./Forms";
import { fmtMoney, fmtPct, fmtPrice, fmtShares, KIND_LABEL, PALETTE, parseExit, parseShares, SEC_LABEL, slices } from "./logic";
import { CAPTABLE_HINT, CAPTABLE_SUGGESTIONS, createCapTableTools } from "./ai/tools";

type TabId = "dashboard" | "table" | "holders" | "securities" | "rounds" | "exit" | "setup" | "access";
const ACCENT = "#126C5A";
const num = { textAlign: "right" } as const;

function Stat({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return <Card variant="outlined" size="sm" sx={{ minWidth: 150, flex: 1 }}><Typography level="body-xs">{label}</Typography><Typography level="h3">{value}</Typography>{sub && <Typography level="body-xs">{sub}</Typography>}</Card>;
}

export default function CapTableApp({ user }: { user: User }) {
  const [level, setLevel] = useState<string | null>(null);
  const [admin, setAdmin] = useState(false);
  const [tab, setTab] = useState<TabId>("dashboard");
  const [err, setErr] = useState("");
  const [today, setToday] = useState("");
  const [dash, setDash] = useState<Dashboard | null>(null);
  const [holders, setHolders] = useState<Stakeholder[]>([]);
  const [classes, setClasses] = useState<Class[]>([]);
  const [pools, setPools] = useState<Pool[]>([]);
  const [secs, setSecs] = useState<Security[]>([]);
  const [rounds, setRounds] = useState<Round[]>([]);
  const [events, setEvents] = useState<EventRec[]>([]);
  const tools = useMemo(() => createCapTableTools(), []);
  const canEdit = level === "edit";

  const load = useCallback(async () => {
    try {
      const w = await captable.whoami();
      setLevel(w.level); setAdmin(w.admin);
      if (!w.level) return;
      const m = await captable.meta();
      setToday(m.today);
      const [d, h, c, p, s, r, e] = await Promise.all([captable.dashboard(), captable.stakeholders(), captable.classes(), captable.pools(), captable.securities(), captable.rounds(), captable.events()]);
      setDash(d); setHolders(h); setClasses(c); setPools(p); setSecs(s); setRounds(r); setEvents(e); setErr("");
    } catch (e) { setErr(errText(e)); }
  }, []);
  useEffect(() => { void load(); }, [load]);

  const TABS: [TabId, string][] = [["dashboard", "Dashboard"], ["table", "Cap table"], ["holders", "Holders"], ["securities", "Securities"], ["rounds", "Rounds"], ["exit", "Exit scenarios"], ["setup", "Classes & pools"], ...(admin ? ([["access", "Access"]] as [TabId, string][]) : [])];

  return (
    <>
      <Header user={user} />
      <Container maxWidth="xl" sx={{ py: { xs: 2, sm: 3 }, px: { xs: 1.5, sm: 3 } }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, mb: 2, flexWrap: "wrap" }}>
          <DonutLargeOutlined sx={{ fontSize: 28, color: ACCENT }} />
          <Typography level="h2" sx={{ fontSize: { xs: "xl", sm: "xl3" } }}>Cap table{dash?.company.name ? ` · ${dash.company.name}` : ""}</Typography>
          {level === "view" && <Chip size="sm" variant="outlined">View only</Chip>}
          {level && <Box sx={{ ml: "auto" }}><BizAssistant app="captable" title="Cap table" tools={tools} suggestions={CAPTABLE_SUGGESTIONS} systemHint={CAPTABLE_HINT} /></Box>}
        </Box>
        {level === null && !err && <Box sx={{ p: 3 }}><CircularProgress /></Box>}
        {err && <Alert color="danger" sx={{ mb: 1 }}>{err}</Alert>}
        {level === "" && (
          <Alert color="warning" data-testid="restricted">The cap table is restricted to named people. Ask an organization admin to add you under Cap table › Access.</Alert>
        )}
        {level && dash && (
          <>
            <Tabs value={tab} onChange={(_, v) => v && setTab(v as TabId)} sx={{ bgcolor: "transparent" }}>
              <TabList sx={{ overflowX: "auto", flexWrap: "nowrap" }}>{TABS.map(([k, l]) => <Tab key={k} value={k} sx={{ flexShrink: 0 }}>{l}</Tab>)}</TabList>
            </Tabs>
            {tab === "dashboard" && <DashboardPane d={dash} />}
            {tab === "table" && <TablePane today={today} />}
            {tab === "holders" && <HoldersPane holders={holders} canEdit={canEdit} onChanged={load} />}
            {tab === "securities" && <SecuritiesPane secs={secs} events={events} holders={holders} classes={classes} pools={pools} today={today} canEdit={canEdit} onChanged={load} />}
            {tab === "rounds" && <RoundsPane rounds={rounds} holders={holders} classes={classes} canEdit={canEdit} onChanged={load} />}
            {tab === "exit" && <ExitPane />}
            {tab === "setup" && <SetupPane dash={dash} classes={classes} pools={pools} canEdit={canEdit} onChanged={load} />}
            {tab === "access" && admin && <AccessPane />}
          </>
        )}
      </Container>
    </>
  );
}

function DashboardPane({ d }: { d: Dashboard }) {
  const t = d.table;
  const byHolder = slices([...t.rows.map((r) => ({ label: r.name, value: r.fully_diluted })), { label: "Unallocated option pool", value: t.pool_unallocated }]);
  const byClass = slices([...t.classes.map((c) => ({ label: c.name, value: c.shares })), { label: "Options & warrants", value: t.rows.reduce((s, r) => s + r.options + r.warrants, 0) }, { label: "Unallocated option pool", value: t.pool_unallocated }]);
  return (
    <Box sx={{ mt: 2 }}>
      <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap", mb: 2 }}>
        <Stat label="Shares outstanding" value={fmtShares(t.outstanding)} sub={`${d.holders} holders`} />
        <Stat label="Fully diluted" value={fmtShares(t.fully_diluted)} sub={`incl. ${fmtShares(t.pool_unallocated)} unallocated`} />
        <Stat label="Capital raised" value={fmtMoney(d.capital_raised, d.company.currency)} sub={d.last_round ? `last: ${d.last_round.name}` : "no rounds yet"} />
        <Stat label="Implied valuation" value={d.implied_valuation ? fmtMoney(d.implied_valuation, d.company.currency) : "—"} sub={d.last_round ? `at ${fmtPrice(d.last_round.price_per_share)} a share` : undefined} />
        <Stat label="Open SAFEs & notes" value={fmtMoney(d.convertible_outstanding, d.company.currency)} />
      </Box>
      <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", lg: "1fr 1fr" }, gap: 2 }}>
        <Card variant="outlined"><Typography level="title-md" sx={{ mb: 1 }}>Ownership, fully diluted</Typography><Donut items={byHolder} centre={`${fmtShares(t.fully_diluted)}`} /></Card>
        <Card variant="outlined"><Typography level="title-md" sx={{ mb: 1 }}>By class</Typography><Donut items={byClass} /></Card>
        <Card variant="outlined" sx={{ gridColumn: { lg: "1 / -1" } }}>
          <Typography level="title-md" sx={{ mb: 0.5 }}>Coming up in the next 6 months</Typography>
          {d.upcoming.length === 0 ? <Typography level="body-sm">No cliffs, full-vest dates or expiries.</Typography> : d.upcoming.map((u, i) => (
            <Box key={i} sx={{ display: "flex", gap: 1.5, py: 0.5, borderBottom: "1px solid", borderColor: "divider", flexWrap: "wrap" }}>
              <Typography level="body-sm" sx={{ width: 100 }}>{fmtDate(u.date)}</Typography><Chip size="sm" variant="soft">{u.kind}</Chip><Typography level="body-sm" sx={{ flex: 1 }}>{u.stakeholder} · {u.label}</Typography>{u.shares ? <Typography level="body-sm">{fmtShares(u.shares)} shares</Typography> : null}
            </Box>
          ))}
        </Card>
      </Box>
    </Box>
  );
}

function TablePane({ today }: { today: string }) {
  const [asOf, setAsOf] = useState(today);
  const [t, setT] = useState<Tbl | null>(null);
  const [pct, setPct] = useState<"fd" | "out">("fd");
  const [err, setErr] = useState("");
  useEffect(() => { captable.table(asOf).then((x) => { setT(x); setErr(""); }).catch((e) => setErr(errText(e))); }, [asOf]);
  return (
    <Box sx={{ mt: 2 }}>
      <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap", alignItems: "center", mb: 1.5 }}>
        <Typography level="body-sm">As of</Typography><Input type="date" size="sm" value={asOf} onChange={(e) => setAsOf(e.target.value)} data-testid="asof" />
        <Select size="sm" value={pct} onChange={(_, v) => v && setPct(v)} sx={{ minWidth: 190 }}><Option value="fd">Percent: fully diluted</Option><Option value="out">Percent: outstanding</Option></Select>
        <Box sx={{ ml: "auto", display: "flex", gap: 1, flexWrap: "wrap" }}>
          <Button size="sm" variant="outlined" component="a" href={captable.exportUrl("captable", asOf)}>Cap table CSV</Button>
          <Button size="sm" variant="outlined" component="a" href={captable.exportUrl("ledger", asOf)}>Ledger CSV</Button>
          <Button size="sm" variant="outlined" component="a" href={captable.exportUrl("vesting", asOf)}>Vesting CSV</Button>
          <Button size="sm" variant="outlined" component="a" href={captable.exportUrl("transactions", asOf)}>Transactions CSV</Button>
        </Box>
      </Box>
      {err && <Alert color="danger">{err}</Alert>}
      {t && (
        <Box sx={{ overflowX: "auto" }}>
          <Table size="sm" stickyHeader aria-label="Cap table" sx={{ minWidth: 720 }}>
            <thead><tr><th>Holder</th><th>Type</th><th style={num}>Shares</th><th style={num}>Options</th><th style={num}>Warrants</th><th style={num}>Fully diluted</th><th style={num}>%</th></tr></thead>
            <tbody>
              {t.rows.map((r) => (
                <tr key={r.id}><td>{r.name}</td><td>{KIND_LABEL[r.kind] ?? r.kind}</td><td style={num}>{fmtShares(r.stock)}</td><td style={num}>{fmtShares(r.options)}</td><td style={num}>{fmtShares(r.warrants)}</td><td style={num}>{fmtShares(r.fully_diluted)}</td><td style={num}>{fmtPct(pct === "fd" ? r.pct_fully_diluted : r.pct_outstanding, 4)}</td></tr>
              ))}
              {pct === "fd" && <tr><td>Unallocated option pool</td><td>Pool</td><td /><td /><td /><td style={num}>{fmtShares(t.pool_unallocated)}</td><td style={num}>{fmtPct(t.pool_pct_fully_diluted, 4)}</td></tr>}
              <tr style={{ fontWeight: 700 }}><td>Total</td><td /><td style={num}>{fmtShares(t.outstanding)}</td><td /><td /><td style={num}>{fmtShares(t.fully_diluted)}</td><td style={num}>100.0000%</td></tr>
            </tbody>
          </Table>
          <Typography level="body-xs" sx={{ mt: 1 }}>Percentages add up to exactly 100% (largest remainder). Open SAFEs and notes ({fmtMoney(t.convertible_cents)}) are not counted until they convert. By class: {t.classes.map((c) => `${c.name} ${fmtShares(c.shares)}`).join(" · ")}</Typography>
          {t.pools.map((p) => <Typography key={p.id} level="body-xs">{p.name}: {fmtShares(p.size)} reserved · {fmtShares(p.granted)} granted · {fmtShares(p.unallocated)} left</Typography>)}
        </Box>
      )}
    </Box>
  );
}

function HoldersPane({ holders, canEdit, onChanged }: { holders: Stakeholder[]; canEdit: boolean; onChanged: () => void }) {
  const [form, setForm] = useState<null | { h?: Stakeholder }>(null);
  const [st, setSt] = useState<Statement | null>(null);
  const [err, setErr] = useState("");
  return (
    <Box sx={{ mt: 2 }}>
      {canEdit && <Button size="sm" sx={{ mb: 1.5 }} onClick={() => setForm({})} data-testid="new-holder">＋ New holder</Button>}
      {err && <Alert color="danger" sx={{ mb: 1 }}>{err}</Alert>}
      <Card variant="outlined" sx={{ p: 0 }}>
        {holders.length === 0 ? <Typography level="body-sm" sx={{ p: 2 }}>No holders yet.</Typography> : holders.map((h) => (
          <Box key={h.id} role="button" tabIndex={0} onClick={() => captable.statement(h.id).then(setSt).catch((e) => setErr(errText(e)))} sx={{ display: "flex", gap: 1.5, px: 1.5, py: 1, borderBottom: "1px solid", borderColor: "divider", alignItems: "center", cursor: "pointer", "&:hover": { bgcolor: "background.level1" } }}>
            <Box sx={{ flex: 1 }}><Typography level="title-sm">{h.name}</Typography><Typography level="body-xs">{h.email || "No email"}</Typography></Box>
            <Chip size="sm" variant="soft">{KIND_LABEL[h.kind] ?? h.kind}</Chip><Typography level="body-xs" sx={{ width: 90, textAlign: "right" }}>{h.securities} records</Typography>
          </Box>
        ))}
      </Card>
      {form && <HolderForm initial={form.h} onClose={() => setForm(null)} onSaved={() => { setForm(null); onChanged(); }} />}
      {st && (
        <Dialog title={`Equity statement: ${st.holder.name}`} onClose={() => setSt(null)} width={820}>
          <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap", mb: 1.5 }}>
            <Stat label="Shares" value={fmtShares(st.stock)} /><Stat label="Options" value={fmtShares(st.options)} sub={`${fmtShares(st.vested_options)} exercisable`} /><Stat label="Ownership, fully diluted" value={fmtPct(st.pct_fully_diluted, 4)} sub={`outstanding ${fmtPct(st.pct_outstanding, 4)}`} />
          </Box>
          {st.securities.map((s) => (
            <Card key={s.id} variant="outlined" size="sm" sx={{ mb: 1 }}>
              <Typography level="title-sm">{s.label} · {SEC_LABEL[s.kind]} {s.class ? `· ${s.class}` : ""}</Typography>
              <Typography level="body-xs">Issued {fmtDate(s.issue_date)} · {s.kind === "safe" || s.kind === "note" ? `${fmtMoney(s.principal)}${s.cap ? ` at a ${fmtMoney(s.cap)} cap` : ""}${s.converted ? " · converted" : ""}` : `${fmtShares(s.quantity)} × ${fmtPrice(s.price)} · ${fmtShares(s.outstanding)} held · ${fmtShares(s.vested)} vested`}</Typography>
              {s.schedule && s.schedule.length > 1 && <VestingCurve points={s.schedule} total={s.quantity} today={new Date().toISOString().slice(0, 10)} />}
            </Card>
          ))}
          <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1 }}>
            {canEdit && <Button variant="outlined" onClick={() => { const h = st.holder; setSt(null); setForm({ h }); }}>Edit holder</Button>}
            {canEdit && st.securities.length === 0 && <Button color="danger" variant="outlined" onClick={() => captable.deleteStakeholder(st.holder.id).then(() => { setSt(null); onChanged(); }).catch((e) => setErr(errText(e)))}>Delete</Button>}
            <Button onClick={() => window.print()}>Print statement</Button>
          </Box>
        </Dialog>
      )}
    </Box>
  );
}

function SecuritiesPane({ secs, events, holders, classes, pools, today, canEdit, onChanged }: { secs: Security[]; events: EventRec[]; holders: Stakeholder[]; classes: Class[]; pools: Pool[]; today: string; canEdit: boolean; onChanged: () => void }) {
  const [kind, setKind] = useState("");
  const [issue, setIssue] = useState(false);
  const [act, setAct] = useState<null | { s: Security; k: "exercise" | "cancel" | "transfer" | "convert" }>(null);
  const [err, setErr] = useState("");
  const list = secs.filter((s) => !kind || s.kind === kind);
  return (
    <Box sx={{ mt: 2 }}>
      <Box sx={{ display: "flex", gap: 1, mb: 1.5, flexWrap: "wrap" }}>
        {canEdit && <Button size="sm" onClick={() => setIssue(true)} data-testid="issue-equity">＋ Issue equity</Button>}
        <Select size="sm" value={kind} onChange={(_, v) => setKind(v ?? "")} placeholder="All kinds" sx={{ minWidth: 150 }}><Option value="">All kinds</Option>{Object.entries(SEC_LABEL).map(([k, l]) => <Option key={k} value={k}>{l}</Option>)}</Select>
      </Box>
      {err && <Alert color="danger" sx={{ mb: 1 }}>{err}</Alert>}
      <Box sx={{ overflowX: "auto" }}>
        <Table size="sm" aria-label="Securities" sx={{ minWidth: 860 }}>
          <thead><tr><th>Label</th><th>Holder</th><th>Kind</th><th>Issued</th><th style={num}>Quantity</th><th style={num}>Held</th><th style={num}>Vested</th><th style={num}>Price</th><th /></tr></thead>
          <tbody>
            {list.map((s) => {
              const conv = s.kind === "safe" || s.kind === "note";
              return (
                <tr key={s.id}>
                  <td>{s.label}</td><td>{s.stakeholder}</td><td>{SEC_LABEL[s.kind]}{s.class ? ` · ${s.class}` : ""}</td><td>{fmtDate(s.issue_date)}</td>
                  <td style={num}>{conv ? fmtMoney(s.principal) : fmtShares(s.quantity)}</td><td style={num}>{conv ? (s.converted ? "converted" : "open") : fmtShares(s.outstanding)}</td><td style={num}>{conv ? "" : fmtShares(s.vested)}</td><td style={num}>{conv ? (s.cap ? `cap ${fmtMoney(s.cap)}` : "") : fmtPrice(s.price)}</td>
                  <td>{canEdit && (
                    <Box sx={{ display: "flex", gap: 0.5 }}>
                      {(s.kind === "option" || s.kind === "warrant") && s.outstanding > 0 && <><Button size="sm" variant="plain" onClick={() => setAct({ s, k: "exercise" })}>Exercise</Button><Button size="sm" variant="plain" onClick={() => setAct({ s, k: "cancel" })}>Cancel</Button></>}
                      {s.kind === "stock" && s.outstanding > 0 && <Button size="sm" variant="plain" onClick={() => setAct({ s, k: "transfer" })}>Transfer</Button>}
                      {conv && !s.converted && <Button size="sm" variant="plain" onClick={() => setAct({ s, k: "convert" })}>Convert</Button>}
                      {!s.parent_id && !s.round_id && !s.converted && s.exercised + s.cancelled + s.transferred === 0 && <Button size="sm" variant="plain" color="danger" onClick={() => captable.deleteSecurity(s.id).then(onChanged).catch((e) => setErr(errText(e)))}>Delete</Button>}
                    </Box>
                  )}</td>
                </tr>
              );
            })}
          </tbody>
        </Table>
      </Box>
      <Typography level="title-md" sx={{ mt: 3, mb: 0.5 }}>Exercises, cancellations, transfers and conversions</Typography>
      {events.length === 0 ? <Typography level="body-sm">None recorded.</Typography> : events.map((e) => (
        <Box key={e.id} sx={{ display: "flex", gap: 1.5, py: 0.6, borderBottom: "1px solid", borderColor: "divider", alignItems: "center", flexWrap: "wrap" }}>
          <Typography level="body-sm" sx={{ width: 100 }}>{fmtDate(e.date)}</Typography><Chip size="sm" variant="soft">{e.kind}</Chip>
          <Typography level="body-sm" sx={{ flex: 1 }}>{e.label} · {fmtShares(e.quantity)} shares{e.to_name ? ` to ${e.to_name}` : ""}{e.note ? ` · ${e.note}` : ""}</Typography>
          {canEdit && <Button size="sm" variant="plain" color="danger" onClick={() => captable.undo(e.id).then(onChanged).catch((x) => setErr(errText(x)))}>Undo</Button>}
        </Box>
      ))}
      {issue && <IssueForm holders={holders} classes={classes} pools={pools} today={today} onClose={() => setIssue(false)} onSaved={() => { setIssue(false); onChanged(); }} />}
      {act && <ActionForm sec={act.s} kind={act.k} holders={holders} classes={classes} today={today} onClose={() => setAct(null)} onSaved={() => { setAct(null); onChanged(); }} />}
    </Box>
  );
}

function RoundsPane({ rounds, holders, classes, canEdit, onChanged }: { rounds: Round[]; holders: Stakeholder[]; classes: Class[]; canEdit: boolean; onChanged: () => void }) {
  const [pre, setPre] = useState("");
  const [inv, setInv] = useState("");
  const [pool, setPool] = useState("10");
  const [name, setName] = useState("Series A");
  const [cls, setCls] = useState(classes.find((c) => c.kind === "preferred")?.id ?? "");
  const [investors, setInvestors] = useState<{ id: string; amount: string }[]>([{ id: "", amount: "" }]);
  const [m, setM] = useState<RoundModel | null>(null);
  const [err, setErr] = useState("");
  const [msg, setMsg] = useState("");
  const input = () => {
    const p = parseExit(pre), i = parseExit(inv);
    if (p === null || p <= 0 || i === null) { setErr("Enter the pre-money and the investment, for example 40 crore and 5 crore"); return null; }
    const ivs = investors.filter((x) => x.id || x.amount).map((x) => ({ stakeholder_id: x.id, amount: parseExit(x.amount) ?? -1 }));
    if (ivs.some((x) => x.amount < 0)) { setErr("Each investor's amount must be readable, for example 3 crore"); return null; }
    return { name, pre_money: p, investment: i, pool_target_bp: Math.round(Number(pool || 0) * 100), investors: ivs, class_id: cls || undefined };
  };
  const model = async () => { const b = input(); if (!b) return; try { setM(await captable.modelRound(b)); setErr(""); setMsg(""); } catch (e) { setErr(errText(e)); } };
  const close = async () => { const b = input(); if (!b) return; try { await captable.closeRound({ ...b, date: new Date().toISOString().slice(0, 10) }); setMsg("Round closed: shares issued, SAFEs and notes converted, pool topped up."); setM(null); onChanged(); } catch (e) { setErr(errText(e)); } };
  return (
    <Box sx={{ mt: 2 }}>
      <Typography level="title-md" sx={{ mb: 0.5 }}>Rounds on record</Typography>
      <Card variant="outlined" sx={{ p: 0, mb: 3 }}>
        {rounds.length === 0 ? <Typography level="body-sm" sx={{ p: 2 }}>No rounds yet.</Typography> : rounds.map((r) => (
          <Box key={r.id} sx={{ display: "flex", gap: 1.5, px: 1.5, py: 1, borderBottom: "1px solid", borderColor: "divider", flexWrap: "wrap", alignItems: "center" }}>
            <Box sx={{ flex: 1, minWidth: 160 }}><Typography level="title-sm">{r.name}</Typography><Typography level="body-xs">{fmtDate(r.date)} · {r.class || "—"}</Typography></Box>
            <Typography level="body-sm">Raised {fmtMoney(r.investment)}</Typography><Typography level="body-sm">Post-money {fmtMoney(r.post_money)}</Typography><Typography level="body-sm">{r.price_per_share ? fmtPrice(r.price_per_share) : ""}</Typography>
          </Box>
        ))}
      </Card>
      <Typography level="title-md" sx={{ mb: 0.5 }}>Round modeller</Typography>
      <Typography level="body-sm" sx={{ mb: 1 }}>Model a priced round against today's cap table. SAFEs and notes convert at the better of their cap and discount; the pool top-up happens before the round, so it dilutes existing holders and not the new investors.</Typography>
      <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr 1fr", md: "repeat(5, 1fr)" }, gap: 1.5, mb: 1.5 }}>
        <Box><Typography level="body-xs">Round name</Typography><Input value={name} onChange={(e) => setName(e.target.value)} /></Box>
        <Box><Typography level="body-xs">Pre-money (₹)</Typography><Input value={pre} onChange={(e) => setPre(e.target.value)} placeholder="40 crore" data-testid="pre-money" /></Box>
        <Box><Typography level="body-xs">New investment (₹)</Typography><Input value={inv} onChange={(e) => setInv(e.target.value)} placeholder="5 crore" data-testid="investment" /></Box>
        <Box><Typography level="body-xs">Pool after round (%)</Typography><Input value={pool} onChange={(e) => setPool(e.target.value)} /></Box>
        <Box><Typography level="body-xs">Class for new shares</Typography><Select value={cls} placeholder="Choose" onChange={(_, v) => setCls(v ?? "")} slotProps={{ listbox: { sx: { zIndex: 2000 } } }}>{classes.map((c) => <Option key={c.id} value={c.id}>{c.name}</Option>)}</Select></Box>
      </Box>
      <Typography level="body-xs" sx={{ mb: 0.5 }}>Investors (needed to close the round; optional for modelling)</Typography>
      {investors.map((iv, i) => (
        <Box key={i} sx={{ display: "flex", gap: 1, mb: 0.75, flexWrap: "wrap" }}>
          <Select sx={{ minWidth: 200, flex: 1 }} value={iv.id} placeholder="Investor" onChange={(_, v) => setInvestors(investors.map((x, j) => (j === i ? { ...x, id: v ?? "" } : x)))} slotProps={{ listbox: { sx: { zIndex: 2000 } } }}>{holders.map((h) => <Option key={h.id} value={h.id}>{h.name}</Option>)}</Select>
          <Input sx={{ width: 160 }} placeholder="Amount, e.g. 3 crore" value={iv.amount} onChange={(e) => setInvestors(investors.map((x, j) => (j === i ? { ...x, amount: e.target.value } : x)))} />
        </Box>
      ))}
      <Box sx={{ display: "flex", gap: 1, mb: 2, flexWrap: "wrap" }}>
        <Button size="sm" variant="plain" onClick={() => setInvestors([...investors, { id: "", amount: "" }])}>+ Investor</Button>
        <Button onClick={() => void model()} data-testid="model-round">Model the round</Button>
        {canEdit && m && <Button color="warning" onClick={() => void close()} disabled={!cls || !m.investors?.length} data-testid="close-round">Close this round (writes to the cap table)</Button>}
        {m && canEdit && <Button variant="outlined" onClick={() => captable.saveScenario("round", `${name}: ${pre} pre`, { pre, inv, pool, name }).then(() => setMsg("Scenario saved.")).catch((e) => setErr(errText(e)))}>Save scenario</Button>}
      </Box>
      {err && <Alert color="danger" sx={{ mb: 1 }}>{err}</Alert>}
      {msg && <Alert color="success" sx={{ mb: 1 }}>{msg}</Alert>}
      {m && (
        <Box>
          <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap", mb: 1.5 }}>
            <Stat label="Price per share" value={fmtPrice((Number(m.price_per_share_cents) / 100).toFixed(6).replace(/0+$/, ""))} />
            <Stat label="New investor shares" value={fmtShares(m.new_investor_shares)} sub={fmtPct(m.new_investor_pct, 4)} />
            <Stat label="Pool top-up" value={fmtShares(m.pool_top_up)} />
            <Stat label="Post-round fully diluted" value={fmtShares(m.post_fully_diluted)} sub={`post-money ${fmtMoney(m.post_money_cents)}`} />
          </Box>
          <Box sx={{ overflowX: "auto" }}>
            <Table size="sm" aria-label="Dilution"><thead><tr><th>Holder</th><th style={num}>Before</th><th style={num}>%</th><th style={num}>After</th><th style={num}>%</th></tr></thead>
              <tbody>{m.holders.map((h) => <tr key={h.id + h.name}><td>{h.name}</td><td style={num}>{fmtShares(h.before)}</td><td style={num}>{h.before ? fmtPct(h.pct_before, 4) : ""}</td><td style={num}>{fmtShares(h.after)}</td><td style={num}>{fmtPct(h.pct_after, 4)}</td></tr>)}</tbody></Table>
          </Box>
          {(m.converted ?? []).map((c) => <Typography key={c.id} level="body-sm" sx={{ mt: 0.5 }}>{c.name}: {fmtMoney(c.value_cents)} converts into {fmtShares(c.shares)} shares at {(Number(c.price_cents) / 100).toFixed(4)} rupees{c.cap_binds ? " (the cap sets the price)" : ""}, {fmtPct(c.ownership_pct, 4)} after the round.</Typography>)}
          {(m.investors ?? []).map((v) => <Typography key={v.stakeholder_id + v.amount} level="body-sm">{v.name}: {fmtMoney(v.amount)} buys {fmtShares(v.shares)} shares ({v.pct}%).</Typography>)}
          {m.unallocated_shares > 0 && <Typography level="body-xs">{m.unallocated_shares} shares are left over from whole-share rounding.</Typography>}
        </Box>
      )}
    </Box>
  );
}

function ExitPane() {
  const [vals, setVals] = useState("50 crore, 100 crore, 200 crore, 500 crore");
  const [unvested, setUnvested] = useState("participate");
  const [out, setOut] = useState<ExitOutcome[] | null>(null);
  const [saved, setSaved] = useState<Scenario[]>([]);
  const [err, setErr] = useState("");
  const loadSaved = useCallback(() => captable.scenarios("exit").then(setSaved).catch(() => undefined), []);
  useEffect(() => { void loadSaved(); }, [loadSaved]);
  const go = async (text = vals, uv = unvested) => {
    const exits = text.split(",").map((s) => parseExit(s.trim()));
    if (exits.some((x) => x === null) || exits.length === 0) return setErr("Enter exit values separated by commas, for example 50 crore, 100 crore");
    try { setOut(await captable.modelExit(exits as number[], uv)); setErr(""); } catch (e) { setErr(errText(e)); }
  };
  const names = out ? Array.from(new Set(out.flatMap((o) => o.holders.map((h) => h.name)))) : [];
  const color = (n: string) => PALETTE[names.indexOf(n) % PALETTE.length];
  return (
    <Box sx={{ mt: 2 }}>
      <Typography level="body-sm" sx={{ mb: 1 }}>See what every holder receives if the company is sold. Notes are repaid first, then SAFE money back, then preferences by seniority, then the rest per share; options pay their strike. SAFEs and notes are treated as debt here (convert them in a round first).</Typography>
      <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", mb: 1.5 }}>
        <Input sx={{ flex: 1, minWidth: 260 }} value={vals} onChange={(e) => setVals(e.target.value)} data-testid="exit-values" aria-label="Exit values" />
        <Select value={unvested} onChange={(_, v) => v && setUnvested(v)} sx={{ minWidth: 230 }}><Option value="participate">Unvested options take part</Option><Option value="forfeit">Unvested options are forfeited</Option></Select>
        <Button onClick={() => void go()} data-testid="run-exit">Run</Button>
        {out && <Button variant="outlined" onClick={() => captable.saveScenario("exit", `Exit at ${vals}`, { vals, unvested }).then(loadSaved)}>Save scenario</Button>}
      </Box>
      {saved.length > 0 && <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", mb: 1.5 }}>{saved.map((s) => <Chip key={s.id} variant="outlined" onClick={() => { const p = s.params as { vals?: string; unvested?: string }; if (p.vals) { setVals(p.vals); setUnvested(p.unvested ?? "participate"); void go(p.vals, p.unvested ?? "participate"); } }} endDecorator={<span role="button" aria-label="Delete scenario" onClick={(e) => { e.stopPropagation(); void captable.deleteScenario(s.id).then(loadSaved); }}>✕</span>}>{s.name}</Chip>)}</Box>}
      {err && <Alert color="danger">{err}</Alert>}
      {out && (
        <>
          <Box sx={{ overflowX: "auto" }}>
            <Table size="sm" aria-label="Exit payouts" sx={{ minWidth: 600 }}>
              <thead><tr><th>Holder</th>{out.map((o) => <th key={o.exit} style={num}>{fmtMoney(o.exit).replace(/\.00$/, "")}</th>)}</tr></thead>
              <tbody>
                {names.map((n) => <tr key={n}><td><Box component="span" sx={{ display: "inline-block", width: 8, height: 8, borderRadius: 2, bgcolor: color(n), mr: 1 }} />{n}</td>{out.map((o) => { const h = o.holders.find((x) => x.name === n); return <td key={o.exit} style={num}>{h ? `${fmtMoney(h.payout)} (${Number(h.pct).toFixed(1)}%)` : "—"}</td>; })}</tr>)}
                <tr style={{ fontWeight: 700 }}><td>Total</td>{out.map((o) => <td key={o.exit} style={num}>{fmtMoney(o.total)}</td>)}</tr>
              </tbody>
            </Table>
          </Box>
          <Box sx={{ mt: 2 }}><PayoutBars colorOf={color} rows={out.map((o) => ({ label: fmtMoney(o.exit).replace(/\.00$/, ""), parts: o.holders.map((h) => ({ name: h.name, value: h.payout })) }))} /></Box>
          {out.map((o) => o.notes?.map((n) => <Alert key={n} color="warning" sx={{ mt: 1 }}>{n}</Alert>))}
          <Typography level="body-xs" sx={{ mt: 1 }}>{out.map((o) => `${fmtMoney(o.exit).replace(/\.00$/, "")}: ${o.payees.filter((p) => p.converted).map((p) => `${p.name} converts`).join(", ") || "no preferred class converts"}`).join(" · ")}</Typography>
        </>
      )}
    </Box>
  );
}

function SetupPane({ dash, classes, pools, canEdit, onChanged }: { dash: Dashboard; classes: Class[]; pools: Pool[]; canEdit: boolean; onChanged: () => void }) {
  const [co, setCo] = useState(dash.company);
  const [cf, setCf] = useState({ name: "", kind: "common", seniority: "0", pref: "1", ratio: "1", participating: false, price: "", auth: "" });
  const [pf, setPf] = useState({ name: "", size: "", adopted: new Date().toISOString().slice(0, 10) });
  const [err, setErr] = useState("");
  const wrap = async (f: () => Promise<unknown>) => { try { await f(); setErr(""); onChanged(); } catch (e) { setErr(errText(e)); } };
  return (
    <Box sx={{ mt: 2, display: "grid", gap: 3, maxWidth: 900 }}>
      {err && <Alert color="danger">{err}</Alert>}
      <Box>
        <Typography level="title-md" sx={{ mb: 0.5 }}>Company</Typography>
        <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap" }}>
          <Input sx={{ flex: 1, minWidth: 200 }} value={co.name} onChange={(e) => setCo({ ...co, name: e.target.value })} placeholder="Company name" disabled={!canEdit} />
          <Input sx={{ width: 90 }} value={co.currency} onChange={(e) => setCo({ ...co, currency: e.target.value })} disabled={!canEdit} />
          <Input type="date" value={co.incorporated} onChange={(e) => setCo({ ...co, incorporated: e.target.value })} disabled={!canEdit} />
          {canEdit && <Button onClick={() => void wrap(() => captable.saveCompany(co))}>Save</Button>}
        </Box>
      </Box>
      <Box>
        <Typography level="title-md" sx={{ mb: 0.5 }}>Share classes</Typography>
        {classes.map((c) => (
          <Box key={c.id} sx={{ display: "flex", gap: 1.5, py: 0.7, borderBottom: "1px solid", borderColor: "divider", alignItems: "center", flexWrap: "wrap" }}>
            <Typography level="title-sm" sx={{ flex: 1, minWidth: 160 }}>{c.name}</Typography><Chip size="sm" variant="soft">{c.kind}</Chip>
            <Typography level="body-xs">{fmtShares(c.issued)} issued{c.authorized ? ` of ${fmtShares(c.authorized)}` : ""}{c.kind === "preferred" ? ` · ${(c.pref_multiple_milli / 1000).toFixed(1)}x ${c.participating ? "participating" : "non-participating"} · seniority ${c.seniority}` : ""}</Typography>
            {canEdit && c.issued === 0 && <Button size="sm" variant="plain" color="danger" onClick={() => void wrap(() => captable.deleteClass(c.id))}>Delete</Button>}
          </Box>
        ))}
        {canEdit && (
          <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", mt: 1 }}>
            <Input size="sm" placeholder="Class name" value={cf.name} onChange={(e) => setCf({ ...cf, name: e.target.value })} data-testid="class-name" />
            <Select size="sm" value={cf.kind} onChange={(_, v) => v && setCf({ ...cf, kind: v })}><Option value="common">Common</Option><Option value="preferred">Preferred</Option></Select>
            {cf.kind === "preferred" && <><Input size="sm" sx={{ width: 110 }} placeholder="Pref. multiple" value={cf.pref} onChange={(e) => setCf({ ...cf, pref: e.target.value })} /><Input size="sm" sx={{ width: 100 }} placeholder="Seniority" value={cf.seniority} onChange={(e) => setCf({ ...cf, seniority: e.target.value })} /><Input size="sm" sx={{ width: 110 }} placeholder="Conv. ratio" value={cf.ratio} onChange={(e) => setCf({ ...cf, ratio: e.target.value })} /><Checkbox label="Participating" checked={cf.participating} onChange={(e) => setCf({ ...cf, participating: e.target.checked })} /></>}
            <Input size="sm" sx={{ width: 130 }} placeholder="Authorised shares" value={cf.auth} onChange={(e) => setCf({ ...cf, auth: e.target.value })} />
            <Button size="sm" onClick={() => void wrap(() => captable.createClass({ name: cf.name, kind: cf.kind as "common", seniority: Number(cf.seniority) || 0, pref_multiple_milli: Math.round(Number(cf.pref || 1) * 1000), convert_ratio_milli: Math.round(Number(cf.ratio || 1) * 1000), participating: cf.participating, authorized: parseShares(cf.auth) ?? 0, issue_price: cf.price }))}>Add class</Button>
          </Box>
        )}
      </Box>
      <Box>
        <Typography level="title-md" sx={{ mb: 0.5 }}>Option pools</Typography>
        {pools.map((p) => (
          <Box key={p.id} sx={{ display: "flex", gap: 1.5, py: 0.7, borderBottom: "1px solid", borderColor: "divider", alignItems: "center", flexWrap: "wrap" }}>
            <Typography level="title-sm" sx={{ flex: 1 }}>{p.name}</Typography><Typography level="body-xs">{fmtShares(p.size)} shares · adopted {fmtDate(p.adopted)}</Typography>
          </Box>
        ))}
        {canEdit && (
          <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", mt: 1 }}>
            <Input size="sm" placeholder="Pool name" value={pf.name} onChange={(e) => setPf({ ...pf, name: e.target.value })} /><Input size="sm" sx={{ width: 130 }} placeholder="Shares" value={pf.size} onChange={(e) => setPf({ ...pf, size: e.target.value })} /><Input size="sm" type="date" value={pf.adopted} onChange={(e) => setPf({ ...pf, adopted: e.target.value })} />
            <Button size="sm" onClick={() => void wrap(() => captable.createPool({ name: pf.name, size: parseShares(pf.size) ?? -1, adopted: pf.adopted }))}>Add pool</Button>
          </Box>
        )}
      </Box>
      <Typography level="body-xs">All share counts are whole numbers and every percentage is computed exactly on the server.</Typography>
    </Box>
  );
}

function AccessPane() {
  const [list, setList] = useState<AccessEntry[]>([]);
  const [people, setPeople] = useState<{ id: string; name: string; email: string }[]>([]);
  const [pick, setPick] = useState("");
  const [level, setLevel] = useState("view");
  const [err, setErr] = useState("");
  const load = useCallback(() => { captable.access().then(setList).catch((e) => setErr(errText(e))); }, []);
  useEffect(() => { load(); captable.people().then(setPeople).catch(() => undefined); }, [load]);
  return (
    <Box sx={{ mt: 2, maxWidth: 700 }}>
      <Typography level="body-sm" sx={{ mb: 1 }}>Organization admins always have full access. Everyone else sees nothing unless named here. Give “view” to finance or legal reviewers and “edit” only to the people who keep the records.</Typography>
      {err && <Alert color="danger" sx={{ mb: 1 }}>{err}</Alert>}
      {list.length === 0 ? <Typography level="body-sm" sx={{ mb: 1 }}>Nobody besides admins has access.</Typography> : list.map((a) => (
        <Box key={a.user_id} sx={{ display: "flex", gap: 1.5, py: 0.7, borderBottom: "1px solid", borderColor: "divider", alignItems: "center" }}>
          <Typography level="body-sm" sx={{ flex: 1 }}>{a.name} <Typography level="body-xs" component="span">{a.email}</Typography></Typography><Chip size="sm" variant="soft">{a.level}</Chip>
          <Button size="sm" variant="plain" color="danger" onClick={() => captable.revoke(a.user_id).then(load)}>Remove</Button>
        </Box>
      ))}
      <Box sx={{ display: "flex", gap: 1, mt: 1.5, flexWrap: "wrap" }}>
        <PersonSelect value={pick} known={people} onChange={(id) => setPick(id)} placeholder="Choose a person" ariaLabel="Person" />
        <Select value={level} onChange={(_, v) => v && setLevel(v)}><Option value="view">View</Option><Option value="edit">Edit</Option></Select>
        <Button disabled={!pick} onClick={() => captable.grant(pick, level).then(() => { setPick(""); load(); }).catch((e) => setErr(errText(e)))}>Give access</Button>
      </Box>
    </Box>
  );
}
