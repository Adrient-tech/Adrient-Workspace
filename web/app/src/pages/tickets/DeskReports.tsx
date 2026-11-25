import { useEffect, useMemo, useState } from "react";
import { Alert, Box, Button, Chip, CircularProgress, Input, Option, Select, Sheet, Typography } from "@mui/joy";
import { svc, fmtMin, PRIORITIES, TICKET_TYPES, type Bucket, type DayCount, type Group, type ReportOpts, type ReportV2 } from "./svcApi";
import { EmptyState } from "../../components/ui/EmptyState";

const RANGES: [string, number][] = [["Last 7 days", 7], ["Last 30 days", 30], ["Last 90 days", 90], ["Last year", 365]];

/** Reports and dashboards: volume, backlog, response and resolution times, SLA, CSAT, workload. */
export function DeskReports({ pid, onClose }: { pid: string; onClose: () => void }) {
  const [days, setDays] = useState(30);
  const [group, setGroup] = useState("");
  const [type, setType] = useState("");
  const [priority, setPriority] = useState("");
  const [groups, setGroups] = useState<Group[]>([]);
  const [rep, setRep] = useState<ReportV2 | null>(null);
  const [saved, setSaved] = useState<{ id: string; name: string; params: ReportOpts }[]>([]);
  const [err, setErr] = useState("");
  const [name, setName] = useState("");
  const opts: ReportOpts = useMemo(() => ({ days, group, type, priority }), [days, group, type, priority]);
  useEffect(() => { svc.groups(pid).then(setGroups).catch(() => undefined); svc.savedReports(pid).then(setSaved).catch(() => undefined); }, [pid]);
  useEffect(() => { setRep(null); setErr(""); svc.report(pid, opts).then(setRep).catch((e) => setErr((e as Error).message)); }, [pid, opts]);
  return (
    <Box sx={{ p: { xs: 2, md: 3 }, height: "calc(100vh - 56px)", overflow: "auto" }} data-testid="desk-reports">
      <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, flexWrap: "wrap", mb: 2 }}>
        <Button size="sm" variant="plain" color="neutral" onClick={onClose}>← Back to tickets</Button>
        <Typography level="h3" sx={{ flex: 1 }}>Reports</Typography>
        <Select size="sm" value={days} onChange={(_, v) => v && setDays(v)} sx={{ minWidth: 140 }} slotProps={{ button: { "data-testid": "rep-range" } as never }}>{RANGES.map(([l, d]) => <Option key={d} value={d}>{l}</Option>)}</Select>
        <Select size="sm" placeholder="All teams" value={group || null} onChange={(_, v) => setGroup(v ?? "")} sx={{ minWidth: 130 }}><Option value="">All teams</Option>{groups.map((g) => <Option key={g.id} value={g.id}>{g.name}</Option>)}</Select>
        <Select size="sm" placeholder="All types" value={type || null} onChange={(_, v) => setType(v ?? "")} sx={{ minWidth: 110 }}><Option value="">All types</Option>{TICKET_TYPES.map((t) => <Option key={t} value={t}>{t}</Option>)}</Select>
        <Select size="sm" placeholder="All priorities" value={priority || null} onChange={(_, v) => setPriority(v ?? "")} sx={{ minWidth: 130 }}><Option value="">All priorities</Option>{PRIORITIES.map((t) => <Option key={t} value={t}>{t}</Option>)}</Select>
        <Button size="sm" variant="outlined" component="a" href={svc.csvUrl(pid, opts)} data-testid="rep-csv">Export CSV</Button>
      </Box>
      <Box sx={{ display: "flex", gap: 1, alignItems: "center", flexWrap: "wrap", mb: 2 }}>
        {saved.map((s) => (
          <Chip key={s.id} size="sm" variant="outlined" onClick={() => { setDays(s.params.days ?? 30); setGroup(s.params.group ?? ""); setType(s.params.type ?? ""); setPriority(s.params.priority ?? ""); }}
            endDecorator={<span onClick={async (e) => { e.stopPropagation(); await svc.deleteReport(s.id).catch(() => undefined); setSaved((x) => x.filter((y) => y.id !== s.id)); }} aria-label={`Delete ${s.name}`}>×</span>}>{s.name}</Chip>
        ))}
        <Input size="sm" placeholder="Name this view to save it" value={name} onChange={(e) => setName(e.target.value)} sx={{ width: 220 }} />
        <Button size="sm" variant="soft" disabled={!name.trim()} onClick={async () => { await svc.saveReport(pid, name.trim(), opts).catch(() => undefined); setName(""); svc.savedReports(pid).then(setSaved).catch(() => undefined); }}>Save view</Button>
      </Box>
      {err && <Alert color="danger">{err}</Alert>}
      {!rep && !err && <Box sx={{ display: "grid", placeItems: "center", py: 8 }}><CircularProgress /></Box>}
      {rep && (rep.created === 0 && rep.open_now === 0 ? <EmptyState illustration="search" title="No tickets in this period" hint="Widen the date range or clear a filter." /> : (
        <>
          <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr 1fr", md: "repeat(4, 1fr)", xl: "repeat(7, 1fr)" }, gap: 1.5, mb: 2 }}>
            <Kpi label="Created" value={rep.created} />
            <Kpi label="Resolved" value={rep.resolved} />
            <Kpi label="Open now" value={rep.open_now} />
            <Kpi label="First response" value={rep.avg_first_response_min ? fmtMin(Math.round(rep.avg_first_response_min)) : "–"} hint="average" />
            <Kpi label="Resolution" value={rep.avg_resolution_min ? fmtMin(Math.round(rep.avg_resolution_min)) : "–"} hint="average" />
            <Kpi label="SLA met" value={rep.sla_checked ? `${Math.round(rep.sla_met_pct)}%` : "–"} hint={`${rep.sla_checked} checked`} tone={rep.sla_checked && rep.sla_met_pct < 80 ? "danger" : "success"} />
            <Kpi label="Satisfaction" value={rep.csat_count ? `${rep.csat_avg.toFixed(1)} / 5` : "–"} hint={`${rep.csat_count} answers`} />
          </Box>
          <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", lg: "2fr 1fr" }, gap: 2 }}>
            <Card title="Created and resolved per day"><Trend a={rep.created_per_day} b={rep.resolved_per_day} /></Card>
            <Card title="Backlog by age"><BarList data={rep.backlog} color="var(--joy-palette-warning-500)" /></Card>
            <Card title="Top categories"><BarList data={rep.by_category} /></Card>
            <Card title="By type"><BarList data={rep.by_type} color="var(--joy-palette-primary-400)" /></Card>
            <Card title="By team"><BarList data={rep.by_group} /></Card>
            <Card title="SLA breaches by priority"><BarList data={rep.sla_breached_by_priority} color="var(--joy-palette-danger-500)" empty="No breaches in this period" /></Card>
            <Card title="Agent workload" wide>
              <Box sx={{ display: "grid", gridTemplateColumns: "1.5fr 80px 80px 2fr", gap: 1, fontSize: 12, color: "text.tertiary", pb: 0.5 }}><span>Agent</span><span>Open</span><span>Resolved</span><span /></Box>
              {rep.workload.map((w) => (
                <Box key={w.agent} sx={{ display: "grid", gridTemplateColumns: "1.5fr 80px 80px 2fr", gap: 1, alignItems: "center", py: 0.5, borderTop: "1px solid", borderColor: "divider" }}>
                  <Typography level="body-sm">{w.agent}</Typography><Typography level="body-sm">{w.open}</Typography><Typography level="body-sm">{w.resolved}</Typography>
                  <Box sx={{ height: 8, borderRadius: 4, bgcolor: "neutral.softBg", overflow: "hidden", display: "flex" }}>
                    <Box sx={{ width: `${(w.open / Math.max(1, ...rep.workload.map((x) => x.open + x.resolved))) * 100}%`, bgcolor: "warning.400" }} />
                    <Box sx={{ width: `${(w.resolved / Math.max(1, ...rep.workload.map((x) => x.open + x.resolved))) * 100}%`, bgcolor: "success.400" }} />
                  </Box>
                </Box>
              ))}
              {rep.time_logged_min > 0 && <Typography level="body-xs" sx={{ mt: 1, color: "text.tertiary" }}>Time logged on these tickets in the period: {fmtMin(rep.time_logged_min)}</Typography>}
            </Card>
          </Box>
        </>
      ))}
    </Box>
  );
}

function Kpi({ label, value, hint, tone }: { label: string; value: string | number; hint?: string; tone?: "success" | "danger" }) {
  return (
    <Sheet variant="outlined" sx={{ borderRadius: "lg", p: 1.75 }}>
      <Typography level="body-xs" sx={{ color: "text.tertiary", textTransform: "uppercase", letterSpacing: 0.4 }}>{label}</Typography>
      <Typography level="h3" sx={{ color: tone === "danger" ? "danger.600" : "text.primary" }}>{value}</Typography>
      {hint && <Typography level="body-xs" sx={{ color: "text.tertiary" }}>{hint}</Typography>}
    </Sheet>
  );
}
function Card({ title, children, wide }: { title: string; children: React.ReactNode; wide?: boolean }) {
  return (
    <Sheet variant="outlined" sx={{ borderRadius: "lg", p: 2, gridColumn: wide ? { lg: "1 / 3" } : undefined }}>
      <Typography level="title-sm" sx={{ mb: 1.25 }}>{title}</Typography>
      {children}
    </Sheet>
  );
}
function BarList({ data, color = "var(--joy-palette-primary-500)", empty = "Nothing to show" }: { data: Bucket[]; color?: string; empty?: string }) {
  const max = Math.max(1, ...data.map((d) => d.count));
  if (!data.length || data.every((d) => d.count === 0)) return <Typography level="body-sm" sx={{ color: "text.tertiary" }}>{empty}</Typography>;
  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 0.9 }}>
      {data.map((d) => (
        <Box key={d.label} sx={{ display: "grid", gridTemplateColumns: "110px 1fr 36px", gap: 1, alignItems: "center" }}>
          <Typography level="body-sm" noWrap>{d.label}</Typography>
          <Box sx={{ height: 10, borderRadius: 5, bgcolor: "neutral.softBg" }}><Box sx={{ height: "100%", width: `${(d.count / max) * 100}%`, borderRadius: 5, bgcolor: color }} /></Box>
          <Typography level="body-sm" sx={{ textAlign: "right", fontWeight: 600 }}>{d.count}</Typography>
        </Box>
      ))}
    </Box>
  );
}
function Trend({ a, b }: { a: DayCount[]; b: DayCount[] }) {
  const n = Math.max(a.length, b.length);
  const max = Math.max(1, ...a.map((d) => d.count), ...b.map((d) => d.count));
  const W = 760, H = 170, pad = 22;
  const pts = (list: DayCount[]) => list.map((d, i) => `${pad + (i / Math.max(1, n - 1)) * (W - pad * 2)},${H - pad - (d.count / max) * (H - pad * 2)}`).join(" ");
  return (
    <Box>
      <svg viewBox={`0 0 ${W} ${H}`} width="100%" role="img" aria-label="Created and resolved tickets per day">
        {[0, 0.5, 1].map((t) => <line key={t} x1={pad} x2={W - pad} y1={H - pad - t * (H - pad * 2)} y2={H - pad - t * (H - pad * 2)} stroke="currentColor" strokeOpacity=".1" />)}
        <polyline fill="none" stroke="var(--joy-palette-primary-500)" strokeWidth="2.4" points={pts(a)} />
        <polyline fill="none" stroke="var(--joy-palette-success-400)" strokeWidth="2.4" strokeDasharray="5 4" points={pts(b)} />
        <text x={pad} y={12} fontSize="11" fill="currentColor" fillOpacity=".55">peak {max}</text>
      </svg>
      <Box sx={{ display: "flex", gap: 2, mt: 0.5 }}>
        <Typography level="body-xs"><span style={{ color: "var(--joy-palette-primary-500)" }}>━</span> Created</Typography>
        <Typography level="body-xs"><span style={{ color: "var(--joy-palette-success-400)" }}>╍</span> Resolved</Typography>
        <Typography level="body-xs" sx={{ ml: "auto", color: "text.tertiary" }}>{a[0]?.day} to {a[a.length - 1]?.day}</Typography>
      </Box>
    </Box>
  );
}
