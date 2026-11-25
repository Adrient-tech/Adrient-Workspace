import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Alert, Avatar, Box, Button, Chip, CircularProgress, Divider, Dropdown, FormControl, FormLabel, IconButton, Input, ListDivider, Menu, MenuButton, MenuItem,
  Option, Select, Sheet, Tooltip, Typography,
} from "@mui/joy";
import SnoozeIcon from "@mui/icons-material/Snooze";
import AccessTimeIcon from "@mui/icons-material/AccessTime";
import NotificationsNoneIcon from "@mui/icons-material/NotificationsNone";
import NotificationsActiveIcon from "@mui/icons-material/NotificationsActive";
import WhatshotIcon from "@mui/icons-material/Whatshot";
import SubdirectoryArrowRightIcon from "@mui/icons-material/SubdirectoryArrowRight";
import { getTicket, patchTicket, type Comment as OldComment } from "./api";
import * as dk from "./deskApi";
import { describeEvent } from "./deskLogic";
import { TicketDeskPanel } from "./Desk";
import { TicketCrmPanel } from "./TicketCrmPanel";
import { TicketLinks } from "./TicketLinks";
import { TicketCustomFields } from "./DeskFields";
import { svc, ago, fmtMin, slaLabel, fmtSize, SENTIMENT_COLOR, PRIORITIES, TICKET_TYPES, type Row, type Group, type StatusDef, type Presence, type Sla, type Approval, type TimeEntry, type AiResult, type Attachment } from "./svcApi";
import { parseBlocks, type Inline } from "./workspaceLogic";
import { Composer, type ComposerHandle } from "./Composer";
import { peopleApi, type Person } from "../../api/people";

export type PeopleMap = Record<string, string>;
export const PRI_COLOR: Record<string, "neutral" | "warning" | "danger" | "success"> = { low: "neutral", normal: "success", high: "warning", urgent: "danger" };

const errText = (e: unknown) => (e instanceof Error ? e.message : String(e));
export const initials = (n: string) => n.split(/\s+/).filter(Boolean).slice(0, 2).map((p) => p[0]?.toUpperCase()).join("") || "?";

/** Renders composer markup as React nodes (no HTML injection possible). */
export function RichText({ text, people }: { text: string; people?: PeopleMap }) {
  const inline = (parts: Inline[], key: string) =>
    parts.map((p, i) => {
      const k = `${key}-${i}`;
      switch (p.t) {
        case "bold": return <b key={k}>{p.v}</b>;
        case "italic": return <i key={k}>{p.v}</i>;
        case "code": return <code key={k} style={{ background: "var(--joy-palette-neutral-softBg)", padding: "0 4px", borderRadius: 4 }}>{p.v}</code>;
        case "link": return <a key={k} href={p.href} target="_blank" rel="noreferrer noopener" style={{ color: "var(--joy-palette-primary-600)" }}>{p.v}</a>;
        case "mention": return <Chip key={k} size="sm" variant="soft" color="primary" sx={{ mx: 0.25 }}>@{people?.[p.user] ?? p.v}</Chip>;
        default: return <span key={k}>{p.v}</span>;
      }
    });
  return (
    <Box sx={{ fontSize: "sm", lineHeight: 1.55, wordBreak: "break-word" }}>
      {parseBlocks(text).map((b, i) =>
        b.kind === "p" || b.kind === "q" ? (
          <Box key={i} sx={{ my: 0.5, ...(b.kind === "q" ? { pl: 1.5, borderLeft: "3px solid", borderColor: "neutral.300", color: "text.secondary" } : {}) }}>
            {b.lines.map((l, j) => <span key={j}>{inline(l, `${i}-${j}`)}{j < b.lines.length - 1 && <br />}</span>)}
          </Box>
        ) : (
          <Box key={i} component={b.kind} sx={{ my: 0.5, pl: 2.5 }}>
            {b.items.map((it, j) => <li key={j}>{inline(it, `${i}-${j}`)}</li>)}
          </Box>
        ),
      )}
    </Box>
  );
}

type Cmd = { type: "reply" | "note" | "assign" | "solve" | "snooze" | "follow"; n: number } | null;

function describe(e: dk.TicketEvent, name: (id: string) => string): string {
  const d = e.detail ?? {};
  const s = (k: string) => String(d[k] ?? "");
  switch (e.kind) {
    case "assignee": return s("to") ? `assigned this to ${name(s("to"))}${s("via") ? ` (${s("via").replace("routing:", "routing: ")})` : ""}` : "removed the assignee";
    case "automation": return Array.isArray(d.rules) ? `Automation ran: ${(d.rules as string[]).join(", ")}` : s("skipped") || s("webhook_error") || "Automation ran";
    case "reopen": return `reopened the ticket after a reply (was ${s("from")})`;
    case "major": return d.on ? "flagged this as a major incident" : "cleared the major incident flag";
    case "snooze": return d.until ? `snoozed until ${new Date(s("until")).toLocaleString()}` : "woke the ticket up";
    case "time": return d.minutes ? `logged ${fmtMin(Number(d.minutes))}${d.billable ? " (billable)" : ""}` : s("time_tracking") || "logged time";
    case "approval": return d.decision ? `${s("decision")} the change (step ${s("step")})` : `asked for approval (${s("requested_steps")} step${Number(d.requested_steps) === 1 ? "" : "s"})`;
    case "fields": return "updated category or group";
    case "parent": return s("to") ? "made this a child ticket" : "detached this from its parent";
    default: return describeEvent(e);
  }
}

interface Props {
  ticketId: string;
  statuses: StatusDef[];
  groups: Group[];
  people: PeopleMap;
  meId: string;
  meName: string;
  projectId: string;
  command: Cmd;
  onChanged: () => void;
  onOpenTicket: (id: string) => void;
  full?: boolean;
}

export function TicketPane(props: Props) {
  const { ticketId, statuses, groups, people, meId, projectId, onChanged, onOpenTicket } = props;
  const [row, setRow] = useState<Row | null>(null);
  const [events, setEvents] = useState<dk.TicketEvent[]>([]);
  const [comments, setComments] = useState<OldComment[]>([]);
  const [following, setFollowing] = useState(false);
  const [extra, setExtra] = useState<{ children: Row[]; approvals: Approval[]; approval_state: string; time: TimeEntry[]; time_total_min: number; sla: Sla } | null>(null);
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(true);
  const [others, setOthers] = useState<Presence[]>([]);
  const [typing, setTyping] = useState(false);
  const [suggestion, setSuggestion] = useState<AiResult | null>(null);
  const [files, setFiles] = useState<Attachment[]>([]);
  const [timeOpen, setTimeOpen] = useState(false);
  const [timeMin, setTimeMin] = useState("15");
  const [timeNote, setTimeNote] = useState("");
  const [timeBill, setTimeBill] = useState(false);
  const composer = useRef<ComposerHandle | null>(null);
  const loadedFor = useRef("");

  const nameOf = useCallback((id: string) => people[id] || (id === meId ? props.meName : "a teammate"), [people, meId, props.meName]);

  const load = useCallback(async (quiet = false) => {
    if (!quiet) setLoading(true);
    try {
      const [d, t, x] = await Promise.all([dk.ticketDesk(ticketId), getTicket(ticketId), svc.detail2(ticketId)]);
      setRow(d.ticket as unknown as Row);
      setEvents(d.events);
      setFollowing(d.following);
      setComments(t.comments ?? []);
      setExtra(x);
      svc.attachments(ticketId).then(setFiles).catch(() => setFiles([]));
      setErr("");
    } catch (e) {
      setErr(errText(e));
    } finally {
      setLoading(false);
    }
  }, [ticketId]);

  useEffect(() => {
    loadedFor.current = ticketId;
    setSuggestion(null);
    void load();
  }, [ticketId, load]);

  // presence heartbeat (collision detection)
  useEffect(() => {
    let alive = true;
    const beat = () => svc.presence(ticketId, typing ? "replying" : "viewing").then((o) => alive && setOthers(o)).catch(() => undefined);
    void beat();
    const t = setInterval(beat, 10000);
    return () => { alive = false; clearInterval(t); };
  }, [ticketId, typing]);

  const act = useCallback(async (fn: () => Promise<unknown>) => {
    setErr("");
    try { await fn(); await load(true); onChanged(); } catch (e) { setErr(errText(e)); }
  }, [load, onChanged]);

  // keyboard commands from the workspace
  const lastCmd = useRef(0);
  useEffect(() => {
    const c = props.command;
    if (!c || c.n === lastCmd.current || !row) return;
    lastCmd.current = c.n;
    if (c.type === "reply" || c.type === "note") composer.current?.focus(c.type);
    if (c.type === "assign") void act(() => patchTicket(row.id, { assignee_user_id: meId }));
    if (c.type === "solve") { const k = statuses.find((s) => s.category === "solved"); if (k) void act(() => patchTicket(row.id, { status: k.key })); }
    if (c.type === "snooze") { const d = new Date(); d.setDate(d.getDate() + 1); d.setHours(9, 0, 0, 0); void act(() => svc.snooze(row.id, d.toISOString())); }
    if (c.type === "follow") void act(() => dk.follow(row.id));
  }, [props.command, row, act, meId, statuses]);

  const timeline = useMemo(() => {
    type Item = { at: number; kind: "comment"; c: OldComment } | { at: number; kind: "event"; e: dk.TicketEvent };
    const items: Item[] = comments.map((c) => ({ at: c.created_at * 1000, kind: "comment" as const, c }));
    for (const e of events) if (e.kind !== "comment") items.push({ at: new Date(e.created_at).getTime(), kind: "event", e });
    return items.sort((a, b) => a.at - b.at);
  }, [comments, events]);

  if (loading && !row) return <Box sx={{ display: "grid", placeItems: "center", height: "100%" }}><CircularProgress /></Box>;
  if (!row) return <Alert color="danger" sx={{ m: 2 }}>{err || "Ticket not found."}</Alert>;
  const sla = extra?.sla;
  const statusDef = statuses.find((s) => s.key === row.status);
  const grp = groups.find((g) => g.id === row.group_id);
  const members = grp?.members ?? [];

  return (
    <Box sx={{ display: "grid", gridTemplateColumns: props.full ? "minmax(0,1fr) 320px" : { xs: "1fr", lg: "minmax(0,1fr) 290px" }, height: "100%", minHeight: 0 }} data-testid="ticket-pane">
      {/* conversation */}
      <Box sx={{ display: "flex", flexDirection: "column", minWidth: 0, minHeight: 0, borderRight: "1px solid", borderColor: "divider" }}>
        {row.major && (
          <Alert color="danger" variant="solid" startDecorator={<WhatshotIcon />} sx={{ borderRadius: 0 }} data-testid="major-banner">
            Major incident. The group has been notified. Post updates as public replies.
          </Alert>
        )}
        <Box sx={{ px: 2.5, py: 1.5, borderBottom: "1px solid", borderColor: "divider" }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
            <Typography level="body-xs" sx={{ fontFamily: "monospace", color: "text.tertiary" }}>{row.ref}</Typography>
            <Chip size="sm" variant="soft" color={PRI_COLOR[row.priority]}>{row.priority}</Chip>
            <Chip size="sm" variant="outlined">{row.type}</Chip>
            {row.sentiment && <Chip size="sm" variant="soft" color={SENTIMENT_COLOR[row.sentiment] ?? "neutral"} data-testid="sentiment-chip">mood: {row.sentiment}</Chip>}
            {row.snoozed_until && <Chip size="sm" variant="soft" startDecorator={<SnoozeIcon fontSize="small" />}>Snoozed until {new Date(row.snoozed_until).toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" })}</Chip>}
            <Box sx={{ flex: 1 }} />
            <Tooltip title={following ? "Stop following (f)" : "Follow updates (f)"}>
              <IconButton size="sm" variant={following ? "soft" : "plain"} color={following ? "primary" : "neutral"} onClick={() => act(() => dk.follow(row.id))} data-testid="follow-btn">
                {following ? <NotificationsActiveIcon fontSize="small" /> : <NotificationsNoneIcon fontSize="small" />}
              </IconButton>
            </Tooltip>
            <Dropdown>
              <MenuButton size="sm" variant="plain" color="neutral" startDecorator={<SnoozeIcon fontSize="small" />} data-testid="snooze-btn">Snooze</MenuButton>
              <Menu size="sm">
                {[["In 4 hours", 4], ["Tomorrow 9:00", -1], ["In 3 days", 72], ["Next week", 168]].map(([l, h]) => (
                  <MenuItem key={String(l)} onClick={() => {
                    const d = new Date();
                    if (Number(h) < 0) { d.setDate(d.getDate() + 1); d.setHours(9, 0, 0, 0); } else d.setHours(d.getHours() + Number(h));
                    void act(() => svc.snooze(row.id, d.toISOString()));
                  }}>{l}</MenuItem>
                ))}
                {row.snoozed_until && <><ListDivider /><MenuItem onClick={() => act(() => svc.snooze(row.id, null))}>Wake up now</MenuItem></>}
              </Menu>
            </Dropdown>
            <Button size="sm" variant="plain" color="neutral" startDecorator={<AccessTimeIcon fontSize="small" />} onClick={() => setTimeOpen((v) => !v)} data-testid="time-btn">
              {extra?.time_total_min ? fmtMin(extra.time_total_min) : "Log time"}
            </Button>
          </Box>
          <Typography level="title-lg" sx={{ mt: 0.5 }}>{row.title}</Typography>
          {timeOpen && (
            <Sheet variant="soft" sx={{ mt: 1, p: 1.25, borderRadius: "md", display: "flex", gap: 1, alignItems: "flex-end", flexWrap: "wrap" }}>
              <FormControl size="sm"><FormLabel>Minutes</FormLabel><Input type="number" value={timeMin} onChange={(e) => setTimeMin(e.target.value)} sx={{ width: 90 }} slotProps={{ input: { min: 1, max: 1440 } }} /></FormControl>
              <FormControl size="sm" sx={{ flex: 1, minWidth: 160 }}><FormLabel>What did you do?</FormLabel><Input value={timeNote} onChange={(e) => setTimeNote(e.target.value)} /></FormControl>
              <Button size="sm" variant={timeBill ? "solid" : "outlined"} onClick={() => setTimeBill((v) => !v)}>{timeBill ? "Billable" : "Not billable"}</Button>
              <Button size="sm" onClick={() => act(async () => { await svc.addTime(row.id, { minutes: Number(timeMin), note: timeNote, billable: timeBill }); setTimeOpen(false); setTimeNote(""); })}>Add</Button>
              {extra && extra.time.length > 0 && (
                <Typography level="body-xs" sx={{ width: "100%", color: "text.tertiary" }}>
                  Logged: {extra.time.slice(0, 4).map((t) => `${t.user_name} ${fmtMin(t.minutes)}${t.note ? ` (${t.note})` : ""}`).join(", ")}. Entries are also sent to Time tracking when it is connected.
                </Typography>
              )}
            </Sheet>
          )}
        </Box>

        {/* timeline */}
        <Box sx={{ flex: 1, overflow: "auto", px: 2.5, py: 2, display: "flex", flexDirection: "column", gap: 1.5 }} data-testid="timeline">
          <Box sx={{ display: "flex", gap: 1.25 }}>
            <Avatar size="sm" color="neutral">{initials(row.requester_name)}</Avatar>
            <Box sx={{ minWidth: 0, flex: 1 }}>
              <Box sx={{ display: "flex", gap: 1, alignItems: "baseline" }}>
                <Typography level="title-sm">{row.requester_name || "Requester"}</Typography>
                <Typography level="body-xs" sx={{ color: "text.tertiary" }}>{row.requester_email} · via {row.source} · {ago(row.created_at)}</Typography>
              </Box>
              <Sheet variant="soft" sx={{ p: 1.5, mt: 0.5, borderRadius: "md" }}>
                <RichText text={row.body || "(no description)"} people={people} />
                {Object.keys(row.custom ?? {}).length > 0 && (
                  <Box sx={{ mt: 1, pt: 1, borderTop: "1px dashed", borderColor: "divider", display: "grid", gridTemplateColumns: "auto 1fr", columnGap: 1.5, rowGap: 0.25 }}>
                    {Object.entries(row.custom).map(([k, v]) => (<><Typography key={k + "k"} level="body-xs" sx={{ color: "text.tertiary" }}>{k.replace(/_/g, " ")}</Typography><Typography key={k + "v"} level="body-xs">{v}</Typography></>))}
                  </Box>
                )}
              </Sheet>
            </Box>
          </Box>
          {timeline.map((it, i) =>
            it.kind === "event" ? (
              <Typography key={`e${i}`} level="body-xs" sx={{ color: "text.tertiary", textAlign: "center" }}>
                <b>{it.e.actor_name || "System"}</b> {describe(it.e, nameOf)} · {ago(it.e.created_at)}
              </Typography>
            ) : (
              <Box key={it.c.id} sx={{ display: "flex", gap: 1.25, flexDirection: it.c.is_internal ? "row" : "row" }}>
                <Avatar size="sm" color={it.c.author_user_id ? "primary" : "neutral"}>{initials(it.c.author_name)}</Avatar>
                <Box sx={{ minWidth: 0, flex: 1 }}>
                  <Box sx={{ display: "flex", gap: 1, alignItems: "baseline" }}>
                    <Typography level="title-sm">{it.c.author_name || "Unknown"}</Typography>
                    {it.c.is_internal && <Chip size="sm" variant="soft" color="warning">internal note</Chip>}
                    <Typography level="body-xs" sx={{ color: "text.tertiary" }}>{ago(new Date(it.c.created_at * 1000).toISOString())}</Typography>
                  </Box>
                  <Sheet variant="soft" color={it.c.is_internal ? "warning" : "neutral"} sx={{ p: 1.5, mt: 0.5, borderRadius: "md", ...(it.c.author_user_id && !it.c.is_internal ? { bgcolor: "primary.softBg" } : {}) }}>
                    <RichText text={it.c.body} people={people} />
                  </Sheet>
                </Box>
              </Box>
            ),
          )}
        </Box>

        <Composer ref={composer} row={row} statuses={statuses} projectId={projectId} meName={props.meName} others={others} onTyping={setTyping}
          onSent={async () => { await load(true); onChanged(); }} onMacro={(id) => act(() => dk.applyMacro(row.id, id))}
          macroPicker={<MacroPicker projectId={projectId} onApply={(id) => act(() => dk.applyMacro(row.id, id))} />}
          onClassification={(r) => setSuggestion(r)} />
      </Box>

      {/* properties */}
      <Box sx={{ overflow: "auto", p: 2, display: { xs: props.full ? "block" : "none", lg: "block" }, bgcolor: "background.surface" }} data-testid="properties">
        <Typography level="title-sm" sx={{ mb: 1.25 }}>Properties</Typography>
        <Box sx={{ display: "flex", flexDirection: "column", gap: 1.25 }}>
          <Field label="Status">
            <Select size="sm" value={row.status} onChange={(_, v) => v && act(() => patchTicket(row.id, { status: v }))} slotProps={{ button: { "data-testid": "status-select" } as never }}>
              {statuses.map((s) => <Option key={s.key} value={s.key}>{s.label}{s.pauses_sla ? " (pauses SLA)" : ""}</Option>)}
            </Select>
            {statusDef?.requires.length ? <Typography level="body-xs" sx={{ color: "text.tertiary" }}>Needs {statusDef.requires.join(", ")} before this status.</Typography> : null}
          </Field>
          <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1 }}>
            <Field label="Priority">
              <Select size="sm" value={row.priority} onChange={(_, v) => v && act(() => patchTicket(row.id, { priority: v as never }))}>
                {PRIORITIES.map((p) => <Option key={p} value={p}>{p}</Option>)}
              </Select>
            </Field>
            <Field label="Type">
              <Select size="sm" value={row.type} onChange={(_, v) => v && act(() => dk.setMeta(row.id, { type: v as dk.TicketType }))}>
                {TICKET_TYPES.map((p) => <Option key={p} value={p}>{p}</Option>)}
              </Select>
            </Field>
            <Field label="Urgency">
              <Select size="sm" value={row.urgency} onChange={(_, v) => v && act(() => svc.setFields(row.id, { urgency: v }))}>
                {["low", "medium", "high"].map((p) => <Option key={p} value={p}>{p}</Option>)}
              </Select>
            </Field>
            <Field label="Impact">
              <Select size="sm" value={row.impact} onChange={(_, v) => v && act(() => svc.setFields(row.id, { impact: v }))}>
                {["low", "medium", "high"].map((p) => <Option key={p} value={p}>{p}</Option>)}
              </Select>
            </Field>
          </Box>
          <Typography level="body-xs" sx={{ color: "text.tertiary", mt: -0.5 }}>Urgency and impact set the priority.</Typography>
          <Field label="Group">
            <Select size="sm" placeholder="No group" value={row.group_id ?? null} onChange={(_, v) => act(() => svc.setFields(row.id, { group_id: v ?? "" }))} slotProps={{ button: { "data-testid": "group-select" } as never }}>
              <Option value="">No group</Option>
              {groups.map((g) => <Option key={g.id} value={g.id}>{g.name}</Option>)}
            </Select>
          </Field>
          <Field label="Assignee">
            <Select size="sm" placeholder="Unassigned" value={row.assignee_user_id ?? null} onChange={(_, v) => act(() => (v ? patchTicket(row.id, { assignee_user_id: v }) : patchTicket(row.id, { clear_assignee: true })))} slotProps={{ button: { "data-testid": "assignee-select" } as never }}>
              <Option value="">Unassigned</Option>
              {[...new Map([...members.map((m) => [m.user_id, m.name || nameOf(m.user_id)] as const), [meId, props.meName] as const, ...(row.assignee_user_id ? [[row.assignee_user_id, nameOf(row.assignee_user_id)] as const] : [])]).entries()].map(([id, n]) => (
                <Option key={id} value={id}>{n}{id === meId ? " (me)" : ""}</Option>
              ))}
            </Select>
          </Field>
          <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1 }}>
            <Field label="Category"><Input size="sm" defaultValue={row.category} key={row.id + row.category} onBlur={(e) => e.target.value !== row.category && act(() => svc.setFields(row.id, { category: e.target.value }))} /></Field>
            <Field label="Subcategory"><Input size="sm" defaultValue={row.subcategory} key={row.id + row.subcategory} onBlur={(e) => e.target.value !== row.subcategory && act(() => svc.setFields(row.id, { subcategory: e.target.value }))} /></Field>
          </Box>

          <Divider />
          <TicketCustomFields row={row} projectId={projectId} onSave={async (c) => { await svc.setFields(row.id, { custom: c }); await load(true); }} />
          {row.need_skills?.length > 0 && <Typography level="body-xs" sx={{ color: "text.tertiary" }}>Needs skills: {row.need_skills.join(", ")}</Typography>}
          {suggestion?.classification && (
            <Sheet variant="soft" color="primary" sx={{ p: 1.25, borderRadius: "md" }} data-testid="classify-card">
              <Typography level="title-sm">AI suggestion</Typography>
              <Typography level="body-sm">Type {suggestion.classification.type}, priority {suggestion.classification.priority}{suggestion.classification.category ? `, category ${suggestion.classification.category}` : ""}{suggestion.classification.group ? `, team ${suggestion.classification.group}` : ""}.</Typography>
              {suggestion.classification.reason && <Typography level="body-xs" sx={{ color: "text.secondary" }}>{suggestion.classification.reason}</Typography>}
              <Box sx={{ display: "flex", gap: 1, mt: 1 }}>
                <Button size="sm" onClick={() => act(async () => { await svc.applyClassification(row.id, suggestion.classification!); setSuggestion(null); })} data-testid="classify-apply">Apply</Button>
                <Button size="sm" variant="plain" color="neutral" onClick={() => setSuggestion(null)}>Dismiss</Button>
              </Box>
            </Sheet>
          )}
          {row.ai_summary && (
            <Box data-testid="ai-summary-card" sx={{ p: 1, borderRadius: "md", bgcolor: "background.level1" }}>
              <Typography level="body-xs" sx={{ color: "text.tertiary", textTransform: "uppercase" }}>AI summary</Typography>
              <Typography level="body-sm" sx={{ whiteSpace: "pre-wrap" }}>{row.ai_summary}</Typography>
            </Box>
          )}
          {files.length > 0 && (
            <Box data-testid="ticket-files">
              <Typography level="title-sm">Files</Typography>
              {files.map((f) => (
                <Box key={f.id} sx={{ display: "flex", gap: 1, alignItems: "center" }}>
                  <a href={`/api/v1/drive/files/${f.file_id}/content`} target="_blank" rel="noreferrer" style={{ flex: 1, fontSize: 13, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{f.name}</a>
                  <Typography level="body-xs" sx={{ color: "text.tertiary" }}>{fmtSize(f.size)}</Typography>
                </Box>
              ))}
            </Box>
          )}
          <Typography level="title-sm">SLA</Typography>
          <SlaRow label="First response" c={sla?.first_response} />
          <SlaRow label="Resolution" c={sla?.resolution} />

          {row.type === "change" && extra && <ApprovalBox ticketId={row.id} approvals={extra.approvals} state={extra.approval_state} meId={meId} people={people} act={act} />}

          {extra && extra.children.length > 0 && (
            <>
              <Divider />
              <Typography level="title-sm">Child tickets</Typography>
              {extra.children.map((c) => (
                <Box key={c.id} onClick={() => onOpenTicket(c.id)} sx={{ display: "flex", gap: 0.75, alignItems: "center", cursor: "pointer", "&:hover": { color: "primary.600" } }}>
                  <SubdirectoryArrowRightIcon fontSize="small" /><Typography level="body-sm" noWrap sx={{ flex: 1 }}>{c.ref} {c.title}</Typography><Chip size="sm" variant="soft">{c.status}</Chip>
                </Box>
              ))}
            </>
          )}
          {row.parent_id && <Button size="sm" variant="soft" onClick={() => onOpenTicket(row.parent_id!)}>Open parent ticket</Button>}
          <Divider />
          <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap" }}>
            <Button size="sm" variant={row.major ? "solid" : "outlined"} color="danger" startDecorator={<WhatshotIcon fontSize="small" />} onClick={() => act(() => svc.setMajor(row.id, !row.major))} data-testid="major-toggle">
              {row.major ? "Clear major incident" : "Major incident"}
            </Button>
          </Box>
          <Typography level="title-sm">Customer</Typography>
          <Typography level="body-sm">{row.requester_name} <span style={{ opacity: 0.6 }}>{row.requester_email}</span></Typography>
          <TicketCrmPanel ticketId={row.id} />
          <TicketLinks ticketId={row.id} requesterEmail={row.requester_email} />
          <Divider />
          <details>
            <summary style={{ cursor: "pointer", fontSize: 13, fontWeight: 600 }}>Tags, merge, macros and history</summary>
            <TicketDeskPanel ticketId={row.id} projectId={projectId} onChanged={() => { void load(true); onChanged(); }} />
          </details>
        </Box>
      </Box>
    </Box>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <FormControl size="sm">
      <FormLabel sx={{ fontSize: 11, textTransform: "uppercase", letterSpacing: 0.4, color: "text.tertiary" }}>{label}</FormLabel>
      {children}
    </FormControl>
  );
}

function SlaRow({ label, c }: { label: string; c?: { state: string; left_min?: number; paused?: boolean; due?: string } }) {
  const l = slaLabel(c as never);
  if (!l.text) return <Typography level="body-xs" sx={{ color: "text.tertiary" }}>{label}: no target</Typography>;
  return (
    <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 1 }}>
      <Typography level="body-sm">{label}</Typography>
      <Tooltip title={c?.due ? `Due ${new Date(c.due).toLocaleString()}` : ""}><Chip size="sm" variant="soft" color={l.tone}>{l.text}</Chip></Tooltip>
    </Box>
  );
}

function MacroPicker({ projectId, onApply }: { projectId: string; onApply: (id: string) => void }) {
  const [list, setList] = useState<dk.Macro[]>([]);
  useEffect(() => { dk.listMacros(projectId).then(setList).catch(() => setList([])); }, [projectId]);
  if (!list.length) return null;
  return (
    <Select size="sm" placeholder="Apply macro" value={null} onChange={(_, v) => v && onApply(String(v))} sx={{ minWidth: 150 }}>
      {list.map((m) => <Option key={m.id} value={m.id!}>{m.name}</Option>)}
    </Select>
  );
}

function ApprovalBox({ ticketId, approvals, state, meId, people, act }: { ticketId: string; approvals: Approval[]; state: string; meId: string; people: PeopleMap; act: (fn: () => Promise<unknown>) => Promise<void> }) {
  const [pick, setPick] = useState("");
  const [hits, setHits] = useState<Person[]>([]);
  const cur = approvals.find((a) => a.decision === "pending")?.step ?? 0;
  const mine = approvals.find((a) => a.approver_id === meId && a.decision === "pending" && a.step === cur);
  useEffect(() => { if (pick.length < 2) return setHits([]); const t = setTimeout(() => peopleApi.search(pick, 5).then(setHits).catch(() => setHits([])), 200); return () => clearTimeout(t); }, [pick]);
  return (
    <>
      <Divider />
      <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
        <Typography level="title-sm" sx={{ flex: 1 }}>Change approval</Typography>
        <Chip size="sm" variant="soft" color={state === "approved" ? "success" : state === "rejected" ? "danger" : state === "pending" ? "warning" : "neutral"} data-testid="approval-state">{state === "none" ? "not requested" : state}</Chip>
      </Box>
      {approvals.map((a) => (
        <Box key={a.id} sx={{ display: "flex", gap: 1, alignItems: "center" }}>
          <Typography level="body-xs" sx={{ width: 44, color: "text.tertiary" }}>Step {a.step}</Typography>
          <Typography level="body-sm" sx={{ flex: 1 }}>{a.approver || people[a.approver_id]}</Typography>
          <Chip size="sm" variant="soft" color={a.decision === "approved" ? "success" : a.decision === "rejected" ? "danger" : "neutral"}>{a.decision}</Chip>
        </Box>
      ))}
      {mine && (
        <Box sx={{ display: "flex", gap: 1 }}>
          <Button size="sm" color="success" onClick={() => act(() => svc.decide(ticketId, "approved", ""))} data-testid="approve">Approve</Button>
          <Button size="sm" color="danger" variant="outlined" onClick={() => act(() => svc.decide(ticketId, "rejected", ""))}>Reject</Button>
        </Box>
      )}
      {state === "none" || state === "approved" || state === "rejected" ? (
        <Box>
          <Input size="sm" placeholder="Ask someone to approve" value={pick} onChange={(e) => setPick(e.target.value)} />
          {hits.map((p) => (
            <Button key={p.id} size="sm" variant="plain" onClick={() => { setPick(""); void act(() => svc.requestApprovals(ticketId, [[p.id]])); }}>{p.name}</Button>
          ))}
        </Box>
      ) : null}
    </>
  );
}
