import { forwardRef, useCallback, useEffect, useImperativeHandle, useRef, useState } from "react";
import { Alert, Box, Button, Chip, Dropdown, Input, ListDivider, Menu, MenuButton, MenuItem, Tab, TabList, Tabs, Typography } from "@mui/joy";
import AttachFileIcon from "@mui/icons-material/AttachFile";
import ScheduleSendIcon from "@mui/icons-material/ScheduleSend";
import AutoAwesomeIcon from "@mui/icons-material/AutoAwesome";
import ArrowDropDownIcon from "@mui/icons-material/ArrowDropDown";
import { addComment, patchTicket } from "./api";
import { CannedPicker } from "./Helpdesk";
import { DrivePicker } from "../drive/DrivePicker";
import { createShare, downloadURL } from "../drive/api";
import { ReplyEditor, type ReplyEditorHandle } from "./ReplyEditor";
import { fillTemplate } from "./workspaceLogic";
import { svc, type Row, type StatusDef, type Presence, type Scheduled, type AiResult } from "./svcApi";

const errText = (e: unknown) => (e instanceof Error ? e.message : String(e));

export interface ComposerHandle {
  focus: (mode: "reply" | "note") => void;
}

interface Props {
  row: Row;
  statuses: StatusDef[];
  projectId: string;
  meName: string;
  others: Presence[];
  onTyping: (typing: boolean) => void;
  onSent: () => Promise<void> | void;
  onMacro: (id: string) => void;
  macroPicker: React.ReactNode;
  onClassification: (r: AiResult) => void;
}

interface Picked {
  id: string;
  name: string;
}

/** Reply and note composer: rich editor, Drive attachments, schedule send, AI help and saved drafts. */
export const Composer = forwardRef<ComposerHandle, Props>(function Composer({ row, statuses, projectId, meName, others, onTyping, onSent, macroPicker, onClassification }, ref) {
  const [mode, setMode] = useState<"reply" | "note">("reply");
  const [draft, setDraft] = useState("");
  const [saved, setSaved] = useState("");
  const [sending, setSending] = useState(false);
  const [err, setErr] = useState("");
  const [files, setFiles] = useState<Picked[]>([]);
  const [picker, setPicker] = useState(false);
  const [suggest, setSuggest] = useState<{ id: string; title: string; body: string }[]>([]);
  const [scheduled, setScheduled] = useState<Scheduled[]>([]);
  const [schedAt, setSchedAt] = useState("");
  const [schedOpen, setSchedOpen] = useState(false);
  const [aiBusy, setAiBusy] = useState("");
  const [aiNote, setAiNote] = useState<AiResult | null>(null);
  const editor = useRef<ReplyEditorHandle | null>(null);
  const live = useRef(row.id);

  useImperativeHandle(ref, () => ({ focus: (m) => { setMode(m); setTimeout(() => editor.current?.focus(), 30); } }), []);

  useEffect(() => {
    live.current = row.id;
    setDraft("");
    setSaved("");
    setMode("reply");
    setFiles([]);
    setErr("");
    setAiNote(null);
    svc.draft(row.id).then((d) => {
      if (live.current === row.id && d.body) {
        setDraft(d.body);
        setSaved(d.body);
        setMode(d.internal ? "note" : "reply");
      }
    }).catch(() => undefined);
    svc.scheduled(row.id).then((s) => live.current === row.id && setScheduled(s)).catch(() => setScheduled([]));
  }, [row.id]);

  useEffect(() => { onTyping(draft.trim() !== ""); }, [draft.trim() === ""]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (draft === saved) return;
    const t = setTimeout(() => {
      svc.saveDraft(row.id, { body: draft, internal: mode === "note" }).then(() => setSaved(draft)).catch(() => undefined);
    }, 1200);
    return () => clearTimeout(t);
  }, [draft, saved, row.id, mode]);

  useEffect(() => {
    if (mode !== "reply" || draft.trim().length < 12) return setSuggest([]);
    const t = setTimeout(() => { svc.suggest(projectId, draft.slice(-200)).then((a) => setSuggest(a.slice(0, 3))).catch(() => setSuggest([])); }, 700);
    return () => clearTimeout(t);
  }, [draft, mode, projectId]);

  /** Public replies get a 30 day view link per file (the requester cannot open Drive); notes link inside the workspace. */
  const withFileLinks = useCallback(async (body: string): Promise<string> => {
    if (!files.length) return body;
    const lines: string[] = [];
    for (const f of files) {
      if (mode === "reply") {
        const sh = await createShare(f.id, "viewer", Math.floor(Date.now() / 1000) + 30 * 86400);
        lines.push(`- [${f.name.replace(/[\]\n]/g, " ")}](${window.location.origin}/drive/share/${sh.token})`);
      } else {
        lines.push(`- [${f.name.replace(/[\]\n]/g, " ")}](${downloadURL(f.id)})`);
      }
    }
    return `${body}\n\nAttachments:\n${lines.join("\n")}`;
  }, [files, mode]);

  async function recordFiles() {
    for (const f of files) await svc.attach(row.id, f.id).catch(() => undefined);
  }

  async function send(solve: boolean) {
    if (!draft.trim()) return;
    setSending(true);
    setErr("");
    try {
      const body = await withFileLinks(draft.trim());
      await addComment(row.id, { body, is_internal: mode === "note" });
      await recordFiles();
      if (solve && mode === "reply") {
        const k = statuses.find((s) => s.category === "solved");
        if (k) await patchTicket(row.id, { status: k.key });
      }
      editor.current?.clear();
      setDraft("");
      setSaved("");
      setFiles([]);
      await svc.saveDraft(row.id, { body: "", internal: false }).catch(() => undefined);
      await onSent();
    } catch (e) {
      setErr(errText(e));
    } finally {
      setSending(false);
    }
  }

  async function schedule(at: Date, solve: boolean) {
    if (!draft.trim()) return;
    setSending(true);
    setErr("");
    try {
      const body = await withFileLinks(draft.trim());
      await svc.schedule(row.id, { body, internal: mode === "note", solve, send_at: at.toISOString() });
      await recordFiles();
      editor.current?.clear();
      setDraft("");
      setSaved("");
      setFiles([]);
      setSchedOpen(false);
      await svc.saveDraft(row.id, { body: "", internal: false }).catch(() => undefined);
      setScheduled(await svc.scheduled(row.id));
      await onSent();
    } catch (e) {
      setErr(errText(e));
    } finally {
      setSending(false);
    }
  }

  async function runAi(kind: "suggest_reply" | "summary" | "sentiment" | "classify") {
    setAiBusy(kind);
    setErr("");
    setAiNote(null);
    try {
      const r = await svc.ai(row.id, kind);
      if (kind === "suggest_reply" && r.text) {
        setMode("reply");
        setDraft(r.text);
        setAiNote({ ...r, text: undefined });
      } else if (kind === "classify") {
        onClassification(r);
      } else {
        setAiNote(r);
        if (kind !== "suggest_reply") await onSent();
      }
    } catch (e) {
      setErr(errText(e));
    } finally {
      setAiBusy("");
    }
  }

  const replying = others.filter((o) => o.mode === "replying");
  const firstName = (row.requester_name || "there").split(" ")[0];
  const tomorrow9 = () => { const d = new Date(); d.setDate(d.getDate() + 1); d.setHours(9, 0, 0, 0); return d; };
  const inHours = (h: number) => new Date(Date.now() + h * 3600 * 1000);
  const canSolve = mode === "reply" && statuses.some((s) => s.category === "solved");

  return (
    <Box sx={{ borderTop: "1px solid", borderColor: "divider", p: 1.5, bgcolor: "background.surface" }}>
      {scheduled.length > 0 && (
        <Box sx={{ mb: 1, display: "flex", flexDirection: "column", gap: 0.5 }} data-testid="scheduled-list">
          {scheduled.map((s) => (
            <Alert key={s.id} size="sm" variant="soft" color={s.state === "failed" ? "danger" : "neutral"} startDecorator={<ScheduleSendIcon fontSize="small" />}
              endDecorator={<Button size="sm" variant="plain" onClick={async () => { await svc.cancelScheduled(row.id, s.id).catch((e) => setErr(errText(e))); setScheduled(await svc.scheduled(row.id)); }}>Cancel</Button>}>
              <Box sx={{ minWidth: 0 }}>
                <b>{s.internal ? "Note" : "Reply"} scheduled for {new Date(s.send_at).toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" })}</b> by {s.user_name}
                {s.state === "failed" && <> · could not be sent: {s.error}</>}
                <Typography level="body-xs" noWrap>{s.body.slice(0, 120)}</Typography>
              </Box>
            </Alert>
          ))}
        </Box>
      )}
      {replying.length > 0 && (
        <Alert color="warning" size="sm" variant="soft" sx={{ mb: 1 }} data-testid="collision">
          {replying.map((o) => o.name || "A teammate").join(", ")} {replying.length === 1 ? "is" : "are"} replying to this ticket right now.
        </Alert>
      )}
      {others.length > 0 && replying.length === 0 && (
        <Typography level="body-xs" sx={{ mb: 0.5, color: "text.tertiary" }} data-testid="viewing">Also viewing: {others.map((o) => o.name).join(", ")}</Typography>
      )}
      <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap", mb: 0.75 }}>
        <Tabs value={mode} onChange={(_, v) => v && setMode(v as "reply" | "note")} size="sm" sx={{ bgcolor: "transparent" }}>
          <TabList disableUnderline sx={{ gap: 0.5 }}>
            <Tab value="reply" variant={mode === "reply" ? "soft" : "plain"} color={mode === "reply" ? "primary" : "neutral"} data-testid="tab-reply">Public reply</Tab>
            <Tab value="note" variant={mode === "note" ? "soft" : "plain"} color={mode === "note" ? "warning" : "neutral"} data-testid="tab-note">Internal note</Tab>
          </TabList>
        </Tabs>
        <Box sx={{ flex: 1 }} />
        <Dropdown>
          <MenuButton size="sm" variant="plain" color="neutral" startDecorator={<AutoAwesomeIcon fontSize="small" />} endDecorator={<ArrowDropDownIcon />} loading={!!aiBusy} data-testid="ai-menu">AI help</MenuButton>
          <Menu size="sm" placement="bottom-end">
            <MenuItem onClick={() => runAi("suggest_reply")} data-testid="ai-reply">Suggest a reply from the knowledge base</MenuItem>
            <MenuItem onClick={() => runAi("summary")} data-testid="ai-summary">Summarise the thread</MenuItem>
            <MenuItem onClick={() => runAi("sentiment")} data-testid="ai-sentiment">Check the customer's mood</MenuItem>
            <MenuItem onClick={() => runAi("classify")} data-testid="ai-classify">Suggest type, priority and team</MenuItem>
            <ListDivider />
            <MenuItem disabled>You review everything before it is used.</MenuItem>
          </Menu>
        </Dropdown>
      </Box>
      {aiNote && (
        <Alert size="sm" variant="soft" color="primary" sx={{ mb: 0.75 }} endDecorator={<Button size="sm" variant="plain" onClick={() => setAiNote(null)}>Dismiss</Button>} data-testid="ai-note">
          <Box>
            {aiNote.kind === "suggest_reply" && <>Draft written from {aiNote.sources?.length ? aiNote.sources.map((s) => s.title).join(", ") : "the ticket only (no matching article)"}. Check it before sending.</>}
            {aiNote.kind === "summary" && <><b>Summary</b><Typography level="body-sm" sx={{ whiteSpace: "pre-wrap" }}>{aiNote.text}</Typography></>}
            {aiNote.kind === "sentiment" && aiNote.sentiment && <><b>Mood: {aiNote.sentiment.sentiment}.</b> {aiNote.sentiment.reason}</>}
          </Box>
        </Alert>
      )}
      <ReplyEditor
        ref={editor}
        value={draft}
        onChange={setDraft}
        tone={mode === "note" ? "warning" : "neutral"}
        placeholder={mode === "note" ? "Write a note only your team can see. Type @ to mention a teammate." : `Reply to ${row.requester_name || "the requester"}. Type @ to mention a teammate.`}
        onSubmit={() => void send(false)}
        footer={
          <Box sx={{ display: "flex", alignItems: "center", gap: 0.75, flexWrap: "wrap" }}>
            <Button size="sm" variant="plain" color="neutral" startDecorator={<AttachFileIcon fontSize="small" />} onClick={() => setPicker(true)} data-testid="attach-btn">Attach from Drive</Button>
            {files.map((f) => <Chip key={f.id} size="sm" variant="soft" endDecorator={<span style={{ cursor: "pointer" }} role="button" aria-label={`Remove ${f.name}`} onClick={() => setFiles((x) => x.filter((y) => y.id !== f.id))}>×</span>}>{f.name}</Chip>)}
            <Typography level="body-xs" sx={{ ml: "auto", color: "text.tertiary" }}>
              {draft.trim() ? (draft === saved ? "Draft saved" : "Saving draft…") : "Ctrl+Enter to send"}
            </Typography>
          </Box>
        }
      />
      {files.length > 0 && mode === "reply" && <Typography level="body-xs" sx={{ mt: 0.5, color: "text.tertiary" }}>Files in a public reply are shared as view-only links that work for 30 days.</Typography>}
      {suggest.length > 0 && (
        <Box sx={{ mt: 0.75, display: "flex", gap: 0.75, flexWrap: "wrap", alignItems: "center" }} data-testid="kb-suggest">
          <Typography level="body-xs" sx={{ color: "text.tertiary" }}>Suggested articles:</Typography>
          {suggest.map((a) => (
            <Chip key={a.id} size="sm" variant="outlined" onClick={() => editor.current?.insertText(`${a.title}: ${a.body.slice(0, 220)}${a.body.length > 220 ? "…" : ""}`)}>{a.title}</Chip>
          ))}
        </Box>
      )}
      <Box sx={{ display: "flex", gap: 1, mt: 1, alignItems: "center", flexWrap: "wrap" }}>
        <CannedPicker projectId={projectId} onPick={(b) => editor.current?.insertText(fillTemplate(b, { "requester.first_name": firstName, "ticket.ref": row.ref, "agent.name": meName }))} />
        {macroPicker}
        <Box sx={{ flex: 1 }} />
        {canSolve && (
          <Button size="sm" variant="outlined" color="success" disabled={!draft.trim() || sending} onClick={() => send(true)} data-testid="send-solve">Send and mark solved</Button>
        )}
        <Dropdown open={schedOpen} onOpenChange={(_, o) => setSchedOpen(o)}>
          <MenuButton size="sm" variant="outlined" color="neutral" disabled={!draft.trim() || sending} startDecorator={<ScheduleSendIcon fontSize="small" />} data-testid="schedule-btn">Send later</MenuButton>
          <Menu size="sm" placement="top-end" sx={{ minWidth: 260, p: 1 }}>
            <MenuItem onClick={() => schedule(inHours(1), false)}>In 1 hour</MenuItem>
            <MenuItem onClick={() => schedule(tomorrow9(), false)} data-testid="sched-tomorrow">Tomorrow at 9:00</MenuItem>
            {canSolve && <MenuItem onClick={() => schedule(tomorrow9(), true)}>Tomorrow at 9:00 and mark solved</MenuItem>}
            <ListDivider />
            <Box sx={{ display: "flex", gap: 0.75, px: 1, py: 0.5 }} onKeyDown={(e) => e.stopPropagation()}>
              <Input size="sm" type="datetime-local" value={schedAt} onChange={(e) => setSchedAt(e.target.value)} slotProps={{ input: { "data-testid": "sched-at" } as never }} />
              <Button size="sm" disabled={!schedAt} onClick={() => schedule(new Date(schedAt), false)} data-testid="sched-confirm">Schedule</Button>
            </Box>
          </Menu>
        </Dropdown>
        <Button size="sm" color={mode === "note" ? "warning" : "primary"} loading={sending} disabled={!draft.trim()} onClick={() => send(false)} data-testid="send-btn">
          {mode === "note" ? "Add note" : "Send reply"}
        </Button>
      </Box>
      {err && <Alert color="danger" size="sm" sx={{ mt: 1 }} data-testid="pane-error">{err}</Alert>}
      {picker && <DrivePicker mode="file" title="Attach a file" confirmLabel="Attach" onClose={() => setPicker(false)} onPick={(f) => { setPicker(false); const p = f as { id: string; name: string }; setFiles((x) => (x.some((y) => y.id === p.id) ? x : [...x, { id: p.id, name: p.name }])); }} />}
    </Box>
  );
});
