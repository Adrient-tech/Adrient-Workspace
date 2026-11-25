import { useCallback, useEffect, useState } from "react";
import { Box, Button, IconButton, Input, Table, Typography } from "@mui/joy";
import ChevronLeft from "@mui/icons-material/ChevronLeft";
import ChevronRight from "@mui/icons-material/ChevronRight";
import { Card, ErrorAlert, Row, useErr } from "../hr/ui";
import { addDays, dayShort, fmtDay, startOfWeek, todayISO, weekDates } from "../ops/dates";
import { tt } from "./api";
import type { Entry, Project } from "./api";
import { fmtClock, parseDuration, secondsByDay } from "./logic";

/** Weekly grid: projects down, days across. Type a duration and press Enter or leave the cell. */
export function Week() {
  const { err, run } = useErr();
  const [week, setWeek] = useState(startOfWeek(todayISO()));
  const [projects, setProjects] = useState<Project[]>([]);
  const [entries, setEntries] = useState<Entry[]>([]);
  const [draft, setDraft] = useState<Record<string, string>>({});
  const days = weekDates(week);
  const today = todayISO();
  const load = useCallback(() => void run(async () => {
    const [p, e] = await Promise.all([tt.projects(), tt.entries({ from: week, to: addDays(week, 6), user: "me" })]);
    setProjects(p);
    setEntries(e);
    setDraft({});
  }), [run, week]);
  useEffect(load, [load]);
  const used = new Set(entries.map((e) => e.project_id));
  const [extra, setExtra] = useState<string[]>([]);
  const rows = projects.filter((p) => used.has(p.id) || extra.includes(p.id));
  const sec = (pid: string, d: string) => entries.filter((e) => e.project_id === pid && e.date === d).reduce((n, e) => n + e.seconds, 0);
  const locked = (pid: string, d: string) => entries.some((e) => e.project_id === pid && e.date === d && e.invoice_id);
  const commit = (pid: string, d: string) => {
    const k = `${pid}|${d}`;
    const v = draft[k];
    if (v === undefined) return;
    const s = v.trim() === "" ? 0 : parseDuration(v);
    if (Number.isNaN(s)) { void run(async () => { throw new Error(`"${v}" is not a duration. Try 1:30, 90m or 1.5h.`); }); return; }
    if (s === sec(pid, d)) { setDraft(({ [k]: _x, ...rest }) => rest); return; }
    void run(async () => { await tt.setCell(pid, d, s); load(); });
  };
  const dayTot = secondsByDay(entries);
  return (
    <Card title="Weekly timesheet">
      <ErrorAlert err={err} />
      <Row sx={{ alignItems: "center", mb: 1 }}>
        <IconButton variant="outlined" size="sm" aria-label="Previous week" onClick={() => setWeek(addDays(week, -7))}><ChevronLeft /></IconButton>
        <Typography level="title-md" sx={{ minWidth: 150, textAlign: "center" }}>{fmtDay(week)} – {fmtDay(addDays(week, 6))}</Typography>
        <IconButton variant="outlined" size="sm" aria-label="Next week" onClick={() => setWeek(addDays(week, 7))}><ChevronRight /></IconButton>
        <Button size="sm" variant="outlined" onClick={() => setWeek(startOfWeek(todayISO()))}>This week</Button>
      </Row>
      <Box sx={{ overflowX: "auto" }}>
        <Table size="sm" aria-label="Timesheet grid" sx={{ minWidth: 760 }}>
          <thead><tr><th style={{ width: 180 }}>Project</th>{days.map((d) => <th key={d} style={d === today ? { color: "var(--joy-palette-primary-600)" } : undefined}>{dayShort(d)} {fmtDay(d)}</th>)}<th>Total</th></tr></thead>
          <tbody>
            {rows.map((p) => (
              <tr key={p.id}>
                <td>{p.name}</td>
                {days.map((d) => {
                  const k = `${p.id}|${d}`;
                  const s = sec(p.id, d);
                  return (
                    <td key={d}>
                      <Input size="sm" disabled={locked(p.id, d) || d > today} value={draft[k] ?? (s ? fmtClock(s) : "")} placeholder="–"
                        onChange={(e) => setDraft({ ...draft, [k]: e.target.value })} onBlur={() => commit(p.id, d)} onKeyDown={(e) => e.key === "Enter" && commit(p.id, d)}
                        slotProps={{ input: { "aria-label": `${p.name} ${d}`, style: { textAlign: "right", width: 64 } } }} />
                    </td>
                  );
                })}
                <td>{fmtClock(days.reduce((n, d) => n + sec(p.id, d), 0))}</td>
              </tr>
            ))}
            <tr><td><b>Total</b></td>{days.map((d) => <td key={d}><b>{fmtClock(dayTot.get(d) ?? 0)}</b></td>)}<td><b>{fmtClock(entries.reduce((n, e) => n + e.seconds, 0))}</b></td></tr>
          </tbody>
        </Table>
      </Box>
      <Row sx={{ mt: 1.5, alignItems: "center" }}>
        <Typography level="body-sm">Add a row:</Typography>
        {projects.filter((p) => !rows.some((r) => r.id === p.id)).map((p) => <Button key={p.id} size="sm" variant="soft" onClick={() => setExtra([...extra, p.id])}>{p.name}</Button>)}
      </Row>
    </Card>
  );
}
