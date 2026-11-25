// App access: who may open each app. No rules = everyone. Allow rules limit the
// app to the listed departments (with sub-departments), groups, roles and
// users; a deny rule always wins. Admins and the app's App admin always get in.
// "Preview as user" shows the app list that user would see.
import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Chip,
  ChipDelete,
  Option,
  Select,
  Sheet,
  Typography,
} from "@mui/joy";
import { apps as catalog } from "../../catalog/apps";
import {
  accessApi,
  type AccessPolicy,
  type Effect,
  type Preview,
  type Subject,
  type SubjectType,
} from "./accessApi";
import { SubjectPicker } from "./SubjectPicker";
import { RolePresets } from "./RolePresets";

const appName = (id: string) => catalog.find((a) => a.id === id)?.name ?? id;

const TYPE_LABEL: Record<SubjectType, string> = {
  everyone: "Everyone",
  department: "Department",
  group: "Group",
  role: "Role",
  user_type: "User type",
  user: "User",
};

function chipLabel(p: AccessPolicy): string {
  if (p.subject_type === "everyone") return "Everyone";
  return `${TYPE_LABEL[p.subject_type]}: ${p.subject_name || p.subject_id}`;
}

export function AppAccessSection() {
  const [apps, setApps] = useState<string[]>([]);
  const [saved, setSaved] = useState<Record<string, AccessPolicy[]>>({});
  const [draft, setDraft] = useState<Record<string, AccessPolicy[]>>({});
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [open, setOpen] = useState<string | null>(null);
  const [savingApp, setSavingApp] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const r = await accessApi.policies();
      const by: Record<string, AccessPolicy[]> = {};
      for (const p of r.policies) (by[p.service_id] ??= []).push(p);
      setApps(r.apps);
      setSaved(by);
      setDraft(structuredClone(by));
    } catch (e) {
      setError((e as Error).message);
    }
  }, []);
  useEffect(() => {
    void load();
  }, [load]);

  const dirty = (id: string) =>
    JSON.stringify(strip(saved[id] ?? [])) !== JSON.stringify(strip(draft[id] ?? []));

  async function save(id: string) {
    setSavingApp(id);
    setError(null);
    try {
      await accessApi.savePolicies(id, draft[id] ?? []);
      setNotice(`Saved access for ${appName(id)}.`);
      await load();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setSavingApp(null);
    }
  }

  return (
    <Box data-testid="app-access">
      <Typography level="h4">App access</Typography>
      <Typography level="body-sm" sx={{ opacity: 0.7, mb: 2 }}>
        Choose who may open each app. With no rules everyone can. As soon as you
        add an Allow rule, only the listed people can. Deny always wins. Admins
        and each app&apos;s App admin are never locked out.
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
      <RolePresets onApplied={load} />
      <PreviewAsUser />
      <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "hidden" }}>
        {apps.map((id) => {
          const rules = draft[id] ?? [];
          const isOpen = open === id;
          return (
            <Box
              key={id}
              data-testid={`access-app-${id}`}
              sx={{ borderBottom: "1px solid", borderColor: "divider", p: 1.5 }}
            >
              <Box
                sx={{ display: "flex", alignItems: "center", gap: 1, cursor: "pointer" }}
                onClick={() => setOpen(isOpen ? null : id)}
              >
                <Typography level="title-sm" sx={{ width: 150 }}>
                  {appName(id)}
                </Typography>
                <Box sx={{ flex: 1, display: "flex", gap: 0.5, flexWrap: "wrap" }}>
                  {rules.length === 0 ? (
                    <Chip size="sm" variant="soft" color="neutral">
                      Everyone
                    </Chip>
                  ) : (
                    rules.map((p, i) => (
                      <Chip
                        key={i}
                        size="sm"
                        variant="soft"
                        color={p.effect === "deny" ? "danger" : "success"}
                      >
                        {p.effect === "deny" ? "Deny " : ""}
                        {chipLabel(p)}
                      </Chip>
                    ))
                  )}
                </Box>
                <Button size="sm" variant="plain" data-testid={`access-edit-${id}`}>
                  {isOpen ? "Close" : "Edit"}
                </Button>
              </Box>
              {isOpen && (
                <Editor
                  rules={rules}
                  appId={id}
                  dirty={dirty(id)}
                  saving={savingApp === id}
                  onChange={(next) => setDraft((d) => ({ ...d, [id]: next }))}
                  onSave={() => save(id)}
                  onReset={() => setDraft((d) => ({ ...d, [id]: structuredClone(saved[id] ?? []) }))}
                />
              )}
            </Box>
          );
        })}
      </Sheet>
    </Box>
  );
}

function strip(l: AccessPolicy[]) {
  return l.map((p) => [p.subject_type, p.subject_id, p.effect]);
}

function Editor({
  rules,
  appId,
  dirty,
  saving,
  onChange,
  onSave,
  onReset,
}: {
  rules: AccessPolicy[];
  appId: string;
  dirty: boolean;
  saving: boolean;
  onChange: (n: AccessPolicy[]) => void;
  onSave: () => void;
  onReset: () => void;
}) {
  const [effect, setEffect] = useState<Effect>("allow");
  const [type, setType] = useState<SubjectType>("department");
  const [subject, setSubject] = useState<Subject | null>(null);

  function add() {
    const p: AccessPolicy =
      type === "everyone"
        ? { service_id: appId, subject_type: "everyone", subject_id: "", effect, subject_name: "Everyone" }
        : {
            service_id: appId,
            subject_type: type,
            subject_id: subject!.id,
            effect,
            subject_name: subject!.name,
          };
    if (rules.some((r) => r.subject_type === p.subject_type && r.subject_id === p.subject_id && r.effect === p.effect)) return;
    onChange([...rules, p]);
    setSubject(null);
  }

  return (
    <Box sx={{ mt: 1.5 }}>
      <Box sx={{ display: "flex", gap: 0.75, flexWrap: "wrap", mb: 1.5 }}>
        {rules.length === 0 && (
          <Typography level="body-sm" sx={{ opacity: 0.6 }}>
            No rules: everyone can open this app.
          </Typography>
        )}
        {rules.map((p, i) => (
          <Chip
            key={i}
            variant="soft"
            color={p.effect === "deny" ? "danger" : "success"}
            endDecorator={
              <ChipDelete
                onDelete={() => onChange(rules.filter((_, j) => j !== i))}
                data-testid={`access-chip-delete-${appId}-${i}`}
              />
            }
          >
            {p.effect === "deny" ? "Deny " : "Allow "}
            {chipLabel(p)}
          </Chip>
        ))}
      </Box>
      <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", alignItems: "center" }}>
        <Select size="sm" value={effect} onChange={(_, v) => v && setEffect(v)} sx={{ width: 100 }} data-testid="access-effect">
          <Option value="allow">Allow</Option>
          <Option value="deny">Deny</Option>
        </Select>
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
          data-testid="access-type"
        >
          {(Object.keys(TYPE_LABEL) as SubjectType[]).map((t) => (
            <Option key={t} value={t}>
              {TYPE_LABEL[t]}
            </Option>
          ))}
        </Select>
        {type !== "everyone" && (
          <SubjectPicker key={type} type={type} value={subject} onChange={setSubject} testId="access-subject" />
        )}
        <Button size="sm" variant="soft" disabled={type !== "everyone" && !subject} onClick={add} data-testid="access-add">
          Add
        </Button>
        <Box sx={{ flex: 1 }} />
        <Button size="sm" variant="plain" color="neutral" disabled={!dirty} onClick={onReset}>
          Reset
        </Button>
        <Button size="sm" disabled={!dirty} loading={saving} onClick={onSave} data-testid={`access-save-${appId}`}>
          Save
        </Button>
      </Box>
    </Box>
  );
}

function PreviewAsUser() {
  const [user, setUser] = useState<Subject | null>(null);
  const [data, setData] = useState<Preview | null>(null);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    if (!user) {
      setData(null);
      return;
    }
    let alive = true;
    accessApi
      .preview(user.id)
      .then((d) => {
        if (alive) {
          setData(d);
          setErr(null);
        }
      })
      .catch((e: Error) => alive && setErr(e.message));
    return () => {
      alive = false;
    };
  }, [user]);

  const denied = useMemo(() => new Set(data?.denied ?? []), [data]);
  return (
    <Sheet variant="soft" sx={{ p: 1.5, borderRadius: "md", mb: 2 }} data-testid="access-preview">
      <Box sx={{ display: "flex", gap: 1, alignItems: "center", flexWrap: "wrap" }}>
        <Typography level="title-sm">Preview as user</Typography>
        <SubjectPicker type="user" value={user} onChange={setUser} placeholder="Pick a user" testId="access-preview-user" />
      </Box>
      {err && (
        <Typography level="body-sm" color="danger" sx={{ mt: 1 }}>
          {err}
        </Typography>
      )}
      {data && (
        <Box sx={{ mt: 1.25 }}>
          <Typography level="body-xs" sx={{ opacity: 0.7, mb: 0.75 }}>
            Roles: {data.roles.join(", ") || "member"}. Sees {data.allowed.length} of{" "}
            {data.apps.length} apps.
          </Typography>
          <Box sx={{ display: "flex", gap: 0.5, flexWrap: "wrap" }}>
            {data.apps.map((a) => (
              <Chip
                key={a.id}
                size="sm"
                variant={denied.has(a.id) ? "outlined" : "soft"}
                color={denied.has(a.id) ? "danger" : "success"}
                title={a.reason}
                data-testid={`preview-${a.id}-${a.allowed ? "yes" : "no"}`}
                sx={denied.has(a.id) ? { textDecoration: "line-through" } : undefined}
              >
                {appName(a.id)}
              </Chip>
            ))}
          </Box>
        </Box>
      )}
    </Sheet>
  );
}
