// Audit log search (text, dates, service) and CSV export.
import { useCallback, useEffect, useState } from "react";
import { Box, Button, FormControl, FormLabel, Input, Table } from "@mui/joy";
import { Notice, Page, api, when } from "./common";

interface Ev {
  id: string;
  created_at: string;
  actor_email: string;
  service: string;
  action: string;
  resource: string;
  status: string;
  ip: string;
}

export function AuditSearchSection() {
  const [q, setQ] = useState("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [service, setService] = useState("");
  const [rows, setRows] = useState<Ev[]>([]);
  const [error, setError] = useState<string | null>(null);

  const qs = useCallback(() => {
    const p = new URLSearchParams();
    if (q.trim()) p.set("q", q.trim());
    if (from) p.set("from", from);
    if (to) p.set("to", to);
    if (service.trim()) p.set("service", service.trim());
    return p;
  }, [q, from, to, service]);

  const run = useCallback(async () => {
    setError(null);
    try {
      setRows((await api<{ events: Ev[] }>(`/api/v1/admin/audit/search?${qs().toString()}`)).events);
    } catch (e) {
      setError((e as Error).message);
    }
  }, [qs]);
  useEffect(() => {
    void run();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const exportUrl = `/api/v1/admin/audit/search?${(() => {
    const p = qs();
    p.set("format", "csv");
    return p.toString();
  })()}`;

  return (
    <Page testId="auditsearch" title="Audit log search" intro="Search every recorded admin and user action by text, service and date, and download the result as CSV. Exports are themselves recorded in the audit log.">
      <Notice error={error} />
      <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap", alignItems: "flex-end", mb: 2 }}>
        <FormControl size="sm">
          <FormLabel>Text</FormLabel>
          <Input size="sm" value={q} onChange={(e) => setQ(e.target.value)} placeholder="person, action or resource" onKeyDown={(e) => e.key === "Enter" && void run()} slotProps={{ input: { "data-testid": "audit-q" } as never }} />
        </FormControl>
        <FormControl size="sm">
          <FormLabel>Service</FormLabel>
          <Input size="sm" sx={{ width: 140 }} value={service} onChange={(e) => setService(e.target.value)} placeholder="identity" />
        </FormControl>
        <FormControl size="sm">
          <FormLabel>From</FormLabel>
          <Input size="sm" type="date" value={from} onChange={(e) => setFrom(e.target.value)} />
        </FormControl>
        <FormControl size="sm">
          <FormLabel>To</FormLabel>
          <Input size="sm" type="date" value={to} onChange={(e) => setTo(e.target.value)} />
        </FormControl>
        <Button size="sm" onClick={() => void run()} data-testid="audit-go">
          Search
        </Button>
        <Button size="sm" variant="outlined" component="a" href={exportUrl} data-testid="audit-export">
          Export CSV
        </Button>
      </Box>
      <Table size="sm" sx={{ maxWidth: 1000 }} data-testid="audit-rows">
        <thead>
          <tr>
            <th>When</th>
            <th>Person</th>
            <th>Service</th>
            <th>Action</th>
            <th>Resource</th>
            <th>Result</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr key={r.id}>
              <td>{when(r.created_at)}</td>
              <td>{r.actor_email}</td>
              <td>{r.service}</td>
              <td>{r.action}</td>
              <td>{r.resource}</td>
              <td>{r.status}</td>
            </tr>
          ))}
          {rows.length === 0 && (
            <tr>
              <td colSpan={6}>No matching events.</td>
            </tr>
          )}
        </tbody>
      </Table>
    </Page>
  );
}
