import { useCallback, useEffect, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Checkbox,
  Chip,
  DialogContent,
  DialogTitle,
  FormControl,
  FormHelperText,
  FormLabel,
  Input,
  LinearProgress,
  Link,
  Modal,
  ModalClose,
  ModalDialog,
  Option,
  Radio,
  RadioGroup,
  Select,
  Sheet,
  Stack,
  Switch,
  Table,
  Textarea,
  Typography,
} from "@mui/joy";
import {
  wizardApi,
  kindLabels,
  type Connection,
  type Counter,
  type ImportError,
  type Kind,
  type Provider,
  type Run,
  type UserMapRow,
} from "./wizardApi";

const activeStatuses = ["queued", "running"];

function statusColor(
  s: string,
): "success" | "warning" | "danger" | "neutral" | "primary" {
  if (s === "done" || s === "ready") return "success";
  if (s === "failed" || s === "error") return "danger";
  if (s === "running" || s === "queued") return "primary";
  if (s === "needs_auth" || s === "cancelled" || s === "interrupted")
    return "warning";
  return "neutral";
}

function statusText(s: string): string {
  return (
    {
      needs_auth: "sign-in needed",
      ready: "connected",
      queued: "queued",
      running: "running",
      done: "finished",
      failed: "failed",
      cancelled: "cancelled",
      interrupted: "interrupted",
      error: "error",
    }[s] ?? s
  );
}

// ---- provider settings (admin) ---------------------------------------------

export function ProviderSettingsDialog({
  provider,
  onClose,
}: {
  provider: Provider;
  onClose: (saved: boolean) => void;
}) {
  const [clientId, setClientId] = useState("");
  const [secret, setSecret] = useState("");
  const [hasSecret, setHasSecret] = useState(false);
  const [tenant, setTenant] = useState("");
  const [sa, setSa] = useState("");
  const [hasSa, setHasSa] = useState(false);
  const [redirect, setRedirect] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    wizardApi
      .getSettings(provider.id)
      .then((s) => {
        setClientId(s.client_id);
        setHasSecret(s.has_secret);
        setHasSa(s.has_service_account);
        setTenant(s.extra?.tenant ?? "");
        setRedirect(s.redirect_url);
      })
      .catch((e) => setErr(String(e.message ?? e)));
  }, [provider.id]);

  const save = async () => {
    setBusy(true);
    setErr(null);
    try {
      await wizardApi.saveSettings(provider.id, {
        client_id: clientId,
        client_secret: secret || undefined,
        service_account_json: sa || undefined,
        extra: provider.id === "microsoft" ? { tenant } : {},
      });
      onClose(true);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open onClose={() => onClose(false)}>
      <ModalDialog sx={{ width: 560, maxWidth: "95vw" }}>
        <ModalClose />
        <DialogTitle>{provider.name}: OAuth app</DialogTitle>
        <DialogContent>
          <Stack spacing={1.5}>
            <Typography level="body-sm">
              Register an OAuth app with {provider.short} and paste its
              credentials here. They are stored encrypted and are never shown
              again. Step-by-step instructions:{" "}
              <code>{provider.setup_doc}</code>.
            </Typography>
            <FormControl>
              <FormLabel>Redirect URI to register</FormLabel>
              <Input
                readOnly
                value={redirect || "(server has no public URL configured)"}
                data-testid="redirect-uri"
              />
            </FormControl>
            <FormControl required>
              <FormLabel>Client ID</FormLabel>
              <Input
                value={clientId}
                onChange={(e) => setClientId(e.target.value)}
              />
            </FormControl>
            <FormControl required={!hasSecret}>
              <FormLabel>Client secret</FormLabel>
              <Input
                type="password"
                value={secret}
                placeholder={hasSecret ? "stored (leave empty to keep)" : ""}
                onChange={(e) => setSecret(e.target.value)}
              />
            </FormControl>
            {provider.id === "microsoft" && (
              <FormControl>
                <FormLabel>Tenant ID</FormLabel>
                <Input
                  value={tenant}
                  placeholder="contoso.onmicrosoft.com or GUID"
                  onChange={(e) => setTenant(e.target.value)}
                />
                <FormHelperText>
                  Required for organisation-wide migration (application
                  permissions).
                </FormHelperText>
              </FormControl>
            )}
            {provider.id === "google" && (
              <FormControl>
                <FormLabel>Service account key (JSON, optional)</FormLabel>
                <Textarea
                  minRows={3}
                  value={sa}
                  placeholder={
                    hasSa
                      ? "stored (leave empty to keep)"
                      : '{"type":"service_account", ...}'
                  }
                  onChange={(e) => setSa(e.target.value)}
                />
                <FormHelperText>
                  Needed only to migrate every user's data at once (domain-wide
                  delegation).
                </FormHelperText>
              </FormControl>
            )}
            {err && <Alert color="danger">{err}</Alert>}
            <Stack direction="row" spacing={1} justifyContent="flex-end">
              <Button variant="plain" onClick={() => onClose(false)}>
                Cancel
              </Button>
              <Button
                loading={busy}
                disabled={!clientId || (!hasSecret && !secret)}
                onClick={save}
              >
                Save
              </Button>
            </Stack>
          </Stack>
        </DialogContent>
      </ModalDialog>
    </Modal>
  );
}

// ---- connect dialog ----------------------------------------------------------

export function ConnectDialog({
  provider,
  isAdmin,
  onClose,
}: {
  provider: Provider;
  isAdmin: boolean;
  onClose: (c: Connection | null) => void;
}) {
  const [scope, setScope] = useState<"me" | "org">("me");
  const [values, setValues] = useState<Record<string, string>>({});
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const fields = provider.fields ?? [];
  const missing = fields.some((f) => f.required && !values[f.key]);

  const submit = async () => {
    setBusy(true);
    setErr(null);
    try {
      const c = await wizardApi.createConnection({
        provider: provider.id,
        scope,
        config: values,
      });
      if (c.status === "needs_auth") {
        const { url } = await wizardApi.oauthStart(c.id);
        window.location.assign(url);
        return;
      }
      onClose(c);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open onClose={() => onClose(null)}>
      <ModalDialog sx={{ width: 560, maxWidth: "95vw" }}>
        <ModalClose />
        <DialogTitle>Connect {provider.name}</DialogTitle>
        <DialogContent>
          <Stack spacing={1.5}>
            <Typography level="body-sm">{provider.notes}</Typography>
            {isAdmin && provider.scopes.includes("org") && (
              <FormControl>
                <FormLabel>What to migrate</FormLabel>
                <RadioGroup
                  value={scope}
                  onChange={(e) => setScope(e.target.value as "me" | "org")}
                >
                  <Radio value="me" label="My own account" />
                  <Radio
                    value="org"
                    label="The whole organisation (every user, mapped to Adrient accounts)"
                  />
                </RadioGroup>
              </FormControl>
            )}
            {fields.map((f) => (
              <FormControl key={f.key} required={f.required}>
                <FormLabel>{f.label}</FormLabel>
                {f.type === "select" ? (
                  <Select
                    value={values[f.key] ?? f.options?.[0] ?? ""}
                    onChange={(_, v) =>
                      setValues((s) => ({ ...s, [f.key]: String(v ?? "") }))
                    }
                  >
                    {(f.options ?? []).map((o) => (
                      <Option key={o} value={o}>
                        {o}
                      </Option>
                    ))}
                  </Select>
                ) : (
                  <Input
                    type={f.type === "password" ? "password" : "text"}
                    placeholder={f.placeholder}
                    value={values[f.key] ?? ""}
                    slotProps={{ input: { "aria-label": f.label } }}
                    onChange={(e) =>
                      setValues((s) => ({ ...s, [f.key]: e.target.value }))
                    }
                  />
                )}
              </FormControl>
            ))}
            {provider.auth === "oauth" && (
              <Typography level="body-xs" sx={{ color: "text.secondary" }}>
                You will be sent to {provider.short} to sign in and approve
                read-only access, then brought back here.
              </Typography>
            )}
            {err && (
              <Alert color="danger" data-testid="connect-error">
                {err}
              </Alert>
            )}
            <Stack direction="row" spacing={1} justifyContent="flex-end">
              <Button variant="plain" onClick={() => onClose(null)}>
                Cancel
              </Button>
              <Button loading={busy} disabled={missing} onClick={submit}>
                {provider.auth === "oauth"
                  ? `Sign in with ${provider.short}`
                  : "Test and connect"}
              </Button>
            </Stack>
          </Stack>
        </DialogContent>
      </ModalDialog>
    </Modal>
  );
}

// ---- user mapping -----------------------------------------------------------

function UserMapping({ conn }: { conn: Connection }) {
  const [rows, setRows] = useState<UserMapRow[] | null>(null);
  const [edits, setEdits] = useState<Record<string, string>>({});
  const [msg, setMsg] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const load = useCallback(() => {
    wizardApi
      .users(conn.id)
      .then(setRows)
      .catch((e) => setMsg(String(e.message ?? e)));
  }, [conn.id]);
  useEffect(load, [load]);

  const discover = async () => {
    setBusy(true);
    setMsg(null);
    try {
      const r = await wizardApi.discover(conn.id);
      setMsg(
        `Found ${r.found} users; ${r.matched} matched an Adrient account by e-mail.`,
      );
      load();
    } catch (e) {
      setMsg(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };
  const save = async (r: UserMapRow, skip: boolean) => {
    try {
      await wizardApi.setUser(conn.id, r.remote_id, {
        local_email: edits[r.remote_id] ?? r.local_email,
        skip,
      });
      load();
    } catch (e) {
      setMsg(e instanceof Error ? e.message : String(e));
    }
  };

  return (
    <Box mt={2}>
      <Stack direction="row" spacing={1} alignItems="center" mb={1}>
        <Typography level="title-sm">User mapping</Typography>
        <Button size="sm" variant="outlined" loading={busy} onClick={discover}>
          Find users
        </Button>
      </Stack>
      {msg && (
        <Alert size="sm" sx={{ mb: 1 }}>
          {msg}
        </Alert>
      )}
      {rows && rows.length > 0 ? (
        <Table size="sm" data-testid="user-map">
          <thead>
            <tr>
              <th>Source account</th>
              <th>Adrient account (e-mail)</th>
              <th style={{ width: 80 }}>Skip</th>
              <th style={{ width: 80 }} />
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <tr key={r.remote_id}>
                <td>
                  {r.remote_name}
                  <Typography level="body-xs">{r.remote_email}</Typography>
                </td>
                <td>
                  <Input
                    size="sm"
                    color={r.local_user_id || r.skip ? "neutral" : "warning"}
                    placeholder="not matched"
                    value={edits[r.remote_id] ?? r.local_email}
                    onChange={(e) =>
                      setEdits((s) => ({ ...s, [r.remote_id]: e.target.value }))
                    }
                  />
                </td>
                <td>
                  <Checkbox
                    checked={r.skip}
                    onChange={(e) => save(r, e.target.checked)}
                  />
                </td>
                <td>
                  <Button
                    size="sm"
                    variant="plain"
                    onClick={() => save(r, r.skip)}
                  >
                    Save
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </Table>
      ) : (
        <Typography level="body-xs" sx={{ color: "text.secondary" }}>
          No users listed yet. "Find users" reads the source directory and
          matches each account to an Adrient user with the same e-mail.
          Unmatched users are reported and skipped until you map them.
        </Typography>
      )}
    </Box>
  );
}

// ---- runs --------------------------------------------------------------------

function CounterTable({ counters }: { counters: Record<string, Counter> }) {
  const keys = Object.keys(counters);
  if (keys.length === 0) return null;
  return (
    <Table size="sm" sx={{ mt: 1 }}>
      <thead>
        <tr>
          <th>Data</th>
          <th>Found</th>
          <th>Imported</th>
          <th>Already there</th>
          <th>Failed</th>
        </tr>
      </thead>
      <tbody>
        {keys.map((k) => {
          const c = counters[k];
          return (
            <tr key={k} data-testid={`counter-${k}`}>
              <td>{kindLabels[k as Kind] ?? k}</td>
              {c.note ? (
                <td colSpan={4}>
                  <Typography level="body-xs">{c.note}</Typography>
                </td>
              ) : (
                <>
                  <td>{c.seen}</td>
                  <td>{c.imported}</td>
                  <td>{c.skipped}</td>
                  <td>{c.failed}</td>
                </>
              )}
            </tr>
          );
        })}
      </tbody>
    </Table>
  );
}

function RunCard({ run, onChange }: { run: Run; onChange: () => void }) {
  const [errors, setErrors] = useState<ImportError[] | null>(null);
  const active = activeStatuses.includes(run.status);
  return (
    <Sheet
      variant="soft"
      sx={{ p: 1.5, borderRadius: "sm", mt: 1 }}
      data-testid="run-card"
    >
      <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap">
        <Chip size="sm" color={statusColor(run.status)} variant="soft">
          {statusText(run.status)}
        </Chip>
        {run.dry_run && (
          <Chip size="sm" variant="outlined">
            dry run
          </Chip>
        )}
        <Typography level="body-xs">
          {new Date(run.created_at).toLocaleString()}
        </Typography>
        <Box flex={1} />
        {active && (
          <Button
            size="sm"
            variant="plain"
            color="danger"
            onClick={() => wizardApi.cancel(run.id).then(onChange)}
          >
            Cancel
          </Button>
        )}
        {run.error_count > 0 && (
          <>
            <Button
              size="sm"
              variant="plain"
              onClick={() => wizardApi.errors(run.id).then(setErrors)}
            >
              {run.error_count} problem{run.error_count === 1 ? "" : "s"}
            </Button>
            <Link href={wizardApi.errorsCsvUrl(run.id)} level="body-xs">
              Download CSV
            </Link>
          </>
        )}
      </Stack>
      {active && <LinearProgress sx={{ mt: 1 }} />}
      {run.message && (
        <Typography level="body-xs" sx={{ mt: 0.5 }}>
          {run.message}
        </Typography>
      )}
      {run.counters && <CounterTable counters={run.counters} />}
      {errors && (
        <Table size="sm" sx={{ mt: 1 }}>
          <tbody>
            {errors.slice(0, 50).map((e, i) => (
              <tr key={i}>
                <td style={{ width: 90 }}>
                  {kindLabels[e.kind as Kind] ?? e.kind}
                </td>
                <td>{e.label}</td>
                <td>{e.message}</td>
              </tr>
            ))}
          </tbody>
        </Table>
      )}
    </Sheet>
  );
}

function ConnectionCard({
  conn,
  provider,
  onDeleted,
}: {
  conn: Connection;
  provider?: Provider;
  onDeleted: () => void;
}) {
  const kinds = (provider?.kinds ?? []).filter(
    (k) => conn.scope === "org" || (k !== "users" && k !== "groups"),
  );
  const [sel, setSel] = useState<Kind[]>(kinds.filter((k) => k !== "chat"));
  const [dry, setDry] = useState(true);
  const [runs, setRuns] = useState<Run[]>([]);
  const [err, setErr] = useState<string | null>(null);

  const load = useCallback(() => {
    wizardApi
      .runs(conn.id)
      .then(setRuns)
      .catch(() => undefined);
  }, [conn.id]);
  useEffect(load, [load]);
  const anyActive = runs.some((r) => activeStatuses.includes(r.status));
  useEffect(() => {
    if (!anyActive) return;
    const t = setInterval(load, 1500);
    return () => clearInterval(t);
  }, [anyActive, load]);

  const start = async () => {
    setErr(null);
    try {
      await wizardApi.startRun(conn.id, sel, dry);
      load();
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    }
  };
  const reconnect = async () => {
    try {
      const { url } = await wizardApi.oauthStart(conn.id);
      window.location.assign(url);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    }
  };
  const remove = async () => {
    if (
      !window.confirm(
        "Remove this connection and its stored credentials? Data already imported stays in Adrient.",
      )
    )
      return;
    await wizardApi.deleteConnection(conn.id);
    onDeleted();
  };

  return (
    <Sheet
      variant="outlined"
      sx={{ p: 2, borderRadius: "md", mb: 2 }}
      data-testid="connection-card"
    >
      <Stack direction="row" spacing={1} alignItems="center">
        <Typography level="title-sm">{conn.label}</Typography>
        <Chip size="sm" variant="soft" color={statusColor(conn.status)}>
          {statusText(conn.status)}
        </Chip>
        {conn.scope === "org" && (
          <Chip size="sm" variant="outlined">
            whole organisation
          </Chip>
        )}
        <Box flex={1} />
        <Button size="sm" variant="plain" color="danger" onClick={remove}>
          Remove
        </Button>
      </Stack>
      {conn.config.account_email && (
        <Typography level="body-xs">
          Signed in as {conn.config.account_email}
        </Typography>
      )}
      {conn.status === "needs_auth" ? (
        <Button size="sm" sx={{ mt: 1 }} onClick={reconnect}>
          Sign in with {provider?.short ?? conn.provider}
        </Button>
      ) : (
        <>
          {conn.scope === "org" && <UserMapping conn={conn} />}
          <Stack
            direction="row"
            spacing={2}
            flexWrap="wrap"
            mt={1.5}
            useFlexGap
          >
            {kinds.map((k) => (
              <Checkbox
                key={k}
                size="sm"
                label={kindLabels[k]}
                checked={sel.includes(k)}
                onChange={(e) =>
                  setSel((s) =>
                    e.target.checked ? [...s, k] : s.filter((x) => x !== k),
                  )
                }
              />
            ))}
          </Stack>
          <Stack direction="row" spacing={2} alignItems="center" mt={1.5}>
            <Switch
              checked={dry}
              onChange={(e) => setDry(e.target.checked)}
              endDecorator="Dry run (count only, write nothing)"
              slotProps={{ input: { "aria-label": "Dry run" } }}
            />
            <Box flex={1} />
            <Button
              size="sm"
              disabled={sel.length === 0 || anyActive}
              onClick={start}
            >
              {dry
                ? "Start dry run"
                : runs.some((r) => !r.dry_run)
                  ? "Run again (skips what is already imported)"
                  : "Start migration"}
            </Button>
          </Stack>
        </>
      )}
      {err && (
        <Alert color="danger" size="sm" sx={{ mt: 1 }}>
          {err}
        </Alert>
      )}
      {runs.slice(0, 5).map((r) => (
        <RunCard key={r.id} run={r} onChange={load} />
      ))}
    </Sheet>
  );
}

// ---- main ---------------------------------------------------------------------

export default function MigrationWizard() {
  const [providers, setProviders] = useState<Provider[]>([]);
  const [isAdmin, setIsAdmin] = useState(false);
  const [conns, setConns] = useState<Connection[]>([]);
  const [connect, setConnect] = useState<Provider | null>(null);
  const [settings, setSettings] = useState<Provider | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const params = new URLSearchParams(window.location.search);
  const oauth = params.get("oauth");
  const oauthMsg = params.get("message");

  const load = useCallback(() => {
    wizardApi
      .providers()
      .then((d) => {
        setProviders(d.providers);
        setIsAdmin(d.is_admin);
      })
      .catch((e) => setErr(String(e.message ?? e)));
    wizardApi
      .connections()
      .then(setConns)
      .catch(() => undefined);
  }, []);
  useEffect(load, [load]);

  const byId = Object.fromEntries(providers.map((p) => [p.id, p]));

  return (
    <Box>
      {oauth === "ok" && (
        <Alert color="success" sx={{ mb: 2 }}>
          Signed in. Choose what to migrate and start with a dry run.
        </Alert>
      )}
      {oauth === "error" && (
        <Alert color="danger" sx={{ mb: 2 }}>
          Sign-in did not complete: {oauthMsg}
        </Alert>
      )}
      {err && (
        <Alert color="danger" sx={{ mb: 2 }}>
          {err}
        </Alert>
      )}
      <Typography level="title-md" mb={1}>
        Where are you moving from?
      </Typography>
      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" },
          gap: 1.5,
          mb: 3,
        }}
      >
        {providers.map((p) => {
          const blocked = p.needs_app && !p.configured;
          return (
            <Sheet
              key={p.id}
              variant="outlined"
              sx={{
                p: 1.5,
                borderRadius: "md",
                display: "flex",
                flexDirection: "column",
              }}
              data-testid={`provider-${p.id}`}
            >
              <Typography level="title-sm">{p.name}</Typography>
              <Stack
                direction="row"
                spacing={0.5}
                flexWrap="wrap"
                useFlexGap
                my={0.75}
              >
                {p.kinds.map((k) => (
                  <Chip key={k} size="sm" variant="soft">
                    {kindLabels[k]}
                  </Chip>
                ))}
              </Stack>
              {blocked && (
                <Typography level="body-xs" color="warning" mb={0.5}>
                  Not configured.{" "}
                  {isAdmin
                    ? "Enter the OAuth app credentials first."
                    : "Ask an administrator to set up this provider."}
                </Typography>
              )}
              <Box flex={1} />
              <Stack direction="row" spacing={1} mt={0.5}>
                <Button
                  size="sm"
                  disabled={blocked}
                  onClick={() => setConnect(p)}
                >
                  Connect
                </Button>
                {isAdmin && p.needs_app && (
                  <Button
                    size="sm"
                    variant="outlined"
                    onClick={() => setSettings(p)}
                  >
                    Provider settings
                  </Button>
                )}
              </Stack>
            </Sheet>
          );
        })}
      </Box>

      {conns.length > 0 && (
        <>
          <Typography level="title-md" mb={1}>
            Connections
          </Typography>
          {conns.map((c) => (
            <ConnectionCard
              key={c.id}
              conn={c}
              provider={byId[c.provider]}
              onDeleted={load}
            />
          ))}
        </>
      )}

      {connect && (
        <ConnectDialog
          provider={connect}
          isAdmin={isAdmin}
          onClose={(c) => {
            setConnect(null);
            if (c) load();
          }}
        />
      )}
      {settings && (
        <ProviderSettingsDialog
          provider={settings}
          onClose={(saved) => {
            setSettings(null);
            if (saved) load();
          }}
        />
      )}
    </Box>
  );
}
