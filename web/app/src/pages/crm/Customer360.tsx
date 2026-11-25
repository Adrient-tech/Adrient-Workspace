import { useEffect, useState } from "react";
import { Box, Button, Chip, CircularProgress, Sheet, Stack, Typography } from "@mui/joy";
import { useNavigate } from "react-router-dom";
import * as api2 from "./api2";
import { money } from "./logic";
import { Dialog, ErrorAlert, fmtDate, useErr } from "./ui";

type Row = Record<string, unknown>;
interface C360 {
  kind: string;
  id: string;
  name: string;
  emails: string[];
  people: Row[];
  deals: Row[];
  followups: Row[];
  activities: Row[];
  meetings: Row[];
  mails: Row[];
  invoices: Row[];
  tickets: Row[];
  projects: Row[];
  files: Row[];
  paid_total: number;
  outstanding_total: number;
  links: { app: string; label: string; href: string }[];
  notes: string[];
}

const s = (v: unknown) => (v === null || v === undefined ? "" : String(v));

function Section({ title, count, testid, children }: { title: string; count: number; testid: string; children: React.ReactNode }) {
  return (
    <Sheet variant="outlined" sx={{ p: 1.25, borderRadius: "md" }} data-testid={testid}>
      <Typography level="title-sm" sx={{ mb: 0.5 }}>
        {title} ({count})
      </Typography>
      {count === 0 ? <Typography level="body-xs">Nothing yet.</Typography> : children}
    </Sheet>
  );
}

/** One page for a company or person: CRM records plus what the other apps know, with links that open them. */
export function Customer360Dialog({ kind, id, onClose }: { kind: "company" | "person"; id: string; onClose: () => void }) {
  const [c, setC] = useState<C360 | null>(null);
  const { err, run } = useErr();
  const nav = useNavigate();
  useEffect(() => {
    void run(() => api2.get<C360>(`360/${kind}/${id}`)).then((r) => r && setC(r));
  }, [kind, id, run]);
  const go = (href: string) => {
    onClose();
    nav(href);
  };
  const link = (href: string, label: string, testid: string) => (
    <Button size="sm" variant="plain" onClick={() => go(href)} data-testid={testid}>
      {label}
    </Button>
  );
  return (
    <Dialog title={c ? `${c.name} · customer 360` : "Customer 360"} onClose={onClose} width={900}>
      <ErrorAlert err={err} />
      {!c && !err && <CircularProgress />}
      {c && (
        <Stack gap={1.25} data-testid="crm-360">
          <Stack direction="row" gap={1} flexWrap="wrap" alignItems="center">
            {c.emails.map((e) => (
              <Chip key={e} size="sm" variant="soft">
                {e}
              </Chip>
            ))}
            <Chip size="sm" color="success" variant="soft" data-testid="crm-360-paid">
              Paid {money(c.paid_total, "INR")}
            </Chip>
            <Chip size="sm" color={c.outstanding_total > 0 ? "warning" : "neutral"} variant="soft" data-testid="crm-360-outstanding">
              Outstanding {money(c.outstanding_total, "INR")}
            </Chip>
          </Stack>
          {kind === "company" && (
            <Section title="People" count={c.people.length} testid="crm-360-people">
              {c.people.map((p) => (
                <Typography key={s(p.id)} level="body-sm">
                  {s(p.name)} · {s(p.status)}
                </Typography>
              ))}
            </Section>
          )}
          <Section title="Deals" count={c.deals.length} testid="crm-360-deals">
            {c.deals.map((d) => (
              <Typography key={s(d.id)} level="body-sm">
                {s(d.title)} · {s(d.stage)} · {money(Number(d.value), s(d.currency) || "INR")}
              </Typography>
            ))}
            {link("/crm", "Open CRM pipeline", "crm-360-open-crm")}
          </Section>
          <Section title="Mail (logged by you)" count={c.mails.length} testid="crm-360-mails">
            {c.mails.map((m) => (
              <Box key={s(m.thread_id) + s(m.sent_at)} sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                <Typography level="body-sm" sx={{ flex: 1 }}>
                  {s(m.direction) === "out" ? "Sent" : "Received"} · {s(m.subject) || "(no subject)"} · {fmtDate(s(m.sent_at))}
                </Typography>
                {link(`/mail?thread=${s(m.thread_id)}`, "Open in Mail", "crm-360-open-mail")}
              </Box>
            ))}
          </Section>
          <Section title="Meetings" count={c.meetings.length} testid="crm-360-meetings">
            {c.meetings.map((m) => (
              <Box key={s(m.id)} sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                <Typography level="body-sm" sx={{ flex: 1 }}>
                  {s(m.summary)} · {fmtDate(s(m.occurred_at))}
                </Typography>
                {link("/calendar", "Open Calendar", "crm-360-open-calendar")}
              </Box>
            ))}
          </Section>
          <Section title="Invoices and payments" count={c.invoices.length} testid="crm-360-invoices">
            {c.invoices.map((d) => (
              <Typography key={s(d.id)} level="body-sm">
                {s(d.number)} · {s(d.kind)} · {s(d.status)} · {money(Number(d.total), s(d.currency) || "INR")}
                {s(d.kind) === "invoice" ? ` · paid ${money(Number(d.paid), s(d.currency) || "INR")}` : ""}
              </Typography>
            ))}
            {link("/invoicing", "Open Invoicing", "crm-360-open-invoicing")}
          </Section>
          <Section title="Support tickets" count={c.tickets.length} testid="crm-360-tickets">
            {c.tickets.map((t) => (
              <Typography key={s(t.id)} level="body-sm">
                {s(t.key)} · {s(t.title)} · {s(t.status)}
              </Typography>
            ))}
            {link("/tickets", "Open Tickets", "crm-360-open-tickets")}
          </Section>
          <Section title="Projects (matched by name)" count={c.projects.length} testid="crm-360-projects">
            {c.projects.map((p) => (
              <Typography key={s(p.id)} level="body-sm">
                {s(p.name)} · {s(p.state)}
              </Typography>
            ))}
            {link("/projects", "Open Projects", "crm-360-open-projects")}
          </Section>
          <Section title="Files in your Drive (matched by name)" count={c.files.length} testid="crm-360-files">
            {c.files.map((f) => (
              <Box key={s(f.id)} sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                <Typography level="body-sm" sx={{ flex: 1 }}>
                  {s(f.name)}
                </Typography>
                {link(`/drive/file/${s(f.id)}`, "Open in Drive", "crm-360-open-drive")}
              </Box>
            ))}
          </Section>
          <Section title="Recent activity" count={c.activities.length} testid="crm-360-activity">
            {c.activities.slice(0, 10).map((a) => (
              <Typography key={s(a.id)} level="body-sm">
                {s(a.kind)} · {fmtDate(s(a.occurred_at))} · {s(a.summary)}
              </Typography>
            ))}
          </Section>
          {c.notes.map((n) => (
            <Typography key={n} level="body-xs">
              {n}
            </Typography>
          ))}
        </Stack>
      )}
    </Dialog>
  );
}

export function Customer360Button({ kind, id, label = "Customer 360" }: { kind: "company" | "person"; id: string; label?: string }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button size="sm" variant="outlined" onClick={() => setOpen(true)} data-testid={`crm-360-open-${kind}`}>
        {label}
      </Button>
      {open && <Customer360Dialog kind={kind} id={id} onClose={() => setOpen(false)} />}
    </>
  );
}
