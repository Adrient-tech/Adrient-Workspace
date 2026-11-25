import { useMemo, useState } from "react";
import { Box, Button, ButtonGroup, Checkbox, Chip, FormControl, FormLabel, IconButton, Input, Option, Select, Sheet, Stack, Textarea, Typography } from "@mui/joy";
import * as api from "./api";
import { addDaysStr, groupFollowups, todayStr, weekDays, weeklyProgress } from "./logic";
import { buildCalendar, crmEvents } from "./ics";
import type { ActivityKind, Followup } from "./types";
import { ContactPicker, Dialog, Empty, ErrorAlert, companyName, contactName, fmtDate, useErr } from "./ui";
import type { Data } from "./ui";

export function FollowupForm({
  data,
  dealId = "",
  contactId = "",
  onClose,
  onSaved,
}: {
  data: Data;
  dealId?: string;
  contactId?: string;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [title, setTitle] = useState("");
  const [date, setDate] = useState(addDaysStr(todayStr(), 3));
  const [time, setTime] = useState("");
  const [deal, setDeal] = useState(dealId);
  const [contact, setContact] = useState(contactId);
  const [cal, setCal] = useState(true);
  const { err, run } = useErr();
  const save = async () => {
    const r = await run(async () => {
      const dn = data.deals.find((d) => d.id === deal)?.title;
      const eventId = cal ? await api.createCalendarEvent(`Follow-up: ${title.trim()}`, date, time, dn ? `CRM deal: ${dn}` : "CRM follow-up") : "";
      return api.saveFollowup("", { title: title.trim(), due_date: date, due_time: time, deal_id: deal, contact_id: contact, calendar_event_id: eventId });
    });
    if (r) onSaved();
  };
  return (
    <Dialog title="Schedule follow-up" onClose={onClose} width={460}>
      <Stack gap={1.5}>
        <FormControl required>
          <FormLabel>What needs doing</FormLabel>
          <Input autoFocus value={title} onChange={(e) => setTitle(e.target.value)} data-testid="crm-fu-title" />
        </FormControl>
        <Stack direction="row" gap={1}>
          <FormControl sx={{ flex: 1 }}>
            <FormLabel>Date</FormLabel>
            <Input type="date" value={date} onChange={(e) => setDate(e.target.value)} slotProps={{ input: { "data-testid": "crm-fu-date" } }} />
          </FormControl>
          <FormControl sx={{ flex: 1 }}>
            <FormLabel>Time (optional)</FormLabel>
            <Input type="time" value={time} onChange={(e) => setTime(e.target.value)} />
          </FormControl>
        </Stack>
        {!dealId && (
          <FormControl>
            <FormLabel>Deal</FormLabel>
            <Select value={deal} onChange={(_, v) => setDeal(v ?? "")} aria-label="Deal">
              <Option value="">None</Option>
              {data.deals.map((d) => (
                <Option key={d.id} value={d.id}>
                  {d.title}
                </Option>
              ))}
            </Select>
          </FormControl>
        )}
        <FormControl>
          <FormLabel>Contact</FormLabel>
          <ContactPicker contacts={data.contacts} value={contact} onChange={setContact} />
        </FormControl>
        <Checkbox label="Also add to my Calendar" checked={cal} onChange={(e) => setCal(e.target.checked)} />
        <ErrorAlert err={err} />
        <Stack direction="row" justifyContent="flex-end">
          <Button disabled={!title.trim() || !date} onClick={save} data-testid="crm-fu-save">
            Save
          </Button>
        </Stack>
      </Stack>
    </Dialog>
  );
}

export function ActivityForm({
  data,
  dealId = "",
  contactId = "",
  companyId = "",
  onClose,
  onSaved,
}: {
  data: Data;
  dealId?: string;
  contactId?: string;
  companyId?: string;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [kind, setKind] = useState<ActivityKind>("call");
  const [summary, setSummary] = useState("");
  const [when, setWhen] = useState(new Date(Date.now() - new Date().getTimezoneOffset() * 60000).toISOString().slice(0, 16));
  const [contact, setContact] = useState(contactId);
  const [deal, setDeal] = useState(dealId);
  const [thread, setThread] = useState("");
  const { err, run } = useErr();
  return (
    <Dialog title="Log activity" onClose={onClose} width={460}>
      <Stack gap={1.5}>
        <Stack direction="row" gap={1}>
          <FormControl sx={{ flex: 1 }}>
            <FormLabel>Type</FormLabel>
            <Select value={kind} onChange={(_, v) => setKind((v ?? "note") as ActivityKind)} aria-label="Type">
              {["call", "meeting", "email", "note"].map((k) => (
                <Option key={k} value={k}>
                  {k}
                </Option>
              ))}
            </Select>
          </FormControl>
          <FormControl sx={{ flex: 1 }}>
            <FormLabel>When</FormLabel>
            <Input type="datetime-local" value={when} onChange={(e) => setWhen(e.target.value)} />
          </FormControl>
        </Stack>
        <FormControl required>
          <FormLabel>What happened</FormLabel>
          <Textarea minRows={3} autoFocus value={summary} onChange={(e) => setSummary(e.target.value)} data-testid="crm-act-summary" />
        </FormControl>
        <FormControl>
          <FormLabel>Contact</FormLabel>
          <ContactPicker contacts={data.contacts} value={contact} onChange={setContact} />
        </FormControl>
        {!dealId && (
          <FormControl>
            <FormLabel>Deal</FormLabel>
            <Select value={deal} onChange={(_, v) => setDeal(v ?? "")} aria-label="Deal">
              <Option value="">None</Option>
              {data.deals.map((d) => (
                <Option key={d.id} value={d.id}>
                  {d.title}
                </Option>
              ))}
            </Select>
          </FormControl>
        )}
        {kind === "email" && (
          <FormControl>
            <FormLabel>Mail thread id (optional)</FormLabel>
            <Input value={thread} onChange={(e) => setThread(e.target.value)} />
          </FormControl>
        )}
        <ErrorAlert err={err} />
        <Stack direction="row" justifyContent="flex-end">
          <Button
            disabled={!summary.trim()}
            data-testid="crm-act-save"
            onClick={async () => {
              const r = await run(() =>
                api.saveActivity({ kind, summary: summary.trim(), occurred_at: new Date(when).toISOString(), contact_id: contact, deal_id: deal, company_id: companyId, mail_thread_id: thread }),
              );
              if (r) onSaved();
            }}
          >
            Save
          </Button>
        </Stack>
      </Stack>
    </Dialog>
  );
}

export function FollowUps({ data, onOpenDeal }: { data: Data; onOpenDeal: (id: string) => void }) {
  const [mode, setMode] = useState<"list" | "week">("list");
  const [adding, setAdding] = useState(false);
  const [anchor, setAnchor] = useState(todayStr());
  const { err, run } = useErr();
  const g = useMemo(() => groupFollowups(data.followups), [data.followups]);
  const today = todayStr();

  const toggle = async (f: Followup) => {
    await run(() => api.saveFollowup(f.id, { ...f, deal_id: f.deal_id ?? "", contact_id: f.contact_id ?? "", done: !f.done }));
    data.reload();
  };
  const resched = async (f: Followup, date: string) => {
    await run(() => api.saveFollowup(f.id, { ...f, deal_id: f.deal_id ?? "", contact_id: f.contact_id ?? "", due_date: date }));
    data.reload();
  };
  const remove = async (f: Followup) => {
    await run(() => api.deleteFollowup(f.id));
    if (f.calendar_event_id) void api.deleteCalendarEvent(f.calendar_event_id);
    data.reload();
  };
  const row = (f: Followup, tone?: "danger" | "primary") => (
    <Sheet key={f.id} variant="outlined" draggable onDragStart={(e) => e.dataTransfer.setData("text/plain", f.id)} data-testid="crm-followup" sx={{ p: 1, borderRadius: "sm", display: "flex", alignItems: "center", gap: 1 }}>
      <Checkbox checked={f.done} onChange={() => toggle(f)} slotProps={{ input: { "aria-label": `Done: ${f.title}` } }} />
      <Box sx={{ flex: 1, minWidth: 0 }}>
        <Typography level="title-sm" sx={{ textDecoration: f.done ? "line-through" : undefined }}>
          {f.title}
        </Typography>
        <Typography level="body-xs">
          {fmtDate(f.due_date)} {f.due_time}
          {f.deal_id ? ` · ${data.deals.find((d) => d.id === f.deal_id)?.title ?? ""}` : ""}
          {f.contact_id ? ` · ${contactName(data, f.contact_id)}` : ""}
        </Typography>
      </Box>
      {tone && (
        <Chip size="sm" color={tone} variant="soft">
          {tone === "danger" ? "overdue" : "today"}
        </Chip>
      )}
      {f.calendar_event_id && (
        <Chip size="sm" variant="outlined">
          Calendar
        </Chip>
      )}
      {f.deal_id && (
        <Button size="sm" variant="plain" onClick={() => onOpenDeal(f.deal_id!)}>
          Deal
        </Button>
      )}
      <IconButton size="sm" aria-label={`Delete ${f.title}`} onClick={() => remove(f)}>
        ×
      </IconButton>
    </Sheet>
  );

  const days = weekDays(anchor);
  const openDue = data.followups.filter((f) => !f.done);

  return (
    <Box>
      <Stack direction="row" gap={1} sx={{ mb: 1.5 }}>
        <Typography level="h4" sx={{ flex: 1 }}>
          Follow-ups
        </Typography>
        <ButtonGroup size="sm">
          <Button variant={mode === "list" ? "solid" : "outlined"} onClick={() => setMode("list")}>
            List
          </Button>
          <Button variant={mode === "week" ? "solid" : "outlined"} onClick={() => setMode("week")} data-testid="crm-fu-week">
            Week
          </Button>
        </ButtonGroup>
        <Button
          size="sm"
          variant="outlined"
          data-testid="crm-fu-ics"
          onClick={() => {
            const dealById = new Map(data.deals.map((d) => [d.id, d.title]));
            const ics = buildCalendar(crmEvents(data.followups, data.deals, { contact: (id) => contactName(data, id), deal: (id) => dealById.get(id) ?? "", company: (id) => companyName(data, id) }));
            const url = URL.createObjectURL(new Blob([ics], { type: "text/calendar" }));
            const a = document.createElement("a");
            a.href = url;
            a.download = "adrient-crm-followups.ics";
            a.click();
            URL.revokeObjectURL(url);
          }}
        >
          Export .ics
        </Button>
        <Button onClick={() => setAdding(true)} data-testid="crm-new-followup">
          New follow-up
        </Button>
      </Stack>
      <ErrorAlert err={err} />
      {mode === "list" && (
        <Stack gap={1}>
          {data.followups.length === 0 && <Empty>No follow-ups scheduled.</Empty>}
          {g.overdue.map((f) => row(f, "danger"))}
          {g.today.map((f) => row(f, "primary"))}
          {g.upcoming.map((f) => row(f))}
          {g.done.length > 0 && <Typography level="title-sm" sx={{ mt: 1 }}>Done</Typography>}
          {g.done.slice(0, 20).map((f) => row(f))}
        </Stack>
      )}
      {mode === "week" && (
        <Box>
          <Stack direction="row" gap={1} sx={{ mb: 1 }} alignItems="center">
            <Button size="sm" variant="outlined" onClick={() => setAnchor(addDaysStr(anchor, -7))}>
              Previous
            </Button>
            <Button size="sm" variant="outlined" onClick={() => setAnchor(todayStr())}>
              This week
            </Button>
            <Button size="sm" variant="outlined" onClick={() => setAnchor(addDaysStr(anchor, 7))}>
              Next
            </Button>
            <Typography level="body-sm">Drag a follow-up onto another day to reschedule it.</Typography>
          </Stack>
          <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "repeat(7, 1fr)" }, gap: 1 }}>
            {days.map((day) => {
              const items = openDue.filter((f) => f.due_date.slice(0, 10) === day);
              const closes = data.deals.filter((d) => d.expected_close === day && d.stage !== "won" && d.stage !== "lost");
              return (
                <Sheet
                  key={day}
                  variant={day === today ? "soft" : "outlined"}
                  color={day === today ? "primary" : "neutral"}
                  data-testid={`crm-day-${day}`}
                  onDragOver={(e) => e.preventDefault()}
                  onDrop={(e) => {
                    const f = data.followups.find((x) => x.id === e.dataTransfer.getData("text/plain"));
                    if (f && f.due_date.slice(0, 10) !== day) void resched(f, day);
                  }}
                  sx={{ p: 1, minHeight: 120, borderRadius: "sm" }}
                >
                  <Typography level="title-sm">
                    {new Date(`${day}T12:00:00`).toLocaleDateString(undefined, { weekday: "short", day: "numeric" })} {items.length > 0 && <Chip size="sm">{items.length}</Chip>}
                  </Typography>
                  <Stack gap={0.5} sx={{ mt: 0.5 }}>
                    {items.map((f) => (
                      <Typography key={f.id} level="body-xs" draggable onDragStart={(e) => e.dataTransfer.setData("text/plain", f.id)} sx={{ cursor: "grab", color: day < today ? "danger.plainColor" : undefined }}>
                        {f.due_time} {f.title}
                      </Typography>
                    ))}
                    {closes.map((d) => (
                      <Typography key={d.id} level="body-xs" sx={{ cursor: "pointer", fontStyle: "italic" }} onClick={() => onOpenDeal(d.id)}>
                        Close: {d.title}
                      </Typography>
                    ))}
                  </Stack>
                </Sheet>
              );
            })}
          </Box>
        </Box>
      )}
      {adding && (
        <FollowupForm
          data={data}
          onClose={() => setAdding(false)}
          onSaved={() => {
            setAdding(false);
            data.reload();
          }}
        />
      )}
    </Box>
  );
}

export function ActivityLog({ data }: { data: Data }) {
  const [kind, setKind] = useState("");
  const [q, setQ] = useState("");
  const [adding, setAdding] = useState(false);
  const { err, run } = useErr();
  const list = data.activities.filter(
    (a) => (!kind || a.kind === kind) && (!q || `${a.summary} ${contactName(data, a.contact_id)} ${companyName(data, a.company_id)}`.toLowerCase().includes(q.toLowerCase())),
  );
  const progress = weeklyProgress(data.activities, data.settings.weekly_target);
  return (
    <Box>
      <Stack direction="row" gap={1} flexWrap="wrap" alignItems="center" sx={{ mb: 1.5 }}>
        <Typography level="h4" sx={{ flex: 1 }}>
          Activity log
        </Typography>
        <Input size="sm" placeholder="Search" value={q} onChange={(e) => setQ(e.target.value)} slotProps={{ input: { "aria-label": "Search activity" } }} />
        <Select size="sm" value={kind} onChange={(_, v) => setKind(v ?? "")} sx={{ width: 130 }} aria-label="Type filter">
          <Option value="">All types</Option>
          {["call", "meeting", "email", "note"].map((k) => (
            <Option key={k} value={k}>
              {k}
            </Option>
          ))}
        </Select>
        <Button onClick={() => setAdding(true)} data-testid="crm-log-activity">
          Log activity
        </Button>
      </Stack>
      {progress.target > 0 && (
        <Typography level="body-sm" sx={{ mb: 1 }}>
          This week: {progress.done} of {progress.target} touches ({progress.pct}%)
        </Typography>
      )}
      <ErrorAlert err={err} />
      {list.length === 0 && <Empty>No activity logged yet.</Empty>}
      <Stack gap={1}>
        {list.map((a) => (
          <Sheet key={a.id} variant="outlined" data-testid="crm-activity" sx={{ p: 1, borderRadius: "sm", display: "flex", gap: 1, alignItems: "flex-start" }}>
            <Chip size="sm" variant="soft">
              {a.kind}
            </Chip>
            <Box sx={{ flex: 1 }}>
              <Typography level="body-sm">{a.summary}</Typography>
              <Typography level="body-xs">
                {fmtDate(a.occurred_at)}
                {a.contact_id ? ` · ${contactName(data, a.contact_id)}` : ""}
                {a.deal_id ? ` · ${data.deals.find((d) => d.id === a.deal_id)?.title ?? ""}` : ""}
              </Typography>
            </Box>
            <IconButton
              size="sm"
              aria-label="Delete activity"
              onClick={async () => {
                await run(() => api.deleteActivity(a.id));
                data.reload();
              }}
            >
              ×
            </IconButton>
          </Sheet>
        ))}
      </Stack>
      {adding && (
        <ActivityForm
          data={data}
          onClose={() => setAdding(false)}
          onSaved={() => {
            setAdding(false);
            data.reload();
          }}
        />
      )}
    </Box>
  );
}
