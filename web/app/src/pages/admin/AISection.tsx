// AISection — Admin → AI assistant (internal/ai). Providers (keys are
// write-only), default and per-app models, which apps have the assistant,
// limits, the data-handling notice, local-only mode and the kill switch.
// Who may use the assistant per user type is the "ai.assistant" app control.
import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Alert, Box, Button, Checkbox, Chip, CircularProgress, FormControl, FormHelperText, FormLabel, IconButton, Input,
  Modal, ModalClose, ModalDialog, Option, Select, Sheet, Switch, Table, Textarea, Tooltip, Typography,
} from "@mui/joy";
import AutoAwesomeIcon from "@mui/icons-material/AutoAwesome";
import DeleteIcon from "@mui/icons-material/DeleteOutline";
import EditIcon from "@mui/icons-material/EditOutlined";
import { Link as RouterLink } from "react-router-dom";
import { aiApi, APP_LABELS, PRESETS, type AIProvider, type AISettings, type ProviderKind, type TestResult, type UsageRow } from "./aiApi";

const KIND_LABEL: Record<ProviderKind, string> = { openai: "OpenAI-compatible", azure: "Azure OpenAI", anthropic: "Anthropic", gemini: "Google Gemini" };

function modelOptions(ps: AIProvider[]) {
  return ps.filter((p) => p.enabled).flatMap((p) => p.models.map((m) => ({ value: `${p.id}/${m}`, label: `${p.name} · ${m}`, local: p.local })));
}

export function AISection() {
  const [settings, setSettings] = useState<AISettings | null>(null);
  const [apps, setApps] = useState<string[]>([]);
  const [serverKey, setServerKey] = useState(true);
  const [providers, setProviders] = useState<AIProvider[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [edit, setEdit] = useState<Partial<AIProvider> | null>(null);
  const [tests, setTests] = useState<Record<string, TestResult | "running">>({});
  const [usageBy, setUsageBy] = useState<"app" | "model" | "user">("app");
  const [usage, setUsage] = useState<UsageRow[]>([]);

  const load = useCallback(async () => {
    setError(null);
    try {
      const [s, p] = await Promise.all([aiApi.settings(), aiApi.providers()]);
      setSettings(s.settings);
      setApps(s.apps);
      setServerKey(s.server_key);
      setProviders(p);
    } catch (e) {
      setError((e as Error).message);
    }
  }, []);
  useEffect(() => void load(), [load]);
  useEffect(() => {
    aiApi.usage(usageBy).then((r) => setUsage(r.rows)).catch(() => setUsage([]));
  }, [usageBy, saved]);

  const opts = useMemo(() => modelOptions(providers), [providers]);
  if (error && !settings) return <Alert color="danger">Could not load AI settings: {error}</Alert>;
  if (!settings) return <CircularProgress />;
  const s = settings;
  const set = (patch: Partial<AISettings>) => setSettings({ ...s, ...patch });
  const disabled = new Set(s.disabled_apps ?? []);

  const save = async (next = s) => {
    setSaving(true);
    setError(null);
    try {
      setSettings(await aiApi.saveSettings(next));
      setSaved(new Date().toLocaleTimeString());
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setSaving(false);
    }
  };
  const test = async (p: AIProvider) => {
    setTests((t) => ({ ...t, [p.id]: "running" }));
    try {
      const r = await aiApi.test(p.id);
      setTests((t) => ({ ...t, [p.id]: r }));
    } catch (e) {
      setTests((t) => ({ ...t, [p.id]: { ok: false, model: "", latency_ms: 0, error: (e as Error).message } }));
    }
  };
  const remove = async (p: AIProvider) => {
    if (!window.confirm(`Remove the provider "${p.name}"? Its stored key is deleted.`)) return;
    try {
      await aiApi.deleteProvider(p.id);
      await load();
    } catch (e) {
      setError((e as Error).message);
    }
  };

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 2, maxWidth: 1100 }} data-testid="admin-ai">
      <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
        <AutoAwesomeIcon color="primary" />
        <Typography level="h3" sx={{ flex: 1 }}>AI assistant</Typography>
        <Chip color={s.kill_switch ? "danger" : providers.some((p) => p.enabled) ? "success" : "warning"} variant="soft">
          {s.kill_switch ? "Switched off" : providers.some((p) => p.enabled) ? "On" : "No provider"}
        </Chip>
      </Box>
      {error && <Alert color="danger">{error}</Alert>}

      <Sheet variant="outlined" sx={{ p: 2, borderRadius: "md", borderColor: s.kill_switch ? "danger.400" : undefined }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 2 }}>
          <Box sx={{ flex: 1 }}>
            <Typography level="title-md">Kill switch</Typography>
            <Typography level="body-sm">Turns the assistant off for everyone in the organisation at once. Running answers stop at the next request.</Typography>
          </Box>
          <Switch color={s.kill_switch ? "danger" : "neutral"} checked={s.kill_switch} onChange={(e) => void save({ ...s, kill_switch: e.target.checked })} slotProps={{ input: { "aria-label": "Kill switch" } }} />
        </Box>
      </Sheet>

      <Sheet variant="outlined" sx={{ p: 2, borderRadius: "md" }}>
        <Box sx={{ display: "flex", alignItems: "center", mb: 1 }}>
          <Typography level="title-md" sx={{ flex: 1 }}>Model providers</Typography>
          <Button size="sm" onClick={() => setEdit({ kind: "openai", enabled: true, local: false, models: [] })}>Add provider</Button>
        </Box>
        <Typography level="body-sm" sx={{ mb: 1 }}>
          Any OpenAI-compatible endpoint (Ollama, llama.cpp, vLLM, LM Studio, OpenRouter, OpenAI), Azure OpenAI, Anthropic or Google Gemini.
          API keys are encrypted on the server and never shown again.
        </Typography>
        {!serverKey && <Alert color="warning" sx={{ mb: 1 }}>The server has no ADRIENT_SECRETS_KEY, so keys cannot be stored. Use a key environment variable (ADRIENT_AI_…) or a local model without a key.</Alert>}
        {providers.length === 0 ? (
          <Typography level="body-sm" sx={{ opacity: 0.7 }}>No provider yet. The assistant shows "AI is not configured" until you add one.</Typography>
        ) : (
          <Table size="sm" sx={{ "& td": { verticalAlign: "top" } }}>
            <thead>
              <tr><th style={{ width: "18%" }}>Name</th><th>Endpoint</th><th>Models</th><th style={{ width: 110 }}>Key</th><th style={{ width: 170 }}>Connection</th><th style={{ width: 80 }} /></tr>
            </thead>
            <tbody>
              {providers.map((p) => {
                const t = tests[p.id];
                return (
                  <tr key={p.id}>
                    <td>
                      <Typography level="title-sm">{p.name}</Typography>
                      <Box sx={{ display: "flex", gap: 0.5, flexWrap: "wrap" }}>
                        <Chip size="sm" variant="soft">{KIND_LABEL[p.kind]}</Chip>
                        {p.local && <Chip size="sm" color="success" variant="soft">Local</Chip>}
                        {!p.enabled && <Chip size="sm" color="neutral">Off</Chip>}
                      </Box>
                    </td>
                    <td style={{ wordBreak: "break-all" }}>{p.base_url}</td>
                    <td>{p.models.join(", ")}</td>
                    <td>{p.key_source === "stored" ? "Stored (encrypted)" : p.key_source === "env" ? `From ${p.key_env}` : "None"}</td>
                    <td>
                      <Button size="sm" variant="outlined" loading={t === "running"} onClick={() => void test(p)}>Test connection</Button>
                      {t && t !== "running" && (
                        <Typography level="body-xs" color={t.ok ? "success" : "danger"} sx={{ mt: 0.5 }}>
                          {t.ok ? `OK · ${t.model} · ${t.latency_ms} ms` : t.error}
                        </Typography>
                      )}
                    </td>
                    <td>
                      <Tooltip title="Edit"><IconButton size="sm" onClick={() => setEdit(p)}><EditIcon /></IconButton></Tooltip>
                      <Tooltip title="Remove"><IconButton size="sm" color="danger" onClick={() => void remove(p)}><DeleteIcon /></IconButton></Tooltip>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </Table>
        )}
      </Sheet>

      <Sheet variant="outlined" sx={{ p: 2, borderRadius: "md", display: "grid", gap: 2, gridTemplateColumns: { xs: "1fr", md: "1fr 1fr" } }}>
        <Box>
          <Typography level="title-md" sx={{ mb: 1 }}>Models and apps</Typography>
          <FormControl sx={{ mb: 1.5 }}>
            <FormLabel>Default model</FormLabel>
            <Select size="sm" value={s.default_model || null} placeholder="First enabled provider" onChange={(_, v) => set({ default_model: v ?? "" })}>
              {opts.map((o) => <Option key={o.value} value={o.value}>{o.label}</Option>)}
            </Select>
          </FormControl>
          <Table size="sm" aria-label="Assistant per app">
            <thead><tr><th>App</th><th style={{ width: 70 }}>On</th><th>Model</th></tr></thead>
            <tbody>
              {apps.map((a) => (
                <tr key={a}>
                  <td>{APP_LABELS[a] ?? a}</td>
                  <td>
                    <Checkbox size="sm" checked={!disabled.has(a)} slotProps={{ input: { "aria-label": `Assistant in ${APP_LABELS[a] ?? a}` } }}
                      onChange={(e) => set({ disabled_apps: e.target.checked ? [...disabled].filter((x) => x !== a) : [...disabled, a] })} />
                  </td>
                  <td>
                    <Select size="sm" value={s.app_models?.[a] || null} placeholder="Default" onChange={(_, v) => {
                      const m = { ...(s.app_models ?? {}) };
                      if (v) m[a] = v;
                      else delete m[a];
                      set({ app_models: m });
                    }}>
                      <Option value={null}>Default</Option>
                      {opts.map((o) => <Option key={o.value} value={o.value}>{o.label}</Option>)}
                    </Select>
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Box>
        <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
          <Typography level="title-md">Privacy and limits</Typography>
          <FormControl orientation="horizontal" sx={{ justifyContent: "space-between" }}>
            <Box>
              <FormLabel>Local models only</FormLabel>
              <FormHelperText>Only providers marked Local (on your own servers) are used.</FormHelperText>
            </Box>
            <Switch checked={s.local_only} onChange={(e) => set({ local_only: e.target.checked })} slotProps={{ input: { "aria-label": "Local models only" } }} />
          </FormControl>
          <FormControl>
            <FormLabel>Data-handling notice (shown in every assistant panel)</FormLabel>
            <Textarea minRows={3} value={s.notice} onChange={(e) => set({ notice: e.target.value })} />
          </FormControl>
          <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1 }}>
            <NumberField label="Org tokens per day (0 = no limit)" value={s.org_daily_tokens} onChange={(v) => set({ org_daily_tokens: v })} />
            <NumberField label="Tokens per user per day" value={s.user_daily_tokens} onChange={(v) => set({ user_daily_tokens: v })} />
            <NumberField label="Requests per user per minute (0 = 20)" value={s.user_per_minute} onChange={(v) => set({ user_per_minute: v })} />
            <NumberField label="Max answer tokens (0 = 2048)" value={s.max_output_tokens} onChange={(v) => set({ max_output_tokens: v })} />
          </Box>
          <Alert variant="soft" color="neutral">
            <Box>
              <Typography level="title-sm">Who can use the assistant</Typography>
              <Typography level="body-sm">
                Members can, guests and external users cannot, by default. Change it per user type, group or network in{" "}
                <RouterLink to="/admin/controls">App controls → Use the AI assistant</RouterLink>. App access rules also apply.
              </Typography>
            </Box>
          </Alert>
        </Box>
        <Box sx={{ gridColumn: "1 / -1", display: "flex", gap: 1, alignItems: "center" }}>
          <Button loading={saving} onClick={() => void save()}>Save settings</Button>
          {saved && <Typography level="body-sm">Saved at {saved}</Typography>}
        </Box>
      </Sheet>

      <Sheet variant="outlined" sx={{ p: 2, borderRadius: "md" }}>
        <Box sx={{ display: "flex", alignItems: "center", mb: 1, gap: 1 }}>
          <Typography level="title-md" sx={{ flex: 1 }}>Usage, last 30 days</Typography>
          <Select size="sm" value={usageBy} onChange={(_, v) => v && setUsageBy(v)} aria-label="Group usage by">
            <Option value="app">By app</Option>
            <Option value="model">By model</Option>
            <Option value="user">By user</Option>
          </Select>
        </Box>
        {usage.length === 0 ? (
          <Typography level="body-sm" sx={{ opacity: 0.7 }}>No AI requests yet.</Typography>
        ) : (
          <Table size="sm">
            <thead><tr><th>{usageBy === "app" ? "App" : usageBy === "model" ? "Model" : "User"}</th><th>Requests</th><th>Input tokens</th><th>Output tokens</th><th>Est. cost (USD)</th></tr></thead>
            <tbody>
              {usage.map((u) => (
                <tr key={u.key}>
                  <td>{usageBy === "app" ? APP_LABELS[u.key] ?? u.key : u.label || u.key || "—"}</td>
                  <td>{u.requests}</td>
                  <td>{u.input_tokens.toLocaleString()}</td>
                  <td>{u.output_tokens.toLocaleString()}</td>
                  <td>{u.cost_usd.toFixed(4)}</td>
                </tr>
              ))}
            </tbody>
          </Table>
        )}
        <Typography level="body-xs" sx={{ mt: 1, opacity: 0.7 }}>Message text is never stored; only token counts per request. Every request and settings change is in the audit log.</Typography>
      </Sheet>

      {edit && <ProviderDialog initial={edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); void load(); }} />}
    </Box>
  );
}

function NumberField({ label, value, onChange }: { label: string; value: number; onChange: (v: number) => void }) {
  return (
    <FormControl size="sm">
      <FormLabel>{label}</FormLabel>
      <Input type="number" value={String(value ?? 0)} slotProps={{ input: { min: 0 } }} onChange={(e) => onChange(Math.max(0, Math.floor(Number(e.target.value) || 0)))} />
    </FormControl>
  );
}

function ProviderDialog({ initial, onClose, onSaved }: { initial: Partial<AIProvider>; onClose: () => void; onSaved: () => void }) {
  const [p, setP] = useState<Partial<AIProvider>>(initial);
  const [models, setModels] = useState((initial.models ?? []).join(", "));
  const [key, setKey] = useState("");
  const [clearKey, setClearKey] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const isNew = !initial.id;
  const save = async () => {
    setBusy(true);
    setErr(null);
    try {
      const body: Partial<AIProvider> & { api_key?: string } = {
        name: p.name, kind: p.kind, base_url: p.base_url, key_env: p.key_env, api_version: p.api_version,
        local: !!p.local, enabled: p.enabled !== false, price_in_per_m: Number(p.price_in_per_m) || 0, price_out_per_m: Number(p.price_out_per_m) || 0,
        models: models.split(",").map((m) => m.trim()).filter(Boolean),
      };
      if (key) body.api_key = key;
      else if (clearKey) body.api_key = "";
      await aiApi.saveProvider(body, initial.id);
      onSaved();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 560, maxWidth: "95vw", overflow: "auto" }} aria-label={isNew ? "Add provider" : "Edit provider"}>
        <ModalClose />
        <Typography level="title-lg">{isNew ? "Add provider" : `Edit ${initial.name}`}</Typography>
        {isNew && (
          <FormControl size="sm">
            <FormLabel>Start from</FormLabel>
            <Select placeholder="Choose a preset (optional)" onChange={(_, v) => {
              const pr = PRESETS.find((x) => x.label === v);
              if (!pr) return;
              setP((cur) => ({ ...cur, name: cur.name || pr.label, kind: pr.kind, base_url: pr.base_url, local: pr.local }));
              if (pr.models) setModels(pr.models);
            }}>
              {PRESETS.map((x) => <Option key={x.label} value={x.label}>{x.label}</Option>)}
            </Select>
          </FormControl>
        )}
        <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1 }}>
          <FormControl size="sm"><FormLabel>Name</FormLabel><Input value={p.name ?? ""} onChange={(e) => setP({ ...p, name: e.target.value })} /></FormControl>
          <FormControl size="sm">
            <FormLabel>Type</FormLabel>
            <Select value={p.kind ?? "openai"} onChange={(_, v) => v && setP({ ...p, kind: v })}>
              {(Object.keys(KIND_LABEL) as ProviderKind[]).map((k) => <Option key={k} value={k}>{KIND_LABEL[k]}</Option>)}
            </Select>
          </FormControl>
        </Box>
        <FormControl size="sm"><FormLabel>Base URL</FormLabel><Input value={p.base_url ?? ""} placeholder="https://api.openai.com/v1" onChange={(e) => setP({ ...p, base_url: e.target.value })} />
          <FormHelperText>Plain http is allowed only for local or private addresses.</FormHelperText></FormControl>
        <FormControl size="sm"><FormLabel>{p.kind === "azure" ? "Deployment names" : "Models"} (comma separated)</FormLabel><Input value={models} onChange={(e) => setModels(e.target.value)} /></FormControl>
        {p.kind === "azure" && <FormControl size="sm"><FormLabel>API version</FormLabel><Input value={p.api_version ?? ""} placeholder="2024-10-21" onChange={(e) => setP({ ...p, api_version: e.target.value })} /></FormControl>}
        <FormControl size="sm">
          <FormLabel>API key</FormLabel>
          <Input type="password" autoComplete="new-password" value={key} placeholder={initial.has_key ? "A key is stored. Type to replace it." : "Not needed for most local models"} onChange={(e) => setKey(e.target.value)} />
          {initial.has_key && initial.key_source === "stored" && <Checkbox size="sm" sx={{ mt: 0.5 }} label="Remove the stored key" checked={clearKey} onChange={(e) => setClearKey(e.target.checked)} />}
        </FormControl>
        <FormControl size="sm"><FormLabel>Or read the key from an environment variable</FormLabel><Input value={p.key_env ?? ""} placeholder="ADRIENT_AI_OPENAI_KEY" onChange={(e) => setP({ ...p, key_env: e.target.value })} /></FormControl>
        <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1 }}>
          <FormControl size="sm"><FormLabel>Input price, USD per 1M tokens</FormLabel><Input type="number" value={String(p.price_in_per_m ?? 0)} onChange={(e) => setP({ ...p, price_in_per_m: Number(e.target.value) })} /></FormControl>
          <FormControl size="sm"><FormLabel>Output price, USD per 1M tokens</FormLabel><Input type="number" value={String(p.price_out_per_m ?? 0)} onChange={(e) => setP({ ...p, price_out_per_m: Number(e.target.value) })} /></FormControl>
        </Box>
        <Box sx={{ display: "flex", gap: 2 }}>
          <Checkbox size="sm" label="Runs on our own servers (local)" checked={!!p.local} onChange={(e) => setP({ ...p, local: e.target.checked })} />
          <Checkbox size="sm" label="Enabled" checked={p.enabled !== false} onChange={(e) => setP({ ...p, enabled: e.target.checked })} />
        </Box>
        {err && <Alert color="danger">{err}</Alert>}
        <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1 }}>
          <Button variant="plain" onClick={onClose}>Cancel</Button>
          <Button loading={busy} onClick={() => void save()}>Save</Button>
        </Box>
      </ModalDialog>
    </Modal>
  );
}
