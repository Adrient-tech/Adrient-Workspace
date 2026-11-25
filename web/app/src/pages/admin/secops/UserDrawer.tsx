// Per-user Security tab: live factors, sessions, tokens, recent sign-ins, alerts and
// the actions the caller's role allows. Opens as a side panel from the people list.
import { useCallback, useEffect, useState } from "react";
import { Alert, Box, Button, Chip, CircularProgress, Divider, Drawer, IconButton, Sheet, Table, Typography } from "@mui/joy";
import CloseIcon from "@mui/icons-material/Close";
import { A, iam, type ActionRow, type SecurityView } from "./api";
import { ActionDialog } from "./ActionDialog";
import { ACTIONS, ago, can, SEVERITY_COLOR, type ActionDef } from "./logic";

const GROUPS: ActionDef["group"][] = ["Password", "Second step", "Sessions and tokens", "Account"];

const when = (s?: string | null) => (s ? new Date(s).toLocaleString() : "never");

export function UserDrawer({ userId, onClose, onChanged }: { userId: string | null; onClose: () => void; onChanged: () => void }) {
  const [v, setV] = useState<SecurityView | null>(null);
  const [err, setErr] = useState("");
  const [dlg, setDlg] = useState<{ def: ActionDef; extra?: Record<string, unknown> } | null>(null);

  const load = useCallback(async () => {
    if (!userId) return;
    setErr("");
    try {
      setV(await iam<SecurityView>(`${A}/users/${userId}/security`));
    } catch (e) {
      setErr((e as Error).message);
    }
  }, [userId]);
  useEffect(() => {
    setV(null);
    void load();
  }, [load]);

  const caps = v?.capabilities;
  const allowed = v?.can.allowed ?? false;
  const ok = (cap: string) => allowed && can(caps, cap);
  const rowDef = (action: string, label: string): ActionDef => ({ action, label, cap: "", group: "Account", help: "" });

  return (
    <Drawer open={!!userId} anchor="right" onClose={onClose} size="lg" slotProps={{ content: { sx: { width: { xs: "100%", md: 760 }, p: 0 } } }}>
      <Box sx={{ p: 2, overflow: "auto", height: "100%" }} data-testid="user-security">
        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          <Box sx={{ flex: 1, minWidth: 0 }}>
            <Typography level="h4" noWrap>
              {v?.person.name || v?.person.email || "Security"}
            </Typography>
            <Typography level="body-sm" sx={{ opacity: 0.7 }}>
              {v?.person.email}
            </Typography>
          </Box>
          <IconButton onClick={onClose} aria-label="Close">
            <CloseIcon />
          </IconButton>
        </Box>
        {err && (
          <Alert color="danger" sx={{ mt: 1 }}>
            {err}
          </Alert>
        )}
        {!v && !err && <CircularProgress sx={{ mt: 4 }} />}
        {v && (
          <>
            <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", my: 1.5 }}>
              <Chip color={v.person.status === "active" ? "success" : "danger"} variant="soft">
                {v.person.status}
              </Chip>
              {v.person.is_admin && <Chip variant="soft" color="primary">Administrator</Chip>}
              {v.person.quarantined && <Chip color="warning" variant="soft">Read only</Chip>}
              {v.idp_state && <Chip variant="outlined">Identity provider: {v.idp_state.toLowerCase()}</Chip>}
              {v.expires_at && <Chip variant="outlined">Expires {when(v.expires_at)}</Chip>}
              {v.methods.length > 0 && <Chip variant="outlined">Methods: {v.methods.join(", ").toLowerCase()}</Chip>}
            </Box>
            {!allowed && (
              <Alert color="warning" variant="soft" sx={{ mb: 1.5 }}>
                {v.can.self ? "You cannot change your own security here." : "Only a super admin can act on another administrator. You can view this account."}
              </Alert>
            )}

            <Typography level="title-md" sx={{ mt: 1 }}>Actions</Typography>
            <Sheet variant="outlined" sx={{ borderRadius: "md", p: 1.5, mt: 0.5 }}>
              {can(caps, "contain") && allowed && (
                <Box sx={{ mb: 1.5, display: "flex", gap: 1, alignItems: "center" }}>
                  <Button color="danger" onClick={() => setDlg({ def: ACTIONS[0] })} data-testid="act-contain">
                    Contain account
                  </Button>
                  <Typography level="body-xs">Sign out everywhere, reset password, remove second steps, notify. Preview first.</Typography>
                </Box>
              )}
              {GROUPS.map((g) => {
                const list = ACTIONS.filter((a) => a.group === g && a.action !== "contain" && ok(a.cap));
                if (!list.length) return null;
                return (
                  <Box key={g} sx={{ mb: 1 }}>
                    <Typography level="body-xs" sx={{ textTransform: "uppercase", opacity: 0.6, mb: 0.5 }}>{g}</Typography>
                    <Box sx={{ display: "flex", gap: 0.75, flexWrap: "wrap" }}>
                      {list.map((a) => (
                        <Button key={a.action} size="sm" variant="outlined" color={a.danger ? "danger" : "neutral"} onClick={() => setDlg({ def: a })} data-testid={`act-${a.action}`}>
                          {a.label}
                        </Button>
                      ))}
                    </Box>
                  </Box>
                );
              })}
              {!allowed && <Typography level="body-sm">No actions available for this account with your role.</Typography>}
            </Sheet>

            <Typography level="title-md" sx={{ mt: 2 }}>Second steps</Typography>
            {v.factors_error && <Alert color="neutral" variant="soft" sx={{ my: 0.5 }}>{v.factors_error}</Alert>}
            {!v.factors_error && v.factors.length === 0 && (
              <Alert color={v.person.is_admin ? "warning" : "neutral"} variant="soft" sx={{ my: 0.5 }} data-testid="no-factors">
                No second step is set up{v.person.is_admin ? ". This is an administrator account." : "."}
              </Alert>
            )}
            {v.factors.length > 0 && (
              <Table size="sm">
                <tbody>
                  {v.factors.map((f) => (
                    <tr key={f.kind + f.id} data-testid="factor-row">
                      <td>{f.name}</td>
                      <td><Chip size="sm" variant="soft">{f.kind}</Chip></td>
                      <td>{f.state.toLowerCase()}</td>
                      <td style={{ textAlign: "right" }}>
                        {ok("reset_mfa") && (
                          <Button size="sm" variant="plain" color="danger" data-testid="revoke-factor" onClick={() => setDlg({ def: { ...rowDef("mfa.revoke", `Remove ${f.name}`), help: "Removes only this second step." }, extra: { factor_id: f.id, kind: f.kind } })}>
                            Revoke
                          </Button>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </Table>
            )}
            {v.idp_links.length > 0 && (
              <>
                <Typography level="title-sm" sx={{ mt: 1 }}>Linked identity providers</Typography>
                {v.idp_links.map((l) => (
                  <Box key={l.idp_id} sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                    <Typography level="body-sm" sx={{ flex: 1 }}>{l.idp_name || l.idp_id}: {l.user_name}</Typography>
                    {ok("account_edit") && (
                      <Button size="sm" variant="plain" color="danger" onClick={() => setDlg({ def: { ...rowDef("account.unlink_idp", `Unlink ${l.idp_name || "provider"}`), help: "The person can no longer sign in with this provider." }, extra: { idp_id: l.idp_id, linked_user_id: l.user_name } })}>
                        Unlink
                      </Button>
                    )}
                  </Box>
                ))}
              </>
            )}

            <Typography level="title-md" sx={{ mt: 2 }}>Active sessions ({v.sessions.length})</Typography>
            {v.sessions.length === 0 ? (
              <Typography level="body-sm">No active sessions.</Typography>
            ) : (
              <Table size="sm" data-testid="sessions-table">
                <thead><tr><th>Device</th><th>Address</th><th>Place</th><th>Last active</th><th /></tr></thead>
                <tbody>
                  {v.sessions.map((s) => (
                    <tr key={s.id}>
                      <td>{s.device}{s.current && <Chip size="sm" sx={{ ml: 0.5 }}>this session</Chip>}</td>
                      <td>{s.ip}</td>
                      <td>{[s.city, s.country].filter(Boolean).join(", ") || "-"}</td>
                      <td>{ago(s.last_seen_at ?? s.created_at)}</td>
                      <td style={{ textAlign: "right" }}>
                        {ok("sessions") && !s.current && (
                          <Button size="sm" variant="plain" color="danger" onClick={() => setDlg({ def: { ...rowDef("sessions.revoke", "Sign out this session"), help: "Ends only this session." }, extra: { session_id: s.id } })}>
                            Sign out
                          </Button>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </Table>
            )}
            {v.tokens.length > 0 && (
              <>
                <Typography level="title-sm" sx={{ mt: 1 }}>API tokens</Typography>
                <Table size="sm">
                  <tbody>
                    {v.tokens.map((t) => (
                      <tr key={t.id}>
                        <td>{t.name}</td><td>{t.prefix}…</td><td>used {ago(t.last_used_at)}</td>
                        <td style={{ textAlign: "right" }}>
                          {ok("tokens") && (
                            <Button size="sm" variant="plain" color="danger" onClick={() => setDlg({ def: { ...rowDef("tokens.revoke", `Revoke ${t.name}`), help: "The token stops working at once." }, extra: { token_id: t.id } })}>
                              Revoke
                            </Button>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </Table>
              </>
            )}

            <Typography level="title-md" sx={{ mt: 2 }}>Alerts</Typography>
            {v.alerts.length === 0 ? <Typography level="body-sm">No alerts for this person.</Typography> : v.alerts.map((a) => (
              <Box key={a.id} sx={{ display: "flex", gap: 1, py: 0.5, alignItems: "center" }}>
                <Chip size="sm" color={SEVERITY_COLOR[a.severity]} variant="soft">{a.severity}</Chip>
                <Typography level="body-sm" sx={{ flex: 1 }}>{a.title}</Typography>
                <Chip size="sm" variant="outlined">{a.status}</Chip>
              </Box>
            ))}

            <Typography level="title-md" sx={{ mt: 2 }}>Recent sign-ins</Typography>
            <Table size="sm" data-testid="user-events">
              <thead><tr><th>When</th><th>Result</th><th>Address</th><th>Place</th><th>Method</th></tr></thead>
              <tbody>
                {v.events.map((e) => (
                  <tr key={e.id}>
                    <td>{when(e.at)}</td>
                    <td><Chip size="sm" color={e.success ? "success" : "danger"} variant="soft">{e.success ? "ok" : "failed"}</Chip></td>
                    <td>{e.ip}</td><td>{[e.city, e.country].filter(Boolean).join(", ") || "-"}</td><td>{e.method}{e.mfa_result ? ` / ${e.mfa_result}` : ""}</td>
                  </tr>
                ))}
              </tbody>
            </Table>

            <Typography level="title-md" sx={{ mt: 2 }}>Recent actions on this account</Typography>
            <ActionTable rows={v.actions} />
          </>
        )}
        <Divider sx={{ my: 2 }} />
      </Box>
      {dlg && v && (
        <ActionDialog
          def={dlg.def}
          extra={dlg.extra}
          target={{ id: v.person.id, email: v.person.email }}
          onClose={() => setDlg(null)}
          onDone={() => {
            void load();
            onChanged();
          }}
        />
      )}
    </Drawer>
  );
}

export function ActionTable({ rows }: { rows: ActionRow[] }) {
  if (!rows.length) return <Typography level="body-sm">Nothing yet.</Typography>;
  return (
    <Table size="sm" data-testid="action-table">
      <thead><tr><th>When</th><th>By</th><th>Action</th><th>Reason</th><th>Ticket</th><th>Status</th></tr></thead>
      <tbody>
        {rows.map((r) => (
          <tr key={r.id}>
            <td>{when(r.created_at)}</td><td>{r.actor || "system"}</td><td>{r.action}{r.target_email ? ` on ${r.target_email}` : ""}</td><td>{r.reason}</td><td>{r.ticket}</td>
            <td><Chip size="sm" variant="soft" color={r.status === "done" ? "success" : r.status === "failed" ? "danger" : "warning"}>{r.status}</Chip></td>
          </tr>
        ))}
      </tbody>
    </Table>
  );
}
