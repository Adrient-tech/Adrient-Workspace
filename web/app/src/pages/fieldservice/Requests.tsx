import { useCallback, useEffect, useState } from "react";
import { Alert, Box, Button, Chip, FormControl, FormLabel, Input, Table, Textarea, Typography } from "@mui/joy";
import { Card, Dialog, ErrorAlert, Field, Row, useErr } from "../hr/ui";
import { todayISO } from "../ops/dates";
import { fs } from "./api";
import type { Request, Service, Settings, Team } from "./api";
import { STATUS_COLOR, STATUS_LABEL, blankSchedule } from "./logic";
import { CustomerPicker, ItemsEditor, PropertyPicker } from "./parts";
import { ScheduleEditor, TeamPicker } from "./Jobs";

export function Requests({ settings, services, team, onOpenJob }: { settings: Settings; services: Service[]; team: Team[]; onOpenJob: (id: string) => void }) {
  const { err, run } = useErr();
  const [list, setList] = useState<Request[]>([]);
  const [filter, setFilter] = useState("");
  const [edit, setEdit] = useState<Partial<Request> | null>(null);
  const [toJob, setToJob] = useState<Request | null>(null);
  const [note, setNote] = useState("");
  const load = useCallback(() => void run(async () => setList(await fs.requests(filter))), [run, filter]);
  useEffect(load, [load]);
  return (
    <Card title="Requests" action={<Button size="sm" onClick={() => setEdit({ title: "", details: "", items: [], tax_rate: settings.tax_rate, preferred_date: "" })}>New request</Button>}>
      <ErrorAlert err={err} />
      {note && <Alert color="success" variant="soft" sx={{ mb: 1 }} endDecorator={<Button size="sm" variant="plain" onClick={() => setNote("")}>Dismiss</Button>}>{note}</Alert>}
      <Row sx={{ mb: 1 }}>
        {["", "new", "assessment", "quoted", "converted", "closed"].map((s) => <Chip key={s} variant={filter === s ? "solid" : "outlined"} onClick={() => setFilter(s)} data-testid={`req-filter-${s || "all"}`}>{s ? STATUS_LABEL[s] : "All"}</Chip>)}
      </Row>
      <Box sx={{ overflowX: "auto" }}>
        <Table size="sm" aria-label="Requests">
          <thead><tr><th>Request</th><th>Customer</th><th>Preferred</th><th>Status</th><th>Quote</th><th /></tr></thead>
          <tbody>
            {list.map((q) => (
              <tr key={q.id}>
                <td>{q.title}</td><td>{q.client_name}</td><td>{q.preferred_date || "—"}</td>
                <td><Chip size="sm" color={STATUS_COLOR[q.status]} variant="soft">{STATUS_LABEL[q.status] ?? q.status}</Chip></td>
                <td>{q.quote_number ? <Button component="a" size="sm" variant="plain" href={`/invoicing?doc=${q.quote_id}`}>{q.quote_number}</Button> : "—"}</td>
                <td>
                  <Row>
                    {q.status !== "converted" && <Button size="sm" variant="outlined" onClick={() => setEdit(q)}>Edit</Button>}
                    {!q.quote_id && q.status !== "converted" && q.status !== "closed" && <Button size="sm" variant="outlined" disabled={!q.items.length} title={q.items.length ? "" : "Add lines first"} onClick={() => void run(async () => { const r = await fs.quote(q.id); setNote(`Quote ${r.quote_number} created in Invoicing (draft). Send it from there.`); load(); })}>Create quote</Button>}
                    {q.status !== "converted" && q.status !== "closed" && <Button size="sm" onClick={() => setToJob(q)}>Create job</Button>}
                    {q.status === "new" && <Button size="sm" variant="plain" onClick={() => void run(async () => { await fs.moveRequest(q.id, "assessment"); load(); })}>Assess</Button>}
                    {q.status !== "converted" && q.status !== "closed" && <Button size="sm" variant="plain" color="neutral" onClick={() => void run(async () => { await fs.moveRequest(q.id, "closed"); load(); })}>Close</Button>}
                    {q.job_id && <Button size="sm" variant="plain" onClick={() => onOpenJob(q.job_id)}>Open job</Button>}
                  </Row>
                </td>
              </tr>
            ))}
          </tbody>
        </Table>
      </Box>
      {!list.length && <Typography level="body-sm" sx={{ mt: 1 }}>No requests here yet.</Typography>}
      {edit && <RequestDialog r={edit} settings={settings} services={services} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); load(); }} />}
      {toJob && <ToJobDialog r={toJob} team={team} onClose={() => setToJob(null)} onDone={(id) => { setToJob(null); load(); onOpenJob(id); }} />}
    </Card>
  );
}

function RequestDialog({ r, settings, services, onClose, onSaved }: { r: Partial<Request>; settings: Settings; services: Service[]; onClose: () => void; onSaved: () => void }) {
  const { err, run } = useErr();
  const [v, setV] = useState<Partial<Request>>(r);
  return (
    <Dialog open onClose={onClose} title={r.id ? "Edit request" : "New request"} wide>
      <ErrorAlert err={err} />
      <CustomerPicker value={v.contact_id ?? ""} name={v.client_name ?? ""} onPick={(c) => setV({ ...v, contact_id: c.id, client_name: c.name, property_id: "" })} />
      <Box sx={{ mt: 1 }}><PropertyPicker contact={v.contact_id ?? ""} value={v.property_id ?? ""} onPick={(id) => setV({ ...v, property_id: id })} /></Box>
      <Row sx={{ mt: 1 }}>
        <Field label="Title"><Input value={v.title ?? ""} onChange={(e) => setV({ ...v, title: e.target.value })} slotProps={{ input: { "aria-label": "Request title" } }} /></Field>
        <Field label="Preferred date" sx={{ maxWidth: 200 }}><Input type="date" value={v.preferred_date ?? ""} onChange={(e) => setV({ ...v, preferred_date: e.target.value })} slotProps={{ input: { "aria-label": "Preferred date" } }} /></Field>
      </Row>
      <FormControl sx={{ mt: 1 }}><FormLabel>Details</FormLabel><Textarea minRows={2} value={v.details ?? ""} onChange={(e) => setV({ ...v, details: e.target.value })} /></FormControl>
      <Box sx={{ mt: 1 }}>
        <ItemsEditor items={v.items ?? []} onChange={(items) => setV({ ...v, items })} discount={null} onDiscount={() => undefined} taxRate={v.tax_rate ?? 0} onTax={(n) => setV({ ...v, tax_rate: n })} currency={settings.currency} services={services} />
      </Box>
      <Row sx={{ mt: 2, justifyContent: "flex-end" }}><Button variant="plain" onClick={onClose}>Cancel</Button><Button onClick={() => void run(async () => { await fs.saveRequest(r.id ?? "", v); onSaved(); })}>Save request</Button></Row>
    </Dialog>
  );
}

function ToJobDialog({ r, team, onClose, onDone }: { r: Request; team: Team[]; onClose: () => void; onDone: (jobID: string) => void }) {
  const { err, run } = useErr();
  const [s, setS] = useState(blankSchedule(r.preferred_date || todayISO()));
  const [who, setWho] = useState<string[]>([]);
  return (
    <Dialog open onClose={onClose} title={`Create a job from “${r.title}”`}>
      <ErrorAlert err={err} />
      <Typography level="body-sm" sx={{ mb: 1 }}>Lines and customer come from the request. Each visit is added to the assigned people's Calendar.</Typography>
      <ScheduleEditor s={s} onChange={setS} />
      <Box sx={{ mt: 1 }}><TeamPicker team={team} value={who} onChange={setWho} /></Box>
      <Row sx={{ mt: 2, justifyContent: "flex-end" }}><Button variant="plain" onClick={onClose}>Cancel</Button><Button onClick={() => void run(async () => { const j = await fs.requestToJob(r.id, s, who); onDone(j.id); })}>Create job</Button></Row>
    </Dialog>
  );
}
