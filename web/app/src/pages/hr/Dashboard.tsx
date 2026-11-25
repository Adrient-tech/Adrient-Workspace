import { PersonSelect } from "../../components/PeoplePicker";
import { useEffect, useState } from "react";
import { Box, Button, LinearProgress, Typography } from "@mui/joy";
import { hr } from "./api";
import type { Count, HRDashboard, ManagerDashboard } from "./api";
import { periodLabel } from "./logic";
import { Card, ErrorAlert, Row, Stat, StatusChip, useErr } from "./ui";

function Bars({ title, items }: { title: string; items: Count[] }) {
  const max = Math.max(1, ...items.map((i) => i.count));
  return (
    <Card title={title} sx={{ flex: 1, minWidth: 240 }}>
      {items.length === 0 && <Typography level="body-sm">No data.</Typography>}
      {items.map((i) => (
        <Box key={i.label} sx={{ mb: 0.75 }}>
          <Row sx={{ justifyContent: "space-between" }}><Typography level="body-sm">{i.label.replace("_", " ")}</Typography><Typography level="body-sm">{i.count}</Typography></Row>
          <LinearProgress determinate value={(i.count / max) * 100} />
        </Box>
      ))}
    </Card>
  );
}

function HrAdmins() {
  const { err, run } = useErr();
  const [admins, setAdmins] = useState<{ user_id: string; name: string; email: string }[]>([]);
  const [pick, setPick] = useState("");
  const load = () => void run(async () => { setAdmins(await hr.hrAdmins()); });
  useEffect(load, []); // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <Card title="HR admins" sx={{ flex: 1, minWidth: 300 }}>
      <ErrorAlert err={err} />
      <Typography level="body-xs" sx={{ mb: 1 }}>Organisation admins are always HR admins. Add colleagues who should manage HR without admin rights.</Typography>
      {admins.map((a) => <Row key={a.user_id} sx={{ alignItems: "center", justifyContent: "space-between" }}><Typography level="body-sm">{a.name} · {a.email}</Typography><Button size="sm" variant="plain" color="danger" onClick={() => void run(async () => { await hr.setHrAdmin(a.user_id, false); load(); })}>Remove</Button></Row>)}
      <Row sx={{ mt: 1 }}>
        <PersonSelect value={pick} onChange={(id) => setPick(id)} placeholder="Choose a colleague" ariaLabel="New HR admin" />
        <Button size="sm" disabled={!pick} onClick={() => void run(async () => { await hr.setHrAdmin(pick, true); setPick(""); load(); })}>Grant HR admin</Button>
      </Row>
    </Card>
  );
}

export function Dashboard({ isHR, onNavigate }: { isHR: boolean; onNavigate: (v: string) => void }) {
  const { err, run } = useErr();
  const [d, setD] = useState<HRDashboard | null>(null);
  const [m, setM] = useState<ManagerDashboard | null>(null);
  useEffect(() => { void run(async () => { if (isHR) setD(await hr.dashboard()); else setM(await hr.managerDashboard()); }); }, [isHR, run]);
  if (m) {
    return (
      <Box sx={{ display: "grid", gap: 2 }}>
        <ErrorAlert err={err} />
        <Row sx={{ alignItems: "stretch" }}>
          <Stat label="Team size" value={m.team_size} testId="mgr-team" /><Stat label="In today" value={m.present_today} /><Stat label="Not in yet" value={m.not_in_yet} />
          <Stat label="Leave to approve" value={m.pending_leave} /><Stat label="Regularisations" value={m.pending_regularisations} />
        </Row>
        {(m.pending_leave + m.pending_regularisations > 0) && <Button sx={{ alignSelf: "flex-start" }} onClick={() => onNavigate("approvals")}>Review approvals</Button>}
        <Card title="On leave today">{m.on_leave_today.length === 0 ? <Typography level="body-sm">Everyone is available.</Typography> : m.on_leave_today.map((l) => <Typography key={l.employee_id + l.from_date} level="body-sm">{l.name} · {l.type_name}</Typography>)}</Card>
        <Card title="My team">{m.team.map((e) => <Typography key={e.id} level="body-sm">{e.name} · {e.designation_name || "—"} · <StatusChip status={e.status} /></Typography>)}</Card>
      </Box>
    );
  }
  if (!d) return <ErrorAlert err={err} />;
  const now = new Date();
  return (
    <Box sx={{ display: "grid", gap: 2 }}>
      <ErrorAlert err={err} />
      <Row sx={{ alignItems: "stretch" }}>
        <Stat label="Headcount" value={d.headcount} testId="dash-headcount" hint={d.on_notice ? `${d.on_notice} on notice` : undefined} />
        <Stat label="Joined (30 days)" value={d.joiners.length} testId="dash-joiners" /><Stat label="Left (30 days)" value={d.leavers.length} />
        <Stat label="Attrition (12 months)" value={`${d.attrition_pct}%`} hint={`${d.leavers_12m} leaver(s)`} testId="dash-attrition" />
        <Stat label="On leave today" value={d.on_leave_today.length} testId="dash-leave-today" />
      </Row>
      <Row sx={{ alignItems: "stretch" }}>
        <Card title="Needs your attention" sx={{ flex: 1, minWidth: 280 }}>
          <Typography level="body-sm">{d.pending_leave} leave request(s) pending · {d.pending_regularisations} regularisation(s) pending</Typography>
          <Typography level="body-sm">Payroll {periodLabel(d.payroll_period)}: <b>{d.payroll_status}</b></Typography>
          {d.employees_without_salary > 0 && <Typography level="body-sm" color="warning">{d.employees_without_salary} employee(s) have no salary set</Typography>}
          <Row sx={{ mt: 1 }}><Button size="sm" onClick={() => onNavigate("approvals")}>Approvals</Button><Button size="sm" variant="outlined" onClick={() => onNavigate("payroll")}>Payroll</Button></Row>
        </Card>
        <Card title="On leave today" sx={{ flex: 1, minWidth: 280 }}>{d.on_leave_today.length === 0 ? <Typography level="body-sm">Nobody.</Typography> : d.on_leave_today.map((l) => <Typography key={l.employee_id} level="body-sm">{l.name} · {l.type_name}</Typography>)}</Card>
        <Card title={`Birthdays and anniversaries · ${now.toLocaleString("en-IN", { month: "long" })}`} sx={{ flex: 1, minWidth: 280 }}>
          {d.celebrations.length === 0 ? <Typography level="body-sm">None in the next 30 days.</Typography> : d.celebrations.slice(0, 10).map((c, i) => <Typography key={i} level="body-sm">{c.date.slice(5)} · {c.name} · {c.kind === "birthday" ? "Birthday" : `${c.years} yr anniversary`}</Typography>)}
        </Card>
      </Row>
      <Row sx={{ alignItems: "stretch" }}><Bars title="By department" items={d.by_department} /><Bars title="By employment type" items={d.by_type} /><Bars title="By location" items={d.by_location} /></Row>
      <Row sx={{ alignItems: "stretch" }}>
        <Card title="Recent joiners" sx={{ flex: 1, minWidth: 260 }}>{d.joiners.length === 0 ? <Typography level="body-sm">None.</Typography> : d.joiners.map((e) => <Typography key={e.id} level="body-sm">{e.name} · {e.join_date}</Typography>)}</Card>
        <Card title="Recent leavers" sx={{ flex: 1, minWidth: 260 }}>{d.leavers.length === 0 ? <Typography level="body-sm">None.</Typography> : d.leavers.map((e) => <Typography key={e.id} level="body-sm">{e.name} · {e.exit_date}</Typography>)}</Card>
        <HrAdmins />
      </Row>
    </Box>
  );
}
