import { useEffect, useMemo, useState } from "react";
import { useParams } from "react-router-dom";
import { Alert, Box, Button, CircularProgress, FormControl, FormLabel, IconButton, Input, Sheet, Textarea, Typography } from "@mui/joy";
import ChevronLeftIcon from "@mui/icons-material/ChevronLeft";
import ChevronRightIcon from "@mui/icons-material/ChevronRight";
import AccessTimeIcon from "@mui/icons-material/AccessTime";
import PlaceIcon from "@mui/icons-material/Place";
import VideocamIcon from "@mui/icons-material/Videocam";
import * as api from "./api";
import { detectTimeZone, formatDuration, fullWhen, groupSlotsByDay, isValidEmail } from "./logic";

const ACCENT = "#126C5A";
const MONTHS = ["January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"];
const DOW = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];

function tzList(current: string): string[] {
  try {
    const l = (Intl as unknown as { supportedValuesOf?: (k: string) => string[] }).supportedValuesOf?.("timeZone") ?? [];
    return l.includes(current) ? l : [current, ...l];
  } catch {
    return [current];
  }
}

/** Public booking page at /book/:slug. No sign-in. */
export function PublicBookingPage() {
  const { slug = "" } = useParams();
  const [page, setPage] = useState<api.PublicPage | null>(null);
  const [missing, setMissing] = useState(false);
  const [tz, setTz] = useState(detectTimeZone());
  const [cursor, setCursor] = useState(() => {
    const n = new Date();
    return new Date(n.getFullYear(), n.getMonth(), 1);
  });
  const [slots, setSlots] = useState<string[]>([]);
  const [loadingSlots, setLoadingSlots] = useState(false);
  const [day, setDay] = useState<string | null>(null);
  const [chosen, setChosen] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [answers, setAnswers] = useState<Record<string, string>>({});
  const [website, setWebsite] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [done, setDone] = useState<{ start_at: string; meet_url?: string; cancel_token: string; email_sent?: boolean } | null>(null);

  useEffect(() => {
    api.publicPage(slug).then(setPage).catch(() => setMissing(true));
  }, [slug]);

  // Load slots for the visible month (guest's zone does not change which instants exist).
  useEffect(() => {
    if (!page) return;
    const from = new Date(cursor.getFullYear(), cursor.getMonth(), 1);
    from.setDate(from.getDate() - 1);
    const to = new Date(cursor.getFullYear(), cursor.getMonth() + 1, 2);
    setLoadingSlots(true);
    api
      .publicSlots(slug, from.toISOString(), to.toISOString())
      .then((r) => setSlots(r.slots ?? []))
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoadingSlots(false));
  }, [page, slug, cursor]);

  const byDay = useMemo(() => groupSlotsByDay(slots, tz), [slots, tz]);
  const grid = useMemo(() => {
    const first = new Date(cursor.getFullYear(), cursor.getMonth(), 1);
    const lead = (first.getDay() + 6) % 7; // Monday first
    const n = new Date(cursor.getFullYear(), cursor.getMonth() + 1, 0).getDate();
    const cells: (string | null)[] = Array(lead).fill(null);
    for (let d = 1; d <= n; d++) {
      cells.push(`${cursor.getFullYear()}-${String(cursor.getMonth() + 1).padStart(2, "0")}-${String(d).padStart(2, "0")}`);
    }
    return cells;
  }, [cursor]);

  async function submit() {
    if (!page || !chosen) return;
    setError("");
    if (!name.trim()) return setError("Please enter your name.");
    if (!isValidEmail(email)) return setError("Please enter a valid email address.");
    for (const q of page.questions) {
      if (q.required && !(answers[q.id] ?? "").trim()) return setError(`Please answer: ${q.label}`);
    }
    setBusy(true);
    try {
      const r = await api.publicBook(slug, { start: chosen, name: name.trim(), email: email.trim(), tz, answers, website });
      setDone({ start_at: r.start_at, meet_url: r.meet_url, cancel_token: r.cancel_token, email_sent: r.email_sent });
    } catch (e) {
      const msg = (e as Error).message;
      setError(msg);
      if (/no longer available|not offered/i.test(msg)) {
        setChosen(null); // refresh the list so the guest picks another time
        setCursor((c) => new Date(c));
      }
    } finally {
      setBusy(false);
    }
  }

  if (missing) {
    return (
      <Box sx={{ p: 6, textAlign: "center" }}>
        <Typography level="h3" data-testid="book-missing">
          This booking page is not available
        </Typography>
      </Box>
    );
  }
  if (!page) return <Box sx={{ p: 6, textAlign: "center" }}><CircularProgress /></Box>;

  if (done) {
    return (
      <Box sx={{ minHeight: "100vh", bgcolor: "background.level1", display: "grid", placeItems: "center", p: 2 }}>
        <Sheet variant="outlined" sx={{ p: 3, borderRadius: "lg", maxWidth: 520, width: "100%" }} data-testid="book-done">
          <Typography level="h3" sx={{ color: ACCENT }}>You are booked</Typography>
          <Typography level="title-md" sx={{ mt: 1 }}>{page.title}</Typography>
          <Typography sx={{ mt: 0.5 }}>{fullWhen(done.start_at, tz)}</Typography>
          {done.meet_url && (
            <Typography sx={{ mt: 1, wordBreak: "break-all" }}>
              Video call: <a href={done.meet_url}>{done.meet_url}</a>
            </Typography>
          )}
          {done.email_sent ? (
            <Typography level="body-sm" sx={{ mt: 2 }}>
              A confirmation with a calendar invite was emailed to {email}.
            </Typography>
          ) : (
            <Typography level="body-sm" sx={{ mt: 2 }}>
              Save this page. Email confirmations are not enabled on this server.
            </Typography>
          )}
          <Button component="a" href={`/book/cancel/${done.cancel_token}`} variant="plain" color="neutral" sx={{ mt: 1 }}>
            Need to cancel?
          </Button>
        </Sheet>
      </Box>
    );
  }

  const times = day ? (byDay.get(day) ?? []) : [];
  const monthLabel = `${MONTHS[cursor.getMonth()]} ${cursor.getFullYear()}`;

  return (
    <Box sx={{ minHeight: "100vh", bgcolor: "background.level1", display: "grid", placeItems: "start center", p: { xs: 1, sm: 3 } }}>
      <Sheet variant="outlined" sx={{ borderRadius: "lg", maxWidth: 940, width: "100%", display: "flex", flexWrap: "wrap" }}>
        <Box sx={{ p: 3, flex: "1 1 260px", borderRight: { sm: "1px solid" }, borderColor: "neutral.outlinedBorder" }}>
          {page.owner_name && <Typography level="body-sm" data-testid="book-owner">{page.owner_name}</Typography>}
          <Typography level="h3" sx={{ color: ACCENT }} data-testid="book-title">{page.title}</Typography>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1, mt: 1.5 }}>
            <AccessTimeIcon fontSize="small" /> <Typography level="body-sm">{formatDuration(page.duration_min)}</Typography>
          </Box>
          {page.has_meet && (
            <Box sx={{ display: "flex", alignItems: "center", gap: 1, mt: 0.5 }}>
              <VideocamIcon fontSize="small" /> <Typography level="body-sm">Video call link included</Typography>
            </Box>
          )}
          {page.location && (
            <Box sx={{ display: "flex", alignItems: "center", gap: 1, mt: 0.5 }}>
              <PlaceIcon fontSize="small" /> <Typography level="body-sm">{page.location}</Typography>
            </Box>
          )}
          {page.description && <Typography level="body-sm" sx={{ mt: 2, whiteSpace: "pre-wrap" }}>{page.description}</Typography>}
          {chosen && (
            <Typography level="title-sm" sx={{ mt: 2 }} data-testid="book-chosen">{fullWhen(chosen, tz)}</Typography>
          )}
        </Box>

        {!chosen ? (
          <Box sx={{ p: 3, flex: "2 1 420px", display: "flex", gap: 3, flexWrap: "wrap" }}>
            <Box sx={{ flex: "1 1 280px", minWidth: 260 }}>
              <Box sx={{ display: "flex", alignItems: "center", mb: 1 }}>
                <Typography level="title-md" sx={{ flex: 1 }}>{monthLabel}</Typography>
                <IconButton size="sm" aria-label="Previous month" onClick={() => setCursor((c) => new Date(c.getFullYear(), c.getMonth() - 1, 1))}>
                  <ChevronLeftIcon />
                </IconButton>
                <IconButton size="sm" aria-label="Next month" onClick={() => setCursor((c) => new Date(c.getFullYear(), c.getMonth() + 1, 1))}>
                  <ChevronRightIcon />
                </IconButton>
              </Box>
              <Box sx={{ display: "grid", gridTemplateColumns: "repeat(7, 1fr)", gap: 0.5, textAlign: "center" }}>
                {DOW.map((d) => (
                  <Typography key={d} level="body-xs">{d}</Typography>
                ))}
                {grid.map((k, i) =>
                  k === null ? (
                    <Box key={`b${i}`} />
                  ) : (
                    <Button
                      key={k}
                      size="sm"
                      variant={day === k ? "solid" : byDay.has(k) ? "soft" : "plain"}
                      color={byDay.has(k) ? "success" : "neutral"}
                      disabled={!byDay.has(k)}
                      onClick={() => setDay(k)}
                      data-testid={byDay.has(k) ? "book-day" : undefined}
                      sx={{ minWidth: 0, px: 0, ...(day === k ? { bgcolor: ACCENT } : {}) }}
                    >
                      {Number(k.slice(8))}
                    </Button>
                  ),
                )}
              </Box>
              <FormControl size="sm" sx={{ mt: 2 }}>
                <FormLabel>Time zone</FormLabel>
                <select
                  value={tz}
                  onChange={(e) => setTz(e.target.value)}
                  aria-label="Time zone"
                  style={{ padding: 6, borderRadius: 6, maxWidth: "100%" }}
                >
                  {tzList(tz).map((z) => (
                    <option key={z} value={z}>{z}</option>
                  ))}
                </select>
              </FormControl>
            </Box>
            <Box sx={{ flex: "1 1 160px", minWidth: 150 }}>
              {loadingSlots && <CircularProgress size="sm" />}
              {!loadingSlots && byDay.size === 0 && (
                <Typography level="body-sm" data-testid="book-none">No times are available this month.</Typography>
              )}
              {day && (
                <Box sx={{ display: "flex", flexDirection: "column", gap: 0.75, maxHeight: 360, overflowY: "auto" }}>
                  {times.map((t) => (
                    <Button key={t.iso} variant="outlined" color="success" onClick={() => setChosen(t.iso)} data-testid="book-slot">
                      {t.label}
                    </Button>
                  ))}
                </Box>
              )}
              {!day && byDay.size > 0 && <Typography level="body-sm">Pick a day to see times.</Typography>}
              {error && <Alert color="danger" sx={{ mt: 1 }}>{error}</Alert>}
            </Box>
          </Box>
        ) : (
          <Box sx={{ p: 3, flex: "2 1 420px" }}>
            <Button size="sm" variant="plain" onClick={() => setChosen(null)} startDecorator={<ChevronLeftIcon />}>
              Back
            </Button>
            <Typography level="title-lg" sx={{ my: 1 }}>Enter your details</Typography>
            <FormControl required sx={{ mb: 1.5 }}>
              <FormLabel>Name</FormLabel>
              <Input value={name} onChange={(e) => setName(e.target.value)} slotProps={{ input: { "data-testid": "book-name", maxLength: 120 } }} />
            </FormControl>
            <FormControl required sx={{ mb: 1.5 }}>
              <FormLabel>Email</FormLabel>
              <Input type="email" value={email} onChange={(e) => setEmail(e.target.value)} slotProps={{ input: { "data-testid": "book-email", maxLength: 254 } }} />
            </FormControl>
            {page.questions.map((q) => (
              <FormControl key={q.id} required={q.required} sx={{ mb: 1.5 }}>
                <FormLabel>{q.label}</FormLabel>
                {q.type === "longtext" ? (
                  <Textarea minRows={3} value={answers[q.id] ?? ""} onChange={(e) => setAnswers((a) => ({ ...a, [q.id]: e.target.value }))} slotProps={{ textarea: { maxLength: 2000 } }} />
                ) : (
                  <Input value={answers[q.id] ?? ""} onChange={(e) => setAnswers((a) => ({ ...a, [q.id]: e.target.value }))} slotProps={{ input: { maxLength: 2000 } }} />
                )}
              </FormControl>
            ))}
            {/* Honeypot: hidden from people. */}
            <input
              tabIndex={-1}
              autoComplete="off"
              aria-hidden="true"
              value={website}
              onChange={(e) => setWebsite(e.target.value)}
              style={{ position: "absolute", left: "-9999px", width: 1, height: 1, opacity: 0 }}
            />
            {error && <Alert color="danger" sx={{ mb: 1 }} data-testid="book-error">{error}</Alert>}
            <Button loading={busy} onClick={submit} sx={{ bgcolor: ACCENT }} data-testid="book-confirm">
              Confirm booking
            </Button>
          </Box>
        )}
      </Sheet>
    </Box>
  );
}

/** Guest cancel page at /book/cancel/:token. */
export function CancelBookingPage() {
  const { token = "" } = useParams();
  const [a, setA] = useState<Awaited<ReturnType<typeof api.publicAppointment>> | null>(null);
  const [missing, setMissing] = useState(false);
  const [cancelled, setCancelled] = useState(false);
  const [err, setErr] = useState("");
  useEffect(() => {
    api.publicAppointment(token).then(setA).catch(() => setMissing(true));
  }, [token]);
  if (missing) return <Box sx={{ p: 6, textAlign: "center" }}><Typography level="h3">Appointment not found</Typography></Box>;
  if (!a) return <Box sx={{ p: 6, textAlign: "center" }}><CircularProgress /></Box>;
  const isCancelled = cancelled || a.status !== "confirmed";
  return (
    <Box sx={{ minHeight: "100vh", bgcolor: "background.level1", display: "grid", placeItems: "center", p: 2 }}>
      <Sheet variant="outlined" sx={{ p: 3, borderRadius: "lg", maxWidth: 520, width: "100%" }}>
        <Typography level="h3" sx={{ color: ACCENT }}>{a.title}</Typography>
        <Typography sx={{ mt: 1 }}>{fullWhen(a.start_at, a.guest_tz || detectTimeZone())}</Typography>
        {isCancelled ? (
          <Alert color="neutral" sx={{ mt: 2 }} data-testid="cancel-done">This appointment is cancelled.</Alert>
        ) : (
          <>
            {err && <Alert color="danger" sx={{ mt: 2 }}>{err}</Alert>}
            <Button
              color="danger"
              sx={{ mt: 2 }}
              data-testid="cancel-confirm"
              onClick={() => api.publicCancel(token).then(() => setCancelled(true)).catch((e: Error) => setErr(e.message))}
            >
              Cancel appointment
            </Button>
          </>
        )}
      </Sheet>
    </Box>
  );
}
