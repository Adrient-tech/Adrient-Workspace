import { useCallback, useEffect, useState } from "react";
import { Box, Button, Checkbox, Input, Table, Typography } from "@mui/joy";
import { Card, ErrorAlert, Field, Row, useErr } from "../hr/ui";
import { DAY_SHORT } from "../ops/dates";
import { shiftsApi } from "./api";
import type { Coverage, Settings as S } from "./api";

export function ShiftSettings() {
  const { err, run } = useErr();
  const [s, setS] = useState<S | null>(null);
  const [cov, setCov] = useState<Coverage[]>([]);
  const [nc, setNc] = useState<Coverage>({ id: "", role: "", days: [1, 2, 3, 4, 5], count: 1, start: "", end: "" });
  const [saved, setSaved] = useState(false);
  const load = useCallback(() => void run(async () => { setS(await shiftsApi.settings()); setCov(await shiftsApi.coverage()); }), [run]);
  useEffect(load, [load]);
  if (!s) return <ErrorAlert err={err} />;
  const n = (v: string) => Number(v) || 0;
  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
      <Card title="Rota rules and costs">
        <ErrorAlert err={err} />
        <Typography level="body-sm" sx={{ mb: 1 }}>All limits are optional (0 = off). They warn when you assign shifts, fill open shifts or approve swaps; they never block.</Typography>
        <Row>
          <Field label="Minimum rest between shifts (h)"><Input type="number" value={s.rules.min_rest_hours} onChange={(e) => setS({ ...s, rules: { ...s.rules, min_rest_hours: n(e.target.value) } })} slotProps={{ input: { "aria-label": "Minimum rest hours", min: 0 } }} /></Field>
          <Field label="Most hours in a day"><Input type="number" value={s.rules.max_daily_hours} onChange={(e) => setS({ ...s, rules: { ...s.rules, max_daily_hours: n(e.target.value) } })} slotProps={{ input: { "aria-label": "Maximum daily hours", min: 0 } }} /></Field>
          <Field label="Most days in a row"><Input type="number" value={s.rules.max_consecutive_days} onChange={(e) => setS({ ...s, rules: { ...s.rules, max_consecutive_days: n(e.target.value) } })} slotProps={{ input: { "aria-label": "Maximum consecutive days", min: 0 } }} /></Field>
        </Row>
        <Row sx={{ mt: 1 }}>
          <Field label="Overtime after (h / week)"><Input type="number" value={s.overtime_threshold_hours} onChange={(e) => setS({ ...s, overtime_threshold_hours: n(e.target.value) })} slotProps={{ input: { "aria-label": "Overtime threshold", min: 0 } }} /></Field>
          <Field label="Overtime multiplier"><Input type="number" value={s.overtime_multiplier} onChange={(e) => setS({ ...s, overtime_multiplier: n(e.target.value) })} slotProps={{ input: { "aria-label": "Overtime multiplier", min: 0, step: 0.25 } }} /></Field>
          <Field label="Weekly labour budget"><Input type="number" value={s.weekly_budget} onChange={(e) => setS({ ...s, weekly_budget: n(e.target.value) })} slotProps={{ input: { "aria-label": "Weekly budget", min: 0 } }} /></Field>
          <Field label="Currency"><Input value={s.currency} onChange={(e) => setS({ ...s, currency: e.target.value.toUpperCase().slice(0, 3) })} /></Field>
          <Field label="Late grace (min)"><Input type="number" value={s.grace_minutes} onChange={(e) => setS({ ...s, grace_minutes: n(e.target.value) })} slotProps={{ input: { "aria-label": "Grace minutes", min: 0 } }} /></Field>
        </Row>
        <Row sx={{ mt: 1.5, alignItems: "center" }}>
          <Button onClick={() => void run(async () => { setS(await shiftsApi.saveSettings(s)); setSaved(true); })}>Save settings</Button>
          {saved && <Typography level="body-sm" color="success">Saved.</Typography>}
        </Row>
      </Card>
      <Card title="Minimum staffing (coverage)">
        <Typography level="body-sm" sx={{ mb: 1 }}>Days that fall below these numbers are flagged on the schedule. Open shifts count as planned cover.</Typography>
        {cov.length > 0 && (
          <Table size="sm" aria-label="Coverage requirements">
            <thead><tr><th>Role</th><th>Days</th><th>Minimum</th><th>Window</th><th /></tr></thead>
            <tbody>{cov.map((c) => <tr key={c.id}><td>{c.role}</td><td>{c.days.map((d) => DAY_SHORT[d]).join(", ")}</td><td>{c.count}</td><td>{c.start ? `${c.start}–${c.end}` : "all day"}</td><td><Button size="sm" variant="plain" color="danger" onClick={() => void run(async () => { await shiftsApi.deleteCoverage(c.id); load(); })}>Remove</Button></td></tr>)}</tbody>
          </Table>
        )}
        <Row sx={{ mt: 1.5, alignItems: "flex-end" }}>
          <Field label="Role"><Input value={nc.role} onChange={(e) => setNc({ ...nc, role: e.target.value })} slotProps={{ input: { "aria-label": "Coverage role" } }} /></Field>
          <Field label="Minimum people" sx={{ maxWidth: 140 }}><Input type="number" value={nc.count} onChange={(e) => setNc({ ...nc, count: n(e.target.value) })} slotProps={{ input: { "aria-label": "Coverage minimum", min: 1 } }} /></Field>
          <Field label="From (optional)" sx={{ maxWidth: 140 }}><Input type="time" value={nc.start} onChange={(e) => setNc({ ...nc, start: e.target.value })} /></Field>
          <Field label="To" sx={{ maxWidth: 140 }}><Input type="time" value={nc.end} onChange={(e) => setNc({ ...nc, end: e.target.value })} /></Field>
        </Row>
        <Row sx={{ mt: 1 }}>
          {DAY_SHORT.map((d, i) => <Checkbox key={d} size="sm" label={d} checked={nc.days.includes(i)} onChange={(e) => setNc({ ...nc, days: e.target.checked ? [...nc.days, i] : nc.days.filter((x) => x !== i) })} />)}
          <Button size="sm" disabled={!nc.role.trim() || !nc.days.length || nc.count < 1} onClick={() => void run(async () => { await shiftsApi.saveCoverage("", nc); setNc({ ...nc, role: "" }); load(); })}>Add requirement</Button>
        </Row>
      </Card>
    </Box>
  );
}
