import { useCallback, useEffect, useMemo, useState } from "react";
import { Box, Button, Card, Chip, CircularProgress, Container, Input, Option, Select, Tab, TabList, Tabs, Typography } from "@mui/joy";
import GavelOutlined from "@mui/icons-material/GavelOutlined";
import { Header } from "../../components/Header";
import { BizAssistant } from "../../components/BizAssistant";
import type { User } from "../../api/types";
import { deepParam } from "../../lib/deepLink";
import { fmtDate, money } from "../know/http";
import { contracts, type Contract, type ObItem, type Stats } from "./api";
import { ContractForm } from "./Form";
import { ContractDetail } from "./Detail";
import { clockText, contractsCsv, groupObligations, PHASE_COLOR, PHASE_LABEL, renewalGroups, TYPE_LABEL } from "./logic";
import { createContractsTools, CONTRACTS_HINT, CONTRACTS_SUGGESTIONS } from "./ai/tools";

const ACCENT = "#126C5A";
type TabId = "dashboard" | "contracts" | "renewals" | "obligations";

function PhaseChip({ c }: { c: Contract }) {
  const r = c.renewal;
  if (r.phase === "none") return <Chip size="sm" variant="soft">{c.status}</Chip>;
  const label = r.handled && r.decision ? `Decided: ${r.decision.kind.replace("-", " ")}` : PHASE_LABEL[r.phase];
  return <Chip size="sm" variant="soft" color={r.handled ? "neutral" : PHASE_COLOR[r.phase]}>{label}</Chip>;
}

function Stat({ label, value, tone }: { label: string; value: string | number; tone?: "danger" | "warning" }) {
  return (
    <Card variant="outlined" size="sm" sx={{ minWidth: 140, flex: 1 }}>
      <Typography level="body-xs">{label}</Typography>
      <Typography level="h3" color={tone}>{value}</Typography>
    </Card>
  );
}

export default function ContractsApp({ user }: { user: User }) {
  const [tab, setTab] = useState<TabId>("dashboard");
  const [list, setList] = useState<Contract[] | null>(null);
  const [stats, setStats] = useState<Stats | null>(null);
  const [obs, setObs] = useState<ObItem[]>([]);
  const [q, setQ] = useState("");
  const [status, setStatus] = useState("");
  const [openId, setOpenId] = useState(deepParam("contract"));
  const [form, setForm] = useState<{ c?: Contract } | null>(null);
  const [err, setErr] = useState("");
  const tools = useMemo(() => createContractsTools(), []);

  const load = useCallback(async () => {
    try {
      const [l, o] = await Promise.all([contracts.list(q, status), contracts.overview()]);
      setList(l); setStats(o.stats); setObs(o.obligations); setErr("");
    } catch (e) { setErr((e as Error).message); }
  }, [q, status]);
  useEffect(() => { const t = setTimeout(() => void load(), q ? 250 : 0); return () => clearTimeout(t); }, [load, q]);
  const opened = list?.find((c) => c.id === openId);
  const attention = (list ?? []).filter((c) => c.renewal.attention);
  const groups = useMemo(() => renewalGroups(list ?? []), [list]);

  const download = () => {
    const blob = new Blob([contractsCsv(list ?? [])], { type: "text/csv" });
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob); a.download = "contracts.csv"; a.click(); URL.revokeObjectURL(a.href);
  };

  const row = (c: Contract) => (
    <Box key={c.id} onClick={() => setOpenId(c.id)} role="button" tabIndex={0} onKeyDown={(e) => e.key === "Enter" && setOpenId(c.id)} data-testid="contract-row"
      sx={{ display: "flex", gap: 1.5, alignItems: "center", px: 1.5, py: 1, borderBottom: "1px solid", borderColor: "divider", cursor: "pointer", flexWrap: "wrap", "&:hover": { bgcolor: "background.level1" } }}>
      <Box sx={{ flex: 1, minWidth: 180 }}>
        <Typography level="title-sm">{c.title}</Typography>
        <Typography level="body-xs">{c.party_label || "No counterparty"} · {TYPE_LABEL[c.type] ?? c.type}</Typography>
      </Box>
      <Typography level="body-sm" sx={{ minWidth: 150 }}>{clockText(c.renewal)}</Typography>
      <Box sx={{ minWidth: 130 }}><PhaseChip c={c} /></Box>
      <Typography level="body-sm" sx={{ minWidth: 110, textAlign: "right" }}>{c.value ? money(c.value, c.currency) : ""}</Typography>
    </Box>
  );

  return (
    <>
      <Header user={user} />
      <Container maxWidth="xl" sx={{ py: { xs: 2, sm: 3 }, px: { xs: 1.5, sm: 3 } }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, mb: 2, flexWrap: "wrap" }}>
          <GavelOutlined sx={{ fontSize: 28, color: ACCENT }} />
          <Typography level="h2" sx={{ fontSize: { xs: "xl", sm: "xl3" } }}>Contracts</Typography>
          <Box sx={{ ml: "auto", display: "flex", gap: 1 }}>
            <Button size="sm" onClick={() => setForm({})} data-testid="new-contract">＋ New contract</Button>
            <BizAssistant app="contracts" title="Contracts" tools={tools} suggestions={CONTRACTS_SUGGESTIONS} systemHint={CONTRACTS_HINT} />
          </Box>
        </Box>
        <Tabs value={tab} onChange={(_, v) => v && setTab(v as TabId)} sx={{ bgcolor: "transparent" }}>
          <TabList sx={{ overflowX: "auto" }}>
            <Tab value="dashboard">Dashboard</Tab>
            <Tab value="contracts">All contracts</Tab>
            <Tab value="renewals">Renewals{attention.length ? ` (${attention.length})` : ""}</Tab>
            <Tab value="obligations">Obligations</Tab>
          </TabList>
        </Tabs>
        {err && <Typography color="danger" role="alert" sx={{ my: 1 }}>{err}</Typography>}
        {!list && !err && <Box sx={{ p: 3 }}><CircularProgress /></Box>}
        {list && tab === "dashboard" && stats && (
          <Box sx={{ mt: 2 }}>
            <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap", mb: 2 }}>
              <Stat label="Active contracts" value={stats.active} />
              <Stat label="Need a decision" value={stats.needs_attention} tone={stats.needs_attention ? "warning" : undefined} />
              <Stat label="Ending in 60 days" value={stats.renewing_60_days} />
              <Stat label="Overdue obligations" value={stats.overdue_obligations} tone={stats.overdue_obligations ? "danger" : undefined} />
              <Stat label="Yearly value" value={Object.entries(stats.annual_by_currency).map(([k, v]) => money(v, k)).join(" + ") || "—"} />
            </Box>
            <Typography level="title-md" sx={{ mb: 0.5 }}>Needs your attention</Typography>
            {attention.length === 0 ? <Typography level="body-sm" sx={{ mb: 2 }}>Nothing is waiting on a decision.</Typography> : <Card variant="outlined" sx={{ p: 0, mb: 2 }}>{attention.map(row)}</Card>}
            <Typography level="title-md" sx={{ mb: 0.5 }}>Next obligations</Typography>
            {obs.length === 0 ? <Typography level="body-sm">No open obligations.</Typography> : (
              <Card variant="outlined" sx={{ p: 0 }}>
                {obs.slice(0, 6).map((o) => (
                  <Box key={o.id} onClick={() => setOpenId(o.contract_id)} sx={{ display: "flex", gap: 1.5, px: 1.5, py: 0.8, borderBottom: "1px solid", borderColor: "divider", cursor: "pointer", flexWrap: "wrap" }}>
                    <Typography level="body-sm" sx={{ flex: 1, minWidth: 160 }}>{o.title}</Typography>
                    <Typography level="body-xs" sx={{ alignSelf: "center" }}>{o.contract_title}</Typography>
                    <Chip size="sm" variant="soft" color={o.urgency === "overdue" ? "danger" : o.urgency === "soon" ? "warning" : "neutral"}>{o.due ? fmtDate(o.due) : "No date"}</Chip>
                  </Box>
                ))}
              </Card>
            )}
            <Box sx={{ mt: 2 }}><Button size="sm" variant="outlined" color="neutral" component="a" href={contracts.icsUrl}>Download deadlines (.ics)</Button></Box>
          </Box>
        )}
        {list && tab === "contracts" && (
          <Box sx={{ mt: 2 }}>
            <Box sx={{ display: "flex", gap: 1, mb: 1.5, flexWrap: "wrap" }}>
              <Input placeholder="Search title, party, tags, notes" value={q} onChange={(e) => setQ(e.target.value)} sx={{ flex: 1, minWidth: 200 }} slotProps={{ input: { "aria-label": "Search contracts" } }} />
              <Select value={status} onChange={(_, v) => setStatus(v ?? "")} sx={{ width: 150 }} slotProps={{ listbox: { sx: { zIndex: 2000 } } }}>
                <Option value="">All statuses</Option><Option value="active">Active</Option><Option value="draft">Draft</Option><Option value="terminated">Terminated</Option>
              </Select>
              <Button variant="outlined" color="neutral" onClick={download}>Export CSV</Button>
            </Box>
            {list.length === 0 ? <Typography sx={{ p: 3, textAlign: "center" }}>No contracts match. Add your first agreement with “New contract”.</Typography> : <Card variant="outlined" sx={{ p: 0 }}>{list.map(row)}</Card>}
          </Box>
        )}
        {list && tab === "renewals" && (
          <Box sx={{ mt: 2 }}>
            {groups.length === 0 && <Typography sx={{ p: 3 }}>No contract has an end date yet.</Typography>}
            {groups.map((g) => (
              <Box key={g.key} sx={{ mb: 2 }}>
                <Typography level="title-md" sx={{ mb: 0.5 }}>{g.label} <Chip size="sm" variant="soft">{g.items.length}</Chip></Typography>
                <Card variant="outlined" sx={{ p: 0 }}>{g.items.map(row)}</Card>
              </Box>
            ))}
          </Box>
        )}
        {list && tab === "obligations" && (
          <Box sx={{ mt: 2 }}>
            {obs.length === 0 && <Typography sx={{ p: 3 }}>No open obligations. Add them from a contract.</Typography>}
            {groupObligations(obs).map((g) => (
              <Box key={g.key} sx={{ mb: 2 }}>
                <Typography level="title-md" sx={{ mb: 0.5 }}>{g.label} <Chip size="sm" variant="soft" color={g.key === "overdue" ? "danger" : "neutral"}>{g.items.length}</Chip></Typography>
                <Card variant="outlined" sx={{ p: 0 }}>
                  {g.items.map((o) => (
                    <Box key={o.id} onClick={() => setOpenId(o.contract_id)} sx={{ display: "flex", gap: 1.5, px: 1.5, py: 0.8, borderBottom: "1px solid", borderColor: "divider", cursor: "pointer", flexWrap: "wrap" }}>
                      <Typography level="body-sm" sx={{ flex: 1, minWidth: 160 }}>{o.title}</Typography>
                      <Typography level="body-xs" sx={{ alignSelf: "center" }}>{o.contract_title}</Typography>
                      <Typography level="body-xs" sx={{ alignSelf: "center", minWidth: 90 }}>{o.due ? fmtDate(o.due) : ""}</Typography>
                    </Box>
                  ))}
                </Card>
              </Box>
            ))}
          </Box>
        )}
      </Container>
      {openId && (opened || list) && !form && <ContractDetail key={openId} id={openId} initial={opened ?? ({ id: openId, title: "", renewal: { phase: "none", handled: false, attention: false }, tags: [], can_edit: false } as unknown as Contract)} onClose={() => { setOpenId(""); window.history.replaceState(null, "", "/contracts"); }} onEdit={(c) => setForm({ c })} onChanged={() => void load()} />}
      {form && <ContractForm initial={form.c} onClose={() => setForm(null)} onSaved={(c) => { setForm(null); setOpenId(c.id); void load(); }} />}
    </>
  );
}
