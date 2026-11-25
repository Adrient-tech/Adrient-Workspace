import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Alert, Box, Button, Checkbox, Chip, CircularProgress, FormControl, FormLabel, IconButton, Input, Modal, ModalDialog, DialogTitle, DialogContent, Option, Select, Sheet, Switch, Textarea, Typography,
} from "@mui/joy";
import DeleteIcon from "@mui/icons-material/DeleteOutline";
import AddIcon from "@mui/icons-material/Add";
import ArrowUpIcon from "@mui/icons-material/ArrowUpward";
import ArrowDownIcon from "@mui/icons-material/ArrowDownward";
import { svc, fmtMin, PRIORITIES, TICKET_TYPES, type StatusDef, type Group, type Calendar, type SlaRule, type Automation, type RequestForm, type FormField, type AutoTest, type Member } from "./svcApi";
import { RulesTab, CannedTab } from "./Helpdesk";
import { MacrosTab } from "./Desk";
import { peopleApi, type Person } from "../../api/people";
import { FieldsSection } from "./DeskFields";
import { SettingsSection } from "./SettingsSection";
export { SettingsSection };
import { EmptyState } from "../../components/ui/EmptyState";

const errText = (e: unknown) => (e instanceof Error ? e.message : String(e));

const NAV = [
  ["workflow", "Statuses and workflow"], ["fields", "Custom fields"], ["teams", "Teams and routing"], ["hours", "Business hours"], ["sla", "SLA policies"],
  ["automations", "Automations"], ["forms", "Request forms"], ["macros", "Macros"], ["canned", "Saved replies"], ["rules", "Legacy rules"],
] as const;

export function DeskSettings({ pid, statuses, onClose, onChanged }: { pid: string; statuses: string[]; onClose: () => void; onChanged: () => void }) {
  const [tab, setTab] = useState<(typeof NAV)[number][0]>("workflow");
  return (
    <Box sx={{ display: "flex", height: "calc(100vh - 56px)", minHeight: 520 }} data-testid="desk-settings">
      <Box sx={{ width: 230, borderRight: "1px solid", borderColor: "divider", bgcolor: "background.surface", p: 1.5, overflow: "auto", flexShrink: 0 }}>
        <Button size="sm" variant="plain" color="neutral" onClick={() => { onChanged(); onClose(); }} sx={{ mb: 1 }}>← Back to tickets</Button>
        <Typography level="title-md" sx={{ px: 1, mb: 1 }}>Desk settings</Typography>
        {NAV.map(([k, l]) => (
          <Box key={k} onClick={() => setTab(k)} data-testid={`set-${k}`} sx={{ px: 1.25, py: 0.9, borderRadius: "md", cursor: "pointer", bgcolor: tab === k ? "primary.softBg" : "transparent", color: tab === k ? "primary.700" : "text.primary", fontWeight: tab === k ? 600 : 400, fontSize: "sm", "&:hover": { bgcolor: tab === k ? "primary.softBg" : "neutral.softHoverBg" } }}>{l}</Box>
        ))}
      </Box>
      <Box sx={{ flex: 1, overflow: "auto", p: { xs: 2, md: 4 } }}>
        {tab === "workflow" && <WorkflowSection pid={pid} />}
        {tab === "fields" && <FieldsSection pid={pid} />}
        {tab === "teams" && <TeamsSection pid={pid} />}
        {tab === "hours" && <HoursSection />}
        {tab === "sla" && <SlaSection pid={pid} />}
        {tab === "automations" && <AutomationsSection pid={pid} />}
        {tab === "forms" && <FormsSection pid={pid} />}
        {tab === "macros" && <SettingsSection title="Macros" hint="One click applies a set of changes and an optional reply."><MacrosTab pid={pid} statuses={statuses} /></SettingsSection>}
        {tab === "canned" && <SettingsSection title="Saved replies" hint="Reusable answers. Use {{requester.first_name}}, {{ticket.ref}} and {{agent.name}} as variables."><CannedTab pid={pid} /></SettingsSection>}
        {tab === "rules" && <SettingsSection title="Legacy assignment rules" hint="The older keyword rules still run on new tickets. Prefer Automations for new rules."><RulesTab pid={pid} statuses={statuses} /></SettingsSection>}
      </Box>
    </Box>
  );
}

function useLoad<T>(fn: () => Promise<T>, deps: unknown[]): [T | null, () => void, string] {
  const [v, setV] = useState<T | null>(null);
  const [err, setErr] = useState("");
  const [n, setN] = useState(0);
  useEffect(() => { fn().then((x) => { setV(x); setErr(""); }).catch((e) => setErr(errText(e))); }, [n, ...deps]); // eslint-disable-line react-hooks/exhaustive-deps
  return [v, () => setN((x) => x + 1), err];
}

function Saving({ err, ok }: { err: string; ok?: string }) {
  if (err) return <Alert color="danger" size="sm" sx={{ mt: 1 }}>{err}</Alert>;
  if (ok) return <Alert color="success" size="sm" sx={{ mt: 1 }}>{ok}</Alert>;
  return null;
}

// ---------------------------------------------------------------- workflow

function WorkflowSection({ pid }: { pid: string }) {
  const [defs, setDefs] = useState<StatusDef[] | null>(null);
  const [dirty, setDirty] = useState(false);
  const [err, setErr] = useState("");
  const [ok, setOk] = useState("");
  useEffect(() => { svc.statuses(pid).then((r) => setDefs(r.statuses)).catch((e) => setErr(errText(e))); }, [pid]);
  if (!defs) return err ? <Alert color="danger">{err}</Alert> : <CircularProgress />;
  const upd = (i: number, p: Partial<StatusDef>) => { setDefs(defs.map((d, j) => (j === i ? { ...d, ...p } : d))); setDirty(true); setOk(""); };
  const move = (i: number, d: number) => { const n = [...defs]; const [x] = n.splice(i, 1); n.splice(i + d, 0, x); setDefs(n); setDirty(true); };
  async function save() {
    setErr("");
    try { const r = await svc.saveStatuses(pid, defs!); setDefs(r.statuses); setDirty(false); setOk("Workflow saved."); } catch (e) { setErr(errText(e)); }
  }
  return (
    <SettingsSection title="Statuses and workflow" hint="Each status belongs to a category. Pending and on-hold statuses can pause the SLA clock. A status can require fields before a ticket may enter it. A reply from the requester reopens a solved ticket automatically."
      action={<Button onClick={save} disabled={!dirty} data-testid="wf-save">Save workflow</Button>}>
      <Sheet variant="outlined" sx={{ borderRadius: "lg", overflow: "hidden" }}>
        <Box sx={{ display: "grid", gridTemplateColumns: "1.1fr 1.4fr 1fr 110px 1.6fr 92px", gap: 1, px: 1.5, py: 1, bgcolor: "background.level1", fontSize: 12, color: "text.tertiary", fontWeight: 600 }}>
          <span>Key</span><span>Label</span><span>Category</span><span>Pauses SLA</span><span>Requires before entering</span><span />
        </Box>
        {defs.map((d, i) => (
          <Box key={i} sx={{ display: "grid", gridTemplateColumns: "1.1fr 1.4fr 1fr 110px 1.6fr 92px", gap: 1, px: 1.5, py: 1, alignItems: "center", borderTop: "1px solid", borderColor: "divider" }} data-testid={`wf-row-${i}`}>
            <Input size="sm" value={d.key} onChange={(e) => upd(i, { key: e.target.value })} />
            <Input size="sm" value={d.label} onChange={(e) => upd(i, { label: e.target.value })} />
            <Select size="sm" value={d.category} onChange={(_, v) => v && upd(i, { category: v as StatusDef["category"] })}>
              {["open", "pending", "hold", "solved", "closed"].map((c) => <Option key={c} value={c}>{c}</Option>)}
            </Select>
            <Switch checked={d.pauses_sla} onChange={(e) => upd(i, { pauses_sla: e.target.checked })} />
            <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap" }}>
              {["assignee", "category", "group"].map((f) => (
                <Checkbox key={f} size="sm" label={f} checked={d.requires?.includes(f)} onChange={(e) => upd(i, { requires: e.target.checked ? [...(d.requires ?? []), f] : (d.requires ?? []).filter((x) => x !== f) })} />
              ))}
            </Box>
            <Box sx={{ display: "flex" }}>
              <IconButton size="sm" variant="plain" disabled={i === 0} onClick={() => move(i, -1)} aria-label="Move up"><ArrowUpIcon fontSize="small" /></IconButton>
              <IconButton size="sm" variant="plain" disabled={i === defs.length - 1} onClick={() => move(i, 1)} aria-label="Move down"><ArrowDownIcon fontSize="small" /></IconButton>
              <IconButton size="sm" variant="plain" color="danger" onClick={() => { setDefs(defs.filter((_, j) => j !== i)); setDirty(true); }} aria-label="Remove"><DeleteIcon fontSize="small" /></IconButton>
            </Box>
          </Box>
        ))}
      </Sheet>
      <Button size="sm" variant="outlined" startDecorator={<AddIcon />} sx={{ mt: 1.5 }} onClick={() => { setDefs([...defs, { key: `status_${defs.length + 1}`, label: "New status", category: "open", pauses_sla: false, requires: [] }]); setDirty(true); }}>Add status</Button>
      <Saving err={err} ok={ok} />
    </SettingsSection>
  );
}

// ---------------------------------------------------------------- teams

function TeamsSection({ pid }: { pid: string }) {
  const [groups, reload, err] = useLoad(() => svc.groups(pid), [pid]);
  const [cals] = useLoad(() => svc.calendars(), []);
  const [edit, setEdit] = useState<Group | null>(null);
  return (
    <SettingsSection title="Teams and routing" hint="Tickets for a team are assigned automatically. Round robin takes turns, load-based picks the person with the most free capacity. People who are out of office or full are skipped."
      action={<Button startDecorator={<AddIcon />} onClick={() => setEdit({ id: "", name: "", description: "", strategy: "round_robin", calendar_id: "", email_alias: "", members: [] })} data-testid="new-group">New team</Button>}>
      {err && <Alert color="danger">{err}</Alert>}
      {groups && groups.length === 0 && <EmptyState illustration="generic" title="No teams yet" hint="Create a team such as IT Support and add the people who handle its tickets." />}
      <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "1fr 1fr" }, gap: 2 }}>
        {groups?.map((g) => (
          <Sheet key={g.id} variant="outlined" sx={{ borderRadius: "lg", p: 2 }} data-testid={`group-card-${g.name}`}>
            <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
              <Typography level="title-md" sx={{ flex: 1 }}>{g.name}</Typography>
              <Chip size="sm" variant="soft" color="primary">{g.strategy.replace("_", " ")}</Chip>
            </Box>
            <Typography level="body-sm" sx={{ color: "text.tertiary", mb: 1 }}>{g.description || "No description"} · {g.open_count ?? 0} open</Typography>
            {g.members.map((m) => (
              <Box key={m.user_id} sx={{ display: "flex", alignItems: "center", gap: 1, py: 0.4 }}>
                <Typography level="body-sm" sx={{ flex: 1 }}>{m.name}</Typography>
                {m.skills.slice(0, 2).map((s) => <Chip key={s} size="sm" variant="outlined">{s}</Chip>)}
                {m.ooo_until && new Date(m.ooo_until) > new Date() && <Chip size="sm" color="warning" variant="soft">out of office</Chip>}
                <Typography level="body-xs" sx={{ color: "text.tertiary", width: 52, textAlign: "right" }}>{m.open ?? 0}/{m.capacity}</Typography>
              </Box>
            ))}
            {g.email_alias && <Typography level="body-xs" sx={{ mt: 1, color: "text.tertiary" }}>Email-to-ticket address: {g.email_alias}</Typography>}
            <Box sx={{ display: "flex", gap: 1, mt: 1.5 }}>
              <Button size="sm" variant="outlined" onClick={() => setEdit(g)}>Edit</Button>
              <Button size="sm" variant="plain" color="danger" onClick={async () => { if (window.confirm(`Delete team "${g.name}"? Its tickets keep their other fields but lose the team.`)) { await svc.deleteGroup(g.id).catch(() => undefined); reload(); } }}>Delete</Button>
            </Box>
          </Sheet>
        ))}
      </Box>
      {edit && <GroupDialog pid={pid} g={edit} calendars={cals ?? []} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); reload(); }} />}
    </SettingsSection>
  );
}

function GroupDialog({ pid, g, calendars, onClose, onSaved }: { pid: string; g: Group; calendars: Calendar[]; onClose: () => void; onSaved: () => void }) {
  const [v, setV] = useState<Group>(g);
  const [q, setQ] = useState("");
  const [hits, setHits] = useState<Person[]>([]);
  const [err, setErr] = useState("");
  useEffect(() => { if (q.length < 1) return setHits([]); const t = setTimeout(() => peopleApi.search(q, 6).then(setHits).catch(() => setHits([])), 200); return () => clearTimeout(t); }, [q]);
  const setM = (i: number, p: Partial<Member>) => setV({ ...v, members: v.members.map((m, j) => (j === i ? { ...m, ...p } : m)) });
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: "min(640px, 96vw)", maxHeight: "92vh", overflow: "auto" }} data-testid="group-dialog">
        <DialogTitle>{g.id ? "Edit team" : "New team"}</DialogTitle>
        <DialogContent>
          <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1.5, mt: 1 }}>
            <FormControl size="sm"><FormLabel>Name</FormLabel><Input value={v.name} onChange={(e) => setV({ ...v, name: e.target.value })} slotProps={{ input: { "data-testid": "g-name" } as never }} /></FormControl>
            <FormControl size="sm"><FormLabel>Routing</FormLabel>
              <Select value={v.strategy} onChange={(_, s) => s && setV({ ...v, strategy: s as Group["strategy"] })}>
                <Option value="manual">Manual (leave unassigned)</Option><Option value="round_robin">Round robin</Option><Option value="load">Load based</Option>
              </Select></FormControl>
            <FormControl size="sm" sx={{ gridColumn: "1 / 3" }}><FormLabel>Description</FormLabel><Input value={v.description} onChange={(e) => setV({ ...v, description: e.target.value })} /></FormControl>
            <FormControl size="sm"><FormLabel>Business hours</FormLabel>
              <Select placeholder="Organisation default" value={v.calendar_id || null} onChange={(_, s) => setV({ ...v, calendar_id: s ?? "" })}><Option value="">Organisation default</Option>{calendars.map((c) => <Option key={c.id} value={c.id}>{c.name}</Option>)}</Select></FormControl>
            <FormControl size="sm"><FormLabel>Email-to-ticket address</FormLabel><Input placeholder="it-support@yourcompany.example" value={v.email_alias} onChange={(e) => setV({ ...v, email_alias: e.target.value })} /></FormControl>
          </Box>
          <Typography level="body-xs" sx={{ mt: 0.5, color: "text.tertiary" }}>Mail sent to this address becomes a ticket for this team and replies are added to the ticket. It needs a workspace mailbox that receives the address; the desk checks for new mail every few minutes.</Typography>
          <Typography level="title-sm" sx={{ mt: 2, mb: 0.5 }}>Members</Typography>
          {v.members.map((m, i) => (
            <Box key={m.user_id} sx={{ display: "grid", gridTemplateColumns: "1.1fr 1.2fr 70px 1.3fr 36px", gap: 1, alignItems: "center", mb: 0.75 }}>
              <Typography level="body-sm">{m.name || m.user_id.slice(0, 8)}</Typography>
              <Input size="sm" placeholder="skills: network, billing" value={m.skills.join(", ")} onChange={(e) => setM(i, { skills: e.target.value.split(",").map((s) => s.trim()).filter(Boolean) })} />
              <Input size="sm" type="number" value={m.capacity} onChange={(e) => setM(i, { capacity: Number(e.target.value) })} slotProps={{ input: { min: 0, max: 500 } }} />
              <Input size="sm" type="date" title="Out of office until" value={m.ooo_until ? m.ooo_until.slice(0, 10) : ""} onChange={(e) => setM(i, { ooo_until: e.target.value ? new Date(e.target.value + "T23:59:00").toISOString() : null })} slotProps={{ input: { "data-testid": `ooo-${i}` } as never }} />
              <IconButton size="sm" variant="plain" color="danger" onClick={() => setV({ ...v, members: v.members.filter((_, j) => j !== i) })}><DeleteIcon fontSize="small" /></IconButton>
            </Box>
          ))}
          <Typography level="body-xs" sx={{ color: "text.tertiary" }}>Columns: person, skills, capacity, out of office until. People who are away or full are skipped by routing.</Typography>
          <Input size="sm" placeholder="Add a person: type a name" value={q} onChange={(e) => setQ(e.target.value)} sx={{ mt: 1 }} />
          {hits.filter((h) => !v.members.some((m) => m.user_id === h.id)).map((h) => (
            <Button key={h.id} size="sm" variant="plain" sx={{ justifyContent: "flex-start" }} onClick={() => { setV({ ...v, members: [...v.members, { user_id: h.id, name: h.name, skills: [], capacity: 10 }] }); setQ(""); }}>{h.name} <span style={{ opacity: 0.5, marginLeft: 8 }}>{h.email}</span></Button>
          ))}
          <Typography level="body-xs" sx={{ mt: 1, color: "text.tertiary" }}>Capacity is the most open tickets a person should hold. Skills are matched by routing rules that name a skill.</Typography>
          {err && <Alert color="danger" size="sm" sx={{ mt: 1 }}>{err}</Alert>}
          <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1, mt: 2 }}>
            <Button variant="plain" color="neutral" onClick={onClose}>Cancel</Button>
            <Button onClick={async () => { try { await svc.saveGroup(pid, v); onSaved(); } catch (e) { setErr(errText(e)); } }} data-testid="g-save">Save team</Button>
          </Box>
        </DialogContent>
      </ModalDialog>
    </Modal>
  );
}

// ---------------------------------------------------------------- business hours

const DAYS = [["mon", "Monday"], ["tue", "Tuesday"], ["wed", "Wednesday"], ["thu", "Thursday"], ["fri", "Friday"], ["sat", "Saturday"], ["sun", "Sunday"]] as const;

function HoursSection() {
  const [cals, reload, err] = useLoad(() => svc.calendars(), []);
  const [edit, setEdit] = useState<Calendar | null>(null);
  return (
    <SettingsSection title="Business hours and holidays" hint="SLA clocks only run during business hours when a policy asks for it. Holidays are closed days. The default calendar applies to teams without their own."
      action={<Button startDecorator={<AddIcon />} onClick={() => setEdit({ id: "", name: "", tz: Intl.DateTimeFormat().resolvedOptions().timeZone || "Asia/Kolkata", is_default: !cals?.length, holidays: [], hours: { mon: [["09:00", "18:00"]], tue: [["09:00", "18:00"]], wed: [["09:00", "18:00"]], thu: [["09:00", "18:00"]], fri: [["09:00", "18:00"]] } })} data-testid="new-cal">New calendar</Button>}>
      {err && <Alert color="danger">{err}</Alert>}
      {cals?.length === 0 && <EmptyState illustration="calendar" title="Round the clock" hint="Without a calendar every SLA runs 24 hours a day. Add business hours to count only working time." />}
      {cals?.map((c) => (
        <Sheet key={c.id} variant="outlined" sx={{ borderRadius: "lg", p: 2, mb: 1.5 }} data-testid={`cal-${c.name}`}>
          <Box sx={{ display: "flex", gap: 1, alignItems: "center" }}>
            <Typography level="title-md">{c.name}</Typography>
            {c.is_default && <Chip size="sm" color="primary" variant="soft">default</Chip>}
            <Typography level="body-xs" sx={{ color: "text.tertiary" }}>{c.tz}</Typography>
            <Box sx={{ flex: 1 }} />
            <Button size="sm" variant="outlined" onClick={() => setEdit(c)}>Edit</Button>
            <Button size="sm" variant="plain" color="danger" onClick={async () => { if (window.confirm(`Delete "${c.name}"?`)) { await svc.deleteCalendar(c.id).catch(() => undefined); reload(); } }}>Delete</Button>
          </Box>
          <Box sx={{ display: "flex", gap: 1.5, mt: 1, flexWrap: "wrap" }}>
            {DAYS.map(([k, l]) => (
              <Box key={k} sx={{ minWidth: 92 }}>
                <Typography level="body-xs" sx={{ color: "text.tertiary" }}>{l.slice(0, 3)}</Typography>
                <Typography level="body-sm">{(c.hours[k] ?? []).map((w) => `${w[0]}–${w[1]}`).join(", ") || "Closed"}</Typography>
              </Box>
            ))}
          </Box>
          {c.holidays.length > 0 && <Typography level="body-xs" sx={{ mt: 1, color: "text.secondary" }}>Holidays: {c.holidays.slice(0, 6).map((h) => `${h.name || "Closed"} (${h.day})`).join(", ")}{c.holidays.length > 6 ? ` +${c.holidays.length - 6} more` : ""}</Typography>}
        </Sheet>
      ))}
      {edit && <CalendarDialog c={edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); reload(); }} />}
    </SettingsSection>
  );
}

function CalendarDialog({ c, onClose, onSaved }: { c: Calendar; onClose: () => void; onSaved: () => void }) {
  const [v, setV] = useState<Calendar>(c);
  const [hd, setHd] = useState("");
  const [hn, setHn] = useState("");
  const [err, setErr] = useState("");
  const setDay = (k: string, w: [string, string] | null) => { const h = { ...v.hours }; if (w) h[k] = [w]; else delete h[k]; setV({ ...v, hours: h }); };
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: "min(640px, 96vw)", maxHeight: "92vh", overflow: "auto" }} data-testid="cal-dialog">
        <DialogTitle>{c.id ? "Edit calendar" : "New calendar"}</DialogTitle>
        <DialogContent>
          <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1.5, mt: 1 }}>
            <FormControl size="sm"><FormLabel>Name</FormLabel><Input value={v.name} onChange={(e) => setV({ ...v, name: e.target.value })} slotProps={{ input: { "data-testid": "cal-name" } as never }} /></FormControl>
            <FormControl size="sm"><FormLabel>Time zone</FormLabel><Input value={v.tz} onChange={(e) => setV({ ...v, tz: e.target.value })} placeholder="Asia/Kolkata" /></FormControl>
          </Box>
          <Checkbox size="sm" sx={{ mt: 1 }} label="Use as the organisation default" checked={v.is_default} onChange={(e) => setV({ ...v, is_default: e.target.checked })} />
          <Typography level="title-sm" sx={{ mt: 2, mb: 0.5 }}>Working hours</Typography>
          {DAYS.map(([k, l]) => {
            const w = v.hours[k]?.[0];
            return (
              <Box key={k} sx={{ display: "grid", gridTemplateColumns: "110px 70px 1fr 1fr", gap: 1, alignItems: "center", mb: 0.5 }}>
                <Typography level="body-sm">{l}</Typography>
                <Switch size="sm" checked={!!w} onChange={(e) => setDay(k, e.target.checked ? ["09:00", "18:00"] : null)} />
                <Input size="sm" type="time" disabled={!w} value={w?.[0] ?? ""} onChange={(e) => w && setDay(k, [e.target.value, w[1]])} />
                <Input size="sm" type="time" disabled={!w} value={w?.[1] ?? ""} onChange={(e) => w && setDay(k, [w[0], e.target.value])} />
              </Box>
            );
          })}
          <Typography level="title-sm" sx={{ mt: 2, mb: 0.5 }}>Holidays (closed days)</Typography>
          <Box sx={{ display: "flex", gap: 1 }}>
            <Input size="sm" type="date" value={hd} onChange={(e) => setHd(e.target.value)} />
            <Input size="sm" placeholder="Name" value={hn} onChange={(e) => setHn(e.target.value)} sx={{ flex: 1 }} />
            <Button size="sm" variant="outlined" disabled={!hd} onClick={() => { setV({ ...v, holidays: [...v.holidays.filter((h) => h.day !== hd), { day: hd, name: hn }].sort((a, b) => a.day.localeCompare(b.day)) }); setHd(""); setHn(""); }}>Add</Button>
          </Box>
          <Box sx={{ display: "flex", gap: 0.75, flexWrap: "wrap", mt: 1 }}>
            {v.holidays.map((h) => <Chip key={h.day} size="sm" variant="soft" endDecorator={<IconButton size="sm" variant="plain" onClick={() => setV({ ...v, holidays: v.holidays.filter((x) => x.day !== h.day) })}>×</IconButton>}>{h.day} {h.name}</Chip>)}
          </Box>
          {err && <Alert color="danger" size="sm" sx={{ mt: 1 }}>{err}</Alert>}
          <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1, mt: 2 }}>
            <Button variant="plain" color="neutral" onClick={onClose}>Cancel</Button>
            <Button onClick={async () => { try { await svc.saveCalendar(v); onSaved(); } catch (e) { setErr(errText(e)); } }} data-testid="cal-save">Save calendar</Button>
          </Box>
        </DialogContent>
      </ModalDialog>
    </Modal>
  );
}

// ---------------------------------------------------------------- SLA

function SlaSection({ pid }: { pid: string }) {
  const [rules, reload, err] = useLoad(() => svc.slaRules(pid), [pid]);
  const [cals] = useLoad(() => svc.calendars(), []);
  const [edit, setEdit] = useState<SlaRule | null>(null);
  const [e2, setE2] = useState("");
  const blank: SlaRule = { id: "", name: "", position: 0, enabled: true, match_priority: "", match_type: "", match_customer: "", first_response_min: 60, resolve_min: 480, business_hours: true, calendar_id: "" };
  return (
    <SettingsSection title="SLA policies" hint="The first matching policy applies to a ticket. Match on priority, type or the customer's email domain. The clock pauses while a ticket is in a status that pauses SLA."
      action={<Button startDecorator={<AddIcon />} onClick={() => setEdit(blank)} data-testid="new-sla">New policy</Button>}>
      {(err || e2) && <Alert color="danger">{err || e2}</Alert>}
      {rules?.length === 0 && <EmptyState illustration="generic" title="No SLA policies" hint="Without a policy tickets have no response or resolution targets." />}
      {rules && rules.length > 0 && (
        <Sheet variant="outlined" sx={{ borderRadius: "lg", overflow: "hidden" }}>
          <Box sx={{ display: "grid", gridTemplateColumns: "28px 1.4fr 1.6fr 0.9fr 0.9fr 0.9fr 120px", gap: 1, px: 1.5, py: 1, bgcolor: "background.level1", fontSize: 12, fontWeight: 600, color: "text.tertiary" }}>
            <span>#</span><span>Policy</span><span>Applies to</span><span>First response</span><span>Resolution</span><span>Clock</span><span />
          </Box>
          {rules.map((r, i) => (
            <Box key={r.id} sx={{ display: "grid", gridTemplateColumns: "28px 1.4fr 1.6fr 0.9fr 0.9fr 0.9fr 120px", gap: 1, px: 1.5, py: 1.1, alignItems: "center", borderTop: "1px solid", borderColor: "divider", opacity: r.enabled ? 1 : 0.5 }} data-testid={`sla-row-${r.name}`}>
              <Typography level="body-xs">{i + 1}</Typography>
              <Typography level="title-sm">{r.name}</Typography>
              <Typography level="body-sm">{[r.match_priority && `priority ${r.match_priority}`, r.match_type && `type ${r.match_type}`, r.match_customer && `customer ${r.match_customer}`].filter(Boolean).join(", ") || "Every ticket"}</Typography>
              <Typography level="body-sm">{r.first_response_min ? fmtMin(r.first_response_min) : "no target"}</Typography>
              <Typography level="body-sm">{r.resolve_min ? fmtMin(r.resolve_min) : "no target"}</Typography>
              <Chip size="sm" variant="soft" color={r.business_hours ? "primary" : "neutral"}>{r.business_hours ? "Business hours" : "24 hours"}</Chip>
              <Box sx={{ display: "flex", gap: 0.5 }}>
                <Button size="sm" variant="plain" onClick={() => setEdit(r)}>Edit</Button>
                <IconButton size="sm" variant="plain" color="danger" aria-label={`Delete ${r.name}`} onClick={async () => { if (window.confirm(`Delete policy "${r.name}"?`)) { await svc.deleteSlaRule(r.id).catch((x) => setE2(errText(x))); reload(); } }}><DeleteIcon fontSize="small" /></IconButton>
              </Box>
            </Box>
          ))}
        </Sheet>
      )}
      {edit && (
        <Modal open onClose={() => setEdit(null)}>
          <ModalDialog sx={{ width: "min(560px, 96vw)" }} data-testid="sla-dialog">
            <DialogTitle>{edit.id ? "Edit SLA policy" : "New SLA policy"}</DialogTitle>
            <DialogContent>
              <SlaForm v={edit} calendars={cals ?? []} onCancel={() => setEdit(null)} onSave={async (v) => { await svc.saveSlaRule(pid, v); setEdit(null); reload(); }} />
            </DialogContent>
          </ModalDialog>
        </Modal>
      )}
    </SettingsSection>
  );
}

function SlaForm({ v: init, calendars, onSave, onCancel }: { v: SlaRule; calendars: Calendar[]; onSave: (v: SlaRule) => Promise<void>; onCancel: () => void }) {
  const [v, setV] = useState(init);
  const [err, setErr] = useState("");
  const num = (s: string) => Math.max(0, Math.floor(Number(s) || 0));
  return (
    <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1.5, mt: 1 }}>
      <FormControl size="sm" sx={{ gridColumn: "1 / 3" }}><FormLabel>Name</FormLabel><Input value={v.name} onChange={(e) => setV({ ...v, name: e.target.value })} slotProps={{ input: { "data-testid": "sla-name" } as never }} /></FormControl>
      <FormControl size="sm"><FormLabel>Priority</FormLabel><Select placeholder="Any" value={v.match_priority || null} onChange={(_, s) => setV({ ...v, match_priority: s ?? "" })}><Option value="">Any</Option>{PRIORITIES.map((p) => <Option key={p} value={p}>{p}</Option>)}</Select></FormControl>
      <FormControl size="sm"><FormLabel>Ticket type</FormLabel><Select placeholder="Any" value={v.match_type || null} onChange={(_, s) => setV({ ...v, match_type: s ?? "" })}><Option value="">Any</Option>{TICKET_TYPES.map((p) => <Option key={p} value={p}>{p}</Option>)}</Select></FormControl>
      <FormControl size="sm" sx={{ gridColumn: "1 / 3" }}><FormLabel>Customer (email domain)</FormLabel><Input placeholder="acme.example" value={v.match_customer} onChange={(e) => setV({ ...v, match_customer: e.target.value })} /></FormControl>
      <FormControl size="sm"><FormLabel>First response (minutes)</FormLabel><Input type="number" value={v.first_response_min} onChange={(e) => setV({ ...v, first_response_min: num(e.target.value) })} slotProps={{ input: { "data-testid": "sla-fr" } as never }} /></FormControl>
      <FormControl size="sm"><FormLabel>Resolution (minutes)</FormLabel><Input type="number" value={v.resolve_min} onChange={(e) => setV({ ...v, resolve_min: num(e.target.value) })} slotProps={{ input: { "data-testid": "sla-res" } as never }} /></FormControl>
      <Box sx={{ gridColumn: "1 / 3", display: "flex", gap: 2, alignItems: "center" }}>
        <Switch checked={v.business_hours} onChange={(e) => setV({ ...v, business_hours: e.target.checked })} endDecorator="Count business hours only" />
        {v.business_hours && <Select size="sm" placeholder="Team or default calendar" value={v.calendar_id || null} onChange={(_, s) => setV({ ...v, calendar_id: s ?? "" })} sx={{ minWidth: 200 }}><Option value="">Team or default calendar</Option>{calendars.map((c) => <Option key={c.id} value={c.id}>{c.name}</Option>)}</Select>}
      </Box>
      <Switch checked={v.enabled} onChange={(e) => setV({ ...v, enabled: e.target.checked })} endDecorator="Enabled" />
      {err && <Alert color="danger" size="sm" sx={{ gridColumn: "1 / 3" }}>{err}</Alert>}
      <Box sx={{ gridColumn: "1 / 3", display: "flex", justifyContent: "flex-end", gap: 1 }}>
        <Button variant="plain" color="neutral" onClick={onCancel}>Cancel</Button>
        <Button onClick={async () => { try { await onSave(v); } catch (e) { setErr(errText(e)); } }} data-testid="sla-save">Save policy</Button>
      </Box>
    </Box>
  );
}

// ---------------------------------------------------------------- automations

const OPS: Record<string, string> = { is: "is", is_not: "is not", contains: "contains", not_contains: "does not contain", starts_with: "starts with", in: "is one of", empty: "is empty", not_empty: "is not empty", gt: "is more than", lt: "is less than" };
const ACTION_LABEL: Record<string, string> = { set_status: "Set status", set_priority: "Set priority", set_type: "Set type", set_group: "Move to team", set_assignee: "Assign to", set_category: "Set category", require_skill: "Require skill", add_tag: "Add tag", remove_tag: "Remove tag", mark_major: "Mark as major incident", notify: "Send notification", webhook: "Call webhook", create_task: "Create a task", add_note: "Add internal note" };
const FIELD_LABEL: Record<string, string> = { hours_since_created: "Hours since created", hours_since_update: "Hours since last update", requester_domain: "Requester email domain", requester_email: "Requester email" };

function AutomationsSection({ pid }: { pid: string }) {
  const [data, reload, err] = useLoad(() => svc.automations(pid), [pid]);
  const [groups] = useLoad(() => svc.groups(pid), [pid]);
  const [defs] = useLoad(() => svc.statuses(pid), [pid]);
  const [edit, setEdit] = useState<Automation | null>(null);
  const blank: Automation = { id: "", name: "", kind: "trigger", event: "create", match_mode: "all", conditions: [{ field: "priority", op: "is", value: "urgent" }], actions: [{ type: "add_tag", value: "" }], position: 0, enabled: true, run_count: 0 };
  return (
    <SettingsSection title="Automations" hint="Rules that run when a ticket is created or updated, or on a schedule for tickets that match. Dispatch rules route new tickets. Every rule can be tested against a real ticket before you save it."
      action={<Button startDecorator={<AddIcon />} onClick={() => setEdit(blank)} data-testid="new-auto">New automation</Button>}>
      {err && <Alert color="danger">{err}</Alert>}
      {data?.automations.length === 0 && <EmptyState illustration="tasks" title="No automations" hint="Start with a rule such as: when priority is urgent and type is incident, notify the team." />}
      <Box sx={{ display: "flex", flexDirection: "column", gap: 1.25 }}>
        {data?.automations.map(({ rule, description }) => (
          <Sheet key={rule.id} variant="outlined" sx={{ borderRadius: "lg", p: 1.75, display: "flex", gap: 1.5, alignItems: "center", opacity: rule.enabled ? 1 : 0.55 }} data-testid={`auto-${rule.name}`}>
            <Switch checked={rule.enabled} onChange={async (e) => { await svc.saveAutomation(pid, { ...rule, enabled: e.target.checked }).catch(() => undefined); reload(); }} aria-label={`Enable ${rule.name}`} />
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <Box sx={{ display: "flex", gap: 1, alignItems: "center" }}>
                <Typography level="title-sm">{rule.name}</Typography>
                <Chip size="sm" variant="soft" color={rule.kind === "time" ? "warning" : rule.kind === "dispatch" ? "primary" : "neutral"}>{rule.kind === "time" ? "time based" : rule.kind}</Chip>
              </Box>
              <Typography level="body-xs" sx={{ color: "text.secondary" }}>{description}</Typography>
              <Typography level="body-xs" sx={{ color: "text.tertiary" }}>Ran {rule.run_count} time{rule.run_count === 1 ? "" : "s"}{rule.last_run_at ? `, last ${new Date(rule.last_run_at).toLocaleString()}` : ""}</Typography>
            </Box>
            <Button size="sm" variant="outlined" onClick={() => setEdit(rule)}>Edit</Button>
            <IconButton size="sm" variant="plain" color="danger" aria-label={`Delete ${rule.name}`} onClick={async () => { if (window.confirm(`Delete "${rule.name}"?`)) { await svc.deleteAutomation(rule.id).catch(() => undefined); reload(); } }}><DeleteIcon fontSize="small" /></IconButton>
          </Sheet>
        ))}
      </Box>
      {edit && data && <AutomationBuilder pid={pid} a={edit} fields={data.condition_fields} actionTypes={data.action_types} groups={groups ?? []} statuses={defs?.statuses ?? []} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); reload(); }} />}
    </SettingsSection>
  );
}

function AutomationBuilder({ pid, a, fields, actionTypes, groups, statuses, onClose, onSaved }: { pid: string; a: Automation; fields: Record<string, string>; actionTypes: string[]; groups: { id: string; name: string }[]; statuses: StatusDef[]; onClose: () => void; onSaved: () => void }) {
  const [v, setV] = useState<Automation>(a);
  const [err, setErr] = useState("");
  const [ref, setRef] = useState("");
  const [test, setTest] = useState<AutoTest | null>(null);
  const [testErr, setTestErr] = useState("");
  const fieldNames = useMemo(() => Object.keys(fields), [fields]);
  const setC = (i: number, p: Partial<Automation["conditions"][number]>) => setV({ ...v, conditions: v.conditions.map((c, j) => (j === i ? { ...c, ...p } : c)) });
  const setA = (i: number, p: Partial<Automation["actions"][number]>) => setV({ ...v, actions: v.actions.map((c, j) => (j === i ? { ...c, ...p } : c)) });
  const valueInput = (type: string, value: string, on: (s: string) => void) => {
    if (type === "set_status") return <Select size="sm" value={value || null} onChange={(_, s) => on(s ?? "")} placeholder="status">{statuses.map((s) => <Option key={s.key} value={s.key}>{s.label}</Option>)}</Select>;
    if (type === "set_priority") return <Select size="sm" value={value || null} onChange={(_, s) => on(s ?? "")} placeholder="priority">{PRIORITIES.map((s) => <Option key={s} value={s}>{s}</Option>)}</Select>;
    if (type === "set_type") return <Select size="sm" value={value || null} onChange={(_, s) => on(s ?? "")} placeholder="type">{TICKET_TYPES.map((s) => <Option key={s} value={s}>{s}</Option>)}</Select>;
    if (type === "set_group") return <Select size="sm" value={value || null} onChange={(_, s) => on(s ?? "")} placeholder="team">{groups.map((s) => <Option key={s.id} value={s.id}>{s.name}</Option>)}</Select>;
    if (type === "mark_major") return <Typography level="body-xs" sx={{ color: "text.tertiary" }}>No value needed</Typography>;
    if (type === "notify") return <Input size="sm" placeholder="assignee, group or a user id" value={value} onChange={(e) => on(e.target.value)} />;
    if (type === "webhook") return <Input size="sm" placeholder="https://…" value={value} onChange={(e) => on(e.target.value)} />;
    return <Input size="sm" value={value} onChange={(e) => on(e.target.value)} placeholder={type === "set_assignee" ? "user id (empty clears)" : "value"} />;
  };
  async function runTest() {
    setTest(null); setTestErr("");
    try {
      const r = await svc.list(pid, { view: "all", q: ref.trim(), limit: 1 });
      if (!r.tickets[0]) return setTestErr("No ticket found for that search.");
      setTest(await svc.testAutomation(pid, v, r.tickets[0].id));
    } catch (e) { setTestErr(errText(e)); }
  }
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: "min(780px, 97vw)", maxHeight: "94vh", overflow: "auto" }} data-testid="auto-dialog">
        <DialogTitle>{a.id ? "Edit automation" : "New automation"}</DialogTitle>
        <DialogContent>
          <Box sx={{ display: "grid", gridTemplateColumns: "1.6fr 1fr 1fr", gap: 1.5, mt: 1 }}>
            <FormControl size="sm"><FormLabel>Name</FormLabel><Input value={v.name} onChange={(e) => setV({ ...v, name: e.target.value })} slotProps={{ input: { "data-testid": "auto-name" } as never }} /></FormControl>
            <FormControl size="sm"><FormLabel>Runs</FormLabel>
              <Select value={v.kind} onChange={(_, s) => s && setV({ ...v, kind: s as Automation["kind"] })}>
                <Option value="trigger">When a ticket changes</Option><Option value="dispatch">Dispatch (new tickets)</Option><Option value="time">On a schedule</Option>
              </Select></FormControl>
            {v.kind === "trigger" && <FormControl size="sm"><FormLabel>Event</FormLabel><Select value={v.event} onChange={(_, s) => s && setV({ ...v, event: s as Automation["event"] })}><Option value="create">Created</Option><Option value="update">Updated</Option><Option value="any">Created or updated</Option></Select></FormControl>}
          </Box>

          <Sheet variant="soft" sx={{ borderRadius: "md", p: 1.5, mt: 2 }}>
            <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1 }}>
              <Typography level="title-sm">If</Typography>
              <Select size="sm" value={v.match_mode} onChange={(_, s) => s && setV({ ...v, match_mode: s as "all" | "any" })} sx={{ width: 150 }}><Option value="all">all conditions match</Option><Option value="any">any condition matches</Option></Select>
            </Box>
            {v.conditions.map((c, i) => (
              <Box key={i} sx={{ display: "grid", gridTemplateColumns: "1.2fr 1fr 1.4fr 36px", gap: 1, mb: 0.75 }} data-testid={`cond-${i}`}>
                <Select size="sm" value={c.field} onChange={(_, s) => s && setC(i, { field: s })}>{fieldNames.map((f) => <Option key={f} value={f}>{FIELD_LABEL[f] ?? f.replace(/_/g, " ")}</Option>)}</Select>
                <Select size="sm" value={c.op} onChange={(_, s) => s && setC(i, { op: s })}>{Object.entries(OPS).filter(([k]) => (k !== "gt" && k !== "lt") || fields[c.field] === "number").map(([k, l]) => <Option key={k} value={k}>{l}</Option>)}</Select>
                <Input size="sm" disabled={c.op === "empty" || c.op === "not_empty"} value={c.value} onChange={(e) => setC(i, { value: e.target.value })} />
                <IconButton size="sm" variant="plain" color="danger" onClick={() => setV({ ...v, conditions: v.conditions.filter((_, j) => j !== i) })}><DeleteIcon fontSize="small" /></IconButton>
              </Box>
            ))}
            <Button size="sm" variant="plain" startDecorator={<AddIcon />} onClick={() => setV({ ...v, conditions: [...v.conditions, { field: "priority", op: "is", value: "" }] })}>Add condition</Button>
          </Sheet>
          <Sheet variant="soft" color="primary" sx={{ borderRadius: "md", p: 1.5, mt: 1.5 }}>
            <Typography level="title-sm" sx={{ mb: 1 }}>Then</Typography>
            {v.actions.map((x, i) => (
              <Box key={i} sx={{ display: "grid", gridTemplateColumns: "1.2fr 2fr 36px", gap: 1, mb: 0.75 }} data-testid={`action-${i}`}>
                <Select size="sm" value={x.type} onChange={(_, s) => s && setA(i, { type: s, value: "" })}>{actionTypes.map((t) => <Option key={t} value={t}>{ACTION_LABEL[t] ?? t}</Option>)}</Select>
                {valueInput(x.type, x.value, (s) => setA(i, { value: s }))}
                <IconButton size="sm" variant="plain" color="danger" onClick={() => setV({ ...v, actions: v.actions.filter((_, j) => j !== i) })}><DeleteIcon fontSize="small" /></IconButton>
              </Box>
            ))}
            <Button size="sm" variant="plain" startDecorator={<AddIcon />} onClick={() => setV({ ...v, actions: [...v.actions, { type: "add_tag", value: "" }] })}>Add action</Button>
          </Sheet>

          <Sheet variant="outlined" sx={{ borderRadius: "md", p: 1.5, mt: 1.5 }}>
            <Typography level="title-sm">Test this rule</Typography>
            <Typography level="body-xs" sx={{ color: "text.tertiary", mb: 1 }}>Checks the rule against an existing ticket and shows what would change. Nothing is saved or sent.</Typography>
            <Box sx={{ display: "flex", gap: 1 }}>
              <Input size="sm" placeholder="Ticket number or words from the title, e.g. #12 or vpn" value={ref} onChange={(e) => setRef(e.target.value)} sx={{ flex: 1 }} slotProps={{ input: { "data-testid": "auto-test-ref" } as never }} />
              <Button size="sm" variant="outlined" disabled={!ref.trim()} onClick={runTest} data-testid="auto-test-run">Run test</Button>
            </Box>
            {testErr && <Alert color="danger" size="sm" sx={{ mt: 1 }}>{testErr}</Alert>}
            {test && (
              <Box sx={{ mt: 1 }} data-testid="auto-test-result">
                <Chip size="sm" color={test.matched ? "success" : "neutral"} variant="soft">{test.matched ? "Rule would run" : "Rule would not run"}</Chip>
                {test.conditions.map((c, i) => <Typography key={i} level="body-xs">{c.pass ? "✓" : "✗"} {c.field} {OPS[c.op] ?? c.op} “{c.value}” (ticket has “{c.actual}”)</Typography>)}
                {test.changes.map((c, i) => <Typography key={i} level="body-xs" sx={{ color: "success.700" }}>→ {c.field}: {c.from || "empty"} → {c.to || "empty"}</Typography>)}
                {test.effects.map((c, i) => <Typography key={i} level="body-xs" sx={{ color: "primary.700" }}>→ {ACTION_LABEL[c.type] ?? c.type}: {c.value}</Typography>)}
              </Box>
            )}
          </Sheet>
          {err && <Alert color="danger" size="sm" sx={{ mt: 1 }}>{err}</Alert>}
          <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1, mt: 2 }}>
            <Button variant="plain" color="neutral" onClick={onClose}>Cancel</Button>
            <Button onClick={async () => { try { await svc.saveAutomation(pid, v); onSaved(); } catch (e) { setErr(errText(e)); } }} data-testid="auto-save">Save automation</Button>
          </Box>
        </DialogContent>
      </ModalDialog>
    </Modal>
  );
}

// ---------------------------------------------------------------- forms

function FormsSection({ pid }: { pid: string }) {
  const [forms, reload, err] = useLoad(() => svc.forms(pid), [pid]);
  const [groups] = useLoad(() => svc.groups(pid), [pid]);
  const [edit, setEdit] = useState<RequestForm | null>(null);
  return (
    <SettingsSection title="Request forms" hint="Forms shown on the requester portal. Each form sets the ticket type, team and category, and asks its own questions."
      action={<Button startDecorator={<AddIcon />} onClick={() => setEdit({ id: "", name: "", description: "", ticket_type: "request", group_id: "", category: "", fields: [{ key: "details", label: "Details", type: "longtext", required: true }], enabled: true })} data-testid="new-form">New form</Button>}>
      {err && <Alert color="danger">{err}</Alert>}
      {forms?.length === 0 && <EmptyState illustration="generic" title="No forms yet" hint="Without forms the portal shows a single generic request box." />}
      <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "1fr 1fr" }, gap: 1.5 }}>
        {forms?.map((f) => (
          <Sheet key={f.id} variant="outlined" sx={{ borderRadius: "lg", p: 2, opacity: f.enabled ? 1 : 0.55 }} data-testid={`form-${f.name}`}>
            <Box sx={{ display: "flex", gap: 1, alignItems: "center" }}><Typography level="title-md" sx={{ flex: 1 }}>{f.name}</Typography><Chip size="sm" variant="soft">{f.ticket_type}</Chip></Box>
            <Typography level="body-sm" sx={{ color: "text.tertiary" }}>{f.description || "No description"}</Typography>
            <Typography level="body-xs" sx={{ mt: 0.75 }}>{f.fields.length} question{f.fields.length === 1 ? "" : "s"} · team {groups?.find((g) => g.id === f.group_id)?.name ?? "none"}{f.category ? ` · ${f.category}` : ""}</Typography>
            <Box sx={{ display: "flex", gap: 1, mt: 1.25 }}>
              <Button size="sm" variant="outlined" onClick={() => setEdit(f)}>Edit</Button>
              <Button size="sm" variant="plain" color="danger" onClick={async () => { if (window.confirm(`Delete form "${f.name}"?`)) { await svc.deleteForm(f.id).catch(() => undefined); reload(); } }}>Delete</Button>
            </Box>
          </Sheet>
        ))}
      </Box>
      {edit && <FormDialog pid={pid} f={edit} groups={groups ?? []} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); reload(); }} />}
    </SettingsSection>
  );
}

function FormDialog({ pid, f, groups, onClose, onSaved }: { pid: string; f: RequestForm; groups: Group[]; onClose: () => void; onSaved: () => void }) {
  const [v, setV] = useState(f);
  const [err, setErr] = useState("");
  const setF = (i: number, p: Partial<FormField>) => setV({ ...v, fields: v.fields.map((x, j) => (j === i ? { ...x, ...p } : x)) });
  const keyOf = useCallback((s: string) => s.toLowerCase().replace(/[^a-z0-9]+/g, "_").replace(/^_+|_+$/g, "").slice(0, 32), []);
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: "min(720px, 97vw)", maxHeight: "94vh", overflow: "auto" }} data-testid="form-dialog">
        <DialogTitle>{f.id ? "Edit form" : "New form"}</DialogTitle>
        <DialogContent>
          <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1.5, mt: 1 }}>
            <FormControl size="sm"><FormLabel>Form name</FormLabel><Input value={v.name} onChange={(e) => setV({ ...v, name: e.target.value })} slotProps={{ input: { "data-testid": "form-name" } as never }} /></FormControl>
            <FormControl size="sm"><FormLabel>Ticket type</FormLabel><Select value={v.ticket_type} onChange={(_, s) => s && setV({ ...v, ticket_type: s as never })}>{TICKET_TYPES.map((t) => <Option key={t} value={t}>{t}</Option>)}</Select></FormControl>
            <FormControl size="sm" sx={{ gridColumn: "1 / 3" }}><FormLabel>Description shown to the requester</FormLabel><Input value={v.description} onChange={(e) => setV({ ...v, description: e.target.value })} /></FormControl>
            <FormControl size="sm"><FormLabel>Team</FormLabel><Select placeholder="No team" value={v.group_id || null} onChange={(_, s) => setV({ ...v, group_id: s ?? "" })}><Option value="">No team</Option>{groups.map((g) => <Option key={g.id} value={g.id}>{g.name}</Option>)}</Select></FormControl>
            <FormControl size="sm"><FormLabel>Category</FormLabel><Input value={v.category} onChange={(e) => setV({ ...v, category: e.target.value })} /></FormControl>
            <FormControl size="sm" sx={{ gridColumn: "1 / 3" }}><FormLabel>Skills the assignee needs (comma separated)</FormLabel><Input value={(v.skills ?? []).join(", ")} onChange={(e) => setV({ ...v, skills: e.target.value.split(",").map((s) => s.trim()).filter(Boolean) })} placeholder="network, hardware" /></FormControl>
          </Box>
          <Typography level="title-sm" sx={{ mt: 2, mb: 0.75 }}>Questions</Typography>
          {v.fields.map((x, i) => (
            <Sheet key={i} variant="soft" sx={{ borderRadius: "md", p: 1.25, mb: 1, display: "grid", gridTemplateColumns: "1.6fr 1fr 80px 36px", gap: 1, alignItems: "center" }}>
              <Input size="sm" placeholder="Question" value={x.label} onChange={(e) => setF(i, { label: e.target.value, key: x.key && x.key !== keyOf(x.label) ? x.key : keyOf(e.target.value) })} />
              <Select size="sm" value={x.type} onChange={(_, s) => s && setF(i, { type: s as FormField["type"] })}>{["text", "longtext", "select", "date", "checkbox"].map((t) => <Option key={t} value={t}>{t}</Option>)}</Select>
              <Checkbox size="sm" label="Required" checked={x.required} onChange={(e) => setF(i, { required: e.target.checked })} />
              <IconButton size="sm" variant="plain" color="danger" onClick={() => setV({ ...v, fields: v.fields.filter((_, j) => j !== i) })}><DeleteIcon fontSize="small" /></IconButton>
              {x.type === "select" && <Textarea sx={{ gridColumn: "1 / 5" }} minRows={2} placeholder="One option per line" value={(x.options ?? []).join("\n")} onChange={(e) => setF(i, { options: e.target.value.split("\n").map((s) => s.trim()).filter(Boolean) })} />}
            </Sheet>
          ))}
          <Button size="sm" variant="outlined" startDecorator={<AddIcon />} onClick={() => setV({ ...v, fields: [...v.fields, { key: `field_${v.fields.length + 1}`, label: "", type: "text", required: false }] })}>Add question</Button>
          <Switch sx={{ mt: 1.5 }} checked={v.enabled} onChange={(e) => setV({ ...v, enabled: e.target.checked })} endDecorator="Shown on the portal" />
          {err && <Alert color="danger" size="sm" sx={{ mt: 1 }}>{err}</Alert>}
          <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1, mt: 2 }}>
            <Button variant="plain" color="neutral" onClick={onClose}>Cancel</Button>
            <Button onClick={async () => { try { await svc.saveForm(pid, v); onSaved(); } catch (e) { setErr(errText(e)); } }} data-testid="form-save">Save form</Button>
          </Box>
        </DialogContent>
      </ModalDialog>
    </Modal>
  );
}
