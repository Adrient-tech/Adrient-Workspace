import { useCallback, useEffect, useState } from "react";
import { Alert, Box, Button, Checkbox, Chip, Input, Option, Select, Table, Typography } from "@mui/joy";
import { Card, Dialog, ErrorAlert, Field, Row, StatusChip, useErr } from "../hr/ui";
import { fmtMonth, money, monthEnd, todayISO } from "../ops/dates";
import { tt } from "./api";
import type { Client, Contact, InvoiceRow, PayLine, Project, Settings } from "./api";
import { fmtDuration } from "./logic";

const num = (v: string): number | null => (v === "" ? null : Number(v));

export function Billing() {
  const { err, run } = useErr();
  const [clients, setClients] = useState<Client[]>([]);
  const [projects, setProjects] = useState<Project[]>([]);
  const [invoices, setInvoices] = useState<InvoiceRow[]>([]);
  const [settings, setSettings] = useState<Settings | null>(null);
  const [ec, setEc] = useState<Partial<Client> | null>(null);
  const [ep, setEp] = useState<Partial<Project> | null>(null);
  const [note, setNote] = useState("");
  const load = useCallback(() => void run(async () => {
    const [c, p, i, s] = await Promise.all([tt.clients(true), tt.projects(true), tt.invoices(), tt.settings()]);
    setClients(c); setProjects(p); setInvoices(i); setSettings(s);
  }), [run]);
  useEffect(load, [load]);
  const cur = settings?.currency || "INR";
  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
      <ErrorAlert err={err} />
      {note && <Alert color="success" variant="soft" endDecorator={<Button size="sm" variant="plain" onClick={() => setNote("")}>Dismiss</Button>}>{note}</Alert>}
      <Card title="Clients" action={<Button size="sm" onClick={() => setEc({ name: "", color: "#0E7490" })}>New client</Button>}>
        <Box sx={{ overflowX: "auto" }}>
          <Table size="sm" aria-label="Clients">
            <thead><tr><th>Client</th><th>Rate / h</th><th>Rounding</th><th>Unbilled</th><th /></tr></thead>
            <tbody>
              {clients.map((c) => (
                <tr key={c.id} style={{ opacity: c.archived ? 0.5 : 1 }}>
                  <td>{c.name}{c.contact_id && <Chip size="sm" variant="outlined" sx={{ ml: 0.5 }}>Contacts</Chip>}</td>
                  <td>{c.rate != null ? money(c.rate, cur) : "default"}</td>
                  <td>{c.rounding ? `${c.rounding} min` : "default"}</td>
                  <td>{c.unbilled_seconds ? `${fmtDuration(c.unbilled_seconds)} · ${money(c.unbilled_amount ?? 0, cur)}` : "—"}</td>
                  <td>
                    <Row>
                      <Button size="sm" variant="outlined" onClick={() => setEc(c)}>Edit</Button>
                      <Button size="sm" disabled={!c.unbilled_seconds} onClick={() => { if (window.confirm(`Create an invoice for all unbilled time of ${c.name}? The time is locked afterwards.`)) void run(async () => { const r = await tt.invoice(c.id, "", todayISO()); setNote(`Invoice ${r.document.number} created for ${r.hours} h (${r.entries} entries).`); load(); }); }}>Invoice</Button>
                    </Row>
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Box>
        {!clients.length && <Typography level="body-sm">No clients yet.</Typography>}
      </Card>
      <Card title="Projects" action={<Button size="sm" onClick={() => setEp({ name: "", billable: true, client_id: "", color: "#126C5A" })}>New project</Button>}>
        <Box sx={{ overflowX: "auto" }}>
          <Table size="sm" aria-label="Projects">
            <thead><tr><th>Project</th><th>Client</th><th>Billable</th><th>Rate / h</th><th>Budget</th><th /></tr></thead>
            <tbody>
              {projects.map((p) => (
                <tr key={p.id} style={{ opacity: p.archived ? 0.5 : 1 }}>
                  <td>{p.name}</td><td>{clients.find((c) => c.id === p.client_id)?.name ?? "—"}</td><td>{p.billable ? "Yes" : "No"}</td>
                  <td>{p.rate != null ? money(p.rate, cur) : "inherit"}</td><td>{p.budget_hours ? `${p.budget_hours} h` : "—"}</td>
                  <td><Button size="sm" variant="outlined" onClick={() => setEp(p)}>Edit</Button></td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Box>
      </Card>
      <Card title="Invoices from tracked time">
        {!invoices.length && <Typography level="body-sm">No invoices yet. Use “Invoice” on a client with unbilled time.</Typography>}
        {invoices.length > 0 && (
          <Box sx={{ overflowX: "auto" }}>
            <Table size="sm" aria-label="Invoices">
              <thead><tr><th>Number</th><th>Client</th><th>Hours</th><th>Total</th><th>Status</th><th /></tr></thead>
              <tbody>{invoices.map((i) => <tr key={i.id}><td>{i.number || "—"}</td><td>{i.client}</td><td>{i.hours}</td><td>{money(i.total, cur)}</td><td><StatusChip status={String(i.status || "draft").toLowerCase()} /></td><td><Button component="a" href={`/invoicing?doc=${i.id}`} size="sm" variant="plain">Open in Invoicing</Button></td></tr>)}</tbody>
            </Table>
          </Box>
        )}
      </Card>
      {settings && <SettingsCard s={settings} onSaved={(v) => { setSettings(v); setNote("Settings saved."); }} />}
      <PayrollCard />
      {ec && <ClientDialog c={ec} onClose={() => setEc(null)} onSaved={() => { setEc(null); load(); }} />}
      {ep && <ProjectDialog p={ep} clients={clients} onClose={() => setEp(null)} onSaved={() => { setEp(null); load(); }} />}
    </Box>
  );
}

function ClientDialog({ c, onClose, onSaved }: { c: Partial<Client>; onClose: () => void; onSaved: () => void }) {
  const { err, run } = useErr();
  const [v, setV] = useState<Partial<Client>>(c);
  const [q, setQ] = useState("");
  const [found, setFound] = useState<Contact[]>([]);
  return (
    <Dialog open onClose={onClose} title={c.id ? `Edit ${c.name}` : "New client"}>
      <ErrorAlert err={err} />
      <Field label="Search Contacts to link (optional)">
        <Input value={q} onChange={(e) => { setQ(e.target.value); if (e.target.value.length > 1) void run(async () => setFound(await tt.contacts(e.target.value))); else setFound([]); }} slotProps={{ input: { "aria-label": "Search contacts" } }} />
      </Field>
      {found.map((x) => <Button key={x.id} size="sm" variant="soft" sx={{ mt: 0.5, mr: 0.5 }} onClick={() => { setV({ ...v, contact_id: x.id, name: v.name || x.name, email: v.email || x.email }); setFound([]); setQ(x.name); }}>{x.name}{x.company ? ` · ${x.company}` : ""}</Button>)}
      {v.contact_id && <Typography level="body-xs" sx={{ mt: 0.5 }}>Linked to a contact: invoices will use it as the customer.</Typography>}
      <Row sx={{ mt: 1 }}>
        <Field label="Name"><Input value={v.name ?? ""} onChange={(e) => setV({ ...v, name: e.target.value })} slotProps={{ input: { "aria-label": "Client name" } }} /></Field>
        <Field label="Email"><Input value={v.email ?? ""} onChange={(e) => setV({ ...v, email: e.target.value })} /></Field>
      </Row>
      <Row sx={{ mt: 1 }}>
        <Field label="Rate per hour (blank = default)"><Input type="number" value={v.rate ?? ""} onChange={(e) => setV({ ...v, rate: num(e.target.value) })} slotProps={{ input: { "aria-label": "Client rate", min: 0 } }} /></Field>
        <Field label="Rounding (min)"><Input type="number" value={v.rounding ?? ""} onChange={(e) => setV({ ...v, rounding: num(e.target.value) })} slotProps={{ input: { "aria-label": "Client rounding", min: 0 } }} /></Field>
        <Field label="Minimum (min)"><Input type="number" value={v.minimum_minutes ?? ""} onChange={(e) => setV({ ...v, minimum_minutes: num(e.target.value) })} /></Field>
      </Row>
      <Row sx={{ mt: 1 }}>
        <Field label="Tax % (blank = default)"><Input type="number" value={v.tax_rate ?? ""} onChange={(e) => setV({ ...v, tax_rate: num(e.target.value) })} /></Field>
        <Field label="Payment terms (days)"><Input type="number" value={v.terms_days ?? ""} onChange={(e) => setV({ ...v, terms_days: num(e.target.value) })} /></Field>
        <Field label="Currency"><Input value={v.currency ?? ""} onChange={(e) => setV({ ...v, currency: e.target.value.toUpperCase().slice(0, 3) })} /></Field>
      </Row>
      <Checkbox sx={{ mt: 1 }} label="Archived (hidden, history kept)" checked={!!v.archived} onChange={(e) => setV({ ...v, archived: e.target.checked })} />
      <Row sx={{ mt: 2, justifyContent: "flex-end" }}><Button variant="plain" onClick={onClose}>Cancel</Button><Button onClick={() => void run(async () => { await tt.saveClient(c.id ?? "", v); onSaved(); })}>Save client</Button></Row>
    </Dialog>
  );
}

function ProjectDialog({ p, clients, onClose, onSaved }: { p: Partial<Project>; clients: Client[]; onClose: () => void; onSaved: () => void }) {
  const { err, run } = useErr();
  const [v, setV] = useState<Partial<Project>>(p);
  return (
    <Dialog open onClose={onClose} title={p.id ? `Edit ${p.name}` : "New project"}>
      <ErrorAlert err={err} />
      <Row>
        <Field label="Name"><Input value={v.name ?? ""} onChange={(e) => setV({ ...v, name: e.target.value })} slotProps={{ input: { "aria-label": "Project name" } }} /></Field>
        <Field label="Client"><Select value={v.client_id ?? ""} onChange={(_, x) => setV({ ...v, client_id: x ?? "" })} slotProps={{ button: { "aria-label": "Project client" } }}><Option value="">No client</Option>{clients.filter((c) => !c.archived).map((c) => <Option key={c.id} value={c.id}>{c.name}</Option>)}</Select></Field>
      </Row>
      <Row sx={{ mt: 1 }}>
        <Field label="Rate per hour (blank = inherit)"><Input type="number" value={v.rate ?? ""} onChange={(e) => setV({ ...v, rate: num(e.target.value) })} slotProps={{ input: { "aria-label": "Project rate", min: 0 } }} /></Field>
        <Field label="Budget (hours)"><Input type="number" value={v.budget_hours ?? 0} onChange={(e) => setV({ ...v, budget_hours: Number(e.target.value) })} /></Field>
        <Field label="Budget (amount)"><Input type="number" value={v.budget_amount ?? 0} onChange={(e) => setV({ ...v, budget_amount: Number(e.target.value) })} /></Field>
      </Row>
      <Row sx={{ mt: 1 }}>
        <Checkbox label="Billable" checked={v.billable !== false} onChange={(e) => setV({ ...v, billable: e.target.checked })} />
        <Checkbox label="Archived" checked={!!v.archived} onChange={(e) => setV({ ...v, archived: e.target.checked })} />
      </Row>
      <Row sx={{ mt: 2, justifyContent: "flex-end" }}><Button variant="plain" onClick={onClose}>Cancel</Button><Button onClick={() => void run(async () => { await tt.saveProject(p.id ?? "", v); onSaved(); })}>Save project</Button></Row>
    </Dialog>
  );
}

function SettingsCard({ s, onSaved }: { s: Settings; onSaved: (s: Settings) => void }) {
  const { err, run } = useErr();
  const [v, setV] = useState(s);
  const n = (x: string) => Number(x) || 0;
  return (
    <Card title="Billing defaults">
      <ErrorAlert err={err} />
      <Row>
        <Field label="Default rate / h"><Input type="number" value={v.default_rate} onChange={(e) => setV({ ...v, default_rate: n(e.target.value) })} slotProps={{ input: { "aria-label": "Default rate", min: 0 } }} /></Field>
        <Field label="Currency"><Input value={v.currency} onChange={(e) => setV({ ...v, currency: e.target.value.toUpperCase().slice(0, 3) })} /></Field>
        <Field label="Round billable time up to">
          <Select value={v.rounding} onChange={(_, x) => setV({ ...v, rounding: x ?? 0 })} slotProps={{ button: { "aria-label": "Rounding" } }}>
            {[0, 5, 6, 10, 15, 30, 60].map((m) => <Option key={m} value={m}>{m ? `${m} minutes` : "No rounding"}</Option>)}
          </Select>
        </Field>
        <Field label="Minimum billable (min)"><Input type="number" value={v.minimum_minutes} onChange={(e) => setV({ ...v, minimum_minutes: n(e.target.value) })} /></Field>
      </Row>
      <Row sx={{ mt: 1 }}>
        <Field label="Tax %"><Input type="number" value={v.tax_rate} onChange={(e) => setV({ ...v, tax_rate: n(e.target.value) })} /></Field>
        <Field label="Payment terms (days)"><Input type="number" value={v.terms_days} onChange={(e) => setV({ ...v, terms_days: n(e.target.value) })} /></Field>
        <Field label="Payroll rate / h (for hours exported to HR)"><Input type="number" value={v.payroll_rate} onChange={(e) => setV({ ...v, payroll_rate: n(e.target.value) })} slotProps={{ input: { "aria-label": "Payroll rate", min: 0 } }} /></Field>
      </Row>
      <Button sx={{ mt: 1.5 }} onClick={() => void run(async () => onSaved(await tt.saveSettings(v)))}>Save defaults</Button>
    </Card>
  );
}

function PayrollCard() {
  const { err, run } = useErr();
  const [period, setPeriod] = useState(todayISO().slice(0, 7));
  const [prev, setPrev] = useState<{ lines: PayLine[]; skipped: string[] } | null>(null);
  const [done, setDone] = useState("");
  return (
    <Card title="Export tracked hours to HR payroll">
      <ErrorAlert err={err} />
      <Typography level="body-sm" sx={{ mb: 1 }}>HR admins only. Adds an earning line (hours × payroll rate) to the draft payroll run of the month. People need an HR employee record and a payroll rate.</Typography>
      <Row sx={{ alignItems: "flex-end" }}>
        <Field label="Month" sx={{ maxWidth: 180 }}><Input type="month" value={period} onChange={(e) => { setPeriod(e.target.value); setPrev(null); setDone(""); }} slotProps={{ input: { "aria-label": "Payroll month" } }} /></Field>
        <Button variant="outlined" onClick={() => void run(async () => { setDone(""); setPrev(await tt.payPreview(period)); })}>Preview {fmtMonth(`${period}-01`)} ({monthEnd(`${period}-01`).slice(8)} days)</Button>
        <Button disabled={!prev?.lines.length} onClick={() => void run(async () => { const r = await tt.payExport(period); setDone(`Added to payroll run ${r.result.period}: ${r.result.applied.length} employee(s)${r.result.created_run ? " (draft run created)" : ""}.`); })}>Export to payroll</Button>
      </Row>
      {prev && <Typography level="body-sm" sx={{ mt: 1 }}>{prev.lines.length ? `${prev.lines.length} employee(s), ${prev.lines.reduce((a, l) => a + l.hours, 0).toFixed(2)} h, ${money(prev.lines.reduce((a, l) => a + l.amount, 0))}` : "Nothing to export."}</Typography>}
      {prev?.skipped.map((s) => <Alert key={s} color="warning" variant="soft" sx={{ mt: 0.5 }}>{s}</Alert>)}
      {done && <Alert color="success" variant="soft" sx={{ mt: 1 }} data-testid="tt-payroll-done">{done}</Alert>}
    </Card>
  );
}
