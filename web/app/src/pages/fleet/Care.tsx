import { useCallback, useEffect, useState } from "react";
import { Alert, Box, Button, Chip, Input, Option, Select, Table, Typography } from "@mui/joy";
import { Card, Dialog, ErrorAlert, Field, Row, useErr } from "../hr/ui";
import { fmtDay, money, todayISO } from "../ops/dates";
import { fleet } from "./api";
import type { Me, Reminder, Renewal, Settings, Vehicle } from "./api";
import { STATUS_COLOR, STATUS_LABEL, compliancePct, describeInterval, dueText } from "./logic";

const UNITS = ["days", "weeks", "months", "years"];

/** Reminders and renewals of one vehicle (vehicleId) or of the whole fleet (""). Each one keeps a Calendar item on its due date. */
export function Care({ vehicleId, vehicles, me, settings }: { vehicleId: string; vehicles: Vehicle[]; me: Me; settings: Settings }) {
  const { err, run } = useErr();
  const [rems, setRems] = useState<Reminder[]>([]);
  const [rens, setRens] = useState<Renewal[]>([]);
  const [er, setEr] = useState<Partial<Reminder> | null>(null);
  const [en, setEn] = useState<Partial<Renewal> | null>(null);
  const [done, setDone] = useState<Reminder | null>(null);
  const [renewing, setRenewing] = useState<Renewal | null>(null);
  const [note, setNote] = useState("");
  const load = useCallback(() => void run(async () => { setRems(await fleet.reminders(vehicleId)); setRens(await fleet.renewals(vehicleId)); }), [run, vehicleId]);
  useEffect(load, [load]);
  const vname = (id: string) => vehicles.find((v) => v.id === id)?.name ?? "";
  const unit = settings.meter_unit;
  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
      <ErrorAlert err={err} />
      {note && <Alert color="success" variant="soft">{note}</Alert>}
      <Card title="Service reminders" action={me.admin ? <Button size="sm" onClick={() => setEr({ vehicle_id: vehicleId || vehicles[0]?.id, title: "", interval: { every: 6, unit: "months" }, meter_interval: 0 })}>New reminder</Button> : undefined}>
        <Box sx={{ overflowX: "auto" }}>
          <Table size="sm" aria-label="Reminders">
            <thead><tr>{!vehicleId && <th>Vehicle</th>}<th>Reminder</th><th>Repeats</th><th>Next due</th><th>Status</th><th /></tr></thead>
            <tbody>
              {rems.map((x) => (
                <tr key={x.id}>
                  {!vehicleId && <td>{vname(x.vehicle_id)}</td>}
                  <td>{x.title}{x.history.length > 0 && <Typography level="body-xs">{compliancePct(x.history)}% on time ({x.history.length})</Typography>}</td>
                  <td>{describeInterval(x.interval, x.meter_interval, unit)}</td>
                  <td>{x.next_date ? fmtDay(x.next_date) : "—"}<Typography level="body-xs">{dueText(x.days_left, x.meter_left, unit)}</Typography></td>
                  <td><Chip size="sm" variant="soft" color={STATUS_COLOR[x.status ?? "upcoming"]}>{STATUS_LABEL[x.status ?? "upcoming"]}</Chip></td>
                  <td>{me.admin && <Row><Button size="sm" onClick={() => setDone(x)}>Done</Button><Button size="sm" variant="outlined" onClick={() => setEr(x)}>Edit</Button></Row>}</td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Box>
        {!rems.length && <Typography level="body-sm">No service reminders yet.</Typography>}
      </Card>
      <Card title="Renewals" action={me.admin ? <Button size="sm" onClick={() => setEn({ vehicle_id: vehicleId || vehicles[0]?.id, kind: "registration", title: "", due_date: todayISO(), repeat: { every: 1, unit: "years" } })}>New renewal</Button> : undefined}>
        <Box sx={{ overflowX: "auto" }}>
          <Table size="sm" aria-label="Renewals">
            <thead><tr>{!vehicleId && <th>Vehicle</th>}<th>Renewal</th><th>Due</th><th>Repeats</th><th>Status</th><th /></tr></thead>
            <tbody>
              {rens.map((x) => (
                <tr key={x.id} style={{ opacity: x.done ? 0.5 : 1 }}>
                  {!vehicleId && <td>{vname(x.vehicle_id)}</td>}
                  <td>{x.title}<Typography level="body-xs">{x.kind}</Typography></td>
                  <td>{fmtDay(x.due_date)}<Typography level="body-xs">{x.days_left !== undefined ? dueText(x.days_left) : ""}</Typography></td>
                  <td>{x.repeat.every ? `Every ${x.repeat.every} ${x.repeat.unit}` : "Once"}</td>
                  <td><Chip size="sm" variant="soft" color={STATUS_COLOR[x.status ?? "upcoming"]}>{x.done ? "Done" : STATUS_LABEL[x.status ?? "upcoming"]}</Chip></td>
                  <td>{me.admin && !x.done && <Row><Button size="sm" onClick={() => setRenewing(x)}>Renewed</Button><Button size="sm" variant="outlined" onClick={() => setEn(x)}>Edit</Button></Row>}</td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Box>
        {!rens.length && <Typography level="body-sm">No renewals yet. Add registration, insurance or emission tests.</Typography>}
      </Card>
      {er && <ReminderDialog r={er} vehicles={vehicles} fixed={!!vehicleId} unit={unit} onClose={() => setEr(null)} onSaved={() => { setEr(null); setNote("Saved. A Calendar item now marks the due date."); load(); }} />}
      {en && <RenewalDialog r={en} vehicles={vehicles} fixed={!!vehicleId} onClose={() => setEn(null)} onSaved={() => { setEn(null); setNote("Saved. A Calendar item now marks the due date."); load(); }} />}
      {done && <DoneDialog x={done} onClose={() => setDone(null)} onSaved={() => { setDone(null); load(); }} />}
      {renewing && <RenewedDialog x={renewing} cur={settings.currency} onClose={() => setRenewing(null)} onSaved={() => { setRenewing(null); load(); }} />}
    </Box>
  );
}

function VehicleSelect({ value, vehicles, onChange, disabled }: { value: string; vehicles: Vehicle[]; onChange: (id: string) => void; disabled: boolean }) {
  return (
    <Field label="Vehicle"><Select value={value} disabled={disabled} onChange={(_, v) => onChange(v ?? "")} slotProps={{ button: { "aria-label": "Vehicle" } }}>{vehicles.map((v) => <Option key={v.id} value={v.id}>{v.name}</Option>)}</Select></Field>
  );
}

function ReminderDialog({ r, vehicles, fixed, unit, onClose, onSaved }: { r: Partial<Reminder>; vehicles: Vehicle[]; fixed: boolean; unit: string; onClose: () => void; onSaved: () => void }) {
  const { err, run } = useErr();
  const [x, setX] = useState<Partial<Reminder>>(r);
  const iv = x.interval ?? { every: 0, unit: "months" };
  return (
    <Dialog open onClose={onClose} title={r.id ? "Edit reminder" : "New service reminder"}>
      <ErrorAlert err={err} />
      <VehicleSelect value={x.vehicle_id ?? ""} vehicles={vehicles} disabled={fixed || !!r.id} onChange={(id) => setX({ ...x, vehicle_id: id })} />
      <Field label="Title" sx={{ mt: 1 }}><Input value={x.title ?? ""} onChange={(e) => setX({ ...x, title: e.target.value })} slotProps={{ input: { "aria-label": "Reminder title" } }} /></Field>
      <Row sx={{ mt: 1 }}>
        <Field label="Every (time)" sx={{ maxWidth: 110 }}><Input type="number" value={iv.every} onChange={(e) => setX({ ...x, interval: { ...iv, every: Number(e.target.value) } })} slotProps={{ input: { "aria-label": "Interval", min: 0 } }} /></Field>
        <Field label="Unit" sx={{ maxWidth: 140 }}><Select value={iv.unit || "months"} onChange={(_, v) => setX({ ...x, interval: { ...iv, unit: v ?? "months" } })} slotProps={{ button: { "aria-label": "Interval unit" } }}>{UNITS.map((u) => <Option key={u} value={u}>{u}</Option>)}</Select></Field>
        <Field label={`or every (${unit})`} sx={{ maxWidth: 160 }}><Input type="number" value={x.meter_interval ?? 0} onChange={(e) => setX({ ...x, meter_interval: Number(e.target.value) })} slotProps={{ input: { "aria-label": "Meter interval", min: 0 } }} /></Field>
      </Row>
      <Typography level="body-xs" sx={{ mt: 0.5 }}>Whichever comes first makes it due.</Typography>
      <Row sx={{ mt: 1 }}>
        <Field label="Last done (optional)" sx={{ maxWidth: 190 }}><Input type="date" value={x.last_date ?? ""} onChange={(e) => setX({ ...x, last_date: e.target.value })} slotProps={{ input: { "aria-label": "Last done" } }} /></Field>
        <Field label={`Meter then (${unit})`} sx={{ maxWidth: 170 }}><Input type="number" value={x.last_meter ?? 0} onChange={(e) => setX({ ...x, last_meter: Number(e.target.value) })} /></Field>
      </Row>
      <Row sx={{ mt: 2, justifyContent: "space-between" }}>
        {r.id ? <Button variant="outlined" color="danger" onClick={() => { if (window.confirm("Delete this reminder and its Calendar item?")) void run(async () => { await fleet.deleteReminder(r.id!); onSaved(); }); }}>Delete</Button> : <span />}
        <Row><Button variant="plain" onClick={onClose}>Cancel</Button><Button onClick={() => void run(async () => { await fleet.saveReminder(r.id ?? "", x); onSaved(); })}>Save reminder</Button></Row>
      </Row>
    </Dialog>
  );
}

function RenewalDialog({ r, vehicles, fixed, onClose, onSaved }: { r: Partial<Renewal>; vehicles: Vehicle[]; fixed: boolean; onClose: () => void; onSaved: () => void }) {
  const { err, run } = useErr();
  const [x, setX] = useState<Partial<Renewal>>(r);
  const rep = x.repeat ?? { every: 0, unit: "years" };
  return (
    <Dialog open onClose={onClose} title={r.id ? "Edit renewal" : "New renewal"}>
      <ErrorAlert err={err} />
      <VehicleSelect value={x.vehicle_id ?? ""} vehicles={vehicles} disabled={fixed || !!r.id} onChange={(id) => setX({ ...x, vehicle_id: id })} />
      <Row sx={{ mt: 1 }}>
        <Field label="Title"><Input value={x.title ?? ""} onChange={(e) => setX({ ...x, title: e.target.value })} slotProps={{ input: { "aria-label": "Renewal title" } }} /></Field>
        <Field label="Kind" sx={{ maxWidth: 180 }}><Select value={x.kind ?? "registration"} onChange={(_, v) => setX({ ...x, kind: v ?? "registration" })} slotProps={{ button: { "aria-label": "Kind" } }}>{["registration", "insurance", "emission", "licence", "other"].map((k) => <Option key={k} value={k}>{k}</Option>)}</Select></Field>
      </Row>
      <Row sx={{ mt: 1 }}>
        <Field label="Due date" sx={{ maxWidth: 190 }}><Input type="date" value={x.due_date ?? ""} onChange={(e) => setX({ ...x, due_date: e.target.value })} slotProps={{ input: { "aria-label": "Due date" } }} /></Field>
        <Field label="Repeats every" sx={{ maxWidth: 120 }}><Input type="number" value={rep.every} onChange={(e) => setX({ ...x, repeat: { ...rep, every: Number(e.target.value) } })} slotProps={{ input: { "aria-label": "Repeat every", min: 0 } }} /></Field>
        <Field label="Unit" sx={{ maxWidth: 140 }}><Select value={rep.unit || "years"} onChange={(_, v) => setX({ ...x, repeat: { ...rep, unit: v ?? "years" } })} slotProps={{ button: { "aria-label": "Repeat unit" } }}>{UNITS.map((u) => <Option key={u} value={u}>{u}</Option>)}</Select></Field>
      </Row>
      <Row sx={{ mt: 2, justifyContent: "flex-end" }}><Button variant="plain" onClick={onClose}>Cancel</Button><Button onClick={() => void run(async () => { await fleet.saveRenewal(r.id ?? "", x); onSaved(); })}>Save renewal</Button></Row>
    </Dialog>
  );
}

function DoneDialog({ x, onClose, onSaved }: { x: Reminder; onClose: () => void; onSaved: () => void }) {
  const { err, run } = useErr();
  const [date, setDate] = useState(todayISO());
  const [meter, setMeter] = useState(0);
  return (
    <Dialog open onClose={onClose} title={`Done: ${x.title}`}>
      <ErrorAlert err={err} />
      <Typography level="body-sm" sx={{ mb: 1 }}>The next due date is worked out from today. Logging a service entry on the vehicle does the same.</Typography>
      <Row>
        <Field label="Date" sx={{ maxWidth: 190 }}><Input type="date" value={date} onChange={(e) => setDate(e.target.value)} slotProps={{ input: { "aria-label": "Done on" } }} /></Field>
        <Field label="Meter (blank = current)" sx={{ maxWidth: 200 }}><Input type="number" value={meter || ""} onChange={(e) => setMeter(Number(e.target.value))} /></Field>
      </Row>
      <Row sx={{ mt: 2, justifyContent: "flex-end" }}><Button variant="plain" onClick={onClose}>Cancel</Button><Button onClick={() => void run(async () => { await fleet.completeReminder(x.id, date, meter); onSaved(); })}>Mark done</Button></Row>
    </Dialog>
  );
}

function RenewedDialog({ x, cur, onClose, onSaved }: { x: Renewal; cur: string; onClose: () => void; onSaved: () => void }) {
  const { err, run } = useErr();
  const [date, setDate] = useState(todayISO());
  const [cost, setCost] = useState(x.cost || 0);
  return (
    <Dialog open onClose={onClose} title={`Renewed: ${x.title}`}>
      <ErrorAlert err={err} />
      <Typography level="body-sm" sx={{ mb: 1 }}>{x.repeat.every ? "The due date rolls forward from the old due date." : "This renewal does not repeat, so it will be marked done."}</Typography>
      <Row>
        <Field label="Renewed on" sx={{ maxWidth: 190 }}><Input type="date" value={date} onChange={(e) => setDate(e.target.value)} slotProps={{ input: { "aria-label": "Renewed on" } }} /></Field>
        <Field label={`Cost (${cur})`} sx={{ maxWidth: 170 }}><Input type="number" value={cost} onChange={(e) => setCost(Number(e.target.value))} slotProps={{ input: { "aria-label": "Renewal cost", min: 0 } }} /></Field>
      </Row>
      <Typography level="body-xs" sx={{ mt: 0.5 }}>{money(cost, cur)}</Typography>
      <Row sx={{ mt: 2, justifyContent: "flex-end" }}><Button variant="plain" onClick={onClose}>Cancel</Button><Button onClick={() => void run(async () => { await fleet.completeRenewal(x.id, date, cost); onSaved(); })}>Save</Button></Row>
    </Dialog>
  );
}
