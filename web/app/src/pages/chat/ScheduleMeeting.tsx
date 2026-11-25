/**
 * Schedule a meeting from Chat: creates a Meet room, a Calendar event with the
 * Meet link attached, notifies the invitees and posts a meeting card (the
 * message link renders as a "Join meeting" card) in the chosen conversation.
 */
import { useMemo, useState } from "react";
import {
  Alert,
  Box,
  Button,
  FormControl,
  FormLabel,
  Input,
  Modal,
  ModalClose,
  ModalDialog,
  Option,
  Select,
  Typography,
} from "@mui/joy";
import { createMeeting, meetingLink } from "../meet/api";
import { createEvent, setEventMeet } from "../calendar/api";
import { PeoplePicker } from "./Dialogs";
import { channelLabel } from "./lib";
import { teams } from "./teamsApi";
import type { TeamsState } from "./useTeams";
import type { TChannel } from "./types";

const REPEAT: Record<string, string> = { none: "", daily: "FREQ=DAILY", weekly: "FREQ=WEEKLY", monthly: "FREQ=MONTHLY" };

function pad(n: number) {
  return String(n).padStart(2, "0");
}

function defaultStart(): { date: string; time: string } {
  const d = new Date(Date.now() + 30 * 60_000);
  d.setMinutes(d.getMinutes() < 30 ? 30 : 60, 0, 0);
  return { date: `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`, time: `${pad(d.getHours())}:${pad(d.getMinutes())}` };
}

export function ScheduleMeetingDialog({
  state,
  channel,
  onClose,
  onDone,
}: {
  state: TeamsState;
  /** Conversation to post the meeting card in (and default invitees). */
  channel?: TChannel;
  onClose: () => void;
  onDone: (msg: string) => void;
}) {
  const { me, people, peopleMap, channels } = state;
  const init = useMemo(defaultStart, []);
  const [title, setTitle] = useState("");
  const [date, setDate] = useState(init.date);
  const [time, setTime] = useState(init.time);
  const [minutes, setMinutes] = useState(30);
  const [repeat, setRepeat] = useState("none");
  const [invitees, setInvitees] = useState<string[]>(() => (channel ? channel.member_ids.filter((id) => id !== me).slice(0, 49) : []));
  const [postIn, setPostIn] = useState(channel?.id ?? "");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  const postable = channels.filter((c) => c.is_member && !c.archived);

  const submit = async () => {
    const start = new Date(`${date}T${time}`);
    if (!title.trim()) return setErr("Give the meeting a title.");
    if (Number.isNaN(start.getTime())) return setErr("Pick a valid date and time.");
    setBusy(true);
    setErr("");
    try {
      const end = new Date(start.getTime() + minutes * 60_000);
      const room = await createMeeting(title.trim());
      const code = room.code ?? "";
      if (!code) throw new Error("meeting has no code");
      const link = meetingLink(code);
      const emails = invitees.map((id) => peopleMap[id]?.email).filter((e): e is string => !!e);
      const ev = await createEvent({
        title: title.trim(),
        description: `Join the meeting: ${link}`,
        location: link,
        start_at: start.toISOString(),
        end_at: end.toISOString(),
        all_day: false,
        color: "",
        recurrence: REPEAT[repeat] ?? "",
        attendees: emails,
        item_type: "event",
        reminders: [10],
        status: "busy",
        visibility: "default",
        task_done: false,
      });
      await setEventMeet(ev.id, { room_id: room.id, code });
      const when = `${start.toLocaleDateString([], { weekday: "short", month: "short", day: "numeric" })}, ${start.toLocaleTimeString([], { hour: "numeric", minute: "2-digit" })} – ${end.toLocaleTimeString([], { hour: "numeric", minute: "2-digit" })}`;
      if (invitees.length) {
        await fetch("/api/v1/chat/x/calls/invite", {
          method: "POST",
          credentials: "same-origin",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ user_ids: invitees, code, title: title.trim(), when }),
        });
      }
      if (postIn) {
        await teams.post(postIn, `📅 **${title.trim()}**\n${when}${repeat !== "none" ? ` (repeats ${repeat})` : ""}\n\n${link}`);
      }
      onDone(`Meeting scheduled: ${title.trim()}`);
      onClose();
    } catch (e) {
      setErr((e as Error).message);
      setBusy(false);
    }
  };

  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 520, maxWidth: "95vw", maxHeight: "92vh", overflow: "auto" }} data-testid="schedule-dialog">
        <ModalClose />
        <Typography level="title-lg">Schedule a meeting</Typography>
        <FormControl>
          <FormLabel>Title</FormLabel>
          <Input autoFocus value={title} onChange={(e) => setTitle(e.target.value)} slotProps={{ input: { "aria-label": "Meeting title", maxLength: 120 } }} />
        </FormControl>
        <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap" }}>
          <FormControl sx={{ flex: 1, minWidth: 140 }}>
            <FormLabel>Date</FormLabel>
            <Input type="date" value={date} onChange={(e) => setDate(e.target.value)} slotProps={{ input: { "aria-label": "Meeting date" } }} />
          </FormControl>
          <FormControl sx={{ width: 120 }}>
            <FormLabel>Start</FormLabel>
            <Input type="time" value={time} onChange={(e) => setTime(e.target.value)} slotProps={{ input: { "aria-label": "Start time" } }} />
          </FormControl>
          <FormControl sx={{ width: 130 }}>
            <FormLabel>Duration</FormLabel>
            <Select value={minutes} onChange={(_e, v) => v && setMinutes(v)} slotProps={{ button: { "aria-label": "Duration" } as never }}>
              {[15, 30, 45, 60, 90, 120].map((m) => (
                <Option key={m} value={m}>
                  {m} min
                </Option>
              ))}
            </Select>
          </FormControl>
        </Box>
        <FormControl>
          <FormLabel>Repeat</FormLabel>
          <Select value={repeat} onChange={(_e, v) => v && setRepeat(v)} slotProps={{ button: { "aria-label": "Repeat" } as never }}>
            <Option value="none">Does not repeat</Option>
            <Option value="daily">Daily</Option>
            <Option value="weekly">Weekly</Option>
            <Option value="monthly">Monthly</Option>
          </Select>
        </FormControl>
        <FormControl>
          <FormLabel>Invite people</FormLabel>
          <PeoplePicker people={people} me={me} selected={invitees} onChange={setInvitees} max={49} />
        </FormControl>
        <FormControl>
          <FormLabel>Post the meeting card in</FormLabel>
          <Select
            value={postIn}
            onChange={(_e, v) => setPostIn(v ?? "")}
            slotProps={{ button: { "aria-label": "Post in" } as never }}
          >
            <Option value="">Nowhere (calendar only)</Option>
            {postable.map((c) => (
              <Option key={c.id} value={c.id}>
                {channelLabel(c, me, peopleMap)}
              </Option>
            ))}
          </Select>
        </FormControl>
        {err && (
          <Alert color="danger" size="sm">
            {err}
          </Alert>
        )}
        <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1 }}>
          <Button variant="plain" color="neutral" onClick={onClose}>
            Cancel
          </Button>
          <Button loading={busy} onClick={() => void submit()} data-testid="schedule-save">
            Schedule
          </Button>
        </Box>
      </ModalDialog>
    </Modal>
  );
}
