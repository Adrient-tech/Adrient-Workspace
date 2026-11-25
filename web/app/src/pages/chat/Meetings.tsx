/** Chat > Meetings: today's and upcoming calendar events with a Join button. */
import { useEffect, useState } from "react";
import { Alert, Box, Button, CircularProgress, Typography } from "@mui/joy";
import VideocamOutlinedIcon from "@mui/icons-material/VideocamOutlined";
import { useNavigate } from "react-router-dom";
import { getEventMeet, listEvents } from "../calendar/api";
import type { CalendarEvent } from "../calendar/types";
import { meetPath } from "../../lib/calls";

interface Row {
  ev: CalendarEvent;
  code: string;
}

const DAYS = 7;

function when(ev: CalendarEvent): string {
  if (ev.all_day) return "All day";
  const f = (s: string) => new Date(s).toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
  return `${f(ev.start_at)} – ${f(ev.end_at)}`;
}

export function MeetingsView({ onSchedule, onMeetNow }: { onSchedule: () => void; onMeetNow: () => void }) {
  const navigate = useNavigate();
  const [rows, setRows] = useState<Row[] | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    let live = true;
    const start = new Date();
    start.setHours(0, 0, 0, 0);
    const end = new Date(start.getTime() + DAYS * 86400_000);
    listEvents(start.toISOString(), end.toISOString())
      .then(async (evs) => {
        const real = evs
          .filter((e) => (e.item_type ?? "event") === "event" && new Date(e.end_at).getTime() >= Date.now() - 3600_000)
          .sort((a, b) => a.start_at.localeCompare(b.start_at))
          .slice(0, 40);
        const withMeet = await Promise.all(
          real.map(async (ev) => ({ ev, code: (await getEventMeet(ev.id).catch(() => null))?.code ?? "" })),
        );
        if (live) setRows(withMeet);
      })
      .catch((e: Error) => live && setError(e.message));
    return () => {
      live = false;
    };
  }, []);

  const todayKey = new Date().toDateString();
  const today = (rows ?? []).filter((r) => new Date(r.ev.start_at).toDateString() === todayKey || new Date(r.ev.start_at) < new Date());
  const later = (rows ?? []).filter((r) => !today.includes(r));

  const section = (title: string, list: Row[]) =>
    list.length > 0 && (
      <Box sx={{ mb: 2 }}>
        <Typography level="title-sm" sx={{ mb: 0.5 }}>
          {title}
        </Typography>
        {list.map(({ ev, code }) => (
          <Box
            key={ev.id + ev.start_at}
            data-testid="meeting-row"
            sx={{ display: "flex", alignItems: "center", gap: 1.5, py: 1, borderBottom: "1px solid", borderColor: "divider" }}
          >
            <Box sx={{ width: 4, alignSelf: "stretch", borderRadius: 2, bgcolor: ev.color || "primary.500" }} />
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <Typography level="title-sm" noWrap>
                {ev.title || "(no title)"}
              </Typography>
              <Typography level="body-xs" sx={{ opacity: 0.7 }}>
                {new Date(ev.start_at).toLocaleDateString([], { weekday: "short", month: "short", day: "numeric" })} · {when(ev)}
              </Typography>
            </Box>
            {code ? (
              <Button
                size="sm"
                startDecorator={<VideocamOutlinedIcon />}
                data-testid="meeting-join"
                onClick={() => navigate(meetPath(code, { returnTo: "/chat/meetings" }))}
              >
                Join
              </Button>
            ) : (
              <Typography level="body-xs" sx={{ opacity: 0.5 }}>
                No meeting link
              </Typography>
            )}
          </Box>
        ))}
      </Box>
    );

  return (
    <Box sx={{ flex: 1, overflowY: "auto", p: 3, maxWidth: 760 }} data-testid="meetings-view">
      <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 2 }}>
        <Typography level="title-lg" sx={{ flex: 1 }}>
          Meetings
        </Typography>
        <Button size="sm" variant="soft" startDecorator={<VideocamOutlinedIcon />} onClick={onMeetNow} data-testid="meetings-meet-now">
          Meet now
        </Button>
        <Button size="sm" onClick={onSchedule} data-testid="meetings-schedule">
          Schedule meeting
        </Button>
      </Box>
      {error && <Alert color="danger">{error}</Alert>}
      {!rows && !error && <CircularProgress size="sm" />}
      {rows && rows.length === 0 && <Typography level="body-sm">No meetings in the next {DAYS} days.</Typography>}
      {section("Today", today)}
      {section("Upcoming", later)}
    </Box>
  );
}
