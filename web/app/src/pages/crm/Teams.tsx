import { PersonSelect, PeopleSelect } from "../../components/PeoplePicker";
import { useCallback, useEffect, useState } from "react";
import { Box, Button, FormControl, FormLabel, Input, LinearProgress, Option, Select, Sheet, Stack, Tab, TabList, TabPanel, Table, Tabs, Typography } from "@mui/joy";
import * as api from "./api";
import * as api2 from "./api2";
import { money } from "./logic";
import { ConfirmButton, Dialog, Empty, ErrorAlert, useErr } from "./ui";
import type { Data } from "./ui";

interface Team {
  id: string;
  name: string;
  manager_id: string;
  members: string[];
}
interface Territory {
  id: string;
  name: string;
  team_id: string;
  industries: string[];
  keywords: string[];
}
interface Leader {
  user_id: string;
  name: string;
  won: number;
  won_count: number;
  open: number;
  target: number;
  pct: number;
  activities: number;
}
interface UserScope {
  user_id: string;
  scope: string;
  auto_log_mail: boolean;
}

const month = () => new Date().toISOString().slice(0, 7);

export function Teams({ data }: { data: Data }) {
  return (
    <Box>
      <Typography level="h4" sx={{ mb: 1 }}>
        Teams, targets and visibility
      </Typography>
      <Tabs defaultValue="leaders" sx={{ bgcolor: "transparent" }}>
        <TabList>
          <Tab value="leaders" data-testid="crm-team-tab-leaders">Leaderboard</Tab>
          <Tab value="teams" data-testid="crm-team-tab-teams">Teams</Tab>
          <Tab value="territories" data-testid="crm-team-tab-territories">Territories</Tab>
          <Tab value="visibility" data-testid="crm-team-tab-visibility">Visibility</Tab>
        </TabList>
        <TabPanel value="leaders"><Leaderboard data={data} /></TabPanel>
        <TabPanel value="teams"><Teams2 data={data} /></TabPanel>
        <TabPanel value="territories"><Territories data={data} /></TabPanel>
        <TabPanel value="visibility"><Visibility data={data} /></TabPanel>
      </Tabs>
    </Box>
  );
}

function Leaderboard({ data }: { data: Data }) {
  const [period, setPeriod] = useState(month());
  const [rows, setRows] = useState<Leader[]>([]);
  const [target, setTarget] = useState({ user: "", amount: 0 });
  const { err, run } = useErr();
  const load = useCallback(() => { void run(() => api.getLeaderboard(period)).then((r) => r && setRows(r)); }, [period, run]);
  useEffect(load, [load]);
  return (
    <Stack gap={1.5} data-testid="crm-leaderboard">
      <Stack direction="row" gap={1} alignItems="center">
        <FormControl><FormLabel>Month</FormLabel><Input type="month" size="sm" value={period} onChange={(e) => setPeriod(e.target.value)} /></FormControl>
        <Typography level="body-sm" sx={{ flex: 1, pt: 2 }}>Won value this month against each person's target. You only see deals you are allowed to see.</Typography>
      </Stack>
      <ErrorAlert err={err} />
      {rows.length === 0 ? <Empty>No deals or targets for this month.</Empty> : (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table data-testid="crm-leader-table">
            <thead><tr><th>#</th><th>Person</th><th style={{ textAlign: "right" }}>Won</th><th style={{ textAlign: "right" }}>Open pipeline</th><th>Target</th><th style={{ textAlign: "right" }}>Activities</th></tr></thead>
            <tbody>
              {rows.map((r, i) => (
                <tr key={r.user_id}>
                  <td>{i + 1}</td><td>{r.name}</td>
                  <td style={{ textAlign: "right" }}>{money(r.won, data.settings.currency)} ({r.won_count})</td>
                  <td style={{ textAlign: "right" }}>{money(r.open, data.settings.currency)}</td>
                  <td style={{ minWidth: 160 }}>{r.target > 0 ? (<><LinearProgress determinate value={Math.min(100, r.pct)} /><Typography level="body-xs">{r.pct}% of {money(r.target, data.settings.currency)}</Typography></>) : <Typography level="body-xs">No target</Typography>}</td>
                  <td style={{ textAlign: "right" }}>{r.activities}</td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
      {data.me.is_admin && (
        <Stack direction="row" gap={1} alignItems="flex-end" flexWrap="wrap">
          <FormControl sx={{ width: 200 }}><FormLabel>Set target for</FormLabel>
            <PersonSelect value={target.user} known={data.users} onChange={(id) => setTarget({ ...target, user: id })} ariaLabel="Target user" testId="crm-quota-user" size="sm" /></FormControl>
          <FormControl sx={{ width: 140 }}><FormLabel>Target ({data.settings.currency})</FormLabel><Input size="sm" type="number" value={target.amount} onChange={(e) => setTarget({ ...target, amount: Number(e.target.value) })} slotProps={{ input: { min: 0, "data-testid": "crm-quota-amount" } }} /></FormControl>
          <Button size="sm" disabled={!target.user} data-testid="crm-quota-save" onClick={async () => { if (await run(() => api2.put("quotas", { user_id: target.user, period, target: target.amount }))) load(); }}>Save target</Button>
        </Stack>
      )}
    </Stack>
  );
}

function Teams2({ data }: { data: Data }) {
  const [teams, setTeams] = useState<Team[]>([]);
  const [editing, setEditing] = useState<Team | "new" | null>(null);
  const { err, run } = useErr();
  const load = useCallback(() => { void run(() => api2.list<Team>("teams", "teams")).then((l) => l && setTeams(l)); }, [run]);
  useEffect(load, [load]);
  const name = (id: string) => data.users.find((u) => u.id === id)?.name ?? id;
  return (
    <Stack gap={1.5} data-testid="crm-teams">
      <Stack direction="row" alignItems="center" gap={1}>
        <Typography level="body-sm" sx={{ flex: 1 }}>A team lets "team" visibility share records between its members, and powers team forecasts.</Typography>
        {data.me.is_admin && <Button size="sm" onClick={() => setEditing("new")} data-testid="crm-new-team">New team</Button>}
      </Stack>
      <ErrorAlert err={err} />
      {teams.length === 0 ? <Empty>No teams yet.</Empty> : teams.map((t) => (
        <Stack key={t.id} direction="row" gap={1} alignItems="center">
          <Typography level="body-sm" sx={{ flex: 1 }}><b>{t.name}</b> · {t.members.map(name).join(", ") || "no members"}{t.manager_id ? ` · manager ${name(t.manager_id)}` : ""}</Typography>
          {data.me.is_admin && <Button size="sm" variant="plain" onClick={() => setEditing(t)}>Edit</Button>}
        </Stack>
      ))}
      {editing && <TeamDialog data={data} team={editing === "new" ? null : editing} onClose={() => setEditing(null)} onSaved={() => { setEditing(null); load(); }} />}
    </Stack>
  );
}

function TeamDialog({ data, team, onClose, onSaved }: { data: Data; team: Team | null; onClose: () => void; onSaved: () => void }) {
  const [f, setF] = useState<Team>(team ?? { id: "", name: "", manager_id: "", members: [] });
  const { err, run } = useErr();
  return (
    <Dialog title={team ? "Edit team" : "New team"} onClose={onClose} width={480}>
      <Stack gap={1.5}>
        <FormControl required><FormLabel>Name</FormLabel><Input value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} slotProps={{ input: { "data-testid": "crm-team-name" } }} /></FormControl>
        <FormControl><FormLabel>Manager</FormLabel>
          <PersonSelect value={f.manager_id} known={data.users} onChange={(id) => setF({ ...f, manager_id: id })} placeholder="None" ariaLabel="Manager" /></FormControl>
        <FormControl><FormLabel>Members</FormLabel>
          <PeopleSelect value={f.members} known={data.users} onChange={(ids) => setF({ ...f, members: ids })} ariaLabel="Members" testId="crm-team-members" /></FormControl>
        <ErrorAlert err={err} />
        <Stack direction="row" justifyContent="space-between">
          {team ? <ConfirmButton label="Delete" onConfirm={async () => { if (await run(() => api2.remove("teams", team.id))) onSaved(); }} /> : <span />}
          <Button disabled={!f.name.trim()} data-testid="crm-team-save" onClick={async () => { if (await run(() => api2.save("teams", team?.id ?? "", f))) onSaved(); }}>Save</Button>
        </Stack>
      </Stack>
    </Dialog>
  );
}

function Territories({ data }: { data: Data }) {
  const [list, setList] = useState<Territory[]>([]);
  const [teams, setTeams] = useState<Team[]>([]);
  const [f, setF] = useState({ name: "", team_id: "", industries: "", keywords: "" });
  const [msg, setMsg] = useState("");
  const { err, run } = useErr();
  const load = useCallback(() => {
    void run(() => api2.list<Territory>("territories", "territories")).then((l) => l && setList(l));
    void api2.list<Team>("teams", "teams").then(setTeams).catch(() => undefined);
  }, [run]);
  useEffect(load, [load]);
  const csv = (s: string) => s.split(",").map((x) => x.trim()).filter(Boolean);
  return (
    <Stack gap={1.5} data-testid="crm-territories">
      <Typography level="body-sm">A territory groups companies by industry or a name keyword and points them at a team. "Apply" tags every company that matches and has no territory yet.</Typography>
      {list.map((t) => (
        <Stack key={t.id} direction="row" gap={1} alignItems="center">
          <Typography level="body-sm" sx={{ flex: 1 }}><b>{t.name}</b> · {teams.find((x) => x.id === t.team_id)?.name ?? "no team"} · industries: {t.industries.join(", ") || "-"} · keywords: {t.keywords.join(", ") || "-"}</Typography>
          {data.me.is_admin && <ConfirmButton label="Delete" onConfirm={async () => { if (await run(() => api2.remove("territories", t.id))) load(); }} />}
        </Stack>
      ))}
      {data.me.is_admin && (
        <Sheet variant="soft" sx={{ p: 1.5, borderRadius: "md" }}>
          <Stack direction="row" gap={1} flexWrap="wrap" alignItems="flex-end">
            <FormControl sx={{ width: 150 }}><FormLabel>Name</FormLabel><Input size="sm" value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} slotProps={{ input: { "data-testid": "crm-terr-name" } }} /></FormControl>
            <FormControl sx={{ width: 150 }}><FormLabel>Team</FormLabel><Select size="sm" value={f.team_id} onChange={(_, v) => setF({ ...f, team_id: v ?? "" })}><Option value="">None</Option>{teams.map((t) => <Option key={t.id} value={t.id}>{t.name}</Option>)}</Select></FormControl>
            <FormControl sx={{ width: 180 }}><FormLabel>Industries (comma)</FormLabel><Input size="sm" value={f.industries} onChange={(e) => setF({ ...f, industries: e.target.value })} /></FormControl>
            <FormControl sx={{ width: 180 }}><FormLabel>Name keywords (comma)</FormLabel><Input size="sm" value={f.keywords} onChange={(e) => setF({ ...f, keywords: e.target.value })} /></FormControl>
            <Button size="sm" disabled={!f.name.trim()} data-testid="crm-terr-save" onClick={async () => { if (await run(() => api2.save("territories", "", { name: f.name, team_id: f.team_id, industries: csv(f.industries), keywords: csv(f.keywords) }))) { setF({ name: "", team_id: "", industries: "", keywords: "" }); load(); } }}>Add</Button>
          </Stack>
        </Sheet>
      )}
      {data.me.is_admin && <Stack direction="row" gap={1} alignItems="center"><Button size="sm" variant="outlined" data-testid="crm-terr-apply" onClick={async () => { const r = await run(() => api2.post<{ assigned: number }>("territories/apply")); if (r) { setMsg(`${r.assigned} companies assigned.`); data.reload(); } }}>Apply to companies</Button>{msg && <Typography level="body-sm" color="success">{msg}</Typography>}</Stack>}
      <ErrorAlert err={err} />
    </Stack>
  );
}

function Visibility({ data }: { data: Data }) {
  const [scopes, setScopes] = useState<UserScope[]>([]);
  const [def, setDef] = useState(data.settings.default_scope ?? "all");
  const { err, run } = useErr();
  const load = useCallback(() => { void run(() => api2.list<UserScope>("user-scopes", "scopes")).then((l) => l && setScopes(l)); }, [run]);
  useEffect(load, [load]);
  const eff = (id: string) => scopes.find((s) => s.user_id === id)?.scope ?? def;
  return (
    <Stack gap={1.5} data-testid="crm-visibility">
      <Typography level="body-sm">
        Who sees which leads, companies and deals. "Own" shows only records you own or created, "Team" adds records owned by people in your teams, "All" shows everything. Administrators always see everything. This is enforced on the server for lists, search, reports and edits.
      </Typography>
      <FormControl sx={{ maxWidth: 300 }}>
        <FormLabel>Default for everyone</FormLabel>
        <Select value={def} disabled={!data.me.is_admin} onChange={async (_, v) => { const n = v ?? "all"; if (await run(() => api.saveSettings({ ...data.settings, default_scope: n }))) { setDef(n); data.reload(); } }} slotProps={{ button: { "data-testid": "crm-default-scope" } as never }}>
          <Option value="all">All records</Option><Option value="team">Own and team</Option><Option value="own">Own records only</Option>
        </Select>
      </FormControl>
      <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
        <Table data-testid="crm-user-scopes">
          <thead><tr><th>Person</th><th>Sees</th></tr></thead>
          <tbody>
            {data.users.map((u) => (
              <tr key={u.id}><td>{u.name}<Typography level="body-xs">{u.email}</Typography></td>
                <td><Select size="sm" sx={{ width: 190 }} value={eff(u.id)} disabled={!data.me.is_admin} onChange={async (_, v) => { if (v && (await run(() => api2.put("user-scopes", { user_id: u.id, scope: v })))) load(); }} aria-label={`Visibility for ${u.name}`} slotProps={{ button: { "data-testid": `crm-scope-${u.email}` } as never }}>
                  <Option value="all">All records</Option><Option value="team">Own and team</Option><Option value="own">Own records only</Option></Select></td></tr>
            ))}
          </tbody>
        </Table>
      </Sheet>
      <ErrorAlert err={err} />
    </Stack>
  );
}
