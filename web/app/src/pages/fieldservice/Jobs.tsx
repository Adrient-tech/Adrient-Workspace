import { PeopleSelect } from "../../components/PeoplePicker";
import { useCallback, useEffect, useState } from "react";
import { Alert, Box, Button, Checkbox, Chip, FormControl, FormLabel, Input, Option, Select, Table, Textarea, Typography } from "@mui/joy";
import { Card, Dialog, ErrorAlert, Field, Row, useErr } from "../hr/ui";
import { DAY_SHORT, fmtDay, fmtRange, money, todayISO } from "../ops/dates";
import { fs } from "./api";
import type { DocRef, Job, Schedule, Service, Settings, Team, Visit } from "./api";
import { STATUS_COLOR, STATUS_LABEL, blankSchedule, describeSchedule } from "./logic";
import { CustomerPicker, ItemsEditor, PropertyPicker } from "./parts";

export function ScheduleEditor({ s, onChange }: { s: Schedule; onChange: (s: Schedule) => void }) {
  const set = (p: Partial<Schedule>) => onChange({ ...s, ...p });
  return (
    <Box>
      <Row>
        <Field label="First visit" sx={{ maxWidth: 190 }}><Input type="date" value={s.start} onChange={(e) => set({ start: e.target.value })} slotProps={{ input: { "aria-label": "First visit date" } }} /></Field>
        <Field label="Starts" sx={{ maxWidth: 140 }}><Input type="time" value={s.start_time} onChange={(e) => set({ start_time: e.target.value })} slotProps={{ input: { "aria-label": "Visit start time" } }} /></Field>
        <Field label="Ends" sx={{ maxWidth: 140 }}><Input type="time" value={s.end_time} onChange={(e) => set({ end_time: e.target.value })} slotProps={{ input: { "aria-label": "Visit end time" } }} /></Field>
        <Field label="Repeats" sx={{ maxWidth: 170 }}>
          <Select value={s.repeat || "none"} onChange={(_, v) => set({ repeat: v ?? "none" })} slotProps={{ button: { "aria-label": "Repeat" } }}>
            <Option value="none">One-off</Option><Option value="daily">Daily</Option><Option value="weekly">Weekly</Option><Option value="monthly">Monthly</Option><Option value="yearly">Yearly</Option>
          </Select>
        </Field>
      </Row>
      {s.repeat && s.repeat !== "none" && (
        <Box sx={{ mt: 1 }}>
          <Row sx={{ alignItems: "flex-end" }}>
            <Field label="Every" sx={{ maxWidth: 90 }}><Input type="number" value={s.interval} onChange={(e) => set({ interval: Number(e.target.value) })} slotProps={{ input: { "aria-label": "Repeat interval", min: 1 } }} /></Field>
            <Field label="Ends after" sx={{ maxWidth: 190 }}>
              <Select value={s.end_type} onChange={(_, v) => set({ end_type: v ?? "count" })} slotProps={{ button: { "aria-label": "End type" } }}><Option value="count">A number of visits</Option><Option value="until">A date</Option></Select>
            </Field>
            {s.end_type === "until" ? <Field label="Until" sx={{ maxWidth: 190 }}><Input type="date" value={s.end_date} onChange={(e) => set({ end_date: e.target.value })} slotProps={{ input: { "aria-label": "Repeat until" } }} /></Field>
              : <Field label="Visits" sx={{ maxWidth: 100 }}><Input type="number" value={s.end_count} onChange={(e) => set({ end_count: Number(e.target.value) })} slotProps={{ input: { "aria-label": "Number of visits", min: 1, max: 400 } }} /></Field>}
          </Row>
          {s.repeat === "weekly" && <Row sx={{ mt: 1 }}>{DAY_SHORT.map((d, i) => <Checkbox key={d} size="sm" label={d} checked={s.weekdays.includes(i)} onChange={(e) => set({ weekdays: e.target.checked ? [...s.weekdays, i] : s.weekdays.filter((x) => x !== i) })} />)}</Row>}
          {s.repeat === "monthly" && <Row sx={{ mt: 1 }}><Select value={s.monthly_by || "date"} onChange={(_, v) => set({ monthly_by: v ?? "date" })} slotProps={{ button: { "aria-label": "Monthly by" } }}><Option value="date">Same day of the month</Option><Option value="weekday">Same weekday (e.g. 2nd Tuesday)</Option></Select></Row>}
        </Box>
      )}
      <Typography level="body-xs" sx={{ mt: 0.5 }} data-testid="schedule-summary">{describeSchedule(s)}</Typography>
    </Box>
  );
}

export function TeamPicker({ team, value, onChange }: { team: Team[]; value: string[]; onChange: (v: string[]) => void }) {
  return (
    <FormControl>
      <FormLabel>Assigned to</FormLabel>
      <PeopleSelect value={value} known={team} onChange={(ids) => onChange(ids)} placeholder="Nobody yet" ariaLabel="Person" />
    </FormControl>
  );
}

export function Jobs({ settings, services, team, openId, onOpened }: { settings: Settings; services: Service[]; team: Team[]; openId: string; onOpened: () => void }) {
  const { err, run } = useErr();
  const [list, setList] = useState<Job[]>([]);
  const [filter, setFilter] = useState("");
  const [edit, setEdit] = useState<Partial<Job> | null>(null);
  const [view, setView] = useState("");
  const load = useCallback(() => void run(async () => setList(await fs.jobs({ status: filter }))), [run, filter]);
  useEffect(load, [load]);
  useEffect(() => { if (openId) { setView(openId); onOpened(); } }, [openId, onOpened]);
  return (
    <Card title="Jobs" action={<Button size="sm" onClick={() => setEdit({ title: "", items: [], tax_rate: settings.tax_rate, billing: settings.default_billing, schedule: blankSchedule(todayISO()), assignees: [], discount: null })}>New job</Button>}>
      <ErrorAlert err={err} />
      <Row sx={{ mb: 1 }}>
        {["", "active", "late", "requires_invoicing", "completed", "cancelled"].map((s) => <Chip key={s} variant={filter === s ? "solid" : "outlined"} onClick={() => setFilter(s)} data-testid={`job-filter-${s || "all"}`}>{s ? STATUS_LABEL[s] : "All"}</Chip>)}
      </Row>
      <Box sx={{ overflowX: "auto" }}>
        <Table size="sm" aria-label="Jobs">
          <thead><tr><th>Job</th><th>Customer</th><th>Schedule</th><th>Total</th><th>Status</th><th /></tr></thead>
          <tbody>
            {list.map((j) => (
              <tr key={j.id}>
                <td>{j.title}</td><td>{j.client_name}</td><td>{describeSchedule(j.schedule)}<Typography level="body-xs">from {fmtDay(j.schedule.start)}</Typography></td>
                <td>{money(j.totals.total, settings.currency)}</td>
                <td><Chip size="sm" color={STATUS_COLOR[j.display_status]} variant="soft">{STATUS_LABEL[j.display_status] ?? j.display_status}</Chip></td>
                <td><Button size="sm" variant="outlined" onClick={() => setView(j.id)}>Open</Button></td>
              </tr>
            ))}
          </tbody>
        </Table>
      </Box>
      {!list.length && <Typography level="body-sm" sx={{ mt: 1 }}>No jobs here. Create one, or turn a request into a job.</Typography>}
      {edit && <JobDialog j={edit} settings={settings} services={services} team={team} onClose={() => setEdit(null)} onSaved={(id) => { setEdit(null); load(); setView(id); }} />}
      {view && <JobDetail id={view} settings={settings} services={services} team={team} onClose={() => { setView(""); load(); }} />}
    </Card>
  );
}

function JobDialog({ j, settings, services, team, onClose, onSaved }: { j: Partial<Job>; settings: Settings; services: Service[]; team: Team[]; onClose: () => void; onSaved: (id: string) => void }) {
  const { err, run } = useErr();
  const [v, setV] = useState<Partial<Job>>(j);
  return (
    <Dialog open onClose={onClose} title={j.id ? "Edit job" : "New job"} wide>
      <ErrorAlert err={err} />
      <CustomerPicker value={v.contact_id ?? ""} name={v.client_name ?? ""} onPick={(c) => setV({ ...v, contact_id: c.id, client_name: c.name, property_id: "" })} />
      <Box sx={{ mt: 1 }}><PropertyPicker contact={v.contact_id ?? ""} value={v.property_id ?? ""} onPick={(id) => setV({ ...v, property_id: id })} /></Box>
      <Field label="Title" sx={{ mt: 1 }}><Input value={v.title ?? ""} onChange={(e) => setV({ ...v, title: e.target.value })} slotProps={{ input: { "aria-label": "Job title" } }} /></Field>
      <FormControl sx={{ mt: 1 }}><FormLabel>Instructions for the crew</FormLabel><Textarea minRows={2} value={v.instructions ?? ""} onChange={(e) => setV({ ...v, instructions: e.target.value })} /></FormControl>
      <Box sx={{ mt: 1 }}><ScheduleEditor s={v.schedule ?? blankSchedule(todayISO())} onChange={(schedule) => setV({ ...v, schedule })} /></Box>
      <Row sx={{ mt: 1, alignItems: "flex-end" }}>
        <Box sx={{ flex: 1, minWidth: 220 }}><TeamPicker team={team} value={v.assignees ?? []} onChange={(assignees) => setV({ ...v, assignees })} /></Box>
        <Field label="Invoice" sx={{ maxWidth: 230 }}>
          <Select value={v.billing ?? "on_completion"} onChange={(_, x) => setV({ ...v, billing: x ?? "on_completion" })} slotProps={{ button: { "aria-label": "Billing" } }}><Option value="on_completion">When visits are done</Option><Option value="per_visit">After every visit</Option></Select>
        </Field>
      </Row>
      <Box sx={{ mt: 1 }}>
        <ItemsEditor items={v.items ?? []} onChange={(items) => setV({ ...v, items })} discount={v.discount ?? null} onDiscount={(discount) => setV({ ...v, discount })} taxRate={v.tax_rate ?? 0} onTax={(n) => setV({ ...v, tax_rate: n })} currency={settings.currency} services={services} />
      </Box>
      <Row sx={{ mt: 2, justifyContent: "flex-end" }}><Button variant="plain" onClick={onClose}>Cancel</Button><Button onClick={() => void run(async () => { const r = await fs.saveJob(j.id ?? "", v); onSaved(r.id); })}>Save job</Button></Row>
    </Dialog>
  );
}

function JobDetail({ id, settings, services, team, onClose }: { id: string; settings: Settings; services: Service[]; team: Team[]; onClose: () => void }) {
  const { err, run } = useErr();
  const [data, setData] = useState<{ job: Job; invoices: DocRef[]; property: string } | null>(null);
  const [edit, setEdit] = useState<Job | null>(null);
  const [note, setNote] = useState("");
  const load = useCallback(() => void run(async () => setData(await fs.job(id))), [run, id]);
  useEffect(load, [load]);
  const who = (ids: string[]) => ids.map((i) => team.find((t) => t.id === i)?.name ?? "").filter(Boolean).join(", ") || "Unassigned";
  if (!data) return <Dialog open onClose={onClose} title="Job"><ErrorAlert err={err} /></Dialog>;
  const j = data.job;
  const waiting = (j.visits ?? []).filter((v) => v.status === "completed" && !v.invoice_id).length;
  return (
    <Dialog open onClose={onClose} title={j.title} wide>
      <ErrorAlert err={err} />
      {note && <Alert color="success" variant="soft" sx={{ mb: 1 }}>{note}</Alert>}
      <Row sx={{ alignItems: "center" }}>
        <Chip color={STATUS_COLOR[j.display_status]} variant="soft">{STATUS_LABEL[j.display_status] ?? j.display_status}</Chip>
        <Typography level="body-sm">{j.client_name}{data.property ? ` · ${data.property}` : ""} · {describeSchedule(j.schedule)} · {who(j.assignees)}</Typography>
      </Row>
      {j.instructions && <Typography level="body-sm" sx={{ mt: 1 }}>{j.instructions}</Typography>}
      <Typography level="body-sm" sx={{ mt: 1 }}>Per visit: {money(j.totals.total, settings.currency)} · estimated profit {money(j.totals.profit, settings.currency)}</Typography>
      <Box sx={{ overflowX: "auto", mt: 1 }}>
        <Table size="sm" aria-label="Visits">
          <thead><tr><th>Date</th><th>Time</th><th>Who</th><th>Status</th><th /></tr></thead>
          <tbody>
            {(j.visits ?? []).map((v) => <VisitRow key={v.id} v={v} who={who(v.assignees)} team={team} onChanged={(m) => { if (m) setNote(m); load(); }} run={run} />)}
          </tbody>
        </Table>
      </Box>
      {data.invoices.length > 0 && (
        <Box sx={{ mt: 1 }}>
          <Typography level="title-sm">Invoices</Typography>
          {data.invoices.map((d) => <Row key={d.id} sx={{ alignItems: "center" }}><Typography level="body-sm">{d.number} · {d.status} · balance {money(d.balance, settings.currency)}</Typography><Button component="a" size="sm" variant="plain" href={`/invoicing?doc=${d.id}`}>Open in Invoicing</Button></Row>)}
        </Box>
      )}
      <Row sx={{ mt: 2, justifyContent: "space-between" }}>
        <Row>
          <Button variant="outlined" onClick={() => setEdit(j)} disabled={j.status === "cancelled"}>Edit job</Button>
          <Button variant="outlined" color="danger" disabled={j.status === "cancelled"} onClick={() => { if (window.confirm("Cancel this job and its open visits?")) void run(async () => { await fs.cancelJob(id); load(); }); }}>Cancel job</Button>
        </Row>
        <Button disabled={!waiting} onClick={() => void run(async () => { const r = await fs.invoiceJob(id); setNote(`Invoice ${r.document.number} created in Invoicing for ${r.visits} visit(s).`); load(); })}>Create invoice{waiting ? ` (${waiting} visit${waiting === 1 ? "" : "s"})` : ""}</Button>
      </Row>
      {edit && <JobDialog j={edit} settings={settings} services={services} team={team} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); load(); }} />}
    </Dialog>
  );
}

function VisitRow({ v, who, team, onChanged, run }: { v: Visit; who: string; team: Team[]; onChanged: (m?: string) => void; run: <T>(fn: () => Promise<T>) => Promise<T | undefined> }) {
  const [moving, setMoving] = useState(false);
  const [d, setD] = useState(v.date);
  const [st, setSt] = useState(v.start);
  const [en, setEn] = useState(v.end);
  const [ppl, setPpl] = useState(v.assignees);
  return (
    <>
      <tr>
        <td>{fmtDay(v.date)}</td><td>{v.start ? fmtRange(v.start, v.end || v.start) : "Any time"}</td><td>{who}</td>
        <td><Chip size="sm" color={STATUS_COLOR[v.status]} variant="soft">{STATUS_LABEL[v.status] ?? v.status}</Chip>{v.invoice_id && <Chip size="sm" sx={{ ml: 0.5 }} variant="outlined">invoiced</Chip>}</td>
        <td>
          <Row>
            {v.status === "scheduled" && <Button size="sm" onClick={() => void run(async () => { const r = await fs.completeVisit(v.id); onChanged(r.invoice ? `Visit done. Invoice ${r.invoice.number} created.` : undefined); })}>Complete</Button>}
            {v.status === "scheduled" && <Button size="sm" variant="outlined" onClick={() => setMoving(!moving)}>Reschedule</Button>}
            {v.status === "scheduled" && <Button size="sm" variant="plain" color="neutral" onClick={() => void run(async () => { await fs.cancelVisit(v.id); onChanged(); })}>Cancel</Button>}
            {v.status === "completed" && !v.invoice_id && <Button size="sm" variant="plain" onClick={() => void run(async () => { await fs.reopenVisit(v.id); onChanged(); })}>Reopen</Button>}
          </Row>
        </td>
      </tr>
      {moving && (
        <tr><td colSpan={5}>
          <Row sx={{ alignItems: "flex-end" }}>
            <Field label="Date" sx={{ maxWidth: 180 }}><Input type="date" value={d} onChange={(e) => setD(e.target.value)} slotProps={{ input: { "aria-label": "Visit date" } }} /></Field>
            <Field label="Start" sx={{ maxWidth: 130 }}><Input type="time" value={st} onChange={(e) => setSt(e.target.value)} /></Field>
            <Field label="End" sx={{ maxWidth: 130 }}><Input type="time" value={en} onChange={(e) => setEn(e.target.value)} /></Field>
            <Box sx={{ minWidth: 200 }}><TeamPicker team={team} value={ppl} onChange={setPpl} /></Box>
            <Button size="sm" onClick={() => void run(async () => { await fs.updateVisit(v.id, { date: d, start: st, end: en, assignees: ppl }); setMoving(false); onChanged("Visit moved. The Calendar items were updated."); })}>Save</Button>
          </Row>
        </td></tr>
      )}
    </>
  );
}
