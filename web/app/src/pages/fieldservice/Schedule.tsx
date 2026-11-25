import { useCallback, useEffect, useState } from "react";
import { Box, Button, Chip, IconButton, Option, Select, Typography } from "@mui/joy";
import ChevronLeft from "@mui/icons-material/ChevronLeft";
import ChevronRight from "@mui/icons-material/ChevronRight";
import { Card, ErrorAlert, Row, Stat, useErr } from "../hr/ui";
import { addDays, dayShort, fmtDay, fmtRange, money, startOfWeek, todayISO, weekDates } from "../ops/dates";
import { fs } from "./api";
import type { Dashboard, Settings, Team, Visit } from "./api";
import { STATUS_COLOR, STATUS_LABEL } from "./logic";

export function Home({ settings, onNavigate }: { settings: Settings; onNavigate: (v: string) => void }) {
  const { err, run } = useErr();
  const [d, setD] = useState<Dashboard | null>(null);
  const load = useCallback(() => void run(async () => setD(await fs.dashboard())), [run]);
  useEffect(load, [load]);
  if (!d) return <ErrorAlert err={err} />;
  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
      <ErrorAlert err={err} />
      <Row>
        <Box sx={{ flex: 1, minWidth: 130, cursor: "pointer" }} onClick={() => onNavigate("requests")}><Stat label="New requests" value={d.new_requests} testId="stat-new-requests" /></Box>
        <Box sx={{ flex: 1, minWidth: 130, cursor: "pointer" }} onClick={() => onNavigate("requests")}><Stat label="Quotes awaiting" value={d.quotes_awaiting} /></Box>
        <Box sx={{ flex: 1, minWidth: 130, cursor: "pointer" }} onClick={() => onNavigate("jobs")}><Stat label="Active jobs" value={d.active_jobs} hint={d.late_jobs ? `${d.late_jobs} late` : undefined} /></Box>
        <Box sx={{ flex: 1, minWidth: 130, cursor: "pointer" }} onClick={() => onNavigate("jobs")}><Stat label="Needs invoicing" value={d.requires_invoicing} /></Box>
        <Stat label="Outstanding" value={money(d.outstanding_amount, settings.currency)} hint="open invoices" />
      </Row>
      <Card title={`Today (${d.visits_today.length}) · ${d.visits_this_week} visits in the next 7 days`}>
        {!d.visits_today.length && <Typography level="body-sm">No visits today.</Typography>}
        {d.visits_today.map((v) => <VisitLine key={v.id} v={v} />)}
      </Card>
    </Box>
  );
}

function VisitLine({ v, team }: { v: Visit; team?: Team[] }) {
  const names = (v.assignees ?? []).map((a) => team?.find((t) => t.id === a)?.name).filter(Boolean).join(", ");
  return (
    <Box sx={{ borderLeft: "4px solid", borderColor: v.status === "completed" ? "success.400" : "warning.400", pl: 1, py: 0.5, mb: 0.75 }} data-testid={`visit-${v.id}`}>
      <Typography level="body-sm"><b>{v.start ? fmtRange(v.start, v.end || v.start) : "Any time"}</b> · {v.job_title} · {v.client_name}</Typography>
      <Typography level="body-xs">{[v.address, names].filter(Boolean).join(" · ")} <Chip size="sm" variant="soft" color={STATUS_COLOR[v.status]}>{STATUS_LABEL[v.status] ?? v.status}</Chip></Typography>
    </Box>
  );
}

export function WeekSchedule({ team, me }: { team: Team[]; me: string }) {
  const { err, run } = useErr();
  const [week, setWeek] = useState(startOfWeek(todayISO()));
  const [who, setWho] = useState("");
  const [visits, setVisits] = useState<Visit[]>([]);
  const days = weekDates(week);
  const load = useCallback(() => void run(async () => setVisits(await fs.visits(week, addDays(week, 6), who))), [run, week, who]);
  useEffect(load, [load]);
  return (
    <Card title="Schedule">
      <ErrorAlert err={err} />
      <Row sx={{ alignItems: "center", mb: 1 }}>
        <IconButton variant="outlined" size="sm" aria-label="Previous week" onClick={() => setWeek(addDays(week, -7))}><ChevronLeft /></IconButton>
        <Typography level="title-md" sx={{ minWidth: 150, textAlign: "center" }}>{fmtDay(week)} – {fmtDay(addDays(week, 6))}</Typography>
        <IconButton variant="outlined" size="sm" aria-label="Next week" onClick={() => setWeek(addDays(week, 7))}><ChevronRight /></IconButton>
        <Button size="sm" variant="outlined" onClick={() => setWeek(startOfWeek(todayISO()))}>This week</Button>
        <Select size="sm" value={who} onChange={(_, v) => setWho(v ?? "")} slotProps={{ button: { "aria-label": "Filter by person" } }} sx={{ minWidth: 170 }}>
          <Option value="">Everyone</Option>{me && <Option value={me}>Only me</Option>}{team.filter((t) => t.id !== me).map((t) => <Option key={t.id} value={t.id}>{t.name || t.email}</Option>)}
        </Select>
      </Row>
      <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "repeat(7, minmax(0, 1fr))" }, gap: 1 }}>
        {days.map((d) => (
          <Box key={d} sx={{ border: "1px solid", borderColor: d === todayISO() ? "primary.400" : "divider", borderRadius: "sm", p: 0.75, minHeight: 90 }}>
            <Typography level="body-xs" fontWeight="lg">{dayShort(d)} {fmtDay(d)}</Typography>
            {visits.filter((v) => v.date === d).map((v) => <VisitLine key={v.id} v={v} team={team} />)}
          </Box>
        ))}
      </Box>
    </Card>
  );
}
