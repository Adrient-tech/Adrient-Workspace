import { useEffect, useState } from "react";
import { Button, Checkbox, FormControl, FormLabel, Input, Option, Select, Stack, Textarea, Typography } from "@mui/joy";
import * as api from "./api";
import * as api2 from "./api2";
import { createMeeting } from "../meet/api";
import { setEventMeet } from "../calendar/api";
import { Dialog, ErrorAlert, useErr } from "./ui";
import type { Data } from "./ui";

interface Template {
  id: string;
  name: string;
  subject: string;
  body: string;
}

/** wa.me link: digits only; a bare 10-digit number gets the India code (the product default region). */
export function whatsappHref(phone: string, defaultCode = "91"): string {
  let d = phone.replace(/\D/g, "").replace(/^0+/, "");
  if (d.length === 10) d = defaultCode + d;
  return d.length >= 8 ? `https://wa.me/${d}` : "";
}
export const telHref = (phone: string) => (phone.replace(/[^\d+]/g, "") ? `tel:${phone.replace(/[^\d+]/g, "")}` : "");

export function firstPhone(p: unknown): string {
  return Array.isArray(p) && typeof p[0] === "string" ? p[0] : "";
}

/** Send a mail to a CRM person through the Mail app, optionally from a template. No tracking pixel is added. */
export function SendMailDialog({ personId, dealId, email, onClose, onSent }: { personId: string; dealId?: string; email: string; onClose: () => void; onSent: () => void }) {
  const [templates, setTemplates] = useState<Template[]>([]);
  const [tid, setTid] = useState("");
  const [subject, setSubject] = useState("");
  const [body, setBody] = useState("");
  const { err, run } = useErr();
  useEffect(() => {
    void api2.list<Template>("templates", "templates").then(setTemplates).catch(() => undefined);
  }, []);
  const pick = (id: string) => {
    setTid(id);
    const t = templates.find((x) => x.id === id);
    if (t) {
      setSubject(t.subject);
      setBody(t.body);
    }
  };
  return (
    <Dialog title={`Email ${email}`} onClose={onClose} width={560}>
      <Stack gap={1.5}>
        <FormControl>
          <FormLabel>Template</FormLabel>
          <Select value={tid} onChange={(_, v) => pick(v ?? "")} placeholder="No template" slotProps={{ button: { "data-testid": "crm-mail-template" } as never }}>
            <Option value="">No template</Option>
            {templates.map((t) => (
              <Option key={t.id} value={t.id}>
                {t.name}
              </Option>
            ))}
          </Select>
          <Typography level="body-xs">Placeholders such as {"{{person.name}}"} and {"{{deal.title}}"} are filled in when sent.</Typography>
        </FormControl>
        <FormControl required>
          <FormLabel>Subject</FormLabel>
          <Input value={subject} onChange={(e) => setSubject(e.target.value)} slotProps={{ input: { "data-testid": "crm-mail-subject" } }} />
        </FormControl>
        <FormControl>
          <FormLabel>Message</FormLabel>
          <Textarea minRows={6} value={body} onChange={(e) => setBody(e.target.value)} slotProps={{ textarea: { "data-testid": "crm-mail-body" } }} />
        </FormControl>
        <ErrorAlert err={err} />
        <Button
          disabled={!subject.trim()}
          data-testid="crm-mail-send"
          onClick={async () => {
            if (await run(() => api2.post("mail/send", { person_id: personId, deal_id: dealId ?? "", template_id: tid, subject, body }))) onSent();
          }}
        >
          Send from my mailbox
        </Button>
      </Stack>
    </Dialog>
  );
}

/** Log a phone call (no telephony provider is connected for external numbers, so calls are logged by hand). */
export function LogCallDialog({ contactId, dealId, onClose, onSaved }: { contactId: string; dealId?: string; onClose: () => void; onSaved: () => void }) {
  const [direction, setDirection] = useState("out");
  const [minutes, setMinutes] = useState("5");
  const [summary, setSummary] = useState("");
  const { err, run } = useErr();
  return (
    <Dialog title="Log a call" onClose={onClose} width={460}>
      <Stack gap={1.5}>
        <Stack direction="row" gap={1}>
          <FormControl sx={{ flex: 1 }}>
            <FormLabel>Direction</FormLabel>
            <Select value={direction} onChange={(_, v) => setDirection(v ?? "out")} aria-label="Direction">
              <Option value="out">Outgoing</Option>
              <Option value="in">Incoming</Option>
            </Select>
          </FormControl>
          <FormControl sx={{ flex: 1 }}>
            <FormLabel>Minutes</FormLabel>
            <Input type="number" value={minutes} onChange={(e) => setMinutes(e.target.value)} slotProps={{ input: { min: 0, "data-testid": "crm-call-minutes" } }} />
          </FormControl>
        </Stack>
        <FormControl required>
          <FormLabel>What was discussed</FormLabel>
          <Textarea minRows={3} value={summary} onChange={(e) => setSummary(e.target.value)} slotProps={{ textarea: { "data-testid": "crm-call-summary" } }} />
        </FormControl>
        <ErrorAlert err={err} />
        <Button
          disabled={!summary.trim()}
          data-testid="crm-call-save"
          onClick={async () => {
            const r = await run(() =>
              api.saveActivity({ kind: "call", summary: summary.trim(), contact_id: contactId, deal_id: dealId ?? "", direction, duration_sec: Math.max(0, Number(minutes) || 0) * 60 } as never),
            );
            if (r) onSaved();
          }}
        >
          Save call
        </Button>
      </Stack>
    </Dialog>
  );
}

/** Schedule a meeting: a Calendar event, optionally with a Meet video room, logged on the person. */
export function ScheduleMeetingDialog({ data, contactId, dealId, onClose, onSaved }: { data: Data; contactId: string; dealId?: string; onClose: () => void; onSaved: () => void }) {
  const contact = data.contacts.find((c) => c.id === contactId);
  const tomorrow = new Date(Date.now() + 86400000).toISOString().slice(0, 10);
  const [title, setTitle] = useState(`Meeting with ${contact?.display_name ?? "contact"}`);
  const [date, setDate] = useState(tomorrow);
  const [time, setTime] = useState("10:00");
  const [mins, setMins] = useState("30");
  const [meet, setMeet] = useState(true);
  const { err, run } = useErr();
  return (
    <Dialog title="Schedule a meeting" onClose={onClose} width={480}>
      <Stack gap={1.5}>
        <FormControl required>
          <FormLabel>Title</FormLabel>
          <Input value={title} onChange={(e) => setTitle(e.target.value)} slotProps={{ input: { "data-testid": "crm-meet-title" } }} />
        </FormControl>
        <Stack direction="row" gap={1}>
          <FormControl sx={{ flex: 1 }}>
            <FormLabel>Date</FormLabel>
            <Input type="date" value={date} onChange={(e) => setDate(e.target.value)} slotProps={{ input: { "data-testid": "crm-meet-date" } }} />
          </FormControl>
          <FormControl sx={{ flex: 1 }}>
            <FormLabel>Time</FormLabel>
            <Input type="time" value={time} onChange={(e) => setTime(e.target.value)} />
          </FormControl>
          <FormControl sx={{ width: 90 }}>
            <FormLabel>Minutes</FormLabel>
            <Input type="number" value={mins} onChange={(e) => setMins(e.target.value)} slotProps={{ input: { min: 5 } }} />
          </FormControl>
        </Stack>
        <Typography level="body-sm">Invites: {contact?.emails?.[0] ?? "no email on file, the event will have no guest"}</Typography>
        <Checkbox label="Add a Meet video link" checked={meet} onChange={(e) => setMeet(e.target.checked)} data-testid="crm-meet-link" />
        <ErrorAlert err={err} />
        <Button
          data-testid="crm-meet-save"
          disabled={!title.trim() || !date}
          onClick={async () => {
            const ok = await run(async () => {
              const start = new Date(`${date}T${time || "09:00"}:00`);
              const end = new Date(start.getTime() + Math.max(5, Number(mins) || 30) * 60000);
              let room: { id: string; code?: string } | null = null;
              if (meet) room = await createMeeting(title.trim());
              const url = room?.code ? `${window.location.origin}/meet/${room.code}` : "";
              const evResp = await fetch("/api/v1/calendar/events", {
                method: "POST",
                credentials: "same-origin",
                headers: { "Content-Type": "application/json", Accept: "application/json" },
                body: JSON.stringify({
                  title: title.trim(), description: url ? `Join: ${url}` : "", location: url, start_at: start.toISOString(), end_at: end.toISOString(), all_day: false, color: "",
                  recurrence: "", attendees: contact?.emails?.[0] ? [contact.emails[0]] : [], item_type: "event", reminders: [10], status: "busy", visibility: "default", task_done: false,
                }),
              });
              if (!evResp.ok) throw new Error((await evResp.text().catch(() => "")) || "Calendar rejected the event");
              const ev = (await evResp.json()) as { id: string };
              if (room?.code) await setEventMeet(ev.id, { room_id: room.id, code: room.code });
              await api.saveActivity({ kind: "meeting", summary: `${title.trim()} on ${date} ${time}`, contact_id: contactId, deal_id: dealId ?? "", calendar_event_id: ev.id, meet_url: url } as never);
              return true;
            });
            if (ok) onSaved();
          }}
        >
          Schedule
        </Button>
      </Stack>
    </Dialog>
  );
}
