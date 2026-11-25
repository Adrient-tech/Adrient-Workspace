import { useCallback, useEffect, useMemo, useState } from "react";
import { Box, Button, Checkbox, Input, Option, Select, Tab, TabList, TabPanel, Tabs, Table, Typography } from "@mui/joy";
import { hr } from "./api";
import type { Balance, HolidayCalendar, LeaveRequest, LeaveType, Lookup, MyHR, TeamLeave, Holiday } from "./api";
import { currentPeriod, monthDays, periodLabel, shiftPeriod } from "./logic";
import { Card, Dialog, ErrorAlert, Field, Row, StatusChip, useErr } from "./ui";
import { ApplyLeaveDialog } from "./Home";

function MyLeave() {
  const { err, run } = useErr();
  const [list, setList] = useState<LeaveRequest[]>([]);
  const [bal, setBal] = useState<Balance[]>([]);
  const [apply, setApply] = useState(false);
  const load = useCallback(() => void run(async () => {
    setList(await hr.leaveRequests("mine"));
    const me: MyHR = await hr.me();
    setBal(me.balances);
  }), [run]);
  useEffect(load, [load]);
  return (
    <Card title="My leave" action={<Button size="sm" onClick={() => setApply(true)}>Apply for leave</Button>}>
      <ErrorAlert err={err} />
      <Row sx={{ mb: 1 }}>{bal.filter((b) => b.tracked).map((b) => <Typography key={b.type_id} level="body-sm"><b>{b.name}</b> {b.balance}</Typography>)}</Row>
      <Table size="sm" aria-label="My leave requests">
        <thead><tr><th>Type</th><th>From</th><th>To</th><th>Days</th><th>Reason</th><th>Status</th><th /></tr></thead>
        <tbody>
          {list.map((r) => (
            <tr key={r.id}>
              <td>{r.type_name}</td><td>{r.from_date}</td><td>{r.to_date}</td><td>{r.days}</td><td>{r.reason || "—"}{r.decision_note ? ` (${r.decision_note})` : ""}</td><td><StatusChip status={r.status} /></td>
              <td>{r.can_cancel && <Button size="sm" variant="plain" color="danger" onClick={() => void run(async () => { await hr.decideLeave(r.id, "cancel"); load(); })}>Cancel</Button>}</td>
            </tr>
          ))}
          {list.length === 0 && <tr><td colSpan={7}>No leave requests.</td></tr>}
        </tbody>
      </Table>
      <ApplyLeaveDialog open={apply} onClose={() => setApply(false)} onDone={load} balances={bal} />
    </Card>
  );
}

function TeamCalendar() {
  const { err, run } = useErr();
  const [period, setPeriod] = useState(currentPeriod());
  const [data, setData] = useState<{ leaves: TeamLeave[]; holidays: Holiday[] }>({ leaves: [], holidays: [] });
  useEffect(() => {
    const days = monthDays(period);
    void run(async () => setData(await hr.teamCalendar(days[0], days[days.length - 1])));
  }, [period, run]);
  const days = monthDays(period);
  const people = useMemo(() => [...new Map(data.leaves.map((l) => [l.employee_id, l.name])).entries()], [data]);
  const hol = new Map(data.holidays.map((h) => [h.day, h.name]));
  return (
    <Card title="Team leave calendar" action={<Row><Button size="sm" variant="outlined" onClick={() => setPeriod(shiftPeriod(period, -1))}>‹</Button><Typography level="title-sm" sx={{ alignSelf: "center" }}>{periodLabel(period)}</Typography><Button size="sm" variant="outlined" onClick={() => setPeriod(shiftPeriod(period, 1))}>›</Button></Row>}>
      <ErrorAlert err={err} />
      {people.length === 0 ? <Typography level="body-sm" data-testid="team-calendar-empty">Nobody in your team is on leave this month.</Typography> : (
        <Box sx={{ overflowX: "auto" }}>
          <Table size="sm" aria-label="Team leave grid" data-testid="team-calendar">
            <thead><tr><th>Who</th>{days.map((d) => <th key={d} style={{ padding: 2, textAlign: "center", background: hol.has(d) ? "#fde8c8" : undefined }} title={hol.get(d)}>{d.slice(8)}</th>)}</tr></thead>
            <tbody>
              {people.map(([id, name]) => (
                <tr key={id}>
                  <td>{name}</td>
                  {days.map((d) => {
                    const l = data.leaves.find((x) => x.employee_id === id && x.from_date <= d && x.to_date >= d);
                    return <td key={d} title={l ? `${l.type_name} (${l.status})` : hol.get(d)} style={{ padding: 0, background: l ? l.color + (l.status === "pending" ? "66" : "") : hol.has(d) ? "#fde8c8" : undefined, minWidth: 18 }} />;
                  })}
                </tr>
              ))}
            </tbody>
          </Table>
        </Box>
      )}
      {data.holidays.length > 0 && <Typography level="body-xs" sx={{ mt: 1 }}>Holidays: {data.holidays.map((h) => `${h.day.slice(8)} ${h.name}`).join(", ")}</Typography>}
    </Card>
  );
}

function TypesEditor() {
  const { err, run } = useErr();
  const [types, setTypes] = useState<LeaveType[]>([]);
  const [edit, setEdit] = useState<Partial<LeaveType> | null>(null);
  const load = useCallback(() => void run(async () => setTypes(await hr.leaveTypes(true))), [run]);
  useEffect(load, [load]);
  const e = edit;
  return (
    <Card title="Leave types and accrual policy" action={<Button size="sm" onClick={() => setEdit({ code: "", name: "", paid: true, tracked: true, annual_quota: 12, accrual: "monthly", carry_forward: false, carry_cap: 0, max_balance: null, allow_half_day: true, employment_types: [], active: true, color: "#126C5A" })}>New leave type</Button>}>
      <ErrorAlert err={err} />
      <Table size="sm" aria-label="Leave types">
        <thead><tr><th>Code</th><th>Name</th><th>Paid</th><th>Quota</th><th>Accrual</th><th>Carry forward</th><th>Cap</th><th /></tr></thead>
        <tbody>
          {types.map((t) => (
            <tr key={t.id} style={{ opacity: t.active ? 1 : 0.5 }}>
              <td>{t.code}</td><td>{t.name}</td><td>{t.paid ? "Paid" : "Unpaid"}</td><td>{t.tracked ? t.annual_quota : "unlimited"}</td><td>{t.accrual}</td>
              <td>{t.carry_forward ? `up to ${t.carry_cap}` : "no"}</td><td>{t.max_balance ?? "—"}</td><td><Button size="sm" variant="outlined" onClick={() => setEdit(t)}>Edit</Button></td>
            </tr>
          ))}
        </tbody>
      </Table>
      <Row sx={{ mt: 1 }}>
        <Button size="sm" variant="outlined" onClick={() => void run(async () => { const r = await hr.runAccrual(); window.alert(`Accrual brought up to date for ${r.employees} employees.`); })}>Run accrual now</Button>
        <Typography level="body-xs" sx={{ alignSelf: "center" }}>Accrual also runs automatically when balances are read. It is safe to run repeatedly.</Typography>
      </Row>
      {e && (
        <Dialog open onClose={() => setEdit(null)} title={e.id ? `Edit ${e.name}` : "New leave type"}>
          <Row>
            <Field label="Code"><Input value={e.code ?? ""} onChange={(x) => setEdit({ ...e, code: x.target.value })} slotProps={{ input: { "aria-label": "Leave code" } }} /></Field>
            <Field label="Name"><Input value={e.name ?? ""} onChange={(x) => setEdit({ ...e, name: x.target.value })} slotProps={{ input: { "aria-label": "Leave name" } }} /></Field>
          </Row>
          <Row sx={{ mt: 1 }}>
            <Field label="Annual quota (days)"><Input type="number" value={e.annual_quota ?? 0} onChange={(x) => setEdit({ ...e, annual_quota: Number(x.target.value) })} slotProps={{ input: { "aria-label": "Annual quota" } }} /></Field>
            <Field label="Accrual">
              <Select value={e.accrual ?? "monthly"} onChange={(_, v) => setEdit({ ...e, accrual: v ?? "monthly" })} slotProps={{ button: { "aria-label": "Accrual" } }}>
                <Option value="monthly">Monthly</Option><Option value="annual">Annual (upfront)</Option><Option value="none">None (manual)</Option>
              </Select>
            </Field>
          </Row>
          <Row sx={{ mt: 1 }}>
            <Field label="Carry forward cap (days)"><Input type="number" disabled={!e.carry_forward} value={e.carry_cap ?? 0} onChange={(x) => setEdit({ ...e, carry_cap: Number(x.target.value) })} /></Field>
            <Field label="Max balance (blank = none)"><Input type="number" value={e.max_balance ?? ""} onChange={(x) => setEdit({ ...e, max_balance: x.target.value === "" ? null : Number(x.target.value) })} /></Field>
          </Row>
          <Row sx={{ mt: 1 }}>
            <Checkbox label="Paid" checked={!!e.paid} onChange={(x) => setEdit({ ...e, paid: x.target.checked })} />
            <Checkbox label="Track balance" checked={!!e.tracked} onChange={(x) => setEdit({ ...e, tracked: x.target.checked })} />
            <Checkbox label="Carry forward" checked={!!e.carry_forward} onChange={(x) => setEdit({ ...e, carry_forward: x.target.checked })} />
            <Checkbox label="Half day allowed" checked={!!e.allow_half_day} onChange={(x) => setEdit({ ...e, allow_half_day: x.target.checked })} />
            <Checkbox label="Active" checked={!!e.active} onChange={(x) => setEdit({ ...e, active: x.target.checked })} />
          </Row>
          <Button sx={{ mt: 2 }} onClick={() => void run(async () => { await hr.saveLeaveType(e.id ?? "", e); setEdit(null); load(); })}>Save leave type</Button>
        </Dialog>
      )}
    </Card>
  );
}

function Holidays() {
  const { err, run } = useErr();
  const [cals, setCals] = useState<HolidayCalendar[]>([]);
  const [locs, setLocs] = useState<Lookup[]>([]);
  const [name, setName] = useState("");
  const [loc, setLoc] = useState("");
  const [h, setH] = useState<Record<string, { day: string; name: string }>>({});
  const load = useCallback(() => void run(async () => { setCals(await hr.holidayCalendars()); setLocs(await hr.lookups("locations")); }), [run]);
  useEffect(load, [load]);
  return (
    <Card title="Holiday calendars">
      <ErrorAlert err={err} />
      <Typography level="body-xs" sx={{ mb: 1 }}>A calendar tied to a location overrides the default calendar for people working there. Holidays are not charged to leave.</Typography>
      {cals.map((c) => (
        <Box key={c.id} sx={{ mb: 2 }} data-testid={`calendar-${c.name}`}>
          <Row sx={{ alignItems: "center", justifyContent: "space-between" }}>
            <Typography level="title-sm">{c.name} {c.location_id ? `· ${locs.find((l) => l.id === c.location_id)?.name ?? ""}` : "· default"}</Typography>
            <Button size="sm" variant="plain" color="danger" onClick={() => void run(async () => { await hr.deleteCalendar(c.id); load(); })}>Delete calendar</Button>
          </Row>
          {c.holidays.map((x) => (
            <Row key={x.id} sx={{ alignItems: "center", justifyContent: "space-between" }}>
              <Typography level="body-sm">{x.day} · {x.name}</Typography>
              <Button size="sm" variant="plain" color="danger" onClick={() => void run(async () => { await hr.deleteHoliday(x.id); load(); })}>×</Button>
            </Row>
          ))}
          <Row sx={{ mt: 0.5 }}>
            <Input size="sm" type="date" value={h[c.id]?.day ?? ""} onChange={(e) => setH({ ...h, [c.id]: { day: e.target.value, name: h[c.id]?.name ?? "" } })} slotProps={{ input: { "aria-label": `Holiday date ${c.name}` } }} />
            <Input size="sm" placeholder="Holiday name" value={h[c.id]?.name ?? ""} onChange={(e) => setH({ ...h, [c.id]: { day: h[c.id]?.day ?? "", name: e.target.value } })} slotProps={{ input: { "aria-label": `Holiday name ${c.name}` } }} />
            <Button size="sm" onClick={() => void run(async () => { await hr.addHoliday(c.id, h[c.id]); setH({ ...h, [c.id]: { day: "", name: "" } }); load(); })}>Add holiday</Button>
          </Row>
        </Box>
      ))}
      <Row>
        <Input size="sm" placeholder="New calendar name" value={name} onChange={(e) => setName(e.target.value)} slotProps={{ input: { "aria-label": "New calendar name" } }} />
        <Select size="sm" value={loc} onChange={(_, v) => setLoc(v ?? "")} placeholder="All locations" slotProps={{ button: { "aria-label": "Calendar location" } }}>
          <Option value="">All locations (default)</Option>
          {locs.map((l) => <Option key={l.id} value={l.id}>{l.name}</Option>)}
        </Select>
        <Button size="sm" onClick={() => void run(async () => { await hr.saveCalendar("", { name, location_id: loc }); setName(""); load(); })}>Add calendar</Button>
      </Row>
    </Card>
  );
}

export function Leave({ isHR }: { isHR: boolean }) {
  return (
    <Tabs defaultValue="mine" sx={{ bgcolor: "transparent" }}>
      <TabList><Tab value="mine">My leave</Tab><Tab value="team">Team calendar</Tab>{isHR && <Tab value="setup">Leave types</Tab>}{isHR && <Tab value="hol">Holidays</Tab>}</TabList>
      <TabPanel value="mine"><MyLeave /></TabPanel>
      <TabPanel value="team"><TeamCalendar /></TabPanel>
      {isHR && <TabPanel value="setup"><TypesEditor /></TabPanel>}
      {isHR && <TabPanel value="hol"><Holidays /></TabPanel>}
    </Tabs>
  );
}
