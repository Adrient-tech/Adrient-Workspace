// Role assignments: give Owner / Admin / App admin (per app) / Manager / Guest
// to users and groups. Member is the implicit default. Every change is written
// to the audit log by the server.
import { useCallback, useEffect, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Chip,
  Option,
  Select,
  Sheet,
  Table,
  Typography,
} from "@mui/joy";
import { apps as catalog } from "../../catalog/apps";
import { accessApi, type RoleAssignment, type Subject } from "./accessApi";
import { SubjectPicker } from "./SubjectPicker";

const appName = (id: string) => catalog.find((a) => a.id === id)?.name ?? id;

export function RoleAssignments() {
  const [roles, setRoles] = useState<{ key: string; name: string }[]>([]);
  const [appIds, setAppIds] = useState<string[]>([]);
  const [rows, setRows] = useState<RoleAssignment[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [subjType, setSubjType] = useState<"user" | "group">("user");
  const [subject, setSubject] = useState<Subject | null>(null);
  const [roleKey, setRoleKey] = useState("app_admin");
  const [scopeApp, setScopeApp] = useState("accounting");
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      const r = await accessApi.roles();
      setRoles(r.roles);
      setAppIds(r.apps);
      setRows(r.assignments);
    } catch (e) {
      setRows([]);
      setError((e as Error).message);
    }
  }, []);
  useEffect(() => {
    void load();
  }, [load]);

  async function add() {
    if (!subject) return;
    setBusy(true);
    setError(null);
    try {
      await accessApi.assign({
        subject_type: subjType,
        subject_id: subject.id,
        role_key: roleKey,
        scope_app: roleKey === "app_admin" ? scopeApp : undefined,
      });
      setNotice(
        `${subject.name} is now ${roleLabel(roleKey, scopeApp, roles)}.`,
      );
      setSubject(null);
      await load();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function revoke(a: RoleAssignment) {
    if (
      !window.confirm(
        `Remove ${roleLabel(a.role_key, a.scope_app, roles)} from ${a.subject_name}?`,
      )
    )
      return;
    setError(null);
    try {
      await accessApi.revoke(a.id);
      setNotice(`Removed ${a.role_name} from ${a.subject_name}.`);
      await load();
    } catch (e) {
      setError((e as Error).message);
    }
  }

  return (
    <Box data-testid="role-assignments" sx={{ mb: 4 }}>
      <Typography level="h4">Roles</Typography>
      <Typography level="body-sm" sx={{ opacity: 0.7, mb: 2 }}>
        Assign roles to users or groups. Everyone is a Member by default. An App
        admin has full access to one app and can always open it.
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
      <Sheet
        variant="outlined"
        sx={{
          p: 1.5,
          borderRadius: "md",
          mb: 2,
          display: "flex",
          gap: 1,
          flexWrap: "wrap",
          alignItems: "center",
        }}
      >
        <Select
          size="sm"
          value={subjType}
          onChange={(_, v) => {
            if (v) {
              setSubjType(v);
              setSubject(null);
            }
          }}
          sx={{ width: 110 }}
          data-testid="role-subject-type"
        >
          <Option value="user">User</Option>
          <Option value="group">Group</Option>
        </Select>
        <SubjectPicker
          key={subjType}
          type={subjType}
          value={subject}
          onChange={setSubject}
          testId="role-subject"
        />
        <Select
          size="sm"
          value={roleKey}
          onChange={(_, v) => v && setRoleKey(v)}
          sx={{ width: 150 }}
          data-testid="role-key"
        >
          {roles.map((r) => (
            <Option key={r.key} value={r.key}>
              {r.name}
            </Option>
          ))}
        </Select>
        {roleKey === "app_admin" && (
          <Select
            size="sm"
            value={scopeApp}
            onChange={(_, v) => v && setScopeApp(v)}
            sx={{ width: 170 }}
            data-testid="role-scope-app"
          >
            {appIds.map((id) => (
              <Option key={id} value={id}>
                {appName(id)}
              </Option>
            ))}
          </Select>
        )}
        <Button
          size="sm"
          disabled={!subject}
          loading={busy}
          onClick={add}
          data-testid="role-assign"
        >
          Assign role
        </Button>
      </Sheet>
      <Sheet variant="outlined" sx={{ borderRadius: "md", overflowX: "auto" }}>
        <Table sx={{ minWidth: 520 }}>
          <thead>
            <tr>
              <th>Who</th>
              <th style={{ width: 120 }}>Type</th>
              <th style={{ width: 240 }}>Role</th>
              <th style={{ width: 90 }} aria-label="Actions" />
            </tr>
          </thead>
          <tbody>
            {(rows ?? []).map((a) => (
              <tr key={a.id} data-testid={`role-row-${a.id}`}>
                <td>
                  <Typography level="body-sm">{a.subject_name}</Typography>
                  {a.subject_detail && (
                    <Typography level="body-xs" sx={{ opacity: 0.6 }}>
                      {a.subject_detail}
                    </Typography>
                  )}
                </td>
                <td>{a.subject_type}</td>
                <td>
                  <Chip
                    size="sm"
                    variant="soft"
                    color={a.role_key === "app_admin" ? "warning" : "primary"}
                  >
                    {roleLabel(a.role_key, a.scope_app, roles)}
                  </Chip>
                </td>
                <td>
                  <Button
                    size="sm"
                    variant="plain"
                    color="danger"
                    onClick={() => revoke(a)}
                    data-testid={`role-revoke-${a.id}`}
                  >
                    Remove
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </Table>
        {rows !== null && rows.length === 0 && (
          <Typography level="body-sm" sx={{ p: 3, textAlign: "center", opacity: 0.6 }}>
            No role assignments yet.
          </Typography>
        )}
      </Sheet>
    </Box>
  );
}

function roleLabel(
  key: string,
  scope: string,
  roles: { key: string; name: string }[],
): string {
  const name = roles.find((r) => r.key === key)?.name ?? key;
  return key === "app_admin" && scope ? `${name} (${appName(scope)})` : name;
}
