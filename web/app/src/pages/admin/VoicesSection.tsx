/** Admin > Voices: text-to-speech providers for Stories, limits and recent audio jobs. */
import { useCallback, useEffect, useState } from "react";
import { Alert, Box, Button, Card, Checkbox, Chip, FormControl, FormHelperText, FormLabel, Input, Modal, ModalDialog, Option, Select, Stack, Table, Textarea, Typography } from "@mui/joy";
import {
  voicesAdmin, type JobRow, type Preset, type ProviderInput, type ProviderRow, type VoiceSettings,
} from "../podcasts/storyApi";

const empty: ProviderInput = { name: "", kind: "openai", base_url: "", model: "", format: "wav", default_voice: "", key_env: "", enabled: true };

export function VoicesSection() {
  const [rows, setRows] = useState<ProviderRow[] | null>(null);
  const [presets, setPresets] = useState<Preset[]>([]);
  const [settings, setSettings] = useState<VoiceSettings | null>(null);
  const [jobs, setJobs] = useState<JobRow[]>([]);
  const [error, setError] = useState("");
  const [edit, setEdit] = useState<{ id?: string; in: ProviderInput } | null>(null);
  const [note, setNote] = useState("");
  const [tests, setTests] = useState<Record<string, string>>({});

  const load = useCallback(() => {
    voicesAdmin.providers().then(setRows).catch((e) => setError(String(e.message ?? e)));
    voicesAdmin.detect().then(setPresets).catch(() => undefined);
    voicesAdmin.settings().then(setSettings).catch(() => undefined);
    voicesAdmin.jobs().then(setJobs).catch(() => undefined);
  }, []);
  useEffect(load, [load]);

  const test = async (p: ProviderRow) => {
    setTests((t) => ({ ...t, [p.id]: "Testing…" }));
    try {
      const r = await voicesAdmin.test(p.id, p.default_voice || undefined);
      setTests((t) => ({ ...t, [p.id]: r.ok ? `Works: spoke a test sentence (${(r.duration_ms / 1000).toFixed(1)} s) in ${r.latency_ms} ms, ${r.voices} voices listed.` : `Failed: ${r.error}` }));
    } catch (e) { setTests((t) => ({ ...t, [p.id]: `Failed: ${(e as Error).message}` })); }
  };

  return (
    <Stack spacing={2.5} data-testid="voices-section">
      <Box>
        <Typography level="h3">Voices</Typography>
        <Typography level="body-sm" sx={{ maxWidth: 760 }}>
          Stories are turned into audio on the server by the voices set up here. No voices are bundled with Adrient: connect a speech service or install a local engine.
          Until you do, listeners hear stories read aloud by their own browser, which needs no setup.
        </Typography>
      </Box>
      {error && <Alert color="danger">{error}</Alert>}
      {note && <Alert color="success" endDecorator={<Button size="sm" variant="plain" onClick={() => setNote("")}>OK</Button>}>{note}</Alert>}

      <Card variant="outlined">
        <Stack direction="row" justifyContent="space-between" alignItems="center">
          <Typography level="title-md">Voice providers</Typography>
          <Button size="sm" onClick={() => setEdit({ in: { ...empty } })} data-testid="add-provider">Add a provider</Button>
        </Stack>
        <Typography level="body-sm">Browser voice is always available as the fallback. It reads the story aloud on each listener's device; nothing is stored on the server.</Typography>
        {!rows && <Typography level="body-sm">Loading…</Typography>}
        {rows?.length === 0 && <Alert variant="soft" color="neutral">No server voice yet. Add an OpenAI-compatible speech endpoint, or a local command such as Piper or eSpeak NG.</Alert>}
        {rows?.map((p) => (
          <Card key={p.id} variant="soft" data-testid="provider-row">
            <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap">
              <Typography level="title-sm">{p.name}</Typography>
              <Chip size="sm">{p.kind === "openai" ? "OpenAI-compatible" : "Local command"}</Chip>
              {p.is_default && <Chip size="sm" color="success">Default</Chip>}
              {!p.enabled && <Chip size="sm" color="neutral">Off</Chip>}
              {p.kind === "command" && !p.command_found && <Chip size="sm" color="danger">Command not found on this server</Chip>}
              {p.kind === "openai" && <Chip size="sm" variant="outlined">Key: {p.key_source === "stored" ? "saved" : p.key_source === "env" ? `from ${p.key_env}` : "none"}</Chip>}
              <Box sx={{ flex: 1 }} />
              <Button size="sm" variant="outlined" onClick={() => void test(p)} data-testid="test-provider">Test</Button>
              <Button size="sm" variant="outlined" onClick={() => setEdit({ id: p.id, in: { ...p, api_key: "" } })}>Edit</Button>
              <Button size="sm" variant="outlined" color="danger" onClick={async () => { if (confirm(`Remove ${p.name}? Stories that used it fall back to the default voice.`)) { await voicesAdmin.remove(p.id); load(); } }}>Remove</Button>
            </Stack>
            <Typography level="body-xs">{p.kind === "openai" ? p.base_url : `${p.command} ${p.args.join(" ")}`}</Typography>
            {tests[p.id] && <Typography level="body-sm" color={tests[p.id].startsWith("Failed") ? "danger" : "success"}>{tests[p.id]}</Typography>}
          </Card>
        ))}
      </Card>

      {settings && (
        <Card variant="outlined">
          <Typography level="title-md">Limits</Typography>
          <Stack direction={{ xs: "column", md: "row" }} spacing={2}>
            <FormControl><FormLabel>Characters per person per day</FormLabel><Input type="number" value={settings.daily_chars} onChange={(e) => setSettings({ ...settings, daily_chars: Number(e.target.value) })} /><FormHelperText>0 means no daily limit.</FormHelperText></FormControl>
            <FormControl><FormLabel>Audio requests per person per minute</FormLabel><Input type="number" value={settings.per_minute_jobs} onChange={(e) => setSettings({ ...settings, per_minute_jobs: Number(e.target.value) })} /></FormControl>
            <FormControl><FormLabel>Longest episode (characters)</FormLabel><Input type="number" value={settings.max_chars} onChange={(e) => setSettings({ ...settings, max_chars: Number(e.target.value) })} /></FormControl>
          </Stack>
          <Checkbox label="Make audio automatically when an episode is published or its text changes" checked={settings.auto_generate} onChange={(e) => setSettings({ ...settings, auto_generate: e.target.checked })} />
          <Box><Button size="sm" onClick={async () => { try { setSettings(await voicesAdmin.saveSettings(settings)); setNote("Limits saved."); } catch (e) { setError((e as Error).message); } }}>Save limits</Button></Box>
          <Typography level="body-xs">Who may write stories, generate audio or download it is set under App controls (Stories).</Typography>
        </Card>
      )}

      <Card variant="outlined">
        <Typography level="title-md">Recent audio jobs</Typography>
        {jobs.length === 0 ? <Typography level="body-sm">Nothing has been generated yet.</Typography> : (
          <Table size="sm">
            <thead><tr><th>Episode</th><th>Author</th><th>Status</th><th>Characters</th><th>When</th><th>Error</th></tr></thead>
            <tbody>{jobs.map((j) => <tr key={j.id}><td>{j.episode}</td><td>{j.user}</td><td>{j.status}{j.status === "running" ? ` ${j.progress}%` : ""}</td><td>{j.chars}</td><td>{new Date(j.created_at).toLocaleString()}</td><td>{j.error}</td></tr>)}</tbody>
          </Table>
        )}
      </Card>

      {edit && <ProviderDialog init={edit} presets={presets} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); setNote("Provider saved. Use Test to check it."); load(); }} />}
    </Stack>
  );
}

function ProviderDialog({ init, presets, onClose, onSaved }: { init: { id?: string; in: ProviderInput }; presets: Preset[]; onClose: () => void; onSaved: () => void }) {
  const [f, setF] = useState<ProviderInput>(init.in);
  const [argsText, setArgsText] = useState((init.in.args ?? []).join("\n"));
  const [voicesText, setVoicesText] = useState((init.in.voices ?? []).map((v) => (v.language ? `${v.id}|${v.name}|${v.language}` : v.id === v.name ? v.id : `${v.id}|${v.name}`)).join("\n"));
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const set = (p: Partial<ProviderInput>) => setF((x) => ({ ...x, ...p }));
  const usePreset = (p: Preset) => { setArgsText(p.args.join("\n")); set({ kind: "command", command: p.command, name: f.name || p.name }); };
  const save = async () => {
    setBusy(true);
    setErr("");
    const voices = voicesText.split("\n").map((l) => l.trim()).filter(Boolean).map((l) => { const [id, name, language] = l.split("|"); return { id: id.trim(), name: (name ?? id).trim(), language: language?.trim() }; });
    const body: ProviderInput = { ...f, args: argsText.split("\n").map((l) => l.trim()).filter(Boolean), voices };
    try {
      if (init.id) await voicesAdmin.update(init.id, body); else await voicesAdmin.create(body);
      onSaved();
    } catch (e) { setErr((e as Error).message); setBusy(false); }
  };
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ maxWidth: 640, width: "100%", overflow: "auto" }} data-testid="provider-dialog">
        <Typography level="title-lg">{init.id ? "Edit provider" : "Add a voice provider"}</Typography>
        <Stack spacing={1.5}>
          <FormControl><FormLabel>Name</FormLabel><Input value={f.name} onChange={(e) => set({ name: e.target.value })} data-testid="prov-name" /></FormControl>
          <FormControl>
            <FormLabel>Type</FormLabel>
            <Select value={f.kind} onChange={(_, v) => v && set({ kind: v as ProviderInput["kind"] })} disabled={!!init.id}>
              <Option value="openai">OpenAI-compatible speech endpoint</Option>
              <Option value="command">Local command (reads text on stdin, writes WAV or MP3)</Option>
            </Select>
          </FormControl>
          {f.kind === "openai" ? (
            <>
              <FormControl><FormLabel>Base URL</FormLabel><Input value={f.base_url ?? ""} placeholder="http://127.0.0.1:8880/v1" onChange={(e) => set({ base_url: e.target.value })} data-testid="prov-url" /><FormHelperText>Works with OpenAI (https://api.openai.com/v1) and local servers such as Kokoro-FastAPI or openedai-speech. The server calls POST {"{base}"}/audio/speech.</FormHelperText></FormControl>
              <Stack direction="row" spacing={1.5}>
                <FormControl sx={{ flex: 1 }}><FormLabel>Model</FormLabel><Input value={f.model ?? ""} placeholder="tts-1" onChange={(e) => set({ model: e.target.value })} /></FormControl>
                <FormControl sx={{ flex: 1 }}><FormLabel>Audio format</FormLabel><Select value={f.format ?? "wav"} onChange={(_, v) => v && set({ format: v })}><Option value="wav">WAV (exact highlighting, larger files)</Option><Option value="mp3">MP3 (smaller files)</Option></Select></FormControl>
              </Stack>
              <FormControl><FormLabel>API key</FormLabel><Input type="password" value={f.api_key ?? ""} placeholder={init.id ? "Leave empty to keep the saved key" : "Optional for local servers"} onChange={(e) => set({ api_key: e.target.value })} autoComplete="off" /><FormHelperText>Stored encrypted and never shown again. Or name an environment variable below.</FormHelperText></FormControl>
              <FormControl><FormLabel>Key from environment variable</FormLabel><Input value={f.key_env ?? ""} placeholder="ADRIENT_TTS_KEY" onChange={(e) => set({ key_env: e.target.value.toUpperCase() })} /></FormControl>
              {init.id && <Checkbox label="Remove the saved key" checked={!!f.clear_key} onChange={(e) => set({ clear_key: e.target.checked })} />}
            </>
          ) : (
            <>
              <Box>
                <Typography level="body-sm" sx={{ mb: 0.5 }}>Engines found on this server</Typography>
                <Stack direction="row" spacing={1} flexWrap="wrap">
                  {presets.map((p) => (
                    <Button key={p.id} size="sm" variant="outlined" color={p.found ? "success" : "neutral"} onClick={() => usePreset(p)} disabled={!p.found} title={p.note}>
                      {p.name}: {p.found ? "installed, use it" : "not installed"}
                    </Button>
                  ))}
                </Stack>
                {presets.filter((p) => !p.found).map((p) => <Typography key={p.id} level="body-xs">{p.name}: {p.note}</Typography>)}
              </Box>
              <FormControl><FormLabel>Command</FormLabel><Input value={f.command ?? ""} placeholder="espeak-ng" onChange={(e) => set({ command: e.target.value })} data-testid="prov-cmd" /></FormControl>
              <FormControl><FormLabel>Arguments, one per line</FormLabel><Textarea minRows={4} value={argsText} onChange={(e) => setArgsText(e.target.value)} /><FormHelperText>Placeholders: {"{voice} {lang} {out} {voices_dir} {speed} {speed_wpm} {length_scale} {pitch} {pitch_99}"}. The text is sent on standard input. Use {"{out}"} when the program writes a file, otherwise it must print audio to standard output. No shell is used.</FormHelperText></FormControl>
              <FormControl><FormLabel>Voices folder (Piper models)</FormLabel><Input value={f.voices_dir ?? ""} placeholder="/opt/piper/voices" onChange={(e) => set({ voices_dir: e.target.value })} /></FormControl>
            </>
          )}
          <FormControl><FormLabel>Default voice</FormLabel><Input value={f.default_voice ?? ""} onChange={(e) => set({ default_voice: e.target.value })} /></FormControl>
          <FormControl><FormLabel>Voices to offer (optional, one per line: id|name|language)</FormLabel><Textarea minRows={3} value={voicesText} placeholder={"hi_priya|Priya|hi\nta_kavya|Kavya|ta"} onChange={(e) => setVoicesText(e.target.value)} /><FormHelperText>Leave empty to list the voices the engine reports. Add a line per voice for English, Hindi, Tamil and other languages the engine supports.</FormHelperText></FormControl>
          <Stack direction="row" spacing={3}>
            <Checkbox label="Enabled" checked={f.enabled !== false} onChange={(e) => set({ enabled: e.target.checked })} />
            <Checkbox label="Use as the company default" checked={!!f.is_default} onChange={(e) => set({ is_default: e.target.checked })} />
          </Stack>
          {err && <Alert color="danger">{err}</Alert>}
          <Stack direction="row" spacing={1} justifyContent="flex-end">
            <Button variant="plain" onClick={onClose}>Cancel</Button>
            <Button onClick={() => void save()} loading={busy} disabled={!f.name.trim()} data-testid="prov-save">Save</Button>
          </Stack>
        </Stack>
      </ModalDialog>
    </Modal>
  );
}
