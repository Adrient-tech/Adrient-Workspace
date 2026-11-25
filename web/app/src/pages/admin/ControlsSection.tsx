// App controls: typed per-app settings (Mail forwarding, Drive external sharing
// and guest download, Chat external chats, Meet recording and guests) with
// overrides by user type, role, department, group or user, optional IP-range
// conditions, and restrict-only rules. The server enforces every control.
import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Chip,
  ChipDelete,
  FormControl,
  FormLabel,
  Input,
  Option,
  Select,
  Sheet,
  Typography,
} from "@mui/joy";
import { apps as catalog } from "../../catalog/apps";
import type { Subject } from "./accessApi";
import { SubjectPicker } from "./SubjectPicker";
import {
  controlsApi,
  showValue,
  type ControlDef,
  type ControlRule,
  type ControlValue,
  type CtlSubject,
} from "./controlsApi";

const appName = (id: string) => (id === "*" ? "All apps" : (catalog.find((a) => a.id === id)?.name ?? id));

const SUBJECT_LABEL: Record<CtlSubject, string> = {
  everyone: "Everyone",
  user_type: "User type",
  role: "Role",
  department: "Department",
  group: "Group",
  user: "User",
};

export function ruleLabel(r: ControlRule): string {
  const who = r.subject_type === "everyone" ? "Everyone" : `${SUBJECT_LABEL[r.subject_type]}: ${r.subject_name || r.subject_id}`;
  const cond = r.condition?.ip_cidrs?.length ? ` from ${r.condition.ip_cidrs.join(", ")}` : "";
  return `${who}${cond}`;
}

export function ControlsSection() {
  const [catalogDefs, setCatalogDefs] = useState<ControlDef[]>([]);
  const [rules, setRules] = useState<ControlRule[]>([]);
  const [userTypes, setUserTypes] = useState<string[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [app, setApp] = useState<string>("drive");

  const load = useCallback(async () => {
    try {
      const r = await controlsApi.list();
      setCatalogDefs(r.catalog);
      setRules(r.rules);
      setUserTypes(r.user_types);
    } catch (e) {
      setError((e as Error).message);
    }
  }, []);
  useEffect(() => {
    void load();
  }, [load]);

  const appIds = useMemo(() => [...new Set(catalogDefs.map((d) => d.app))], [catalogDefs]);
  const defs = catalogDefs.filter((d) => d.app === app);

  async function remove(id: string) {
    setError(null);
    try {
      await controlsApi.remove(id);
      setNotice("Rule removed.");
      await load();
    } catch (e) {
      setError((e as Error).message);
    }
  }

  return (
    <Box data-testid="app-controls">
      <Typography level="h4">App controls</Typography>
      <Typography level="body-sm" sx={{ opacity: 0.7, mb: 2 }}>
        Fine-grained settings per app. The most specific rule wins (user, group, department, user type,
        role, everyone). A restrict-only rule can only tighten a setting and always wins.
      </Typography>
      {notice && (
        <Alert color="success" variant="soft" sx={{ mb: 1.5 }}>
          {notice}
        </Alert>
      )}
      {error && (
        <Alert color="danger" variant="soft" sx={{ mb: 1.5 }}>
          {error}
        </Alert>
      )}
      <Box sx={{ display: "flex", gap: 2, alignItems: "flex-start" }}>
        <Sheet variant="outlined" sx={{ borderRadius: "md", width: 170, flexShrink: 0 }}>
          {appIds.map((id) => (
            <Box
              key={id}
              role="button"
              tabIndex={0}
              data-testid={`controls-app-${id}`}
              onClick={() => setApp(id)}
              onKeyDown={(e) => e.key === "Enter" && setApp(id)}
              sx={{
                px: 1.5,
                py: 1,
                cursor: "pointer",
                borderBottom: "1px solid",
                borderColor: "divider",
                bgcolor: id === app ? "background.level1" : "transparent",
                fontWeight: id === app ? 600 : 400,
              }}
            >
              {appName(id)}
            </Box>
          ))}
        </Sheet>
        <Box sx={{ flex: 1, minWidth: 0, display: "grid", gap: 1.5 }}>
          {defs.map((d) => (
            <ControlCard
              key={d.key}
              def={d}
              rules={rules.filter((r) => r.control === d.key)}
              userTypes={userTypes}
              onRemove={remove}
              onSaved={async (msg) => {
                setNotice(msg);
                setError(null);
                await load();
              }}
              onError={setError}
            />
          ))}
        </Box>
      </Box>
    </Box>
  );
}

function ControlCard({
  def,
  rules,
  userTypes,
  onRemove,
  onSaved,
  onError,
}: {
  def: ControlDef;
  rules: ControlRule[];
  userTypes: string[];
  onRemove: (id: string) => void;
  onSaved: (msg: string) => Promise<void>;
  onError: (m: string) => void;
}) {
  const [adding, setAdding] = useState(false);
  const [value, setValue] = useState<ControlValue>(def.type === "bool" ? false : def.type === "enum" ? (def.enum?.[0] ?? "") : (def.default as number));
  const [type, setType] = useState<CtlSubject>("user_type");
  const [typeId, setTypeId] = useState<string>("guest");
  const [subject, setSubject] = useState<Subject | null>(null);
  const [ips, setIps] = useState("");
  const [restrict, setRestrict] = useState(false);

  const options: ControlValue[] = def.type === "bool" ? [true, false] : def.type === "enum" ? (def.enum ?? []) : [];
  const needsPicker = type !== "everyone" && type !== "user_type";

  async function save() {
    const subjectId = type === "everyone" ? "" : type === "user_type" ? typeId : subject?.id;
    if (subjectId === undefined || (type !== "everyone" && !subjectId)) return;
    const cidrs = ips
      .split(/[\s,]+/)
      .map((x) => x.trim())
      .filter(Boolean);
    try {
      await controlsApi.create({
        control: def.key,
        value: def.type === "number" ? Number(value) : value,
        subject_type: type,
        subject_id: subjectId,
        condition: cidrs.length ? { ip_cidrs: cidrs } : null,
        priority: 100,
        effect_mode: restrict ? "restrict_only" : "set",
      });
      setAdding(false);
      setSubject(null);
      setIps("");
      await onSaved(`Rule added for ${def.label}.`);
    } catch (e) {
      onError((e as Error).message);
    }
  }

  const guestDefault = def.type_defaults?.guest;
  return (
    <Sheet variant="outlined" sx={{ p: 1.5, borderRadius: "md" }} data-testid={`control-${def.key}`}>
      <Box sx={{ display: "flex", alignItems: "baseline", gap: 1, flexWrap: "wrap" }}>
        <Typography level="title-sm">{def.label}</Typography>
        <Chip size="sm" variant="soft">
          Default: {showValue(def, def.default)}
          {guestDefault !== undefined ? `, guests ${showValue(def, guestDefault)}` : ""}
        </Chip>
        <Box sx={{ flex: 1 }} />
        <Button size="sm" variant="plain" onClick={() => setAdding((a) => !a)} data-testid={`control-add-${def.key}`}>
          {adding ? "Cancel" : "Add rule"}
        </Button>
      </Box>
      <Typography level="body-xs" sx={{ opacity: 0.7, mb: 1 }}>
        {def.help}
      </Typography>
      <Box sx={{ display: "flex", gap: 0.75, flexWrap: "wrap" }}>
        {rules.length === 0 && (
          <Typography level="body-sm" sx={{ opacity: 0.6 }}>
            No rules. The default applies.
          </Typography>
        )}
        {rules.map((r) => (
          <Chip
            key={r.id}
            variant="soft"
            color={r.effect_mode === "restrict_only" ? "danger" : "primary"}
            data-testid={`rule-${def.key}-${r.id}`}
            endDecorator={<ChipDelete onDelete={() => r.id && onRemove(r.id)} />}
          >
            {showValue(def, r.value)} · {ruleLabel(r)}
            {r.effect_mode === "restrict_only" ? " · restrict only" : ""}
          </Chip>
        ))}
      </Box>
      {adding && (
        <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", alignItems: "flex-end", mt: 1.5 }}>
          <FormControl size="sm">
            <FormLabel>Value</FormLabel>
            {def.type === "number" ? (
              <Input size="sm" type="number" value={String(value)} onChange={(e) => setValue(Number(e.target.value))} sx={{ width: 110 }} />
            ) : (
              <Select size="sm" value={String(value)} onChange={(_, v) => v !== null && setValue(def.type === "bool" ? v === "true" : v)} sx={{ width: 130 }} data-testid="rule-value">
                {options.map((o) => (
                  <Option key={String(o)} value={String(o)}>
                    {showValue(def, o)}
                  </Option>
                ))}
              </Select>
            )}
          </FormControl>
          <FormControl size="sm">
            <FormLabel>Applies to</FormLabel>
            <Select
              size="sm"
              value={type}
              onChange={(_, v) => {
                if (v) {
                  setType(v);
                  setSubject(null);
                }
              }}
              sx={{ width: 140 }}
              data-testid="rule-subject-type"
            >
              {(Object.keys(SUBJECT_LABEL) as CtlSubject[]).map((t) => (
                <Option key={t} value={t}>
                  {SUBJECT_LABEL[t]}
                </Option>
              ))}
            </Select>
          </FormControl>
          {type === "user_type" && (
            <FormControl size="sm">
              <FormLabel>User type</FormLabel>
              <Select size="sm" value={typeId} onChange={(_, v) => v && setTypeId(v)} sx={{ width: 130 }} data-testid="rule-user-type">
                {userTypes.map((t) => (
                  <Option key={t} value={t}>
                    {t.charAt(0).toUpperCase() + t.slice(1)}
                  </Option>
                ))}
              </Select>
            </FormControl>
          )}
          {needsPicker && (
            <SubjectPicker key={type} type={type as "user" | "group" | "department" | "role"} value={subject} onChange={setSubject} testId="rule-subject" />
          )}
          <FormControl size="sm">
            <FormLabel>Only from IP ranges (optional)</FormLabel>
            <Input size="sm" placeholder="203.0.113.0/24, 10.0.0.0/8" value={ips} onChange={(e) => setIps(e.target.value)} sx={{ width: 230 }} slotProps={{ input: { "data-testid": "rule-ips" } as never }} />
          </FormControl>
          <FormControl size="sm">
            <FormLabel>Mode</FormLabel>
            <Select size="sm" value={restrict ? "restrict" : "set"} onChange={(_, v) => setRestrict(v === "restrict")} sx={{ width: 150 }}>
              <Option value="set">Set value</Option>
              <Option value="restrict">Restrict only</Option>
            </Select>
          </FormControl>
          <Button size="sm" onClick={save} disabled={needsPicker && !subject} data-testid="rule-save">
            Save rule
          </Button>
        </Box>
      )}
    </Sheet>
  );
}
