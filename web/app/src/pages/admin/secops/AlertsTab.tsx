// Triage queue: filters, bulk actions, SLA timers, and a detail panel with the
// evidence, sign-in timeline, comments and the one-click containment playbook.
import { useCallback, useEffect, useRef, useState } from "react";
import { Alert as JAlert, Box, Button, Checkbox, Chip, Drawer, FormControl, IconButton, Input, Option, Select, Sheet, Table, Textarea, Typography } from "@mui/joy";
import CloseIcon from "@mui/icons-material/Close";
import { A, iam, type Alert, type Comment, type Me, type SignIn, type Summary } from "./api";
import { ActionDialog } from "./ActionDialog";
import { ago, can, CONTAIN, SEVERITY_COLOR, slaText, STATUS_LABEL } from "./logic";

const when = (s?: string | null) => (s ? new Date(s).toLocaleString() : "");

export function AlertsTab({ me, openId, onOpenUser, onSummary }: { me: Me; openId?: string | null; onOpenUser: (id: string) => void; onSummary: (s: Summary) => void }) {
  const [items, setItems] = useState<Alert[]>([]);
  const [next, setNext] = useState("");
  const [total, setTotal] = useState(0);
  const [status, setStatus] = useState("active");
  const [sev, setSev] = useState("");
  const [mine, setMine] = useState(false);
  const [q, setQ] = useState("");
  const [sel, setSel] = useState<Set<string>>(new Set());
  const [detail, setDetail] = useState<string | null>(openId ?? null);
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(false);
  const qRef = useRef(q);
  qRef.current = q;

  const load = useCallback(
    async (cursor = "") => {
      setLoading(true);
      setErr("");
      try {
        const p = new URLSearchParams({ status, limit: "25" });
        if (sev) p.set("severity", sev);
        if (mine) p.set("assignee", "me");
        if (qRef.current.trim()) p.set("q", qRef.current.trim());
        if (cursor) p.set("cursor", cursor);
        const r = await iam<{ items: Alert[]; next_cursor: string; total: number; summary: Summary }>(`${A}/alerts?${p}`);
        setItems((old) => (cursor ? [...old, ...r.items] : r.items));
        setNext(r.next_cursor);
        setTotal(r.total);
        onSummary(r.summary);
      } catch (e) {
        setErr((e as Error).message);
      } finally {
        setLoading(false);
      }
    },
    [status, sev, mine, onSummary],
  );
  useEffect(() => {
    setSel(new Set());
    void load();
  }, [load]);
  useEffect(() => {
    const t = setTimeout(() => void load(), 300);
    return () => clearTimeout(t);
  }, [q]); // eslint-disable-line react-hooks/exhaustive-deps

  const bulk = async (statusTo: string) => {
    try {
      await iam(`${A}/alerts/bulk`, { method: "POST", json: { ids: [...sel], status: statusTo } });
      setSel(new Set());
      void load();
    } catch (e) {
      setErr((e as Error).message);
    }
  };
  const canTriage = can(me.capabilities, "triage");

  return (
    <Box data-testid="alerts-tab">
      <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", mb: 1.5, alignItems: "center" }}>
        <Select size="sm" value={status} onChange={(_, v) => v && setStatus(v)} sx={{ minWidth: 150 }}>
          <Option value="active">Active</Option>
          <Option value="open">Open</Option>
          <Option value="investigating">Investigating</Option>
          <Option value="resolved">Resolved</Option>
          <Option value="all">All</Option>
        </Select>
        <Select size="sm" placeholder="Any severity" value={sev || null} onChange={(_, v) => setSev(v ?? "")} sx={{ minWidth: 150 }}>
          <Option value="critical">Critical</Option>
          <Option value="high">High</Option>
          <Option value="medium">Medium</Option>
          <Option value="low">Low</Option>
        </Select>
        <Checkbox size="sm" label="Assigned to me" checked={mine} onChange={(e) => setMine(e.target.checked)} />
        <Input size="sm" placeholder="Search person, address or title" value={q} onChange={(e) => setQ(e.target.value)} sx={{ width: 280 }} slotProps={{ input: { "data-testid": "alert-search" } }} />
        <Box sx={{ flex: 1 }} />
        <Typography level="body-sm">{total} alert{total === 1 ? "" : "s"}</Typography>
      </Box>
      {sel.size > 0 && canTriage && (
        <Sheet variant="soft" color="primary" sx={{ p: 1, mb: 1, borderRadius: "sm", display: "flex", gap: 1, alignItems: "center" }} data-testid="bulk-bar">
          <Typography level="body-sm">{sel.size} selected</Typography>
          <Button size="sm" variant="outlined" onClick={() => void bulk("investigating")}>Mark investigating</Button>
          <Button size="sm" variant="outlined" onClick={() => void bulk("false_positive")}>False positive</Button>
          <Button size="sm" variant="outlined" onClick={() => void bulk("policy")}>Expected / policy</Button>
        </Sheet>
      )}
      {err && <JAlert color="danger" sx={{ mb: 1 }}>{err}</JAlert>}
      <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
        <Table hoverRow stickyHeader sx={{ tableLayout: "auto", "& td, & th": { whiteSpace: "nowrap" } }}>
          <thead>
            <tr>
              <th style={{ width: 36 }} />
              <th style={{ width: 90 }}>Severity</th>
              <th>Alert</th>
              <th>Person / address</th>
              <th style={{ width: 130 }}>Status</th>
              <th style={{ width: 130 }}>Assignee</th>
              <th style={{ width: 130 }}>SLA</th>
              <th style={{ width: 110 }}>Raised</th>
            </tr>
          </thead>
          <tbody>
            {items.map((a) => {
              const sla = slaText(a.sla_due_at, a.status);
              return (
                <tr key={a.id} onClick={() => setDetail(a.id)} style={{ cursor: "pointer" }} data-testid="alert-row">
                  <td onClick={(e) => e.stopPropagation()}>
                    <Checkbox size="sm" checked={sel.has(a.id)} onChange={(e) => setSel((s) => { const n = new Set(s); if (e.target.checked) n.add(a.id); else n.delete(a.id); return n; })} />
                  </td>
                  <td><Chip size="sm" color={SEVERITY_COLOR[a.severity]} variant="solid">{a.severity}</Chip></td>
                  <td style={{ whiteSpace: "normal", minWidth: 240, maxWidth: 340 }}>{a.title}</td>
                  <td>{a.email || a.ip}</td>
                  <td><Chip size="sm" variant="soft">{STATUS_LABEL[a.status] ?? a.status}</Chip></td>
                  <td>{a.assignee_name || "-"}</td>
                  <td><Typography level="body-sm" color={sla.late ? "danger" : "neutral"}>{sla.text}</Typography></td>
                  <td>{ago(a.created_at)}</td>
                </tr>
              );
            })}
            {!items.length && !loading && (
              <tr><td colSpan={8}><Typography level="body-sm" sx={{ p: 2 }}>No alerts match. Sign-ins are checked as they happen; nothing suspicious is waiting.</Typography></td></tr>
            )}
          </tbody>
        </Table>
      </Sheet>
      {next && <Button sx={{ mt: 1 }} variant="outlined" loading={loading} onClick={() => void load(next)} data-testid="alerts-more">Load more</Button>}
      <AlertDetail id={detail} me={me} onClose={() => setDetail(null)} onChanged={() => void load()} onOpenUser={onOpenUser} />
    </Box>
  );
}

function AlertDetail({ id, me, onClose, onChanged, onOpenUser }: { id: string | null; me: Me; onClose: () => void; onChanged: () => void; onOpenUser: (id: string) => void }) {
  const [d, setD] = useState<{ alert: Alert; comments: Comment[]; events: SignIn[] } | null>(null);
  const [note, setNote] = useState("");
  const [err, setErr] = useState("");
  const [contain, setContain] = useState(false);
  const load = useCallback(async () => {
    if (!id) return;
    try {
      setD(await iam(`${A}/alerts/${id}`));
    } catch (e) {
      setErr((e as Error).message);
    }
  }, [id]);
  useEffect(() => {
    setD(null);
    setErr("");
    void load();
  }, [load]);
  const patch = async (json: Record<string, unknown>) => {
    try {
      await iam(`${A}/alerts/${id}`, { method: "PATCH", json });
      await load();
      onChanged();
    } catch (e) {
      setErr((e as Error).message);
    }
  };
  const a = d?.alert;
  const canTriage = can(me.capabilities, "triage");
  const ev = a ? Object.entries(a.evidence).filter(([k]) => !["event_id"].includes(k)) : [];
  return (
    <Drawer open={!!id} anchor="right" onClose={onClose} slotProps={{ content: { sx: { width: { xs: "100%", md: 720 } } } }}>
      <Box sx={{ p: 2, overflow: "auto" }} data-testid="alert-detail">
        <Box sx={{ display: "flex", alignItems: "flex-start", gap: 1 }}>
          <Box sx={{ flex: 1 }}>
            {a && <Chip size="sm" color={SEVERITY_COLOR[a.severity]} variant="solid" sx={{ mb: 0.5 }}>{a.severity}</Chip>}
            <Typography level="h4">{a?.title ?? "Alert"}</Typography>
          </Box>
          <IconButton onClick={onClose} aria-label="Close"><CloseIcon /></IconButton>
        </Box>
        {err && <JAlert color="danger" sx={{ my: 1 }}>{err}</JAlert>}
        {a && (
          <>
            <Box sx={{ display: "flex", gap: 1, my: 1, flexWrap: "wrap", alignItems: "center" }}>
              <Chip variant="soft">{STATUS_LABEL[a.status] ?? a.status}</Chip>
              <Chip variant="outlined" color={slaText(a.sla_due_at, a.status).late ? "danger" : "neutral"}>SLA: {slaText(a.sla_due_at, a.status).text}</Chip>
              {a.assignee_name && <Chip variant="outlined">Assigned to {a.assignee_name}</Chip>}
              <Typography level="body-xs">Raised {when(a.created_at)}</Typography>
            </Box>
            <Sheet variant="soft" sx={{ p: 1.5, borderRadius: "md", mb: 1.5 }}>
              <Typography level="title-sm">Recommended</Typography>
              <Typography level="body-sm">{a.recommended}</Typography>
            </Sheet>
            {canTriage && (
              <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", mb: 1.5 }}>
                {a.status === "open" && <Button size="sm" variant="outlined" onClick={() => void patch({ status: "investigating", assignee_id: me.user_id })} data-testid="take">Take and investigate</Button>}
                {(a.status === "open" || a.status === "investigating") && (
                  <>
                    <Button size="sm" variant="outlined" onClick={() => void patch({ status: "false_positive" })} data-testid="fp">False positive</Button>
                    <Button size="sm" variant="outlined" onClick={() => void patch({ status: "policy" })}>Expected / policy</Button>
                    <Button size="sm" variant="outlined" color="danger" onClick={() => void patch({ status: "compromised" })}>Mark compromised</Button>
                  </>
                )}
                {(a.status !== "open" && a.status !== "investigating") && <Button size="sm" variant="outlined" onClick={() => void patch({ status: "open" })}>Reopen</Button>}
                {a.user_id && can(me.capabilities, "contain") && (
                  <Button size="sm" color="danger" onClick={() => setContain(true)} data-testid="contain-alert">Contain account…</Button>
                )}
                {a.user_id && <Button size="sm" variant="plain" onClick={() => onOpenUser(a.user_id)}>Open security tab</Button>}
              </Box>
            )}
            <Typography level="title-sm">Evidence</Typography>
            <Table size="sm" sx={{ mb: 1.5 }}>
              <tbody>
                {ev.map(([k, v]) => (
                  <tr key={k}><td style={{ width: 180, opacity: 0.7 }}>{k.replace(/_/g, " ")}</td><td>{typeof v === "object" ? JSON.stringify(v) : String(v)}</td></tr>
                ))}
              </tbody>
            </Table>
            <Typography level="title-sm">Sign-in timeline (48 hours around the alert)</Typography>
            <Table size="sm" sx={{ mb: 1.5 }} data-testid="alert-timeline">
              <thead><tr><th>When</th><th>Account</th><th>Result</th><th>Address</th><th>Place</th></tr></thead>
              <tbody>
                {d.events.map((e) => (
                  <tr key={e.id}><td>{when(e.at)}</td><td>{e.email}</td><td><Chip size="sm" variant="soft" color={e.success ? "success" : "danger"}>{e.success ? "ok" : "failed"}</Chip></td><td>{e.ip}</td><td>{[e.city, e.country].filter(Boolean).join(", ") || "-"}</td></tr>
                ))}
                {!d.events.length && <tr><td colSpan={5}>No events in range.</td></tr>}
              </tbody>
            </Table>
            <Typography level="title-sm">Notes and history</Typography>
            <Box sx={{ mb: 1 }} data-testid="alert-comments">
              {d.comments.map((c) => (
                <Box key={c.id} sx={{ py: 0.5, borderBottom: "1px solid", borderColor: "divider" }}>
                  <Typography level="body-xs">{c.author}, {when(c.created_at)} {c.kind !== "comment" && <Chip size="sm" variant="outlined">{c.kind}</Chip>}</Typography>
                  <Typography level="body-sm">{c.body}</Typography>
                </Box>
              ))}
            </Box>
            {canTriage && (
              <FormControl>
                <Textarea minRows={2} placeholder="Add a note" value={note} onChange={(e) => setNote(e.target.value)} slotProps={{ textarea: { "data-testid": "comment-input" } }} />
                <Button sx={{ mt: 1, alignSelf: "flex-start" }} size="sm" disabled={!note.trim()} data-testid="comment-add" onClick={async () => { try { await iam(`${A}/alerts/${id}/comments`, { method: "POST", json: { body: note } }); setNote(""); await load(); } catch (e) { setErr((e as Error).message); } }}>Add note</Button>
              </FormControl>
            )}
            {contain && <ActionDialog def={CONTAIN} target={{ id: a.user_id, email: a.email }} alertId={a.id} onClose={() => setContain(false)} onDone={() => { void load(); onChanged(); }} />}
          </>
        )}
      </Box>
    </Drawer>
  );
}
