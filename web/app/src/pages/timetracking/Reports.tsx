import { useCallback, useEffect, useState } from "react";
import { Box, Input, Option, Select, Table } from "@mui/joy";
import { Card, DownloadLink, ErrorAlert, Field, Row, Stat, useErr } from "../hr/ui";
import { money, monthEnd, monthStart, todayISO } from "../ops/dates";
import { tt } from "./api";
import type { Me, ReportRow } from "./api";
import { fmtDuration, toHours } from "./logic";

export function Reports({ me }: { me: Me }) {
  const { err, run } = useErr();
  const [from, setFrom] = useState(monthStart(todayISO()));
  const [to, setTo] = useState(monthEnd(todayISO()));
  const [group, setGroup] = useState("project");
  const [rows, setRows] = useState<ReportRow[]>([]);
  const [cur, setCur] = useState("INR");
  const load = useCallback(() => void run(async () => {
    setRows(await tt.report({ from, to }, group));
    if (me.admin) setCur((await tt.settings()).currency || "INR");
  }), [run, from, to, group, me.admin]);
  useEffect(load, [load]);
  const sec = rows.reduce((n, r) => n + r.seconds, 0);
  const billed = rows.reduce((n, r) => n + r.billed_seconds, 0);
  const amt = rows.reduce((n, r) => n + (r.amount ?? 0), 0);
  return (
    <Card title="Reports">
      <ErrorAlert err={err} />
      <Row sx={{ alignItems: "flex-end" }}>
        <Field label="From" sx={{ maxWidth: 180 }}><Input type="date" value={from} onChange={(e) => setFrom(e.target.value)} slotProps={{ input: { "aria-label": "Report from" } }} /></Field>
        <Field label="To" sx={{ maxWidth: 180 }}><Input type="date" value={to} onChange={(e) => setTo(e.target.value)} slotProps={{ input: { "aria-label": "Report to" } }} /></Field>
        <Field label="Group by" sx={{ maxWidth: 180 }}>
          <Select value={group} onChange={(_, v) => setGroup(v ?? "project")} slotProps={{ button: { "aria-label": "Group by" } }}>
            {me.admin && <Option value="client">Client</Option>}<Option value="project">Project</Option>{me.admin && <Option value="user">Person</Option>}<Option value="task">Task</Option><Option value="day">Day</Option>
          </Select>
        </Field>
        <DownloadLink href={tt.reportCsv({ from, to }, group)} testId="report-csv">Report CSV</DownloadLink>
        <DownloadLink href={tt.entriesCsv({ from, to })}>All entries CSV</DownloadLink>
      </Row>
      <Row sx={{ mt: 1.5 }}>
        <Stat label="Total time" value={fmtDuration(sec)} />
        <Stat label="Billable time" value={fmtDuration(billed)} hint="after rounding and minimums" />
        {me.admin && <Stat label="Amount" value={money(amt, cur)} testId="report-amount" />}
      </Row>
      <Box sx={{ overflowX: "auto", mt: 1.5 }}>
        <Table size="sm" aria-label="Report">
          <thead><tr><th>{group}</th><th>Entries</th><th>Hours</th><th>Billable hours</th>{me.admin && <th>Amount</th>}</tr></thead>
          <tbody>{rows.map((r) => <tr key={r.key}><td>{r.label || "—"}</td><td>{r.entries}</td><td>{toHours(r.seconds)}</td><td>{toHours(r.billed_seconds)}</td>{me.admin && <td>{money(r.amount ?? 0, cur)}</td>}</tr>)}</tbody>
        </Table>
      </Box>
    </Card>
  );
}
