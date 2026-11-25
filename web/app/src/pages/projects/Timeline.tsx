import { useMemo, useState } from "react";
import { Box, Button, Chip, Sheet, ToggleButtonGroup, Typography } from "@mui/joy";
import { depConflicts, layoutChart, monthSpans, timeSummary, type IssuePlan, type Milestone } from "./plan";
import type { Issue } from "./types";

const ROW = 30;
const HEAD = 44;
const LABEL_W = 220;

/** Gantt chart of issues by start and due date, with milestones and prerequisites. */
export function TimelineView({ issues, plans, milestones, onOpen }: {
  issues: Issue[];
  plans: Record<string, IssuePlan>;
  milestones: Milestone[];
  onOpen: (id: string) => void;
}) {
  const [zoom, setZoom] = useState<"day" | "week">("day");
  const dayW = zoom === "day" ? 28 : 12;
  const top = useMemo(() => issues.filter((i) => !i.parent_issue_id), [issues]);
  const chart = useMemo(() => layoutChart(top.map((i) => i.id), plans, milestones, new Date()), [top, plans, milestones]);
  const conflicts = useMemo(() => depConflicts(plans), [plans]);
  const conflicted = new Set(conflicts.map((c) => c.issue));
  const scheduled = top.filter((i) => chart.bars[i.id]);
  const rowOf = new Map(scheduled.map((i, n) => [i.id, n]));
  const width = chart.days * dayW;
  const height = scheduled.length * ROW;
  const months = monthSpans(chart.startDay, chart.days);

  return (
    <Box sx={{ p: 2 }} data-testid="timeline">
      <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1.5, flexWrap: "wrap" }}>
        <Typography level="title-md" sx={{ flex: 1 }}>Timeline</Typography>
        {conflicts.length > 0 && <Chip size="sm" color="warning" variant="soft" data-testid="timeline-conflicts">{conflicts.length} schedule conflict{conflicts.length === 1 ? "" : "s"}</Chip>}
        <ToggleButtonGroup size="sm" value={zoom} onChange={(_, v) => v && setZoom(v)}>
          <Button value="day">Days</Button>
          <Button value="week">Compact</Button>
        </ToggleButtonGroup>
      </Box>
      {scheduled.length === 0 && (
        <Sheet variant="soft" sx={{ p: 3, borderRadius: "md", mb: 2 }}>
          <Typography level="body-sm">No issue has dates yet. Open an issue and set a start and due date to see it here.</Typography>
        </Sheet>
      )}
      <Sheet variant="outlined" sx={{ borderRadius: "md", display: "flex", overflow: "hidden" }}>
        <Box sx={{ width: LABEL_W, flexShrink: 0, borderRight: "1px solid", borderColor: "divider" }}>
          <Box sx={{ height: HEAD, borderBottom: "1px solid", borderColor: "divider", px: 1, display: "flex", alignItems: "center" }}>
            <Typography level="body-xs">Issue</Typography>
          </Box>
          {scheduled.map((i) => (
            <Box key={i.id} onClick={() => onOpen(i.id)} sx={{ height: ROW, px: 1, display: "flex", alignItems: "center", cursor: "pointer", "&:hover": { bgcolor: "neutral.softBg" } }}>
              <Typography level="body-xs" noWrap>{i.identifier} {i.title}</Typography>
            </Box>
          ))}
        </Box>
        <Box sx={{ overflowX: "auto", flex: 1 }}>
          <Box sx={{ width, position: "relative", minHeight: HEAD + Math.max(height, ROW) }}>
            <Box sx={{ display: "flex", height: HEAD / 2, borderBottom: "1px solid", borderColor: "divider" }}>
              {months.map((m, k) => (
                <Typography key={k} level="body-xs" sx={{ width: m.days * dayW, px: 0.5, borderRight: "1px solid", borderColor: "divider", overflow: "hidden", whiteSpace: "nowrap" }}>{m.label}</Typography>
              ))}
            </Box>
            <Box sx={{ display: "flex", height: HEAD / 2, borderBottom: "1px solid", borderColor: "divider" }}>
              {Array.from({ length: chart.days }, (_, d) => {
                const date = new Date((chart.startDay + d) * 86400000);
                const wk = date.getUTCDay() === 0 || date.getUTCDay() === 6;
                return (
                  <Box key={d} sx={{ width: dayW, flexShrink: 0, bgcolor: wk ? "neutral.softBg" : undefined, textAlign: "center", fontSize: 10, lineHeight: `${HEAD / 2}px`, color: "text.tertiary" }}>
                    {zoom === "day" ? date.getUTCDate() : date.getUTCDate() % 7 === 1 ? date.getUTCDate() : ""}
                  </Box>
                );
              })}
            </Box>
            {chart.todayLeft !== null && <Box sx={{ position: "absolute", top: 0, bottom: 0, left: chart.todayLeft * dayW + dayW / 2, width: 2, bgcolor: "danger.400", opacity: 0.7, zIndex: 1 }} title="Today" />}
            {chart.milestones.map((m) => (
              <Box key={m.id} title={m.name} data-testid="timeline-milestone" sx={{ position: "absolute", top: HEAD / 2 + 4, left: m.left * dayW + dayW / 2 - 7, width: 14, height: 14, transform: "rotate(45deg)", bgcolor: m.done ? "success.500" : "warning.500", zIndex: 2 }} />
            ))}
            <svg width={width} height={height} style={{ position: "absolute", top: HEAD, left: 0, pointerEvents: "none" }}>
              {scheduled.flatMap((i) =>
                (plans[i.id]?.depends_on ?? []).map((d) => {
                  const a = chart.bars[d];
                  const b = chart.bars[i.id];
                  const ra = rowOf.get(d);
                  const rb = rowOf.get(i.id);
                  if (!a || !b || ra === undefined || rb === undefined) return null;
                  const x1 = (a.left + a.width) * dayW;
                  const y1 = ra * ROW + ROW / 2;
                  const x2 = b.left * dayW;
                  const y2 = rb * ROW + ROW / 2;
                  const bad = x2 < x1;
                  return <path key={`${d}-${i.id}`} d={`M${x1},${y1} C${x1 + 14},${y1} ${x2 - 14},${y2} ${x2},${y2}`} fill="none" stroke={bad ? "#d93025" : "#8a8f98"} strokeWidth={1.5} markerEnd="url(#arrow)" />;
                }),
              )}
              <defs>
                <marker id="arrow" markerWidth="6" markerHeight="6" refX="5" refY="3" orient="auto"><path d="M0,0 L6,3 L0,6 z" fill="#8a8f98" /></marker>
              </defs>
            </svg>
            <Box sx={{ position: "relative", height }}>
              {scheduled.map((i, n) => {
                const b = chart.bars[i.id];
                const ts = timeSummary(plans[i.id]);
                const done = i.status === "done";
                return (
                  <Box
                    key={i.id}
                    onClick={() => onOpen(i.id)}
                    title={`${i.identifier} ${i.title}${ts.text ? ` · ${ts.text}` : ""}${conflicted.has(i.id) ? " · starts before a prerequisite ends" : ""}`}
                    data-testid="timeline-bar"
                    sx={{
                      position: "absolute", top: n * ROW + 6, height: ROW - 12, left: b.left * dayW, width: b.width * dayW - 2, borderRadius: "6px",
                      bgcolor: done ? "success.400" : "primary.500", opacity: done ? 0.6 : 1, cursor: "pointer", overflow: "hidden", color: "#fff", fontSize: 11, px: 0.75,
                      lineHeight: `${ROW - 12}px`, whiteSpace: "nowrap", outline: conflicted.has(i.id) ? "2px solid #d93025" : "none",
                    }}
                  >
                    {b.width * dayW > 70 ? i.title : ""}
                  </Box>
                );
              })}
            </Box>
          </Box>
        </Box>
      </Sheet>
      {chart.unscheduled.length > 0 && (
        <Box sx={{ mt: 2 }} data-testid="timeline-unscheduled">
          <Typography level="title-sm" sx={{ mb: 0.5 }}>Not scheduled ({chart.unscheduled.length})</Typography>
          <Box sx={{ display: "flex", gap: 0.5, flexWrap: "wrap" }}>
            {chart.unscheduled.slice(0, 40).map((id) => {
              const i = top.find((x) => x.id === id)!;
              return <Chip key={id} size="sm" variant="outlined" onClick={() => onOpen(id)}>{i.identifier} {i.title.slice(0, 30)}</Chip>;
            })}
          </Box>
        </Box>
      )}
    </Box>
  );
}
