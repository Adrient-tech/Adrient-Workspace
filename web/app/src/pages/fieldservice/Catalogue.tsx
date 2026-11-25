import { useState } from "react";
import { Button, Checkbox, Input, Option, Select, Table, Typography } from "@mui/joy";
import { Card, ErrorAlert, Field, Row, useErr } from "../hr/ui";
import { money } from "../ops/dates";
import { fs } from "./api";
import type { Service, Settings } from "./api";

export function Catalogue({ services, settings, admin, onChanged }: { services: Service[]; settings: Settings; admin: boolean; onChanged: () => void }) {
  const { err, run } = useErr();
  const [ns, setNs] = useState<Partial<Service>>({ name: "", unit_price: 0, unit_cost: 0, taxable: true });
  const [s, setS] = useState(settings);
  const [saved, setSaved] = useState(false);
  return (
    <>
      <Card title="Services catalogue">
        <ErrorAlert err={err} />
        <Typography level="body-sm" sx={{ mb: 1 }}>Services fill the lines of requests, quotes and jobs.{!admin && " Only admins can change the catalogue."}</Typography>
        <Table size="sm" aria-label="Services">
          <thead><tr><th>Service</th><th>Price</th><th>Cost</th><th>Taxable</th><th /></tr></thead>
          <tbody>
            {services.map((x) => <tr key={x.id}><td>{x.name}</td><td>{money(x.unit_price, settings.currency)}</td><td>{money(x.unit_cost, settings.currency)}</td><td>{x.taxable ? "Yes" : "No"}</td><td>{admin && <Button size="sm" variant="plain" color="danger" onClick={() => void run(async () => { await fs.saveService(x.id, { ...x, archived: true }); onChanged(); })}>Remove</Button>}</td></tr>)}
          </tbody>
        </Table>
        {admin && (
          <Row sx={{ mt: 1.5, alignItems: "flex-end" }}>
            <Field label="Name"><Input value={ns.name ?? ""} onChange={(e) => setNs({ ...ns, name: e.target.value })} slotProps={{ input: { "aria-label": "Service name" } }} /></Field>
            <Field label="Price" sx={{ maxWidth: 120 }}><Input type="number" value={ns.unit_price ?? 0} onChange={(e) => setNs({ ...ns, unit_price: Number(e.target.value) })} slotProps={{ input: { "aria-label": "Service price", min: 0 } }} /></Field>
            <Field label="Cost" sx={{ maxWidth: 120 }}><Input type="number" value={ns.unit_cost ?? 0} onChange={(e) => setNs({ ...ns, unit_cost: Number(e.target.value) })} slotProps={{ input: { "aria-label": "Service cost", min: 0 } }} /></Field>
            <Checkbox label="Taxable" checked={ns.taxable !== false} onChange={(e) => setNs({ ...ns, taxable: e.target.checked })} sx={{ pb: 1 }} />
            <Button disabled={!(ns.name ?? "").trim()} onClick={() => void run(async () => { await fs.saveService("", ns); setNs({ name: "", unit_price: 0, unit_cost: 0, taxable: true }); onChanged(); })}>Add service</Button>
          </Row>
        )}
      </Card>
      {admin && (
        <Card title="Defaults" sx={{ mt: 1.5 }}>
          <Row sx={{ alignItems: "flex-end" }}>
            <Field label="Tax %" sx={{ maxWidth: 110 }}><Input type="number" value={s.tax_rate} onChange={(e) => setS({ ...s, tax_rate: Number(e.target.value) })} slotProps={{ input: { "aria-label": "Default tax", min: 0 } }} /></Field>
            <Field label="Currency" sx={{ maxWidth: 110 }}><Input value={s.currency} onChange={(e) => setS({ ...s, currency: e.target.value.toUpperCase().slice(0, 3) })} /></Field>
            <Field label="Payment terms (days)" sx={{ maxWidth: 170 }}><Input type="number" value={s.terms_days} onChange={(e) => setS({ ...s, terms_days: Number(e.target.value) })} /></Field>
            <Field label="Invoice jobs" sx={{ maxWidth: 240 }}>
              <Select value={s.default_billing} onChange={(_, v) => setS({ ...s, default_billing: v ?? "on_completion" })} slotProps={{ button: { "aria-label": "Default billing" } }}><Option value="on_completion">When visits are done</Option><Option value="per_visit">After every visit</Option></Select>
            </Field>
            <Button onClick={() => void run(async () => { await fs.saveSettings(s); setSaved(true); onChanged(); })}>Save defaults</Button>
            {saved && <Typography level="body-sm" color="success">Saved.</Typography>}
          </Row>
        </Card>
      )}
    </>
  );
}
