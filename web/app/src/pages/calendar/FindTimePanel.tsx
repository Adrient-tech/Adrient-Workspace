import { useState } from "react";
import { Alert, Box, Button, CircularProgress, Typography } from "@mui/joy";
import { findTime, type Suggestion } from "./extrasApi";

const fmt = new Intl.DateTimeFormat(undefined, { weekday: "short", day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" });

/** Suggests times when the guests (and you) are free; picking one sets the event time. */
export function FindTimePanel({ emails, durationMin, from, onPick }: {
  emails: string[];
  durationMin: number;
  from: Date;
  onPick: (start: Date, end: Date) => void;
}) {
  const [busy, setBusy] = useState(false);
  const [list, setList] = useState<Suggestion[] | null>(null);
  const [outside, setOutside] = useState<string[]>([]);
  const [err, setErr] = useState("");

  async function run() {
    setBusy(true);
    setErr("");
    try {
      const start = new Date(Math.max(from.getTime(), Date.now()));
      const to = new Date(start.getTime() + 14 * 24 * 3600 * 1000);
      const r = await findTime({ emails, from: start.toISOString(), to: to.toISOString(), duration_min: Math.max(5, durationMin), limit: 8 });
      setList(r.suggestions);
      setOutside(r.not_in_org);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <Box sx={{ mt: 1 }} data-testid="find-time">
      <Button size="sm" variant="soft" color="neutral" onClick={run} loading={busy} data-testid="find-time-btn">
        Find a time
      </Button>
      {err && <Alert color="danger" sx={{ mt: 1 }}>{err}</Alert>}
      {busy && <CircularProgress size="sm" sx={{ ml: 1 }} />}
      {list && list.length === 0 && (
        <Typography level="body-sm" sx={{ mt: 1 }}>No shared free time in the next 14 days.</Typography>
      )}
      {list && list.length > 0 && (
        <Box sx={{ display: "flex", flexDirection: "column", gap: 0.5, mt: 1 }}>
          {list.map((s) => (
            <Button
              key={s.start}
              size="sm"
              variant="outlined"
              color={s.conflicts.length ? "warning" : "success"}
              sx={{ justifyContent: "flex-start", textAlign: "left", whiteSpace: "normal" }}
              data-testid="find-time-option"
              onClick={() => onPick(new Date(s.start), new Date(s.end))}
            >
              {fmt.format(new Date(s.start))}
              {s.conflicts.length > 0 && ` · busy or off hours: ${s.conflicts.join(", ")}`}
            </Button>
          ))}
        </Box>
      )}
      {outside.length > 0 && (
        <Typography level="body-xs" sx={{ mt: 0.5 }}>
          Not in your organisation, so availability is unknown: {outside.join(", ")}
        </Typography>
      )}
    </Box>
  );
}
