import { useCallback, useEffect, useMemo, useState } from "react";
import { Alert, Box, Button, Checkbox, Chip, FormControl, FormLabel, IconButton, Input, LinearProgress, Option, Select, Table, Textarea, Typography } from "@mui/joy";
import ChevronLeft from "@mui/icons-material/ChevronLeft";
import ChevronRight from "@mui/icons-material/ChevronRight";
import { Card, Dialog, ErrorAlert, Field, Row, useErr } from "../hr/ui";
import { addDays, dayShort, fmtDay, fmtMinutes, fmtRange, money, netMinutes, startOfWeek, todayISO, weekDates } from "../ops/dates";
import { shiftsApi } from "./api";
import type { Attendance, Candidate, Cost, FillProposal, Gap, Me, Shift, Staff } from "./api";

const EMPTY: Shift = { id: "", date: "", start: "09:00", end: "17:00", break_minutes: 30, role: "", location: "", notes: "", employee_id: "", published: false };

export function Schedule({ me }: { me: Me }) {
  const { err, setErr, run } = useErr();
  const [week, setWeek] = useState(startOfWeek(todayISO()));
  const [staff, setStaff] = useState<Staff[]>([]);
  const [shifts, setShifts] = useState<Shift[]>([]);
  const [gaps, setGaps] = useState<Gap[]>([]);
  const [att, setAtt] = useState<Record<string, Attendance>>({});
  const [cost, setCost] = useState<{ cost: Cost; weekly_budget: number; currency: string } | null>(null);
  const [edit, setEdit] = useState<Shift | null>(null);
  const [fill, setFill] = useState<FillProposal[] | null>(null);
  const [note, setNote] = useState("");
  const days = useMemo(() => weekDates(week), [week]);
  const to = days[6];
  const load = useCallback(
    () =>
      void run(async () => {
        const [st, sh, gp, at] = await Promise.all([shiftsApi.staff(week, to), shiftsApi.shifts(addDays(week, -1), to), shiftsApi.gaps(week, to), shiftsApi.attendance(week, to)]);
        setStaff(st);
        setShifts(sh);
        setGaps(gp);
        setAtt(Object.fromEntries(at.map((a) => [a.shift_id, a])));
        if (me.scheduler) setCost(await shiftsApi.cost(week, to));
      }),
    [run, week, to, me.scheduler],
  );
  useEffect(load, [load]);

  const byId = useMemo(() => new Map(staff.map((s) => [s.employee_id, s])), [staff]);
  const inWeek = shifts.filter((s) => s.date >= week && s.date <= to);
  const unpublished = inWeek.filter((s) => !s.published).length;
  const rows = me.scheduler ? staff : staff.filter((s) => s.employee_id === me.employee_id);
  const open = inWeek.filter((s) => !s.employee_id);
  const weekMin = (id: string) => inWeek.filter((s) => s.employee_id === id).reduce((n, s) => n + netMinutes(s.start, s.end, s.break_minutes), 0);
  const today = todayISO();
  const mineToday = shifts.filter((s) => s.employee_id === me.employee_id && s.published && (s.date === today || (s.date === addDays(today, -1) && s.end < s.start)));

  const chip = (s: Shift) => {
    const emp = byId.get(s.employee_id);
    const a = att[s.id];
    return (
      <Box
        key={s.id}
        role="button"
        tabIndex={0}
        data-testid={`shift-${s.id}`}
        onClick={() => setEdit(s)}
        onKeyDown={(e) => (e.key === "Enter" || e.key === " ") && setEdit(s)}
        sx={{ borderLeft: `4px solid ${emp?.color ?? "#9aa5a1"}`, bgcolor: s.published ? "background.level1" : "background.surface", border: s.published ? undefined : "1px dashed", borderColor: "neutral.outlinedBorder", borderRadius: "sm", px: 0.75, py: 0.5, mb: 0.5, cursor: "pointer", fontSize: 12, lineHeight: 1.25 }}
      >
        <b>{fmtRange(s.start, s.end)}</b>
        <br />
        {s.role}
        {a && <Chip size="sm" color={a.status === "absent" ? "danger" : a.status === "late" ? "warning" : "success"} sx={{ ml: 0.5 }}>{a.status}</Chip>}
        {!s.published && <Chip size="sm" variant="outlined" sx={{ ml: 0.5 }}>draft</Chip>}
      </Box>
    );
  };
  const cell = (date: string, empId: string) => {
    const list = inWeek.filter((s) => s.date === date && s.employee_id === empId);
    const off = empId ? byId.get(empId)?.availability.exceptions.find((x) => x.from <= date && date <= x.to) : undefined;
    return (
      <td key={date} style={{ verticalAlign: "top", minWidth: 118, background: off ? "var(--joy-palette-neutral-softBg)" : undefined }}>
        {off && <Typography level="body-xs" sx={{ color: "text.tertiary" }}>{off.source === "leave" ? `Leave: ${off.note}` : `Off${off.note ? `: ${off.note}` : ""}`}</Typography>}
        {list.map(chip)}
        {me.scheduler && (
          <Button size="sm" variant="plain" sx={{ minHeight: 0, p: 0, fontSize: 12 }} aria-label={`Add shift ${date}`} onClick={() => setEdit({ ...EMPTY, date, employee_id: empId })}>+ add</Button>
        )}
      </td>
    );
  };

  const copyPrev = () => void run(async () => {
    const r = await shiftsApi.copyWeek(addDays(week, -7), week);
    setNote(r.copied ? `Copied ${r.copied} shift(s) from last week as drafts.` : "Nothing to copy: last week is empty or already copied.");
    load();
  });
  const publish = (p: boolean) => void run(async () => {
    const r = await shiftsApi.publish(week, to, p);
    setNote(`${p ? "Published" : "Unpublished"} ${r.changed} shift(s).`);
    load();
  });

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
      <ErrorAlert err={err} />
      {note && <Alert color="success" variant="soft" endDecorator={<Button size="sm" variant="plain" onClick={() => setNote("")}>Dismiss</Button>}>{note}</Alert>}
      {mineToday.length > 0 && <ClockCard shifts={mineToday} att={att} onChange={load} />}
      <Row sx={{ alignItems: "center" }}>
        <IconButton variant="outlined" size="sm" aria-label="Previous week" onClick={() => setWeek(addDays(week, -7))}><ChevronLeft /></IconButton>
        <Typography level="title-md" data-testid="week-label" sx={{ minWidth: 160, textAlign: "center" }}>{fmtDay(week)} – {fmtDay(to)}</Typography>
        <IconButton variant="outlined" size="sm" aria-label="Next week" onClick={() => setWeek(addDays(week, 7))}><ChevronRight /></IconButton>
        <Button size="sm" variant="outlined" onClick={() => setWeek(startOfWeek(todayISO()))}>This week</Button>
        {me.scheduler && (
          <>
            <Button size="sm" onClick={() => setEdit({ ...EMPTY, date: week })}>Add shift</Button>
            <Button size="sm" variant="outlined" onClick={copyPrev}>Copy last week</Button>
            <Button size="sm" variant="outlined" disabled={!open.length} onClick={() => void run(async () => setFill(await shiftsApi.fillPlan(week, to)))}>Fill open shifts</Button>
            <Button size="sm" variant="outlined" color="success" disabled={!unpublished} onClick={() => publish(true)}>Publish week{unpublished ? ` (${unpublished})` : ""}</Button>
            <Button size="sm" variant="plain" disabled={unpublished === inWeek.length} onClick={() => publish(false)}>Unpublish</Button>
          </>
        )}
      </Row>
      {gaps.length > 0 && (
        <Alert color="warning" variant="soft" data-testid="coverage-gaps">
          Understaffed: {gaps.slice(0, 6).map((g) => `${dayShort(g.date)} ${fmtDay(g.date)} ${g.role} (${g.scheduled}/${g.required})`).join(", ")}
          {gaps.length > 6 ? ` and ${gaps.length - 6} more` : ""}
        </Alert>
      )}
      {cost && me.scheduler && <CostBar cost={cost.cost} budget={cost.weekly_budget} currency={cost.currency} />}
      <Card>
        <Box sx={{ overflowX: "auto" }}>
          <Table size="sm" aria-label="Weekly schedule" sx={{ minWidth: 900, "& th": { whiteSpace: "nowrap" } }}>
            <thead>
              <tr>
                <th style={{ width: 150 }}>Team</th>
                {days.map((d) => (
                  <th key={d} style={d === today ? { color: "var(--joy-palette-primary-600)" } : undefined}>{dayShort(d)} {fmtDay(d)}</th>
                ))}
                <th style={{ width: 70 }}>Hours</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((s) => {
                const min = weekMin(s.employee_id);
                const over = s.max_hours != null && min > s.max_hours * 60;
                return (
                  <tr key={s.employee_id}>
                    <td>
                      <Box sx={{ display: "flex", alignItems: "center", gap: 0.75 }}>
                        <Box sx={{ width: 10, height: 10, borderRadius: "50%", bgcolor: s.color, flexShrink: 0 }} />
                        <Box><Typography level="body-sm" fontWeight="lg">{s.name}</Typography><Typography level="body-xs">{s.role}</Typography></Box>
                      </Box>
                    </td>
                    {days.map((d) => cell(d, s.employee_id))}
                    <td><Typography level="body-sm" color={over ? "danger" : "neutral"}>{fmtMinutes(min)}</Typography>{s.target_hours != null && <Typography level="body-xs">of {s.target_hours}h</Typography>}</td>
                  </tr>
                );
              })}
              <tr data-testid="open-row" style={{ background: "var(--joy-palette-warning-softBg)" }}>
                <td><Typography level="body-sm" fontWeight="lg">Open shifts</Typography><Typography level="body-xs">{open.length} unfilled</Typography></td>
                {days.map((d) => cell(d, ""))}
                <td />
              </tr>
            </tbody>
          </Table>
        </Box>
        {!me.scheduler && !rows.length && <Typography level="body-sm">You are not linked to an employee record, so you have no shifts of your own. Ask HR to link your account.</Typography>}
      </Card>
      {edit && <ShiftDialog me={me} shift={edit} staff={staff} onClose={() => setEdit(null)} onSaved={(w) => { setEdit(null); if (w.length) setNote("Saved with warnings: " + w.join(" ")); load(); }} setErr={setErr} />}
      {fill && <FillDialog plan={fill} byId={byId} onClose={() => setFill(null)} onDone={(msg) => { setFill(null); setNote(msg); load(); }} />}
    </Box>
  );
}

function CostBar({ cost, budget, currency }: { cost: Cost; budget: number; currency: string }) {
  const ratio = budget > 0 ? cost.cost / budget : 0;
  return (
    <Card sx={{ py: 1 }}>
      <Row sx={{ alignItems: "center" }}>
        <Typography level="body-sm" data-testid="week-cost">Projected labour cost <b>{money(cost.cost, currency)}</b> for {fmtMinutes(cost.minutes)}</Typography>
        {cost.overtime_minutes > 0 && <Chip size="sm" color="warning">{fmtMinutes(cost.overtime_minutes)} overtime ({money(cost.overtime_cost, currency)})</Chip>}
        {cost.employee_count > cost.rated_count && <Chip size="sm" variant="outlined">{cost.rated_count} of {cost.employee_count} people have rates</Chip>}
        {budget > 0 && (
          <Box sx={{ minWidth: 180, flex: 1 }}>
            <LinearProgress determinate value={Math.min(100, ratio * 100)} color={ratio > 1 ? "danger" : "success"} />
            <Typography level="body-xs">{ratio > 1 ? `Over budget by ${money(cost.cost - budget, currency)}` : `${Math.round(ratio * 100)}% of ${money(budget, currency)} budget`}</Typography>
          </Box>
        )}
      </Row>
    </Card>
  );
}

function ClockCard({ shifts, att, onChange }: { shifts: Shift[]; att: Record<string, Attendance>; onChange: () => void }) {
  const { err, run } = useErr();
  return (
    <Card title="Your shifts today">
      <ErrorAlert err={err} />
      {shifts.map((s) => {
        const a = att[s.id];
        return (
          <Row key={s.id} sx={{ alignItems: "center", mb: 0.5 }}>
            <Typography level="body-sm" sx={{ minWidth: 180 }}>{fmtRange(s.start, s.end)} · {s.role}</Typography>
            {a?.clock_in && <Chip size="sm" color={a.status === "late" ? "warning" : "success"}>in {a.clock_in}{a.clock_out ? ` · out ${a.clock_out}` : ""}</Chip>}
            {!a?.clock_in && <Button size="sm" onClick={() => void run(async () => { await shiftsApi.clock(s.id, "in"); onChange(); })}>Clock in</Button>}
            {a?.clock_in && !a.clock_out && <Button size="sm" variant="outlined" onClick={() => void run(async () => { await shiftsApi.clock(s.id, "out"); onChange(); })}>Clock out</Button>}
          </Row>
        );
      })}
    </Card>
  );
}

function ShiftDialog({ me, shift, staff, onClose, onSaved, setErr }: { me: Me; shift: Shift; staff: Staff[]; onClose: () => void; onSaved: (warnings: string[]) => void; setErr: (s: string) => void }) {
  const [s, setS] = useState<Shift>(shift);
  const [msg, setMsg] = useState("");
  const [cands, setCands] = useState<Candidate[] | null>(null);
  const [swapTo, setSwapTo] = useState("");
  const [swapReason, setSwapReason] = useState("");
  const isNew = !shift.id;
  const mine = !!me.employee_id && shift.employee_id === me.employee_id;
  const ro = !me.scheduler;
  const set = <K extends keyof Shift>(k: K, v: Shift[K]) => setS((x) => ({ ...x, [k]: v }));
  const go = async (fn: () => Promise<void>) => {
    try {
      setMsg("");
      await fn();
    } catch (e) {
      setMsg((e as Error).message);
    }
  };
  return (
    <Dialog open onClose={onClose} title={isNew ? "New shift" : ro ? "Shift" : "Edit shift"}>
      {msg && <Alert color="danger" variant="soft" role="alert">{msg}</Alert>}
      <Row>
        <Field label="Date"><Input type="date" disabled={ro} value={s.date} onChange={(e) => set("date", e.target.value)} slotProps={{ input: { "aria-label": "Shift date" } }} /></Field>
        <Field label="Role / label"><Input disabled={ro} value={s.role} onChange={(e) => set("role", e.target.value)} slotProps={{ input: { "aria-label": "Shift role" } }} /></Field>
      </Row>
      <Row sx={{ mt: 1 }}>
        <Field label="Start"><Input type="time" disabled={ro} value={s.start} onChange={(e) => set("start", e.target.value)} slotProps={{ input: { "aria-label": "Start time" } }} /></Field>
        <Field label="End"><Input type="time" disabled={ro} value={s.end} onChange={(e) => set("end", e.target.value)} slotProps={{ input: { "aria-label": "End time" } }} /></Field>
        <Field label="Unpaid break (min)"><Input type="number" disabled={ro} value={s.break_minutes} onChange={(e) => set("break_minutes", Number(e.target.value))} slotProps={{ input: { "aria-label": "Break minutes", min: 0 } }} /></Field>
      </Row>
      <Row sx={{ mt: 1 }}>
        <Field label="Assigned to">
          <Select disabled={ro} value={s.employee_id} onChange={(_, v) => set("employee_id", v ?? "")} slotProps={{ button: { "aria-label": "Assignee" } }}>
            <Option value="">Open shift (nobody)</Option>
            {staff.map((p) => <Option key={p.employee_id} value={p.employee_id}>{p.name}</Option>)}
          </Select>
        </Field>
        <Field label="Location"><Input disabled={ro} value={s.location} onChange={(e) => set("location", e.target.value)} /></Field>
      </Row>
      <FormControl sx={{ mt: 1 }}><FormLabel>Notes</FormLabel><Textarea minRows={2} disabled={ro} value={s.notes} onChange={(e) => set("notes", e.target.value)} /></FormControl>
      {!ro && <Checkbox sx={{ mt: 1 }} label="Published (visible to the team)" checked={s.published} onChange={(e) => set("published", e.target.checked)} />}
      {!ro && !isNew && !s.employee_id && (
        <Box sx={{ mt: 1 }}>
          <Button size="sm" variant="outlined" onClick={() => void go(async () => setCands(await shiftsApi.suggest(shift.id)))}>Suggest who can take it</Button>
          {cands && !cands.length && <Typography level="body-sm" sx={{ mt: 0.5 }}>Nobody is free: everyone is on time off, unavailable, overlapping or at their weekly maximum.</Typography>}
          {cands?.map((c) => (
            <Row key={c.employee_id} sx={{ alignItems: "center", mt: 0.5 }}>
              <Button size="sm" variant="soft" onClick={() => set("employee_id", c.employee_id)}>{c.name}</Button>
              <Typography level="body-xs">{c.reasons.join(" · ")}</Typography>
            </Row>
          ))}
        </Box>
      )}
      {!isNew && mine && s.employee_id && (
        <Card title="Ask a colleague to take this shift" sx={{ mt: 1 }}>
          <Row>
            <Field label="Colleague">
              <Select value={swapTo} onChange={(_, v) => setSwapTo(v ?? "")} slotProps={{ button: { "aria-label": "Swap with" } }}>
                {staff.filter((p) => p.employee_id !== me.employee_id).map((p) => <Option key={p.employee_id} value={p.employee_id}>{p.name}</Option>)}
              </Select>
            </Field>
            <Field label="Reason"><Input value={swapReason} onChange={(e) => setSwapReason(e.target.value)} /></Field>
            <Button disabled={!swapTo} onClick={() => void go(async () => { await shiftsApi.requestSwap(shift.id, swapTo, swapReason); onSaved([]); })}>Request swap</Button>
          </Row>
        </Card>
      )}
      <Row sx={{ mt: 2, justifyContent: "space-between" }}>
        {!ro && !isNew ? <Button color="danger" variant="outlined" onClick={() => { if (window.confirm("Delete this shift?")) void go(async () => { await shiftsApi.deleteShift(shift.id); onSaved([]); }); }}>Delete</Button> : <span />}
        <Row>
          <Button variant="plain" onClick={onClose}>{ro ? "Close" : "Cancel"}</Button>
          {!ro && <Button onClick={() => void go(async () => { try { const r = await shiftsApi.saveShift(shift.id, s); onSaved(r.warnings); } catch (e) { setErr(""); throw e; } })}>Save shift</Button>}
        </Row>
      </Row>
    </Dialog>
  );
}

function FillDialog({ plan, byId, onClose, onDone }: { plan: FillProposal[]; byId: Map<string, Staff>; onClose: () => void; onDone: (m: string) => void }) {
  const { err, run } = useErr();
  const [chosen, setChosen] = useState<Record<string, string>>(() => Object.fromEntries(plan.filter((p) => p.proposal).map((p) => [p.shift.id, p.proposal!.employee_id])));
  const n = Object.values(chosen).filter(Boolean).length;
  return (
    <Dialog open onClose={onClose} title="Fill open shifts" wide>
      <ErrorAlert err={err} />
      <Typography level="body-sm" sx={{ mb: 1 }}>Each proposal already accounts for the ones above it. Nothing changes until you apply.</Typography>
      <Table size="sm" aria-label="Fill proposals">
        <thead><tr><th>Shift</th><th>Assign to</th><th>Why</th></tr></thead>
        <tbody>
          {plan.map((p) => (
            <tr key={p.shift.id}>
              <td>{dayShort(p.shift.date)} {fmtDay(p.shift.date)}<br /><Typography level="body-xs">{fmtRange(p.shift.start, p.shift.end)} · {p.shift.role}</Typography></td>
              <td>
                <Select size="sm" value={chosen[p.shift.id] ?? ""} onChange={(_, v) => setChosen((c) => ({ ...c, [p.shift.id]: v ?? "" }))} slotProps={{ button: { "aria-label": `Assignee for ${p.shift.date}` } }}>
                  <Option value="">Leave open</Option>
                  {p.candidates.map((c) => <Option key={c.employee_id} value={c.employee_id}>{byId.get(c.employee_id)?.name ?? c.name}</Option>)}
                </Select>
              </td>
              <td><Typography level="body-xs">{p.candidates.length ? (p.candidates.find((c) => c.employee_id === chosen[p.shift.id])?.reasons ?? p.candidates[0].reasons).join(" · ") : "Nobody is available"}</Typography></td>
            </tr>
          ))}
        </tbody>
      </Table>
      <Row sx={{ mt: 2, justifyContent: "flex-end" }}>
        <Button variant="plain" onClick={onClose}>Cancel</Button>
        <Button disabled={!n} onClick={() => void run(async () => {
          const r = await shiftsApi.fillApply(Object.entries(chosen).filter(([, v]) => v).map(([shift_id, employee_id]) => ({ shift_id, employee_id })));
          onDone(`Assigned ${r.assigned} shift(s).${r.problems.length ? " Not assigned: " + r.problems.join("; ") : ""}`);
        })}>Assign {n} shift(s)</Button>
      </Row>
    </Dialog>
  );
}

