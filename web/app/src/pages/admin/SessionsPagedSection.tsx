import { useMemo, useState } from "react";
import { Box, Button, Chip, Typography } from "@mui/joy";
import { DataTable, type Column, type TablePage, type TableQuery } from "../../components/DataTable";
import { revokeOrgSession, type SessionRow } from "./orgApi";

function params(q: TableQuery): string {
  const p = new URLSearchParams({ limit: String(q.limit), page: String(q.page) });
  if (q.q) p.set("q", q.q);
  if (q.filters.state) p.set("state", q.filters.state);
  return p.toString();
}

async function load(q: TableQuery, signal: AbortSignal): Promise<TablePage<SessionRow>> {
  const r = await fetch(`/api/v1/admin/sessions?${params(q)}`, { credentials: "same-origin", signal });
  if (r.status === 403) throw new Error("You need admin privileges to view sessions.");
  if (!r.ok) throw new Error(`Could not load sessions (HTTP ${r.status})`);
  const d = (await r.json()) as { items?: SessionRow[]; sessions?: SessionRow[]; next_cursor?: string };
  return { items: d.items ?? d.sessions ?? [], next_cursor: d.next_cursor };
}

/** Sessions and logins, paged on the server (50 per page), searchable by email. */
export function SessionsPagedSection() {
  const [tick, setTick] = useState(0);
  const [error, setError] = useState("");
  const columns: Column<SessionRow>[] = useMemo(
    () => [
      { key: "who", header: "Person", render: (s) => s.display_name || s.email },
      { key: "email", header: "Email", render: (s) => s.email },
      { key: "created", header: "Signed in", width: 170, render: (s) => new Date(s.created_at).toLocaleString() },
      { key: "seen", header: "Last seen", width: 170, render: (s) => (s.last_seen_at ? new Date(s.last_seen_at).toLocaleString() : "-") },
      { key: "ip", header: "Address", width: 120, render: (s) => s.ip || "-" },
      {
        key: "state",
        header: "State",
        width: 120,
        render: (s) => (
          <Chip size="sm" color={s.active ? "success" : "neutral"}>
            {s.current ? "this session" : s.active ? "active" : s.revoked ? "revoked" : "expired"}
          </Chip>
        ),
      },
      {
        key: "act",
        header: "",
        width: 90,
        render: (s) =>
          s.active ? (
            <Button
              size="sm"
              variant="plain"
              color="danger"
              onClick={async () => {
                if (!window.confirm(`Revoke ${s.email}'s session${s.current ? " (this is your own session)" : ""}?`)) return;
                try {
                  await revokeOrgSession(s.id);
                  setTick((n) => n + 1);
                } catch (e) {
                  setError((e as Error).message);
                }
              }}
            >
              Revoke
            </Button>
          ) : null,
      },
    ],
    [],
  );
  return (
    <>
      <Typography level="h4">Sessions &amp; logins</Typography>
      <Typography level="body-sm" sx={{ opacity: 0.7, mb: 1.5 }}>
        Logins across your organisation, 25 per page by default (up to 100). Revoke to sign a device out.
      </Typography>
      {error && (
        <Box role="alert" sx={{ color: "danger.plainColor", mb: 1 }}>
          {error}
        </Box>
      )}
      <DataTable<SessionRow>
        testId="sessions-table"
        columns={columns}
        rowKey={(s) => s.id}
        load={load}
        mode="page"
        searchPlaceholder="Search by email"
        filters={[
          {
            key: "state",
            label: "State",
            type: "select",
            options: [
              { value: "active", label: "Active" },
              { value: "revoked", label: "Revoked or expired" },
            ],
          },
        ]}
        reloadKey={tick}
        emptyText="No sessions to show."
      />
    </>
  );
}
