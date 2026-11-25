// Delegated admin roles with an optional department scope, and admin API tokens.
import { useCallback, useEffect, useState } from "react";
import { Box, Button, FormControl, FormLabel, Input, Option, Select, Sheet, Table, Typography } from "@mui/joy";
import { SubjectPicker } from "../SubjectPicker";
import type { Subject } from "../accessApi";
import { Notice, Page, api, when } from "./common";

interface Role {
  key: string;
  name: string;
  description: string;
  permissions: string[];
}
interface Assignment {
  id: string;
  email: string;
  name: string;
  role_key: string;
  departments: string[];
}
interface Token {
  id: string;
  name: string;
  role_key: string;
  prefix: string;
  created_at: string;
  revoked_at: string | null;
}

export function AdminRolesSection() {
  const [roles, setRoles] = useState<Role[]>([]);
  const [as, setAs] = useState<Assignment[]>([]);
  const [tokens, setTokens] = useState<Token[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [user, setUser] = useState<Subject | null>(null);
  const [role, setRole] = useState("help_desk");
  const [dept, setDept] = useState<Subject | null>(null);
  const [tname, setTname] = useState("");
  const [trole, setTrole] = useState("readonly_admin");
  const [secret, setSecret] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const r = await api<{ roles: Role[]; assignments: Assignment[] }>("/api/v1/admin/admin-roles");
      setRoles(r.roles);
      setAs(r.assignments);
      setTokens((await api<{ tokens: Token[] }>("/api/v1/admin/admin-tokens")).tokens);
    } catch (e) {
      setError((e as Error).message);
    }
  }, []);
  useEffect(() => {
    void load();
  }, [load]);

  async function assign() {
    setError(null);
    setNotice(null);
    try {
      await api("/api/v1/admin/admin-roles", { method: "POST", json: { user_id: user?.id, role_key: role, department_ids: dept ? [dept.id] : [] } });
      setNotice(`${user?.name} is now ${roles.find((r) => r.key === role)?.name}${dept ? ` for ${dept.name}` : ""}.`);
      setUser(null);
      setDept(null);
      await load();
    } catch (e) {
      setError((e as Error).message);
    }
  }
  async function makeToken() {
    setError(null);
    try {
      const r = await api<{ token: string }>("/api/v1/admin/admin-tokens", { method: "POST", json: { name: tname, role_key: trole } });
      setSecret(r.token);
      setTname("");
      await load();
    } catch (e) {
      setError((e as Error).message);
    }
  }

  return (
    <Page testId="adminroles" title="Admin roles and API tokens" intro="Super admins can do everything. These roles give part of the admin console to other people. A scoped role only applies to people in the chosen department and its sub-departments.">
      <Notice error={error} notice={notice} />
      <Table size="sm" sx={{ maxWidth: 900, mb: 3 }} data-testid="role-defs">
        <tbody>
          {roles.map((r) => (
            <tr key={r.key}>
              <td>
                <b>{r.name}</b>
              </td>
              <td>{r.description}</td>
            </tr>
          ))}
        </tbody>
      </Table>

      <Sheet variant="outlined" sx={{ p: 2, borderRadius: "md", maxWidth: 760, mb: 2, display: "flex", gap: 1.5, flexWrap: "wrap", alignItems: "flex-end" }}>
        <FormControl size="sm">
          <FormLabel>Person</FormLabel>
          <SubjectPicker type="user" value={user} onChange={setUser} placeholder="Pick a person" testId="role-user" />
        </FormControl>
        <FormControl size="sm">
          <FormLabel>Role</FormLabel>
          <Select size="sm" value={role} onChange={(_, v) => v && setRole(v)} sx={{ width: 170 }} data-testid="role-key">
            {roles.map((r) => (
              <Option key={r.key} value={r.key}>
                {r.name}
              </Option>
            ))}
          </Select>
        </FormControl>
        <FormControl size="sm">
          <FormLabel>Only for department (optional)</FormLabel>
          <SubjectPicker type="department" value={dept} onChange={setDept} placeholder="Whole organization" testId="role-dept" />
        </FormControl>
        <Button size="sm" disabled={!user} onClick={assign} data-testid="role-assign">
          Assign
        </Button>
      </Sheet>
      <Table size="sm" sx={{ maxWidth: 900, mb: 4 }} data-testid="role-assignments">
        <tbody>
          {as.map((a) => (
            <tr key={a.id}>
              <td>
                {a.name} <Typography level="body-xs">{a.email}</Typography>
              </td>
              <td>{roles.find((r) => r.key === a.role_key)?.name}</td>
              <td>{a.departments.length ? a.departments.join(", ") : "whole organization"}</td>
              <td>
                <Button size="sm" variant="plain" color="danger" onClick={() => void api(`/api/v1/admin/admin-roles/${a.id}`, { method: "DELETE" }).then(load)}>
                  Remove
                </Button>
              </td>
            </tr>
          ))}
          {as.length === 0 && (
            <tr>
              <td>No delegated admins yet.</td>
            </tr>
          )}
        </tbody>
      </Table>

      <Typography level="title-md" sx={{ mb: 1 }}>
        Admin API tokens
      </Typography>
      <Box sx={{ display: "flex", gap: 1, mb: 1.5, flexWrap: "wrap", alignItems: "flex-end" }}>
        <Input size="sm" placeholder="Token name, for example Directory sync" value={tname} onChange={(e) => setTname(e.target.value)} sx={{ width: 280 }} slotProps={{ input: { "data-testid": "atoken-name" } as never }} />
        <Select size="sm" value={trole} onChange={(_, v) => v && setTrole(v)} sx={{ width: 170 }} data-testid="atoken-role">
          {roles.map((r) => (
            <Option key={r.key} value={r.key}>
              {r.name}
            </Option>
          ))}
        </Select>
        <Button size="sm" disabled={!tname.trim()} onClick={makeToken} data-testid="atoken-create">
          Create token
        </Button>
      </Box>
      {secret && (
        <Sheet variant="soft" color="success" sx={{ p: 1.5, mb: 1.5, borderRadius: "md", maxWidth: 760 }} data-testid="atoken-once">
          Copy this token now, it is not shown again. Use it as <code>Authorization: Bearer …</code> on /api/v1/admin/* requests.
          <Box component="code" sx={{ display: "block", wordBreak: "break-all", mt: 0.5 }}>
            {secret}
          </Box>
        </Sheet>
      )}
      <Table size="sm" sx={{ maxWidth: 900 }} data-testid="atokens">
        <tbody>
          {tokens.map((t) => (
            <tr key={t.id}>
              <td>{t.name}</td>
              <td>{roles.find((r) => r.key === t.role_key)?.name}</td>
              <td>
                <code>{t.prefix}…</code>
              </td>
              <td>{t.revoked_at ? "revoked" : when(t.created_at)}</td>
              <td>
                {!t.revoked_at && (
                  <Button size="sm" variant="plain" color="danger" onClick={() => void api(`/api/v1/admin/admin-tokens/${t.id}`, { method: "DELETE" }).then(load)}>
                    Revoke
                  </Button>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </Table>
    </Page>
  );
}
