// Offboarding: suspend, archive, delete with transfer, restore, legal holds,
// retention and e-mail alerts to admins.
import { useCallback, useEffect, useState } from "react";
import { Box, Button, Checkbox, Chip, FormControl, FormLabel, Input, Modal, ModalClose, ModalDialog, Sheet, Switch, Table, Tab, TabList, TabPanel, Tabs, Typography } from "@mui/joy";
import { SubjectPicker } from "../SubjectPicker";
import type { Subject } from "../accessApi";
import { Notice, Page, api, when } from "./common";

interface Options {
  transfer_groups: { id: string; includes: string[] }[];
  restore_window_days: number;
  alert_events: string[];
}
interface Row {
  id: string;
  email: string;
  name: string;
  status: string;
  reason: string;
  deleted_at: string | null;
  purge_after: string | null;
  archived_at: string | null;
  on_hold: boolean;
}
interface Hold {
  id: string;
  name: string;
  reason: string;
  org_wide: boolean;
  user_ids: string[];
  created_at: string;
  released_at: string | null;
}
interface Job {
  id: string;
  email: string;
  kind: string;
  status: string;
  note: string;
  created_at: string;
  result: { transferred?: Record<string, number>; mail_forward?: string; chats_left?: number };
}

export function OffboardingSection() {
  const [opts, setOpts] = useState<Options | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [who, setWho] = useState<Subject | null>(null);
  const [heir, setHeir] = useState<Subject | null>(null);
  const [groups, setGroups] = useState<Record<string, boolean>>({});
  const [forward, setForward] = useState(true);
  const [leave, setLeave] = useState(true);
  const [reason, setReason] = useState("");
  const [dlg, setDlg] = useState<"delete" | null>(null);
  const [list, setList] = useState<Row[]>([]);
  const [state, setState] = useState("deleted");
  const [holds, setHolds] = useState<Hold[]>([]);
  const [holdName, setHoldName] = useState("");
  const [holdUser, setHoldUser] = useState<Subject | null>(null);
  const [holdOrg, setHoldOrg] = useState(false);
  const [history, setHistory] = useState<Job[]>([]);
  const [win, setWin] = useState("30");
  const [alertTo, setAlertTo] = useState("");
  const [alertEv, setAlertEv] = useState<string[]>([]);

  const load = useCallback(async () => {
    try {
      const o = await api<Options>("/api/v1/admin/lifecycle/options");
      setOpts(o);
      setGroups((g) => (Object.keys(g).length ? g : Object.fromEntries(o.transfer_groups.map((x) => [x.id, true]))));
      const [u, h, hi, ret, al] = await Promise.all([
        api<{ users: Row[] }>(`/api/v1/admin/lifecycle/users?state=${state}`),
        api<{ holds: Hold[] }>("/api/v1/admin/lifecycle/holds"),
        api<{ history: Job[] }>("/api/v1/admin/lifecycle/history"),
        api<{ restore_window_days: number }>("/api/v1/admin/lifecycle/retention"),
        api<{ recipients: string[]; events: string[] }>("/api/v1/admin/lifecycle/alerts"),
      ]);
      setList(u.users);
      setHolds(h.holds);
      setHistory(hi.history);
      setWin(String(ret.restore_window_days));
      setAlertTo(al.recipients.join(", "));
      setAlertEv(al.events);
    } catch (e) {
      setError((e as Error).message);
    }
  }, [state]);
  useEffect(() => {
    void load();
  }, [load]);

  async function act(id: string, action: string, body: object = {}) {
    setError(null);
    setNotice(null);
    try {
      const r = await api<{ transferred?: Record<string, number>; mail_forward?: string }>(`/api/v1/admin/lifecycle/users/${id}/${action}`, { method: "POST", json: body });
      const moved = r.transferred ? Object.entries(r.transferred).map(([k, v]) => `${v} ${k.toLowerCase()}`).join(", ") : "";
      setNotice(`Done: ${action}${moved ? `. Transferred ${moved}.` : "."}`);
      setDlg(null);
      await load();
    } catch (e) {
      setError((e as Error).message);
      setDlg(null);
    }
  }
  async function addHold() {
    setError(null);
    try {
      await api("/api/v1/admin/lifecycle/holds", { method: "POST", json: { name: holdName, org_wide: holdOrg, user_ids: holdUser ? [holdUser.id] : [] } });
      setHoldName("");
      setHoldUser(null);
      setHoldOrg(false);
      await load();
    } catch (e) {
      setError((e as Error).message);
    }
  }

  return (
    <Page testId="offboarding" title="Offboarding and retention" intro="Suspend or archive a person, or delete them with their files, calendar and mail handed to someone else. A legal hold blocks deletion until it is released.">
      <Notice error={error} notice={notice} />
      <Tabs defaultValue="act" sx={{ background: "transparent" }}>
        <TabList>
          <Tab value="act">Offboard a person</Tab>
          <Tab value="list">Deleted and archived</Tab>
          <Tab value="holds">Legal holds</Tab>
          <Tab value="settings">Retention and alerts</Tab>
          <Tab value="history">History</Tab>
        </TabList>
        <TabPanel value="act">
          <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", alignItems: "flex-end", mb: 2 }}>
            <FormControl size="sm">
              <FormLabel>Person</FormLabel>
              <SubjectPicker type="user" value={who} onChange={setWho} placeholder="Pick a person" testId="off-user" />
            </FormControl>
            <FormControl size="sm">
              <FormLabel>Reason or note</FormLabel>
              <Input size="sm" value={reason} onChange={(e) => setReason(e.target.value)} sx={{ width: 260 }} />
            </FormControl>
            <Button size="sm" variant="outlined" disabled={!who} onClick={() => who && void act(who.id, "suspend", { reason })} data-testid="off-suspend">
              Suspend
            </Button>
            <Button size="sm" variant="outlined" disabled={!who} onClick={() => who && void act(who.id, "unsuspend")}>
              Unsuspend
            </Button>
            <Button size="sm" variant="outlined" disabled={!who} onClick={() => who && void act(who.id, "archive", { note: reason })} data-testid="off-archive">
              Archive
            </Button>
            <Button size="sm" color="danger" disabled={!who} onClick={() => setDlg("delete")} data-testid="off-delete">
              Delete…
            </Button>
          </Box>
          <Typography level="body-sm" sx={{ maxWidth: 700, opacity: 0.8 }}>
            Suspend blocks sign-in and ends every session. Archive does the same and frees the seat. Delete also schedules the account for removal after {opts?.restore_window_days ?? 30} days;
            until then it can be restored.
          </Typography>
        </TabPanel>
        <TabPanel value="list">
          <Box sx={{ display: "flex", gap: 1, mb: 1 }}>
            {["deleted", "archived", "suspended"].map((s) => (
              <Chip key={s} variant={state === s ? "solid" : "outlined"} onClick={() => setState(s)} data-testid={`list-${s}`}>
                {s}
              </Chip>
            ))}
          </Box>
          <Table size="sm" sx={{ maxWidth: 900 }} data-testid="off-list">
            <thead>
              <tr>
                <th>Person</th>
                <th>Status</th>
                <th>Removed for good on</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {list.map((r) => (
                <tr key={r.id}>
                  <td>
                    {r.name} <Typography level="body-xs">{r.email}</Typography>
                  </td>
                  <td>
                    {r.deleted_at ? "deleted" : r.archived_at ? "archived" : "suspended"} {r.on_hold && <Chip size="sm" color="warning">legal hold</Chip>}
                  </td>
                  <td>{r.purge_after ? when(r.purge_after) : ""}</td>
                  <td>
                    {r.deleted_at ? (
                      <Button size="sm" onClick={() => void act(r.id, "restore")} data-testid={`restore-${r.email}`}>
                        Restore
                      </Button>
                    ) : (
                      <Button size="sm" variant="outlined" onClick={() => void act(r.id, "unsuspend")}>
                        Reactivate
                      </Button>
                    )}
                  </td>
                </tr>
              ))}
              {list.length === 0 && (
                <tr>
                  <td colSpan={4}>Nobody here.</td>
                </tr>
              )}
            </tbody>
          </Table>
        </TabPanel>
        <TabPanel value="holds">
          <Sheet variant="outlined" sx={{ p: 1.5, borderRadius: "md", maxWidth: 700, mb: 2, display: "flex", gap: 1, flexWrap: "wrap", alignItems: "flex-end" }}>
            <FormControl size="sm">
              <FormLabel>Hold name</FormLabel>
              <Input size="sm" value={holdName} onChange={(e) => setHoldName(e.target.value)} slotProps={{ input: { "data-testid": "hold-name" } as never }} />
            </FormControl>
            <FormControl size="sm">
              <FormLabel>Person</FormLabel>
              <SubjectPicker type="user" value={holdUser} onChange={setHoldUser} placeholder="Pick a person" testId="hold-user" />
            </FormControl>
            <FormControl size="sm" orientation="horizontal" sx={{ gap: 1 }}>
              <Switch checked={holdOrg} onChange={(e) => setHoldOrg(e.target.checked)} />
              <FormLabel>Whole organization</FormLabel>
            </FormControl>
            <Button size="sm" disabled={!holdName.trim() || (!holdOrg && !holdUser)} onClick={addHold} data-testid="hold-add">
              Place hold
            </Button>
          </Sheet>
          <Table size="sm" sx={{ maxWidth: 800 }} data-testid="holds">
            <tbody>
              {holds.map((h) => (
                <tr key={h.id}>
                  <td>{h.name}</td>
                  <td>{h.org_wide ? "whole organization" : `${h.user_ids.length} person`}</td>
                  <td>{h.released_at ? `released ${when(h.released_at)}` : "active"}</td>
                  <td>
                    {!h.released_at && (
                      <Button size="sm" variant="outlined" onClick={() => void api(`/api/v1/admin/lifecycle/holds/${h.id}`, { method: "DELETE" }).then(load)}>
                        Release
                      </Button>
                    )}
                  </td>
                </tr>
              ))}
              {holds.length === 0 && (
                <tr>
                  <td>No holds.</td>
                </tr>
              )}
            </tbody>
          </Table>
        </TabPanel>
        <TabPanel value="settings">
          <Sheet variant="outlined" sx={{ p: 2, borderRadius: "md", maxWidth: 640, display: "grid", gap: 1.25 }}>
            <FormControl size="sm">
              <FormLabel>Restore window for deleted people (days)</FormLabel>
              <Input size="sm" sx={{ width: 120 }} value={win} onChange={(e) => setWin(e.target.value)} slotProps={{ input: { "data-testid": "restore-days" } as never }} />
            </FormControl>
            <Button
              size="sm"
              sx={{ width: 120 }}
              onClick={() =>
                void api("/api/v1/admin/lifecycle/retention", { method: "PUT", json: { restore_window_days: Number(win), audit_retention_days: null } })
                  .then(() => setNotice("Retention saved."))
                  .catch((e) => setError((e as Error).message))
              }
              data-testid="retention-save"
            >
              Save
            </Button>
            <Typography level="title-sm" sx={{ mt: 1 }}>
              E-mail me about
            </Typography>
            <FormControl size="sm">
              <FormLabel>Recipients (comma separated)</FormLabel>
              <Input size="sm" value={alertTo} onChange={(e) => setAlertTo(e.target.value)} slotProps={{ input: { "data-testid": "alert-to" } as never }} />
            </FormControl>
            <Box sx={{ display: "flex", gap: 2, flexWrap: "wrap" }}>
              {opts?.alert_events.map((ev) => (
                <Checkbox key={ev} size="sm" label={ev} checked={alertEv.includes(ev)} onChange={(e) => setAlertEv(e.target.checked ? [...alertEv, ev] : alertEv.filter((x) => x !== ev))} />
              ))}
            </Box>
            <Button
              size="sm"
              sx={{ width: 120 }}
              onClick={() =>
                void api("/api/v1/admin/lifecycle/alerts", { method: "PUT", json: { recipients: alertTo.split(/[,\s]+/).filter(Boolean), events: alertEv } })
                  .then(() => setNotice("Alerts saved."))
                  .catch((e) => setError((e as Error).message))
              }
              data-testid="alerts-save"
            >
              Save alerts
            </Button>
          </Sheet>
        </TabPanel>
        <TabPanel value="history">
          <Table size="sm" sx={{ maxWidth: 900 }} data-testid="off-history">
            <tbody>
              {history.map((j) => (
                <tr key={j.id}>
                  <td>{when(j.created_at)}</td>
                  <td>{j.email}</td>
                  <td>
                    {j.kind} {j.status !== "done" && <Chip size="sm" color="warning">{j.status}</Chip>}
                  </td>
                  <td>{j.note}</td>
                </tr>
              ))}
            </tbody>
          </Table>
        </TabPanel>
      </Tabs>

      <Modal open={dlg === "delete"} onClose={() => setDlg(null)}>
        <ModalDialog sx={{ maxWidth: 520 }} data-testid="delete-dialog">
          <ModalClose />
          <Typography level="title-md">Delete {who?.name}</Typography>
          <Typography level="body-sm">Hand their content to someone before the account is removed. Nothing is transferred if you leave the person empty.</Typography>
          <FormControl size="sm">
            <FormLabel>Give their data to</FormLabel>
            <SubjectPicker type="user" value={heir} onChange={setHeir} placeholder="Pick a person" testId="heir" />
          </FormControl>
          {heir && (
            <>
              <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap" }}>
                {opts?.transfer_groups.map((g) => (
                  <Checkbox key={g.id} size="sm" label={`${g.id} (${g.includes.join(", ")})`} checked={!!groups[g.id]} onChange={(e) => setGroups({ ...groups, [g.id]: e.target.checked })} />
                ))}
              </Box>
              <Checkbox size="sm" label="Forward new mail to them" checked={forward} onChange={(e) => setForward(e.target.checked)} />
            </>
          )}
          <Checkbox size="sm" label="Remove them from group chats (direct messages stay)" checked={leave} onChange={(e) => setLeave(e.target.checked)} />
          <Button
            color="danger"
            data-testid="delete-confirm"
            onClick={() =>
              who &&
              void act(who.id, "delete", {
                transfer_to: heir?.id ?? "",
                transfer: heir ? Object.keys(groups).filter((k) => groups[k]) : [],
                forward_mail: !!heir && forward,
                leave_chats: leave,
                note: reason,
              })
            }
          >
            Delete the account
          </Button>
        </ModalDialog>
      </Modal>
    </Page>
  );
}
