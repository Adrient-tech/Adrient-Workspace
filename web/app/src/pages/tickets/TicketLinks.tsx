import { useCallback, useEffect, useState } from "react";
import { Alert, Box, Button, Chip, Typography } from "@mui/joy";
import { findPersonByEmail, type Person } from "../../api/people";

interface Link { app: string; label: string; href: string }

/** Records linked to a ticket (Mail, Chat, Projects, CRM), a Projects issue button and the requester's directory entry. */
export function TicketLinks({ ticketId, requesterEmail }: { ticketId: string; requesterEmail: string }) {
  const [links, setLinks] = useState<Link[]>([]);
  const [person, setPerson] = useState<Person | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const load = useCallback(() => {
    fetch(`/api/v1/bridge/links?app=tickets&type=ticket&id=${encodeURIComponent(ticketId)}`, { credentials: "same-origin" })
      .then((r) => (r.ok ? r.json() : { links: [] }))
      .then((d: { links?: Link[] }) => setLinks(d.links ?? []))
      .catch(() => setLinks([]));
  }, [ticketId]);
  useEffect(load, [load]);
  useEffect(() => {
    setPerson(null);
    if (requesterEmail) findPersonByEmail(requesterEmail).then(setPerson).catch(() => setPerson(null));
  }, [requesterEmail]);
  async function issue() {
    setBusy(true);
    setErr("");
    try {
      const r = await fetch(`/api/v1/ts/tickets/${ticketId}/issue`, { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: "{}" });
      if (!r.ok) throw new Error((await r.text()).trim() || `HTTP ${r.status}`);
      load();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  const hasIssue = links.some((l) => l.app === "projects");
  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 0.75 }} data-testid="ticket-links">
      {person && (
        <Box data-testid="directory-card" sx={{ p: 1, borderRadius: "md", bgcolor: "background.level1" }}>
          <Typography level="body-xs" sx={{ color: "text.tertiary", textTransform: "uppercase" }}>In the directory</Typography>
          <Typography level="title-sm">{person.name}</Typography>
          <Typography level="body-xs">{[person.job_title, person.department_name].filter(Boolean).join(" · ")}</Typography>
          {person.manager_name && <Typography level="body-xs" sx={{ color: "text.tertiary" }}>Reports to {person.manager_name}{person.location ? ` · ${person.location}` : ""}</Typography>}
        </Box>
      )}
      <Typography level="title-sm">Linked records</Typography>
      {links.length === 0 && <Typography level="body-xs" sx={{ color: "text.tertiary" }}>Nothing linked yet.</Typography>}
      <Box sx={{ display: "flex", gap: 0.5, flexWrap: "wrap" }}>
        {links.map((l, i) => (
          <Chip key={i} size="sm" variant="soft" slotProps={{ action: { component: "a", href: l.href } as never }}>{l.label || l.app}</Chip>
        ))}
      </Box>
      {!hasIssue && <Button size="sm" variant="outlined" loading={busy} onClick={issue} data-testid="make-issue">Create a Projects issue</Button>}
      {err && <Alert color="warning" size="sm">{err}</Alert>}
    </Box>
  );
}
