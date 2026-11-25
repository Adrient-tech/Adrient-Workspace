import { PersonSelect } from "../../components/PeoplePicker";
import { useEffect, useState } from "react";
import { Alert, Box, Button, FormControl, FormLabel, Input, Modal, ModalClose, ModalDialog, Option, Select, Textarea, Typography } from "@mui/joy";
import { describeRule } from "./quickparse";
import { dayFromDue, dueFromDay, emptyExtras, PRIORITIES, RECURRENCES, REMINDERS, type TaskExtras } from "./logic";
import { directory, type Member } from "./extrasApi";
import type { Task } from "./types";

export interface EditorResult {
  title: string;
  notes: string;
  due_at: string;
  extras: TaskExtras;
}

/** Full task editor: title, notes, due date, priority, repeat, reminder, assignee, subtasks. */
export function TaskEditor({ task, extras, subtasks, readOnly, onSave, onAddSubtask, onClose }: {
  task: Task;
  extras?: TaskExtras;
  subtasks: Task[];
  readOnly: boolean;
  onSave: (r: EditorResult) => Promise<void>;
  onAddSubtask: (title: string) => Promise<void>;
  onClose: () => void;
}) {
  const [title, setTitle] = useState(task.title);
  const [notes, setNotes] = useState(task.notes);
  const [day, setDay] = useState(dayFromDue(task.due_at));
  const [x, setX] = useState<TaskExtras>(extras ?? emptyExtras(task.id));
  const [members, setMembers] = useState<Member[]>([]);
  const [sub, setSub] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const isSub = !!task.parent_task_id;

  useEffect(() => {
    directory("").then(setMembers).catch(() => undefined);
  }, []);

  async function save() {
    if (!title.trim()) return setErr("A title is required.");
    if (x.remind_minutes !== null && !day) return setErr("Pick a due date to use a reminder.");
    setBusy(true);
    setErr("");
    try {
      await onSave({ title: title.trim(), notes, due_at: dueFromDay(day), extras: x });
      onClose();
    } catch (e) {
      setErr((e as Error).message);
      setBusy(false);
    }
  }

  async function addSub() {
    const t = sub.trim();
    if (!t) return;
    setSub("");
    try {
      await onAddSubtask(t);
    } catch (e) {
      setErr((e as Error).message);
    }
  }

  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: { xs: "100vw", sm: 520 }, maxWidth: "100vw", maxHeight: "100dvh", overflowY: "auto" }} data-testid="task-editor">
        <ModalClose />
        <Typography level="title-md">{readOnly ? "Task" : "Edit task"}</Typography>
        <Input autoFocus placeholder="Title" value={title} disabled={readOnly} onChange={(e) => setTitle(e.target.value)} slotProps={{ input: { "data-testid": "te-title" } }} />
        <Textarea placeholder="Notes" minRows={3} value={notes} disabled={readOnly} onChange={(e) => setNotes(e.target.value)} />
        <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap" }}>
          <FormControl sx={{ flex: 1, minWidth: 150 }}>
            <FormLabel>Due date</FormLabel>
            <Input type="date" value={day} disabled={readOnly} onChange={(e) => setDay(e.target.value)} slotProps={{ input: { "data-testid": "te-due" } }} />
          </FormControl>
          <FormControl sx={{ flex: 1, minWidth: 150 }}>
            <FormLabel>Priority</FormLabel>
            <Select value={x.priority} disabled={readOnly} onChange={(_, v) => setX({ ...x, priority: (v ?? 0) as number })}>
              {PRIORITIES.map((p, i) => (
                <Option key={p} value={i}>{p}</Option>
              ))}
            </Select>
          </FormControl>
        </Box>
        {!isSub && (
          <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap" }}>
            <FormControl sx={{ flex: 1, minWidth: 150 }}>
              <FormLabel>Repeat</FormLabel>
              <Select value={x.recurrence} disabled={readOnly} onChange={(_, v) => setX({ ...x, recurrence: (v ?? "") as string })} slotProps={{ button: { "data-testid": "te-repeat" } as never }}>
                {RECURRENCES.map((r) => (
                  <Option key={r.value} value={r.value}>{r.label}</Option>
                ))}
                {x.recurrence && !RECURRENCES.some((r) => r.value === x.recurrence) && (
                  <Option value={x.recurrence}>{describeRule(x.recurrence) || x.recurrence}</Option>
                )}
              </Select>
            </FormControl>
            <FormControl sx={{ flex: 1, minWidth: 150 }}>
              <FormLabel>Reminder</FormLabel>
              <Select value={x.remind_minutes} disabled={readOnly} onChange={(_, v) => setX({ ...x, remind_minutes: (v ?? null) as number | null })}>
                {REMINDERS.map((r) => (
                  <Option key={String(r.value)} value={r.value}>{r.label}</Option>
                ))}
              </Select>
            </FormControl>
          </Box>
        )}
        {!isSub && (
          <FormControl>
            <FormLabel>Tags (up to 5, comma separated)</FormLabel>
            <Input
              defaultValue={(x.tags ?? []).join(", ")}
              disabled={readOnly}
              placeholder="home, errands"
              onBlur={(e) =>
                setX({ ...x, tags: e.target.value.split(",").map((t) => t.trim().toLowerCase().replace(/^#/, "").replace(/\s+/g, "-")).filter(Boolean).slice(0, 5) })
              }
              slotProps={{ input: { "data-testid": "te-tags" } }}
            />
          </FormControl>
        )}
        {!isSub && (
          <FormControl>
            <FormLabel>Assigned to</FormLabel>
            <PersonSelect value={x.assignee_id} disabled={readOnly} known={members} onChange={(id) => setX({ ...x, assignee_id: (id) as string })} placeholder="Nobody" ariaLabel="Person" testId={"te-assignee"} />
            {x.assignee_id && <Typography level="body-xs" sx={{ mt: 0.5 }}>The list is shared with them so they can open this task.</Typography>}
          </FormControl>
        )}
        {x.link_url && (
          <Typography level="body-sm">
            Linked: <a href={x.link_url}>{x.link_url}</a>
          </Typography>
        )}
        {!isSub && (
          <Box>
            <Typography level="title-sm" sx={{ mb: 0.5 }}>Subtasks</Typography>
            {subtasks.map((s) => (
              <Typography key={s.id} level="body-sm" sx={{ textDecoration: s.completed ? "line-through" : "none", opacity: s.completed ? 0.6 : 1 }}>
                • {s.title}
              </Typography>
            ))}
            {!readOnly && (
              <Input
                size="sm"
                placeholder="Add a subtask and press Enter"
                value={sub}
                onChange={(e) => setSub(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    e.preventDefault();
                    void addSub();
                  }
                }}
                sx={{ mt: 0.5 }}
                slotProps={{ input: { "data-testid": "te-subtask" } }}
              />
            )}
          </Box>
        )}
        {err && <Alert color="danger">{err}</Alert>}
        <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1 }}>
          <Button variant="plain" color="neutral" onClick={onClose}>{readOnly ? "Close" : "Cancel"}</Button>
          {!readOnly && <Button loading={busy} onClick={save} data-testid="te-save">Save</Button>}
        </Box>
      </ModalDialog>
    </Modal>
  );
}
