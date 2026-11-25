import { PersonSelect } from "../../components/PeoplePicker";
import { useEffect, useMemo, useState } from "react";
import { Box, Button, Checkbox, Chip, FormControl, FormLabel, Input, Option, Select, Sheet, Stack, Table, Textarea, Typography } from "@mui/joy";
import * as api from "./api";
import { dealContacts, isCold, lastTouch, money, toCSV } from "./logic";
import type { Company, Person } from "./types";
import { ConfirmButton, ContactPicker, Dialog, Empty, ErrorAlert, ago, companyName, downloadCSV, fmtDate, useErr } from "./ui";
import type { Data } from "./ui";
import { ActivityForm, FollowupForm } from "./Agenda";
import { CustomFieldInputs, CustomFilter, defsFor, matchesCustom, showValue } from "./CustomFields";
import { SavedViews } from "./SavedViews";
import { Customer360Button } from "./Customer360";
import { LogCallDialog, ScheduleMeetingDialog, SendMailDialog, firstPhone, telHref, whatsappHref } from "./Comms";
import { slaState } from "./sla";

const tagsOf = (s: string) => s.split(",").map((t) => t.trim()).filter(Boolean);

export function Leads({ data, focusId = "" }: { data: Data; focusId?: string }) {
  const [q, setQ] = useState("");
  const [status, setStatus] = useState("");
  const [cold, setCold] = useState(false);
  const [editing, setEditing] = useState<Person | "new" | null>(null);
  const [cf, setCf] = useState("");
  const [cfv, setCfv] = useState("");
  const s = data.settings;
  const defs = useMemo(() => defsFor(data.fields, "person"), [data.fields]);
  useEffect(() => {
    const p = focusId ? data.people.find((x) => x.id === focusId) : undefined;
    if (p) setEditing(p);
  }, [focusId, data.people]);

  // The loaded list is the newest 1,000 leads; a search runs on the server.
  const found = api.useServerSearch(q, (t) => api.searchPeople(t, status));
  const rows = useMemo(() => {
    const ql = q.trim().toLowerCase();
    return (found ?? data.people)
      .map((p) => ({ p, last: lastTouch(data.activities, [p.contact_id]) }))
      .filter(({ p, last }) => {
        if (status && p.status !== status) return false;
        if (cold && !isCold(last, s.cold_days)) return false;
        if (cf && !matchesCustom(defs.find((d) => d.key === cf), p.custom, cfv, data.users)) return false;
        if (!found && ql && !`${p.display_name} ${(p.emails ?? []).join(" ")} ${companyName(data, p.company_id)} ${p.tags.join(" ")}`.toLowerCase().includes(ql)) return false;
        return true;
      })
      .sort((a, b) => (cold ? a.last - b.last : a.p.display_name.localeCompare(b.p.display_name)));
  }, [data, found, q, status, cold, s.cold_days, cf, cfv, defs]);

  const exportCSV = () =>
    downloadCSV(
      "crm-leads.csv",
      toCSV([["name", "email", "company", "status", "source", "tags", "score", ...defs.map((d) => `custom:${d.key}`)], ...data.people.map((p) => [p.display_name, (p.emails ?? [])[0] ?? "", companyName(data, p.company_id), p.status, p.source, p.tags.join(";"), String(p.score ?? 0), ...defs.map((d) => showValue(d, p.custom?.[d.key], data.users))])]),
    );

  return (
    <Box>
      <Stack direction="row" gap={1} flexWrap="wrap" alignItems="center" sx={{ mb: 1.5 }}>
        <Typography level="h4" sx={{ flex: 1 }}>
          Leads
        </Typography>
        <Input size="sm" placeholder="Search leads" value={q} onChange={(e) => setQ(e.target.value)} slotProps={{ input: { "aria-label": "Search leads" } }} />
        <Select size="sm" value={status} onChange={(_, v) => setStatus(v ?? "")} sx={{ width: 130 }} aria-label="Status filter">
          <Option value="">All statuses</Option>
          <Option value="lead">Lead</Option>
          <Option value="customer">Customer</Option>
          <Option value="inactive">Inactive</Option>
        </Select>
        <Checkbox size="sm" label="Going cold" checked={cold} onChange={(e) => setCold(e.target.checked)} data-testid="crm-cold-filter" />
        <CustomFilter defs={defs} field={cf} needle={cfv} onChange={(a, b) => { setCf(a); setCfv(b); }} />
        <Button size="sm" variant="outlined" onClick={exportCSV}>
          Export CSV
        </Button>
        <Button onClick={() => setEditing("new")} data-testid="crm-new-lead">
          New lead
        </Button>
      </Stack>
      <SavedViews
        entity="lead"
        getConfig={() => ({ q, status, cold, cf, cfv })}
        onApply={(c) => { setQ(String(c.q ?? "")); setStatus(String(c.status ?? "")); setCold(!!c.cold); setCf(String(c.cf ?? "")); setCfv(String(c.cfv ?? "")); }}
        onClear={() => { setQ(""); setStatus(""); setCold(false); setCf(""); setCfv(""); }}
      />
      {!q && data.people.length >= 1000 && (
        <Typography level="body-xs" sx={{ mb: 1, opacity: 0.75 }} data-testid="crm-leads-window">
          Showing the newest 1,000 leads. Search to find any lead.
        </Typography>
      )}
      {rows.length === 0 && <Empty>No leads yet. Add a lead from your Contacts or create a new one.</Empty>}
      {rows.length > 0 && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table hoverRow data-testid="crm-lead-list" sx={{ tableLayout: "auto", "& th, & td": { whiteSpace: "nowrap" }, "& td:first-of-type, & th:first-of-type": { maxWidth: 300, overflow: "hidden", textOverflow: "ellipsis" } }}>
            <thead>
              <tr>
                <th>Name</th>
                <th>Company</th>
                <th>Status</th>
                <th>Source</th>
                <th>Last touch</th>
                <th>Score</th>
                <th>Response</th>
                {defs.slice(0, 2).map((d) => <th key={d.id}>{d.label}</th>)}
              </tr>
            </thead>
            <tbody>
              {rows.map(({ p, last }) => (
                <tr key={p.id} onClick={() => setEditing(p)} style={{ cursor: "pointer" }}>
                  <td>
                    {p.display_name}
                    <Typography level="body-xs">{(p.emails ?? [])[0]}</Typography>
                  </td>
                  <td>{companyName(data, p.company_id)}</td>
                  <td>
                    <Chip size="sm" variant="soft" color={p.status === "customer" ? "success" : p.status === "lead" ? "primary" : "neutral"}>
                      {p.status}
                    </Chip>
                  </td>
                  <td>{p.source}</td>
                  <td>
                    {ago(last)}
                    {isCold(last, s.cold_days) && (
                      <Chip size="sm" sx={{ ml: 0.5 }} variant="soft">
                        cold
                      </Chip>
                    )}
                  </td>
                  <td data-testid="crm-lead-score">{p.score ?? 0}</td>
                  <td>
                    {(() => {
                      const st = slaState(p.sla_due_at, p.first_response_at);
                      return st === "none" ? "" : <Chip size="sm" variant="soft" color={st === "breached" ? "danger" : st === "ok" ? "success" : "warning"} data-testid={`crm-sla-${st}`}>{st === "breached" ? "SLA missed" : st === "ok" ? "answered" : "due soon"}</Chip>;
                    })()}
                  </td>
                  {defs.slice(0, 2).map((d) => <td key={d.id}>{showValue(d, p.custom?.[d.key], data.users)}</td>)}
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
      {editing && (
        <LeadDialog
          data={data}
          person={editing === "new" ? null : editing}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            data.reload();
          }}
        />
      )}
    </Box>
  );
}

function LeadDialog({ data, person, onClose, onSaved }: { data: Data; person: Person | null; onClose: () => void; onSaved: () => void }) {
  const [contactId, setContactId] = useState(person?.contact_id ?? "");
  const [newName, setNewName] = useState("");
  const [newEmail, setNewEmail] = useState("");
  const [companyId, setCompanyId] = useState(person?.company_id ?? "");
  const [status, setStatus] = useState<string>(person?.status ?? "lead");
  const [source, setSource] = useState(person?.source ?? "");
  const [tags, setTags] = useState((person?.tags ?? []).join(", "));
  const [notes, setNotes] = useState(person?.notes ?? "");
  const [custom, setCustom] = useState<Record<string, unknown>>(person?.custom ?? {});
  const [owner, setOwner] = useState(person?.owner_id ?? "");
  const [sub, setSub] = useState<"" | "followup" | "activity" | "mail" | "call" | "meeting">("");
  const { err, run } = useErr();
  const deals = person ? data.deals.filter((d) => dealContacts(d).includes(person.contact_id)) : [];
  const acts = person ? data.activities.filter((a) => a.contact_id === person.contact_id).slice(0, 8) : [];
  const fups = person ? data.followups.filter((f) => f.contact_id === person.contact_id && !f.done) : [];

  const save = async () => {
    const r = await run(async () => {
      let cid = contactId;
      if (!person && !cid) {
        const c = await api.createContact({ display_name: newName.trim(), emails: newEmail.trim() ? [newEmail.trim()] : [] });
        cid = c.id;
      }
      return api.savePerson(person?.id ?? "", { contact_id: cid, company_id: companyId, status: status as Person["status"], source, tags: tagsOf(tags), notes, custom, owner_id: owner } as Partial<Person>);
    });
    if (r) onSaved();
  };
  return (
    <Dialog title={person ? person.display_name : "New lead"} onClose={onClose} width={620}>
      <Stack gap={1.5}>
        {!person && (
          <>
            <FormControl>
              <FormLabel>Existing contact</FormLabel>
              <ContactPicker contacts={data.contacts} value={contactId} onChange={setContactId} testid="crm-lead-contact" />
            </FormControl>
            {!contactId && (
              <Stack direction="row" gap={1}>
                <FormControl sx={{ flex: 1 }}>
                  <FormLabel>…or new contact name</FormLabel>
                  <Input value={newName} onChange={(e) => setNewName(e.target.value)} slotProps={{ input: { "data-testid": "crm-lead-name" } }} />
                </FormControl>
                <FormControl sx={{ flex: 1 }}>
                  <FormLabel>Email</FormLabel>
                  <Input type="email" value={newEmail} onChange={(e) => setNewEmail(e.target.value)} slotProps={{ input: { "data-testid": "crm-lead-email" } }} />
                </FormControl>
              </Stack>
            )}
          </>
        )}
        <Stack direction="row" gap={1}>
          <FormControl sx={{ flex: 1 }}>
            <FormLabel>Company</FormLabel>
            <Select value={companyId} onChange={(_, v) => setCompanyId(v ?? "")} aria-label="Company">
              <Option value="">None</Option>
              {data.companies.map((c) => (
                <Option key={c.id} value={c.id}>
                  {c.name}
                </Option>
              ))}
            </Select>
          </FormControl>
          <FormControl sx={{ flex: 1 }}>
            <FormLabel>Status</FormLabel>
            <Select value={status} onChange={(_, v) => setStatus(v ?? "lead")} aria-label="Status">
              <Option value="lead">Lead</Option>
              <Option value="customer">Customer</Option>
              <Option value="inactive">Inactive</Option>
            </Select>
          </FormControl>
        </Stack>
        <Stack direction="row" gap={1}>
          <FormControl sx={{ flex: 1 }}>
            <FormLabel>Source</FormLabel>
            <Input value={source} onChange={(e) => setSource(e.target.value)} />
          </FormControl>
          <FormControl sx={{ flex: 1 }}>
            <FormLabel>Tags (comma separated)</FormLabel>
            <Input value={tags} onChange={(e) => setTags(e.target.value)} />
          </FormControl>
        </Stack>
        <FormControl>
          <FormLabel>Owner</FormLabel>
          <PersonSelect value={owner} known={data.users} onChange={(id) => setOwner(id)} placeholder={person ? "Keep current owner" : "Me"} ariaLabel="Owner" testId="crm-lead-owner" />
        </FormControl>
        <CustomFieldInputs defs={defsFor(data.fields, "person")} value={custom} onChange={setCustom} users={data.users} />
        <FormControl>
          <FormLabel>Notes</FormLabel>
          <Textarea minRows={2} value={notes} onChange={(e) => setNotes(e.target.value)} />
        </FormControl>
        {person && (
          <>
            <Typography level="title-sm">Deals ({deals.length})</Typography>
            {deals.map((d) => (
              <Typography key={d.id} level="body-sm">
                {d.title} · {d.stage} · {money(d.value, d.currency)}
              </Typography>
            ))}
            <Typography level="title-sm">Open follow-ups ({fups.length})</Typography>
            {fups.map((f) => (
              <Typography key={f.id} level="body-sm">
                {f.title} · {fmtDate(f.due_date)}
              </Typography>
            ))}
            <Typography level="title-sm">Recent activity</Typography>
            {acts.map((a) => (
              <Typography key={a.id} level="body-sm">
                {a.kind} · {fmtDate(a.occurred_at)} · {a.summary}
              </Typography>
            ))}
            <Stack direction="row" gap={1}>
              <Button size="sm" variant="soft" onClick={() => setSub("followup")}>
                Schedule follow-up
              </Button>
              <Button size="sm" variant="soft" onClick={() => setSub("activity")}>
                Log activity
              </Button>
            </Stack>
            <Stack direction="row" gap={1} flexWrap="wrap" data-testid="crm-lead-comms">
              {person.emails?.[0] && <Button size="sm" variant="soft" onClick={() => setSub("mail")} data-testid="crm-lead-email-btn">Email</Button>}
              <Button size="sm" variant="soft" onClick={() => setSub("call")} data-testid="crm-lead-call-btn">Log call</Button>
              <Button size="sm" variant="soft" onClick={() => setSub("meeting")} data-testid="crm-lead-meeting-btn">Schedule meeting</Button>
              {telHref(firstPhone(person.phones)) && <Button size="sm" variant="plain" component="a" href={telHref(firstPhone(person.phones))}>Call {firstPhone(person.phones)}</Button>}
              {whatsappHref(firstPhone(person.phones)) && <Button size="sm" variant="plain" component="a" href={whatsappHref(firstPhone(person.phones))} target="_blank" rel="noopener noreferrer" data-testid="crm-lead-whatsapp">WhatsApp</Button>}
              <Customer360Button kind="person" id={person.id} />
            </Stack>
          </>
        )}
        <ErrorAlert err={err} />
        <Stack direction="row" justifyContent="space-between">
          {person ? (
            <ConfirmButton
              label="Remove from CRM"
              testid="crm-lead-delete"
              onConfirm={async () => {
                if (await run(() => api.deletePerson(person.id))) onSaved();
              }}
            />
          ) : (
            <span />
          )}
          <Button data-testid="crm-lead-save" disabled={!person && !contactId && !newName.trim()} onClick={save}>
            Save
          </Button>
        </Stack>
      </Stack>
      {sub === "followup" && person && <FollowupForm data={data} contactId={person.contact_id} onClose={() => setSub("")} onSaved={() => { setSub(""); data.reload(); }} />}
      {sub === "mail" && person && person.emails?.[0] && <SendMailDialog personId={person.id} email={person.emails[0]} onClose={() => setSub("")} onSent={() => { setSub(""); data.reload(); }} />}
      {sub === "call" && person && <LogCallDialog contactId={person.contact_id} onClose={() => setSub("")} onSaved={() => { setSub(""); data.reload(); }} />}
      {sub === "meeting" && person && <ScheduleMeetingDialog data={data} contactId={person.contact_id} onClose={() => setSub("")} onSaved={() => { setSub(""); data.reload(); }} />}
      {sub === "activity" && person && <ActivityForm data={data} contactId={person.contact_id} onClose={() => setSub("")} onSaved={() => { setSub(""); data.reload(); }} />}
    </Dialog>
  );
}

export function Companies({ data, focusId = "" }: { data: Data; focusId?: string }) {
  const [editing, setEditing] = useState<Company | "new" | null>(null);
  const [q, setQ] = useState("");
  const [cf, setCf] = useState("");
  const [cfv, setCfv] = useState("");
  const defs = useMemo(() => defsFor(data.fields, "company"), [data.fields]);
  useEffect(() => {
    const c = focusId ? data.companies.find((x) => x.id === focusId) : undefined;
    if (c) setEditing(c);
  }, [focusId, data.companies]);
  // The loaded list is the first 1,000 companies; a search runs on the server.
  const found = api.useServerSearch(q, (t) => api.searchCompanies(t));
  const list = (found ?? data.companies).filter((c) => (found || !q || `${c.name} ${c.industry} ${c.tags.join(" ")}`.toLowerCase().includes(q.toLowerCase())) && (!cf || matchesCustom(defs.find((d) => d.key === cf), c.custom, cfv, data.users)));
  return (
    <Box>
      <Stack direction="row" gap={1} alignItems="center" sx={{ mb: 1.5 }}>
        <Typography level="h4" sx={{ flex: 1 }}>
          Companies
        </Typography>
        <Input size="sm" placeholder="Search companies" value={q} onChange={(e) => setQ(e.target.value)} slotProps={{ input: { "aria-label": "Search companies" } }} />
        <CustomFilter defs={defs} field={cf} needle={cfv} onChange={(a, b) => { setCf(a); setCfv(b); }} />
        <Button
          size="sm"
          variant="outlined"
          onClick={() => downloadCSV("crm-companies.csv", toCSV([["name", "industry", "website", "phone", ...defs.map((d) => `custom:${d.key}`)], ...data.companies.map((c) => [c.name, c.industry, c.website, c.phone, ...defs.map((d) => showValue(d, c.custom?.[d.key], data.users))])]))}
        >
          Export CSV
        </Button>
        <Button onClick={() => setEditing("new")} data-testid="crm-new-company">
          New company
        </Button>
      </Stack>
      <SavedViews entity="company" getConfig={() => ({ q, cf, cfv })} onApply={(c) => { setQ(String(c.q ?? "")); setCf(String(c.cf ?? "")); setCfv(String(c.cfv ?? "")); }} onClear={() => { setQ(""); setCf(""); setCfv(""); }} />
      {!q && data.companies.length >= 1000 && (
        <Typography level="body-xs" sx={{ mb: 1, opacity: 0.75 }}>
          Showing the first 1,000 companies. Search to find any company.
        </Typography>
      )}
      {list.length === 0 && <Empty>{q ? "No companies match." : "No companies yet."}</Empty>}
      {list.length > 0 && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table hoverRow data-testid="crm-company-list" sx={{ tableLayout: "auto", "& th, & td": { whiteSpace: "nowrap" }, "& td:first-of-type, & th:first-of-type": { maxWidth: 300, overflow: "hidden", textOverflow: "ellipsis" } }}>
            <thead>
              <tr>
                <th>Company</th>
                <th>Industry</th>
                <th>People</th>
                <th>Open deals</th>
                <th style={{ textAlign: "right" }}>Open value</th>
                {defs.slice(0, 2).map((d) => <th key={d.id}>{d.label}</th>)}
              </tr>
            </thead>
            <tbody>
              {list.map((c) => {
                const od = data.deals.filter((d) => d.company_id === c.id && d.stage !== "won" && d.stage !== "lost");
                return (
                  <tr key={c.id} onClick={() => setEditing(c)} style={{ cursor: "pointer" }}>
                    <td>{c.name}</td>
                    <td>{c.industry}</td>
                    <td>{data.people.filter((p) => p.company_id === c.id).length}</td>
                    <td>{od.length}</td>
                    <td style={{ textAlign: "right" }}>{money(od.reduce((a, d) => a + d.value, 0), data.settings.currency)}</td>
                    {defs.slice(0, 2).map((d) => <td key={d.id}>{showValue(d, c.custom?.[d.key], data.users)}</td>)}
                  </tr>
                );
              })}
            </tbody>
          </Table>
        </Sheet>
      )}
      {editing && (
        <CompanyDialog
          data={data}
          company={editing === "new" ? null : editing}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            data.reload();
          }}
        />
      )}
    </Box>
  );
}

function CompanyDialog({ data, company, onClose, onSaved }: { data: Data; company: Company | null; onClose: () => void; onSaved: () => void }) {
  const [f, setF] = useState({ name: company?.name ?? "", industry: company?.industry ?? "", website: company?.website ?? "", phone: company?.phone ?? "", address: company?.address ?? "", notes: company?.notes ?? "", tags: (company?.tags ?? []).join(", ") });
  const [custom, setCustom] = useState<Record<string, unknown>>(company?.custom ?? {});
  const [owner, setOwner] = useState(company?.owner_id ?? "");
  const { err, run } = useErr();
  const set = (k: keyof typeof f, v: string) => setF((p) => ({ ...p, [k]: v }));
  const deals = company ? data.deals.filter((d) => d.company_id === company.id) : [];
  const people = company ? data.people.filter((p) => p.company_id === company.id) : [];
  return (
    <Dialog title={company ? company.name : "New company"} onClose={onClose}>
      <Stack gap={1.5}>
        <FormControl required>
          <FormLabel>Name</FormLabel>
          <Input autoFocus value={f.name} onChange={(e) => set("name", e.target.value)} slotProps={{ input: { "data-testid": "crm-company-name" } }} />
        </FormControl>
        <Stack direction="row" gap={1}>
          <FormControl sx={{ flex: 1 }}>
            <FormLabel>Industry</FormLabel>
            <Input value={f.industry} onChange={(e) => set("industry", e.target.value)} />
          </FormControl>
          <FormControl sx={{ flex: 1 }}>
            <FormLabel>Website</FormLabel>
            <Input value={f.website} onChange={(e) => set("website", e.target.value)} />
          </FormControl>
        </Stack>
        <Stack direction="row" gap={1}>
          <FormControl sx={{ flex: 1 }}>
            <FormLabel>Phone</FormLabel>
            <Input value={f.phone} onChange={(e) => set("phone", e.target.value)} />
          </FormControl>
          <FormControl sx={{ flex: 1 }}>
            <FormLabel>Tags</FormLabel>
            <Input value={f.tags} onChange={(e) => set("tags", e.target.value)} />
          </FormControl>
        </Stack>
        <FormControl>
          <FormLabel>Address</FormLabel>
          <Input value={f.address} onChange={(e) => set("address", e.target.value)} />
        </FormControl>
        <FormControl>
          <FormLabel>Owner</FormLabel>
          <PersonSelect value={owner} known={data.users} onChange={(id) => setOwner(id)} placeholder={company ? "Keep current owner" : "Me"} ariaLabel="Owner" />
        </FormControl>
        <CustomFieldInputs defs={defsFor(data.fields, "company")} value={custom} onChange={setCustom} users={data.users} />
        <FormControl>
          <FormLabel>Notes</FormLabel>
          <Textarea minRows={2} value={f.notes} onChange={(e) => set("notes", e.target.value)} />
        </FormControl>
        {company && (
          <>
            <Typography level="title-sm">People ({people.length})</Typography>
            {people.map((p) => (
              <Typography key={p.id} level="body-sm">
                {p.display_name}
              </Typography>
            ))}
            <Typography level="title-sm">Deals ({deals.length})</Typography>
            {deals.map((d) => (
              <Typography key={d.id} level="body-sm">
                {d.title} · {d.stage} · {money(d.value, d.currency)}
              </Typography>
            ))}
          </>
        )}
        {company && <Customer360Button kind="company" id={company.id} />}
        <ErrorAlert err={err} />
        <Stack direction="row" justifyContent="space-between">
          {company ? (
            <ConfirmButton
              label="Delete"
              testid="crm-company-delete"
              onConfirm={async () => {
                if (await run(() => api.deleteCompany(company.id))) onSaved();
              }}
            />
          ) : (
            <span />
          )}
          <Button
            disabled={!f.name.trim()}
            data-testid="crm-company-save"
            onClick={async () => {
              if (await run(() => api.saveCompany(company?.id ?? "", { ...f, tags: tagsOf(f.tags), custom, owner_id: owner }))) onSaved();
            }}
          >
            Save
          </Button>
        </Stack>
      </Stack>
    </Dialog>
  );
}
