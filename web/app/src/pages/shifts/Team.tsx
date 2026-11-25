import { useCallback, useEffect, useState } from "react";
import { Alert, Box, Button, Checkbox, Input, Table, Typography } from "@mui/joy";
import { Card, Dialog, ErrorAlert, Field, Row, useErr } from "../hr/ui";
import { DAY_SHORT, addDays, fmtDay, money, todayISO } from "../ops/dates";
import { shiftsApi } from "./api";
import type { Me, Staff, TimeOff } from "./api";

export function Team({ me }: { me: Me }) {
  const { err, run } = useErr();
  const [staff, setStaff] = useState<Staff[]>([]);
  const [cur, setCur] = useState("INR");
  const [edit, setEdit] = useState<Staff | null>(null);
  const load = useCallback(() => void run(async () => {
    const t = todayISO();
    setStaff(await shiftsApi.staff(t, addDays(t, 120)));
    if (me.scheduler) setCur((await shiftsApi.settings()).currency || "INR");
  }), [run, me.scheduler]);
  useEffect(load, [load]);
  const summary = (s: Staff) => {
    const on = s.availability.days.map((d, i) => (d.off ? "" : DAY_SHORT[i])).filter(Boolean);
    return on.length === 7 ? "Every day" : on.join(", ") || "Never";
  };
  return (
    <Card title="Team and availability">
      <ErrorAlert err={err} />
      <Typography level="body-sm" sx={{ mb: 1 }}>People come from HR. Approved leave in HR shows up here as time off automatically, so nobody is scheduled on leave without a warning.</Typography>
      <Box sx={{ overflowX: "auto" }}>
        <Table size="sm" aria-label="Team">
          <thead><tr><th>Name</th><th>Role</th><th>Available</th><th>Weekly target / max</th>{me.scheduler && <th>Rate / hour</th>}<th>Time off (next 120 days)</th><th /></tr></thead>
          <tbody>
            {staff.map((s) => (
              <tr key={s.employee_id}>
                <td><Box component="span" sx={{ display: "inline-block", width: 10, height: 10, borderRadius: "50%", bgcolor: s.color, mr: 0.75 }} />{s.name}</td>
                <td>{s.role || "—"}</td>
                <td>{summary(s)}</td>
                <td>{s.target_hours ?? "—"} / {s.max_hours ?? "—"} h</td>
                {me.scheduler && <td>{s.hourly_rate != null ? money(s.hourly_rate, cur) : "—"}</td>}
                <td>{s.availability.exceptions.length ? s.availability.exceptions.map((x) => `${fmtDay(x.from)}${x.to !== x.from ? "–" + fmtDay(x.to) : ""}${x.source === "leave" ? " (leave)" : ""}`).join(", ") : "—"}</td>
                <td>{me.scheduler && <Button size="sm" variant="outlined" onClick={() => setEdit(s)}>Edit</Button>}</td>
              </tr>
            ))}
          </tbody>
        </Table>
      </Box>
      {edit && <StaffDialog staff={edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); load(); }} />}
    </Card>
  );
}

function StaffDialog({ staff, onClose, onSaved }: { staff: Staff; onClose: () => void; onSaved: () => void }) {
  const { err, run } = useErr();
  const [s, setS] = useState<Staff>(structuredClone(staff));
  const [off, setOff] = useState<TimeOff>({ from: "", to: "", note: "" });
  const manual = s.availability.exceptions.filter((x) => x.source !== "leave");
  const leave = s.availability.exceptions.filter((x) => x.source === "leave");
  const num = (v: string) => (v === "" ? null : Number(v));
  return (
    <Dialog open onClose={onClose} title={`Scheduling profile: ${s.name}`} wide>
      <ErrorAlert err={err} />
      <Row>
        <Field label="Role"><Input value={s.role} onChange={(e) => setS({ ...s, role: e.target.value })} slotProps={{ input: { "aria-label": "Role" } }} /></Field>
        <Field label="Colour"><Input type="color" value={s.color} onChange={(e) => setS({ ...s, color: e.target.value })} /></Field>
        <Field label="Hourly rate"><Input type="number" value={s.hourly_rate ?? ""} onChange={(e) => setS({ ...s, hourly_rate: num(e.target.value) })} slotProps={{ input: { "aria-label": "Hourly rate", min: 0 } }} /></Field>
      </Row>
      <Row sx={{ mt: 1 }}>
        <Field label="Target hours / week"><Input type="number" value={s.target_hours ?? ""} onChange={(e) => setS({ ...s, target_hours: num(e.target.value) })} slotProps={{ input: { "aria-label": "Target hours", min: 0 } }} /></Field>
        <Field label="Maximum hours / week"><Input type="number" value={s.max_hours ?? ""} onChange={(e) => setS({ ...s, max_hours: num(e.target.value) })} slotProps={{ input: { "aria-label": "Maximum hours", min: 0 } }} /></Field>
      </Row>
      <Typography level="title-sm" sx={{ mt: 2 }}>Weekly availability</Typography>
      {s.availability.days.map((d, i) => (
        <Row key={i} sx={{ alignItems: "center", mt: 0.5 }}>
          <Typography level="body-sm" sx={{ width: 40 }}>{DAY_SHORT[i]}</Typography>
          <Checkbox label="Off" checked={d.off} onChange={(e) => setS({ ...s, availability: { ...s.availability, days: s.availability.days.map((x, j) => (j === i ? { ...x, off: e.target.checked } : x)) } })} />
          <Input type="time" size="sm" disabled={d.off} value={d.start} onChange={(e) => setS({ ...s, availability: { ...s.availability, days: s.availability.days.map((x, j) => (j === i ? { ...x, start: e.target.value } : x)) } })} slotProps={{ input: { "aria-label": `${DAY_SHORT[i]} from` } }} />
          <Input type="time" size="sm" disabled={d.off} value={d.end} onChange={(e) => setS({ ...s, availability: { ...s.availability, days: s.availability.days.map((x, j) => (j === i ? { ...x, end: e.target.value } : x)) } })} slotProps={{ input: { "aria-label": `${DAY_SHORT[i]} to` } }} />
        </Row>
      ))}
      <Typography level="title-sm" sx={{ mt: 2 }}>Time off</Typography>
      {leave.map((x, i) => <Alert key={i} size="sm" variant="soft" sx={{ mt: 0.5 }}>{fmtDay(x.from)} – {fmtDay(x.to)}: {x.note} (approved in HR)</Alert>)}
      {manual.map((x, i) => (
        <Row key={i} sx={{ alignItems: "center", mt: 0.5 }}>
          <Typography level="body-sm">{fmtDay(x.from)} – {fmtDay(x.to)} {x.note}</Typography>
          <Button size="sm" variant="plain" color="danger" onClick={() => setS({ ...s, availability: { ...s.availability, exceptions: s.availability.exceptions.filter((y) => y !== x) } })}>Remove</Button>
        </Row>
      ))}
      <Row sx={{ mt: 0.5 }}>
        <Input type="date" size="sm" value={off.from} onChange={(e) => setOff({ ...off, from: e.target.value, to: off.to || e.target.value })} slotProps={{ input: { "aria-label": "Time off from" } }} />
        <Input type="date" size="sm" value={off.to} onChange={(e) => setOff({ ...off, to: e.target.value })} slotProps={{ input: { "aria-label": "Time off to" } }} />
        <Input size="sm" placeholder="Note" value={off.note} onChange={(e) => setOff({ ...off, note: e.target.value })} />
        <Button size="sm" variant="outlined" disabled={!off.from || !off.to} onClick={() => { setS({ ...s, availability: { ...s.availability, exceptions: [...s.availability.exceptions, off] } }); setOff({ from: "", to: "", note: "" }); }}>Add</Button>
      </Row>
      <Row sx={{ mt: 2, justifyContent: "flex-end" }}>
        <Button variant="plain" onClick={onClose}>Cancel</Button>
        <Button onClick={() => void run(async () => { await shiftsApi.saveStaff(s.employee_id, { role: s.role, color: s.color, hourly_rate: s.hourly_rate, target_hours: s.target_hours, max_hours: s.max_hours, availability: { days: s.availability.days, exceptions: manual.length === s.availability.exceptions.length - leave.length ? s.availability.exceptions.filter((x) => x.source !== "leave") : [] } }); onSaved(); })}>Save profile</Button>
      </Row>
    </Dialog>
  );
}

