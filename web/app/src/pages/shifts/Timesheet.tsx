import { useCallback, useEffect, useState } from "react";
import { Alert, Box, Button, Chip, Input, Option, Select, Table, Typography } from "@mui/joy";
import { Card, Dialog, DownloadLink, ErrorAlert, Field, Row, Stat, useErr } from "../hr/ui";
import { fmtDay, fmtMonth, fmtRange, money, monthEnd, monthStart, todayISO } from "../ops/dates";
import { shiftsApi } from "./api";
import type { Me, PayLine, PayResult, TimesheetRow, TimesheetSummary } from "./api";

export function Timesheet({ me }: { me: Me }) {
  const { err, run } = useErr();
  const [from, setFrom] = useState(monthStart(todayISO()));
  const [to, setTo] = useState(monthEnd(todayISO()));
  const [rows, setRows] = useState<TimesheetRow[]>([]);
  const [sum, setSum] = useState<TimesheetSummary[]>([]);
  const [rec, setRec] = useState<TimesheetRow | null>(null);
  const load = useCallback(() => void run(async () => { const r = await shiftsApi.timesheet(from, to); setRows(r.rows); setSum(r.summary); }), [run, from, to]);
  useEffect(load, [load]);
  const tot = sum.reduce((a, s) => ({ sched: a.sched + s.scheduled_hours, paid: a.paid + s.paid_hours, late: a.late + s.late, absent: a.absent + s.absent }), { sched: 0, paid: 0, late: 0, absent: 0 });
  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
      <Card title="Attendance and timesheet">
        <ErrorAlert err={err} />
        <Row sx={{ alignItems: "flex-end" }}>
          <Field label="From" sx={{ maxWidth: 190 }}><Input type="date" value={from} onChange={(e) => setFrom(e.target.value)} slotProps={{ input: { "aria-label": "Timesheet from" } }} /></Field>
          <Field label="To" sx={{ maxWidth: 190 }}><Input type="date" value={to} onChange={(e) => setTo(e.target.value)} slotProps={{ input: { "aria-label": "Timesheet to" } }} /></Field>
          <DownloadLink href={shiftsApi.csvUrl(from, to)} testId="timesheet-csv">Download CSV</DownloadLink>
        </Row>
        <Row sx={{ mt: 1.5 }}>
          <Stat label="Scheduled hours" value={tot.sched.toFixed(1)} />
          <Stat label="Paid hours" value={tot.paid.toFixed(1)} hint="clocked time, else scheduled" />
          <Stat label="Late" value={tot.late} />
          <Stat label="Absent" value={tot.absent} />
        </Row>
        {sum.length > 0 && (
          <Box sx={{ overflowX: "auto", mt: 1.5 }}>
            <Table size="sm" aria-label="Summary by employee">
              <thead><tr><th>Employee</th><th>Shifts</th><th>Scheduled</th><th>Worked</th><th>Paid</th><th>Present</th><th>Late</th><th>Absent</th><th>Not recorded</th>{me.scheduler && <th>Pay</th>}</tr></thead>
              <tbody>{sum.map((s) => <tr key={s.employee_id}><td>{s.employee}</td><td>{s.shifts}</td><td>{s.scheduled_hours}</td><td>{s.worked_hours}</td><td>{s.paid_hours}</td><td>{s.present}</td><td>{s.late}</td><td>{s.absent}</td><td>{s.not_recorded}</td>{me.scheduler && <td>{s.pay != null ? money(s.pay) : "—"}</td>}</tr>)}</tbody>
            </Table>
          </Box>
        )}
        <Box sx={{ overflowX: "auto", mt: 1.5 }}>
          <Table size="sm" aria-label="Shift rows">
            <thead><tr><th>Date</th><th>Employee</th><th>Shift</th><th>Status</th><th>Clock</th><th>Paid h</th><th /></tr></thead>
            <tbody>
              {rows.map((r, i) => (
                <tr key={i}>
                  <td>{fmtDay(r.date)}</td><td>{r.employee}</td><td>{fmtRange(r.start, r.end)} <Typography level="body-xs">{r.role}</Typography></td>
                  <td><Chip size="sm" color={r.status === "absent" ? "danger" : r.status === "late" ? "warning" : r.status === "present" ? "success" : "neutral"}>{r.status}</Chip></td>
                  <td>{r.clock_in ? `${r.clock_in} – ${r.clock_out || "…"}` : "—"}{r.minutes_late > 0 && <Typography level="body-xs" color="warning"> {r.minutes_late} min late</Typography>}</td>
                  <td>{r.paid_hours}</td>
                  <td>{me.scheduler && <ShiftLink row={r} onClick={() => setRec(r)} />}</td>
                </tr>
              ))}
            </tbody>
          </Table>
          {!rows.length && <Typography level="body-sm" sx={{ mt: 1 }}>No assigned shifts in this range.</Typography>}
        </Box>
      </Card>
      {me.hr_admin && <PayrollExport />}
      {rec && <RecordDialog row={rec} onClose={() => setRec(null)} onSaved={() => { setRec(null); load(); }} />}
    </Box>
  );
}

const ShiftLink = ({ row, onClick }: { row: TimesheetRow; onClick: () => void }) => <Button size="sm" variant="plain" onClick={onClick} aria-label={`Record attendance ${row.employee} ${row.date}`}>Record</Button>;

function RecordDialog({ row, onClose, onSaved }: { row: TimesheetRow; onClose: () => void; onSaved: () => void }) {
  const { err, run } = useErr();
  const [status, setStatus] = useState(row.status === "not recorded" ? "present" : row.status);
  const [inT, setIn] = useState(row.clock_in || row.start);
  const [outT, setOut] = useState(row.clock_out || row.end);
  return (
    <Dialog open onClose={onClose} title={`Attendance: ${row.employee}, ${fmtDay(row.date)}`}>
      <ErrorAlert err={err} />
      <Row>
        <Field label="Status"><Select value={status} onChange={(_, v) => setStatus(v ?? "present")} slotProps={{ button: { "aria-label": "Attendance status" } }}><Option value="present">Present</Option><Option value="late">Late</Option><Option value="absent">Absent</Option></Select></Field>
        <Field label="Clock in"><Input type="time" disabled={status === "absent"} value={inT} onChange={(e) => setIn(e.target.value)} slotProps={{ input: { "aria-label": "Clock in" } }} /></Field>
        <Field label="Clock out"><Input type="time" disabled={status === "absent"} value={outT} onChange={(e) => setOut(e.target.value)} slotProps={{ input: { "aria-label": "Clock out" } }} /></Field>
      </Row>
      <Row sx={{ mt: 2, justifyContent: "flex-end" }}>
        <Button variant="plain" onClick={onClose}>Cancel</Button>
        <Button onClick={() => void run(async () => {
          await shiftsApi.setAttendance(row.shift_id, { status, clock_in: status === "absent" ? "" : inT, clock_out: status === "absent" ? "" : outT });
          onSaved();
        })}>Save</Button>
      </Row>
    </Dialog>
  );
}

function PayrollExport() {
  const { err, run } = useErr();
  const [period, setPeriod] = useState(todayISO().slice(0, 7));
  const [lines, setLines] = useState<PayLine[] | null>(null);
  const [res, setRes] = useState<PayResult | null>(null);
  return (
    <Card title="Export hours to HR payroll">
      <ErrorAlert err={err} />
      <Typography level="body-sm" sx={{ mb: 1 }}>Adds one earning line per employee (paid hours × hourly rate) to the draft payroll run of the month in HR. Exporting again replaces the earlier line, it never doubles it.</Typography>
      <Row sx={{ alignItems: "flex-end" }}>
        <Field label="Month" sx={{ maxWidth: 180 }}><Input type="month" value={period} onChange={(e) => { setPeriod(e.target.value); setLines(null); setRes(null); }} slotProps={{ input: { "aria-label": "Payroll month" } }} /></Field>
        <Button variant="outlined" onClick={() => void run(async () => { setRes(null); setLines(await shiftsApi.payPreview(period)); })}>Preview {period ? fmtMonth(period + "-01") : ""}</Button>
        <Button disabled={!lines?.length} onClick={() => void run(async () => setRes(await shiftsApi.payExport(period)))}>Export to payroll</Button>
      </Row>
      {lines && !res && <Typography level="body-sm" sx={{ mt: 1 }}>{lines.length ? `${lines.length} employee(s), ${lines.reduce((n, l) => n + l.hours, 0).toFixed(2)} h, ${money(lines.reduce((n, l) => n + l.amount, 0))}` : "No shifts with a pay rate in that month."}</Typography>}
      {res && (
        <Box sx={{ mt: 1 }} data-testid="payroll-result">
          <Alert color="success" variant="soft">Added to payroll run {res.period}: {res.applied.length} employee(s){res.created_run ? " (a draft run was created)" : ""}.</Alert>
          {res.skipped.map((s) => <Alert key={s.employee_id} color="warning" variant="soft" sx={{ mt: 0.5 }}>Skipped one employee: {s.reason}</Alert>)}
        </Box>
      )}
    </Card>
  );
}
