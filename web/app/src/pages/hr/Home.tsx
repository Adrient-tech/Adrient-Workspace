import { useCallback, useEffect, useMemo, useState } from "react";
import { Avatar, Box, Button, Chip, Input, Option, Select, Table, Textarea, Typography } from "@mui/joy";
import { hr } from "./api";
import type { Balance, Employee, LeaveType, MyHR, Settings } from "./api";
import { formatINR, initials, periodLabel, previewLeaveDays } from "./logic";
import { Card, DateInput, Dialog, DownloadLink, ErrorAlert, Field, FileButton, Row, StatusChip, useErr } from "./ui";

export function CheckInCard({ today, onChange }: { today: MyHR["today"]; onChange: () => void }) {
  const { err, run } = useErr();
  const [busy, setBusy] = useState(false);
  const punch = async () => {
    setBusy(true);
    await run(async () => {
      let pos: { lat: number; lng: number } | undefined;
      if (today.geofence) {
        pos = await new Promise((res, rej) =>
          navigator.geolocation.getCurrentPosition(
            (p) => res({ lat: p.coords.latitude, lng: p.coords.longitude }),
            () => rej(new Error("Allow location access to check in")),
            { enableHighAccuracy: true, timeout: 10000 },
          ),
        );
      }
      await hr.punch(pos);
      onChange();
    });
    setBusy(false);
  };
  const d = today.day;
  return (
    <Card title="Today" sx={{ minWidth: 280, flex: 1 }}>
      <ErrorAlert err={err} />
      <Typography level="body-sm" sx={{ mb: 1 }}>
        {today.date} · {today.punches.length === 0 ? "Not checked in yet" : today.checked_in ? `Checked in at ${today.punches[0].at.slice(0, 5)}` : `Last out ${today.punches[today.punches.length - 1].at.slice(0, 5)}`}
      </Typography>
      <Button onClick={() => void punch()} loading={busy} color={today.checked_in ? "warning" : "success"} data-testid="hr-punch">
        {today.checked_in ? "Check out" : "Check in"}
      </Button>
      <Box sx={{ mt: 1, display: "flex", gap: 0.5, flexWrap: "wrap" }}>
        {today.punches.map((p, i) => (
          <Chip key={i} size="sm" variant="outlined">
            {p.at.slice(0, 5)} {p.source}
          </Chip>
        ))}
        {d?.status && <StatusChip status={d.status} />}
        {d?.late && <Chip size="sm" color="warning">Late {d.late_min}m</Chip>}
      </Box>
      {today.geofence && <Typography level="body-xs" sx={{ mt: 1 }}>Location is checked against the office geofence.</Typography>}
    </Card>
  );
}

export function ApplyLeaveDialog({ open, onClose, onDone, balances, employeeId }: { open: boolean; onClose: () => void; onDone: () => void; balances: Balance[]; employeeId?: string }) {
  const { err, setErr, run } = useErr();
  const [types, setTypes] = useState<LeaveType[]>([]);
  const [settings, setSettings] = useState<Settings | null>(null);
  const [holidays, setHolidays] = useState<string[]>([]);
  const [typeId, setTypeId] = useState("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [half, setHalf] = useState("");
  const [reason, setReason] = useState("");
  useEffect(() => {
    if (!open) return;
    void Promise.all([hr.leaveTypes(), hr.settings(), hr.holidayCalendars()]).then(([t, s, c]) => {
      setTypes(t);
      setSettings(s);
      setHolidays(c.flatMap((x) => x.holidays.filter((h) => !h.optional).map((h) => h.day)));
      setTypeId((cur) => cur || t[0]?.id || "");
    }).catch((e: Error) => setErr(e.message));
  }, [open, setErr]);
  const type = types.find((t) => t.id === typeId);
  const bal = balances.find((b) => b.type_id === typeId);
  const days = useMemo(() => previewLeaveDays(from, to || from, half, settings?.weekly_off ?? [0], holidays), [from, to, half, settings, holidays]);
  const submit = async () => {
    const ok = await run(() => hr.applyLeave({ employee_id: employeeId, type_id: typeId, from_date: from, to_date: to || from, half_day: half, reason }));
    if (ok) {
      setFrom(""); setTo(""); setHalf(""); setReason("");
      onDone();
      onClose();
    }
  };
  return (
    <Dialog open={open} onClose={onClose} title="Apply for leave">
      <ErrorAlert err={err} />
      <Row sx={{ mt: 1 }}>
        <Field label="Leave type">
          <Select value={typeId} onChange={(_, v) => setTypeId(v ?? "")} slotProps={{ button: { "aria-label": "Leave type" } }}>
            {types.map((t) => <Option key={t.id} value={t.id}>{t.name}</Option>)}
          </Select>
        </Field>
        {bal?.tracked && <Typography level="body-sm" sx={{ alignSelf: "center" }}>Balance: <b>{bal.balance}</b> day(s)</Typography>}
      </Row>
      <Row sx={{ mt: 1 }}>
        <Field label="From"><DateInput value={from} onChange={(v) => { setFrom(v); if (!to || to < v) setTo(v); }} label="From date" /></Field>
        <Field label="To"><DateInput value={to} onChange={setTo} label="To date" min={from} /></Field>
        {type?.allow_half_day && from && from === (to || from) && (
          <Field label="Duration">
            <Select value={half} onChange={(_, v) => setHalf(v ?? "")} slotProps={{ button: { "aria-label": "Duration" } }}>
              <Option value="">Full day</Option>
              <Option value="first">First half</Option>
              <Option value="second">Second half</Option>
            </Select>
          </Field>
        )}
      </Row>
      <Field label="Reason" sx={{ mt: 1 }}><Textarea minRows={2} value={reason} onChange={(e) => setReason(e.target.value)} /></Field>
      <Typography level="body-sm" sx={{ mt: 1 }} data-testid="leave-days-preview">{from ? `${days} working day(s) will be deducted` : "Choose dates"}</Typography>
      <Row sx={{ mt: 2, justifyContent: "flex-end" }}>
        <Button variant="plain" color="neutral" onClick={onClose}>Cancel</Button>
        <Button onClick={() => void submit()} disabled={!typeId || !from || days === 0} data-testid="leave-submit">Submit request</Button>
      </Row>
    </Dialog>
  );
}

function ProfileCard({ e, onSaved }: { e: Employee; onSaved: () => void }) {
  const { err, run } = useErr();
  const [open, setOpen] = useState(false);
  const [f, setF] = useState({ phone: e.phone, personal_email: e.personal_email, address: e.address, marital_status: e.marital_status, blood_group: e.blood_group });
  const [ec, setEc] = useState<{ id: string; name: string; relationship: string; phone: string }[]>([]);
  const [n, setN] = useState({ name: "", relationship: "", phone: "" });
  const load = useCallback(() => void hr.emergency(e.id).then(setEc).catch(() => undefined), [e.id]);
  useEffect(() => { if (open) load(); }, [open, load]);
  return (
    <Card title="My details" action={<Button size="sm" variant="outlined" onClick={() => setOpen(true)} data-testid="edit-my-details">Edit</Button>}>
      <Typography level="body-sm">{e.emp_code} · {e.designation_name || "No designation"} · {e.department_name || "No department"}</Typography>
      <Typography level="body-sm">Joined {e.join_date} · {e.location_name || "No location"} · Manager: {e.manager_name || "none"}</Typography>
      <Dialog open={open} onClose={() => setOpen(false)} title="My details">
        <ErrorAlert err={err} />
        <Row>
          <Field label="Phone"><Input value={f.phone} onChange={(x) => setF({ ...f, phone: x.target.value })} /></Field>
          <Field label="Personal email"><Input value={f.personal_email} onChange={(x) => setF({ ...f, personal_email: x.target.value })} /></Field>
        </Row>
        <Row sx={{ mt: 1 }}>
          <Field label="Marital status"><Input value={f.marital_status} onChange={(x) => setF({ ...f, marital_status: x.target.value })} /></Field>
          <Field label="Blood group"><Input value={f.blood_group} onChange={(x) => setF({ ...f, blood_group: x.target.value })} /></Field>
        </Row>
        <Field label="Address" sx={{ mt: 1 }}><Textarea minRows={2} value={f.address} onChange={(x) => setF({ ...f, address: x.target.value })} /></Field>
        <Button sx={{ mt: 1 }} onClick={() => void run(async () => { await hr.saveOwnProfile(f); onSaved(); })}>Save details</Button>
        <Typography level="title-sm" sx={{ mt: 2 }}>Emergency contacts</Typography>
        {ec.map((c) => (
          <Row key={c.id} sx={{ alignItems: "center", mt: 0.5 }}>
            <Typography level="body-sm">{c.name} ({c.relationship || "contact"}) · {c.phone}</Typography>
            <Button size="sm" variant="plain" color="danger" onClick={() => void run(async () => { await hr.deleteEmergency(e.id, c.id); load(); })}>Remove</Button>
          </Row>
        ))}
        <Row sx={{ mt: 1 }}>
          <Field label="Name"><Input value={n.name} onChange={(x) => setN({ ...n, name: x.target.value })} /></Field>
          <Field label="Relationship"><Input value={n.relationship} onChange={(x) => setN({ ...n, relationship: x.target.value })} /></Field>
          <Field label="Phone"><Input value={n.phone} onChange={(x) => setN({ ...n, phone: x.target.value })} /></Field>
          <Button onClick={() => void run(async () => { await hr.addEmergency(e.id, n); setN({ name: "", relationship: "", phone: "" }); load(); })}>Add</Button>
        </Row>
      </Dialog>
    </Card>
  );
}

export function Home({ onNavigate }: { onNavigate: (v: string) => void }) {
  const { err, run } = useErr();
  const [data, setData] = useState<MyHR | null>(null);
  const [apply, setApply] = useState(false);
  const load = useCallback(() => void run(async () => setData(await hr.me())), [run]);
  useEffect(load, [load]);
  if (!data) return <ErrorAlert err={err} />;
  if (!data.employee) {
    return (
      <Card title="My HR">
        <ErrorAlert err={err} />
        <Typography level="body-md" data-testid="hr-no-employee">
          Your account is not linked to an employee record yet.{data.roles.hr ? " Add yourself under People and link your account." : " Ask HR to link your account."}
        </Typography>
        {data.roles.hr && <Button sx={{ mt: 1 }} onClick={() => onNavigate("people")}>Open People</Button>}
      </Card>
    );
  }
  const e = data.employee;
  const cancel = (id: string) => void run(async () => { await hr.decideLeave(id, "cancel"); load(); });
  return (
    <Box sx={{ display: "grid", gap: 2 }}>
      <ErrorAlert err={err} />
      <Row sx={{ alignItems: "stretch" }}>
        <Card sx={{ flex: 2, minWidth: 280 }}>
          <Box sx={{ display: "flex", gap: 2, alignItems: "center" }}>
            <Avatar size="lg">{initials(e.name)}</Avatar>
            <Box>
              <Typography level="h3" data-testid="hr-my-name">{e.name}</Typography>
              <Typography level="body-sm">{e.designation_name || "Employee"} · {e.department_name || "No department"}</Typography>
            </Box>
          </Box>
          {data.pending_approvals > 0 && (
            <Button size="sm" color="warning" variant="soft" sx={{ mt: 1.5 }} onClick={() => onNavigate("approvals")} data-testid="hr-pending-banner">
              {data.pending_approvals} request(s) waiting for your approval
            </Button>
          )}
        </Card>
        <CheckInCard today={data.today} onChange={load} />
      </Row>
      <Card title="Leave balance" action={<Button size="sm" onClick={() => setApply(true)} data-testid="hr-apply-leave">Apply for leave</Button>}>
        <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap" }}>
          {data.balances.filter((b) => b.tracked).map((b) => (
            <Box key={b.type_id} sx={{ borderLeft: `4px solid ${b.color}`, pl: 1.5, minWidth: 120 }} data-testid={`balance-${b.code}`}>
              <Typography level="body-xs">{b.name}</Typography>
              <Typography level="h3">{b.balance}</Typography>
              <Typography level="body-xs">used {b.used}{b.pending ? ` · ${b.pending} pending` : ""}</Typography>
            </Box>
          ))}
        </Box>
      </Card>
      <Row sx={{ alignItems: "stretch" }}>
        <Card title="My recent leave" sx={{ flex: 2, minWidth: 300 }}>
          {data.my_leave.length === 0 ? <Typography level="body-sm">No leave requests yet.</Typography> : (
            <Table size="sm">
              <thead><tr><th>Type</th><th>Dates</th><th>Days</th><th>Status</th><th /></tr></thead>
              <tbody>
                {data.my_leave.map((r) => (
                  <tr key={r.id}>
                    <td>{r.type_name}</td>
                    <td>{r.from_date}{r.to_date !== r.from_date ? ` → ${r.to_date}` : ""}{r.half_day ? ` (${r.half_day} half)` : ""}</td>
                    <td>{r.days}</td>
                    <td><StatusChip status={r.status} /></td>
                    <td>{r.can_cancel && <Button size="sm" variant="plain" color="danger" onClick={() => cancel(r.id)}>Cancel</Button>}</td>
                  </tr>
                ))}
              </tbody>
            </Table>
          )}
        </Card>
        <Card title="Upcoming holidays" sx={{ flex: 1, minWidth: 220 }}>
          {data.holidays.length === 0 ? <Typography level="body-sm">None scheduled.</Typography> : data.holidays.map((h) => <Typography key={h.day} level="body-sm">{h.day} · {h.name}</Typography>)}
        </Card>
      </Row>
      <Row sx={{ alignItems: "stretch" }}>
        <Card title="My payslips" sx={{ flex: 1, minWidth: 280 }}>
          {data.payslips.length === 0 ? <Typography level="body-sm">No payslips published yet.</Typography> : data.payslips.slice(0, 6).map((p) => (
            <Row key={p.id} sx={{ alignItems: "center", justifyContent: "space-between", mb: 0.5 }}>
              <Typography level="body-sm">{periodLabel(p.period)} · net {formatINR(p.net_pay)}</Typography>
              <DownloadLink href={hr.payslipPdfUrl(p.id)} testId={`payslip-pdf-${p.period}`}>PDF</DownloadLink>
            </Row>
          ))}
        </Card>
        <Card title="My documents" sx={{ flex: 1, minWidth: 280 }} action={
          <FileButton label="Upload ID proof" onFile={(f) => void run(async () => { await hr.uploadDocument(e.id, "id_proof", f); load(); })} />
        }>
          {data.documents.length === 0 ? <Typography level="body-sm">No documents.</Typography> : data.documents.map((d) => (
            <Row key={d.id} sx={{ alignItems: "center", justifyContent: "space-between", mb: 0.5 }}>
              <Typography level="body-sm">{d.name} <Chip size="sm" variant="outlined">{d.category.replace("_", " ")}</Chip></Typography>
              <DownloadLink href={hr.documentUrl(d.id)}>Download</DownloadLink>
            </Row>
          ))}
        </Card>
      </Row>
      <Row sx={{ alignItems: "stretch" }}>
        <Card title="My team" sx={{ flex: 1, minWidth: 280 }}>
          {data.manager && <Typography level="body-sm">Manager: <b>{data.manager.name}</b></Typography>}
          {[...data.team, ...data.reports].map((p) => <Typography key={p.id} level="body-sm">{p.name} · {p.designation_name || "—"}</Typography>)}
          {!data.manager && data.team.length === 0 && data.reports.length === 0 && <Typography level="body-sm">No team yet.</Typography>}
          {data.on_leave_today.length > 0 && <Typography level="body-xs" sx={{ mt: 1 }}>On leave today: {data.on_leave_today.map((l) => l.name).join(", ")}</Typography>}
        </Card>
        <Card title="Birthdays and anniversaries" sx={{ flex: 1, minWidth: 280 }}>
          {data.celebrations.length === 0 ? <Typography level="body-sm">Nothing in the next 30 days.</Typography> : data.celebrations.slice(0, 8).map((c, i) => (
            <Typography key={i} level="body-sm">{c.date.slice(5)} · {c.name} · {c.kind === "birthday" ? "Birthday" : `${c.years} year${c.years === 1 ? "" : "s"} at work`}</Typography>
          ))}
        </Card>
      </Row>
      <ProfileCard e={e} onSaved={load} />
      <ApplyLeaveDialog open={apply} onClose={() => setApply(false)} onDone={load} balances={data.balances} />
    </Box>
  );
}
