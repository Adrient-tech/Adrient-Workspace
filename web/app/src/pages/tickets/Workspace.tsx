import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Alert, Box, Button, Checkbox, Chip, CircularProgress, Dropdown, FormControl, FormLabel, IconButton, Input, ListDivider, Menu, MenuButton, MenuItem,
  Modal, ModalDialog, Option, Select, Sheet, Textarea, Tooltip, Typography, DialogTitle, DialogContent,
} from "@mui/joy";
import AddIcon from "@mui/icons-material/Add";
import SearchIcon from "@mui/icons-material/Search";
import CloseIcon from "@mui/icons-material/Close";
import ViewSidebarIcon from "@mui/icons-material/ViewSidebarOutlined";
import ViewListIcon from "@mui/icons-material/ViewListOutlined";
import WhatshotIcon from "@mui/icons-material/Whatshot";
import SnoozeIcon from "@mui/icons-material/Snooze";
import KeyboardIcon from "@mui/icons-material/KeyboardOutlined";
import DensityMediumIcon from "@mui/icons-material/DensityMedium";
import { createTicket, type Project, type Priority } from "./api";
import * as dk from "./deskApi";
import { svc, ago, slaLabel, PRIORITIES, TICKET_TYPES, type Row, type ViewCount, type Group, type StatusDef, type Sla } from "./svcApi";
import { TicketPane, PRI_COLOR, initials, type PeopleMap } from "./TicketPane";
import { GOTO_KEYS, ROW_HEIGHT, SHORTCUTS, closeTab, keyAction, openTab, visibleWindow, type Density, type KeyState, type ShortcutAction, type Tab } from "./workspaceLogic";
import { peopleApi } from "../../api/people";
import { OutOfOffice } from "./OutOfOffice";
import { EmptyState } from "../../components/ui/EmptyState";
import type { User } from "../../api/types";

const VIEW_ICON: Record<string, string> = { mine: "👤", unassigned: "📥", urgent: "🔥", all_open: "📂", stale: "⏳", incidents: "🚨", major: "🚨", snoozed: "💤", solved_recent: "✅" };
const PAGE = 60;

interface Props {
  user: User;
  project: Project;
  projects: Project[];
  onSelectProject: (id: string) => void;
  onNewProject: () => void;
  onTool: (t: "reports" | "settings" | "kb" | "customers") => void;
  refreshKey: number;
  onChanged: () => void;
  assistant?: React.ReactNode;
}

export function Workspace({ user, project, projects, onSelectProject, onNewProject, onTool, refreshKey, onChanged, assistant }: Props) {
  const [view, setView] = useState("all_open");
  const [q, setQ] = useState("");
  const [applied, setApplied] = useState("");
  const [group, setGroup] = useState("");
  const [rows, setRows] = useState<Row[]>([]);
  const [next, setNext] = useState("");
  const [views, setViews] = useState<ViewCount[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState("");
  const [density, setDensity] = useState<Density>(() => (localStorage.getItem("desk.density") as Density) || "comfortable");
  const [layout, setLayout] = useState<"split" | "full">(() => (localStorage.getItem("desk.layout") as "split" | "full") || "split");
  const [sel, setSel] = useState(0);
  const [checked, setChecked] = useState<Set<string>>(new Set());
  const [tabs, setTabs] = useState<Tab[]>([]);
  const [active, setActive] = useState<string | null>(null);
  const [statuses, setStatuses] = useState<StatusDef[]>([]);
  const [groups, setGroups] = useState<Group[]>([]);
  const [people, setPeople] = useState<PeopleMap>({});
  const [sla, setSla] = useState<Record<string, Sla>>({});
  const [major, setMajor] = useState<Row[]>([]);
  const [cmd, setCmd] = useState<{ type: "reply" | "note" | "assign" | "solve" | "snooze" | "follow"; n: number } | null>(null);
  const [palette, setPalette] = useState(false);
  const [help, setHelp] = useState(false);
  const [newOpen, setNewOpen] = useState(false);
  const [scrollTop, setScrollTop] = useState(0);
  const [vh, setVh] = useState(600);
  const listRef = useRef<HTMLDivElement | null>(null);
  const searchRef = useRef<HTMLInputElement | null>(null);
  const keyState = useRef<KeyState>({ pendingG: false });
  const gTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const reqId = useRef(0);
  const cmdN = useRef(0);

  const rowH = ROW_HEIGHT[density];
  const nameOf = useCallback((id?: string) => (id ? people[id] || (id === user.id ? user.display_name : "") : ""), [people, user]);

  // ---- data ----
  const loadFirst = useCallback(async () => {
    const id = ++reqId.current;
    setLoading(true);
    try {
      const r = await svc.list(project.id, { view, q: applied, group, limit: PAGE });
      if (id !== reqId.current) return;
      setRows(r.tickets);
      setNext(r.next);
      if (r.views) setViews(r.views);
      setErr("");
      setSel(0);
      if (listRef.current) listRef.current.scrollTop = 0;
    } catch (e) {
      if (id === reqId.current) setErr((e as Error).message);
    } finally {
      if (id === reqId.current) setLoading(false);
    }
  }, [project.id, view, applied, group]);
  useEffect(() => { void loadFirst(); }, [loadFirst, refreshKey]);

  const loadMore = useCallback(async () => {
    if (!next || loading) return;
    const id = reqId.current;
    try {
      const r = await svc.list(project.id, { view, q: applied, group, limit: PAGE, cursor: next });
      if (id !== reqId.current) return;
      setRows((cur) => [...cur, ...r.tickets.filter((t) => !cur.some((c) => c.id === t.id))]);
      setNext(r.next);
    } catch (e) { setErr((e as Error).message); }
  }, [next, loading, project.id, view, applied, group]);

  useEffect(() => {
    svc.statuses(project.id).then((r) => setStatuses(r.statuses)).catch(() => setStatuses([]));
    svc.groups(project.id).then(setGroups).catch(() => setGroups([]));
    svc.slaStates(project.id).then(setSla).catch(() => setSla({}));
    peopleApi.search("", 200).then((ps) => setPeople(Object.fromEntries(ps.map((p) => [p.id, p.name])))).catch(() => undefined);
  }, [project.id, refreshKey]);
  useEffect(() => { svc.major().then(setMajor).catch(() => setMajor([])); }, [refreshKey]);
  useEffect(() => { setTabs([]); setActive(null); setView("all_open"); setQ(""); setApplied(""); setGroup(""); }, [project.id]);
  useEffect(() => { localStorage.setItem("desk.density", density); }, [density]);
  useEffect(() => { localStorage.setItem("desk.layout", layout); }, [layout]);
  useEffect(() => {
    const el = listRef.current;
    if (!el) return;
    const ro = new ResizeObserver(() => setVh(el.clientHeight));
    ro.observe(el);
    setVh(el.clientHeight);
    return () => ro.disconnect();
  }, [layout, loading]);

  // ---- open tickets ----
  const openRow = useCallback((r: Row) => {
    setTabs((t) => openTab(t, { id: r.id, ref: r.ref, title: r.title }));
    setActive(r.id);
  }, []);
  const openById = useCallback(async (id: string) => {
    const known = rows.find((r) => r.id === id);
    if (known) return openRow(known);
    try { const d = await dk.ticketDesk(id); openRow(d.ticket as unknown as Row); } catch (e) { setErr((e as Error).message); }
  }, [rows, openRow]);
  const closeT = (id: string) => { const r = closeTab(tabs, id, active); setTabs(r.tabs); setActive(r.active); };

  // ---- keyboard ----
  const run = useCallback((a: ShortcutAction) => {
    const issue = (type: NonNullable<typeof cmd>["type"]) => { if (active) setCmd({ type, n: ++cmdN.current }); };
    switch (a.type) {
      case "move": setSel((s) => { const n = Math.max(0, Math.min(rows.length - 1, s + a.delta)); const el = listRef.current; if (el) { const top = n * rowH; if (top < el.scrollTop) el.scrollTop = top; else if (top + rowH > el.scrollTop + el.clientHeight) el.scrollTop = top + rowH - el.clientHeight; } return n; }); break;
      case "open": if (rows[sel]) openRow(rows[sel]); break;
      case "reply": case "note": case "assign": case "solve": case "snooze": case "follow": issue(a.type); break;
      case "palette": setPalette(true); break;
      case "search": searchRef.current?.focus(); break;
      case "help": setHelp(true); break;
      case "close-tab": if (active) closeT(active); break;
      case "goto": setView(a.view); break;
    }
  }, [rows, sel, rowH, active, openRow]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement;
      const typing = !!t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.tagName === "SELECT" || t.isContentEditable);
      if (e.key === "Escape") { setHelp(false); setPalette(false); if (typing) (t as HTMLElement).blur(); return; }
      const r = keyAction(keyState.current, { key: e.key, shift: e.shiftKey, ctrl: e.ctrlKey, meta: e.metaKey, alt: e.altKey, typing });
      keyState.current = r.next;
      if (gTimer.current) clearTimeout(gTimer.current);
      if (r.next.pendingG) gTimer.current = setTimeout(() => { keyState.current = { pendingG: false }; }, 1200);
      if (r.action.type !== "none") { e.preventDefault(); run(r.action); }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [run]);

  const win = useMemo(() => visibleWindow(rows.length, rowH, scrollTop, vh), [rows.length, rowH, scrollTop, vh]);
  const showList = layout === "split" || !active;

  async function bulk(c: Omit<dk.BulkChange, "ids">) {
    try { await dk.bulk({ ids: [...checked], ...c }); setChecked(new Set()); await loadFirst(); onChanged(); } catch (e) { setErr((e as Error).message); }
  }

  return (
    <Box sx={{ display: "flex", flexDirection: "column", height: "calc(100vh - 56px)", minHeight: 520, bgcolor: "background.body" }} data-testid="workspace">
      {/* top bar */}
      <Box sx={{ display: "flex", alignItems: "center", gap: 1, px: 2, py: 1, borderBottom: "1px solid", borderColor: "divider", bgcolor: "background.surface", flexWrap: "wrap" }}>
        <Select size="sm" value={project.id} onChange={(_, v) => v === "__new" ? onNewProject() : v && onSelectProject(v)} sx={{ minWidth: 190, fontWeight: 600 }} slotProps={{ button: { "data-testid": "project-select" } as never }}>
          {projects.map((p) => <Option key={p.id} value={p.id}>{p.name}</Option>)}
          <ListDivider />
          <Option value="__new">+ New project</Option>
        </Select>
        <Input
          size="sm" placeholder="Search tickets (try status:open assignee:me tag:vpn)  /" value={q} startDecorator={<SearchIcon fontSize="small" />}
          onChange={(e) => setQ(e.target.value)} onKeyDown={(e) => { if (e.key === "Enter") setApplied(q.trim()); }}
          endDecorator={q ? <IconButton size="sm" variant="plain" onClick={() => { setQ(""); setApplied(""); }}><CloseIcon fontSize="small" /></IconButton> : undefined}
          slotProps={{ input: { ref: searchRef, "data-testid": "desk-search" } as never }} sx={{ flex: 1, minWidth: 220, maxWidth: 520 }}
        />
        <Box sx={{ flex: 1 }} />
        <Button size="sm" variant="plain" color="neutral" onClick={() => onTool("reports")} data-testid="open-report">Reports</Button>
        <Button size="sm" variant="plain" color="neutral" onClick={() => onTool("kb")} data-testid="open-kb">Knowledge base</Button>
        <Button size="sm" variant="plain" color="neutral" onClick={() => onTool("customers")} data-testid="open-customers">Customers</Button>
        <Button size="sm" variant="plain" color="neutral" onClick={() => onTool("settings")} data-testid="open-help-settings">Settings</Button>
        <Dropdown>
          <MenuButton slots={{ root: IconButton }} slotProps={{ root: { size: "sm", variant: "plain", color: "neutral", "aria-label": "Layout and density" } }}><DensityMediumIcon fontSize="small" /></MenuButton>
          <Menu size="sm" placement="bottom-end">
            {(["compact", "comfortable", "spacious"] as Density[]).map((d) => <MenuItem key={d} selected={density === d} onClick={() => setDensity(d)}>{d[0].toUpperCase() + d.slice(1)} rows</MenuItem>)}
            <ListDivider />
            <MenuItem selected={layout === "split"} onClick={() => setLayout("split")}><ViewSidebarIcon fontSize="small" />&nbsp;Split view</MenuItem>
            <MenuItem selected={layout === "full"} onClick={() => setLayout("full")}><ViewListIcon fontSize="small" />&nbsp;Full view</MenuItem>
          </Menu>
        </Dropdown>
        <OutOfOffice />
        <Tooltip title="Desk shortcuts (h)"><IconButton size="sm" variant="plain" color="neutral" onClick={() => setHelp(true)}><KeyboardIcon fontSize="small" /></IconButton></Tooltip>
        {assistant}
        <Button size="sm" startDecorator={<AddIcon />} onClick={() => setNewOpen(true)} data-testid="new-ticket">New ticket</Button>
      </Box>

      {major.length > 0 && (
        <Alert color="danger" variant="solid" size="sm" startDecorator={<WhatshotIcon />} sx={{ borderRadius: 0 }} data-testid="major-incidents">
          <Box sx={{ display: "flex", gap: 1.5, alignItems: "center", flexWrap: "wrap", width: "100%" }}>
            <b>{major.length === 1 ? "Major incident" : `${major.length} major incidents`}</b>
            {major.slice(0, 3).map((m) => <Chip key={m.id} size="sm" variant="soft" onClick={() => void openById(m.id)}>{m.ref} {m.title.slice(0, 48)}</Chip>)}
          </Box>
        </Alert>
      )}

      <Box sx={{ flex: 1, minHeight: 0, display: "grid", gridTemplateColumns: layout === "split" ? { xs: "1fr", lg: "208px 380px minmax(0,1fr)" } : { xs: "1fr", lg: "208px minmax(0,1fr)" } }}>
        {/* views */}
        <Box sx={{ display: { xs: "none", lg: "flex" }, flexDirection: "column", gap: 0.25, p: 1, borderRight: "1px solid", borderColor: "divider", overflow: "auto", bgcolor: "background.surface" }} data-testid="views">
          <Typography level="body-xs" sx={{ px: 1, py: 0.5, textTransform: "uppercase", letterSpacing: 0.5, color: "text.tertiary" }}>Views</Typography>
          {views.map((v) => (
            <Box key={v.id} onClick={() => { setView(v.id); setApplied(""); setQ(""); }} data-testid={`view-${v.id}`}
              sx={{ display: "flex", alignItems: "center", gap: 1, px: 1, py: 0.75, borderRadius: "md", cursor: "pointer", bgcolor: view === v.id ? "primary.softBg" : "transparent", color: view === v.id ? "primary.700" : "text.primary", "&:hover": { bgcolor: view === v.id ? "primary.softBg" : "neutral.softHoverBg" } }}>
              <span style={{ width: 18, textAlign: "center" }}>{VIEW_ICON[v.id] ?? "•"}</span>
              <Typography level="body-sm" sx={{ flex: 1, fontWeight: view === v.id ? 600 : 400, color: "inherit" }}>{v.title}</Typography>
              <Typography level="body-xs" sx={{ color: v.id === "major" && v.count ? "danger.600" : "text.tertiary", fontWeight: 600 }}>{v.count}</Typography>
            </Box>
          ))}
          <Typography level="body-xs" sx={{ px: 1, pt: 1.5, pb: 0.5, textTransform: "uppercase", letterSpacing: 0.5, color: "text.tertiary" }}>Teams</Typography>
          <Box onClick={() => setGroup("")} sx={{ px: 1, py: 0.75, borderRadius: "md", cursor: "pointer", bgcolor: group === "" ? "neutral.softBg" : "transparent", "&:hover": { bgcolor: "neutral.softHoverBg" } }}><Typography level="body-sm">All teams</Typography></Box>
          {groups.map((g) => (
            <Box key={g.id} onClick={() => setGroup(g.id)} data-testid={`group-${g.name}`} sx={{ display: "flex", alignItems: "center", px: 1, py: 0.75, borderRadius: "md", cursor: "pointer", bgcolor: group === g.id ? "primary.softBg" : "transparent", "&:hover": { bgcolor: "neutral.softHoverBg" } }}>
              <Typography level="body-sm" sx={{ flex: 1, fontWeight: group === g.id ? 600 : 400 }}>{g.name}</Typography>
              <Typography level="body-xs" sx={{ color: "text.tertiary", fontWeight: 600 }}>{g.open_count}</Typography>
            </Box>
          ))}
          <Box onClick={() => setGroup("none")} sx={{ px: 1, py: 0.75, borderRadius: "md", cursor: "pointer", bgcolor: group === "none" ? "neutral.softBg" : "transparent", "&:hover": { bgcolor: "neutral.softHoverBg" } }}><Typography level="body-sm" sx={{ color: "text.tertiary" }}>No team</Typography></Box>
        </Box>

        {/* list */}
        {showList && (
          <Box sx={{ display: { xs: active ? "none" : "flex", lg: "flex" }, flexDirection: "column", minWidth: 0, minHeight: 0, borderRight: layout === "split" ? "1px solid" : "none", borderColor: "divider", bgcolor: "background.surface" }} data-testid="list-pane">
            <Box sx={{ display: "flex", alignItems: "center", gap: 1, px: 1.5, py: 0.75, borderBottom: "1px solid", borderColor: "divider", minHeight: 38 }}>
              <Checkbox size="sm" checked={rows.length > 0 && checked.size === rows.length} indeterminate={checked.size > 0 && checked.size < rows.length}
                onChange={(e) => setChecked(e.target.checked ? new Set(rows.map((r) => r.id)) : new Set())} aria-label="Select all" />
              {checked.size > 0 ? (
                <Box sx={{ display: "flex", gap: 0.75, alignItems: "center", flexWrap: "wrap" }}>
                  <Typography level="body-sm" sx={{ fontWeight: 600 }}>{checked.size} selected</Typography>
                  <Button size="sm" variant="soft" onClick={() => bulk({ assignee: "me" })}>Assign to me</Button>
                  <Select size="sm" placeholder="Set status" value={null} onChange={(_, v) => v && bulk({ status: String(v) })} sx={{ minWidth: 110 }}>{statuses.map((s) => <Option key={s.key} value={s.key}>{s.label}</Option>)}</Select>
                  <Select size="sm" placeholder="Priority" value={null} onChange={(_, v) => v && bulk({ priority: String(v) })} sx={{ minWidth: 92 }}>{PRIORITIES.map((p) => <Option key={p} value={p}>{p}</Option>)}</Select>
                </Box>
              ) : (
                <Typography level="body-sm" sx={{ color: "text.secondary" }} data-testid="list-title">
                  {applied ? `Search: ${applied}` : views.find((v) => v.id === view)?.title ?? "Tickets"}
                  {rows.length > 0 && <span style={{ opacity: 0.6 }}> · {rows.length}{next ? "+" : ""}</span>}
                </Typography>
              )}
              <Box sx={{ flex: 1 }} />
              <Select size="sm" variant="plain" value={view} onChange={(_, v) => v && (setView(v), setApplied(""), setQ(""))} sx={{ display: { lg: "none" } }}>
                {views.map((v) => <Option key={v.id} value={v.id}>{v.title} ({v.count})</Option>)}
              </Select>
            </Box>
            <Box ref={listRef} onScroll={(e) => { const el = e.currentTarget; setScrollTop(el.scrollTop); if (next && el.scrollTop + el.clientHeight > el.scrollHeight - rowH * 8) void loadMore(); }} sx={{ flex: 1, overflow: "auto" }} data-testid="ticket-list">
              {loading && rows.length === 0 ? (
                <Box sx={{ display: "grid", placeItems: "center", py: 8 }}><CircularProgress /></Box>
              ) : rows.length === 0 ? (
                <EmptyState illustration="inbox" title={applied ? "No tickets match" : "Nothing here"} hint={applied ? "Try fewer filters, or search by words from the title." : "Tickets that match this view will show up here."} />
              ) : (
                <Box sx={{ pt: `${win.padTop}px`, pb: `${win.padBottom}px` }}>
                  {rows.slice(win.start, win.end).map((r, i) => (
                    <TicketRow key={r.id} r={r} index={win.start + i} h={rowH} density={density} selected={sel === win.start + i} open={active === r.id}
                      checked={checked.has(r.id)} onCheck={(on) => setChecked((c) => { const n = new Set(c); on ? n.add(r.id) : n.delete(r.id); return n; })}
                      assignee={nameOf(r.assignee_user_id)} sla={sla[r.id]} statusLabel={statuses.find((s) => s.key === r.status)?.label ?? r.status}
                      onClick={() => { setSel(win.start + i); openRow(r); }} />
                  ))}
                </Box>
              )}
              {next && <Box sx={{ p: 1.5, textAlign: "center" }}><Button size="sm" variant="plain" onClick={() => void loadMore()}>Load more</Button></Box>}
            </Box>
            {err && <Alert color="danger" size="sm" sx={{ m: 1 }}>{err}</Alert>}
          </Box>
        )}

        {/* detail */}
        {(layout === "split" || active) && (
          <Box sx={{ display: { xs: active ? "flex" : "none", lg: layout === "split" || active ? "flex" : "none" }, flexDirection: "column", minWidth: 0, minHeight: 0 }} data-testid="detail-pane">
            {tabs.length > 0 && (
              <Box sx={{ display: "flex", alignItems: "center", gap: 0.5, px: 1, pt: 0.75, borderBottom: "1px solid", borderColor: "divider", overflowX: "auto", bgcolor: "background.level1" }} data-testid="tabs">
                <Button size="sm" variant="plain" sx={{ display: { xs: "inline-flex", lg: layout === "full" ? "inline-flex" : "none" } }} onClick={() => setActive(null)} startDecorator={<ViewListIcon fontSize="small" />}>List</Button>
                {tabs.map((t) => (
                  <Box key={t.id} onClick={() => setActive(t.id)} data-testid={`tab-${t.ref}`}
                    sx={{ display: "flex", alignItems: "center", gap: 0.75, px: 1.25, py: 0.6, maxWidth: 230, borderRadius: "8px 8px 0 0", cursor: "pointer", bgcolor: active === t.id ? "background.surface" : "transparent", borderBottom: active === t.id ? "2px solid" : "2px solid transparent", borderColor: active === t.id ? "primary.500" : "transparent" }}>
                    <Typography level="body-xs" sx={{ fontFamily: "monospace", fontWeight: 600 }}>{t.ref}</Typography>
                    <Typography level="body-xs" noWrap sx={{ maxWidth: 110 }}>{t.title}</Typography>
                    <IconButton size="sm" variant="plain" sx={{ "--IconButton-size": "18px" }} aria-label={`Close ${t.ref}`} onClick={(e) => { e.stopPropagation(); closeT(t.id); }}><CloseIcon sx={{ fontSize: 14 }} /></IconButton>
                  </Box>
                ))}
              </Box>
            )}
            <Box sx={{ flex: 1, minHeight: 0 }}>
              {active ? (
                <TicketPane key={active} ticketId={active} statuses={statuses} groups={groups} people={people} meId={user.id} meName={user.display_name} projectId={project.id}
                  command={cmd} onChanged={() => { onChanged(); void loadFirst(); }} onOpenTicket={(id) => void openById(id)} full={layout === "full"} />
              ) : (
                <EmptyState illustration="inbox" title="Select a ticket" hint="Pick a ticket from the list. Use j and k to move, Enter to open, r to reply." />
              )}
            </Box>
          </Box>
        )}
      </Box>

      {palette && <CommandBar onClose={() => setPalette(false)} projectId={project.id} onGoto={(v) => { setView(v); setApplied(""); setQ(""); }} onTool={onTool} onOpen={(r) => openRow(r)} onNew={() => setNewOpen(true)} setDensity={setDensity} setLayout={setLayout} />}
      {help && <ShortcutHelp onClose={() => setHelp(false)} />}
      {newOpen && <NewTicket project={project} groups={groups} onClose={() => setNewOpen(false)} onCreated={(id) => { setNewOpen(false); onChanged(); void loadFirst().then(() => openById(id)); }} />}
    </Box>
  );
}

function TicketRow({ r, index, h, density, selected, open, checked, onCheck, onClick, assignee, sla, statusLabel }: {
  r: Row; index: number; h: number; density: Density; selected: boolean; open: boolean; checked: boolean; onCheck: (on: boolean) => void; onClick: () => void; assignee: string; sla?: Sla; statusLabel: string;
}) {
  const clk = slaLabel(sla?.resolution.state === "met" || !sla ? sla?.first_response : (sla.first_response.state === "breached" ? sla.first_response : sla.resolution));
  const stripe = r.priority === "urgent" ? "danger.500" : r.priority === "high" ? "warning.500" : "transparent";
  return (
    <Box onClick={onClick} data-testid={`row-${r.ref}`} data-index={index}
      sx={{ height: h, display: "flex", alignItems: "center", gap: 1, px: 1.25, borderBottom: "1px solid", borderColor: "divider", cursor: "pointer", borderLeft: "3px solid", borderLeftColor: stripe,
        bgcolor: open ? "primary.softBg" : selected ? "neutral.softBg" : "transparent", "&:hover": { bgcolor: open ? "primary.softBg" : "neutral.softHoverBg" }, outline: selected ? "2px solid" : "none", outlineColor: "primary.300", outlineOffset: -2 }}>
      <Checkbox size="sm" checked={checked} onClick={(e) => e.stopPropagation()} onChange={(e) => onCheck(e.target.checked)} aria-label={`Select ${r.ref}`} />
      {density !== "compact" && <Box sx={{ width: 28, height: 28, borderRadius: "50%", bgcolor: "neutral.softBg", display: "grid", placeItems: "center", fontSize: 11, fontWeight: 700, flexShrink: 0 }}>{initials(r.requester_name)}</Box>}
      <Box sx={{ minWidth: 0, flex: 1 }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 0.75 }}>
          {r.major && <WhatshotIcon sx={{ fontSize: 15, color: "danger.500" }} />}
          {r.snoozed_until && <SnoozeIcon sx={{ fontSize: 14, color: "text.tertiary" }} />}
          <Typography level="title-sm" noWrap sx={{ flex: 1, fontWeight: r.status === "open" ? 700 : 500 }}>{r.title}</Typography>
          <Typography level="body-xs" sx={{ color: "text.tertiary", flexShrink: 0 }}>{ago(r.updated_at)}</Typography>
        </Box>
        {density !== "compact" && (
          <Box sx={{ display: "flex", alignItems: "center", gap: 0.75, mt: 0.25 }}>
            <Typography level="body-xs" sx={{ fontFamily: "monospace", color: "text.tertiary" }}>{r.ref}</Typography>
            <Typography level="body-xs" noWrap sx={{ color: "text.secondary", flex: 1 }}>{r.requester_name}{assignee ? ` → ${assignee}` : " · unassigned"}</Typography>
            {clk.text && <Chip size="sm" variant="soft" color={clk.tone} sx={{ "--Chip-minHeight": "18px", fontSize: 10 }}>{clk.text}</Chip>}
          </Box>
        )}
        {density === "spacious" && (
          <Box sx={{ display: "flex", gap: 0.5, mt: 0.5, alignItems: "center" }}>
            <Chip size="sm" variant="soft" color={PRI_COLOR[r.priority]}>{r.priority}</Chip>
            <Chip size="sm" variant="outlined">{statusLabel}</Chip>
            <Chip size="sm" variant="outlined">{r.type}</Chip>
            {r.tags.slice(0, 2).map((t) => <Chip key={t} size="sm" variant="soft" color="primary">{t}</Chip>)}
          </Box>
        )}
      </Box>
      {density !== "spacious" && <Chip size="sm" variant="soft" color={PRI_COLOR[r.priority]} sx={{ flexShrink: 0 }}>{density === "compact" ? r.priority[0].toUpperCase() : statusLabel}</Chip>}
    </Box>
  );
}

function ShortcutHelp({ onClose }: { onClose: () => void }) {
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ maxWidth: 520, width: "100%" }} data-testid="shortcut-help">
        <DialogTitle>Keyboard shortcuts</DialogTitle>
        <DialogContent>
          <Box sx={{ display: "grid", gridTemplateColumns: "auto 1fr", gap: 1, columnGap: 2 }}>
            {SHORTCUTS.map((s) => (<><Chip key={s.keys + "k"} size="sm" variant="outlined" sx={{ fontFamily: "monospace", justifySelf: "start" }}>{s.keys}</Chip><Typography key={s.keys + "d"} level="body-sm">{s.does}</Typography></>))}
          </Box>
          <Typography level="body-xs" sx={{ mt: 1.5, color: "text.tertiary" }}>Shortcuts are off while you type in a box. Press Escape to leave a box.</Typography>
        </DialogContent>
      </ModalDialog>
    </Modal>
  );
}

function CommandBar({ onClose, projectId, onGoto, onTool, onOpen, onNew, setDensity, setLayout }: {
  onClose: () => void; projectId: string; onGoto: (v: string) => void; onTool: (t: "reports" | "settings" | "kb" | "customers") => void; onOpen: (r: Row) => void; onNew: () => void; setDensity: (d: Density) => void; setLayout: (l: "split" | "full") => void;
}) {
  const [text, setText] = useState("");
  const [hits, setHits] = useState<Row[]>([]);
  const [idx, setIdx] = useState(0);
  const commands = useMemo(() => [
    { label: "New ticket", run: onNew },
    ...Object.values(GOTO_KEYS).map((g) => ({ label: `Go to: ${g.label}`, run: () => onGoto(g.view) })),
    { label: "Open reports", run: () => onTool("reports") }, { label: "Open knowledge base", run: () => onTool("kb") }, { label: "Open desk settings", run: () => onTool("settings") },
    { label: "Layout: split view", run: () => setLayout("split") }, { label: "Layout: full view", run: () => setLayout("full") },
    { label: "Rows: compact", run: () => setDensity("compact") }, { label: "Rows: comfortable", run: () => setDensity("comfortable") }, { label: "Rows: spacious", run: () => setDensity("spacious") },
  ], []); // eslint-disable-line react-hooks/exhaustive-deps
  const cmds = commands.filter((c) => c.label.toLowerCase().includes(text.toLowerCase())).slice(0, 8);
  useEffect(() => {
    if (text.trim().length < 2) return setHits([]);
    const t = setTimeout(() => { svc.list(projectId, { view: "all", q: text.trim(), limit: 6 }).then((r) => setHits(r.tickets)).catch(() => setHits([])); }, 200);
    return () => clearTimeout(t);
  }, [text, projectId]);
  const items = [...hits.map((h) => ({ label: `${h.ref}  ${h.title}`, run: () => onOpen(h) })), ...cmds];
  const go = (i: number) => { const it = items[i]; if (it) { onClose(); it.run(); } };
  return (
    <Modal open onClose={onClose}>
      <ModalDialog layout="center" sx={{ width: 560, maxWidth: "94vw", p: 0, top: "22%", transform: "translateX(-50%)" }} data-testid="command-bar">
        <Input autoFocus size="lg" variant="plain" placeholder="Type a command or search tickets…" value={text} onChange={(e) => { setText(e.target.value); setIdx(0); }}
          onKeyDown={(e) => { if (e.key === "ArrowDown") { e.preventDefault(); setIdx((i) => Math.min(items.length - 1, i + 1)); } if (e.key === "ArrowUp") { e.preventDefault(); setIdx((i) => Math.max(0, i - 1)); } if (e.key === "Enter") go(idx); }}
          startDecorator={<SearchIcon />} sx={{ borderBottom: "1px solid", borderColor: "divider", borderRadius: 0 }} />
        <Box sx={{ maxHeight: 340, overflow: "auto", p: 0.5 }}>
          {items.length === 0 && <Typography level="body-sm" sx={{ p: 2, color: "text.tertiary" }}>Nothing matches.</Typography>}
          {items.map((it, i) => (
            <Box key={it.label + i} onClick={() => go(i)} onMouseEnter={() => setIdx(i)} sx={{ px: 1.5, py: 0.9, borderRadius: "md", cursor: "pointer", bgcolor: i === idx ? "primary.softBg" : "transparent" }}>
              <Typography level="body-sm" noWrap>{it.label}</Typography>
            </Box>
          ))}
        </Box>
      </ModalDialog>
    </Modal>
  );
}

function NewTicket({ project, groups, onClose, onCreated }: { project: Project; groups: Group[]; onClose: () => void; onCreated: (id: string) => void }) {
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [type, setType] = useState<dk.TicketType>("question");
  const [urgency, setUrgency] = useState("medium");
  const [impact, setImpact] = useState("medium");
  const [group, setGroup] = useState("");
  const [category, setCategory] = useState("");
  const [priority, setPriority] = useState<Priority>("normal");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [sug, setSug] = useState<{ id: string; title: string }[]>([]);
  useEffect(() => {
    if (title.trim().length < 4) return setSug([]);
    const t = setTimeout(() => { svc.suggest(project.id, title + " " + body).then((a) => setSug(a)).catch(() => setSug([])); }, 400);
    return () => clearTimeout(t);
  }, [title, body, project.id]);
  async function submit() {
    setBusy(true);
    setErr("");
    try {
      const t = await createTicket(project.id, { title: title.trim(), body: body.trim(), priority });
      await svc.setFields(t.id, { urgency, impact, category, group_id: group });
      if (type !== "question") await dk.setMeta(t.id, { type });
      onCreated(t.id);
    } catch (e) { setErr((e as Error).message); } finally { setBusy(false); }
  }
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ maxWidth: 560, width: "100%", maxHeight: "92vh", overflow: "auto" }} data-testid="new-ticket-dialog">
        <DialogTitle>New ticket · {project.name}</DialogTitle>
        <DialogContent>
          <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5, mt: 1 }}>
            <FormControl size="sm"><FormLabel>Subject</FormLabel><Input autoFocus value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Short summary" slotProps={{ input: { "data-testid": "nt-title" } as never }} /></FormControl>
            {sug.length > 0 && (
              <Sheet variant="soft" color="primary" sx={{ p: 1, borderRadius: "md" }} data-testid="nt-suggest">
                <Typography level="body-xs" sx={{ fontWeight: 600 }}>Articles that may already answer this</Typography>
                {sug.map((a) => <Typography key={a.id} level="body-sm">• {a.title}</Typography>)}
              </Sheet>
            )}
            <FormControl size="sm"><FormLabel>Details</FormLabel><Textarea minRows={4} value={body} onChange={(e) => setBody(e.target.value)} placeholder="Describe the request…" /></FormControl>
            <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1.5 }}>
              <FormControl size="sm"><FormLabel>Type</FormLabel><Select value={type} onChange={(_, v) => v && setType(v as dk.TicketType)}>{TICKET_TYPES.map((t) => <Option key={t} value={t}>{t}</Option>)}</Select></FormControl>
              <FormControl size="sm"><FormLabel>Team</FormLabel><Select placeholder="No team" value={group || null} onChange={(_, v) => setGroup(v ?? "")}><Option value="">No team</Option>{groups.map((g) => <Option key={g.id} value={g.id}>{g.name}</Option>)}</Select></FormControl>
              <FormControl size="sm"><FormLabel>Urgency</FormLabel><Select value={urgency} onChange={(_, v) => v && setUrgency(v)}>{["low", "medium", "high"].map((t) => <Option key={t} value={t}>{t}</Option>)}</Select></FormControl>
              <FormControl size="sm"><FormLabel>Impact</FormLabel><Select value={impact} onChange={(_, v) => v && setImpact(v)}>{["low", "medium", "high"].map((t) => <Option key={t} value={t}>{t}</Option>)}</Select></FormControl>
              <FormControl size="sm"><FormLabel>Category</FormLabel><Input value={category} onChange={(e) => setCategory(e.target.value)} placeholder="Network, Accounts…" /></FormControl>
              <FormControl size="sm"><FormLabel>Priority (before matrix)</FormLabel><Select value={priority} onChange={(_, v) => v && setPriority(v as Priority)}>{PRIORITIES.map((t) => <Option key={t} value={t}>{t}</Option>)}</Select></FormControl>
            </Box>
            <Typography level="body-xs" sx={{ color: "text.tertiary" }}>Urgency and impact set the final priority. Routing rules and automations run when the ticket is created.</Typography>
            {err && <Alert color="danger" size="sm">{err}</Alert>}
            <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1 }}>
              <Button variant="plain" color="neutral" onClick={onClose}>Cancel</Button>
              <Button loading={busy} disabled={!title.trim()} onClick={submit} data-testid="nt-create">Create ticket</Button>
            </Box>
          </Box>
        </DialogContent>
      </ModalDialog>
    </Modal>
  );
}

