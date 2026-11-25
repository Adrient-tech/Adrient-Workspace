// My security: my sessions, sign-in history, second steps and password, plus
// "this wasn't me" for any sign-in. Backed by /api/v1/iam/my (internal/iamops).
import { useCallback, useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { Alert, Box, Button, Chip, Container, FormControl, FormLabel, Input, Sheet, Table, Typography } from "@mui/joy";
import { IamError, iam, MY } from "../admin/secops/api";
import type { Factor, SessionRow, SignIn } from "../admin/secops/api";

interface My {
  sessions: SessionRow[];
  history: SignIn[];
  factors: Factor[];
  factors_error?: string;
  email: string;
  quarantined: boolean;
}

const when = (s?: string | null) => (s ? new Date(s).toLocaleString() : "");

export default function MySecurityPage(_props: { user?: unknown }) {
  const [params] = useSearchParams();
  const [d, setD] = useState<My | null>(null);
  const [msg, setMsg] = useState("");
  const [err, setErr] = useState("");
  const [report, setReport] = useState<SignIn | null>(null);
  const [pw, setPw] = useState({ current: "", next: "" });
  const [totp, setTotp] = useState<{ secret: string; uri: string } | null>(null);
  const [code, setCode] = useState("");
  const [reauth, setReauth] = useState("");
  const load = useCallback(async () => {
    try {
      setD(await iam<My>(MY));
    } catch (e) {
      setErr((e as Error).message);
    }
  }, []);
  useEffect(() => void load(), [load]);
  const flag = Number(params.get("event") || 0);
  useEffect(() => {
    if (d && flag) setReport(d.history.find((h) => h.id === flag) ?? null);
  }, [d, flag]);

  const run = async (fn: () => Promise<unknown>, ok: string) => {
    setErr("");
    setMsg("");
    try {
      await fn();
      setMsg(ok);
      await load();
    } catch (e) {
      setErr(e instanceof IamError ? e.message : String(e));
    }
  };
  const send = (verdict: string) =>
    run(async () => {
      await iam(`${MY}/report`, { method: "POST", json: { event_id: report?.id ?? 0, verdict } });
      setReport(null);
    }, verdict === "was_me" ? "Thanks. Marked as you." : "Reported. We signed out your other sessions and told the security team.");

  return (
    <Container maxWidth="lg" sx={{ py: 3 }} data-testid="my-security">
      <Typography level="h3">My security</Typography>
      <Typography level="body-sm" sx={{ opacity: 0.7, mb: 2 }}>
        Where you are signed in, your recent sign-ins and your second steps. Report anything you do not recognise.
      </Typography>
      {msg && (
        <Alert color="success" sx={{ mb: 1.5 }} data-testid="my-msg">
          {msg}
        </Alert>
      )}
      {err && (
        <Alert color="danger" sx={{ mb: 1.5 }} data-testid="my-err">
          {err}
        </Alert>
      )}
      {d?.quarantined && (
        <Alert color="warning" sx={{ mb: 1.5 }}>
          Your account is read-only while a security check runs. Contact your IT team.
        </Alert>
      )}
      {report && (
        <Alert color="warning" variant="soft" sx={{ mb: 2, display: "block" }} data-testid="was-this-you">
          <Typography level="title-md">New sign-in from {[report.city, report.country].filter(Boolean).join(", ") || report.ip}. Was this you?</Typography>
          <Typography level="body-sm">
            {when(report.at)} from {report.ip}
          </Typography>
          <Box sx={{ display: "flex", gap: 1, mt: 1 }}>
            <Button size="sm" variant="outlined" onClick={() => void send("was_me")} data-testid="was-me">
              Yes, it was me
            </Button>
            <Button size="sm" color="danger" onClick={() => void send("not_me")} data-testid="not-me">
              No, this wasn't me
            </Button>
          </Box>
        </Alert>
      )}

      <Sheet variant="outlined" sx={{ p: 2, borderRadius: "md", mb: 2 }}>
        <Box sx={{ display: "flex", alignItems: "center", mb: 1 }}>
          <Typography level="title-md" sx={{ flex: 1 }}>
            Where you are signed in
          </Typography>
          <Button size="sm" variant="outlined" color="danger" onClick={() => void run(() => iam(`${MY}/sessions/revoke-others`, { method: "POST", json: {} }), "Signed out of your other sessions.")} data-testid="signout-others">
            Sign out everywhere else
          </Button>
        </Box>
        <Table size="sm" data-testid="my-sessions">
          <thead>
            <tr>
              <th>Device</th>
              <th>Address</th>
              <th>Place</th>
              <th>Last active</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {(d?.sessions ?? []).map((s) => (
              <tr key={s.id}>
                <td>
                  {s.device}
                  {s.current && (
                    <Chip size="sm" sx={{ ml: 0.5 }}>
                      this device
                    </Chip>
                  )}
                </td>
                <td>{s.ip}</td>
                <td>{[s.city, s.country].filter(Boolean).join(", ") || "-"}</td>
                <td>{when(s.last_seen_at ?? s.created_at)}</td>
                <td style={{ textAlign: "right" }}>
                  {!s.current && (
                    <Button size="sm" variant="plain" color="danger" onClick={() => void run(() => iam(`${MY}/sessions/${s.id}/revoke`, { method: "POST", json: {} }), "Session ended.")}>
                      Sign out
                    </Button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </Table>
      </Sheet>

      <Sheet variant="outlined" sx={{ p: 2, borderRadius: "md", mb: 2 }}>
        <Typography level="title-md">Second steps</Typography>
        {d?.factors_error && <Typography level="body-sm">{d.factors_error}</Typography>}
        {d && d.factors.length === 0 && !d.factors_error && <Typography level="body-sm">You have no second step. Add an authenticator app below.</Typography>}
        <Box sx={{ my: 1 }}>
          {(d?.factors ?? []).map((f) => (
            <Box key={f.kind + f.id} sx={{ display: "flex", alignItems: "center", gap: 1, py: 0.5 }} data-testid="my-factor">
              <Typography sx={{ flex: 1 }}>{f.name}</Typography>
              <Chip size="sm" variant="soft">
                {f.kind}
              </Chip>
              <Button size="sm" variant="plain" color="danger" disabled={!reauth} onClick={() => void run(() => iam(`${MY}/factors/remove`, { method: "POST", json: { kind: f.kind, id: f.id, password: reauth } }), "Second step removed.")}>
                Remove
              </Button>
            </Box>
          ))}
        </Box>
        <FormControl sx={{ maxWidth: 360, mb: 1 }}>
          <FormLabel>Your password (needed to add or remove a second step)</FormLabel>
          <Input type="password" value={reauth} onChange={(e) => setReauth(e.target.value)} slotProps={{ input: { "data-testid": "my-reauth" } }} />
        </FormControl>
        {!totp ? (
          <Button
            size="sm"
            disabled={!reauth}
            data-testid="totp-start"
            onClick={() =>
              void run(async () => {
                setTotp(await iam<{ secret: string; uri: string }>(`${MY}/factors/totp/start`, { method: "POST", json: { password: reauth } }));
              }, "Enter the code from your app to finish.")
            }
          >
            Add authenticator app
          </Button>
        ) : (
          <Box>
            <Typography level="body-sm">Add this key to your authenticator app, then enter the 6-digit code.</Typography>
            <Typography level="title-md" sx={{ fontFamily: "monospace", userSelect: "all" }}>
              {totp.secret}
            </Typography>
            <Box sx={{ display: "flex", gap: 1, mt: 1 }}>
              <Input size="sm" placeholder="123456" value={code} onChange={(e) => setCode(e.target.value)} slotProps={{ input: { "data-testid": "totp-code" } }} />
              <Button
                size="sm"
                onClick={() =>
                  void run(async () => {
                    await iam(`${MY}/factors/totp/verify`, { method: "POST", json: { code } });
                    setTotp(null);
                    setCode("");
                  }, "Authenticator app added.")
                }
              >
                Verify
              </Button>
            </Box>
          </Box>
        )}
      </Sheet>

      <Sheet variant="outlined" sx={{ p: 2, borderRadius: "md", mb: 2 }}>
        <Typography level="title-md">Change password</Typography>
        <Box sx={{ display: "flex", gap: 1, my: 1, flexWrap: "wrap" }}>
          <Input type="password" placeholder="Current password" value={pw.current} onChange={(e) => setPw({ ...pw, current: e.target.value })} />
          <Input type="password" placeholder="New password" value={pw.next} onChange={(e) => setPw({ ...pw, next: e.target.value })} />
          <Button
            disabled={!pw.current || pw.next.length < 8}
            onClick={() =>
              void run(async () => {
                await iam(`${MY}/password`, { method: "POST", json: { current: pw.current, new: pw.next } });
                setPw({ current: "", next: "" });
              }, "Password changed.")
            }
          >
            Change
          </Button>
        </Box>
      </Sheet>

      <Sheet variant="outlined" sx={{ p: 2, borderRadius: "md" }}>
        <Typography level="title-md">Recent sign-ins</Typography>
        <Table size="sm" data-testid="my-history">
          <thead>
            <tr>
              <th>When</th>
              <th>Result</th>
              <th>Address</th>
              <th>Place</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {(d?.history ?? []).map((h) => (
              <tr key={h.id}>
                <td>{when(h.at)}</td>
                <td>
                  <Chip size="sm" variant="soft" color={h.success ? "success" : "danger"}>
                    {h.success ? "ok" : "failed"}
                  </Chip>
                </td>
                <td>{h.ip}</td>
                <td>{[h.city, h.country].filter(Boolean).join(", ") || "-"}</td>
                <td style={{ textAlign: "right" }}>
                  {h.success && (
                    <Button size="sm" variant="plain" color="danger" onClick={() => setReport(h)} data-testid="report-event">
                      This wasn't me
                    </Button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </Table>
      </Sheet>
    </Container>
  );
}
