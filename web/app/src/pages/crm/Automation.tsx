import { PersonSelect, PeopleSelect } from "../../components/PeoplePicker";
import { useCallback, useEffect, useState } from "react";
import { Box, Button, Checkbox, Chip, FormControl, FormLabel, IconButton, Input, Option, Select, Sheet, Stack, Tab, TabList, TabPanel, Table, Tabs, Textarea, Typography } from "@mui/joy";
import * as api from "./api";
import * as api2 from "./api2";
import { allStages } from "./logic";
import { ConfirmButton, Dialog, Empty, ErrorAlert, useErr } from "./ui";
import type { Data } from "./ui";

interface Action {
  type: "create_followup" | "assign_owner" | "send_template" | "notify_chat";
  title?: string;
  due_in_days?: number;
  users?: string[];
  team_id?: string;
  template_id?: string;
  to?: string;
  text?: string;
}
interface Rule {
  id: string;
  name: string;
  enabled: boolean;
  trigger: string;
  conditions: Record<string, unknown>;
  actions: Action[];
}
interface Run {
  rule_id: string;
  rule_name: string;
  event_key: string;
  result: string[] | null;
  ran_at: string;
}
interface Playbook {
  id: string;
  stage: string;
  name: string;
  items: string[];
  required: boolean;
}
interface ScoreRule {
  id: string;
  name: string;
  field: string;
  op: string;
  value: string;
  points: number;
}
interface Template {
  id: string;
  name: string;
}
interface Team {
  id: string;
  name: string;
}

const TRIGGERS: Record<string, string> = {
  stage_changed: "A deal changes stage",
  deal_idle: "A deal is idle for N days",
  form_submitted: "A form is submitted",
  email_received: "An email arrives from a lead",
  lead_created: "A new lead is created",
};
const ACTIONS: Record<Action["type"], string> = {
  create_followup: "Create a follow-up",
  assign_owner: "Assign owner (round robin)",
  send_template: "Send a mail template",
  notify_chat: "Notify in Chat",
};

/** Checklist of the stage playbook on a deal. */
export function ChecklistPanel({ dealId, stage, stageName, onChanged }: { dealId: string; stage: string; stageName: string; onChanged: () => void }) {
  const [items, setItems] = useState<{ stage: string; position: number; text: string; done: boolean }[]>([]);
  const { err, run } = useErr();
  const load = useCallback(() => {
    void api2.get<{ items: typeof items }>(`deals/${dealId}/checklist`).then((r) => setItems(r.items)).catch(() => undefined);
  }, [dealId]);
  useEffect(load, [load, stage]);
  if (items.length === 0) return null;
  return (
    <Box data-testid="crm-checklist">
      <Typography level="title-sm">Playbook checklist</Typography>
      {Array.from(new Set(items.map((i) => i.stage))).map((st) => (
        <Box key={st} sx={{ mb: 0.5 }}>
          <Typography level="body-xs">{st === stage ? `${stageName} (current)` : st}</Typography>
          {items.filter((i) => i.stage === st).map((i) => (
            <Box key={i.position}>
            <Checkbox
              size="sm"
              label={i.text}
              checked={i.done}
              data-testid={`crm-check-${st}-${i.position}`}
              onChange={async (e) => {
                if (await run(() => api2.post(`deals/${dealId}/checklist`, { stage: st, position: i.position, done: e.target.checked }))) {
                  load();
                  onChanged();
                }
              }}
            />
            </Box>
          ))}
        </Box>
      ))}
      <ErrorAlert err={err} />
    </Box>
  );
}

export function Automation({ data }: { data: Data }) {
  return (
    <Box>
      <Typography level="h4" sx={{ mb: 1 }}>
        Automation
      </Typography>
      {!data.me.is_admin && <Typography level="body-sm" sx={{ mb: 1 }}>Only a CRM administrator can change automation. You can see what is set up.</Typography>}
      <Tabs defaultValue="rules" sx={{ bgcolor: "transparent" }}>
        <TabList>
          <Tab value="rules" data-testid="crm-auto-tab-rules">Workflow rules</Tab>
          <Tab value="playbooks" data-testid="crm-auto-tab-playbooks">Playbooks</Tab>
          <Tab value="scoring" data-testid="crm-auto-tab-scoring">Lead scoring</Tab>
          <Tab value="sla" data-testid="crm-auto-tab-sla">SLA timers</Tab>
        </TabList>
        <TabPanel value="rules"><Rules data={data} /></TabPanel>
        <TabPanel value="playbooks"><Playbooks data={data} /></TabPanel>
        <TabPanel value="scoring"><Scoring data={data} /></TabPanel>
        <TabPanel value="sla"><SLA data={data} /></TabPanel>
      </Tabs>
    </Box>
  );
}

function Rules({ data }: { data: Data }) {
  const [rules, setRules] = useState<Rule[]>([]);
  const [runs, setRuns] = useState<Run[]>([]);
  const [editing, setEditing] = useState<Rule | "new" | null>(null);
  const [note, setNote] = useState("");
  const { err, run } = useErr();
  const load = useCallback(() => {
    void run(() => api2.list<Rule>("rules", "rules")).then((r) => r && setRules(r));
    void api2.list<Run>("rule-runs", "runs").then(setRuns).catch(() => undefined);
  }, [run]);
  useEffect(load, [load]);
  return (
    <Stack gap={1.5}>
      <Stack direction="row" gap={1} alignItems="center">
        <Typography level="body-sm" sx={{ flex: 1 }}>Each rule runs once per event. Retries and repeated sweeps never run it twice.</Typography>
        {data.me.is_admin && (
          <>
            <Button size="sm" variant="outlined" data-testid="crm-run-sweep" onClick={async () => {
              const r = await run(() => api2.post<{ fired: number }>("automation/sweep"));
              if (r) { setNote(`Idle sweep done: ${r.fired} rule run(s).`); load(); }
            }}>
              Run idle check now
            </Button>
            <Button size="sm" onClick={() => setEditing("new")} data-testid="crm-new-rule">New rule</Button>
          </>
        )}
      </Stack>
      {note && <Typography level="body-sm" color="success" data-testid="crm-sweep-note">{note}</Typography>}
      <ErrorAlert err={err} />
      {rules.length === 0 ? <Empty>No rules yet.</Empty> : (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table hoverRow data-testid="crm-rules">
            <thead><tr><th>Rule</th><th>When</th><th>Then</th><th>On</th></tr></thead>
            <tbody>
              {rules.map((r) => (
                <tr key={r.id} onClick={() => data.me.is_admin && setEditing(r)} style={{ cursor: data.me.is_admin ? "pointer" : undefined }}>
                  <td>{r.name}</td>
                  <td>{TRIGGERS[r.trigger] ?? r.trigger}{r.conditions.stage ? ` → ${String(r.conditions.stage)}` : ""}{r.conditions.idle_days ? ` (${String(r.conditions.idle_days)} days)` : ""}</td>
                  <td>{r.actions.map((a) => ACTIONS[a.type]).join(", ")}</td>
                  <td>{r.enabled ? "Yes" : "No"}</td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
      <Typography level="title-sm">Recent runs</Typography>
      {runs.length === 0 ? <Typography level="body-sm">Nothing has run yet.</Typography> : (
        <Stack gap={0.25} data-testid="crm-rule-runs">
          {runs.slice(0, 12).map((r) => (
            <Typography key={r.rule_id + r.event_key} level="body-xs">
              {new Date(r.ran_at).toLocaleString()} · {r.rule_name} · {(r.result ?? []).join("; ")}
            </Typography>
          ))}
        </Stack>
      )}
      {editing && <RuleDialog data={data} rule={editing === "new" ? null : editing} onClose={() => setEditing(null)} onSaved={() => { setEditing(null); load(); }} />}
    </Stack>
  );
}

function RuleDialog({ data, rule, onClose, onSaved }: { data: Data; rule: Rule | null; onClose: () => void; onSaved: () => void }) {
  const [f, setF] = useState<Rule>(rule ?? { id: "", name: "", enabled: true, trigger: "stage_changed", conditions: {}, actions: [{ type: "create_followup", title: "Follow up: {{deal.title}}", due_in_days: 2 }] });
  const [templates, setTemplates] = useState<Template[]>([]);
  const [teams, setTeams] = useState<Team[]>([]);
  const { err, run } = useErr();
  useEffect(() => {
    void api2.list<Template>("templates", "templates").then(setTemplates).catch(() => undefined);
    void api2.list<Team>("teams", "teams").then(setTeams).catch(() => undefined);
  }, []);
  const cond = (k: string, v: unknown) => setF((p) => ({ ...p, conditions: { ...p.conditions, [k]: v } }));
  const act = (i: number, patch: Partial<Action>) => setF((p) => ({ ...p, actions: p.actions.map((a, j) => (j === i ? { ...a, ...patch } : a)) }));
  return (
    <Dialog title={rule ? "Edit rule" : "New rule"} onClose={onClose} width={640}>
      <Stack gap={1.5}>
        <FormControl required>
          <FormLabel>Name</FormLabel>
          <Input value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} slotProps={{ input: { "data-testid": "crm-rule-name" } }} />
        </FormControl>
        <FormControl>
          <FormLabel>When</FormLabel>
          <Select value={f.trigger} onChange={(_, v) => setF({ ...f, trigger: v ?? "stage_changed", conditions: {} })} slotProps={{ button: { "data-testid": "crm-rule-trigger" } as never }}>
            {Object.entries(TRIGGERS).map(([k, l]) => <Option key={k} value={k}>{l}</Option>)}
          </Select>
        </FormControl>
        {(f.trigger === "stage_changed" || f.trigger === "deal_idle") && (
          <FormControl>
            <FormLabel>Stage (optional)</FormLabel>
            <Select value={String(f.conditions.stage ?? "")} onChange={(_, v) => cond("stage", v ?? "")} slotProps={{ button: { "data-testid": "crm-rule-stage" } as never }}>
              <Option value="">Any stage</Option>
              {allStages(data.settings).map((s) => <Option key={s.id} value={s.id}>{s.name}</Option>)}
            </Select>
          </FormControl>
        )}
        {f.trigger === "deal_idle" && (
          <FormControl required>
            <FormLabel>Idle for (days)</FormLabel>
            <Input type="number" value={Number(f.conditions.idle_days ?? 7)} onChange={(e) => cond("idle_days", Number(e.target.value))} slotProps={{ input: { min: 1, "data-testid": "crm-rule-idle" } }} />
          </FormControl>
        )}
        {f.trigger === "form_submitted" && (
          <FormControl><FormLabel>Form id (optional)</FormLabel><Input value={String(f.conditions.form_id ?? "")} onChange={(e) => cond("form_id", e.target.value)} /></FormControl>
        )}
        {f.trigger === "email_received" && (
          <FormControl><FormLabel>From domain (optional)</FormLabel><Input placeholder="example.com" value={String(f.conditions.from_domain ?? "")} onChange={(e) => cond("from_domain", e.target.value)} /></FormControl>
        )}
        {f.trigger === "lead_created" && (
          <FormControl><FormLabel>Source (optional)</FormLabel><Input value={String(f.conditions.source ?? "")} onChange={(e) => cond("source", e.target.value)} /></FormControl>
        )}
        <Typography level="title-sm">Then</Typography>
        {f.actions.map((a, i) => (
          <Sheet key={i} variant="soft" sx={{ p: 1, borderRadius: "md" }} data-testid={`crm-rule-action-${i}`}>
            <Stack gap={1}>
              <Stack direction="row" gap={1}>
                <Select size="sm" value={a.type} onChange={(_, v) => v && act(i, { type: v as Action["type"] })} sx={{ flex: 1 }} aria-label="Action type" slotProps={{ button: { "data-testid": `crm-rule-action-type-${i}` } as never }}>
                  {Object.entries(ACTIONS).map(([k, l]) => <Option key={k} value={k}>{l}</Option>)}
                </Select>
                <IconButton size="sm" aria-label="Remove action" onClick={() => setF({ ...f, actions: f.actions.filter((_, j) => j !== i) })}>×</IconButton>
              </Stack>
              {a.type === "create_followup" && (
                <Stack direction="row" gap={1}>
                  <Input size="sm" sx={{ flex: 1 }} value={a.title ?? ""} placeholder="Title, e.g. Call {{person.name}}" onChange={(e) => act(i, { title: e.target.value })} slotProps={{ input: { "data-testid": `crm-rule-fu-title-${i}`, "aria-label": "Follow-up title" } }} />
                  <Input size="sm" sx={{ width: 110 }} type="number" value={a.due_in_days ?? 0} onChange={(e) => act(i, { due_in_days: Number(e.target.value) })} startDecorator="in" endDecorator="days" slotProps={{ input: { min: 0, "aria-label": "Due in days" } }} />
                </Stack>
              )}
              {a.type === "assign_owner" && (
                <Stack gap={0.5}>
                  <PeopleSelect placeholder="Users to rotate between" value={a.users ?? []} known={data.users} onChange={(ids) => act(i, { users: ids, team_id: "" })} ariaLabel="Users" testId={`crm-rule-users-${i}`} />
                  <Select size="sm" placeholder="...or a team" value={a.team_id ?? ""} onChange={(_, v) => act(i, { team_id: v ?? "", users: v ? [] : a.users })} aria-label="Team">
                    <Option value="">No team</Option>
                    {teams.map((t) => <Option key={t.id} value={t.id}>{t.name}</Option>)}
                  </Select>
                </Stack>
              )}
              {a.type === "send_template" && (
                <Select size="sm" placeholder="Mail template" value={a.template_id ?? ""} onChange={(_, v) => act(i, { template_id: v ?? "" })} aria-label="Template">
                  {templates.map((t) => <Option key={t.id} value={t.id}>{t.name}</Option>)}
                </Select>
              )}
              {a.type === "notify_chat" && (
                <Stack gap={0.5}>
                  <Select size="sm" value={a.to && a.to !== "owner" ? "person" : "owner"} onChange={(_, v) => act(i, { to: v === "person" ? " " : "owner" })} aria-label="Notify">
                    <Option value="owner">The record owner</Option>
                    <Option value="person">A specific person</Option>
                  </Select>
                  {a.to && a.to !== "owner" && (
                    <PersonSelect value={a.to.trim()} known={data.users} onChange={(id) => act(i, { to: id || " " })} placeholder="Choose who to notify" ariaLabel="Notify person" />
                  )}
                  <Input size="sm" value={a.text ?? ""} placeholder="Message, e.g. {{deal.title}} moved to {{deal.stage}}" onChange={(e) => act(i, { text: e.target.value })} slotProps={{ input: { "data-testid": `crm-rule-notify-text-${i}`, "aria-label": "Message" } }} />
                </Stack>
              )}
            </Stack>
          </Sheet>
        ))}
        <Button size="sm" variant="soft" sx={{ alignSelf: "flex-start" }} onClick={() => setF({ ...f, actions: [...f.actions, { type: "notify_chat", text: "" }] })}>Add action</Button>
        <Checkbox label="Rule is on" checked={f.enabled} onChange={(e) => setF({ ...f, enabled: e.target.checked })} />
        <ErrorAlert err={err} />
        <Stack direction="row" justifyContent="space-between">
          {rule ? <ConfirmButton label="Delete" testid="crm-rule-delete" onConfirm={async () => { if (await run(() => api2.remove("rules", rule.id))) onSaved(); }} /> : <span />}
          <Button disabled={!f.name.trim()} data-testid="crm-rule-save" onClick={async () => { if (await run(() => api2.save("rules", rule?.id ?? "", f))) onSaved(); }}>Save</Button>
        </Stack>
      </Stack>
    </Dialog>
  );
}

function Playbooks({ data }: { data: Data }) {
  const [list, setList] = useState<Playbook[]>([]);
  const [stage, setStage] = useState(data.settings.stages[0]?.id ?? "");
  const [text, setText] = useState("");
  const [required, setRequired] = useState(false);
  const { err, run } = useErr();
  const load = useCallback(() => { void run(() => api2.list<Playbook>("playbooks", "playbooks")).then((l) => l && setList(l)); }, [run]);
  useEffect(load, [load]);
  useEffect(() => {
    const p = list.find((x) => x.stage === stage);
    setText(p ? p.items.join("\n") : "");
    setRequired(p?.required ?? false);
  }, [stage, list]);
  const existing = list.find((x) => x.stage === stage);
  return (
    <Stack gap={1.5} data-testid="crm-playbooks">
      <Typography level="body-sm">A playbook is a checklist copied onto a deal when it enters a stage. Mark it required to block moving on until every item is ticked (marking a deal lost is always allowed).</Typography>
      <FormControl sx={{ maxWidth: 260 }}>
        <FormLabel>Stage</FormLabel>
        <Select value={stage} onChange={(_, v) => setStage(v ?? stage)} slotProps={{ button: { "data-testid": "crm-playbook-stage" } as never }}>
          {allStages(data.settings).filter((s) => s.id !== "lost").map((s) => <Option key={s.id} value={s.id}>{s.name}{list.some((p) => p.stage === s.id) ? " ✓" : ""}</Option>)}
        </Select>
      </FormControl>
      <FormControl>
        <FormLabel>Checklist items (one per line)</FormLabel>
        <Textarea minRows={5} value={text} onChange={(e) => setText(e.target.value)} disabled={!data.me.is_admin} slotProps={{ textarea: { "data-testid": "crm-playbook-items" } }} />
      </FormControl>
      <Checkbox label="Required before leaving this stage" checked={required} onChange={(e) => setRequired(e.target.checked)} disabled={!data.me.is_admin} data-testid="crm-playbook-required" />
      <ErrorAlert err={err} />
      {data.me.is_admin && (
        <Stack direction="row" gap={1}>
          <Button data-testid="crm-playbook-save" disabled={!text.trim()} onClick={async () => { if (await run(() => api2.save("playbooks", "", { stage, name: "", items: text.split("\n"), required }))) load(); }}>Save playbook</Button>
          {existing && <ConfirmButton label="Remove" onConfirm={async () => { if (await run(() => api2.remove("playbooks", existing.id))) load(); }} />}
        </Stack>
      )}
    </Stack>
  );
}

const SCORE_FIELDS = [["status", "Status"], ["source", "Source"], ["tag", "Tag"], ["has_email", "Has email"], ["has_phone", "Has phone"], ["has_company", "Has company"], ["activities_30d", "Activities in 30 days"]];

function Scoring({ data }: { data: Data }) {
  const [rules, setRules] = useState<ScoreRule[]>([]);
  const [f, setF] = useState({ name: "", field: "source", op: "eq", value: "", points: 10 });
  const [msg, setMsg] = useState("");
  const { err, run } = useErr();
  const load = useCallback(() => { void run(() => api2.list<ScoreRule>("score-rules", "rules")).then((l) => l && setRules(l)); }, [run]);
  useEffect(load, [load]);
  const fields = [...SCORE_FIELDS, ...data.fields.filter((d) => d.entity === "lead" || d.entity === "contact").map((d) => [`custom:${d.key}`, `${d.label} (custom)`])];
  return (
    <Stack gap={1.5} data-testid="crm-scoring">
      <Typography level="body-sm">Each matching rule adds (or subtracts) points on a lead. Scores refresh when a lead or its activity changes.</Typography>
      {rules.map((r) => (
        <Stack key={r.id} direction="row" gap={1} alignItems="center">
          <Chip size="sm" color={r.points > 0 ? "success" : "danger"} variant="soft">{r.points > 0 ? "+" : ""}{r.points}</Chip>
          <Typography level="body-sm" sx={{ flex: 1 }}>{r.name || `${r.field} ${r.op} ${r.value}`} ({r.field} {r.op} {r.value || "-"})</Typography>
          {data.me.is_admin && <ConfirmButton label="Delete" onConfirm={async () => { if (await run(() => api2.remove("score-rules", r.id))) load(); }} />}
        </Stack>
      ))}
      {data.me.is_admin && (
        <Sheet variant="soft" sx={{ p: 1.5, borderRadius: "md" }}>
          <Stack direction="row" gap={1} flexWrap="wrap" alignItems="flex-end">
            <FormControl sx={{ width: 160 }}><FormLabel>Name</FormLabel><Input size="sm" value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} /></FormControl>
            <FormControl sx={{ width: 190 }}><FormLabel>Field</FormLabel>
              <Select size="sm" value={f.field} onChange={(_, v) => setF({ ...f, field: v ?? "source" })} slotProps={{ button: { "data-testid": "crm-score-field" } as never }}>
                {fields.map(([k, l]) => <Option key={k} value={k}>{l}</Option>)}
              </Select></FormControl>
            <FormControl sx={{ width: 120 }}><FormLabel>Is</FormLabel>
              <Select size="sm" value={f.op} onChange={(_, v) => setF({ ...f, op: v ?? "eq" })} slotProps={{ button: { "data-testid": "crm-score-op" } as never }}>
                <Option value="eq">equal to</Option><Option value="contains">contains</Option><Option value="gt">above</Option><Option value="lt">below</Option><Option value="exists">present</Option>
              </Select></FormControl>
            <FormControl sx={{ width: 130 }}><FormLabel>Value</FormLabel><Input size="sm" value={f.value} onChange={(e) => setF({ ...f, value: e.target.value })} slotProps={{ input: { "data-testid": "crm-score-value" } }} /></FormControl>
            <FormControl sx={{ width: 90 }}><FormLabel>Points</FormLabel><Input size="sm" type="number" value={f.points} onChange={(e) => setF({ ...f, points: Number(e.target.value) })} slotProps={{ input: { "data-testid": "crm-score-points" } }} /></FormControl>
            <Button size="sm" data-testid="crm-score-add" onClick={async () => { if (await run(() => api2.save("score-rules", "", f))) { setF({ ...f, value: "" }); load(); } }}>Add rule</Button>
          </Stack>
        </Sheet>
      )}
      <ErrorAlert err={err} />
      {data.me.is_admin && (
        <Stack direction="row" gap={1} alignItems="center">
          <Button size="sm" variant="outlined" data-testid="crm-rescore" onClick={async () => { const r = await run(() => api2.post<{ rescored: number }>("rescore")); if (r) { setMsg(`Rescored ${r.rescored} leads.`); data.reload(); } }}>Rescore all leads now</Button>
          {msg && <Typography level="body-sm" color="success">{msg}</Typography>}
        </Stack>
      )}
    </Stack>
  );
}

function SLA({ data }: { data: Data }) {
  const s = data.settings;
  const [hours, setHours] = useState(s.sla_lead_hours ?? 0);
  const [days, setDays] = useState<Record<string, number>>(s.sla_stage_days ?? {});
  const [saved, setSaved] = useState(false);
  const { err, run } = useErr();
  return (
    <Stack gap={1.5} data-testid="crm-sla">
      <Typography level="body-sm">A lead breaches its SLA when nobody has called, met or emailed it within the response window. A deal breaches when it stays in a stage longer than that stage allows. Breaches show as red chips on leads and deals. Use "A deal is idle" rules to alert people.</Typography>
      <FormControl sx={{ maxWidth: 260 }}>
        <FormLabel>First response within (hours, 0 = off)</FormLabel>
        <Input type="number" value={hours} disabled={!data.me.is_admin} onChange={(e) => setHours(Number(e.target.value))} slotProps={{ input: { min: 0, "data-testid": "crm-sla-hours" } }} />
      </FormControl>
      <Typography level="title-sm">Longest time in a stage (days, 0 = no limit)</Typography>
      <Stack direction="row" gap={1} flexWrap="wrap">
        {s.stages.map((st) => (
          <FormControl key={st.id} sx={{ width: 150 }}>
            <FormLabel>{st.name}</FormLabel>
            <Input type="number" size="sm" value={days[st.id] ?? 0} disabled={!data.me.is_admin} onChange={(e) => setDays({ ...days, [st.id]: Number(e.target.value) })} slotProps={{ input: { min: 0, "data-testid": `crm-sla-stage-${st.id}` } }} />
          </FormControl>
        ))}
      </Stack>
      <ErrorAlert err={err} />
      {data.me.is_admin && (
        <Stack direction="row" gap={1} alignItems="center">
          <Button data-testid="crm-sla-save" onClick={async () => {
            const clean = Object.fromEntries(Object.entries(days).filter(([, v]) => v > 0));
            if (await run(() => api.saveSettings({ ...s, sla_lead_hours: hours, sla_stage_days: clean }))) { setSaved(true); data.reload(); }
          }}>Save SLA</Button>
          {saved && <Typography level="body-sm" color="success">Saved.</Typography>}
        </Stack>
      )}
    </Stack>
  );
}
