// Lists, settings, delegation and approvals tabs.
import { useCallback, useEffect, useState } from "react";
import { Alert, Box, Button, Checkbox, Chip, FormControl, FormLabel, Input, Option, Select, Sheet, Switch, Table, Typography } from "@mui/joy";
import { A, iam, type IamSettings, type Me, type Person } from "./api";
import { StepUpDialog } from "./ActionDialog";
import { can } from "./logic";

const when = (s?: string | null) => (s ? new Date(s).toLocaleString() : "never");

function useRetryStepUp() {
  const [open, setOpen] = useState(false);
  const [retry, setRetry] = useState<(() => void) | null>(null);
  const dialog = (
    <StepUpDialog
      open={open}
      onClose={() => setOpen(false)}
      onDone={() => {
        setOpen(false);
        retry?.();
      }}
    />
  );
  const guard = async (fn: () => Promise<void>, onErr: (m: string) => void) => {
    try {
      await fn();
    } catch (e) {
      const ie = e as { stepUp?: boolean; message: string };
      if (ie.stepUp) {
        setRetry(() => () => void guard(fn, onErr));
        setOpen(true);
      } else onErr(ie.message);
    }
  };
  return { dialog, guard };
}

interface Rule {
  id: string;
  kind: string;
  scope: string;
  value: string;
  reason: string;
  expires_at: string | null;
  expired: boolean;
}

export function ListsTab({ me }: { me: Me }) {
  const [items, setItems] = useState<Rule[]>([]);
  const [f, setF] = useState({ kind: "block", scope: "ip", value: "", reason: "", hours: "" });
  const [err, setErr] = useState("");
  const { dialog, guard } = useRetryStepUp();
  const load = useCallback(async () => {
    try {
      setItems((await iam<{ items: Rule[] }>(`${A}/lists`)).items);
    } catch (e) {
      setErr((e as Error).message);
    }
  }, []);
  useEffect(() => void load(), [load]);
  const edit = can(me.capabilities, "list_edit");
  const add = () =>
    guard(async () => {
      await iam(`${A}/lists`, { method: "POST", json: { kind: f.kind, scope: f.scope, value: f.value, reason: f.reason, hours: Number(f.hours) || 0 } });
      setF({ ...f, value: "", reason: "" });
      setErr("");
      await load();
    }, setErr);
  return (
    <Box data-testid="lists-tab">
      {err && (
        <Alert color="danger" sx={{ mb: 1 }}>
          {err}
        </Alert>
      )}
      {edit && (
        <Sheet variant="outlined" sx={{ p: 1.5, borderRadius: "md", mb: 2, display: "flex", gap: 1, flexWrap: "wrap", alignItems: "flex-end" }}>
          <FormControl>
            <FormLabel>List</FormLabel>
            <Select size="sm" value={f.kind} onChange={(_, v) => v && setF({ ...f, kind: v })} sx={{ width: 110 }}>
              <Option value="block">Block</Option>
              <Option value="allow">Allow</Option>
            </Select>
          </FormControl>
          <FormControl>
            <FormLabel>Type</FormLabel>
            <Select size="sm" value={f.scope} onChange={(_, v) => v && setF({ ...f, scope: v })} sx={{ width: 160 }}>
              <Option value="ip">Address or range</Option>
              <Option value="country">Country</Option>
            </Select>
          </FormControl>
          <FormControl>
            <FormLabel>{f.scope === "country" ? "Country code" : "Address or CIDR"}</FormLabel>
            <Input size="sm" value={f.value} onChange={(e) => setF({ ...f, value: e.target.value })} placeholder={f.scope === "country" ? "KP" : "203.0.113.0/24"} slotProps={{ input: { "data-testid": "rule-value" } }} />
          </FormControl>
          <FormControl>
            <FormLabel>Reason</FormLabel>
            <Input size="sm" value={f.reason} onChange={(e) => setF({ ...f, reason: e.target.value })} sx={{ width: 220 }} slotProps={{ input: { "data-testid": "rule-reason" } }} />
          </FormControl>
          <FormControl>
            <FormLabel>Expires after (hours)</FormLabel>
            <Input size="sm" type="number" value={f.hours} onChange={(e) => setF({ ...f, hours: e.target.value })} sx={{ width: 130 }} placeholder="never" />
          </FormControl>
          <Button size="sm" data-testid="rule-add" onClick={() => void add()}>
            Add
          </Button>
        </Sheet>
      )}
      <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
        <Table>
          <thead>
            <tr>
              <th>List</th>
              <th>Entry</th>
              <th>Reason</th>
              <th>Expires</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {items.map((r) => (
              <tr key={r.id} data-testid="rule-row">
                <td>
                  <Chip size="sm" color={r.kind === "block" ? "danger" : "success"} variant="soft">
                    {r.kind}
                  </Chip>
                </td>
                <td>{r.scope === "country" ? `Country ${r.value}` : r.value}</td>
                <td>{r.reason}</td>
                <td>{r.expires_at ? (r.expired ? "expired" : when(r.expires_at)) : "never"}</td>
                <td style={{ textAlign: "right" }}>
                  {edit && (
                    <Button
                      size="sm"
                      variant="plain"
                      color="danger"
                      onClick={() =>
                        void guard(async () => {
                          await iam(`${A}/lists/${r.id}`, { method: "DELETE" });
                          await load();
                        }, setErr)
                      }
                    >
                      Remove
                    </Button>
                  )}
                </td>
              </tr>
            ))}
            {!items.length && (
              <tr>
                <td colSpan={5}>
                  <Typography level="body-sm" sx={{ p: 2 }}>
                    No entries. A blocked address or country cannot sign in; an allow entry overrides a block.
                  </Typography>
                </td>
              </tr>
            )}
          </tbody>
        </Table>
      </Sheet>
      {dialog}
    </Box>
  );
}

const APPROVABLE: [string, string][] = [
  ["mfa.bypass", "MFA bypass"],
  ["password.temp", "Temporary password"],
  ["mfa.reset_all", "Reset all second steps"],
  ["contain", "Contain account"],
  ["account.suspend", "Suspend"],
];
const THRESHOLDS: [string, string][] = [
  ["fails_per_account", "Failed sign-ins on one account"],
  ["fails_per_ip", "Failed sign-ins from one address"],
  ["stuffing_accounts", "Accounts tried from one address"],
  ["mfa_denied", "Denied second-step prompts"],
  ["window_minutes", "Window for the counts (minutes)"],
  ["travel_minutes", "Impossible travel window (minutes)"],
  ["off_hours_start", "Off hours start (hour 0-23)"],
  ["off_hours_end", "Off hours end (hour 0-23)"],
  ["off_hours_utc_offset_min", "Org clock offset from UTC (minutes)"],
  ["sla_hours_critical", "SLA hours: critical"],
  ["sla_hours_high", "SLA hours: high"],
  ["sla_hours_medium", "SLA hours: medium"],
  ["sla_hours_low", "SLA hours: low"],
];

export function SettingsTab({ me }: { me: Me }) {
  const [s, setS] = useState<IamSettings | null>(null);
  const [rules, setRules] = useState<Record<string, string>>({});
  const [anon, setAnon] = useState("");
  const [msg, setMsg] = useState("");
  const [err, setErr] = useState("");
  const { dialog, guard } = useRetryStepUp();
  useEffect(() => {
    void iam<{ settings: IamSettings; rules: Record<string, string> }>(`${A}/settings`)
      .then((r) => {
        setS(r.settings);
        setRules(r.rules);
        setAnon(r.settings.anon_networks.join("\n"));
      })
      .catch((e) => setErr((e as Error).message));
  }, []);
  if (!s) return err ? <Alert color="danger">{err}</Alert> : null;
  const edit = can(me.capabilities, "policy_edit");
  const th = s.thresholds;
  const save = () =>
    guard(async () => {
      const r = await iam<{ settings: IamSettings; note: string }>(`${A}/settings`, {
        method: "PUT",
        json: { ...s, anon_networks: anon.split(/[\n,]/).map((x) => x.trim()).filter(Boolean) },
      });
      setS(r.settings);
      setErr("");
      setMsg(r.note || "Saved.");
    }, setErr);
  return (
    <Box data-testid="settings-tab" sx={{ maxWidth: 900 }}>
      {msg && (
        <Alert color="success" sx={{ mb: 1 }} data-testid="settings-msg">
          {msg}
        </Alert>
      )}
      {err && (
        <Alert color="danger" sx={{ mb: 1 }}>
          {err}
        </Alert>
      )}
      <Typography level="title-md">Second-step policy</Typography>
      <Select disabled={!edit} value={s.mfa_policy.mode} onChange={(_, v) => v && setS({ ...s, mfa_policy: { mode: v } })} sx={{ maxWidth: 520, my: 1 }}>
        <Option value="off">Off: people choose</Option>
        <Option value="admins">Administrators: watched, reported when an admin has none</Option>
        <Option value="all">Everyone: enforced by the identity provider at sign-in</Option>
        <Option value="risky">Risky sign-ins: watched, alerts raised</Option>
      </Select>
      <Typography level="body-xs" sx={{ mb: 2, opacity: 0.7 }}>
        Only "Everyone" blocks a sign-in. The other modes raise alerts and fill the report of privileged accounts without a second step.
      </Typography>

      <Typography level="title-md">Detection thresholds</Typography>
      <Box sx={{ display: "grid", gridTemplateColumns: "repeat(auto-fill,minmax(260px,1fr))", gap: 1.5, my: 1 }}>
        {THRESHOLDS.map(([k, label]) => (
          <FormControl key={k}>
            <FormLabel>{label}</FormLabel>
            <Input size="sm" type="number" disabled={!edit} value={String(th[k] ?? "")} onChange={(e) => setS({ ...s, thresholds: { ...th, [k]: Number(e.target.value) } })} slotProps={{ input: { "data-testid": `th-${k}` } }} />
          </FormControl>
        ))}
      </Box>
      <Box sx={{ display: "flex", gap: 3, mb: 2 }}>
        <Switch disabled={!edit} checked={!!th.off_hours_enabled} onChange={(e) => setS({ ...s, thresholds: { ...th, off_hours_enabled: e.target.checked } })} endDecorator="Off-hours rule" />
        <Switch disabled={!edit} checked={!!th.new_device_enabled} onChange={(e) => setS({ ...s, thresholds: { ...th, new_device_enabled: e.target.checked } })} endDecorator="New-device rule" />
      </Box>
      <Typography level="body-xs" sx={{ mb: 2, opacity: 0.7 }}>
        Rules: {Object.values(rules).join("; ")}.
      </Typography>

      <Typography level="title-md">Anonymising networks (VPN, proxy, Tor)</Typography>
      <Input component="textarea" disabled={!edit} sx={{ my: 1, "& textarea": { minHeight: 80 } }} value={anon} onChange={(e) => setAnon(e.target.value)} placeholder="One address or range per line, e.g. 198.51.100.0/24" />

      <Typography level="title-md" sx={{ mt: 1 }}>
        Where alerts go
      </Typography>
      <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap", my: 1 }}>
        <FormControl>
          <FormLabel>Security mailbox</FormLabel>
          <Input size="sm" disabled={!edit} sx={{ width: 300 }} value={s.security_mailbox} onChange={(e) => setS({ ...s, security_mailbox: e.target.value })} placeholder="security@yourcompany.com" />
        </FormControl>
        <FormControl>
          <FormLabel>Webhook address</FormLabel>
          <Input size="sm" disabled={!edit} sx={{ width: 380 }} value={s.webhook_url} onChange={(e) => setS({ ...s, webhook_url: e.target.value })} placeholder="https://..." />
        </FormControl>
      </Box>
      <Typography level="body-xs" sx={{ mb: 1, opacity: 0.7 }}>
        Alerts always appear in the notification centre of admins and analysts.
      </Typography>
      <Switch disabled={!edit} checked={s.notify_manager} onChange={(e) => setS({ ...s, notify_manager: e.target.checked })} endDecorator="Tell the person's manager when an action is taken on them" />

      <Typography level="title-md" sx={{ mt: 2 }}>
        Two-person approval
      </Typography>
      <Typography level="body-xs" sx={{ mb: 0.5, opacity: 0.7 }}>
        These actions wait for a second identity admin before they run.
      </Typography>
      {APPROVABLE.map(([k, label]) => (
        <Checkbox
          key={k}
          disabled={!edit}
          sx={{ mr: 2, mb: 0.5 }}
          label={label}
          checked={s.require_approval.includes(k)}
          onChange={(e) => setS({ ...s, require_approval: e.target.checked ? [...s.require_approval, k] : s.require_approval.filter((x) => x !== k) })}
        />
      ))}
      <Box sx={{ mt: 2 }}>
        {edit ? (
          <Button onClick={() => void save()} data-testid="settings-save">
            Save settings
          </Button>
        ) : (
          <Typography level="body-sm">Your role can read these settings but not change them.</Typography>
        )}
      </Box>
      {dialog}
    </Box>
  );
}

interface RoleRow {
  user_id: string;
  email: string;
  name: string;
  role: string;
}

export function RolesTab({ me }: { me: Me }) {
  const [rows, setRows] = useState<RoleRow[]>([]);
  const [people, setPeople] = useState<Person[]>([]);
  const [q, setQ] = useState("");
  const [pick, setPick] = useState("");
  const [role, setRole] = useState("help_desk");
  const [err, setErr] = useState("");
  const { dialog, guard } = useRetryStepUp();
  const load = useCallback(async () => {
    try {
      setRows((await iam<{ items: RoleRow[] }>(`${A}/roles`)).items);
    } catch (e) {
      setErr((e as Error).message);
    }
  }, []);
  useEffect(() => void load(), [load]);
  useEffect(() => {
    const t = setTimeout(
      () =>
        void iam<{ items: Person[] }>(`${A}/users?limit=8&q=${encodeURIComponent(q)}`)
          .then((r) => setPeople(r.items))
          .catch(() => undefined),
      250,
    );
    return () => clearTimeout(t);
  }, [q]);
  const edit = can(me.capabilities, "delegate");
  return (
    <Box data-testid="roles-tab">
      <Sheet variant="soft" sx={{ p: 1.5, borderRadius: "md", mb: 2 }}>
        {me.role_catalogue.map((r) => (
          <Typography key={r.Key} level="body-sm">
            <b>{r.Name}</b>: {r.Description}
          </Typography>
        ))}
        <Typography level="body-xs" sx={{ mt: 1, opacity: 0.7 }}>
          Organisation admins are identity admins automatically. Older "Help desk" and "Read-only admin" assignments count as Help desk and Auditor.
        </Typography>
      </Sheet>
      {err && (
        <Alert color="danger" sx={{ mb: 1 }}>
          {err}
        </Alert>
      )}
      {edit && (
        <Box sx={{ display: "flex", gap: 1, mb: 2, alignItems: "flex-end", flexWrap: "wrap" }}>
          <FormControl>
            <FormLabel>Find a person</FormLabel>
            <Input size="sm" value={q} onChange={(e) => setQ(e.target.value)} sx={{ width: 240 }} placeholder="Name or e-mail" slotProps={{ input: { "data-testid": "role-search" } }} />
          </FormControl>
          <FormControl>
            <FormLabel>Person</FormLabel>
            <Select size="sm" value={pick || null} placeholder="Choose" onChange={(_, v) => setPick(v ?? "")} sx={{ width: 260 }}>
              {people.map((p) => (
                <Option key={p.id} value={p.id}>
                  {p.email}
                </Option>
              ))}
            </Select>
          </FormControl>
          <FormControl>
            <FormLabel>Role</FormLabel>
            <Select size="sm" value={role} onChange={(_, v) => v && setRole(v)} sx={{ width: 190 }}>
              {me.role_catalogue.map((r) => (
                <Option key={r.Key} value={r.Key}>
                  {r.Name}
                </Option>
              ))}
            </Select>
          </FormControl>
          <Button
            size="sm"
            disabled={!pick}
            data-testid="role-add"
            onClick={() =>
              void guard(async () => {
                await iam(`${A}/roles`, { method: "POST", json: { user_id: pick, role } });
                await load();
              }, setErr)
            }
          >
            Assign
          </Button>
        </Box>
      )}
      <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
        <Table>
          <thead>
            <tr>
              <th>Person</th>
              <th>Role</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <tr key={r.user_id + r.role} data-testid="role-row">
                <td>
                  {r.name || r.email}
                  <Typography level="body-xs">{r.email}</Typography>
                </td>
                <td>
                  <Chip size="sm" variant="soft">
                    {r.role.replace("_", " ")}
                  </Chip>
                </td>
                <td style={{ textAlign: "right" }}>
                  {edit && (
                    <Button
                      size="sm"
                      variant="plain"
                      color="danger"
                      onClick={() =>
                        void guard(async () => {
                          await iam(`${A}/roles`, { method: "DELETE", json: { user_id: r.user_id, role: r.role } });
                          await load();
                        }, setErr)
                      }
                    >
                      Remove
                    </Button>
                  )}
                </td>
              </tr>
            ))}
            {!rows.length && (
              <tr>
                <td colSpan={3}>
                  <Typography level="body-sm" sx={{ p: 2 }}>
                    No one has an operations role yet. Organisation admins have all of them.
                  </Typography>
                </td>
              </tr>
            )}
          </tbody>
        </Table>
      </Sheet>
      {dialog}
    </Box>
  );
}

interface Appr {
  id: string;
  action: string;
  target_email: string;
  requester: string;
  status: string;
  reason: string;
  ticket: string;
}

export function ApprovalsTab({ me, onChanged }: { me: Me; onChanged: () => void }) {
  const [items, setItems] = useState<Appr[]>([]);
  const [err, setErr] = useState("");
  const [res, setRes] = useState<{ message: string; secret?: string } | null>(null);
  const { dialog, guard } = useRetryStepUp();
  const load = useCallback(async () => {
    try {
      setItems((await iam<{ items: Appr[] }>(`${A}/approvals`)).items);
    } catch (e) {
      setErr((e as Error).message);
    }
  }, []);
  useEffect(() => void load(), [load]);
  const decide = (id: string, what: "approve" | "reject") =>
    guard(async () => {
      const r = await iam<{ message: string; secret?: string }>(`${A}/approvals/${id}/${what}`, { method: "POST", json: {} });
      setRes(r);
      setErr("");
      await load();
      onChanged();
    }, setErr);
  return (
    <Box data-testid="approvals-tab">
      {err && (
        <Alert color="danger" sx={{ mb: 1 }}>
          {err}
        </Alert>
      )}
      {res && (
        <Alert color="success" sx={{ mb: 1, display: "block" }}>
          {res.message}
          {res.secret && (
            <Typography sx={{ fontFamily: "monospace" }} level="title-md">
              {res.secret}
            </Typography>
          )}
        </Alert>
      )}
      <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
        <Table>
          <thead>
            <tr>
              <th>Action</th>
              <th>Person</th>
              <th>Asked by</th>
              <th>Reason</th>
              <th>Status</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {items.map((a) => (
              <tr key={a.id} data-testid="approval-row">
                <td>{a.action}</td>
                <td>{a.target_email}</td>
                <td>{a.requester}</td>
                <td>
                  {a.reason}
                  {a.ticket ? ` (${a.ticket})` : ""}
                </td>
                <td>
                  <Chip size="sm" variant="soft">
                    {a.status}
                  </Chip>
                </td>
                <td style={{ textAlign: "right", whiteSpace: "nowrap" }}>
                  {a.status === "pending" && can(me.capabilities, "approve") && (
                    <>
                      <Button size="sm" onClick={() => void decide(a.id, "approve")} data-testid="approve">
                        Approve
                      </Button>{" "}
                      <Button size="sm" variant="plain" color="danger" onClick={() => void decide(a.id, "reject")}>
                        Reject
                      </Button>
                    </>
                  )}
                </td>
              </tr>
            ))}
            {!items.length && (
              <tr>
                <td colSpan={6}>
                  <Typography level="body-sm" sx={{ p: 2 }}>
                    Nothing is waiting. Choose which actions need a second person under Detection and policy.
                  </Typography>
                </td>
              </tr>
            )}
          </tbody>
        </Table>
      </Sheet>
      {dialog}
    </Box>
  );
}
