import { Box, Typography } from "@mui/joy";
import { agendaTime, groupAgenda, AGENDA_DAYS } from "./agenda";
import type { CalendarEvent } from "./types";

/** A scrolling list of the next 30 days, one block per day that has something on it. */
export function AgendaView({ from, events, onEventClick }: { from: Date; events: CalendarEvent[]; onEventClick: (e: CalendarEvent) => void }) {
  const days = groupAgenda(events, from, AGENDA_DAYS);
  const today = new Date();
  return (
    <Box sx={{ flex: 1, overflowY: "auto", p: { xs: 1, sm: 2 } }} data-testid="agenda-view">
      {days.length === 0 && (
        <Typography level="body-md" sx={{ opacity: 0.6, textAlign: "center", py: 6 }}>
          Nothing scheduled in the next {AGENDA_DAYS} days.
        </Typography>
      )}
      {days.map((d) => {
        const isToday = d.date.toDateString() === today.toDateString();
        return (
          <Box key={d.key} sx={{ display: "flex", gap: 2, py: 1, borderBottom: "1px solid", borderColor: "divider" }} data-testid="agenda-day">
            <Box sx={{ width: { xs: 56, sm: 90 }, flexShrink: 0 }}>
              <Typography level="title-sm" color={isToday ? "primary" : undefined}>
                {d.date.toLocaleDateString([], { weekday: "short" })}
              </Typography>
              <Typography level="body-xs" color={isToday ? "primary" : undefined}>
                {d.date.toLocaleDateString([], { month: "short", day: "numeric" })}
              </Typography>
            </Box>
            <Box sx={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", gap: 0.5 }}>
              {d.events.map((e) => (
                <Box
                  key={`${e.id}:${d.key}`}
                  role="button"
                  tabIndex={0}
                  onClick={() => onEventClick(e)}
                  onKeyDown={(k) => (k.key === "Enter" || k.key === " ") && onEventClick(e)}
                  sx={{ display: "flex", gap: 1.5, alignItems: "center", cursor: "pointer", borderRadius: "sm", px: 1, py: 0.5, "&:hover": { bgcolor: "background.level1" } }}
                  data-testid="agenda-event"
                >
                  <Box sx={{ width: 10, height: 10, borderRadius: "50%", bgcolor: e.color || "primary.500", flexShrink: 0 }} />
                  <Typography level="body-sm" sx={{ width: { xs: 90, sm: 150 }, flexShrink: 0, opacity: 0.8 }}>
                    {agendaTime(e, d.date)}
                  </Typography>
                  <Box sx={{ minWidth: 0 }}>
                    <Typography level="title-sm" noWrap>{e.title || "(No title)"}</Typography>
                    {e.location && <Typography level="body-xs" noWrap>{e.location}</Typography>}
                  </Box>
                </Box>
              ))}
            </Box>
          </Box>
        );
      })}
    </Box>
  );
}
