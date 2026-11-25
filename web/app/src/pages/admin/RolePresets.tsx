// Role presets: give a group (or one person) the apps of a job role in one step.
import { useEffect, useState } from "react";
import { Alert, Box, Button, Chip, Option, Select, Sheet, Typography } from "@mui/joy";
import { apps as catalog } from "../../catalog/apps";
import { accessApi, type RolePreset, type Subject } from "./accessApi";
import { SubjectPicker } from "./SubjectPicker";

const appName = (id: string) => catalog.find((a) => a.id === id)?.name ?? id;

export function RolePresets({ onApplied }: { onApplied: () => void }) {
  const [presets, setPresets] = useState<RolePreset[]>([]);
  const [key, setKey] = useState<string>("");
  const [kind, setKind] = useState<"group" | "user">("group");
  const [subject, setSubject] = useState<Subject | null>(null);
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    accessApi.presets().then((r) => setPresets(r.presets)).catch(() => setPresets([]));
  }, []);
  const preset = presets.find((p) => p.key === key);
  const apply = async () => {
    if (!preset || !subject) return;
    setBusy(true);
    setMsg(null);
    try {
      const r = await accessApi.applyPreset(preset.key, { subject_type: kind, subject_id: subject.id });
      const rs = r.newly_restricted.length ? ` ${r.newly_restricted.map(appName).join(", ")} now open only to people named in allow rules.` : "";
      setMsg({ ok: true, text: `${preset.name} applied to ${subject.name}.${rs}` });
      onApplied();
    } catch (e) {
      setMsg({ ok: false, text: e instanceof Error ? e.message : "Could not apply the preset" });
    } finally {
      setBusy(false);
    }
  };
  if (presets.length === 0) return null;
  return (
    <Sheet variant="outlined" sx={{ borderRadius: "md", p: 1.5, mb: 2 }} data-testid="role-presets">
      <Typography level="title-sm">Role presets</Typography>
      <Typography level="body-xs" sx={{ mb: 1 }}>
        Give a group the business apps of a job role. Applying adds an Allow rule for that group on HR, Talent, Accounting,
        Inventory, POS, Invoicing and CRM as listed. Once one of those apps has an Allow rule, only the people named in its
        rules can open it, so apply the matching preset to every group that needs the app. Contractors (external users) never
        get these apps unless named.
      </Typography>
      <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", alignItems: "center" }}>
        <Select size="sm" placeholder="Choose a role" value={key || null} onChange={(_, v) => setKey(v ?? "")} sx={{ minWidth: 200 }} data-testid="preset-select">
          {presets.map((p) => (
            <Option key={p.key} value={p.key}>{p.name}</Option>
          ))}
        </Select>
        <Select size="sm" value={kind} onChange={(_, v) => { setKind((v as "group" | "user") ?? "group"); setSubject(null); }} sx={{ width: 110 }}>
          <Option value="group">Group</Option>
          <Option value="user">Person</Option>
        </Select>
        <SubjectPicker type={kind} value={subject} onChange={setSubject} placeholder={kind === "group" ? "Choose a group" : "Choose a person"} testId="preset-subject" />
        <Button size="sm" disabled={!preset || !subject || busy} loading={busy} onClick={apply} data-testid="preset-apply">Apply</Button>
      </Box>
      {preset && (
        <Box sx={{ mt: 1 }}>
          <Typography level="body-sm">{preset.description}</Typography>
          <Box sx={{ display: "flex", gap: 0.5, flexWrap: "wrap", mt: 0.5 }}>
            {preset.apps.map((a) => <Chip key={a} size="sm" variant="soft">{appName(a)}</Chip>)}
            {(preset.app_admin ?? []).map((a) => <Chip key={"a" + a} size="sm" variant="outlined" color="primary">App admin: {appName(a)}</Chip>)}
          </Box>
        </Box>
      )}
      {msg && <Alert sx={{ mt: 1 }} color={msg.ok ? "success" : "danger"} variant="soft">{msg.text}</Alert>}
    </Sheet>
  );
}
