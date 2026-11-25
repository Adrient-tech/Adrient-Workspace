import { useCallback, useEffect, useState } from "react";
import { Alert, Box, Button, Checkbox, Chip, Input, Option, Select, Tab, TabList, TabPanel, Table, Tabs, Textarea, Typography } from "@mui/joy";
import { Dialog, ErrorAlert, Field, Row, Stat, useErr } from "../hr/ui";
import { fmtDay, money, todayISO } from "../ops/dates";
import { fleet } from "./api";
import type { Category, Costs, Fuel, Inspection, Issue, MeterReading, Me, Service, Settings, Vehicle } from "./api";
import { STATUS_COLOR, STATUS_LABEL } from "./logic";
import { Care } from "./Care";
import { IssueList } from "./Issues";

export function VehicleDetail({ id, me, settings, onClose, onEdit }: { id: string; me: Me; settings: Settings; onClose: () => void; onEdit: (v: Vehicle) => void }) {
  const { err, run } = useErr();
  const [data, setData] = useState<{ vehicle: Vehicle; costs: Costs } | null>(null);
  const [tab, setTab] = useState("overview");
  const load = useCallback(() => void run(async () => setData(await fleet.vehicle(id))), [run, id]);
  useEffect(load, [load]);
  if (!data) return <Dialog open onClose={onClose} title="Vehicle"><ErrorAlert err={err} /></Dialog>;
  const v = data.vehicle;
  const u = settings.meter_unit;
  return (
    <Dialog open onClose={onClose} title={`${v.name}${v.plate ? ` · ${v.plate}` : ""}`} wide>
      <ErrorAlert err={err} />
      <Row sx={{ alignItems: "center" }}>
        <Chip variant="soft" color={STATUS_COLOR[v.status]}>{STATUS_LABEL[v.status]}</Chip>
        <Typography level="body-sm">{[v.year || "", v.make, v.model, v.fuel_type].filter(Boolean).join(" · ")} · {v.meter.toLocaleString("en-IN")} {u}{v.assignee_name ? ` · driver ${v.assignee_name}` : ""}</Typography>
        {me.admin && <Button size="sm" variant="outlined" sx={{ ml: "auto" }} onClick={() => onEdit(v)}>Edit vehicle</Button>}
      </Row>
      <Tabs value={tab} onChange={(_, t) => setTab(String(t))} sx={{ mt: 1, bgcolor: "transparent" }}>
        <TabList sx={{ overflowX: "auto" }}>
          {["overview", "fuel", "service", "reminders", "issues", "meter"].map((t) => <Tab key={t} value={t} data-testid={`veh-tab-${t}`}>{t[0].toUpperCase() + t.slice(1)}</Tab>)}
        </TabList>
        <TabPanel value="overview">
          <Row>
            <Stat label="Fuel" value={money(data.costs.fuel, settings.currency)} />
            <Stat label="Service" value={money(data.costs.service, settings.currency)} />
            <Stat label="Total spend" value={money(data.costs.total, settings.currency)} />
            <Stat label={`Cost per ${u}`} value={data.costs.per_distance ? money(data.costs.per_distance, settings.currency) : "—"} hint={data.costs.distance ? `${data.costs.distance.toLocaleString("en-IN")} ${u} logged` : "needs two meter readings"} />
          </Row>
          {v.vin && <Typography level="body-sm" sx={{ mt: 1 }}>VIN {v.vin}</Typography>}
          {v.notes && <Typography level="body-sm" sx={{ mt: 1 }}>{v.notes}</Typography>}
          <MeterAdd vehicle={v} unit={u} onDone={load} />
        </TabPanel>
        <TabPanel value="fuel"><FuelTab vehicle={v} me={me} settings={settings} onChanged={load} /></TabPanel>
        <TabPanel value="service"><ServiceTab vehicle={v} me={me} settings={settings} onChanged={load} /></TabPanel>
        <TabPanel value="reminders"><Care vehicleId={v.id} vehicles={[v]} me={me} settings={settings} /></TabPanel>
        <TabPanel value="issues"><IssueList vehicleId={v.id} vehicles={[v]} me={me} /></TabPanel>
        <TabPanel value="meter"><MeterTab vehicle={v} unit={u} /></TabPanel>
      </Tabs>
    </Dialog>
  );
}

function MeterAdd({ vehicle, unit, onDone }: { vehicle: Vehicle; unit: string; onDone: () => void }) {
  const { err, run } = useErr();
  const [val, setVal] = useState(0);
  const [warn, setWarn] = useState<string[]>([]);
  const [askForce, setAskForce] = useState(false);
  const add = (force: boolean) => void run(async () => {
    try {
      const r = await fleet.addMeter(vehicle.id, val, todayISO(), force);
      setWarn(r.warnings); setAskForce(false); setVal(0); onDone();
    } catch (e) {
      if (/lower than the last/.test((e as Error).message)) { setAskForce(true); throw e; }
      throw e;
    }
  });
  return (
    <Box sx={{ mt: 2 }}>
      <ErrorAlert err={err} />
      {warn.map((w) => <Alert key={w} color="warning" variant="soft" sx={{ mb: 1 }}>{w}</Alert>)}
      <Row sx={{ alignItems: "flex-end" }}>
        <Field label={`New meter reading (${unit})`} sx={{ maxWidth: 240 }}><Input type="number" value={val || ""} onChange={(e) => setVal(Number(e.target.value))} slotProps={{ input: { "aria-label": "New meter reading", min: 0 } }} /></Field>
        <Button disabled={!val} onClick={() => add(false)}>Record reading</Button>
        {askForce && <Button color="warning" variant="outlined" onClick={() => add(true)}>Record the lower reading anyway</Button>}
      </Row>
    </Box>
  );
}

function MeterTab({ vehicle, unit }: { vehicle: Vehicle; unit: string }) {
  const { err, run } = useErr();
  const [list, setList] = useState<MeterReading[]>([]);
  useEffect(() => { void run(async () => setList(await fleet.meter(vehicle.id))); }, [run, vehicle.id, vehicle.meter]);
  return (
    <Box sx={{ overflowX: "auto" }}>
      <ErrorAlert err={err} />
      <Table size="sm" aria-label="Meter readings"><thead><tr><th>Date</th><th>Reading</th><th>Source</th></tr></thead><tbody>{list.map((m) => <tr key={m.id}><td>{fmtDay(m.date)}</td><td>{m.value.toLocaleString("en-IN")} {unit}</td><td>{m.source}</td></tr>)}</tbody></Table>
    </Box>
  );
}

function FuelTab({ vehicle, me, settings, onChanged }: { vehicle: Vehicle; me: Me; settings: Settings; onChanged: () => void }) {
  const { err, run } = useErr();
  const [list, setList] = useState<Fuel[]>([]);
  const [add, setAdd] = useState<Partial<Fuel> | null>(null);
  const [post, setPost] = useState<{ kind: "fuel" | "service"; id: string } | null>(null);
  const load = useCallback(() => void run(async () => setList(await fleet.fuel(vehicle.id))), [run, vehicle.id]);
  useEffect(load, [load]);
  return (
    <Box>
      <ErrorAlert err={err} />
      <Button size="sm" onClick={() => setAdd({ vehicle_id: vehicle.id, date: todayISO(), quantity: 0, total: 0, meter: vehicle.meter, full_tank: true })}>Add fuel</Button>
      <Box sx={{ overflowX: "auto", mt: 1 }}>
        <Table size="sm" aria-label="Fuel"><thead><tr><th>Date</th><th>Quantity</th><th>Cost</th><th>Meter</th><th>Economy</th><th /></tr></thead>
          <tbody>{list.map((f) => <tr key={f.id}><td>{fmtDay(f.date)}</td><td>{f.quantity}</td><td>{money(f.total, settings.currency)}</td><td>{f.meter ? f.meter.toLocaleString("en-IN") : "—"}</td><td>{f.economy ? `${f.economy} ${settings.meter_unit}/unit` : "—"}</td>
            <td>{me.admin && f.total > 0 && (f.expense_id ? <Chip size="sm" color="success" variant="soft">In Accounting</Chip> : <Button size="sm" variant="plain" onClick={() => setPost({ kind: "fuel", id: f.id })}>Post to Accounting</Button>)}</td></tr>)}</tbody></Table>
      </Box>
      {add && <FuelDialog f={add} onClose={() => setAdd(null)} onSaved={() => { setAdd(null); load(); onChanged(); }} />}
      {post && <PostDialog {...post} settings={settings} onClose={() => setPost(null)} onDone={() => { setPost(null); load(); }} />}
    </Box>
  );
}

function FuelDialog({ f, onClose, onSaved }: { f: Partial<Fuel>; onClose: () => void; onSaved: () => void }) {
  const { err, run } = useErr();
  const [x, setX] = useState(f);
  return (
    <Dialog open onClose={onClose} title="Add fuel">
      <ErrorAlert err={err} />
      <Row>
        <Field label="Date" sx={{ maxWidth: 180 }}><Input type="date" value={x.date ?? ""} onChange={(e) => setX({ ...x, date: e.target.value })} slotProps={{ input: { "aria-label": "Fuel date" } }} /></Field>
        <Field label="Quantity" sx={{ maxWidth: 120 }}><Input type="number" value={x.quantity ?? 0} onChange={(e) => setX({ ...x, quantity: Number(e.target.value) })} slotProps={{ input: { "aria-label": "Fuel quantity", min: 0 } }} /></Field>
        <Field label="Total cost" sx={{ maxWidth: 140 }}><Input type="number" value={x.total ?? 0} onChange={(e) => setX({ ...x, total: Number(e.target.value) })} slotProps={{ input: { "aria-label": "Fuel cost", min: 0 } }} /></Field>
      </Row>
      <Row sx={{ mt: 1 }}>
        <Field label="Meter"><Input type="number" value={x.meter ?? 0} onChange={(e) => setX({ ...x, meter: Number(e.target.value) })} slotProps={{ input: { "aria-label": "Fuel meter", min: 0 } }} /></Field>
        <Field label="Station"><Input value={x.vendor ?? ""} onChange={(e) => setX({ ...x, vendor: e.target.value })} /></Field>
      </Row>
      <Checkbox sx={{ mt: 1 }} label="Filled to full (used for fuel economy)" checked={!!x.full_tank} onChange={(e) => setX({ ...x, full_tank: e.target.checked })} />
      <Row sx={{ mt: 2, justifyContent: "flex-end" }}><Button variant="plain" onClick={onClose}>Cancel</Button><Button onClick={() => void run(async () => { await fleet.addFuel(x); onSaved(); })}>Save</Button></Row>
    </Dialog>
  );
}

function ServiceTab({ vehicle, me, settings, onChanged }: { vehicle: Vehicle; me: Me; settings: Settings; onChanged: () => void }) {
  const { err, run } = useErr();
  const [list, setList] = useState<Service[]>([]);
  const [add, setAdd] = useState<Partial<Service> | null>(null);
  const [post, setPost] = useState<{ kind: "fuel" | "service"; id: string } | null>(null);
  const load = useCallback(() => void run(async () => setList(await fleet.service(vehicle.id))), [run, vehicle.id]);
  useEffect(load, [load]);
  return (
    <Box>
      <ErrorAlert err={err} />
      {me.admin && <Button size="sm" onClick={() => setAdd({ vehicle_id: vehicle.id, date: todayISO(), meter: vehicle.meter, description: "", cost: 0, reminder_ids: [] })}>Add service entry</Button>}
      <Box sx={{ overflowX: "auto", mt: 1 }}>
        <Table size="sm" aria-label="Service history"><thead><tr><th>Date</th><th>Work</th><th>Vendor</th><th>Cost</th><th /></tr></thead>
          <tbody>{list.map((s) => <tr key={s.id}><td>{fmtDay(s.date)}</td><td>{s.description}</td><td>{s.vendor || "—"}</td><td>{money(s.cost, settings.currency)}</td>
            <td>{me.admin && s.cost > 0 && (s.expense_id ? <Chip size="sm" color="success" variant="soft">In Accounting</Chip> : <Button size="sm" variant="plain" onClick={() => setPost({ kind: "service", id: s.id })}>Post to Accounting</Button>)}</td></tr>)}</tbody></Table>
      </Box>
      {add && <ServiceDialog s={add} onClose={() => setAdd(null)} onSaved={() => { setAdd(null); load(); onChanged(); }} />}
      {post && <PostDialog {...post} settings={settings} onClose={() => setPost(null)} onDone={() => { setPost(null); load(); }} />}
    </Box>
  );
}

function ServiceDialog({ s, onClose, onSaved }: { s: Partial<Service>; onClose: () => void; onSaved: () => void }) {
  const { err, run } = useErr();
  const [x, setX] = useState(s);
  const [rems, setRems] = useState<{ id: string; title: string; status?: string }[]>([]);
  useEffect(() => { void run(async () => setRems(await fleet.reminders(s.vehicle_id ?? ""))); }, [run, s.vehicle_id]);
  return (
    <Dialog open onClose={onClose} title="Add service entry">
      <ErrorAlert err={err} />
      <Row>
        <Field label="Date" sx={{ maxWidth: 180 }}><Input type="date" value={x.date ?? ""} onChange={(e) => setX({ ...x, date: e.target.value })} slotProps={{ input: { "aria-label": "Service date" } }} /></Field>
        <Field label="Meter" sx={{ maxWidth: 150 }}><Input type="number" value={x.meter ?? 0} onChange={(e) => setX({ ...x, meter: Number(e.target.value) })} slotProps={{ input: { "aria-label": "Service meter", min: 0 } }} /></Field>
        <Field label="Cost" sx={{ maxWidth: 140 }}><Input type="number" value={x.cost ?? 0} onChange={(e) => setX({ ...x, cost: Number(e.target.value) })} slotProps={{ input: { "aria-label": "Service cost", min: 0 } }} /></Field>
      </Row>
      <Field label="Work done" sx={{ mt: 1 }}><Textarea minRows={2} value={x.description ?? ""} onChange={(e) => setX({ ...x, description: e.target.value })} slotProps={{ textarea: { "aria-label": "Work done" } }} /></Field>
      <Field label="Garage / vendor" sx={{ mt: 1 }}><Input value={x.vendor ?? ""} onChange={(e) => setX({ ...x, vendor: e.target.value })} /></Field>
      {rems.length > 0 && <Box sx={{ mt: 1 }}><Typography level="body-sm">This completes these reminders:</Typography>{rems.map((r) => <Checkbox key={r.id} size="sm" label={`${r.title} (${r.status ?? ""})`} checked={(x.reminder_ids ?? []).includes(r.id)} onChange={(e) => setX({ ...x, reminder_ids: e.target.checked ? [...(x.reminder_ids ?? []), r.id] : (x.reminder_ids ?? []).filter((i) => i !== r.id) })} />)}</Box>}
      <Row sx={{ mt: 2, justifyContent: "flex-end" }}><Button variant="plain" onClick={onClose}>Cancel</Button><Button onClick={() => void run(async () => { await fleet.addService(x); onSaved(); })}>Save</Button></Row>
    </Dialog>
  );
}

function PostDialog({ kind, id, settings, onClose, onDone }: { kind: "fuel" | "service"; id: string; settings: Settings; onClose: () => void; onDone: () => void }) {
  const { err, run } = useErr();
  const [cats, setCats] = useState<Category[]>([]);
  const [cat, setCat] = useState(settings.expense_category_id);
  useEffect(() => { void run(async () => setCats(await fleet.categories())); }, [run]);
  return (
    <Dialog open onClose={onClose} title="Post cost to Accounting">
      <ErrorAlert err={err} />
      <Typography level="body-sm" sx={{ mb: 1 }}>Creates an unpaid bill in Accounting under the expense category you choose. It can only be posted once.</Typography>
      <Field label="Expense category"><Select value={cat} onChange={(_, v) => setCat(v ?? "")} slotProps={{ button: { "aria-label": "Expense category" } }} placeholder="Choose a category">{cats.map((c) => <Option key={c.id} value={c.id}>{c.code} {c.name}</Option>)}</Select></Field>
      <Row sx={{ mt: 2, justifyContent: "flex-end" }}><Button variant="plain" onClick={onClose}>Cancel</Button><Button disabled={!cat} onClick={() => void run(async () => { await fleet.postExpense(kind, id, cat); onDone(); })}>Post to Accounting</Button></Row>
    </Dialog>
  );
}

export type { Inspection, Issue };
