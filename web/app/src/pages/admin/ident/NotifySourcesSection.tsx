// Notification sources: tokens that let other apps and outside systems post
// into people's notification center.
import { useCallback, useEffect, useState } from "react";
import { Box, Button, Input, Sheet, Table, Typography } from "@mui/joy";
import { Notice, Page, api, when } from "./common";

interface Src {
  id: string;
  name: string;
  app: string;
  prefix: string;
  created_at: string;
  last_used_at: string | null;
  revoked_at: string | null;
}

export function NotifySourcesSection() {
  const [list, setList] = useState<Src[]>([]);
  const [name, setName] = useState("");
  const [app, setApp] = useState("");
  const [secret, setSecret] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const load = useCallback(async () => {
    try {
      setList((await api<{ sources: Src[] }>("/api/v1/admin/notify-sources")).sources);
    } catch (e) {
      setError((e as Error).message);
    }
  }, []);
  useEffect(() => {
    void load();
  }, [load]);
  return (
    <Page testId="notifysources" title="Notification sources" intro="Give HR, CRM, Accounting or any outside system a token to post into people's notification center. See docs/notifications-hook.md for the request format.">
      <Notice error={error} />
      <Box sx={{ display: "flex", gap: 1, mb: 1.5, flexWrap: "wrap" }}>
        <Input size="sm" placeholder="Name, for example Payroll" value={name} onChange={(e) => setName(e.target.value)} slotProps={{ input: { "data-testid": "src-name" } as never }} />
        <Input size="sm" placeholder="App label, for example hr" value={app} onChange={(e) => setApp(e.target.value)} slotProps={{ input: { "data-testid": "src-app" } as never }} />
        <Button
          size="sm"
          disabled={!name.trim() || !app.trim()}
          data-testid="src-create"
          onClick={() =>
            void api<{ token: string }>("/api/v1/admin/notify-sources", { method: "POST", json: { name, app } })
              .then((r) => {
                setSecret(r.token);
                setName("");
                setApp("");
                return load();
              })
              .catch((e) => setError((e as Error).message))
          }
        >
          Create source
        </Button>
      </Box>
      {secret && (
        <Sheet variant="soft" color="success" sx={{ p: 1.5, mb: 1.5, borderRadius: "md", maxWidth: 760 }} data-testid="src-once">
          Copy this token now, it is not shown again:
          <Box component="code" sx={{ display: "block", wordBreak: "break-all" }}>
            {secret}
          </Box>
          <Typography level="body-xs">POST /api/v1/notify/emit with Authorization: Bearer … and {"{ title, body, link, emails: [...] }"}</Typography>
        </Sheet>
      )}
      <Table size="sm" sx={{ maxWidth: 900 }}>
        <tbody>
          {list.map((s) => (
            <tr key={s.id}>
              <td>{s.name}</td>
              <td>{s.app}</td>
              <td>
                <code>{s.prefix}…</code>
              </td>
              <td>{s.last_used_at ? `used ${when(s.last_used_at)}` : "never used"}</td>
              <td>
                {!s.revoked_at && (
                  <Button size="sm" variant="plain" color="danger" onClick={() => void api(`/api/v1/admin/notify-sources/${s.id}`, { method: "DELETE" }).then(load)}>
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
