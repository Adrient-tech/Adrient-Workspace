import { useCallback, useEffect, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Checkbox,
  Chip,
  CircularProgress,
  FormControl,
  FormLabel,
  IconButton,
  Input,
  Modal,
  ModalClose,
  ModalDialog,
  Option,
  Select,
  Switch,
  Tab,
  TabList,
  TabPanel,
  Tabs,
  Textarea,
  Typography,
} from "@mui/joy";
import AddIcon from "@mui/icons-material/Add";
import DeleteIcon from "@mui/icons-material/DeleteOutline";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import * as api from "../booking/api";
import {
  WEEKDAYS,
  blankPage,
  cleanAvailability,
  detectTimeZone,
  fullWhen,
  publicLink,
  validateWindows,
  type BookingPage,
} from "../booking/logic";

type Draft = Omit<BookingPage, "id" | "slug"> & { id?: string; slug?: string };

const NOTICE = [
  [0, "No minimum"],
  [60, "1 hour"],
  [240, "4 hours"],
  [1440, "1 day"],
  [2880, "2 days"],
  [10080, "1 week"],
] as const;

function PageEditor({ draft, setDraft, onSave, onCancel, saving, error }: {
  draft: Draft;
  setDraft: (d: Draft) => void;
  onSave: () => void;
  onCancel: () => void;
  saving: boolean;
  error: string;
}) {
  const set = <K extends keyof Draft>(k: K, v: Draft[K]) => setDraft({ ...draft, [k]: v });
  const num = (k: keyof Draft, v: string) => set(k, (Math.max(0, Math.floor(Number(v) || 0)) as never));
  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5, overflowY: "auto", pr: 0.5 }}>
      <FormControl required>
        <FormLabel>Title</FormLabel>
        <Input value={draft.title} onChange={(e) => set("title", e.target.value)} placeholder="30 minute meeting" slotProps={{ input: { "data-testid": "bk-title", maxLength: 120 } }} />
      </FormControl>
      <FormControl>
        <FormLabel>Description</FormLabel>
        <Textarea minRows={2} value={draft.description} onChange={(e) => set("description", e.target.value)} />
      </FormControl>
      <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap" }}>
        <FormControl sx={{ width: 120 }}>
          <FormLabel>Duration (min)</FormLabel>
          <Input type="number" value={draft.duration_min} onChange={(e) => num("duration_min", e.target.value)} slotProps={{ input: { min: 5, max: 480, "data-testid": "bk-duration" } }} />
        </FormControl>
        <FormControl sx={{ width: 130 }}>
          <FormLabel>Buffer before</FormLabel>
          <Input type="number" value={draft.buffer_before_min} onChange={(e) => num("buffer_before_min", e.target.value)} endDecorator="min" />
        </FormControl>
        <FormControl sx={{ width: 130 }}>
          <FormLabel>Buffer after</FormLabel>
          <Input type="number" value={draft.buffer_after_min} onChange={(e) => num("buffer_after_min", e.target.value)} endDecorator="min" />
        </FormControl>
        <FormControl sx={{ width: 140 }}>
          <FormLabel>Start times every</FormLabel>
          <Input type="number" value={draft.slot_step_min} onChange={(e) => num("slot_step_min", e.target.value)} endDecorator="min" />
        </FormControl>
      </Box>
      <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap" }}>
        <FormControl sx={{ width: 170 }}>
          <FormLabel>Minimum notice</FormLabel>
          <Select value={draft.min_notice_min} onChange={(_, v) => set("min_notice_min", (v ?? 0) as number)}>
            {NOTICE.map(([v, l]) => (
              <Option key={v} value={v}>{l}</Option>
            ))}
          </Select>
        </FormControl>
        <FormControl sx={{ width: 140 }}>
          <FormLabel>Bookable up to</FormLabel>
          <Input type="number" value={draft.max_days_ahead} onChange={(e) => num("max_days_ahead", e.target.value)} endDecorator="days" />
        </FormControl>
        <FormControl sx={{ width: 140 }}>
          <FormLabel>Max per day</FormLabel>
          <Input type="number" value={draft.max_per_day} onChange={(e) => num("max_per_day", e.target.value)} endDecorator="0 = any" />
        </FormControl>
      </Box>
      <FormControl>
        <FormLabel>Location (optional)</FormLabel>
        <Input value={draft.location} onChange={(e) => set("location", e.target.value)} placeholder="Office, phone number, address" />
      </FormControl>
      <FormControl orientation="horizontal" sx={{ gap: 1 }}>
        <Switch checked={draft.create_meet} onChange={(e) => set("create_meet", e.target.checked)} />
        <FormLabel>Add a video call (Meet) to every booking</FormLabel>
      </FormControl>

      <Typography level="title-sm">Weekly hours</Typography>
      <FormControl sx={{ maxWidth: 320 }}>
        <FormLabel>Time zone of these hours</FormLabel>
        <Input value={draft.timezone} onChange={(e) => set("timezone", e.target.value)} placeholder="Asia/Kolkata" slotProps={{ input: { "data-testid": "bk-tz" } }} />
      </FormControl>
      {WEEKDAYS.map(({ key, label }) => {
        const ws = draft.availability[key] ?? [];
        const err = validateWindows(ws);
        const setWs = (n: typeof ws) => set("availability", { ...draft.availability, [key]: n });
        return (
          <Box key={key} sx={{ display: "flex", alignItems: "flex-start", gap: 1, flexWrap: "wrap" }}>
            <Checkbox
              label={label}
              sx={{ width: 120, pt: 0.75 }}
              checked={ws.length > 0}
              onChange={(e) => setWs(e.target.checked ? [{ start: "09:00", end: "17:00" }] : [])}
            />
            <Box sx={{ display: "flex", flexDirection: "column", gap: 0.5 }}>
              {ws.length === 0 && <Typography level="body-sm" sx={{ pt: 0.75 }}>Unavailable</Typography>}
              {ws.map((w, i) => (
                <Box key={i} sx={{ display: "flex", gap: 0.5, alignItems: "center" }}>
                  <Input size="sm" type="time" value={w.start} onChange={(e) => setWs(ws.map((x, j) => (j === i ? { ...x, start: e.target.value } : x)))} sx={{ width: 120 }} />
                  <span>–</span>
                  <Input size="sm" type="time" value={w.end} onChange={(e) => setWs(ws.map((x, j) => (j === i ? { ...x, end: e.target.value } : x)))} sx={{ width: 120 }} />
                  <IconButton size="sm" variant="plain" aria-label={`Remove ${label} window`} onClick={() => setWs(ws.filter((_, j) => j !== i))}>
                    <DeleteIcon fontSize="small" />
                  </IconButton>
                </Box>
              ))}
              {err && <Typography level="body-xs" color="danger">{err}</Typography>}
            </Box>
            {ws.length > 0 && (
              <IconButton size="sm" variant="plain" aria-label={`Add ${label} window`} onClick={() => setWs([...ws, { start: "13:00", end: "17:00" }])}>
                <AddIcon fontSize="small" />
              </IconButton>
            )}
          </Box>
        );
      })}

      <Typography level="title-sm">Questions for guests</Typography>
      {draft.questions.map((q, i) => (
        <Box key={q.id + i} sx={{ display: "flex", gap: 1, alignItems: "center", flexWrap: "wrap" }}>
          <Input size="sm" sx={{ flex: 1, minWidth: 180 }} value={q.label} placeholder="Question"
            onChange={(e) => set("questions", draft.questions.map((x, j) => (j === i ? { ...x, label: e.target.value } : x)))} />
          <Select size="sm" value={q.type} onChange={(_, v) => set("questions", draft.questions.map((x, j) => (j === i ? { ...x, type: (v ?? "text") as "text" | "longtext" } : x)))}>
            <Option value="text">Short answer</Option>
            <Option value="longtext">Long answer</Option>
          </Select>
          <Checkbox size="sm" label="Required" checked={q.required}
            onChange={(e) => set("questions", draft.questions.map((x, j) => (j === i ? { ...x, required: e.target.checked } : x)))} />
          <IconButton size="sm" variant="plain" aria-label="Remove question" onClick={() => set("questions", draft.questions.filter((_, j) => j !== i))}>
            <DeleteIcon fontSize="small" />
          </IconButton>
        </Box>
      ))}
      <Box>
        <Button size="sm" variant="outlined" color="neutral" startDecorator={<AddIcon />} disabled={draft.questions.length >= 10}
          onClick={() => set("questions", [...draft.questions, { id: `q${draft.questions.length + 1}`, label: "", type: "text", required: false }])}>
          Add question
        </Button>
      </Box>
      <FormControl orientation="horizontal" sx={{ gap: 1 }}>
        <Switch checked={draft.active} onChange={(e) => set("active", e.target.checked)} />
        <FormLabel>Accepting bookings</FormLabel>
      </FormControl>
      {error && <Alert color="danger">{error}</Alert>}
      <Box sx={{ display: "flex", gap: 1, justifyContent: "flex-end", position: "sticky", bottom: 0, bgcolor: "background.surface", py: 1 }}>
        <Button variant="plain" color="neutral" onClick={onCancel}>Cancel</Button>
        <Button loading={saving} onClick={onSave} data-testid="bk-save">Save schedule</Button>
      </Box>
    </Box>
  );
}

/** Manage appointment schedules (booking pages) and see who booked. */
export function BookingDialog({ onClose, onChanged }: { onClose: () => void; onChanged: () => void }) {
  const [pages, setPages] = useState<BookingPage[] | null>(null);
  const [appts, setAppts] = useState<api.Appointment[]>([]);
  const [base, setBase] = useState("");
  const [draft, setDraft] = useState<Draft | null>(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [copied, setCopied] = useState("");
  const [tab, setTab] = useState(0);

  const load = useCallback(async () => {
    try {
      const [p, a] = await Promise.all([api.listPages(), api.listAppointments()]);
      setPages(p.pages);
      setBase(p.base_url || window.location.origin);
      setAppts(a.appointments);
    } catch (e) {
      setError((e as Error).message);
      setPages([]);
    }
  }, []);
  useEffect(() => {
    void load();
  }, [load]);

  async function save() {
    if (!draft) return;
    for (const ws of Object.values(draft.availability)) {
      const m = validateWindows(ws);
      if (m) return setError(m);
    }
    setSaving(true);
    setError("");
    try {
      const body = { ...draft, availability: cleanAvailability(draft.availability) };
      if (draft.id) await api.updatePage(draft.id, body);
      else await api.createPage(body);
      setDraft(null);
      await load();
      onChanged();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setSaving(false);
    }
  }

  async function copy(slug: string) {
    const link = publicLink(window.location.origin, slug);
    try {
      await navigator.clipboard.writeText(link);
    } catch {
      window.prompt("Copy this link", link);
    }
    setCopied(slug);
    setTimeout(() => setCopied(""), 2000);
  }

  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: "min(780px, 96vw)", maxHeight: "92vh", overflow: "hidden", display: "flex", flexDirection: "column" }} data-testid="booking-dialog">
        <ModalClose />
        <Typography level="title-lg">Appointment schedules</Typography>
        {draft ? (
          <PageEditor draft={draft} setDraft={setDraft} onSave={save} onCancel={() => { setDraft(null); setError(""); }} saving={saving} error={error} />
        ) : (
          <Tabs value={tab} onChange={(_, v) => setTab(v as number)} sx={{ bgcolor: "transparent", minHeight: 0, flex: 1, overflow: "hidden" }}>
            <TabList>
              <Tab>Schedules</Tab>
              <Tab>Booked ({appts.filter((a) => a.status === "confirmed").length})</Tab>
            </TabList>
            <TabPanel value={0} sx={{ overflowY: "auto" }}>
              <Button startDecorator={<AddIcon />} onClick={() => { setError(""); setDraft(blankPage(detectTimeZone())); }} data-testid="bk-new">
                New schedule
              </Button>
              {error && <Alert color="danger" sx={{ mt: 1 }}>{error}</Alert>}
              {pages === null && <CircularProgress sx={{ mt: 2 }} />}
              {pages?.length === 0 && (
                <Typography level="body-sm" sx={{ mt: 2 }}>
                  No schedules yet. Create one to get a public link where people book time with you.
                </Typography>
              )}
              {pages?.map((p) => (
                <Box key={p.id} sx={{ border: "1px solid", borderColor: "divider", borderRadius: "md", p: 1.5, mt: 1.5 }} data-testid="bk-row">
                  <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
                    <Typography level="title-md" sx={{ flex: 1 }}>{p.title}</Typography>
                    {!p.active && <Chip size="sm" color="warning">Paused</Chip>}
                    <Chip size="sm" variant="soft">{p.duration_min} min</Chip>
                  </Box>
                  <Typography level="body-xs" sx={{ my: 0.5, wordBreak: "break-all" }}>{publicLink(base || window.location.origin, p.slug)}</Typography>
                  <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap" }}>
                    <Button size="sm" variant="outlined" color="neutral" startDecorator={<ContentCopyIcon />} onClick={() => copy(p.slug)}>
                      {copied === p.slug ? "Copied" : "Copy link"}
                    </Button>
                    <Button size="sm" variant="outlined" color="neutral" component="a" href={`/book/${p.slug}`} target="_blank" rel="noreferrer">Open page</Button>
                    <Button size="sm" variant="outlined" color="neutral" onClick={() => { setError(""); setDraft({ ...p }); }}>Edit</Button>
                    <Button size="sm" variant="plain" color="danger" onClick={async () => {
                      if (!window.confirm(`Delete "${p.title}"? Its link stops working. Events already in your calendar stay.`)) return;
                      try { await api.deletePage(p.id); await load(); } catch (e) { setError((e as Error).message); }
                    }}>Delete</Button>
                  </Box>
                </Box>
              ))}
            </TabPanel>
            <TabPanel value={1} sx={{ overflowY: "auto" }}>
              {appts.length === 0 && <Typography level="body-sm">Nobody has booked yet.</Typography>}
              {appts.map((a) => (
                <Box key={a.id} sx={{ display: "flex", alignItems: "center", gap: 1, py: 1, borderBottom: "1px solid", borderColor: "divider", flexWrap: "wrap" }} data-testid="bk-appt">
                  <Box sx={{ flex: 1, minWidth: 200 }}>
                    <Typography level="title-sm">{a.guest_name} · {a.page_title}</Typography>
                    <Typography level="body-xs">{fullWhen(a.start_at, Intl.DateTimeFormat().resolvedOptions().timeZone)} · {a.guest_email}</Typography>
                  </Box>
                  {a.status === "confirmed" ? (
                    <Button size="sm" variant="plain" color="danger" onClick={async () => {
                      if (!window.confirm(`Cancel the appointment with ${a.guest_name}? They get an email.`)) return;
                      try { await api.cancelAppointment(a.id); await load(); onChanged(); } catch (e) { setError((e as Error).message); }
                    }}>Cancel</Button>
                  ) : (
                    <Chip size="sm" color="neutral">Cancelled</Chip>
                  )}
                </Box>
              ))}
            </TabPanel>
          </Tabs>
        )}
      </ModalDialog>
    </Modal>
  );
}
