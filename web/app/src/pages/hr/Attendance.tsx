import { LoadOlder } from "../../components/LoadOlder";
import { EmployeeMultiPicker } from "./EmployeePicker";
import { useCallback, useEffect, useState } from "react";
import { Box, Button, Checkbox, Input, Option, Select, Tab, TabList, TabPanel, Tabs, Table, Typography } from "@mui/joy";
import { hr } from "./api";
import type { ImportResult, MonthlyRow, Regularisation, Settings, Shift, Timesheet, Today } from "./api";
import { currentPeriod, minutesToHM, periodLabel, shiftPeriod, WEEKDAYS } from "./logic";
import { Card, DownloadLink, ErrorAlert, Field, FileButton, Row, StatusChip, useErr } from "./ui";
import { CheckInCard } from "./Home";

function Regularise({ onDone }: { onDone: () => void }) {
  const { err, run } = useErr();
  const [f, setF] = useState({ day: "", in_time: "09:30", out_time: "18:30", reason: "" });
  const [list, setList] = useState<Regularisation[]>([]);
  const load = useCallback(() => void run(async () => setList(await hr.regularisations("mine"))), [run]);
  useEffect(load, [load]);
  return (
    <Card title="Regularise attendance" sx={{ mt: 2 }}>
      <ErrorAlert err={err} />
      <Row>
        <Field label="Date"><Input type="date" value={f.day} onChange={(e) => setF({ ...f, day: e.target.value })} slotProps={{ input: { "aria-label": "Regularise date" } }} /></Field>
        <Field label="In"><Input type="time" value={f.in_time} onChange={(e) => setF({ ...f, in_time: e.target.value })} slotProps={{ input: { "aria-label": "In time" } }} /></Field>
        <Field label="Out"><Input type="time" value={f.out_time} onChange={(e) => setF({ ...f, out_time: e.target.value })} slotProps={{ input: { "aria-label": "Out time" } }} /></Field>
        <Field label="Reason" sx={{ flex: 2 }}><Input value={f.reason} onChange={(e) => setF({ ...f, reason: e.target.value })} slotProps={{ input: { "aria-label": "Regularise reason" } }} /></Field>
        <Button onClick={() => void run(async () => { await hr.requestRegularisation(f.day, f.in_time, f.out_time, f.reason); setF({ ...f, day: "", reason: "" }); load(); onDone(); })}>Send for approval</Button>
      </Row>
      {list.slice(0, 8).map((r) => (
        <Row key={r.id} sx={{ alignItems: "center", mt: 0.5 }}>
          <Typography level="body-sm">{r.day} {r.in_time}-{r.out_time} · {r.reason}</Typography><StatusChip status={r.status} />
          {r.can_cancel && <Button size="sm" variant="plain" color="danger" onClick={() => void run(async () => { await hr.decideRegularisation(r.id, "cancel"); load(); })}>Withdraw</Button>}
        </Row>
      ))}
    </Card>
  );
}

function MyAttendance() {
  const { err, run } = useErr();
  const [today, setToday] = useState<Today | null>(null);
  const [period, setPeriod] = useState(currentPeriod());
  const [ts, setTs] = useState<Timesheet | null>(null);
  const load = useCallback(() => void run(async () => {
    setToday(await hr.today());
    const me = await hr.me();
    if (me.employee) setTs(await hr.timesheet(me.employee.id, period));
  }), [period, run]);
  useEffect(load, [load]);
  return (
    <Box>
      <ErrorAlert err={err} />
      {today && <CheckInCard today={today} onChange={load} />}
      {ts && (
        <Card title={`Timesheet ${periodLabel(period)}`} sx={{ mt: 2 }} action={<Row><Button size="sm" variant="outlined" onClick={() => setPeriod(shiftPeriod(period, -1))}>‹</Button><Button size="sm" variant="outlined" onClick={() => setPeriod(shiftPeriod(period, 1))}>›</Button></Row>}>
          <Row sx={{ mb: 1 }}>
            <Typography level="body-sm">Present {ts.summary.Present}</Typography><Typography level="body-sm">Absent {ts.summary.Absent}</Typography><Typography level="body-sm">Leave {ts.summary.Leave}</Typography>
            <Typography level="body-sm">Late {ts.summary.LateCount}</Typography><Typography level="body-sm">Worked {minutesToHM(ts.summary.WorkedMin)}</Typography>
          </Row>
          <Table size="sm" aria-label="Timesheet" data-testid="timesheet">
            <thead><tr><th>Date</th><th>Status</th><th>In</th><th>Out</th><th>Worked</th><th>Flags</th></tr></thead>
            <tbody>
              {ts.days.map((d) => (
                <tr key={d.date}><td>{d.date}</td><td><StatusChip status={d.status} />{d.holiday ? ` ${d.holiday}` : ""}</td><td>{d.first_in || "—"}</td><td>{d.last_out || "—"}</td><td>{d.worked_min ? minutesToHM(d.worked_min) : "—"}</td>
                  <td>{d.late ? `late ${d.late_min}m ` : ""}{d.early_out ? "early out " : ""}{d.ot_min ? `OT ${minutesToHM(d.ot_min)}` : ""}</td></tr>
              ))}
            </tbody>
          </Table>
        </Card>
      )}
      <Regularise onDone={load} />
    </Box>
  );
}

function TeamReport() {
  const { err, run } = useErr();
  const [period, setPeriod] = useState(currentPeriod());
  const [rows, setRows] = useState<MonthlyRow[]>([]);
  const [nextRows, setNextRows] = useState("");
  // The report is worked out per employee, so it loads 100 employees at a time.
  useEffect(() => { void run(async () => { const pg = await hr.attendanceReportPage(period); setRows(pg.items); setNextRows(pg.next); }); }, [period, run]);
  return (
    <Card title="Monthly attendance report" action={<Row><Input size="sm" type="month" value={period} onChange={(e) => setPeriod(e.target.value)} slotProps={{ input: { "aria-label": "Report month" } }} /><DownloadLink href={hr.attendanceReportUrl(period)} testId="attendance-csv">CSV</DownloadLink></Row>}>
      <ErrorAlert err={err} />
      <Box sx={{ overflowX: "auto" }}>
        <Table size="sm" aria-label="Attendance report" data-testid="attendance-report">
          <thead><tr><th>Employee</th><th>Present</th><th>Half</th><th>Absent</th><th>Leave</th><th>Holidays</th><th>Offs</th><th>Late</th><th>Hours</th><th>OT</th><th>LOP</th></tr></thead>
          <tbody>{rows.map((r) => <tr key={r.employee_id}><td>{r.name}<Typography level="body-xs">{r.emp_code}</Typography></td><td>{r.present}</td><td>{r.half_days}</td><td>{r.absent}</td><td>{r.leave}</td><td>{r.holidays}</td><td>{r.weekly_offs}</td><td>{r.late}</td><td>{r.worked_hours}</td><td>{r.ot_hours}</td><td>{r.lop}</td></tr>)}</tbody>
        </Table>
        <LoadOlder next={nextRows} shown={rows.length} label="Load more employees" testId="att-more" loadMore={async (c) => {
          const pg = await hr.attendanceReportPage(period, Number(c));
          setRows((x) => [...x, ...pg.items]);
          setNextRows(pg.next);
        }} />
      </Box>
    </Card>
  );
}

function Shifts() {
  const { err, run } = useErr();
  const [shifts, setShifts] = useState<Shift[]>([]);
  const [r, setR] = useState({ ids: [] as string[], from: "", to: "", shift: "" });
  const [n, setN] = useState<Partial<Shift>>({ name: "", start_time: "09:30", end_time: "18:30", grace_minutes: 10, early_minutes: 10, full_day_mins: 480, half_day_mins: 240, ot_after_mins: 540, is_default: false });
  const load = useCallback(() => void run(async () => { setShifts(await hr.shifts()); }), [run]);
  useEffect(load, [load]);
  return (
    <Box sx={{ display: "grid", gap: 2 }}>
      <ErrorAlert err={err} />
      <Card title="Shifts">
        {shifts.map((s) => (
          <Row key={s.id} sx={{ alignItems: "center", justifyContent: "space-between" }}>
            <Typography level="body-sm">{s.name}{s.is_default ? " (default)" : ""} · {s.start_time}-{s.end_time} · grace {s.grace_minutes}m · full day {minutesToHM(s.full_day_mins)} · overtime after {minutesToHM(s.ot_after_mins)}</Typography>
            <Button size="sm" variant="plain" color="danger" onClick={() => void run(async () => { await hr.deleteShift(s.id); load(); })}>Delete</Button>
          </Row>
        ))}
        <Row sx={{ mt: 1 }}>
          <Field label="Name"><Input size="sm" value={n.name ?? ""} onChange={(e) => setN({ ...n, name: e.target.value })} slotProps={{ input: { "aria-label": "Shift name" } }} /></Field>
          <Field label="Start"><Input size="sm" type="time" value={n.start_time} onChange={(e) => setN({ ...n, start_time: e.target.value })} /></Field>
          <Field label="End"><Input size="sm" type="time" value={n.end_time} onChange={(e) => setN({ ...n, end_time: e.target.value })} /></Field>
          <Field label="Grace (min)"><Input size="sm" type="number" value={n.grace_minutes} onChange={(e) => setN({ ...n, grace_minutes: Number(e.target.value) })} /></Field>
          <Field label="Full day (min)"><Input size="sm" type="number" value={n.full_day_mins} onChange={(e) => setN({ ...n, full_day_mins: Number(e.target.value) })} /></Field>
          <Field label="Half day (min)"><Input size="sm" type="number" value={n.half_day_mins} onChange={(e) => setN({ ...n, half_day_mins: Number(e.target.value) })} /></Field>
          <Field label="Overtime after (min)"><Input size="sm" type="number" value={n.ot_after_mins} onChange={(e) => setN({ ...n, ot_after_mins: Number(e.target.value) })} /></Field>
          <Checkbox label="Default" checked={!!n.is_default} onChange={(e) => setN({ ...n, is_default: e.target.checked })} />
          <Button size="sm" onClick={() => void run(async () => { await hr.saveShift("", n); load(); })}>Add shift</Button>
        </Row>
      </Card>
      <Card title="Roster">
        <Typography level="body-xs" sx={{ mb: 1 }}>Assign a shift, or a day off, to employees for a date range. Employees without a roster entry use their own shift or the default.</Typography>
        <Row>
          <Field label="Employees" sx={{ minWidth: 260 }}>
            <EmployeeMultiPicker value={r.ids} onChange={(ids) => setR({ ...r, ids })} ariaLabel="Roster employees" />
          </Field>
          <Field label="From"><Input type="date" value={r.from} onChange={(e) => setR({ ...r, from: e.target.value })} slotProps={{ input: { "aria-label": "Roster from" } }} /></Field>
          <Field label="To"><Input type="date" value={r.to} onChange={(e) => setR({ ...r, to: e.target.value })} slotProps={{ input: { "aria-label": "Roster to" } }} /></Field>
          <Field label="Shift">
            <Select value={r.shift} onChange={(_, v) => setR({ ...r, shift: v ?? "" })} slotProps={{ button: { "aria-label": "Roster shift" } }}>
              {shifts.map((s) => <Option key={s.id} value={s.id}>{s.name}</Option>)}<Option value="off">Day off</Option><Option value="clear">Clear roster</Option>
            </Select>
          </Field>
          <Button onClick={() => void run(async () => { const x = await hr.setRoster(r.ids, r.from, r.to, r.shift); window.alert(`${x.days} roster day(s) updated`); })}>Apply roster</Button>
        </Row>
      </Card>
    </Box>
  );
}

function ImportPunches() {
  const { err, run } = useErr();
  const [res, setRes] = useState<ImportResult | null>(null);
  return (
    <Card title="Import biometric logs (CSV)">
      <ErrorAlert err={err} />
      <Typography level="body-xs" sx={{ mb: 1 }}>Columns: emp_code, timestamp (e.g. 2026-06-10 09:31:05 or 10/06/2026 09:31). Duplicate scans are ignored.</Typography>
      <FileButton label="Choose CSV" accept=".csv,text/csv" onFile={(f) => void run(async () => setRes(await hr.importPunches(f)))} />
      {res && <Typography level="body-sm" sx={{ mt: 1 }} data-testid="punch-import-result">{res.imported} imported, {res.duplicates} duplicate(s), {res.errors.length} problem row(s). {res.errors.slice(0, 5).map((e) => `row ${e.row}: ${e.error}`).join("; ")}</Typography>}
    </Card>
  );
}

function AttendanceSettings() {
  const { err, run } = useErr();
  const [s, setS] = useState<Settings | null>(null);
  const [msg, setMsg] = useState("");
  useEffect(() => { void run(async () => setS(await hr.settings())); }, [run]);
  if (!s) return <ErrorAlert err={err} />;
  const set = (p: Partial<Settings>) => setS({ ...s, ...p });
  return (
    <Card title="Attendance and payroll settings" action={<Button size="sm" onClick={() => void run(async () => { setS(await hr.saveSettings(s)); setMsg("Saved"); })} data-testid="settings-save">Save settings</Button>}>
      <ErrorAlert err={err} />{msg && <Typography level="body-xs" data-testid="settings-saved">{msg}</Typography>}
      <Row>
        <Field label="Company name (payslips)"><Input value={s.company_name} onChange={(e) => set({ company_name: e.target.value })} slotProps={{ input: { "aria-label": "Company name" } }} /></Field>
        <Field label="Company address"><Input value={s.company_address} onChange={(e) => set({ company_address: e.target.value })} /></Field>
        <Field label="Time zone"><Input value={s.timezone} onChange={(e) => set({ timezone: e.target.value })} /></Field>
      </Row>
      <Typography level="title-sm" sx={{ mt: 2 }}>Weekly off</Typography>
      <Row>{WEEKDAYS.map((d, i) => <Checkbox key={d} label={d} checked={s.weekly_off.includes(i)} onChange={(e) => set({ weekly_off: e.target.checked ? [...s.weekly_off, i].sort() : s.weekly_off.filter((x) => x !== i) })} />)}</Row>
      <Row sx={{ mt: 2 }}>
        <Field label="Leave year starts in month"><Input type="number" value={s.leave_year_start} onChange={(e) => set({ leave_year_start: Number(e.target.value) })} /></Field>
        <Field label="Loss of pay from">
          <Select value={s.lop_mode} onChange={(_, v) => set({ lop_mode: v ?? "leave_only" })} slotProps={{ button: { "aria-label": "LOP mode" } }}>
            <Option value="leave_only">Unpaid leave only</Option><Option value="attendance_and_leave">Unpaid leave and attendance</Option>
          </Select>
        </Field>
        <Checkbox label="Overtime tracking" checked={s.ot_enabled} onChange={(e) => set({ ot_enabled: e.target.checked })} />
      </Row>
      <Row sx={{ mt: 2 }}>
        <Field label="PF wage ceiling (₹)"><Input type="number" value={s.pf_wage_ceiling} onChange={(e) => set({ pf_wage_ceiling: Number(e.target.value) })} /></Field>
        <Checkbox label="Cap PF at the ceiling" checked={s.pf_capped} onChange={(e) => set({ pf_capped: e.target.checked })} />
        <Field label="ESI wage limit (₹)"><Input type="number" value={s.esi_wage_limit} onChange={(e) => set({ esi_wage_limit: Number(e.target.value) })} /></Field>
      </Row>
      <Typography level="title-sm" sx={{ mt: 2 }}>Geofence for web check-in</Typography>
      <Row>
        <Checkbox label="Require location near the office" checked={s.geofence_enabled} onChange={(e) => set({ geofence_enabled: e.target.checked })} data-testid="geofence-toggle" />
        <Field label="Latitude"><Input type="number" value={s.geofence_lat} onChange={(e) => set({ geofence_lat: Number(e.target.value) })} /></Field>
        <Field label="Longitude"><Input type="number" value={s.geofence_lng} onChange={(e) => set({ geofence_lng: Number(e.target.value) })} /></Field>
        <Field label="Radius (m)"><Input type="number" value={s.geofence_radius_m} onChange={(e) => set({ geofence_radius_m: Number(e.target.value) })} /></Field>
      </Row>
    </Card>
  );
}

export function Attendance({ isHR, isManager }: { isHR: boolean; isManager: boolean }) {
  return (
    <Tabs defaultValue="mine" sx={{ bgcolor: "transparent" }}>
      <TabList>
        <Tab value="mine">My attendance</Tab>{(isHR || isManager) && <Tab value="report">Team report</Tab>}{(isHR || isManager) && <Tab value="shifts">Shifts and roster</Tab>}
        {isHR && <Tab value="import">Import logs</Tab>}{isHR && <Tab value="settings">Settings</Tab>}
      </TabList>
      <TabPanel value="mine"><MyAttendance /></TabPanel>
      {(isHR || isManager) && <TabPanel value="report"><TeamReport /></TabPanel>}
      {(isHR || isManager) && <TabPanel value="shifts"><Shifts /></TabPanel>}
      {isHR && <TabPanel value="import"><ImportPunches /></TabPanel>}
      {isHR && <TabPanel value="settings"><AttendanceSettings /></TabPanel>}
    </Tabs>
  );
}
