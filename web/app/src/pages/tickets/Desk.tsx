import { useCallback, useEffect, useState } from "react";
import { Alert, Box, Button, Checkbox, Chip, CircularProgress, FormControl, FormLabel, IconButton, Input, Option, Select, Sheet, Textarea, Typography } from "@mui/joy";
import DeleteIcon from "@mui/icons-material/DeleteOutline";
import * as dk from "./deskApi";
import { FILTER_HELP, describeEvent, parseTags, selectionState, toggleAll, withFilter } from "./deskLogic";
import { SlaBadge } from "./Helpdesk";
import type { SlaInfo } from "./helpLogic";

const PRIORITIES = ["urgent", "high", "normal", "low"];
const PRIORITY_COLOR: Record<string, "neutral" | "warning" | "danger"> = { low: "neutral", normal: "neutral", high: "warning", urgent: "danger" };
const TYPES: dk.TicketType[] = ["question", "incident", "problem", "task"];
const when = (iso: string) => new Date(iso).toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" });
const errText = (e: unknown) => (e instanceof Error ? e.message : String(e));

/** The ticket list: built-in views with live counts, search with filters, tags and bulk edit. */
export function DeskList({
  projectId, statuses, sla, onOpenTicket, refreshKey, onChanged,
}: {
  projectId: string;
  statuses: string[];
  sla: Record<string, SlaInfo>;
  onOpenTicket: (id: string) => void;
  refreshKey: number;
  onChanged: () => void;
}) {
  const [view, setView] = useState("all_open");
  const [q, setQ] = useState("");
  const [applied, setApplied] = useState("");
  const [tips, setTips] = useState(false);
  const [data, setData] = useState<{ tickets: dk.DeskTicket[]; views: dk.DeskView[] } | null>(null);
  const [err, setErr] = useState("");
  const [sel, setSel] = useState<Set<string>>(new Set());
  const [bStatus, setBStatus] = useState("");
  const [bPriority, setBPriority] = useState("");
  const [bTag, setBTag] = useState("");
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState("");

  const load = useCallback(() => {
    // A search runs across every ticket; picking a view clears the search.
    dk.desk(projectId, applied ? { q: applied } : { view })
      .then((d) => {
        setData(d);
        setErr("");
      })
      .catch((e) => setErr(errText(e)));
  }, [projectId, view, applied]);
  useEffect(() => {
    setSel(new Set());
    load();
  }, [load, refreshKey]);
  useEffect(() => {
    setView("all_open");
    setQ("");
    setApplied("");
  }, [projectId]);

  const ids = (data?.tickets ?? []).map((t) => t.id);
  const state = selectionState(ids, sel);

  async function runBulk(c: Omit<dk.BulkChange, "ids">, done: string) {
    setBusy(true);
    setMsg("");
    try {
      const r = await dk.bulk({ ids: [...sel], ...c });
      setMsg(`${done}: ${r.changed} ticket${r.changed === 1 ? "" : "s"}.`);
      setBStatus("");
      setBPriority("");
      setBTag("");
      setSel(new Set());
      load();
      onChanged();
    } catch (e) {
      setErr(errText(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }} data-testid="desk-list">
      <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap" }} role="tablist" aria-label="Views">
        {(data?.views ?? []).map((v) => (
          <Chip
            key={v.id}
            size="md"
            variant={!applied && view === v.id ? "solid" : "outlined"}
            color={!applied && view === v.id ? "primary" : "neutral"}
            onClick={() => {
              setApplied("");
              setQ("");
              setView(v.id);
            }}
            slotProps={{ action: { role: "tab", "aria-selected": !applied && view === v.id, "data-testid": `desk-view-${v.id}` } }}
          >
            {v.title} · {v.count}
          </Chip>
        ))}
      </Box>

      <Box sx={{ display: "flex", gap: 1, alignItems: "center" }}>
        <Input
          size="sm"
          placeholder="Search tickets, e.g. status:open tag:billing refund"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") setApplied(q.trim());
          }}
          sx={{ flex: 1 }}
          slotProps={{ input: { "aria-label": "Search tickets", "data-testid": "desk-search" } }}
        />
        <Button size="sm" variant="soft" onClick={() => setApplied(q.trim())} data-testid="desk-search-go">Search</Button>
        {applied && (
          <Button size="sm" variant="plain" color="neutral" onClick={() => { setQ(""); setApplied(""); }}>Clear</Button>
        )}
        <Button size="sm" variant="plain" color="neutral" onClick={() => setTips((v) => !v)} aria-expanded={tips}>Search tips</Button>
      </Box>
      {tips && (
        <Sheet variant="soft" sx={{ borderRadius: "md", p: 1.5, display: "flex", gap: 1, flexWrap: "wrap" }} data-testid="desk-tips">
          {FILTER_HELP.map(([f, d]) => (
            <Chip key={f} size="sm" variant="outlined" onClick={() => setQ((cur) => withFilter(cur, f))} title={d}>
              {f}
            </Chip>
          ))}
        </Sheet>
      )}

      {err && <Alert color="danger" size="sm">{err}</Alert>}
      {msg && <Alert color="success" size="sm">{msg}</Alert>}

      {sel.size > 0 && (
        <Sheet variant="soft" color="primary" sx={{ borderRadius: "md", p: 1, display: "flex", gap: 1, flexWrap: "wrap", alignItems: "center" }} data-testid="bulk-bar">
          <Typography level="body-sm" sx={{ fontWeight: 600 }}>{sel.size} selected</Typography>
          <Select size="sm" placeholder="Status" value={bStatus || null} onChange={(_, v) => setBStatus(v ?? "")} sx={{ minWidth: 120 }}>
            {statuses.map((s) => <Option key={s} value={s}>{s}</Option>)}
          </Select>
          <Select size="sm" placeholder="Priority" value={bPriority || null} onChange={(_, v) => setBPriority(v ?? "")} sx={{ minWidth: 110 }}>
            {PRIORITIES.map((s) => <Option key={s} value={s}>{s}</Option>)}
          </Select>
          <Input size="sm" placeholder="Add tag" value={bTag} onChange={(e) => setBTag(e.target.value)} sx={{ width: 120 }} />
          <Button
            size="sm"
            loading={busy}
            disabled={!bStatus && !bPriority && !parseTags(bTag).length}
            onClick={() => runBulk({ status: bStatus || undefined, priority: bPriority || undefined, add_tags: parseTags(bTag) }, "Updated")}
            data-testid="bulk-apply"
          >
            Apply
          </Button>
          <Button size="sm" variant="outlined" loading={busy} onClick={() => runBulk({ assignee: "me" }, "Assigned to you")}>Assign to me</Button>
          <Button size="sm" variant="outlined" color="neutral" loading={busy} onClick={() => runBulk({ assignee: "none" }, "Unassigned")}>Unassign</Button>
          <Button size="sm" variant="plain" color="neutral" onClick={() => setSel(new Set())}>Clear selection</Button>
        </Sheet>
      )}

      <Sheet variant="outlined" sx={{ borderRadius: "lg", p: 0, overflow: "hidden" }}>
        {!data ? (
          <Box sx={{ display: "flex", justifyContent: "center", py: 6 }}><CircularProgress /></Box>
        ) : data.tickets.length === 0 ? (
          <Typography level="body-sm" sx={{ opacity: 0.6, p: 3 }}>
            {applied ? `No tickets match "${applied}".` : "No tickets in this view."}
          </Typography>
        ) : (
          <Box sx={{ display: "flex", flexDirection: "column" }}>
            <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, px: 1.5, py: 0.75, bgcolor: "neutral.softBg" }}>
              <Checkbox
                size="sm"
                checked={state === "all"}
                indeterminate={state === "some"}
                onChange={() => setSel(toggleAll(ids, sel))}
                slotProps={{ input: { "aria-label": "Select all tickets" } }}
              />
              <Typography level="body-xs" sx={{ opacity: 0.7 }}>{data.tickets.length} ticket{data.tickets.length === 1 ? "" : "s"}</Typography>
            </Box>
            {data.tickets.map((t) => (
              <Box
                key={t.id}
                sx={{ display: "flex", alignItems: "center", gap: 1.5, px: 1.5, py: 1, borderTop: "1px solid", borderColor: "divider", "&:hover": { bgcolor: "neutral.softBg" }, flexWrap: { xs: "wrap", md: "nowrap" } }}
                data-testid="desk-row"
              >
                <Checkbox
                  size="sm"
                  checked={sel.has(t.id)}
                  onChange={() => setSel((cur) => { const n = new Set(cur); if (n.has(t.id)) n.delete(t.id); else n.add(t.id); return n; })}
                  slotProps={{ input: { "aria-label": `Select ${t.ref}` } }}
                />
                <Box onClick={() => onOpenTicket(t.id)} sx={{ display: "flex", alignItems: "center", gap: 1.5, flex: 1, minWidth: 0, cursor: "pointer", flexWrap: { xs: "wrap", md: "nowrap" } }}>
                  <Typography level="body-xs" sx={{ fontFamily: "monospace", opacity: 0.6, minWidth: 72 }}>{t.ref}</Typography>
                  <Typography level="body-sm" sx={{ fontWeight: 600, flex: 1, minWidth: 120 }} noWrap>{t.title}</Typography>
                  {t.type !== "question" && <Chip size="sm" variant="outlined" color={t.type === "problem" ? "danger" : "neutral"}>{t.type}</Chip>}
                  {t.tags.slice(0, 3).map((g) => <Chip key={g} size="sm" variant="soft" color="primary">{g}</Chip>)}
                  {t.tags.length > 3 && <Typography level="body-xs">+{t.tags.length - 3}</Typography>}
                  <SlaBadge info={t.status === "resolved" || t.status === "closed" ? undefined : sla[t.id]} />
                  <Chip size="sm" variant="soft" color={t.status === "resolved" || t.status === "closed" ? "success" : "primary"}>{t.status}</Chip>
                  <Chip size="sm" variant="soft" color={PRIORITY_COLOR[t.priority]}>{t.priority}</Chip>
                  <Typography level="body-xs" sx={{ opacity: 0.6, minWidth: 100, textAlign: "right" }} noWrap>{t.requester_name || "-"}</Typography>
                  <Typography level="body-xs" sx={{ opacity: 0.5, minWidth: 110, textAlign: "right" }} noWrap>{when(t.updated_at)}</Typography>
                </Box>
              </Box>
            ))}
          </Box>
        )}
      </Sheet>
    </Box>
  );
}

/** Tags, type, problem link, merge, macros, follow and history for one ticket. */
export function TicketDeskPanel({
  ticketId, projectId, onChanged,
}: {
  ticketId: string;
  projectId: string;
  onChanged: () => void;
}) {
  const [d, setD] = useState<Awaited<ReturnType<typeof dk.ticketDesk>> | null>(null);
  const [macros, setMacros] = useState<dk.Macro[]>([]);
  const [problems, setProblems] = useState<dk.DeskTicket[]>([]);
  const [tagText, setTagText] = useState("");
  const [mergeQ, setMergeQ] = useState("");
  const [mergeHits, setMergeHits] = useState<dk.DeskTicket[]>([]);
  const [err, setErr] = useState("");
  const [showHistory, setShowHistory] = useState(false);

  const load = useCallback(() => {
    dk.ticketDesk(ticketId).then(setD).catch((e) => setErr(errText(e)));
  }, [ticketId]);
  useEffect(() => {
    load();
    dk.listMacros(projectId).then(setMacros).catch(() => setMacros([]));
    dk.desk(projectId, { q: "type:problem" }).then((r) => setProblems(r.tickets.filter((t) => t.id !== ticketId))).catch(() => setProblems([]));
  }, [load, projectId, ticketId]);

  async function act(fn: () => Promise<unknown>) {
    setErr("");
    try {
      await fn();
      load();
      onChanged();
    } catch (e) {
      setErr(errText(e));
    }
  }

  async function searchMerge() {
    if (!mergeQ.trim()) return setMergeHits([]);
    try {
      const r = await dk.desk(projectId, { q: mergeQ.trim() });
      setMergeHits(r.tickets.filter((t) => t.id !== ticketId && !t.merged_into).slice(0, 6));
    } catch (e) {
      setErr(errText(e));
    }
  }

  if (!d) return err ? <Alert color="danger" size="sm" sx={{ mt: 1 }}>{err}</Alert> : null;
  const t = d.ticket;
  return (
    <Box sx={{ mt: 2, display: "flex", flexDirection: "column", gap: 1.5 }} data-testid="desk-panel">
      {t.merged_into && <Alert color="warning" size="sm">This ticket was merged into another ticket and is closed.</Alert>}
      <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap", alignItems: "flex-end" }}>
        <FormControl size="sm">
          <FormLabel>Type</FormLabel>
          <Select value={t.type} onChange={(_, v) => v && act(() => dk.setMeta(t.id, { type: v as dk.TicketType }))} sx={{ minWidth: 120 }} data-testid="desk-type">
            {TYPES.map((x) => <Option key={x} value={x}>{x}</Option>)}
          </Select>
        </FormControl>
        {t.type !== "problem" && (
          <FormControl size="sm">
            <FormLabel>Problem</FormLabel>
            <Select
              placeholder="Not linked"
              value={t.problem_id ?? null}
              onChange={(_, v) => act(() => dk.setMeta(t.id, { problem_id: v ?? "" }))}
              sx={{ minWidth: 180 }}
              slotProps={{ button: { "data-testid": "desk-problem" } }}
            >
              {problems.map((p) => <Option key={p.id} value={p.id}>{p.ref} {p.title}</Option>)}
            </Select>
          </FormControl>
        )}
        {macros.length > 0 && (
          <FormControl size="sm">
            <FormLabel>Macro</FormLabel>
            <Select placeholder="Apply a macro" value={null} onChange={(_, v) => v && act(() => dk.applyMacro(t.id, String(v)))} sx={{ minWidth: 160 }} slotProps={{ button: { "data-testid": "desk-macro" } }}>
              {macros.map((m) => <Option key={m.id} value={m.id!}>{m.name}</Option>)}
            </Select>
          </FormControl>
        )}
        <Button size="sm" variant="outlined" onClick={() => act(() => dk.bulk({ ids: [t.id], assignee: t.assignee_user_id ? "none" : "me" }))}>
          {t.assignee_user_id ? "Unassign" : "Assign to me"}
        </Button>
        <Button size="sm" variant={d.following ? "solid" : "outlined"} onClick={() => act(() => dk.follow(t.id))} data-testid="desk-follow">
          {d.following ? "Following" : "Follow"}
        </Button>
      </Box>

      <Box sx={{ display: "flex", gap: 0.75, alignItems: "center", flexWrap: "wrap" }}>
        <Typography level="body-xs" sx={{ opacity: 0.7 }}>Tags</Typography>
        {t.tags.map((g) => (
          <Chip key={g} size="sm" variant="soft" color="primary" endDecorator={<IconButton size="sm" variant="plain" aria-label={`Remove tag ${g}`} onClick={() => act(() => dk.setMeta(t.id, { tags: t.tags.filter((x) => x !== g) }))}><DeleteIcon fontSize="inherit" /></IconButton>}>
            {g}
          </Chip>
        ))}
        <Input
          size="sm"
          placeholder="Add tag, press Enter"
          value={tagText}
          onChange={(e) => setTagText(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && parseTags(tagText).length) {
              const next = parseTags([...t.tags, tagText].join(","));
              setTagText("");
              void act(() => dk.setMeta(t.id, { tags: next }));
            }
          }}
          sx={{ width: 170 }}
          slotProps={{ input: { "data-testid": "desk-tag-input" } }}
        />
      </Box>

      {t.type === "problem" && (
        <Box>
          <Typography level="body-xs" sx={{ opacity: 0.7 }}>Linked incidents ({d.incidents.length})</Typography>
          {d.incidents.map((i) => <Typography key={i.id} level="body-sm">{i.ref} {i.title} <Chip size="sm" variant="soft">{i.status}</Chip></Typography>)}
        </Box>
      )}

      {!t.merged_into && (
        <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", alignItems: "center" }}>
          <Input
            size="sm"
            placeholder="Merge into another ticket: search"
            value={mergeQ}
            onChange={(e) => setMergeQ(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && void searchMerge()}
            sx={{ width: 260 }}
          />
          <Button size="sm" variant="outlined" onClick={() => void searchMerge()}>Find</Button>
          {mergeHits.map((m) => (
            <Button
              key={m.id}
              size="sm"
              variant="soft"
              color="warning"
              onClick={() => {
                if (window.confirm(`Merge ${t.ref} into ${m.ref}? ${t.ref} will be closed and its comments copied to ${m.ref}.`)) {
                  void act(async () => { await dk.merge(t.id, m.id); setMergeHits([]); setMergeQ(""); });
                }
              }}
              data-testid="desk-merge"
            >
              Merge into {m.ref}
            </Button>
          ))}
        </Box>
      )}

      {err && <Alert color="danger" size="sm">{err}</Alert>}

      <Box>
        <Button size="sm" variant="plain" color="neutral" onClick={() => setShowHistory((v) => !v)} aria-expanded={showHistory}>
          History ({d.events.length})
        </Button>
        {showHistory && (
          <Box component="ul" sx={{ m: 0, pl: 2.5 }} data-testid="desk-history">
            {d.events.length === 0 && <Typography level="body-xs">No changes recorded yet.</Typography>}
            {d.events.map((e) => (
              <li key={e.id}>
                <Typography level="body-xs"><b>{e.actor_name || "System"}</b> {describeEvent(e)} <span style={{ opacity: 0.55 }}>{when(e.created_at)}</span></Typography>
              </li>
            ))}
          </Box>
        )}
      </Box>
    </Box>
  );
}

/** Settings tab: create and delete macros. */
export function MacrosTab({ pid, statuses }: { pid: string; statuses: string[] }) {
  const [list, setList] = useState<dk.Macro[]>([]);
  const [name, setName] = useState("");
  const [status, setStatus] = useState("");
  const [priority, setPriority] = useState("");
  const [tags, setTags] = useState("");
  const [reply, setReply] = useState("");
  const [internal, setInternal] = useState(false);
  const [assignMe, setAssignMe] = useState(false);
  const [err, setErr] = useState("");
  const load = useCallback(() => { dk.listMacros(pid).then(setList).catch((e) => setErr(errText(e))); }, [pid]);
  useEffect(load, [load]);

  async function save() {
    setErr("");
    try {
      await dk.saveMacro({
        name,
        project_id: pid,
        actions: { status: status || undefined, priority: priority || undefined, add_tags: parseTags(tags), reply: reply.trim() || undefined, internal, assignee: assignMe ? "me" : undefined },
      });
      setName(""); setStatus(""); setPriority(""); setTags(""); setReply(""); setInternal(false); setAssignMe(false);
      load();
    } catch (e) {
      setErr(errText(e));
    }
  }

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }} data-testid="macros-tab">
      <Typography level="body-sm">A macro changes several things on a ticket in one click and can post a reply. In the reply, use {"{{requester.first_name}}"}, {"{{ticket.ref}}"} and {"{{agent.name}}"}.</Typography>
      {list.map((m) => (
        <Sheet key={m.id} variant="outlined" sx={{ p: 1, borderRadius: "md", display: "flex", justifyContent: "space-between", gap: 1 }}>
          <Box>
            <Typography level="title-sm">{m.name}</Typography>
            <Typography level="body-xs">
              {[m.actions.status && `status ${m.actions.status}`, m.actions.priority && `priority ${m.actions.priority}`, m.actions.assignee && "assign to me", m.actions.add_tags?.length ? `tags +${m.actions.add_tags.join(", ")}` : "", m.actions.reply && (m.actions.internal ? "internal note" : "public reply")].filter(Boolean).join(" · ")}
            </Typography>
          </Box>
          <IconButton size="sm" variant="plain" color="danger" aria-label={`Delete macro ${m.name}`} onClick={() => dk.deleteMacro(m.id!).then(load).catch((e) => setErr(errText(e)))}><DeleteIcon /></IconButton>
        </Sheet>
      ))}
      <Sheet variant="soft" sx={{ p: 1.5, borderRadius: "md", display: "flex", flexDirection: "column", gap: 1 }}>
        <Input size="sm" placeholder="Macro name, e.g. Solve and thank" value={name} onChange={(e) => setName(e.target.value)} slotProps={{ input: { "data-testid": "macro-name" } }} />
        <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap" }}>
          <Select size="sm" placeholder="Set status" value={status || null} onChange={(_, v) => setStatus(v ?? "")} sx={{ minWidth: 130 }}>
            {statuses.map((s) => <Option key={s} value={s}>{s}</Option>)}
          </Select>
          <Select size="sm" placeholder="Set priority" value={priority || null} onChange={(_, v) => setPriority(v ?? "")} sx={{ minWidth: 130 }}>
            {PRIORITIES.map((s) => <Option key={s} value={s}>{s}</Option>)}
          </Select>
          <Input size="sm" placeholder="Add tags, comma separated" value={tags} onChange={(e) => setTags(e.target.value)} sx={{ flex: 1, minWidth: 160 }} />
          <Checkbox size="sm" label="Assign to the agent" checked={assignMe} onChange={(e) => setAssignMe(e.target.checked)} />
        </Box>
        <Textarea minRows={2} placeholder="Reply text (optional)" value={reply} onChange={(e) => setReply(e.target.value)} />
        <Checkbox size="sm" label="Post as internal note" checked={internal} onChange={(e) => setInternal(e.target.checked)} />
        {err && <Alert color="danger" size="sm">{err}</Alert>}
        <Button size="sm" onClick={save} disabled={!name.trim()} sx={{ alignSelf: "flex-start" }} data-testid="macro-save">Add macro</Button>
      </Sheet>
    </Box>
  );
}
