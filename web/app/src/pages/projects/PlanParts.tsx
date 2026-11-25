import { useCallback, useEffect, useState } from "react";
import { Alert, Box, Button, Checkbox, Chip, FormControl, FormLabel, IconButton, Input, Modal, ModalClose, ModalDialog, Option, Select, Tab, TabList, TabPanel, Tabs, Textarea, Typography } from "@mui/joy";
import DeleteIcon from "@mui/icons-material/DeleteOutline";
import * as api from "./planApi";
import { emptyPlan, milestoneProgress, timeSummary, wouldCycle, type IssuePlan, type Milestone } from "./plan";
import type { Issue, Project, Team } from "./types";
import { ExchangeTab } from "./ExchangeTab";
import { makeTask } from "../../lib/makeTask";

/** Dates, time, milestone and prerequisites of one issue. Lives in the issue panel. */
export function PlanSection({ issue, plan, plans, issues, milestones, onSaved }: {
  issue: Issue;
  plan?: IssuePlan;
  plans: Record<string, IssuePlan>;
  issues: Issue[];
  milestones: Milestone[];
  onSaved: (p: IssuePlan) => void;
}) {
  const [p, setP] = useState<IssuePlan>(plan ?? emptyPlan());
  const [err, setErr] = useState("");
  const [saved, setSaved] = useState(false);
  const [hours, setHours] = useState("");
  const [note, setNote] = useState("");
  useEffect(() => { setP(plan ?? emptyPlan()); setErr(""); }, [issue.id, plan]);
  const mine = milestones.filter((m) => !issue.project_id || m.project_id === issue.project_id);
  const candidates = issues.filter((i) => i.id !== issue.id && !wouldCycle(plans, issue.id, [i.id]));

  async function save(next = p) {
    setErr("");
    try {
      const out = await api.savePlan(issue.id, next);
      onSaved({ ...next, logged_hours: out.logged_hours ?? next.logged_hours, depends_on: out.depends_on ?? next.depends_on });
      setSaved(true);
      setTimeout(() => setSaved(false), 1500);
    } catch (e) {
      setErr((e as Error).message);
    }
  }
  async function log() {
    const h = Number(hours);
    if (!(h > 0)) return setErr("Enter the hours you worked, such as 1.5.");
    setErr("");
    try {
      const r = await api.logHours(issue.id, h);
      setHours("");
      const next = { ...p, logged_hours: r.logged_hours };
      setP(next);
      onSaved(next);
    } catch (e) {
      setErr((e as Error).message);
    }
  }
  async function toTask() {
    try {
      await makeTask({ title: `${issue.identifier} ${issue.title}`.trim(), notes: issue.description.slice(0, 1000), link: "/projects" });
      setNote("Task added to your Inbox list in Tasks.");
    } catch (e) {
      setErr((e as Error).message);
    }
  }
  const ts = timeSummary(p);
  return (
    <Box sx={{ mt: 2 }} data-testid="plan-section">
      <Typography level="title-sm" sx={{ mb: 1 }}>Plan</Typography>
      <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap" }}>
        <FormControl size="sm" sx={{ flex: 1, minWidth: 130 }}>
          <FormLabel>Start</FormLabel>
          <Input type="date" value={p.start_date} onChange={(e) => setP({ ...p, start_date: e.target.value })} slotProps={{ input: { "data-testid": "plan-start" } }} />
        </FormControl>
        <FormControl size="sm" sx={{ flex: 1, minWidth: 130 }}>
          <FormLabel>Due</FormLabel>
          <Input type="date" value={p.due_date} onChange={(e) => setP({ ...p, due_date: e.target.value })} slotProps={{ input: { "data-testid": "plan-due" } }} />
        </FormControl>
      </Box>
      <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", mt: 1 }}>
        <FormControl size="sm" sx={{ flex: 1, minWidth: 110 }}>
          <FormLabel>Estimate (hours)</FormLabel>
          <Input type="number" value={p.estimate_hours ?? ""} onChange={(e) => setP({ ...p, estimate_hours: e.target.value === "" ? null : Number(e.target.value) })} slotProps={{ input: { min: 0, step: 0.5, "data-testid": "plan-estimate" } }} />
        </FormControl>
        <FormControl size="sm" sx={{ flex: 1, minWidth: 150 }}>
          <FormLabel>Milestone</FormLabel>
          <Select value={p.milestone_id || null} placeholder="None" onChange={(_, v) => setP({ ...p, milestone_id: v ?? "" })}>
            <Option value="">None</Option>
            {mine.map((m) => <Option key={m.id} value={m.id}>{m.name}</Option>)}
          </Select>
        </FormControl>
      </Box>
      <FormControl size="sm" sx={{ mt: 1 }}>
        <FormLabel>Waits for</FormLabel>
        <Select multiple value={p.depends_on} placeholder="No prerequisites" onChange={(_, v) => setP({ ...p, depends_on: v as string[] })} slotProps={{ button: { "data-testid": "plan-deps" } as never }}>
          {candidates.map((i) => <Option key={i.id} value={i.id}>{i.identifier} {i.title.slice(0, 50)}</Option>)}
        </Select>
      </FormControl>
      <Box sx={{ display: "flex", gap: 1, alignItems: "center", mt: 1.5, flexWrap: "wrap" }}>
        <Button size="sm" onClick={() => save()} data-testid="plan-save">{saved ? "Saved" : "Save plan"}</Button>
        <Button size="sm" variant="outlined" color="neutral" onClick={toTask} data-testid="issue-make-task">Make a task</Button>
        {ts.text && <Chip size="sm" variant="soft" color={ts.over ? "danger" : "neutral"} data-testid="plan-time">{ts.text}</Chip>}
      </Box>
      <Box sx={{ display: "flex", gap: 1, mt: 1, alignItems: "center" }}>
        <Input size="sm" type="number" placeholder="Log hours" value={hours} onChange={(e) => setHours(e.target.value)} sx={{ width: 120 }} slotProps={{ input: { min: 0, step: 0.25, "data-testid": "plan-log-hours" } }} />
        <Button size="sm" variant="outlined" color="neutral" onClick={log} data-testid="plan-log">Log time</Button>
      </Box>
      {note && <Typography level="body-xs" sx={{ mt: 0.5 }}>{note}</Typography>}
      {err && <Alert color="danger" sx={{ mt: 1 }} data-testid="plan-error">{err}</Alert>}
    </Box>
  );
}

/** Brief, milestones and links of one project. */
export function ProjectDialog({ project, issues, plans, onClose, onChanged }: { project: Project; issues: Issue[]; plans: Record<string, IssuePlan>; onClose: () => void; onChanged: () => void }) {
  return (
    <Modal open onClose={() => { onChanged(); onClose(); }}>
      <ModalDialog sx={{ width: "min(720px, 96vw)", maxHeight: "92vh", overflow: "hidden", display: "flex", flexDirection: "column" }} data-testid="project-dialog">
        <ModalClose />
        <Typography level="title-lg">{project.name}</Typography>
        <Tabs defaultValue={0} sx={{ bgcolor: "transparent", minHeight: 0, flex: 1, overflow: "hidden" }}>
          <TabList><Tab>Brief</Tab><Tab>Milestones</Tab><Tab>Links</Tab><Tab data-testid="tab-exchange">Export / import</Tab></TabList>
          <TabPanel value={0} sx={{ overflowY: "auto" }}><BriefTab pid={project.id} /></TabPanel>
          <TabPanel value={1} sx={{ overflowY: "auto" }}><MilestonesTab pid={project.id} issues={issues.filter((i) => i.project_id === project.id)} plans={plans} onChanged={onChanged} /></TabPanel>
          <TabPanel value={2} sx={{ overflowY: "auto" }}><LinksTab pid={project.id} /></TabPanel>
          <TabPanel value={3} sx={{ overflowY: "auto" }}><ExchangeTab project={project} issues={issues} plans={plans} onChanged={onChanged} /></TabPanel>
        </Tabs>
      </ModalDialog>
    </Modal>
  );
}

function BriefTab({ pid }: { pid: string }) {
  const [body, setBody] = useState<string | null>(null);
  const [msg, setMsg] = useState("");
  useEffect(() => { api.getBrief(pid).then(setBody).catch((e: Error) => { setMsg(e.message); setBody(""); }); }, [pid]);
  if (body === null) return null;
  return (
    <Box>
      <Typography level="body-sm" sx={{ mb: 1 }}>The one-page summary of this project: goal, scope, who is involved, risks.</Typography>
      <Textarea minRows={12} value={body} onChange={(e) => setBody(e.target.value)} slotProps={{ textarea: { "data-testid": "brief-text" } }} />
      <Box sx={{ display: "flex", gap: 1, alignItems: "center", mt: 1 }}>
        <Button size="sm" data-testid="brief-save" onClick={async () => { try { await api.saveBrief(pid, body); setMsg("Saved."); } catch (e) { setMsg((e as Error).message); } }}>Save brief</Button>
        {msg && <Typography level="body-xs">{msg}</Typography>}
      </Box>
    </Box>
  );
}

function MilestonesTab({ pid, issues, plans, onChanged }: { pid: string; issues: Issue[]; plans: Record<string, IssuePlan>; onChanged: () => void }) {
  const [list, setList] = useState<Milestone[]>([]);
  const [name, setName] = useState("");
  const [due, setDue] = useState("");
  const [err, setErr] = useState("");
  const load = useCallback(() => api.listMilestones(pid).then(setList).catch(() => setList([])), [pid]);
  useEffect(() => { void load(); }, [load]);
  async function add() {
    setErr("");
    try {
      await api.createMilestone(pid, { name, due_date: due, done: false });
      setName("");
      setDue("");
      await load();
      onChanged();
    } catch (e) {
      setErr((e as Error).message);
    }
  }
  return (
    <Box>
      {list.map((m) => {
        const pr = milestoneProgress(m.id, issues, plans);
        return (
          <Box key={m.id} sx={{ display: "flex", gap: 1, alignItems: "center", py: 0.75, borderBottom: "1px solid", borderColor: "divider" }} data-testid="milestone-row">
            <Checkbox size="sm" checked={m.done} onChange={async (e) => { await api.updateMilestone({ ...m, done: e.target.checked }).catch((x: Error) => setErr(x.message)); await load(); onChanged(); }} aria-label={`${m.name} done`} />
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <Typography level="title-sm" sx={{ textDecoration: m.done ? "line-through" : "none" }}>{m.name}</Typography>
              <Typography level="body-xs">{m.due_date || "No date"} · {pr.total ? `${pr.done}/${pr.total} issues done` : "no issues yet"}</Typography>
            </Box>
            <IconButton size="sm" variant="plain" color="danger" aria-label={`Delete ${m.name}`} onClick={async () => { await api.deleteMilestone(m.id).catch((x: Error) => setErr(x.message)); await load(); onChanged(); }}><DeleteIcon fontSize="small" /></IconButton>
          </Box>
        );
      })}
      {list.length === 0 && <Typography level="body-sm">No milestones yet.</Typography>}
      <Box sx={{ display: "flex", gap: 1, mt: 2, flexWrap: "wrap" }}>
        <Input placeholder="Milestone name" value={name} onChange={(e) => setName(e.target.value)} sx={{ flex: 1, minWidth: 160 }} slotProps={{ input: { "data-testid": "milestone-name" } }} />
        <Input type="date" value={due} onChange={(e) => setDue(e.target.value)} slotProps={{ input: { "data-testid": "milestone-due" } }} />
        <Button onClick={add} data-testid="milestone-add">Add</Button>
      </Box>
      {err && <Alert color="danger" sx={{ mt: 1 }}>{err}</Alert>}
    </Box>
  );
}

function LinksTab({ pid }: { pid: string }) {
  const [links, setLinks] = useState<api.Link[]>([]);
  const [kind, setKind] = useState<"crm_deal" | "hr_employee">("crm_deal");
  const [q, setQ] = useState("");
  const [items, setItems] = useState<{ id: string; label: string }[]>([]);
  const [err, setErr] = useState("");
  const load = useCallback(() => api.listLinks(pid).then(setLinks).catch(() => setLinks([])), [pid]);
  useEffect(() => { void load(); }, [load]);
  useEffect(() => {
    const t = setTimeout(() => api.pick(kind, q).then(setItems).catch(() => setItems([])), 200);
    return () => clearTimeout(t);
  }, [kind, q]);
  return (
    <Box>
      {links.map((l) => (
        <Box key={l.id} sx={{ display: "flex", gap: 1, alignItems: "center", py: 0.5, borderBottom: "1px solid", borderColor: "divider" }} data-testid="link-row">
          <Chip size="sm" variant="soft">{l.kind === "crm_deal" ? "CRM deal" : "Person (HR)"}</Chip>
          <Typography level="body-sm" sx={{ flex: 1 }}>{l.label}</Typography>
          <IconButton size="sm" variant="plain" color="danger" aria-label={`Remove link ${l.label}`} onClick={async () => { await api.deleteLink(l.id).catch((x: Error) => setErr(x.message)); await load(); }}><DeleteIcon fontSize="small" /></IconButton>
        </Box>
      ))}
      {links.length === 0 && <Typography level="body-sm">Nothing linked yet.</Typography>}
      <Typography level="title-sm" sx={{ mt: 2 }}>Add a link</Typography>
      <Box sx={{ display: "flex", gap: 1, mt: 0.5, flexWrap: "wrap" }}>
        <Select size="sm" value={kind} onChange={(_, v) => setKind((v ?? "crm_deal") as "crm_deal" | "hr_employee")}>
          <Option value="crm_deal">CRM deal</Option>
          <Option value="hr_employee">Person (HR)</Option>
        </Select>
        <Input size="sm" placeholder="Search" value={q} onChange={(e) => setQ(e.target.value)} sx={{ flex: 1, minWidth: 140 }} />
      </Box>
      <Box sx={{ mt: 1, maxHeight: 180, overflowY: "auto" }}>
        {items.map((it) => (
          <Box key={it.id} sx={{ display: "flex", alignItems: "center", gap: 1, py: 0.25 }}>
            <Typography level="body-sm" sx={{ flex: 1 }}>{it.label}</Typography>
            <Button size="sm" variant="outlined" color="neutral" onClick={async () => { setErr(""); try { await api.addLink(pid, kind, it.id); await load(); } catch (e) { setErr((e as Error).message); } }}>Link</Button>
          </Box>
        ))}
        {items.length === 0 && <Typography level="body-xs">Nothing found.</Typography>}
      </Box>
      {err && <Alert color="danger" sx={{ mt: 1 }}>{err}</Alert>}
    </Box>
  );
}

/** Create a project, with issues, dates and dependencies, from a ready-made plan. */
export function TemplateDialog({ teams, defaultTeam, onClose, onCreated }: { teams: Team[]; defaultTeam: string; onClose: () => void; onCreated: (id: string) => void }) {
  const [list, setList] = useState<api.TemplateInfo[]>([]);
  const [tpl, setTpl] = useState("");
  const [name, setName] = useState("");
  const [team, setTeam] = useState(defaultTeam);
  const [start, setStart] = useState(() => new Date().toISOString().slice(0, 10));
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  useEffect(() => { api.templates().then((t) => { setList(t); if (t[0]) setTpl(t[0].id); }).catch(() => setList([])); }, []);
  const sel = list.find((t) => t.id === tpl);
  async function go() {
    setBusy(true);
    setErr("");
    try {
      const r = await api.fromTemplate({ template: tpl, name: name.trim(), team_id: team, start_date: start });
      onCreated(r.id);
      onClose();
    } catch (e) {
      setErr((e as Error).message);
      setBusy(false);
    }
  }
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: "min(520px, 96vw)" }} data-testid="template-dialog">
        <ModalClose />
        <Typography level="title-lg">New project from a template</Typography>
        <FormControl>
          <FormLabel>Template</FormLabel>
          <Select value={tpl || null} onChange={(_, v) => setTpl(v ?? "")} slotProps={{ button: { "data-testid": "template-select" } as never }}>
            {list.map((t) => <Option key={t.id} value={t.id}>{t.name}</Option>)}
          </Select>
          {sel && <Typography level="body-xs" sx={{ mt: 0.5 }}>{sel.description} {sel.issues} issues, {sel.milestones} milestones.</Typography>}
        </FormControl>
        <FormControl><FormLabel>Project name</FormLabel><Input value={name} onChange={(e) => setName(e.target.value)} slotProps={{ input: { "data-testid": "template-name" } }} /></FormControl>
        <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap" }}>
          <FormControl sx={{ flex: 1, minWidth: 150 }}>
            <FormLabel>Team for the issues</FormLabel>
            <Select value={team || null} onChange={(_, v) => setTeam(v ?? "")}>
              {teams.map((t) => <Option key={t.id} value={t.id}>{t.name}</Option>)}
            </Select>
          </FormControl>
          <FormControl sx={{ flex: 1, minWidth: 150 }}>
            <FormLabel>Start date</FormLabel>
            <Input type="date" value={start} onChange={(e) => setStart(e.target.value)} />
          </FormControl>
        </Box>
        {err && <Alert color="danger">{err}</Alert>}
        <Button loading={busy} disabled={!name.trim() || !tpl || !team} onClick={go} data-testid="template-create">Create project</Button>
      </ModalDialog>
    </Modal>
  );
}
