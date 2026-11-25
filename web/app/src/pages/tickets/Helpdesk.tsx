import { useCallback, useEffect, useState } from "react";
import {
  Alert, Box, Button, Checkbox, Chip, CircularProgress, IconButton, Input, Modal, ModalClose, ModalDialog,
  Option, Select, Sheet, Tab, TabList, TabPanel, Tabs, Textarea, Typography,
} from "@mui/joy";
import DeleteIcon from "@mui/icons-material/DeleteOutline";
import * as api from "./helpApi";
import { MacrosTab } from "./Desk";
import { SLA_COLOR, activeClock, barHeights, fmtMinutes, slaText, splitMinutes, toMinutes, worstState, type SlaInfo } from "./helpLogic";

const PRIORITIES = ["urgent", "high", "normal", "low"];

/** One badge for the row of a ticket. */
export function SlaBadge({ info }: { info?: SlaInfo }) {
  const a = activeClock(info);
  const w = worstState(info);
  if (!a || !w) return null;
  const text = slaText(a.clock);
  return (
    <Chip size="sm" variant="soft" color={SLA_COLOR[w] ?? "neutral"} data-testid="sla-badge" title={`${a.name}: ${text}`}>
      {w === "breached" ? "SLA breached" : w === "at_risk" ? "SLA at risk" : text || "SLA"}
    </Chip>
  );
}

/** SLA clocks and survey result on a ticket. */
export function TicketExtras({ ticketId, resolved, sla }: { ticketId: string; resolved: boolean; sla?: SlaInfo }) {
  const [csat, setCsat] = useState<api.Csat | null>(null);
  useEffect(() => {
    setCsat(null);
    if (resolved) api.csatFor(ticketId).then(setCsat).catch(() => undefined);
  }, [ticketId, resolved]);
  const fr = sla?.first_response;
  const rs = sla?.resolution;
  return (
    <Box sx={{ display: "flex", gap: 2, flexWrap: "wrap", mt: 1.5 }} data-testid="ticket-extras">
      {fr?.state && (
        <Typography level="body-xs">
          First response: <Chip size="sm" variant="soft" color={SLA_COLOR[fr.state]}>{slaText(fr)}</Chip>
        </Typography>
      )}
      {rs?.state && (
        <Typography level="body-xs">
          Resolution: <Chip size="sm" variant="soft" color={SLA_COLOR[rs.state]}>{slaText(rs)}</Chip>
        </Typography>
      )}
      {csat && (
        <Typography level="body-xs" data-testid="csat-line">
          Satisfaction survey:{" "}
          {csat.answered ? (
            <b>{csat.rating} of 5{csat.comment ? ` · "${csat.comment}"` : ""}</b>
          ) : (
            <>
              not answered yet ·{" "}
              <a href={`/csat/${csat.token}`} target="_blank" rel="noreferrer">survey link</a>{" "}
              <Button size="sm" variant="plain" onClick={() => void navigator.clipboard?.writeText(`${window.location.origin}/csat/${csat.token}`)}>Copy</Button>
            </>
          )}
        </Typography>
      )}
    </Box>
  );
}

/** Insert a saved reply into the comment box. */
export function CannedPicker({ projectId, onPick }: { projectId: string; onPick: (body: string) => void }) {
  const [list, setList] = useState<api.Canned[]>([]);
  useEffect(() => {
    api.listCanned(projectId).then(setList).catch(() => setList([]));
  }, [projectId]);
  if (list.length === 0) return null;
  return (
    <Select size="sm" placeholder="Insert saved reply" value={null} onChange={(_, v) => { const c = list.find((x) => x.id === v); if (c) onPick(c.body); }} sx={{ minWidth: 190 }} slotProps={{ button: { "data-testid": "canned-picker" } as never }}>
      {list.map((c) => (
        <Option key={c.id} value={c.id}>{c.title}</Option>
      ))}
    </Select>
  );
}

// ---------------------------------------------------------------- settings

function SlaTab({ pid }: { pid: string }) {
  const [rows, setRows] = useState<Record<string, { fr: { value: number; unit: "min" | "h" | "d" }; rs: { value: number; unit: "min" | "h" | "d" } }>>({});
  const [err, setErr] = useState("");
  const [saved, setSaved] = useState(false);
  useEffect(() => {
    api.getSla(pid).then((ps) => {
      const m: typeof rows = {};
      for (const p of PRIORITIES) {
        const x = ps.find((q) => q.priority === p);
        m[p] = { fr: splitMinutes(x?.first_response_min ?? 0), rs: splitMinutes(x?.resolve_min ?? 0) };
      }
      setRows(m);
    }).catch((e: Error) => setErr(e.message));
  }, [pid]);
  async function save() {
    setErr("");
    try {
      await api.saveSla(pid, PRIORITIES.map((p) => ({ priority: p, first_response_min: toMinutes(rows[p].fr.value, rows[p].fr.unit), resolve_min: toMinutes(rows[p].rs.value, rows[p].rs.unit) })));
      setSaved(true);
      setTimeout(() => setSaved(false), 2000);
    } catch (e) {
      setErr((e as Error).message);
    }
  }
  const cell = (p: string, k: "fr" | "rs") => {
    const v = rows[p]?.[k];
    if (!v) return null;
    return (
      <Box sx={{ display: "flex", gap: 0.5 }}>
        <Input size="sm" type="number" value={v.value} sx={{ width: 90 }} onChange={(e) => setRows({ ...rows, [p]: { ...rows[p], [k]: { ...v, value: Number(e.target.value) } } })} slotProps={{ input: { min: 0, "data-testid": `sla-${p}-${k}` } }} />
        <Select size="sm" value={v.unit} onChange={(_, u) => setRows({ ...rows, [p]: { ...rows[p], [k]: { ...v, unit: (u ?? "min") as "min" | "h" | "d" } } })}>
          <Option value="min">min</Option>
          <Option value="h">hours</Option>
          <Option value="d">days</Option>
        </Select>
      </Box>
    );
  };
  return (
    <Box>
      <Typography level="body-sm" sx={{ mb: 1 }}>Time targets per priority. Leave 0 for no target. The timers run on calendar time.</Typography>
      <Box sx={{ display: "grid", gridTemplateColumns: "90px 1fr 1fr", gap: 1, alignItems: "center", overflowX: "auto" }}>
        <span />
        <Typography level="title-sm">First response within</Typography>
        <Typography level="title-sm">Resolved within</Typography>
        {PRIORITIES.map((p) => (
          <Box key={p} sx={{ display: "contents" }}>
            <Typography level="body-sm" sx={{ textTransform: "capitalize" }}>{p}</Typography>
            {cell(p, "fr")}
            {cell(p, "rs")}
          </Box>
        ))}
      </Box>
      {err && <Alert color="danger" sx={{ mt: 1 }}>{err}</Alert>}
      <Button sx={{ mt: 1.5 }} onClick={save} data-testid="sla-save">{saved ? "Saved" : "Save targets"}</Button>
    </Box>
  );
}

export function RulesTab({ pid, statuses }: { pid: string; statuses: string[] }) {
  const [rules, setRules] = useState<api.Rule[] | null>(null);
  const [mem, setMem] = useState<api.Member[]>([]);
  const blank = (): api.Rule => ({ name: "", enabled: true, match_keyword: "", match_priority: "", match_source: "", set_assignee: "", set_priority: "", set_status: "" });
  const [draft, setDraft] = useState<api.Rule>(blank());
  const [err, setErr] = useState("");
  const load = useCallback(() => api.listRules(pid).then(setRules).catch((e: Error) => { setErr(e.message); setRules([]); }), [pid]);
  useEffect(() => { void load(); api.members().then(setMem).catch(() => undefined); }, [load]);
  async function add() {
    setErr("");
    try {
      await api.createRule(pid, draft);
      setDraft(blank());
      await load();
    } catch (e) {
      setErr((e as Error).message);
    }
  }
  const describe = (r: api.Rule) =>
    `When ${[r.match_keyword && `text has "${r.match_keyword}"`, r.match_priority && `priority is ${r.match_priority}`, r.match_source && `from ${r.match_source}`].filter(Boolean).join(" and ")}: ${[
      r.set_assignee && `assign to ${mem.find((m) => m.id === r.set_assignee)?.name ?? "a person"}`, r.set_priority && `set ${r.set_priority} priority`, r.set_status && `set status ${r.set_status}`,
    ].filter(Boolean).join(", ")}`;
  return (
    <Box>
      <Typography level="body-sm" sx={{ mb: 1 }}>Rules run when a ticket is created. The first matching rule applies.</Typography>
      {rules?.map((r) => (
        <Box key={r.id} sx={{ display: "flex", gap: 1, alignItems: "center", py: 0.5, borderBottom: "1px solid", borderColor: "divider" }} data-testid="rule-row">
          <Checkbox size="sm" checked={r.enabled} onChange={async (e) => { await api.updateRule(pid, { ...r, enabled: e.target.checked }).catch((x: Error) => setErr(x.message)); await load(); }} />
          <Box sx={{ flex: 1, minWidth: 0 }}>
            <Typography level="title-sm">{r.name}</Typography>
            <Typography level="body-xs">{describe(r)}</Typography>
          </Box>
          <IconButton size="sm" variant="plain" color="danger" aria-label={`Delete rule ${r.name}`} onClick={async () => { await api.deleteRule(r.id!).catch((x: Error) => setErr(x.message)); await load(); }}><DeleteIcon fontSize="small" /></IconButton>
        </Box>
      ))}
      {rules?.length === 0 && <Typography level="body-sm">No rules yet.</Typography>}
      <Typography level="title-sm" sx={{ mt: 2 }}>New rule</Typography>
      <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" }, gap: 1, mt: 0.5 }}>
        <Input placeholder="Rule name" value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} slotProps={{ input: { "data-testid": "rule-name" } }} />
        <Input placeholder='Words in title or text, comma separated' value={draft.match_keyword} onChange={(e) => setDraft({ ...draft, match_keyword: e.target.value })} slotProps={{ input: { "data-testid": "rule-keyword" } }} />
        <Select placeholder="Priority is..." value={draft.match_priority || null} onChange={(_, v) => setDraft({ ...draft, match_priority: v ?? "" })}>
          {PRIORITIES.map((p) => <Option key={p} value={p}>{p}</Option>)}
        </Select>
        <Select placeholder="Comes from..." value={draft.match_source || null} onChange={(_, v) => setDraft({ ...draft, match_source: v ?? "" })}>
          <Option value="public">Public form</Option>
          <Option value="team">Team members</Option>
        </Select>
        <Select placeholder="Assign to..." value={draft.set_assignee || null} onChange={(_, v) => setDraft({ ...draft, set_assignee: v ?? "" })}>
          {mem.map((m) => <Option key={m.id} value={m.id}>{m.name || m.email}</Option>)}
        </Select>
        <Select placeholder="Set priority..." value={draft.set_priority || null} onChange={(_, v) => setDraft({ ...draft, set_priority: v ?? "" })}>
          {PRIORITIES.map((p) => <Option key={p} value={p}>{p}</Option>)}
        </Select>
        <Select placeholder="Set status..." value={draft.set_status || null} onChange={(_, v) => setDraft({ ...draft, set_status: v ?? "" })}>
          {statuses.map((s) => <Option key={s} value={s}>{s}</Option>)}
        </Select>
      </Box>
      {err && <Alert color="danger" sx={{ mt: 1 }}>{err}</Alert>}
      <Button sx={{ mt: 1 }} onClick={add} data-testid="rule-add">Add rule</Button>
    </Box>
  );
}

export function CannedTab({ pid }: { pid: string }) {
  const [list, setList] = useState<api.Canned[]>([]);
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [all, setAll] = useState(false);
  const [err, setErr] = useState("");
  const load = useCallback(() => api.listCanned(pid).then(setList).catch(() => setList([])), [pid]);
  useEffect(() => { void load(); }, [load]);
  async function add() {
    setErr("");
    try {
      await api.createCanned(all ? "" : pid, title, body);
      setTitle("");
      setBody("");
      await load();
    } catch (e) {
      setErr((e as Error).message);
    }
  }
  return (
    <Box>
      {list.map((c) => (
        <Box key={c.id} sx={{ display: "flex", gap: 1, py: 0.5, borderBottom: "1px solid", borderColor: "divider" }} data-testid="canned-row">
          <Box sx={{ flex: 1, minWidth: 0 }}>
            <Typography level="title-sm">{c.title}{!c.project_id && <Chip size="sm" sx={{ ml: 1 }}>all projects</Chip>}</Typography>
            <Typography level="body-xs" noWrap>{c.body}</Typography>
          </Box>
          <IconButton size="sm" variant="plain" color="danger" aria-label={`Delete ${c.title}`} onClick={async () => { await api.deleteCanned(c.id).catch((x: Error) => setErr(x.message)); await load(); }}><DeleteIcon fontSize="small" /></IconButton>
        </Box>
      ))}
      {list.length === 0 && <Typography level="body-sm">No saved replies yet.</Typography>}
      <Input sx={{ mt: 2 }} placeholder="Title, e.g. Thanks for reaching out" value={title} onChange={(e) => setTitle(e.target.value)} slotProps={{ input: { "data-testid": "canned-title" } }} />
      <Textarea sx={{ mt: 1 }} minRows={3} placeholder="Reply text" value={body} onChange={(e) => setBody(e.target.value)} slotProps={{ textarea: { "data-testid": "canned-body" } }} />
      <Checkbox sx={{ mt: 1 }} size="sm" label="Available in every project" checked={all} onChange={(e) => setAll(e.target.checked)} />
      {err && <Alert color="danger" sx={{ mt: 1 }}>{err}</Alert>}
      <Box><Button sx={{ mt: 1 }} onClick={add} data-testid="canned-add">Save reply</Button></Box>
    </Box>
  );
}

function KbTab({ pid, publicToken }: { pid: string; publicToken?: string }) {
  const [list, setList] = useState<api.Article[]>([]);
  const [edit, setEdit] = useState<Partial<api.Article> | null>(null);
  const [err, setErr] = useState("");
  const load = useCallback(() => api.listArticles(pid).then(setList).catch(() => setList([])), [pid]);
  useEffect(() => { void load(); }, [load]);
  async function save() {
    if (!edit) return;
    setErr("");
    try {
      await api.saveArticle(pid, edit);
      setEdit(null);
      await load();
    } catch (e) {
      setErr((e as Error).message);
    }
  }
  if (edit) {
    return (
      <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
        <Input placeholder="Article title" value={edit.title ?? ""} onChange={(e) => setEdit({ ...edit, title: e.target.value })} slotProps={{ input: { "data-testid": "kb-title" } }} />
        <Textarea minRows={8} placeholder={"Write the answer. Use '# Heading' and '- list item' lines."} value={edit.body ?? ""} onChange={(e) => setEdit({ ...edit, body: e.target.value })} slotProps={{ textarea: { "data-testid": "kb-body" } }} />
        <Checkbox size="sm" label="Published on the help centre" checked={!!edit.published} onChange={(e) => setEdit({ ...edit, published: e.target.checked })} data-testid="kb-published" />
        {err && <Alert color="danger">{err}</Alert>}
        <Box sx={{ display: "flex", gap: 1 }}>
          <Button onClick={save} data-testid="kb-save">Save article</Button>
          <Button variant="plain" color="neutral" onClick={() => setEdit(null)}>Cancel</Button>
        </Box>
      </Box>
    );
  }
  return (
    <Box>
      {publicToken ? (
        <Typography level="body-sm" sx={{ mb: 1 }}>
          Public help centre: <a href={`/help/${publicToken}`} target="_blank" rel="noreferrer" data-testid="help-link">{window.location.origin}/help/{publicToken}</a>
        </Typography>
      ) : (
        <Typography level="body-sm" sx={{ mb: 1 }}>Turn on the public intake link for this project to get a public help centre address.</Typography>
      )}
      <Button size="sm" onClick={() => setEdit({ title: "", body: "", published: false })} data-testid="kb-new">New article</Button>
      {list.map((a) => (
        <Box key={a.id} sx={{ display: "flex", gap: 1, alignItems: "center", py: 0.75, borderBottom: "1px solid", borderColor: "divider" }} data-testid="kb-row">
          <Box sx={{ flex: 1, minWidth: 0 }}>
            <Typography level="title-sm" noWrap>{a.title}</Typography>
            <Typography level="body-xs">{a.published ? "Published" : "Draft"}</Typography>
          </Box>
          <Button size="sm" variant="outlined" color="neutral" onClick={() => setEdit(a)}>Edit</Button>
          <IconButton size="sm" variant="plain" color="danger" aria-label={`Delete ${a.title}`} onClick={async () => { if (window.confirm(`Delete "${a.title}"?`)) { await api.deleteArticle(a.id).catch((x: Error) => setErr(x.message)); await load(); } }}><DeleteIcon fontSize="small" /></IconButton>
        </Box>
      ))}
    </Box>
  );
}

/** Help desk settings: SLA targets, rules, saved replies, knowledge base. */
export function HelpSettingsDialog({ projectId, statuses, publicToken, onClose, onChanged }: { projectId: string; statuses: string[]; publicToken?: string; onClose: () => void; onChanged: () => void }) {
  return (
    <Modal open onClose={() => { onChanged(); onClose(); }}>
      <ModalDialog sx={{ width: "min(760px, 96vw)", maxHeight: "92vh", overflow: "hidden", display: "flex", flexDirection: "column" }} data-testid="help-settings">
        <ModalClose />
        <Typography level="title-lg">Help desk settings</Typography>
        <Tabs defaultValue={0} sx={{ bgcolor: "transparent", minHeight: 0, flex: 1, overflow: "hidden" }}>
          <TabList sx={{ overflowX: "auto" }}>
            <Tab>SLA</Tab>
            <Tab>Rules</Tab>
            <Tab>Saved replies</Tab>
            <Tab>Macros</Tab>
            <Tab>Help centre</Tab>
          </TabList>
          <TabPanel value={0} sx={{ overflowY: "auto" }}><SlaTab pid={projectId} /></TabPanel>
          <TabPanel value={1} sx={{ overflowY: "auto" }}><RulesTab pid={projectId} statuses={statuses} /></TabPanel>
          <TabPanel value={2} sx={{ overflowY: "auto" }}><CannedTab pid={projectId} /></TabPanel>
          <TabPanel value={3} sx={{ overflowY: "auto" }}><MacrosTab pid={projectId} statuses={statuses} /></TabPanel>
          <TabPanel value={4} sx={{ overflowY: "auto" }}><KbTab pid={projectId} publicToken={publicToken} /></TabPanel>
        </Tabs>
      </ModalDialog>
    </Modal>
  );
}

// ---------------------------------------------------------------- reports / customers

function Bars({ data, color }: { data: { day: string; count: number }[]; color: string }) {
  const h = barHeights(data.map((d) => d.count));
  return (
    <Box sx={{ display: "flex", alignItems: "flex-end", gap: "2px", height: 80 }}>
      {data.map((d, i) => (
        <Box key={d.day} title={`${d.day}: ${d.count}`} sx={{ flex: 1, minWidth: 2, height: `${Math.max(h[i], d.count ? 4 : 1)}%`, bgcolor: color, opacity: d.count ? 1 : 0.25, borderRadius: "2px 2px 0 0" }} />
      ))}
    </Box>
  );
}

export function ReportDialog({ projectId, onClose }: { projectId: string; onClose: () => void }) {
  const [days, setDays] = useState(30);
  const [rep, setRep] = useState<api.Report | null>(null);
  const [err, setErr] = useState("");
  useEffect(() => {
    setRep(null);
    api.report(projectId, days).then(setRep).catch((e: Error) => setErr(e.message));
  }, [projectId, days]);
  const stat = (label: string, value: string, testid: string) => (
    <Sheet variant="outlined" sx={{ p: 1.25, borderRadius: "md", minWidth: 120, flex: "1 1 120px" }}>
      <Typography level="body-xs">{label}</Typography>
      <Typography level="title-lg" data-testid={testid}>{value}</Typography>
    </Sheet>
  );
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: "min(820px, 96vw)", maxHeight: "92vh", overflowY: "auto" }} data-testid="report-dialog">
        <ModalClose />
        <Typography level="title-lg">Reports</Typography>
        <Select size="sm" value={days} onChange={(_, v) => setDays((v ?? 30) as number)} sx={{ width: 160 }}>
          <Option value={7}>Last 7 days</Option>
          <Option value={30}>Last 30 days</Option>
          <Option value={90}>Last 90 days</Option>
        </Select>
        {err && <Alert color="danger">{err}</Alert>}
        {!rep && !err && <CircularProgress />}
        {rep && (
          <>
            <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap" }}>
              {stat("Created", String(rep.created), "rp-created")}
              {stat("Resolved", String(rep.resolved), "rp-resolved")}
              {stat("Open now", String(rep.open_now), "rp-open")}
              {stat("Avg first response", fmtMinutes(rep.avg_first_response_min), "rp-first")}
              {stat("Avg resolution", fmtMinutes(rep.avg_resolution_min), "rp-res")}
              {stat("SLA met", rep.sla_checked ? `${Math.round(rep.sla_met_pct)}%` : "-", "rp-sla")}
              {stat("Satisfaction", rep.csat_count ? `${rep.csat_avg.toFixed(1)} / 5 (${rep.csat_count})` : "-", "rp-csat")}
            </Box>
            <Typography level="title-sm">Created per day</Typography>
            <Bars data={rep.created_per_day} color="#126C5A" />
            <Typography level="title-sm">Resolved per day</Typography>
            <Bars data={rep.resolved_per_day} color="#1a73e8" />
            <Box sx={{ display: "flex", gap: 3, flexWrap: "wrap" }}>
              <Box>
                <Typography level="title-sm">By priority</Typography>
                {Object.entries(rep.by_priority).map(([k, v]) => <Typography key={k} level="body-sm">{k}: {v}</Typography>)}
              </Box>
              <Box>
                <Typography level="title-sm">By assignee</Typography>
                {Object.entries(rep.by_assignee).map(([k, v]) => <Typography key={k} level="body-sm">{k}: {v}</Typography>)}
              </Box>
            </Box>
          </>
        )}
      </ModalDialog>
    </Modal>
  );
}

export function CustomersDialog({ projectId, onClose }: { projectId: string; onClose: () => void }) {
  const [q, setQ] = useState("");
  const [list, setList] = useState<api.Customer[] | null>(null);
  useEffect(() => {
    const t = setTimeout(() => api.customers(projectId, q).then(setList).catch(() => setList([])), 200);
    return () => clearTimeout(t);
  }, [projectId, q]);
  const orgs = new Map<string, number>();
  for (const c of list ?? []) if (c.organisation) orgs.set(c.organisation, (orgs.get(c.organisation) ?? 0) + c.total);
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: "min(760px, 96vw)", maxHeight: "92vh", overflowY: "auto" }} data-testid="customers-dialog">
        <ModalClose />
        <Typography level="title-lg">Customers</Typography>
        <Input placeholder="Search by name or email" value={q} onChange={(e) => setQ(e.target.value)} />
        {orgs.size > 0 && (
          <Box sx={{ display: "flex", gap: 0.5, flexWrap: "wrap" }}>
            {[...orgs.entries()].map(([o, n]) => (
              <Chip key={o} size="sm" variant="soft" onClick={() => setQ(o)}>{o} · {n}</Chip>
            ))}
          </Box>
        )}
        {list === null && <CircularProgress />}
        {list?.length === 0 && <Typography level="body-sm">No customers yet.</Typography>}
        {list?.map((c) => (
          <Box key={c.email} sx={{ display: "flex", alignItems: "center", gap: 1, py: 0.75, borderBottom: "1px solid", borderColor: "divider", flexWrap: "wrap" }} data-testid="customer-row">
            <Box sx={{ flex: 1, minWidth: 160 }}>
              <Typography level="title-sm">{c.name || c.email}</Typography>
              <Typography level="body-xs">{c.email}{c.organisation ? ` · ${c.organisation}` : ""}</Typography>
            </Box>
            <Chip size="sm" variant="soft">{c.total} ticket{c.total === 1 ? "" : "s"}</Chip>
            {c.open > 0 && <Chip size="sm" variant="soft" color="warning">{c.open} open</Chip>}
          </Box>
        ))}
      </ModalDialog>
    </Modal>
  );
}
