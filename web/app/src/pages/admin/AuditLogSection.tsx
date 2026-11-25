import { useCallback, useEffect, useMemo, useState } from "react";
import { Alert, Box, Chip, Switch, Typography } from "@mui/joy";
import { DataTable, type Column, type FilterDef, type TableQuery, type TablePage } from "../../components/DataTable";
import { PeoplePicker, usePeopleByIds, type DirectoryItem } from "../../components/PeoplePicker";
import type { AuditEvent } from "./auditApi";

const API = "/api/v1/admin/audit";

const SERVICES = ["drive", "docs", "sheets", "slides", "mail", "calendar", "chat", "meet", "contacts", "crm", "hr", "accounting", "inventory", "pos", "tickets", "admin", "security"];

/** Builds the audit query string shared by the table and the CSV export. */
export function auditParams(q: TableQuery, forExport = false): URLSearchParams {
  const p = new URLSearchParams();
  const f = q.filters;
  if (f.actor) p.set("actor_id", f.actor);
  if (f.service) p.set("service", f.service);
  if (f.action) p.set("action", f.action);
  if (f.status) p.set("status", f.status);
  if (f.resource) p.set("resource_id", f.resource);
  if (f.from) p.set("from", `${f.from}T00:00:00Z`);
  if (f.to) p.set("to", `${f.to}T23:59:59.999Z`);
  if (forExport) p.set("format", "csv");
  else {
    p.set("limit", String(q.limit));
    if (q.cursor) p.set("cursor", q.cursor);
  }
  return p;
}

async function loadAudit(q: TableQuery, signal: AbortSignal): Promise<TablePage<AuditEvent>> {
  const r = await fetch(`${API}?${auditParams(q)}`, { credentials: "same-origin", signal });
  if (r.status === 403) throw new Error("You need admin privileges to view the audit log.");
  if (!r.ok) throw new Error(`Could not load the audit log (HTTP ${r.status})`);
  const d = (await r.json()) as { items: AuditEvent[]; next_cursor?: string; total_estimate?: number; total_capped?: boolean };
  return { items: d.items ?? [], next_cursor: d.next_cursor, total_estimate: d.total_estimate, total_capped: d.total_capped };
}

function ActorFilter({ value, set }: { value: string; set: (v: string) => void }) {
  const picked = usePeopleByIds(value ? [value] : []);
  const [local, setLocal] = useState<DirectoryItem[]>([]);
  const shown = value ? (local.find((l) => l.id === value) ? local : picked) : [];
  return (
    <Box sx={{ width: 230 }}>
      <PeoplePicker
        ariaLabel="Actor"
        placeholder="Actor (search people)"
        value={shown}
        hideMe={false}
        onChange={(n) => {
          setLocal(n);
          set(n[0]?.id ?? "");
        }}
        testId="audit-actor"
      />
    </Box>
  );
}

/** Admin audit log: server-side filters, keyset paging, CSV export and a live refresh. */
export function AuditLogSection() {
  const [live, setLive] = useState(false);
  const [tick, setTick] = useState(0);
  useEffect(() => {
    if (!live) return;
    const t = setInterval(() => setTick((n) => n + 1), 5000);
    return () => clearInterval(t);
  }, [live]);

  const filters: FilterDef[] = useMemo(
    () => [
      { key: "actor", label: "Actor", type: "custom", render: (v, set) => <ActorFilter value={v} set={set} /> },
      { key: "service", label: "Service", type: "select", options: SERVICES.map((s) => ({ value: s, label: s })) },
      { key: "status", label: "Status", type: "select", options: [{ value: "ok", label: "ok" }, { value: "error", label: "error" }] },
      { key: "action", label: "Action", type: "text" },
      { key: "resource", label: "Record id", type: "text" },
      { key: "from", label: "From date", type: "date" },
      { key: "to", label: "Up to date (jump to)", type: "date" },
    ],
    [],
  );

  const columns: Column<AuditEvent>[] = useMemo(
    () => [
      { key: "time", header: "Time", width: 170, render: (e) => new Date(e.created_at).toLocaleString() },
      { key: "actor", header: "Actor", width: 220, render: (e) => e.actor_email || "system" },
      { key: "service", header: "Service", width: 100, render: (e) => e.service },
      { key: "action", header: "Action", width: 100, render: (e) => e.action },
      { key: "resource", header: "Record", width: 180, render: (e) => [e.resource_type, e.resource_id].filter(Boolean).join(" ") },
      {
        key: "status",
        header: "Status",
        width: 90,
        render: (e) => (
          <Chip size="sm" variant="soft" color={e.status === "ok" ? "success" : "danger"}>
            {e.status}
          </Chip>
        ),
      },
      { key: "ip", header: "IP", width: 130, render: (e) => e.ip },
    ],
    [],
  );

  const exportUrl = useCallback((q: TableQuery) => `${API}?${auditParams(q, true)}`, []);

  return (
    <>
      <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, mb: 1.5, flexWrap: "wrap" }}>
        <Box sx={{ flex: 1, minWidth: 220 }}>
          <Typography level="h4">Audit log</Typography>
          <Typography level="body-sm" sx={{ opacity: 0.7 }}>
            Every service in your organisation. Filters run on the server, so it stays fast with millions of rows.
          </Typography>
        </Box>
        <Switch checked={live} onChange={(e) => setLive(e.target.checked)} endDecorator="Live (refresh every 5 s)" slotProps={{ input: { "aria-label": "Live refresh" } }} />
      </Box>
      <Alert variant="soft" color="neutral" sx={{ mb: 1.5 }}>
        Dates are interpreted in UTC. Choose "Up to date" to jump back to a day.
      </Alert>
      <DataTable
        testId="audit-table"
        columns={columns}
        rowKey={(e) => e.id}
        load={loadAudit}
        mode="cursor"
        pageSizes={[25, 50, 100]}
        filters={filters}
        search={false}
        exportUrl={exportUrl}
        reloadKey={tick}
        emptyText="No audit events yet"
      />
    </>
  );
}
