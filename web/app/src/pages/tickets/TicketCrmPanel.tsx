// CRM link of a ticket: the company or person it belongs to, matched by the
// requester's e-mail or linked by hand. Hidden when the user cannot open CRM.
import { useCallback, useEffect, useState } from "react";
import { Box, Button, Chip, Option, Select, Typography } from "@mui/joy";

interface Match { kind: "company" | "person"; id: string; name: string; href: string; source: "link" | "email" }
interface Company { id: string; name: string }

async function jf<T>(url: string, init?: RequestInit): Promise<T> {
  const r = await fetch(url, { credentials: "same-origin", headers: { "Content-Type": "application/json" }, ...init });
  if (!r.ok) throw new Error(r.status === 403 ? "no access" : `HTTP ${r.status}`);
  return (await r.json()) as T;
}

export function TicketCrmPanel({ ticketId }: { ticketId: string }) {
  const [matches, setMatches] = useState<Match[] | null>(null);
  const [companies, setCompanies] = useState<Company[]>([]);
  const [pick, setPick] = useState("");
  const [err, setErr] = useState("");
  const load = useCallback(() => {
    jf<{ crm: Match[] }>(`/api/v1/bridge/ticket-crm?ticket_id=${encodeURIComponent(ticketId)}`)
      .then((r) => setMatches(r.crm ?? []))
      .catch(() => setMatches(null));
  }, [ticketId]);
  useEffect(load, [load]);
  useEffect(() => {
    if (matches === null) return;
    jf<{ companies: Company[] }>("/api/v1/crm/companies").then((r) => setCompanies(r.companies ?? [])).catch(() => setCompanies([]));
  }, [matches === null]); // eslint-disable-line react-hooks/exhaustive-deps
  if (matches === null) return null;
  const link = async (remove: boolean, to: { type: string; id: string }) => {
    setErr("");
    try {
      await jf(`/api/v1/bridge/${remove ? "unlink" : "link"}`, { method: "POST", body: JSON.stringify({ from_app: "tickets", from_type: "ticket", from_id: ticketId, to_app: "crm", to_type: to.type, to_id: to.id, label: "CRM " + to.type }) });
      setPick("");
      load();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "failed");
    }
  };
  return (
    <Box sx={{ mt: 1.5 }} data-testid="ticket-crm">
      <Typography level="title-sm">CRM</Typography>
      <Box sx={{ display: "flex", gap: 0.75, flexWrap: "wrap", alignItems: "center", mt: 0.5 }}>
        {matches.length === 0 && <Typography level="body-xs">Not linked to a CRM company or person.</Typography>}
        {matches.map((m) => (
          <Box key={m.kind + m.id + m.source} sx={{ display: "flex", gap: 0.5, alignItems: "center" }}>
            <Button size="sm" variant="soft" component="a" href={m.href}>Open {m.name} in CRM</Button>
            <Chip size="sm" variant="outlined">{m.source === "link" ? "linked" : "matched by e-mail"}</Chip>
            {m.source === "link" && <Button size="sm" variant="plain" color="neutral" onClick={() => link(true, { type: m.kind, id: m.id })}>Unlink</Button>}
          </Box>
        ))}
        {companies.length > 0 && (
          <>
            <Select size="sm" placeholder="Link to company" value={pick || null} onChange={(_, v) => setPick(v ?? "")} sx={{ minWidth: 200 }} data-testid="ticket-crm-pick">
              {companies.map((c) => <Option key={c.id} value={c.id}>{c.name}</Option>)}
            </Select>
            <Button size="sm" disabled={!pick} onClick={() => link(false, { type: "company", id: pick })} data-testid="ticket-crm-link">Link</Button>
          </>
        )}
      </Box>
      {err && <Typography level="body-xs" color="danger">{err}</Typography>}
    </Box>
  );
}
