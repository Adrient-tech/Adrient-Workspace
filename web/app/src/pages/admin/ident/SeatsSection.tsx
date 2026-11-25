// Seats, plan, per-app licences and bulk CSV invitations.
import { useCallback, useEffect, useRef, useState } from "react";
import { Box, Button, Chip, FormControl, FormLabel, Input, Sheet, Switch, Table, Typography } from "@mui/joy";
import { apps as catalog } from "../../../catalog/apps";
import { SubjectPicker } from "../SubjectPicker";
import type { Subject } from "../accessApi";
import { Notice, Page, api } from "./common";

interface Usage {
  plan: string;
  seats: number | null;
  used: number;
  pending_invites: number;
  free: number | null;
  guests: number;
}
interface AppUse {
  app_id: string;
  cap: number | null;
  assigned: number;
  over: boolean;
}
interface Row {
  line: number;
  email: string;
  status: string;
  reason?: string;
  email_sent?: boolean;
}
const appName = (id: string) => catalog.find((a) => a.id === id)?.name ?? id;

export function SeatsSection() {
  const [usage, setUsage] = useState<Usage | null>(null);
  const [apps, setApps] = useState<AppUse[]>([]);
  const [seats, setSeats] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [rows, setRows] = useState<Row[]>([]);
  const [dry, setDry] = useState(true);
  const [pickApp, setPickApp] = useState("");
  const [pickUser, setPickUser] = useState<Subject | null>(null);
  const file = useRef<HTMLInputElement>(null);

  const load = useCallback(async () => {
    try {
      const [u, r] = await Promise.all([api<Usage>("/api/v1/admin/seats/usage"), api<{ apps: AppUse[] }>("/api/v1/admin/seats/report")]);
      setUsage(u);
      setApps(r.apps);
      setSeats(u.seats == null ? "" : String(u.seats));
    } catch (e) {
      setError((e as Error).message);
    }
  }, []);
  useEffect(() => {
    void load();
  }, [load]);

  async function savePlan() {
    setError(null);
    try {
      await api("/api/v1/admin/seats/plan", { method: "PUT", json: { plan: usage?.plan ?? "standard", seats: seats.trim() === "" ? null : Number(seats) } });
      setNotice("Seat count saved.");
      await load();
    } catch (e) {
      setError((e as Error).message);
    }
  }
  async function saveCap(appId: string, v: string) {
    setError(null);
    try {
      await api(`/api/v1/admin/seats/caps/${appId}`, { method: "PUT", json: { seats: v.trim() === "" ? null : Number(v) } });
      await load();
    } catch (e) {
      setError((e as Error).message);
    }
  }
  async function assign(on: boolean) {
    if (!pickApp || !pickUser) return;
    setError(null);
    setNotice(null);
    try {
      await api("/api/v1/admin/seats/assign", { method: "POST", json: { app_id: pickApp, subject_type: "user", subject_id: pickUser.id, assign: on } });
      setNotice(`${appName(pickApp)} ${on ? "given to" : "taken from"} ${pickUser.name}.`);
      await load();
    } catch (e) {
      setError((e as Error).message);
    }
  }
  async function upload() {
    const f = file.current?.files?.[0];
    if (!f) return;
    setError(null);
    setNotice(null);
    const fd = new FormData();
    fd.append("file", f);
    try {
      const r = await fetch(`/api/v1/admin/seats/invite-csv${dry ? "?dry_run=1" : ""}`, { method: "POST", body: fd, credentials: "same-origin" });
      const b = (await r.json()) as { rows?: Row[]; error?: string };
      if (!r.ok) throw new Error(b.error ?? `HTTP ${r.status}`);
      setRows(b.rows ?? []);
      const ok = (b.rows ?? []).filter((x) => x.status === "invited").length;
      setNotice(dry ? `Checked ${b.rows?.length} lines: ${ok} would be invited. Nothing was sent.` : `${ok} invitations created.`);
      if (!dry) await load();
    } catch (e) {
      setError((e as Error).message);
    }
  }

  return (
    <Page testId="seats" title="Seats and licences" intro="A seat is an active member. Guests, external members and service accounts do not use one. When every seat is taken, SCIM, first sign-in, restore and invitations are refused until one frees up.">
      <Notice error={error} notice={notice} />
      {usage && (
        <Sheet variant="outlined" sx={{ p: 2, borderRadius: "md", maxWidth: 640, mb: 3 }} data-testid="seat-usage">
          <Box sx={{ display: "flex", gap: 3, flexWrap: "wrap", alignItems: "flex-end" }}>
            <Box>
              <Typography level="h2" data-testid="seat-used">
                {usage.used}
                <Typography level="body-md"> of {usage.seats ?? "unlimited"} seats used</Typography>
              </Typography>
              <Typography level="body-sm">
                {usage.pending_invites} open invitations · {usage.guests} guests (no seat)
              </Typography>
            </Box>
            <FormControl size="sm">
              <FormLabel>Seats in the plan (empty = unlimited)</FormLabel>
              <Box sx={{ display: "flex", gap: 1 }}>
                <Input size="sm" value={seats} onChange={(e) => setSeats(e.target.value)} sx={{ width: 120 }} slotProps={{ input: { "data-testid": "seat-input", inputMode: "numeric" } as never }} />
                <Button size="sm" onClick={savePlan} data-testid="seat-save">
                  Save
                </Button>
              </Box>
            </FormControl>
          </Box>
        </Sheet>
      )}

      <Typography level="title-md" sx={{ mb: 1 }}>
        Licence usage by app
      </Typography>
      <Table size="sm" sx={{ maxWidth: 640, mb: 2 }} data-testid="license-table">
        <thead>
          <tr>
            <th>App</th>
            <th>People with access</th>
            <th>Seat cap (empty = none)</th>
          </tr>
        </thead>
        <tbody>
          {apps.map((a) => (
            <tr key={a.app_id} data-testid={`license-${a.app_id}`}>
              <td>{appName(a.app_id)}</td>
              <td>
                {a.assigned} {a.over && <Chip size="sm" color="danger">over the cap</Chip>}
              </td>
              <td>
                <Input
                  size="sm"
                  defaultValue={a.cap ?? ""}
                  key={`${a.app_id}-${a.cap}`}
                  sx={{ width: 90 }}
                  onBlur={(e) => {
                    const v = e.target.value;
                    if (v !== String(a.cap ?? "")) void saveCap(a.app_id, v);
                  }}
                  slotProps={{ input: { "data-testid": `cap-${a.app_id}`, inputMode: "numeric" } as never }}
                />
              </td>
            </tr>
          ))}
        </tbody>
      </Table>

      <Typography level="title-md" sx={{ mb: 1 }}>
        Give or take an app
      </Typography>
      <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", alignItems: "flex-end", mb: 3 }}>
        <FormControl size="sm">
          <FormLabel>App</FormLabel>
          <select value={pickApp} onChange={(e) => setPickApp(e.target.value)} data-testid="assign-app" style={{ padding: 6 }}>
            <option value="">Choose…</option>
            {apps.map((a) => (
              <option key={a.app_id} value={a.app_id}>
                {appName(a.app_id)}
              </option>
            ))}
          </select>
        </FormControl>
        <FormControl size="sm">
          <FormLabel>Person</FormLabel>
          <SubjectPicker type="user" value={pickUser} onChange={setPickUser} placeholder="Pick a person" testId="assign-user" />
        </FormControl>
        <Button size="sm" disabled={!pickApp || !pickUser} onClick={() => void assign(true)} data-testid="assign-give">
          Give access
        </Button>
        <Button size="sm" variant="outlined" disabled={!pickApp || !pickUser} onClick={() => void assign(false)}>
          Take away
        </Button>
        <Typography level="body-xs" sx={{ width: "100%", opacity: 0.7 }}>
          This edits the same rules as Admin &gt; App access. A person can also get an app through a group, department or role.
        </Typography>
      </Box>

      <Typography level="title-md" sx={{ mb: 0.5 }}>
        Invite many people (CSV)
      </Typography>
      <Typography level="body-sm" sx={{ mb: 1, opacity: 0.8 }}>
        One person per line: <code>email,name</code>. Up to 500 lines. Invitations that would go past your free seats are refused.
      </Typography>
      <Box sx={{ display: "flex", gap: 2, alignItems: "center", mb: 1.5, flexWrap: "wrap" }}>
        <input type="file" accept=".csv,text/csv,text/plain" ref={file} data-testid="csv-file" />
        <FormControl size="sm" orientation="horizontal" sx={{ gap: 1 }}>
          <Switch checked={dry} onChange={(e) => setDry(e.target.checked)} />
          <FormLabel>Check only, do not invite</FormLabel>
        </FormControl>
        <Button size="sm" onClick={upload} data-testid="csv-go">
          {dry ? "Check file" : "Send invitations"}
        </Button>
      </Box>
      {rows.length > 0 && (
        <Table size="sm" sx={{ maxWidth: 760 }} data-testid="csv-result">
          <thead>
            <tr>
              <th>Line</th>
              <th>E-mail</th>
              <th>Result</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <tr key={r.line}>
                <td>{r.line}</td>
                <td>{r.email}</td>
                <td>
                  <Chip size="sm" color={r.status === "invited" ? "success" : r.status === "skipped" ? "neutral" : "danger"}>
                    {r.status}
                  </Chip>{" "}
                  {r.reason}
                  {r.status === "invited" && !dry && (r.email_sent ? " · e-mail sent" : " · link only (no e-mail configured)")}
                </td>
              </tr>
            ))}
          </tbody>
        </Table>
      )}
    </Page>
  );
}
