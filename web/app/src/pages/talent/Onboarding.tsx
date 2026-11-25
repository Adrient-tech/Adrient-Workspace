/* eslint-disable @typescript-eslint/no-explicit-any */
import { PersonSelect } from "../../components/PeoplePicker";
import { useCallback, useEffect, useState } from "react";
import { Box, Button, Checkbox, Chip, FormControl, FormLabel, Input, Option, Select, Table, Textarea, Typography } from "@mui/joy";
import * as api from "./api";
import { ACCENT, dueLabel } from "./logic";
import { Empty, ErrorAlert, Section, useErr } from "./ui";
import type { Ctx } from "./ui";

const ROLES = ["hr", "it", "manager", "employee"];

export function OnboardingView({ ctx }: { ctx: Ctx }) {
  const [lists, setLists] = useState<any[]>([]);
  const [mine, setMine] = useState<any[]>([]);
  const [open, setOpen] = useState("");
  const [tpls, setTpls] = useState<any[]>([]);
  const { err, run } = useErr();
  const load = useCallback(() => run(async () => {
    setLists(await api.listChecklists());
    setMine(await api.myChecklistTasks());
    if (ctx.me.is_hr) setTpls(await api.listTemplates());
  }), [run, ctx.me.is_hr]);
  useEffect(() => { load(); }, [load]);

  if (open) return <ChecklistDetail ctx={ctx} id={open} onBack={() => { setOpen(""); load(); }} />;
  return (
    <>
      <ErrorAlert err={err} />
      <Section title="My open tasks">
        {mine.length === 0 ? <Typography level="body-sm">Nothing assigned to you.</Typography> : mine.map((t) => {
          const d = dueLabel(t.due_date);
          return (
            <Box key={t.id} sx={{ display: "flex", gap: 1, alignItems: "center" }} data-testid="my-task">
              <Checkbox onChange={() => run(async () => { await api.setTaskDone(t.id, true); await load(); })} label={`${t.title} (${t.subject_name})`} />
              <Chip size="sm" color={d.overdue ? "danger" : "neutral"} variant="soft">{d.text}</Chip>
              {t.link && <Button size="sm" variant="plain" component="a" href={t.link}>Open</Button>}
            </Box>
          );
        })}
      </Section>
      <Section title="Onboarding and offboarding" actions={ctx.me.is_hr && <><Button size="sm" variant="outlined" onClick={() => run(async () => { const r = await api.remindOverdue(); alert(`${r.sent} reminder${r.sent === 1 ? "" : "s"} sent`); })}>Send overdue reminders</Button></>}>
        {lists.length === 0 && <Empty>No checklists yet.</Empty>}
        {lists.length > 0 && (
          <Table size="sm" data-testid="checklists">
            <thead><tr><th>Person</th><th>Type</th><th>Date</th><th>Progress</th><th /></tr></thead>
            <tbody>
              {lists.map((c) => (
                <tr key={c.id}>
                  <td>{c.subject_name}</td><td>{c.kind}</td><td>{String(c.anchor_date).slice(0, 10)}</td>
                  <td>{c.done}/{c.total} {c.overdue > 0 && <Chip size="sm" color="danger" variant="soft">{c.overdue} overdue</Chip>}</td>
                  <td><Button size="sm" variant="soft" onClick={() => setOpen(c.id)} data-testid="checklist-open">Open</Button></td>
                </tr>
              ))}
            </tbody>
          </Table>
        )}
      </Section>
      {ctx.me.is_hr && <Start ctx={ctx} tpls={tpls} onStarted={(id) => { load(); setOpen(id); }} />}
      {ctx.me.is_hr && <Templates tpls={tpls} onChange={load} />}
    </>
  );
}

function Start({ ctx, tpls, onStarted }: { ctx: Ctx; tpls: any[]; onStarted: (id: string) => void }) {
  const [f, setF] = useState({ template_id: "", subject_user_id: "", subject_name: "", subject_email: "", anchor_date: new Date().toISOString().slice(0, 10), it_id: "", manager_id: "" });
  const { err, run } = useErr();
  return (
    <Section title="Start a checklist">
      <ErrorAlert err={err} />
      <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", alignItems: "end" }}>
        <FormControl><FormLabel>Template</FormLabel><Select value={f.template_id} onChange={(_, v) => setF({ ...f, template_id: v ?? "" })} sx={{ minWidth: 200 }} slotProps={{ button: { "data-testid": "start-template" } }}>{tpls.map((t) => <Option key={t.id} value={t.id}>{t.name} ({t.kind})</Option>)}</Select></FormControl>
        <FormControl><FormLabel>Existing user</FormLabel><PersonSelect value={f.subject_user_id} known={ctx.people} onChange={(id) => setF({ ...f, subject_user_id: id })} placeholder="New hire (not a user yet)" ariaLabel="Person" /></FormControl>
        {!f.subject_user_id && <FormControl><FormLabel>Name</FormLabel><Input value={f.subject_name} onChange={(e) => setF({ ...f, subject_name: e.target.value })} slotProps={{ input: { "data-testid": "start-name" } }} /></FormControl>}
        {!f.subject_user_id && <FormControl><FormLabel>Email</FormLabel><Input value={f.subject_email} onChange={(e) => setF({ ...f, subject_email: e.target.value })} /></FormControl>}
        <FormControl><FormLabel>Manager</FormLabel><PersonSelect value={f.manager_id} known={ctx.people} onChange={(id) => setF({ ...f, manager_id: id })} placeholder="From directory" ariaLabel="Person" /></FormControl>
        <FormControl><FormLabel>IT owner</FormLabel><PersonSelect value={f.it_id} known={ctx.people} onChange={(id) => setF({ ...f, it_id: id })} placeholder="Me" ariaLabel="Person" /></FormControl>
        <FormControl><FormLabel>Start / last day</FormLabel><Input type="date" value={f.anchor_date} onChange={(e) => setF({ ...f, anchor_date: e.target.value })} /></FormControl>
        <Button sx={{ bgcolor: ACCENT }} onClick={() => run(async () => { const c = await api.startChecklist(f); onStarted(c.id); })} data-testid="start-go">Start</Button>
      </Box>
    </Section>
  );
}

function ChecklistDetail({ ctx, id, onBack }: { ctx: Ctx; id: string; onBack: () => void }) {
  const [c, setC] = useState<any>(null);
  const [asset, setAsset] = useState({ name: "", serial: "" });
  const [exit, setExit] = useState({ reason: "", notes: "" });
  const { err, run } = useErr();
  const load = useCallback(() => run(async () => setC(await api.getChecklist(id))), [id, run]);
  useEffect(() => { load(); }, [load]);
  const upd = (fn: () => Promise<any>) => run(async () => setC(await fn()));
  if (!c) return <ErrorAlert err={err} />;
  return (
    <Section title={`${c.kind === "offboarding" ? "Offboarding" : "Onboarding"}: ${c.subject_name}`} actions={<Button size="sm" variant="plain" onClick={onBack}>Back</Button>}>
      <ErrorAlert err={err} />
      <Table size="sm" data-testid="checklist-tasks">
        <thead><tr><th style={{ width: 40 }} /><th>Task</th><th>Owner</th><th>Due</th><th /></tr></thead>
        <tbody>
          {c.tasks.map((t: any) => {
            const d = dueLabel(t.due_date);
            return (
              <tr key={t.id}>
                <td><Checkbox checked={t.done} onChange={(e) => upd(() => api.setTaskDone(t.id, e.target.checked))} slotProps={{ input: { "data-testid": "task-check" } as any }} /></td>
                <td style={{ textDecoration: t.done ? "line-through" : "none" }}>{t.title}</td>
                <td>{ROLES.includes(t.assignee_role) ? t.assignee_role : ""}{t.assignee_name ? `: ${t.assignee_name}` : ""}</td>
                <td>{t.done ? "Done" : <Chip size="sm" color={d.overdue ? "danger" : "neutral"} variant="soft">{d.text}</Chip>}</td>
                <td>{t.link && <Button size="sm" variant="plain" component="a" href={t.link} data-testid="task-link">Open</Button>}</td>
              </tr>
            );
          })}
        </tbody>
      </Table>
      <Typography level="title-md" sx={{ mt: 2 }}>Assets</Typography>
      {c.assets.map((a: any) => (
        <Box key={a.id} sx={{ display: "flex", gap: 1, alignItems: "center" }}>
          <Typography level="body-sm">{a.name} {a.serial && `(${a.serial})`}</Typography>
          <Select size="sm" value={a.status} onChange={(_, v) => v && upd(() => api.setAssetStatus(a.id, v))}>{["assigned", "returned", "lost"].map((s) => <Option key={s} value={s}>{s}</Option>)}</Select>
        </Box>
      ))}
      <Box sx={{ display: "flex", gap: 1, mt: 0.5 }}>
        <Input size="sm" placeholder="Asset (laptop, badge...)" value={asset.name} onChange={(e) => setAsset({ ...asset, name: e.target.value })} />
        <Input size="sm" placeholder="Serial" value={asset.serial} onChange={(e) => setAsset({ ...asset, serial: e.target.value })} />
        <Button size="sm" onClick={() => upd(async () => { const r = await api.addAsset(id, asset); setAsset({ name: "", serial: "" }); return r; })}>Add</Button>
      </Box>
      {c.kind === "offboarding" && ctx.me.is_hr && (
        <>
          <Typography level="title-md" sx={{ mt: 2 }}>Exit interview (HR only)</Typography>
          {c.exit_interview && <Typography level="body-sm">Recorded: {c.exit_interview.reason}</Typography>}
          <Input size="sm" placeholder="Main reason for leaving" value={exit.reason} onChange={(e) => setExit({ ...exit, reason: e.target.value })} sx={{ my: 0.5 }} />
          <Textarea minRows={2} placeholder="Notes" value={exit.notes} onChange={(e) => setExit({ ...exit, notes: e.target.value })} />
          <Button size="sm" sx={{ mt: 0.5 }} onClick={() => upd(() => api.saveExit(id, exit))}>Save exit interview</Button>
        </>
      )}
    </Section>
  );
}

function Templates({ tpls, onChange }: { tpls: any[]; onChange: () => void }) {
  const [f, setF] = useState({ name: "", kind: "onboarding", text: "" });
  const { err, run } = useErr();
  // One task per line: title | role | due offset days (negative = before) | optional link
  const parse = () => f.text.split("\n").map((l) => l.trim()).filter(Boolean).map((l) => {
    const [title, role, off, link] = l.split("|").map((x) => x.trim());
    return { title, role: role || "hr", due_offset_days: Number(off || 0), link: link || "" };
  });
  return (
    <Section title="Checklist templates">
      <ErrorAlert err={err} />
      {tpls.map((t) => (
        <Box key={t.id} sx={{ display: "flex", gap: 1, alignItems: "center" }} data-testid="template-row">
          <Typography level="body-sm">{t.name} ({t.kind}, {t.tasks.length} tasks)</Typography>
          <Button size="sm" variant="plain" color="danger" onClick={() => run(async () => { await api.deleteTemplate(t.id); onChange(); })}>Delete</Button>
        </Box>
      ))}
      <Box sx={{ display: "grid", gap: 1, maxWidth: 640, mt: 1 }}>
        <Box sx={{ display: "flex", gap: 1 }}>
          <Input placeholder="Template name" value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} slotProps={{ input: { "data-testid": "tpl-name" } }} />
          <Select value={f.kind} onChange={(_, v) => setF({ ...f, kind: v ?? "onboarding" })}><Option value="onboarding">Onboarding</Option><Option value="offboarding">Offboarding</Option></Select>
        </Box>
        <Textarea minRows={5} value={f.text} onChange={(e) => setF({ ...f, text: e.target.value })} slotProps={{ textarea: { "data-testid": "tpl-tasks" } }}
          placeholder={"One task per line: title | role (hr, it, manager, employee) | days from start (negative = before)\nOrder laptop | it | -5\nWelcome lunch | manager | 0\nSign handbook | employee | 1"} />
        <Button sx={{ bgcolor: ACCENT, width: 160 }} onClick={() => run(async () => { await api.saveTemplate("", { name: f.name, kind: f.kind, tasks: parse() }); setF({ name: "", kind: f.kind, text: "" }); onChange(); })} data-testid="tpl-save">Save template</Button>
      </Box>
    </Section>
  );
}
