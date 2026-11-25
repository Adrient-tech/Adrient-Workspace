import { PersonSelect } from "../../components/PeoplePicker";
import { useCallback, useEffect, useState } from "react";
import { Alert, Box, Button, Chip, Input, Option, Select, Table, Textarea, Typography } from "@mui/joy";
import { Card, Dialog, ErrorAlert, Field, Row, useErr } from "../hr/ui";
import { fleet } from "./api";
import type { Me, Settings, Vehicle } from "./api";
import { STATUS_COLOR, STATUS_LABEL, cleanVin, vinCheckOk, vinShapeOk } from "./logic";
import { VehicleDetail } from "./Detail";

export function Vehicles({ me, settings, openId, onOpened }: { me: Me; settings: Settings; openId: string; onOpened: () => void }) {
  const { err, run } = useErr();
  const [list, setList] = useState<Vehicle[]>([]);
  const [status, setStatus] = useState("");
  const [q, setQ] = useState("");
  const [edit, setEdit] = useState<Partial<Vehicle> | null>(null);
  const [view, setView] = useState("");
  const load = useCallback(() => void run(async () => setList(await fleet.vehicles({ status, q }))), [run, status, q]);
  useEffect(load, [load]);
  useEffect(() => { if (openId) { setView(openId); onOpened(); } }, [openId, onOpened]);
  return (
    <Card title="Vehicles" action={me.admin ? <Button size="sm" onClick={() => setEdit({ name: "", status: "active", vin: "", plate: "", meter: 0 })}>Add vehicle</Button> : undefined}>
      <ErrorAlert err={err} />
      <Row sx={{ mb: 1, alignItems: "center" }}>
        <Input size="sm" placeholder="Search name, plate or VIN" value={q} onChange={(e) => setQ(e.target.value)} slotProps={{ input: { "aria-label": "Search vehicles" } }} sx={{ minWidth: 240 }} />
        {["", "active", "maintenance", "inactive", "sold"].map((s) => <Chip key={s} variant={status === s ? "solid" : "outlined"} onClick={() => setStatus(s)} data-testid={`veh-filter-${s || "all"}`}>{s ? STATUS_LABEL[s] : "All"}</Chip>)}
      </Row>
      <Box sx={{ overflowX: "auto" }}>
        <Table size="sm" aria-label="Vehicles">
          <thead><tr><th>Vehicle</th><th>Plate</th><th>Driver</th><th>Meter</th><th>Status</th><th>Alerts</th><th /></tr></thead>
          <tbody>
            {list.map((v) => (
              <tr key={v.id}>
                <td>{v.name}<Typography level="body-xs">{[v.year || "", v.make, v.model].filter(Boolean).join(" ")}</Typography></td>
                <td>{v.plate || "—"}</td><td>{v.assignee_name || "—"}</td><td>{v.meter.toLocaleString("en-IN")} {settings.meter_unit}</td>
                <td><Chip size="sm" variant="soft" color={STATUS_COLOR[v.status]}>{STATUS_LABEL[v.status] ?? v.status}</Chip></td>
                <td>
                  {v.overdue ? <Chip size="sm" color="danger" variant="soft" sx={{ mr: 0.5 }}>{v.overdue} overdue</Chip> : null}
                  {v.due_soon ? <Chip size="sm" color="warning" variant="soft" sx={{ mr: 0.5 }}>{v.due_soon} due soon</Chip> : null}
                  {v.open_issues ? <Chip size="sm" color="danger" variant="outlined">{v.open_issues} issue{v.open_issues === 1 ? "" : "s"}</Chip> : null}
                </td>
                <td><Button size="sm" variant="outlined" onClick={() => setView(v.id)}>Open</Button></td>
              </tr>
            ))}
          </tbody>
        </Table>
      </Box>
      {!list.length && <Typography level="body-sm" sx={{ mt: 1 }}>No vehicles here. {me.admin ? "Add your first vehicle." : "An admin can add vehicles."}</Typography>}
      {edit && <VehicleDialog v={edit} onClose={() => setEdit(null)} onSaved={(id) => { setEdit(null); load(); setView(id); }} />}
      {view && <VehicleDetail id={view} me={me} settings={settings} onClose={() => { setView(""); load(); }} onEdit={(v) => setEdit(v)} />}
    </Card>
  );
}

export function VehicleDialog({ v, onClose, onSaved }: { v: Partial<Vehicle>; onClose: () => void; onSaved: (id: string) => void }) {
  const { err, run } = useErr();
  const [x, setX] = useState<Partial<Vehicle>>(v);
  const [warn, setWarn] = useState<string[]>([]);
  const vin = cleanVin(x.vin ?? "");
  const set = (p: Partial<Vehicle>) => setX({ ...x, ...p });
  return (
    <Dialog open onClose={onClose} title={v.id ? `Edit ${v.name}` : "Add vehicle"} wide>
      <ErrorAlert err={err} />
      {warn.map((w) => <Alert key={w} color="warning" variant="soft" sx={{ mb: 1 }}>{w}</Alert>)}
      <Row>
        <Field label="Name"><Input value={x.name ?? ""} onChange={(e) => set({ name: e.target.value })} slotProps={{ input: { "aria-label": "Vehicle name" } }} /></Field>
        <Field label="Plate"><Input value={x.plate ?? ""} onChange={(e) => set({ plate: e.target.value.toUpperCase() })} slotProps={{ input: { "aria-label": "Plate" } }} /></Field>
        <Field label="Status">
          <Select value={x.status ?? "active"} onChange={(_, s) => set({ status: s ?? "active" })} slotProps={{ button: { "aria-label": "Status" } }}>
            {["active", "maintenance", "inactive", "sold"].map((s) => <Option key={s} value={s}>{STATUS_LABEL[s]}</Option>)}
          </Select>
        </Field>
      </Row>
      <Field label="VIN" sx={{ mt: 1 }}>
        <Input value={x.vin ?? ""} onChange={(e) => set({ vin: e.target.value.toUpperCase() })} slotProps={{ input: { "aria-label": "VIN" } }}
          color={vin && !vinShapeOk(vin) ? "danger" : "neutral"} endDecorator={vin ? <Typography level="body-xs">{vinShapeOk(vin) ? (vinCheckOk(vin) ? "valid" : "check digit differs") : `${vin.length}/17`}</Typography> : undefined} />
      </Field>
      <Row sx={{ mt: 1 }}>
        <Field label="Make"><Input value={x.make ?? ""} onChange={(e) => set({ make: e.target.value })} /></Field>
        <Field label="Model"><Input value={x.model ?? ""} onChange={(e) => set({ model: e.target.value })} /></Field>
        <Field label="Year" sx={{ maxWidth: 110 }}><Input type="number" value={x.year ?? ""} onChange={(e) => set({ year: Number(e.target.value) })} /></Field>
      </Row>
      <Row sx={{ mt: 1 }}>
        <Field label="Type"><Input value={x.type ?? ""} placeholder="Van, truck, forklift…" onChange={(e) => set({ type: e.target.value })} /></Field>
        <Field label="Fuel"><Input value={x.fuel_type ?? ""} placeholder="Diesel, petrol, EV…" onChange={(e) => set({ fuel_type: e.target.value })} /></Field>
        <Field label="Group"><Input value={x.group ?? ""} onChange={(e) => set({ group: e.target.value })} /></Field>
      </Row>
      <Row sx={{ mt: 1 }}>
        <Field label="Current meter" sx={{ maxWidth: 180 }}><Input type="number" disabled={!!v.id} value={x.meter ?? 0} onChange={(e) => set({ meter: Number(e.target.value) })} slotProps={{ input: { "aria-label": "Current meter", min: 0 } }} /></Field>
        <Field label="Purchased" sx={{ maxWidth: 190 }}><Input type="date" value={x.purchase_date ?? ""} onChange={(e) => set({ purchase_date: e.target.value })} /></Field>
        <Field label="Purchase price" sx={{ maxWidth: 180 }}><Input type="number" value={x.purchase_price ?? 0} onChange={(e) => set({ purchase_price: Number(e.target.value) })} /></Field>
      </Row>
      <DriverField value={x.assignee_id ?? ""} onChange={(id) => set({ assignee_id: id })} />
      <Field label="Notes" sx={{ mt: 1 }}><Textarea minRows={2} value={x.notes ?? ""} onChange={(e) => set({ notes: e.target.value })} /></Field>
      <Row sx={{ mt: 2, justifyContent: "space-between" }}>
        {v.id ? <Button variant="outlined" color="neutral" onClick={() => void run(async () => { await fleet.saveVehicle(v.id!, { ...x, archived: true }); onSaved(v.id!); })}>Archive</Button> : <span />}
        <Row><Button variant="plain" onClick={onClose}>Cancel</Button><Button onClick={() => void run(async () => { const r = await fleet.saveVehicle(v.id ?? "", { ...x, vin }); if (r.warnings.length) { setWarn(r.warnings); window.setTimeout(() => onSaved(r.vehicle.id), 900); } else onSaved(r.vehicle.id); })}>Save vehicle</Button></Row>
      </Row>
    </Dialog>
  );
}

function DriverField({ value, onChange }: { value: string; onChange: (id: string) => void }) {
  const [team, setTeam] = useState<{ id: string; name: string; email: string }[]>([]);
  useEffect(() => { fetch("/api/v1/fleet/team", { credentials: "same-origin" }).then((r) => (r.ok ? r.json() : { team: [] })).then((d: { team: { id: string; name: string; email: string }[] }) => setTeam(d.team ?? [])).catch(() => undefined); }, []);
  return (
    <Field label="Assigned driver" sx={{ mt: 1 }}>
      <PersonSelect value={value} known={team} onChange={(id) => onChange(id)} placeholder="Nobody" ariaLabel="Person" />
    </Field>
  );
}
