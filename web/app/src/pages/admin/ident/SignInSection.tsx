// Sign-in security: session limits (enforced), login history with suspicious
// sign-in flags, per-person sign-out. Password rules and two-step sign-in live
// in Admin > Security (they are enforced by Zitadel).
import { useCallback, useEffect, useState } from "react";
import { Box, Button, Chip, FormControl, FormLabel, Input, Sheet, Switch, Typography } from "@mui/joy";
import { DataTable, type Column, type TablePage, type TableQuery } from "../../../components/DataTable";
import { SubjectPicker } from "../SubjectPicker";
import type { Subject } from "../accessApi";
import { Notice, Page, api, when } from "./common";
import { useNavigate } from "react-router-dom";

interface Settings {
  session_max_hours: number | null;
  session_idle_minutes: number | null;
  max_concurrent_sessions: number | null;
  alert_new_device: boolean;
  alert_failed_threshold: number;
}
interface Login {
  id: number;
  email: string;
  ip: string;
  user_agent: string;
  method: string;
  success: boolean;
  reason: string;
  suspicious: boolean;
  created_at: string;
}

function historyParams(q: TableQuery, forExport = false): string {
  const p = new URLSearchParams();
  if (q.q) p.set("email", q.q);
  if (q.filters.filter) p.set("filter", q.filters.filter);
  if (forExport) p.set("format", "csv");
  else {
    p.set("limit", String(q.limit));
    if (q.cursor) p.set("cursor", q.cursor);
  }
  return p.toString();
}

async function loadHistory(q: TableQuery): Promise<TablePage<Login>> {
  const h = await api<{ items: Login[]; next_cursor?: string }>(`/api/v1/admin/signin/history?${historyParams(q)}`);
  return { items: h.items ?? [], next_cursor: h.next_cursor };
}

const historyColumns: Column<Login>[] = [
  { key: "when", header: "When", width: 170, render: (e) => when(e.created_at) },
  { key: "person", header: "Person", render: (e) => e.email },
  { key: "ip", header: "Address", width: 130, render: (e) => e.ip },
  { key: "how", header: "How", width: 100, render: (e) => e.method },
  {
    key: "result",
    header: "Result",
    width: 230,
    render: (e) => (
      <>
        <Chip size="sm" color={e.success ? "success" : "warning"}>
          {e.success ? "signed in" : "failed"}
        </Chip>{" "}
        {e.suspicious && (
          <Chip size="sm" color="danger" data-testid="suspicious-chip">
            suspicious
          </Chip>
        )}{" "}
        {e.reason}
      </>
    ),
  },
];
const num = (v: string): number | null => (v.trim() === "" ? null : Number(v));

export function SignInSection() {
  const nav = useNavigate();
  const [s, setS] = useState<Settings | null>(null);
  const [vals, setVals] = useState({ max: "", idle: "", conc: "", fail: "5" });
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [who, setWho] = useState<Subject | null>(null);

  const load = useCallback(async () => {
    try {
      const st = await api<Settings>("/api/v1/admin/signin/settings");
      setS(st);
      setVals({ max: st.session_max_hours?.toString() ?? "", idle: st.session_idle_minutes?.toString() ?? "", conc: st.max_concurrent_sessions?.toString() ?? "", fail: String(st.alert_failed_threshold) });
    } catch (e) {
      setError((e as Error).message);
    }
  }, []);
  useEffect(() => {
    void load();
  }, [load]);

  async function save(alertNew = s?.alert_new_device ?? true) {
    setError(null);
    setNotice(null);
    try {
      await api("/api/v1/admin/signin/settings", {
        method: "PUT",
        json: { session_max_hours: num(vals.max), session_idle_minutes: num(vals.idle), max_concurrent_sessions: num(vals.conc), alert_new_device: alertNew, alert_failed_threshold: Number(vals.fail) },
      });
      setNotice("Session policy saved. It applies to the next request of every signed-in person.");
      await load();
    } catch (e) {
      setError((e as Error).message);
    }
  }
  async function signOut() {
    if (!who) return;
    try {
      const r = await api<{ signed_out: number }>(`/api/v1/admin/signin/users/${who.id}/sign-out`, { method: "POST" });
      setNotice(`${who.name} was signed out of ${r.signed_out} session(s).`);
    } catch (e) {
      setError((e as Error).message);
    }
  }

  return (
    <Page testId="signin" title="Sign-in security" intro="Every setting here is enforced by the server. Password rules and two-step sign-in are enforced by Zitadel and are set under Security.">
      <Notice error={error} notice={notice} />
      <Sheet variant="outlined" sx={{ p: 2, borderRadius: "md", maxWidth: 680, mb: 3, display: "grid", gap: 1.25 }}>
        <Typography level="title-sm">Sessions</Typography>
        <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap" }}>
          <FormControl size="sm">
            <FormLabel>Sign out after (hours)</FormLabel>
            <Input size="sm" sx={{ width: 130 }} placeholder="no limit" value={vals.max} onChange={(e) => setVals({ ...vals, max: e.target.value })} slotProps={{ input: { "data-testid": "sess-max" } as never }} />
          </FormControl>
          <FormControl size="sm">
            <FormLabel>Idle timeout (minutes)</FormLabel>
            <Input size="sm" sx={{ width: 130 }} placeholder="no limit" value={vals.idle} onChange={(e) => setVals({ ...vals, idle: e.target.value })} slotProps={{ input: { "data-testid": "sess-idle" } as never }} />
          </FormControl>
          <FormControl size="sm">
            <FormLabel>Sessions per person</FormLabel>
            <Input size="sm" sx={{ width: 130 }} placeholder="no limit" value={vals.conc} onChange={(e) => setVals({ ...vals, conc: e.target.value })} slotProps={{ input: { "data-testid": "sess-conc" } as never }} />
          </FormControl>
          <FormControl size="sm">
            <FormLabel>Alert after failed sign-ins</FormLabel>
            <Input size="sm" sx={{ width: 130 }} value={vals.fail} onChange={(e) => setVals({ ...vals, fail: e.target.value })} />
          </FormControl>
        </Box>
        <FormControl size="sm" orientation="horizontal" sx={{ gap: 1 }}>
          <Switch checked={s?.alert_new_device ?? true} onChange={(e) => void save(e.target.checked)} />
          <FormLabel>Flag and e-mail admins about a sign-in from a new device and address</FormLabel>
        </FormControl>
        <Box sx={{ display: "flex", gap: 1 }}>
          <Button size="sm" onClick={() => void save()} data-testid="sess-save">
            Save
          </Button>
          <Button size="sm" variant="plain" onClick={() => nav("/admin/security")}>
            Password and two-step rules
          </Button>
        </Box>
        <Typography level="body-xs" sx={{ opacity: 0.7 }}>
          The idle timeout is measured from the last activity, which is recorded every 5 minutes.
        </Typography>
      </Sheet>

      <Typography level="title-md" sx={{ mb: 1 }}>
        Sign out a person everywhere
      </Typography>
      <Box sx={{ display: "flex", gap: 1, mb: 3, alignItems: "flex-end" }}>
        <SubjectPicker type="user" value={who} onChange={setWho} placeholder="Pick a person" testId="signout-user" />
        <Button size="sm" color="danger" variant="outlined" disabled={!who} onClick={signOut} data-testid="signout-go">
          Sign out of all sessions
        </Button>
      </Box>

      <Typography level="title-md" sx={{ mb: 1 }}>
        Login history
      </Typography>
      <DataTable<Login>
        testId="history"
        columns={historyColumns}
        rowKey={(e) => String(e.id)}
        load={loadHistory}
        mode="feed"
        pageSizes={[50, 100]}
        searchPlaceholder="Search by email"
        filters={[
          {
            key: "filter",
            label: "Result",
            type: "select",
            options: [
              { value: "suspicious", label: "Suspicious only" },
              { value: "failed", label: "Failed only" },
            ],
          },
        ]}
        exportUrl={(q) => `/api/v1/admin/signin/history?${historyParams(q, true)}`}
        emptyText="Nothing recorded yet."
        maxHeight={480}
      />
    </Page>
  );
}
