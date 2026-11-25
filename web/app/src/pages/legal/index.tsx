import { useCallback, useEffect, useMemo, useState } from "react";
import { Alert, Box, Button, Card, Chip, CircularProgress, Container, Input, Option, Select, Tab, TabList, Tabs, Textarea, Typography } from "@mui/joy";
import BalanceOutlined from "@mui/icons-material/BalanceOutlined";
import { Header } from "../../components/Header";
import { BizAssistant } from "../../components/BizAssistant";
import type { User } from "../../api/types";
import { deepParam } from "../../lib/deepLink";
import { errText, fmtDate } from "../know/http";
import { legal, type Bill, type Client, type ConflictResult, type Deadline, type Entry, type Matter, type Meta, type Overview, type UserHours } from "./api";
import { ClientForm, MatterForm } from "./Dialogs";
import { MatterDetail } from "./MatterDetail";
import { BILL_COLOR, csv, fmtHM, fmtMoney, KIND_LABEL, relDays, STATUS_COLOR, STATUS_LABEL, URGENCY_COLOR } from "./logic";
import { createLegalTools, LEGAL_HINT, LEGAL_SUGGESTIONS } from "./ai/tools";

const ACCENT = "#126C5A";
type TabId = "dashboard" | "matters" | "clients" | "deadlines" | "time" | "bills" | "conflicts" | "settings";
const TABS: [TabId, string][] = [["dashboard", "Dashboard"], ["matters", "Matters"], ["clients", "Clients"], ["deadlines", "Deadlines"], ["time", "Time & expenses"], ["bills", "Bills"], ["conflicts", "Conflict check"], ["settings", "Settings"]];

function Stat({ label, value, sub, tone }: { label: string; value: string | number; sub?: string; tone?: "danger" | "warning" }) {
  return (
    <Card variant="outlined" size="sm" sx={{ minWidth: 150, flex: 1 }}>
      <Typography level="body-xs">{label}</Typography>
      <Typography level="h3" color={tone}>{value}</Typography>
      {sub && <Typography level="body-xs">{sub}</Typography>}
    </Card>
  );
}

const rowSx = { display: "flex", gap: 1.5, alignItems: "center", px: 1.5, py: 1, borderBottom: "1px solid", borderColor: "divider", flexWrap: "wrap", "&:hover": { bgcolor: "background.level1" } } as const;

export default function LegalApp({ user }: { user: User }) {
  const [tab, setTab] = useState<TabId>("dashboard");
  const [meta, setMeta] = useState<Meta | null>(null);
  const [ov, setOv] = useState<Overview | null>(null);
  const [matters, setMatters] = useState<Matter[]>([]);
  const [clients, setClients] = useState<Client[]>([]);
  const [people, setPeople] = useState<{ id: string; name: string }[]>([]);
  const [deadlines, setDeadlines] = useState<Deadline[]>([]);
  const [entries, setEntries] = useState<Entry[]>([]);
  const [bills, setBills] = useState<Bill[]>([]);
  const [hours, setHours] = useState<UserHours[]>([]);
  const [q, setQ] = useState("");
  const [status, setStatus] = useState("");
  const [openId, setOpenId] = useState(deepParam("matter"));
  const [form, setForm] = useState<null | "matter" | "client">(null);
  const [editClient, setEditClient] = useState<Client | undefined>();
  const [err, setErr] = useState("");
  const tools = useMemo(() => createLegalTools(), []);

  const load = useCallback(async () => {
    try {
      const m = await legal.meta();
      setMeta(m);
      const [o, ms, cs, pp, dl, en, bl] = await Promise.all([legal.overview(), legal.matters({ q, status }), legal.clients("", true), legal.people(), legal.deadlines(), legal.entries({}), legal.bills()]);
      setOv(o); setMatters(ms); setClients(cs); setPeople(pp.people); setDeadlines(dl); setEntries(en); setBills(bl);
      if (m.admin) legal.hours().then(setHours).catch(() => setHours([]));
      setErr("");
    } catch (e) { setErr(errText(e)); }
  }, [q, status]);
  useEffect(() => { const t = setTimeout(() => void load(), q ? 250 : 0); return () => clearTimeout(t); }, [load, q]);

  const mrow = (m: Matter) => (
    <Box key={m.id} role="button" tabIndex={0} data-testid="matter-row" onClick={() => setOpenId(m.id)} onKeyDown={(e) => e.key === "Enter" && setOpenId(m.id)} sx={{ ...rowSx, cursor: "pointer" }}>
      <Typography level="body-sm" sx={{ width: 110, fontFamily: "monospace" }}>{m.number}</Typography>
      <Box sx={{ flex: 1, minWidth: 200 }}>
        <Typography level="title-sm">{m.title}</Typography>
        <Typography level="body-xs">{m.client_name || "No client"} · {m.practice_area || "—"}{m.responsible ? ` · ${m.responsible}` : ""}</Typography>
      </Box>
      <Chip size="sm" variant="soft" color={STATUS_COLOR[m.status]}>{STATUS_LABEL[m.status]}</Chip>
      <Typography level="body-sm" sx={{ width: 120, textAlign: "right" }} title="Unbilled work">{m.unbilled ? fmtMoney(m.unbilled) : "—"}</Typography>
    </Box>
  );
  const drow = (d: Deadline) => (
    <Box key={d.id} sx={{ ...rowSx, cursor: "pointer" }} onClick={() => setOpenId(d.matter_id)}>
      <Chip size="sm" variant="soft" color={URGENCY_COLOR[d.urgency]} sx={{ width: 96, justifyContent: "center" }}>{relDays(d.days_left)}</Chip>
      <Box sx={{ flex: 1, minWidth: 180 }}><Typography level="title-sm">{d.title}</Typography><Typography level="body-xs">{KIND_LABEL[d.kind]} · {fmtDate(d.due_date)} · {d.matter_number} {d.matter_title}</Typography></Box>
      <Typography level="body-xs">{d.assignee}</Typography>
    </Box>
  );

  const exportEntries = () => {
    const rows = [["Date", "Matter", "Kind", "Person", "Time", "Rate", "Amount", "Billable", "Billed", "Description"], ...entries.map((e) => [e.date, e.matter_number ?? "", e.kind, e.user, e.kind === "time" ? fmtHM(e.minutes) : "", e.kind === "time" ? fmtMoney(e.rate) : "", fmtMoney(e.amount), e.billable ? "yes" : "no", e.bill_id ? "yes" : "no", e.description])];
    const a = document.createElement("a");
    a.href = URL.createObjectURL(new Blob([csv(rows)], { type: "text/csv" })); a.download = "legal-time-and-expenses.csv"; a.click(); URL.revokeObjectURL(a.href);
  };

  return (
    <>
      <Header user={user} />
      <Container maxWidth="xl" sx={{ py: { xs: 2, sm: 3 }, px: { xs: 1.5, sm: 3 } }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, mb: 2, flexWrap: "wrap" }}>
          <BalanceOutlined sx={{ fontSize: 28, color: ACCENT }} />
          <Typography level="h2" sx={{ fontSize: { xs: "xl", sm: "xl3" } }}>Legal</Typography>
          <Box sx={{ ml: "auto", display: "flex", gap: 1 }}>
            <Button size="sm" onClick={() => setForm("matter")} disabled={!meta} data-testid="new-matter">＋ New matter</Button>
            <BizAssistant app="legal" title="Legal" tools={tools} suggestions={LEGAL_SUGGESTIONS} systemHint={LEGAL_HINT} />
          </Box>
        </Box>
        <Tabs value={tab} onChange={(_, v) => v && setTab(v as TabId)} sx={{ bgcolor: "transparent" }}>
          <TabList sx={{ overflowX: "auto", flexWrap: "nowrap" }}>{TABS.map(([k, l]) => <Tab key={k} value={k} sx={{ flexShrink: 0 }}>{l}</Tab>)}</TabList>
        </Tabs>
        {err && <Alert color="danger" sx={{ my: 1 }}>{err}</Alert>}
        {!meta && !err && <Box sx={{ p: 3 }}><CircularProgress /></Box>}

        {meta && ov && tab === "dashboard" && (
          <Box sx={{ mt: 2 }}>
            <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap", mb: 2 }}>
              <Stat label="Open matters" value={ov.open_matters} sub={`${ov.by_status.intake ?? 0} in intake`} />
              <Stat label="Unbilled work" value={fmtMoney(ov.unbilled)} sub={`${fmtHM(ov.unbilled_minutes)} of time`} />
              <Stat label="Billed, last 30 days" value={fmtMoney(ov.billed_30_days)} />
              <Stat label="Hours this month" value={fmtHM(ov.minutes_this_month)} />
              <Stat label="Overdue deadlines" value={ov.overdue_deadlines} tone={ov.overdue_deadlines ? "danger" : undefined} sub={`${ov.deadlines_next_7_days} due within 7 days`} />
            </Box>
            <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", lg: "1fr 1fr" }, gap: 2 }}>
              <Box>
                <Typography level="title-md" sx={{ mb: 0.5 }}>Upcoming deadlines</Typography>
                <Card variant="outlined" sx={{ p: 0 }}>{ov.deadlines.length === 0 ? <Typography level="body-sm" sx={{ p: 2 }}>Nothing is due.</Typography> : ov.deadlines.map(drow)}</Card>
                {ov.limitation_watch.length > 0 && (
                  <>
                    <Typography level="title-md" sx={{ mt: 2, mb: 0.5 }}>Limitation dates within 90 days</Typography>
                    <Card variant="outlined" sx={{ p: 0 }}>{ov.limitation_watch.map(mrow)}</Card>
                  </>
                )}
              </Box>
              <Box>
                <Typography level="title-md" sx={{ mb: 0.5 }}>Recent matters</Typography>
                <Card variant="outlined" sx={{ p: 0 }}>{ov.recent_matters.length === 0 ? <Typography level="body-sm" sx={{ p: 2 }}>No matters yet. Create one to start.</Typography> : ov.recent_matters.map(mrow)}</Card>
                {meta.admin && hours.length > 0 && (
                  <>
                    <Typography level="title-md" sx={{ mt: 2, mb: 0.5 }}>Hours by person</Typography>
                    <Card variant="outlined" sx={{ p: 0 }}>
                      {hours.map((h) => (
                        <Box key={h.user_id} sx={rowSx}><Typography level="body-sm" sx={{ flex: 1 }}>{h.user}</Typography><Typography level="body-sm">{fmtHM(h.minutes)}</Typography><Typography level="body-sm" sx={{ width: 120, textAlign: "right" }}>{fmtMoney(h.value)}</Typography></Box>
                      ))}
                    </Card>
                  </>
                )}
              </Box>
            </Box>
          </Box>
        )}

        {meta && tab === "matters" && (
          <Box sx={{ mt: 2 }}>
            <Box sx={{ display: "flex", gap: 1, mb: 1.5, flexWrap: "wrap" }}>
              <Input sx={{ flex: 1, minWidth: 200 }} placeholder="Search number, title or client" value={q} onChange={(e) => setQ(e.target.value)} />
              <Select value={status} onChange={(_, v) => setStatus(v ?? "")} placeholder="All statuses" sx={{ minWidth: 150 }}>
                <Option value="">All statuses</Option>{meta.statuses.map((s) => <Option key={s} value={s}>{STATUS_LABEL[s]}</Option>)}
              </Select>
            </Box>
            <Card variant="outlined" sx={{ p: 0 }}>{matters.length === 0 ? <Typography level="body-sm" sx={{ p: 2 }}>No matters match.</Typography> : matters.map(mrow)}</Card>
          </Box>
        )}

        {meta && tab === "clients" && (
          <Box sx={{ mt: 2 }}>
            <Button size="sm" sx={{ mb: 1.5 }} onClick={() => { setEditClient(undefined); setForm("client"); }} data-testid="new-client">＋ New client</Button>
            <Card variant="outlined" sx={{ p: 0 }}>
              {clients.length === 0 ? <Typography level="body-sm" sx={{ p: 2 }}>No clients yet.</Typography> : clients.map((c) => (
                <Box key={c.id} sx={{ ...rowSx, cursor: "pointer" }} onClick={() => { setEditClient(c); setForm("client"); }}>
                  <Box sx={{ flex: 1, minWidth: 200 }}><Typography level="title-sm">{c.name} {c.archived && <Chip size="sm">Archived</Chip>}</Typography><Typography level="body-xs">{[c.email, c.phone].filter(Boolean).join(" · ") || "No contact details"}</Typography></Box>
                  <Chip size="sm" variant="soft">{c.kind === "company" ? "Company" : "Person"}</Chip>
                  <Typography level="body-sm" sx={{ width: 130, textAlign: "right" }}>{c.open_matters} open of {c.matters}</Typography>
                </Box>
              ))}
            </Card>
          </Box>
        )}

        {meta && tab === "deadlines" && (
          <Box sx={{ mt: 2 }}>
            <Card variant="outlined" sx={{ p: 0 }}>{deadlines.length === 0 ? <Typography level="body-sm" sx={{ p: 2 }}>No open deadlines. Add them from a matter.</Typography> : deadlines.map(drow)}</Card>
          </Box>
        )}

        {meta && tab === "time" && (
          <Box sx={{ mt: 2 }}>
            <Box sx={{ display: "flex", gap: 1, mb: 1.5 }}><Button size="sm" variant="outlined" onClick={exportEntries} disabled={entries.length === 0}>Export CSV</Button><Typography level="body-xs" sx={{ alignSelf: "center" }}>Record work from a matter. Time is rounded up to {meta.settings.increment_min} minutes.</Typography></Box>
            <Card variant="outlined" sx={{ p: 0 }}>
              {entries.length === 0 ? <Typography level="body-sm" sx={{ p: 2 }}>No time or expenses yet.</Typography> : entries.slice(0, 200).map((e) => (
                <Box key={e.id} sx={{ ...rowSx, cursor: "pointer" }} onClick={() => setOpenId(e.matter_id)}>
                  <Typography level="body-sm" sx={{ width: 96 }}>{fmtDate(e.date)}</Typography>
                  <Typography level="body-sm" sx={{ flex: 1, minWidth: 200 }}>{e.description || "—"} <Typography level="body-xs" component="span">· {e.matter_number} · {e.user}</Typography></Typography>
                  <Chip size="sm" variant="soft">{e.kind === "time" ? fmtHM(e.minutes) : e.kind}</Chip>
                  <Typography level="body-sm" sx={{ width: 110, textAlign: "right" }}>{e.billable ? fmtMoney(e.amount) : "non-billable"}</Typography>
                  {e.bill_id && <Chip size="sm" color="success" variant="soft">Billed</Chip>}
                </Box>
              ))}
            </Card>
          </Box>
        )}

        {meta && tab === "bills" && (
          <Box sx={{ mt: 2 }}>
            <Card variant="outlined" sx={{ p: 0 }}>
              {bills.length === 0 ? <Typography level="body-sm" sx={{ p: 2 }}>No bills yet. Prepare one from a matter's Bills tab.</Typography> : bills.map((b) => (
                <Box key={b.id} sx={{ ...rowSx, cursor: "pointer" }} onClick={() => setOpenId(b.matter_id)}>
                  <Typography level="body-sm" sx={{ width: 150, fontFamily: "monospace" }}>{b.number}</Typography>
                  <Box sx={{ flex: 1, minWidth: 180 }}><Typography level="title-sm">{b.client_name || b.matter_title}</Typography><Typography level="body-xs">{fmtDate(b.issue_date)} · due {fmtDate(b.due_date)}{b.invoice_number ? ` · Invoice ${b.invoice_number}` : ""}</Typography></Box>
                  <Chip size="sm" variant="soft" color={BILL_COLOR[b.status]}>{b.status}</Chip>
                  <Typography level="body-sm" sx={{ width: 120, textAlign: "right" }}>{fmtMoney(b.total)}</Typography>
                </Box>
              ))}
            </Card>
          </Box>
        )}

        {meta && tab === "conflicts" && <Conflicts onOpen={setOpenId} />}
        {meta && tab === "settings" && <SettingsPane meta={meta} onSaved={() => void load()} />}
      </Container>

      {form === "matter" && meta && <MatterForm meta={meta} clients={clients} people={people} onClose={() => setForm(null)} onSaved={(m) => { setForm(null); setOpenId(m.id); void load(); }} />}
      {form === "client" && <ClientForm initial={editClient} onClose={() => setForm(null)} onSaved={() => { setForm(null); void load(); }} />}
      {openId && meta && <MatterDetail id={openId} meta={meta} clients={clients} people={people} onClose={() => setOpenId("")} onChanged={() => void load()} />}
    </>
  );
}

function Conflicts({ onOpen }: { onOpen: (id: string) => void }) {
  const [q, setQ] = useState("");
  const [res, setRes] = useState<ConflictResult | null>(null);
  const [err, setErr] = useState("");
  const go = async () => { try { setRes(await legal.conflictCheck(q)); setErr(""); } catch (e) { setErr(errText(e)); } };
  return (
    <Box sx={{ mt: 2, maxWidth: 820 }}>
      <Typography level="body-sm" sx={{ mb: 1 }}>Search every client, every party on every matter and every contact by name, email or phone. Accents and case are ignored. To save a check on a matter's file, use “Run and save conflict check” on the matter's Overview.</Typography>
      <Box sx={{ display: "flex", gap: 1, mb: 2 }}>
        <Input sx={{ flex: 1 }} placeholder="Name, email or phone" value={q} onChange={(e) => setQ(e.target.value)} onKeyDown={(e) => e.key === "Enter" && void go()} data-testid="conflict-q" />
        <Button onClick={() => void go()} disabled={q.trim().length < 2} data-testid="conflict-go">Check</Button>
      </Box>
      {err && <Alert color="danger">{err}</Alert>}
      {res && res.clear && <Alert color="success">No matches for “{res.query}”.</Alert>}
      {res?.hits.map((h) => (
        <Card key={h.source + h.id} variant="outlined" size="sm" sx={{ mb: 1 }}>
          <Typography level="title-sm">{h.name} <Chip size="sm" variant="soft">{h.source === "party" ? h.role : h.source}</Chip> <Chip size="sm" variant="outlined">{h.score}% match</Chip></Typography>
          {h.matters.length === 0 ? <Typography level="body-xs">Not on any matter.</Typography> : h.matters.map((m, i) => (
            m.restricted ? <Typography key={i} level="body-xs">A restricted matter as {m.role} (details hidden)</Typography>
              : <Typography key={i} level="body-xs" sx={{ cursor: "pointer", textDecoration: "underline" }} onClick={() => m.id && onOpen(m.id)}>{m.number} {m.title} · {STATUS_LABEL[m.status ?? ""] ?? m.status} · as {m.role}</Typography>
          ))}
        </Card>
      ))}
    </Box>
  );
}

function SettingsPane({ meta, onSaved }: { meta: Meta; onSaved: () => void }) {
  const s = meta.settings;
  const [f, setF] = useState({ number_format: s.number_format, increment_min: String(s.increment_min), default_rate: (s.default_rate / 100).toString(), terms_days: String(s.terms_days), tax_rate: (s.tax_rate_bp / 100).toString(), tax_name: s.tax_name, areas: s.practice_areas.join("\n") });
  const [msg, setMsg] = useState("");
  if (!meta.admin) return <Typography level="body-sm" sx={{ mt: 2 }}>Only organization admins change the billing settings. Billing increment {s.increment_min} minutes, default rate {fmtMoney(s.default_rate)}, {s.tax_name} {(s.tax_rate_bp / 100).toFixed(2)}%.</Typography>;
  const save = async () => {
    try {
      const rate = Math.round(Number(f.default_rate.replace(/,/g, "")) * 100);
      await legal.saveSettings({ ...s, number_format: f.number_format, increment_min: Number(f.increment_min), default_rate: rate, terms_days: Number(f.terms_days), tax_rate_bp: Math.round(Number(f.tax_rate) * 100), tax_name: f.tax_name, practice_areas: f.areas.split("\n").map((x) => x.trim()).filter(Boolean) });
      setMsg("Saved."); onSaved();
    } catch (e) { setMsg(errText(e)); }
  };
  return (
    <Box sx={{ mt: 2, maxWidth: 560, display: "grid", gap: 1.5 }}>
      <Typography level="body-sm">Matter numbers: <Select size="sm" sx={{ display: "inline-flex" }} value={f.number_format} onChange={(_, v) => v && setF({ ...f, number_format: v })}><Option value="year-number">2026-00012</Option><Option value="number">00012</Option><Option value="number-client">00012-Lopez</Option></Select> (applies to new matters)</Typography>
      <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1.5 }}>
        <Box><Typography level="body-xs">Billing increment (minutes)</Typography><Input value={f.increment_min} onChange={(e) => setF({ ...f, increment_min: e.target.value })} /></Box>
        <Box><Typography level="body-xs">Default hourly rate (₹)</Typography><Input value={f.default_rate} onChange={(e) => setF({ ...f, default_rate: e.target.value })} /></Box>
        <Box><Typography level="body-xs">Tax name</Typography><Input value={f.tax_name} onChange={(e) => setF({ ...f, tax_name: e.target.value })} /></Box>
        <Box><Typography level="body-xs">Tax rate (%)</Typography><Input value={f.tax_rate} onChange={(e) => setF({ ...f, tax_rate: e.target.value })} /></Box>
        <Box><Typography level="body-xs">Payment terms (days)</Typography><Input value={f.terms_days} onChange={(e) => setF({ ...f, terms_days: e.target.value })} /></Box>
      </Box>
      <Box><Typography level="body-xs">Practice areas (one per line)</Typography><Textarea minRows={5} value={f.areas} onChange={(e) => setF({ ...f, areas: e.target.value })} /></Box>
      <Box sx={{ display: "flex", gap: 1, alignItems: "center" }}><Button onClick={() => void save()}>Save settings</Button><Typography level="body-sm">{msg}</Typography></Box>
    </Box>
  );
}

