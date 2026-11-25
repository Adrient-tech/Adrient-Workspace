import { useCallback, useEffect, useState } from "react";
import { Alert, Box, Button, Checkbox, Chip, FormControl, IconButton, Input, Option, Select, Table, Typography } from "@mui/joy";
import DeleteOutline from "@mui/icons-material/DeleteOutline";
import PlayArrow from "@mui/icons-material/PlayArrow";
import Stop from "@mui/icons-material/Stop";
import { Card, ErrorAlert, Field, Row, Stat, useErr } from "../hr/ui";
import { addDays, fmtDay, todayISO } from "../ops/dates";
import { tt } from "./api";
import type { Entry, Me, Project, Timer } from "./api";
import { fmtClock, fmtDuration, fmtTimer, parseDuration } from "./logic";

export function Today({ me, onChanged }: { me: Me; onChanged: () => void }) {
  const { err, run } = useErr();
  const [projects, setProjects] = useState<Project[]>([]);
  const [timer, setTimer] = useState<Timer | null>(null);
  const [tick, setTick] = useState(0);
  const [entries, setEntries] = useState<Entry[]>([]);
  const [warn, setWarn] = useState<string[]>([]);
  const [project, setProject] = useState("");
  const [task, setTask] = useState("");
  const [billable, setBillable] = useState(true);
  const [dur, setDur] = useState("");
  const [date, setDate] = useState(todayISO());
  const today = todayISO();
  const from = addDays(today, -13);
  const load = useCallback(() => void run(async () => {
    const [p, t, e] = await Promise.all([tt.projects(), tt.timer(), tt.entries({ from, to: today, user: "me" })]);
    setProjects(p);
    setTimer(t);
    setEntries(e);
    setProject((cur) => cur || t?.project_id || p[0]?.id || "");
  }), [run, from, today]);
  useEffect(load, [load]);
  useEffect(() => {
    if (!timer) return;
    const h = window.setInterval(() => setTick((x) => x + 1), 1000);
    return () => window.clearInterval(h);
  }, [timer]);
  const pname = (id: string) => projects.find((p) => p.id === id)?.name ?? "(archived project)";
  const elapsed = timer ? Math.max(0, Math.floor((Date.now() - new Date(timer.started_at).getTime()) / 1000)) + 0 * tick : 0;
  const days = [...new Set(entries.map((e) => e.date))].sort().reverse();
  const todaySec = entries.filter((e) => e.date === today).reduce((n, e) => n + e.seconds, 0);
  const weekSec = entries.filter((e) => e.date > addDays(today, -7)).reduce((n, e) => n + e.seconds, 0);
  const after = (w: string[]) => { setWarn(w); load(); onChanged(); };

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
      <ErrorAlert err={err} />
      {warn.map((w, i) => <Alert key={i} color="warning" variant="soft" endDecorator={<Button size="sm" variant="plain" onClick={() => setWarn([])}>Dismiss</Button>}>{w}</Alert>)}
      <Card title="Timer">
        <Row sx={{ alignItems: "flex-end" }}>
          <Field label="Project" sx={{ minWidth: 220 }}>
            <Select value={project} onChange={(_, v) => setProject(v ?? "")} disabled={!!timer} slotProps={{ button: { "aria-label": "Project" } }} placeholder="Choose a project">
              {projects.map((p) => <Option key={p.id} value={p.id}>{p.name}{p.billable ? "" : " (non-billable)"}</Option>)}
            </Select>
          </Field>
          <Field label="Task"><Input value={timer ? timer.task : task} disabled={!!timer} onChange={(e) => setTask(e.target.value)} slotProps={{ input: { "aria-label": "Task" } }} /></Field>
          {!timer && <Checkbox label="Billable" checked={billable} onChange={(e) => setBillable(e.target.checked)} sx={{ pb: 1 }} />}
          {timer ? (
            <>
              <Typography level="h2" data-testid="timer-clock" sx={{ fontVariantNumeric: "tabular-nums", px: 1 }}>{fmtTimer(elapsed)}</Typography>
              <Button color="danger" startDecorator={<Stop />} onClick={() => void run(async () => { const r = await tt.stop(); after(r.warnings); })}>Stop</Button>
              <Button variant="plain" color="neutral" onClick={() => { if (window.confirm("Discard the running timer without saving?")) void run(async () => { await tt.discard(); load(); }); }}>Discard</Button>
            </>
          ) : (
            <Button startDecorator={<PlayArrow />} disabled={!project} onClick={() => void run(async () => { await tt.start(project, task, billable); setTask(""); load(); })}>Start</Button>
          )}
        </Row>
        {!projects.length && <Typography level="body-sm" sx={{ mt: 1 }}>No projects yet. {me.admin ? "Add one under Billing and setup." : "Ask an admin to create a project."}</Typography>}
      </Card>
      <Card title="Add time by hand">
        <Row sx={{ alignItems: "flex-end" }}>
          <Field label="Date" sx={{ maxWidth: 170 }}><Input type="date" value={date} onChange={(e) => setDate(e.target.value)} slotProps={{ input: { "aria-label": "Entry date", max: today } }} /></Field>
          <Field label="Duration (1:30, 90m, 1.5h)" sx={{ maxWidth: 200 }}><Input value={dur} onChange={(e) => setDur(e.target.value)} slotProps={{ input: { "aria-label": "Duration" } }} /></Field>
          <Button disabled={!project || Number.isNaN(parseDuration(dur))} onClick={() => void run(async () => {
            const r = await tt.saveEntry("", { project_id: project, date, seconds: parseDuration(dur), billable, task });
            setDur("");
            after(r.warnings);
          })}>Add entry</Button>
        </Row>
        {dur && Number.isNaN(parseDuration(dur)) && <Typography level="body-xs" color="danger" sx={{ mt: 0.5 }}>That is not a duration. Try 1:30, 90m or 1.5h.</Typography>}
      </Card>
      <Row>
        <Stat label="Today" value={fmtDuration(todaySec)} testId="stat-today" />
        <Stat label="Last 7 days" value={fmtDuration(weekSec)} />
      </Row>
      {days.map((d) => (
        <Card key={d} title={`${d === today ? "Today" : fmtDay(d)}  ·  ${fmtClock(entries.filter((e) => e.date === d).reduce((n, e) => n + e.seconds, 0))}`}>
          <Box sx={{ overflowX: "auto" }}>
            <Table size="sm" aria-label={`Entries ${d}`}>
              <tbody>
                {entries.filter((e) => e.date === d).map((e) => (
                  <tr key={e.id}>
                    <td>{pname(e.project_id)}<Typography level="body-xs">{e.task}</Typography></td>
                    <td style={{ width: 90 }}>{fmtClock(e.seconds)}</td>
                    <td style={{ width: 120 }}>{e.invoice_id ? <Chip size="sm" color="success">invoiced</Chip> : !e.billable ? <Chip size="sm" variant="outlined">non-billable</Chip> : null}</td>
                    <td style={{ width: 48 }}>{!e.invoice_id && <IconButton size="sm" variant="plain" aria-label={`Delete entry ${e.id}`} onClick={() => void run(async () => { await tt.deleteEntry(e.id); load(); onChanged(); })}><DeleteOutline /></IconButton>}</td>
                  </tr>
                ))}
              </tbody>
            </Table>
          </Box>
        </Card>
      ))}
      {!days.length && <FormControl><Typography level="body-sm">No time logged in the last two weeks.</Typography></FormControl>}
    </Box>
  );
}
