import { useCallback, useEffect, useState } from "react";
import { Alert, Box, Button, Chip, Input, Option, Select, Table, Textarea, Typography } from "@mui/joy";
import { Card, Dialog, ErrorAlert, Field, Row, useErr } from "../hr/ui";
import { fmtDay, todayISO } from "../ops/dates";
import { fleet } from "./api";
import type { Inspection, InspectionItem, Issue, Me, Settings, Vehicle } from "./api";
import { STATUS_COLOR, STATUS_LABEL } from "./logic";

export function IssueList({ vehicleId, vehicles, me }: { vehicleId: string; vehicles: Vehicle[]; me: Me }) {
  const { err, run } = useErr();
  const [list, setList] = useState<Issue[]>([]);
  const [status, setStatus] = useState("open");
  const [edit, setEdit] = useState<Partial<Issue> | null>(null);
  const load = useCallback(() => void run(async () => setList(await fleet.issues(vehicleId, status))), [run, vehicleId, status]);
  useEffect(load, [load]);
  const vname = (id: string) => vehicles.find((v) => v.id === id)?.name ?? "";
  return (
    <Card title="Issues" action={<Button size="sm" onClick={() => setEdit({ vehicle_id: vehicleId || vehicles[0]?.id, title: "", priority: "medium" })}>Report an issue</Button>}>
      <ErrorAlert err={err} />
      <Row sx={{ mb: 1 }}>{["open", "resolved", "closed", ""].map((s) => <Chip key={s} variant={status === s ? "solid" : "outlined"} onClick={() => setStatus(s)} data-testid={`issue-filter-${s || "all"}`}>{s ? STATUS_LABEL[s] : "All"}</Chip>)}</Row>
      <Box sx={{ overflowX: "auto" }}>
        <Table size="sm" aria-label="Issues">
          <thead><tr>{!vehicleId && <th>Vehicle</th>}<th>Issue</th><th>Priority</th><th>Status</th><th>Reported</th><th /></tr></thead>
          <tbody>
            {list.map((i) => (
              <tr key={i.id}>
                {!vehicleId && <td>{vname(i.vehicle_id)}</td>}
                <td>{i.title}{i.source === "inspection" && <Chip size="sm" variant="outlined" sx={{ ml: 0.5 }}>inspection</Chip>}<Typography level="body-xs">{i.description}</Typography></td>
                <td><Chip size="sm" variant="soft" color={STATUS_COLOR[i.priority]}>{i.priority}</Chip></td>
                <td><Chip size="sm" variant="soft" color={STATUS_COLOR[i.status]}>{STATUS_LABEL[i.status]}</Chip></td>
                <td>{i.created_at ? fmtDay(i.created_at) : "—"}</td>
                <td>{me.admin && <Button size="sm" variant="outlined" onClick={() => setEdit(i)}>Manage</Button>}</td>
              </tr>
            ))}
          </tbody>
        </Table>
      </Box>
      {!list.length && <Typography level="body-sm" sx={{ mt: 1 }}>No issues here.</Typography>}
      {edit && <IssueDialog i={edit} vehicles={vehicles} fixed={!!vehicleId} admin={me.admin} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); load(); }} />}
    </Card>
  );
}

function IssueDialog({ i, vehicles, fixed, admin, onClose, onSaved }: { i: Partial<Issue>; vehicles: Vehicle[]; fixed: boolean; admin: boolean; onClose: () => void; onSaved: () => void }) {
  const { err, run } = useErr();
  const [x, setX] = useState<Partial<Issue>>(i);
  return (
    <Dialog open onClose={onClose} title={i.id ? "Manage issue" : "Report an issue"}>
      <ErrorAlert err={err} />
      <Field label="Vehicle"><Select value={x.vehicle_id ?? ""} disabled={fixed || !!i.id} onChange={(_, v) => setX({ ...x, vehicle_id: v ?? "" })} slotProps={{ button: { "aria-label": "Vehicle" } }}>{vehicles.map((v) => <Option key={v.id} value={v.id}>{v.name}</Option>)}</Select></Field>
      <Field label="What is wrong?" sx={{ mt: 1 }}><Input value={x.title ?? ""} onChange={(e) => setX({ ...x, title: e.target.value })} slotProps={{ input: { "aria-label": "Issue title" } }} /></Field>
      <Field label="Details" sx={{ mt: 1 }}><Textarea minRows={2} value={x.description ?? ""} onChange={(e) => setX({ ...x, description: e.target.value })} /></Field>
      <Row sx={{ mt: 1 }}>
        <Field label="Priority" sx={{ maxWidth: 160 }}><Select value={x.priority ?? "medium"} onChange={(_, v) => setX({ ...x, priority: v ?? "medium" })} slotProps={{ button: { "aria-label": "Priority" } }}><Option value="low">Low</Option><Option value="medium">Medium</Option><Option value="high">High</Option></Select></Field>
        {i.id && admin && <Field label="Status" sx={{ maxWidth: 160 }}><Select value={x.status ?? "open"} onChange={(_, v) => setX({ ...x, status: v ?? "open" })} slotProps={{ button: { "aria-label": "Issue status" } }}><Option value="open">Open</Option><Option value="resolved">Resolved</Option><Option value="closed">Closed</Option></Select></Field>}
      </Row>
      {i.id && admin && <Field label="Resolution" sx={{ mt: 1 }}><Input value={x.resolution ?? ""} onChange={(e) => setX({ ...x, resolution: e.target.value })} slotProps={{ input: { "aria-label": "Resolution" } }} /></Field>}
      <Row sx={{ mt: 2, justifyContent: "flex-end" }}><Button variant="plain" onClick={onClose}>Cancel</Button><Button onClick={() => void run(async () => { await fleet.saveIssue(i.id ?? "", x); onSaved(); })}>Save</Button></Row>
    </Dialog>
  );
}

/** Pre-trip inspection: large Pass / Fail buttons, notes on failures. Each failed item opens an issue. */
export function Inspect({ vehicles, settings }: { vehicles: Vehicle[]; settings: Settings }) {
  const { err, run } = useErr();
  const [vehicle, setVehicle] = useState(vehicles[0]?.id ?? "");
  const [items, setItems] = useState<InspectionItem[]>([]);
  const [meter, setMeter] = useState(0);
  const [result, setResult] = useState<Inspection | null>(null);
  const [history, setHistory] = useState<Inspection[]>([]);
  const load = useCallback(() => void run(async () => { setItems((await fleet.checklist()).map((label) => ({ label, result: "pass", note: "" }))); setHistory((await fleet.inspections()).slice(0, 8)); }), [run]);
  useEffect(load, [load]);
  const set = (i: number, p: Partial<InspectionItem>) => setItems(items.map((x, j) => (j === i ? { ...x, ...p } : x)));
  const vname = (id: string) => vehicles.find((v) => v.id === id)?.name ?? "";
  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
      <Card title="Pre-trip inspection">
        <ErrorAlert err={err} />
        {result && <Alert color={result.failed ? "warning" : "success"} variant="soft" sx={{ mb: 1 }} data-testid="inspection-result">{result.failed ? `Submitted: ${result.failed} failed item(s), ${result.issue_ids.length} issue(s) opened.` : "Submitted: all items passed."}</Alert>}
        <Row sx={{ alignItems: "flex-end" }}>
          <Field label="Vehicle" sx={{ maxWidth: 280 }}><Select value={vehicle} onChange={(_, v) => setVehicle(v ?? "")} slotProps={{ button: { "aria-label": "Inspected vehicle" } }}>{vehicles.map((v) => <Option key={v.id} value={v.id}>{v.name}{v.plate ? ` · ${v.plate}` : ""}</Option>)}</Select></Field>
          <Field label={`Meter (${settings.meter_unit}, optional)`} sx={{ maxWidth: 220 }}><Input type="number" value={meter || ""} onChange={(e) => setMeter(Number(e.target.value))} slotProps={{ input: { "aria-label": "Inspection meter", min: 0 } }} /></Field>
          <Button size="sm" variant="outlined" onClick={() => setItems(items.map((x) => ({ ...x, result: "pass", note: "" })))}>Pass all</Button>
        </Row>
        {items.map((it, i) => (
          <Box key={it.label} sx={{ mt: 1, p: 1, border: "1px solid", borderColor: it.result === "fail" ? "danger.300" : "divider", borderRadius: "sm" }}>
            <Row sx={{ alignItems: "center" }}>
              <Typography level="body-md" sx={{ flex: 1, minWidth: 180 }}>{it.label}</Typography>
              <Button size="lg" variant={it.result === "pass" ? "solid" : "outlined"} color="success" onClick={() => set(i, { result: "pass" })} aria-label={`${it.label}: pass`} aria-pressed={it.result === "pass"}>Pass</Button>
              <Button size="lg" variant={it.result === "fail" ? "solid" : "outlined"} color="danger" onClick={() => set(i, { result: "fail" })} aria-label={`${it.label}: fail`} aria-pressed={it.result === "fail"}>Fail</Button>
              <Button size="lg" variant={it.result === "na" ? "solid" : "outlined"} color="neutral" onClick={() => set(i, { result: "na" })} aria-label={`${it.label}: not applicable`} aria-pressed={it.result === "na"}>N/A</Button>
            </Row>
            {it.result === "fail" && <Input sx={{ mt: 0.75 }} placeholder="What is wrong?" value={it.note} onChange={(e) => set(i, { note: e.target.value })} slotProps={{ input: { "aria-label": `${it.label} note` } }} />}
          </Box>
        ))}
        <Row sx={{ mt: 2, justifyContent: "flex-end" }}>
          <Button disabled={!vehicle || !items.length} onClick={() => void run(async () => { const r = await fleet.submitInspection({ vehicle_id: vehicle, date: todayISO(), meter, items }); setResult(r); setMeter(0); setItems(items.map((x) => ({ ...x, result: "pass", note: "" }))); load(); })}>Submit inspection</Button>
        </Row>
      </Card>
      <Card title="Recent inspections">
        {!history.length && <Typography level="body-sm">None yet.</Typography>}
        {history.length > 0 && <Table size="sm" aria-label="Recent inspections"><thead><tr><th>Date</th><th>Vehicle</th><th>By</th><th>Result</th></tr></thead><tbody>{history.map((h) => <tr key={h.id}><td>{fmtDay(h.date)}</td><td>{vname(h.vehicle_id)}</td><td>{h.by_name || "—"}</td><td><Chip size="sm" variant="soft" color={h.failed ? "danger" : "success"}>{h.failed ? `${h.failed} failed` : "Passed"}</Chip></td></tr>)}</tbody></Table>}
      </Card>
    </Box>
  );
}

export function FleetSettings({ settings, onSaved }: { settings: Settings; onSaved: (s: Settings) => void }) {
  const { err, run } = useErr();
  const [s, setS] = useState(settings);
  const [ok, setOk] = useState(false);
  return (
    <Card title="Fleet settings">
      <ErrorAlert err={err} />
      <Row>
        <Field label="Meter unit" sx={{ maxWidth: 160 }}><Select value={s.meter_unit} onChange={(_, v) => setS({ ...s, meter_unit: v ?? "km" })} slotProps={{ button: { "aria-label": "Meter unit" } }}><Option value="km">Kilometres</Option><Option value="mi">Miles</Option></Select></Field>
        <Field label="Due soon (days)" sx={{ maxWidth: 150 }}><Input type="number" value={s.due_soon_days} onChange={(e) => setS({ ...s, due_soon_days: Number(e.target.value) })} slotProps={{ input: { "aria-label": "Due soon days", min: 0 } }} /></Field>
        <Field label="Due soon (meter)" sx={{ maxWidth: 150 }}><Input type="number" value={s.due_soon_meter} onChange={(e) => setS({ ...s, due_soon_meter: Number(e.target.value) })} slotProps={{ input: { "aria-label": "Due soon meter", min: 0 } }} /></Field>
        <Field label="Currency" sx={{ maxWidth: 110 }}><Input value={s.currency} onChange={(e) => setS({ ...s, currency: e.target.value.toUpperCase().slice(0, 3) })} /></Field>
      </Row>
      <Field label="Inspection checklist (one item per line)" sx={{ mt: 1 }}><Textarea minRows={6} value={s.inspection_items.join("\n")} onChange={(e) => setS({ ...s, inspection_items: e.target.value.split("\n") })} slotProps={{ textarea: { "aria-label": "Inspection checklist" } }} /></Field>
      <Row sx={{ mt: 1.5, alignItems: "center" }}><Button onClick={() => void run(async () => { onSaved(await fleet.saveSettings(s)); setOk(true); })}>Save settings</Button>{ok && <Typography level="body-sm" color="success">Saved.</Typography>}</Row>
    </Card>
  );
}
