import { useMemo, useState } from "react";
import { Box, Chip, IconButton, Sheet, Typography } from "@mui/joy";
import ChevronLeftIcon from "@mui/icons-material/ChevronLeft";
import ChevronRightIcon from "@mui/icons-material/ChevronRight";
import { byDueDay, isOverdue, monthCells, toColumns, type Column, type TaskExtras } from "./logic";
import type { Task } from "./types";

const COLS: { id: Column; title: string }[] = [
  { id: "todo", title: "To do" },
  { id: "doing", title: "In progress" },
  { id: "done", title: "Done" },
];

/** Three-column board. Dragging a card between columns changes its status. */
export function BoardView({ tasks, extras, readOnly, onMove, onOpen }: {
  tasks: Task[];
  extras: Record<string, TaskExtras>;
  readOnly: boolean;
  onMove: (t: Task, to: Column) => void;
  onOpen: (t: Task) => void;
}) {
  const cols = useMemo(() => toColumns(tasks, extras), [tasks, extras]);
  const [over, setOver] = useState<Column | null>(null);
  return (
    <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "repeat(3, 1fr)" }, gap: 1.5 }} data-testid="board">
      {COLS.map((c) => (
        <Sheet
          key={c.id}
          variant="soft"
          data-testid={`col-${c.id}`}
          onDragOver={(e) => {
            if (readOnly) return;
            e.preventDefault();
            setOver(c.id);
          }}
          onDragLeave={() => setOver((o) => (o === c.id ? null : o))}
          onDrop={(e) => {
            e.preventDefault();
            setOver(null);
            const id = e.dataTransfer.getData("text/task");
            const t = tasks.find((x) => x.id === id);
            if (t && !readOnly) onMove(t, c.id);
          }}
          sx={{ p: 1, borderRadius: "md", minHeight: 160, outline: over === c.id ? "2px solid" : "none", outlineColor: "primary.500" }}
        >
          <Typography level="title-sm" sx={{ mb: 1 }}>
            {c.title} <Chip size="sm" variant="outlined">{cols[c.id].length}</Chip>
          </Typography>
          <Box sx={{ display: "flex", flexDirection: "column", gap: 0.75 }}>
            {cols[c.id].map((t) => (
              <Sheet
                key={t.id}
                variant="outlined"
                draggable={!readOnly}
                onDragStart={(e) => e.dataTransfer.setData("text/task", t.id)}
                onClick={() => onOpen(t)}
                data-testid="board-card"
                sx={{ p: 1, borderRadius: "sm", cursor: readOnly ? "pointer" : "grab", bgcolor: "background.surface" }}
              >
                <Typography level="body-sm" sx={{ textDecoration: t.completed ? "line-through" : "none" }}>{t.title}</Typography>
                <Box sx={{ display: "flex", gap: 0.5, flexWrap: "wrap", mt: 0.25 }}>
                  {t.due_at && (
                    <Typography level="body-xs" color={isOverdue(t) ? "danger" : "neutral"}>
                      {new Date(t.due_at).toLocaleDateString(undefined, { month: "short", day: "numeric" })}
                    </Typography>
                  )}
                  {extras[t.id]?.assignee_name && <Typography level="body-xs">{extras[t.id].assignee_name}</Typography>}
                </Box>
              </Sheet>
            ))}
          </Box>
          {!readOnly && c.id !== "done" && (
            <Box sx={{ display: { xs: "block", md: "none" }, mt: 1 }}>
              <Typography level="body-xs">Tap a card to edit it. On a wide screen, drag cards between columns.</Typography>
            </Box>
          )}
        </Sheet>
      ))}
    </Box>
  );
}

const DOW = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];
const MONTHS = ["January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"];

/** Month calendar of tasks by due date. Clicking a task opens it. */
export function TaskCalendar({ tasks, onOpen }: { tasks: Task[]; onOpen: (t: Task) => void }) {
  const [cur, setCur] = useState(() => {
    const n = new Date();
    return { y: n.getFullYear(), m: n.getMonth() };
  });
  const cells = useMemo(() => monthCells(cur.y, cur.m), [cur]);
  const by = useMemo(() => byDueDay(tasks), [tasks]);
  const today = new Date();
  const todayKey = `${today.getFullYear()}-${String(today.getMonth() + 1).padStart(2, "0")}-${String(today.getDate()).padStart(2, "0")}`;
  const shift = (d: number) => setCur((c) => { const n = new Date(c.y, c.m + d, 1); return { y: n.getFullYear(), m: n.getMonth() }; });
  return (
    <Box data-testid="task-calendar">
      <Box sx={{ display: "flex", alignItems: "center", mb: 1 }}>
        <Typography level="title-md" sx={{ flex: 1 }}>{MONTHS[cur.m]} {cur.y}</Typography>
        <IconButton size="sm" aria-label="Previous month" onClick={() => shift(-1)}><ChevronLeftIcon /></IconButton>
        <IconButton size="sm" aria-label="Next month" onClick={() => shift(1)}><ChevronRightIcon /></IconButton>
      </Box>
      <Box sx={{ display: "grid", gridTemplateColumns: "repeat(7, minmax(0, 1fr))", gap: 0.5 }}>
        {DOW.map((d) => (
          <Typography key={d} level="body-xs" sx={{ textAlign: "center" }}>{d}</Typography>
        ))}
        {cells.map((k, i) =>
          k === null ? (
            <Box key={`b${i}`} />
          ) : (
            <Sheet key={k} variant={k === todayKey ? "soft" : "outlined"} color={k === todayKey ? "primary" : "neutral"} sx={{ p: 0.5, minHeight: { xs: 52, sm: 84 }, borderRadius: "sm", overflow: "hidden" }}>
              <Typography level="body-xs">{Number(k.slice(8))}</Typography>
              {(by[k] ?? []).slice(0, 3).map((t) => (
                <Typography
                  key={t.id}
                  level="body-xs"
                  noWrap
                  onClick={() => onOpen(t)}
                  data-testid="cal-task"
                  sx={{ cursor: "pointer", bgcolor: t.completed ? "neutral.softBg" : isOverdue(t) ? "danger.softBg" : "primary.softBg", borderRadius: "xs", px: 0.5, mb: 0.25, textDecoration: t.completed ? "line-through" : "none" }}
                >
                  {t.title}
                </Typography>
              ))}
              {(by[k]?.length ?? 0) > 3 && <Typography level="body-xs">+{by[k].length - 3} more</Typography>}
            </Sheet>
          ),
        )}
      </Box>
    </Box>
  );
}
