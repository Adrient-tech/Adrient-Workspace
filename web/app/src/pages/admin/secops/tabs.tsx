// The smaller tabs of the console: people, sign-ins, action log, reports. The
// lists, settings, roles and approvals tabs live in tabs2.tsx.
import { useCallback, useEffect, useState } from "react";
import { Alert, Box, Button, Chip, FormControl, FormLabel, Input, Option, Select, Sheet, Table, Typography } from "@mui/joy";
import { A, iam, type ActionRow, type Person, type SignIn } from "./api";
import { ActionTable } from "./UserDrawer";
import { ago } from "./logic";

export { ListsTab, SettingsTab, RolesTab, ApprovalsTab } from "./tabs2";

const when = (s?: string | null) => (s ? new Date(s).toLocaleString() : "never");

export function PeopleTab({ onOpen, refreshKey }: { onOpen: (id: string) => void; refreshKey: number }) {
  const [items, setItems] = useState<Person[]>([]);
  const [next, setNext] = useState("");
  const [total, setTotal] = useState(0);
  const [q, setQ] = useState("");
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(false);
  const load = useCallback(
    async (cursor = "") => {
      setLoading(true);
      try {
        const p = new URLSearchParams({ limit: "25", q });
        if (cursor) p.set("cursor", cursor);
        const r = await iam<{ items: Person[]; next_cursor: string; total: number }>(`${A}/users?${p}`);
        setItems((o) => (cursor ? [...o, ...r.items] : r.items));
        setNext(r.next_cursor);
        setTotal(r.total);
      } catch (e) {
        setErr((e as Error).message);
      } finally {
        setLoading(false);
      }
    },
    [q],
  );
  useEffect(() => {
    const t = setTimeout(() => void load(), 250);
    return () => clearTimeout(t);
  }, [load, refreshKey]);
  return (
    <Box data-testid="people-tab">
      <Box sx={{ display: "flex", gap: 1, mb: 1.5, alignItems: "center" }}>
        <Input size="sm" placeholder="Search by name or e-mail" value={q} onChange={(e) => setQ(e.target.value)} sx={{ width: 320 }} slotProps={{ input: { "data-testid": "people-search" } }} />
        <Box sx={{ flex: 1 }} />
        <Typography level="body-sm">{total} people</Typography>
      </Box>
      {err && (
        <Alert color="danger" sx={{ mb: 1 }}>
          {err}
        </Alert>
      )}
      <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
        <Table hoverRow stickyHeader sx={{ tableLayout: "auto", "& td, & th": { whiteSpace: "nowrap" } }}>
          <thead>
            <tr>
              <th>Person</th>
              <th>Status</th>
              <th>Last sign-in</th>
              <th>Sessions</th>
              <th>Open alerts</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {items.map((p) => (
              <tr key={p.id} onClick={() => onOpen(p.id)} style={{ cursor: "pointer" }} data-testid="person-row">
                <td>
                  <Typography level="title-sm">{p.name || p.email}</Typography>
                  <Typography level="body-xs">{p.email}</Typography>
                </td>
                <td>
                  <Chip size="sm" variant="soft" color={p.status === "active" ? "success" : "danger"}>
                    {p.status}
                  </Chip>
                  {p.is_admin && (
                    <Chip size="sm" variant="soft" color="primary" sx={{ ml: 0.5 }}>
                      admin
                    </Chip>
                  )}
                  {p.quarantined && (
                    <Chip size="sm" variant="soft" color="warning" sx={{ ml: 0.5 }}>
                      read only
                    </Chip>
                  )}
                </td>
                <td>{ago(p.last_sign_in)}</td>
                <td>{p.sessions}</td>
                <td>
                  {p.open_alerts > 0 ? (
                    <Chip size="sm" color="danger" variant="solid">
                      {p.open_alerts}
                    </Chip>
                  ) : (
                    0
                  )}
                </td>
                <td style={{ textAlign: "right" }}>
                  <Button size="sm" variant="plain">
                    Security
                  </Button>
                </td>
              </tr>
            ))}
            {!items.length && !loading && (
              <tr>
                <td colSpan={6}>
                  <Typography level="body-sm" sx={{ p: 2 }}>
                    Nobody matches.
                  </Typography>
                </td>
              </tr>
            )}
          </tbody>
        </Table>
      </Sheet>
      {next && (
        <Button sx={{ mt: 1 }} variant="outlined" loading={loading} onClick={() => void load(next)}>
          Load more
        </Button>
      )}
    </Box>
  );
}

export function EventsTab({ onOpenUser }: { onOpenUser: (id: string) => void }) {
  const [items, setItems] = useState<SignIn[]>([]);
  const [next, setNext] = useState("");
  const [q, setQ] = useState("");
  const [result, setResult] = useState("");
  const [err, setErr] = useState("");
  const load = useCallback(
    async (cursor = "") => {
      try {
        const p = new URLSearchParams({ limit: "50", q });
        if (result) p.set("result", result);
        if (cursor) p.set("cursor", cursor);
        const r = await iam<{ items: SignIn[]; next_cursor: string }>(`${A}/events?${p}`);
        setItems((o) => (cursor ? [...o, ...r.items] : r.items));
        setNext(r.next_cursor);
      } catch (e) {
        setErr((e as Error).message);
      }
    },
    [q, result],
  );
  useEffect(() => {
    const t = setTimeout(() => void load(), 250);
    return () => clearTimeout(t);
  }, [load]);
  return (
    <Box data-testid="events-tab">
      <Box sx={{ display: "flex", gap: 1, mb: 1.5 }}>
        <Input size="sm" placeholder="Search e-mail or address" value={q} onChange={(e) => setQ(e.target.value)} sx={{ width: 300 }} />
        <Select size="sm" placeholder="All results" value={result || null} onChange={(_, v) => setResult(v ?? "")} sx={{ width: 180 }}>
          <Option value="ok">Successful</Option>
          <Option value="failed">Failed</Option>
          <Option value="suspicious">Flagged</Option>
        </Select>
      </Box>
      {err && (
        <Alert color="danger" sx={{ mb: 1 }}>
          {err}
        </Alert>
      )}
      <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
        <Table hoverRow stickyHeader sx={{ tableLayout: "auto", "& td, & th": { whiteSpace: "nowrap" } }}>
          <thead>
            <tr>
              <th>When</th>
              <th>Account</th>
              <th>Result</th>
              <th>Address</th>
              <th>Place</th>
              <th>Method</th>
              <th>Note</th>
            </tr>
          </thead>
          <tbody>
            {items.map((e) => (
              <tr key={e.id} onClick={() => e.user_id && onOpenUser(e.user_id)} style={{ cursor: e.user_id ? "pointer" : "default" }}>
                <td>{when(e.at)}</td>
                <td>{e.email}</td>
                <td>
                  <Chip size="sm" variant="soft" color={e.success ? "success" : "danger"}>
                    {e.success ? "ok" : "failed"}
                  </Chip>
                </td>
                <td>{e.ip}</td>
                <td>{[e.city, e.country].filter(Boolean).join(", ") || "-"}</td>
                <td>
                  {e.method}
                  {e.mfa_result ? ` / ${e.mfa_result}` : ""}
                </td>
                <td style={{ whiteSpace: "normal", maxWidth: 360 }}>{e.reason}</td>
              </tr>
            ))}
          </tbody>
        </Table>
      </Sheet>
      {next && (
        <Button sx={{ mt: 1 }} variant="outlined" onClick={() => void load(next)}>
          Load more
        </Button>
      )}
    </Box>
  );
}

interface Rep {
  key: string;
  title: string;
  columns: string[];
  rows: string[][];
  note?: string;
}
const REPORTS: [string, string][] = [
  ["privileged_no_mfa", "Privileged accounts without a second step"],
  ["dormant", "No sign-in for N days"],
  ["stale_sessions", "Stale sessions"],
  ["coverage", "Policy coverage"],
  ["recent_resets", "Recent resets and who did them"],
];

export function ReportsTab() {
  const [key, setKey] = useState("privileged_no_mfa");
  const [days, setDays] = useState("90");
  const [rep, setRep] = useState<Rep | null>(null);
  const [err, setErr] = useState("");
  useEffect(() => {
    setRep(null);
    iam<Rep>(`${A}/reports/${key}?days=${Number(days) || 90}`)
      .then(setRep)
      .catch((e) => setErr((e as Error).message));
  }, [key, days]);
  return (
    <Box data-testid="reports-tab">
      <Box sx={{ display: "flex", gap: 1, mb: 1.5, alignItems: "flex-end" }}>
        <FormControl>
          <FormLabel>Report</FormLabel>
          <Select size="sm" value={key} onChange={(_, v) => v && setKey(v)} sx={{ width: 380 }}>
            {REPORTS.map(([k, l]) => (
              <Option key={k} value={k}>
                {l}
              </Option>
            ))}
          </Select>
        </FormControl>
        <FormControl>
          <FormLabel>Days</FormLabel>
          <Input size="sm" type="number" value={days} onChange={(e) => setDays(e.target.value)} sx={{ width: 90 }} />
        </FormControl>
        <Button size="sm" variant="outlined" component="a" href={`${A}/reports/${key}?days=${Number(days) || 90}&format=csv`} data-testid="report-csv">
          Export CSV
        </Button>
      </Box>
      {err && (
        <Alert color="danger" sx={{ mb: 1 }}>
          {err}
        </Alert>
      )}
      {rep && (
        <>
          <Typography level="title-md">
            {rep.title} ({rep.rows.length})
          </Typography>
          {rep.note && (
            <Typography level="body-sm" sx={{ opacity: 0.7, mb: 1 }}>
              {rep.note}
            </Typography>
          )}
          <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto", maxHeight: 520 }}>
            <Table stickyHeader>
              <thead>
                <tr>
                  {rep.columns.map((c) => (
                    <th key={c}>{c}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {rep.rows.slice(0, 500).map((r, i) => (
                  <tr key={i}>
                    {r.map((c, j) => (
                      <td key={j}>{c}</td>
                    ))}
                  </tr>
                ))}
                {!rep.rows.length && (
                  <tr>
                    <td colSpan={rep.columns.length}>
                      <Typography level="body-sm" sx={{ p: 2 }}>
                        Nothing to report.
                      </Typography>
                    </td>
                  </tr>
                )}
              </tbody>
            </Table>
          </Sheet>
        </>
      )}
    </Box>
  );
}

export function ActionLogTab() {
  const [rows, setRows] = useState<ActionRow[]>([]);
  const [next, setNext] = useState("");
  const [err, setErr] = useState("");
  const load = useCallback(async (c = "") => {
    try {
      const r = await iam<{ items: ActionRow[]; next_cursor: string }>(`${A}/actions?limit=50${c ? `&cursor=${c}` : ""}`);
      setRows((o) => (c ? [...o, ...r.items] : r.items));
      setNext(r.next_cursor);
    } catch (e) {
      setErr((e as Error).message);
    }
  }, []);
  useEffect(() => void load(), [load]);
  return (
    <Box data-testid="log-tab">
      {err && (
        <Alert color="danger" sx={{ mb: 1 }}>
          {err}
        </Alert>
      )}
      <ActionTable rows={rows} />
      {next && (
        <Button sx={{ mt: 1 }} variant="outlined" onClick={() => void load(next)}>
          Load more
        </Button>
      )}
    </Box>
  );
}
